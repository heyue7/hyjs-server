package service

import (
	"encoding/json"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p7AwakeFoodTestBinding(t *testing.T, f *p1Fixture, build int64, sessionMatches bool) {
	t.Helper()
	if err := db.GetIns().AutoMigrate(&db.ClientUpdateSession{}); err != nil {
		t.Fatal(err)
	}
	var profile db.Profile
	if err := db.GetIns().First(&profile, f.id).Error; err != nil {
		t.Fatal(err)
	}
	hash := clientUpdateSessionHash(profile.Session)
	if !sessionMatches {
		hash = strings.Repeat("0", 64)
	}
	if err := db.GetIns().Save(&db.ClientUpdateSession{PlayerID: f.id, SessionHash: hash, BuildCode: build}).Error; err != nil {
		t.Fatal(err)
	}
}

func TestP7AwakeFoodClientVersionGate(t *testing.T) {
	f := p7AwakeExploreFixture(t)
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
	if err := db.GetIns().AutoMigrate(&db.ClientUpdateSession{}); err != nil {
		t.Fatal(err)
	}
	entry := &pb.CSEnterAwakeCopy{CopyId: proto.String("501"), Items: []*pb.SimpleItem{{Id: proto.String("FishItem_026"), Num: proto.Int32(1)}}}
	checkBlocked := func() {
		t.Helper()
		if _, err := f.s.p7AwakeExplore(f.ctx, f.id, entry, now, 100); err == nil || !strings.Contains(err.Error(), "1.0.91") {
			t.Fatal("unsafe client passed food gate", err)
		}
	}
	checkBlocked()
	p7AwakeFoodTestBinding(t, f, 2026103104, true)
	checkBlocked()
	p7AwakeFoodTestBinding(t, f, 2026103105, false)
	checkBlocked()
	// An older client can still enter and leave when it carries nothing.
	p7AwakeFoodTestBinding(t, f, 2026103104, true)
	if _, err := f.s.p7AwakeExplore(f.ctx, f.id, &pb.CSEnterAwakeCopy{CopyId: proto.String("501")}, now, 101); err != nil {
		t.Fatal("empty entry was gated", err)
	}
	if _, err := f.s.p7AwakeExplore(f.ctx, f.id, &pb.CSAwakeUseItem{ItemId: proto.String("FishItem_026")}, now, 102); err == nil || !strings.Contains(err.Error(), "1.0.91") {
		t.Fatal("old client used food", err)
	}
	if _, err := f.s.p7AwakeExplore(f.ctx, f.id, &pb.CSAwakeCopySettle{}, now, 103); err != nil {
		t.Fatal("empty exploration could not settle", err)
	}
	p7AwakeFoodTestBinding(t, f, 2026103105, true)
	if err := db.GetIns().Transaction(func(tx *gorm.DB) error { return p7AwakeRequireFoodClient(tx, f.id) }); err != nil {
		t.Fatal("current authenticated build rejected", err)
	}
}

// The injured actor is a bounded fixture; a real battle terminal is verified by
// the separate typed16 suite, not simulated or claimed by this test.
func TestP7AwakeFoodSelectionUseCooldownAndCold(t *testing.T) {
	f := p7AwakeExploreFixture(t)
	p7AwakeFoodTestBinding(t, f, 2026103105, true)
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
	if err := updateProfile(f.ctx, f.id, func(_ *pb.Player, g *model.GameState) error {
		g.Cooking = append(g.Cooking, &pb.SimpleItem{Id: proto.String("FishItem_026"), Num: proto.Int32(4)})
		g.Items = append(g.Items, &pb.SimpleItem{Id: proto.String("item_068"), Num: proto.Int32(3)})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	item := func(id string, n int32) *pb.SimpleItem {
		return &pb.SimpleItem{Id: proto.String(id), Num: proto.Int32(n)}
	}
	for _, invalid := range [][]*pb.SimpleItem{
		{item("FishItem_026", 4)},
		{item("FishItem_026", 1), item("FishItem_026", 1)},
		{item("FishItem_026", 1), item("item_068", 1), item("FishItem_030", 1)},
		{item("FishItem_027", 1)},
		{item("FishItem_030", 1)},
	} {
		if _, err := f.s.p7AwakeExplore(f.ctx, f.id, &pb.CSEnterAwakeCopy{CopyId: proto.String("501"), Items: invalid}, now, 100); err == nil {
			t.Fatal("invalid or unowned carried item accepted", invalid)
		}
	}
	request := &pb.CSEnterAwakeCopy{CopyId: proto.String("501"), Items: []*pb.SimpleItem{item("FishItem_026", 3), item("item_068", 2)}}
	entry, err := f.s.p7AwakeExplore(f.ctx, f.id, request, now, 100)
	if err != nil {
		t.Fatal(err)
	}
	first := entry[0].(*pb.SCEnterAwakeCopy)
	if len(first.GetRefreshData().GetItems()) != 2 || first.GetRefreshData().GetItems()[0].GetNum() != 3 || first.GetRefreshData().GetItems()[1].GetNum() != 2 {
		t.Fatal("selection absent from original HUD response")
	}
	p, g := f.profile(t)
	if p4ResourceBalance(p, g, "FishItem_026") != 4 || p4ResourceBalance(p, g, "item_068") != 3 {
		t.Fatal("enter consumed unused supplies")
	}
	if _, err = f.s.p7AwakeExplore(f.ctx, f.id, &pb.CSAwakeUseItem{ItemId: proto.String("FishItem_026")}, now, 101); err == nil {
		t.Fatal("full health consumed food")
	}
	var maximum float64
	if err = db.GetIns().Transaction(func(tx *gorm.DB) error {
		ledger, run, err := p7AwakeActive(tx, f.id)
		if err != nil {
			return err
		}
		frozen, err := p7AwakeReadFrozen(run)
		if err != nil {
			return err
		}
		state, err := p7AwakeReadProgress(ledger, frozen)
		if err != nil {
			return err
		}
		actor := &pb.BattlePlayer{}
		if err = proto.Unmarshal(run.Actor, actor); err != nil {
			return err
		}
		for _, hero := range actor.Formation {
			maximum += hero.GetProp().GetHp()
			hero.Inst.CurHp = proto.Float64(hero.GetProp().GetHp() / 4)
		}
		dead := proto.Clone(actor.Formation[0]).(*pb.BattleHero)
		dead.Guid = proto.String("isolated-dead-actor")
		dead.Inst.CurHp = proto.Float64(0)
		actor.Formation = append(actor.Formation, dead)
		state.LastFight = "isolated-injured-fixture"
		state.CurrentActor, err = proto.Marshal(actor)
		if err != nil {
			return err
		}
		return p7AwakeSaveProgress(tx, ledger, state)
	}); err != nil {
		t.Fatal(err)
	}
	use := &pb.CSAwakeUseItem{ItemId: proto.String("FishItem_026")}
	var wait sync.WaitGroup
	replies := make(chan []proto.Message, 8)
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			messages, err := f.s.p7AwakeExplore(f.ctx, f.id, use, now, 101)
			if err != nil {
				errs <- err
				return
			}
			replies <- messages
		}()
	}
	wait.Wait()
	close(errs)
	close(replies)
	for err := range errs {
		t.Error(err)
	}
	var result *pb.SCAwakeUseItem
	count := 0
	for messages := range replies {
		count++
		got := messages[0].(*pb.SCAwakeUseItem)
		if result == nil {
			result = got
		} else if !proto.Equal(got, result) {
			t.Fatal("concurrent item retries returned different receipts")
		}
	}
	if count != 8 {
		t.Fatal("concurrent item retries incomplete", count)
	}
	if len(result.Items) != 1 || result.Items[0].GetId() != "FishItem_026" || result.Items[0].GetNum() != 2 {
		t.Fatal("response is not the remaining selected count", result)
	}
	var ledger db.P7AwakeExploration
	var run db.P7AwakeRun
	if err := db.GetIns().First(&ledger, "player_id=?", f.id).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().First(&run, "id=?", ledger.RunID).Error; err != nil {
		t.Fatal(err)
	}
	frozen, _ := p7AwakeReadFrozen(&run)
	state, err := p7AwakeReadProgress(&ledger, frozen)
	if err != nil {
		t.Fatal(err)
	}
	actor := &pb.BattlePlayer{}
	if err := proto.Unmarshal(state.CurrentActor, actor); err != nil {
		t.Fatal(err)
	}
	var current float64
	for _, hero := range actor.Formation {
		current += hero.GetInst().GetCurHp()
		if hero.GetGuid() == "isolated-dead-actor" && hero.GetInst().GetCurHp() != 0 {
			t.Fatal("food revived a dead party member")
		}
	}
	if math.Abs(current-maximum*0.30) > maximum*1e-9 {
		t.Fatal("5 percent food did not restore each member", current, maximum)
	}
	if replay, err := f.s.p7AwakeExplore(f.ctx, f.id, use, now, 101); err != nil || !proto.Equal(replay[0], result) {
		t.Fatal("same request did not replay once", err)
	}
	if _, err := f.s.p7AwakeExplore(f.ctx, f.id, &pb.CSAwakeUseItem{ItemId: proto.String("item_068")}, now.Add(30*time.Second), 102); err == nil {
		t.Fatal("shared cooldown bypassed")
	}
	if _, err := f.s.p7AwakeExplore(f.ctx, f.id, &pb.CSAwakeUseItem{ItemId: proto.String("item_068")}, now.Add(time.Minute), 103); err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	resume, err := f.s.p7AwakeExplore(f.ctx, f.id, &pb.CSEnterAwakeCopy{CopyId: proto.String("501")}, now.Add(time.Minute), 104)
	if err != nil {
		t.Fatal(err)
	}
	carried := resume[0].(*pb.SCEnterAwakeCopy).GetRefreshData().GetItems()
	if len(carried) != 2 || carried[0].GetNum() != 2 || carried[1].GetNum() != 1 {
		t.Fatal("cold resume lost remaining items", carried)
	}
	if replay, err := f.s.p7AwakeExplore(f.ctx, f.id, use, now.Add(time.Minute), 101); err != nil || !proto.Equal(replay[0], result) {
		t.Fatal("cold duplicate was not stable", err)
	}
	if _, err := f.s.p7AwakeExplore(f.ctx, f.id, &pb.CSAwakeCopySettle{}, now.Add(time.Minute), 105); err != nil {
		t.Fatal(err)
	}
	p, g = f.profile(t)
	if p4ResourceBalance(p, g, "FishItem_026") != 3 || p4ResourceBalance(p, g, "item_068") != 2 {
		t.Fatal("used or unused inventory changed incorrectly")
	}
}

// The roster stats below are an explicitly isolated test precondition that
// forces a surviving timeout. Both results still come from the real typed16
// engine and the actual Begin/End transaction, never from a fabricated receipt.
func TestP7AwakeFoodActualLossAndContinue(t *testing.T) {
	f := awake85BusinessFixture(t)
	p7AwakeFoodTestBinding(t, f, 2026103105, true)
	now := awake85Monday()
	if err := updateProfile(f.ctx, f.id, func(_ *pb.Player, g *model.GameState) error {
		g.Cooking = append(g.Cooking, &pb.SimpleItem{Id: proto.String("FishItem_026"), Num: proto.Int32(3)})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	entered, err := f.s.p7AwakeExplore(f.ctx, f.id, &pb.CSEnterAwakeCopy{CopyId: proto.String("102"), Items: []*pb.SimpleItem{{Id: proto.String("FishItem_026"), Num: proto.Int32(3)}}}, now, 201)
	if err != nil {
		t.Fatal(err)
	}
	entry := entered[0].(*pb.SCEnterAwakeCopy)
	if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		_, run, err := p7AwakeActive(tx, f.id)
		if err != nil {
			return err
		}
		frozen, err := p7AwakeReadFrozen(run)
		if err != nil {
			return err
		}
		actor := &pb.BattlePlayer{}
		if err := proto.Unmarshal(run.Actor, actor); err != nil {
			return err
		}
		for _, hero := range actor.Formation {
			hero.Prop.Att = proto.Float64(1)
			hero.Prop.Hp = proto.Float64(100000000)
			hero.Prop.Pdef = proto.Float64(0)
			hero.Prop.Mdef = proto.Float64(0)
			hero.Inst.CurHp = proto.Float64(100000000)
			hero.TalentList = nil
			hero.AwakenSkill = nil
			hero.AwakeStage = proto.Int32(0)
		}
		run.Actor, err = proto.Marshal(actor)
		if err != nil {
			return err
		}
		frozen.ActorHash = p6Digest(run.Actor)
		monID := frozen.Content.Monsters[0].GetGuid()
		enemy := &pb.BattleMonster{}
		if err = proto.Unmarshal(frozen.MonsterSources85[monID], enemy); err != nil {
			return err
		}
		for _, hero := range enemy.Formation {
			hero.Prop.Hp = proto.Float64(1e14)
			hero.Prop.Att = proto.Float64(100000)
			hero.Inst.CurHp = proto.Float64(1e14)
		}
		frozen.MonsterSources85[monID], err = proto.MarshalOptions{Deterministic: true}.Marshal(enemy)
		if err != nil {
			return err
		}
		frozen.MonsterSourceHashes85[monID] = p6Digest(frozen.MonsterSources85[monID])
		run.Freeze, err = json.Marshal(frozen)
		if err != nil {
			return err
		}
		run.FreezeHash = p6Digest(run.Freeze)
		return tx.Model(run).Updates(map[string]any{"actor": run.Actor, "freeze": run.Freeze, "freeze_hash": run.FreezeHash}).Error
	}); err != nil {
		t.Fatal(err)
	}
	first, end := p7AwakeBusinessStartEnd(t, f, entry.Monsters[0].GetGuid(), now.Add(time.Second))
	if end.GetIsWinClient() != 0 {
		t.Fatalf("isolated timeout did not produce a loss: rounds=%d attacker=%v defender=%v", end.GetOptRecord().GetFinalRoundNum(), end.GetOptRecord().GetAttackerTotalLeftHp(), end.GetOptRecord().GetDefenderTotalLeftHp())
	}
	if _, err := f.s.p7AwakeFightEnd(f.ctx, f.id, end, now.Add(2*time.Second)); err != nil {
		t.Fatal("actual loss settlement", err)
	}
	var fight db.P7AwakeFight
	if err := db.GetIns().First(&fight, "id=?", first.GetFightId()).Error; err != nil {
		t.Fatal(err)
	}
	var terminal model.CombatFinalHPOutcome
	if json.Unmarshal(fight.Terminal, &terminal) != nil || terminal.Outcome == nil || terminal.Outcome.Winner == 0 {
		t.Fatal("actual previous terminal missing")
	}
	used, err := f.s.p7AwakeExplore(f.ctx, f.id, &pb.CSAwakeUseItem{ItemId: proto.String("FishItem_026")}, now.Add(3*time.Second), 202)
	if err != nil || used[0].(*pb.SCAwakeUseItem).GetItems()[0].GetNum() != 2 {
		t.Fatal("restore after actual loss", err)
	}
	secondMessages, err := f.s.p7AwakeFightBegin(f.ctx, f.id, &pb.CSAwakeCopyFightBegin{MonGuid: entry.Monsters[0].Guid}, now.Add(4*time.Second))
	if err != nil {
		t.Fatal("continued actual Begin", err)
	}
	second := secondMessages[0].(*pb.SCAwakeCopyFightBegin)
	for _, hero := range second.BattleMonser.Formation {
		found := false
		for _, final := range terminal.FinalActors {
			if final.Camp == 1 && final.GUID == hero.GetGuid() {
				found = hero.GetInst().GetCurHp() == final.CurrentHP
			}
		}
		if !found {
			t.Fatal("continued enemy did not use accepted per-actor HP", hero.GetGuid())
		}
	}
	f.restart(t)
	resume, err := f.s.p7AwakeFightBegin(f.ctx, f.id, &pb.CSAwakeCopyFightBegin{MonGuid: entry.Monsters[0].Guid}, now.Add(5*time.Second))
	if err != nil || !proto.Equal(resume[0], second) {
		t.Fatal("continued battle did not cold resume", err)
	}
	var continued db.P7AwakeFight
	if err := db.GetIns().First(&continued, "id=?", second.GetFightId()).Error; err != nil {
		t.Fatal(err)
	}
	input, _, err := p7AwakeFightInput(db.GetIns().WithContext(f.ctx), &continued)
	if err != nil {
		t.Fatal("continued battle proof", err)
	}
	actual, err := f.s.computeFrozenBattleFinalHP(f.ctx, input, nil, true)
	if err != nil {
		t.Fatal("continued independent engine", err)
	}
	win := int32(0)
	if actual.Outcome.Winner == 0 {
		win = 1
	}
	if _, err := f.s.p7AwakeFightEnd(f.ctx, f.id, &pb.CSAwakeCopyFightEnd{FightId: second.FightId, IsWinClient: proto.Int32(win), OptRecord: actual.OriginalOperates}, now.Add(6*time.Second)); err != nil {
		t.Fatal("continued actual End", err)
	}
}
