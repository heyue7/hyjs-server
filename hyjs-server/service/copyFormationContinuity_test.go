package service

import (
	"context"
	"encoding/json"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// This uses the original HTTP packet dispatcher and an isolated SQLite profile.
// The direct profile edit below models an already inconsistent historical run.
func TestCopyFormationContinuityOriginalRequestsAndCold(t *testing.T) {
	t.Setenv("HYJS_P1_MYSQL_DATABASE", "")
	f := p1Setup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, initialize := range []func(context.Context) error{
		f.s.InitializeP5, f.s.InitializeOpenServerActivity, f.s.InitializeDailyTasks,
		f.s.InitializeCopyDrops, f.s.InitializeDailyCopy, f.s.InitCooking,
	} {
		if err := initialize(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	f.s.P5Ready = true
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(100)
		p.Stamina = proto.Int32(1000)
		p.WarriorBagSize = proto.Int32(1000)
		_, err := f.s.GrantFixedReward(p, g, "hero_020,1;hero_011,1;hero_003,1;equip_001,1")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	_, g := f.profile(t)
	heroes := map[string]*pb.Warrior{}
	for _, hero := range g.Warriors {
		heroes[hero.GetId()] = hero
	}
	main := heroes["hero_043"]
	partner, replacement, other := heroes["hero_020"], heroes["hero_011"], heroes["hero_003"]
	if main == nil || partner == nil || replacement == nil || other == nil || len(g.Equipment) == 0 {
		t.Fatal("missing isolated formation fixture")
	}
	gear := g.Equipment[0]
	ctx := func(request proto.Message) context.Context {
		return context.WithValue(context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s), constUtil.GameRequestContextKey, request)
	}
	partnerOn := &pb.CSFormationOn{Id: partner.Id, Guid: partner.Guid, FormatPos: proto.Int32(2)}
	originalOn, err := f.s.p5FormationOnNative(ctx(partnerOn), f.id, partnerOn, 710001)
	if err != nil {
		t.Fatal(err)
	}
	enter := &pb.CSCopyEnter{ChapterId: proto.String("1"), MissionId: proto.String("1")}
	if out := copyOverflowWire(t, f, enter); len(out) == 0 {
		t.Fatal("empty copy entry")
	} else if _, ok := out[0].(*pb.SCCopyEnter); !ok {
		t.Fatal("original copy entry", out)
	}
	tasks := f.s.tasks("1")
	if len(tasks) < 2 {
		t.Fatal("fixture needs more than one encounter")
	}
	startFight := func(task string) *pb.SCCopyFightStart {
		t.Helper()
		out := copyOverflowWire(t, f, &pb.CSCopyFightStart{ChapterId: proto.String("1"), MissionId: proto.String("1"), TaskId: proto.String(task)})
		if len(out) == 0 {
			t.Fatal("empty fight start")
		}
		start, ok := out[0].(*pb.SCCopyFightStart)
		if !ok {
			t.Fatal("original fight start", out)
		}
		return start
	}
	endFight := func(start *pb.SCCopyFightStart, battle *pb.BattlePlayer) *pb.SCCopyFightEnd {
		t.Helper()
		out := copyOverflowWire(t, f, &pb.CSCopyFightEnd{FightId: start.FightId, Result: proto.Int32(1), BattlePlayer: battle})
		if len(out) == 0 {
			t.Fatal("empty fight end")
		}
		end, ok := out[0].(*pb.SCCopyFightEnd)
		if !ok {
			t.Fatal("original fight end", out)
		}
		return end
	}
	start := startFight(tasks[0]["TaskId"])
	battle := proto.Clone(start.BattlePlayer).(*pb.BattlePlayer)
	for _, hero := range battle.Formation {
		switch hero.GetGuid() {
		case main.GetGuid():
			hero.Inst.CurHp = proto.Float64(hero.GetProp().GetHp() / 2)
		case partner.GetGuid():
			hero.Inst.CurHp = proto.Float64(0)
		}
	}
	battle.Inst.Anger = proto.Float64(117)
	settled := endFight(start, battle)
	_, g = f.profile(t)
	if g.Run == nil || g.Run.Party == nil || !g.Run.Fight.Settled || len(g.Run.Passed) != 1 {
		t.Fatal("first fight did not save its accepted run")
	}
	partyBefore := proto.Clone(g.Run.Party).(*pb.BattlePlayer)
	gainBefore := proto.Clone(g.Run.Gain).(*pb.ResultObject)
	f.restart(t)
	if replay, err := f.s.p5FormationOnNative(ctx(partnerOn), f.id, partnerOn, 710001); err != nil || !proto.Equal(originalOn[0], replay[0]) {
		t.Fatal("cold positive-stamp hero receipt did not replay", err)
	}
	if _, err := f.s.p5FormationOnNative(ctx(partnerOn), f.id, partnerOn, 710002); err != nil {
		t.Fatal("same hero with fresh stamp", err)
	}
	assertRejectedWithoutMutation := func(request proto.Message) {
		t.Helper()
		beforeP, beforeG := f.profile(t)
		before, err := json.Marshal(beforeG)
		if err != nil {
			t.Fatal(err)
		}
		out := copyOverflowWire(t, f, request)
		if len(out) == 0 {
			t.Fatal("empty rejection")
		}
		failure, ok := out[0].(*pb.ErrorMessage)
		if !ok || failure.GetMsg() != constUtil.CopyContinuityLineupChanged {
			t.Fatal("unexpected formation rejection", out)
		}
		afterP, afterG := f.profile(t)
		after, err := json.Marshal(afterG)
		if err != nil || !proto.Equal(beforeP, afterP) || string(before) != string(after) {
			t.Fatal("rejected change altered profile, rewards or run", err)
		}
	}
	partnerGearOn := &pb.CSFormationOn{Id: gear.EquipId, Guid: gear.Guid, FormatPos: proto.Int32(2)}
	if out := copyOverflowWire(t, f, partnerGearOn); len(out) == 0 {
		t.Fatal("empty partner equip response")
	} else if _, ok := out[0].(*pb.SCFormationOn); !ok {
		t.Fatal("partner equipment during run", out)
	}
	assertRejectedWithoutMutation(&pb.CSFormationOn{Id: replacement.Id, Guid: replacement.Guid, FormatPos: proto.Int32(2)})
	assertRejectedWithoutMutation(&pb.CSFormationOff{Id: partner.Id, Guid: partner.Guid, FormatPos: proto.Int32(2)})
	partnerGearOff := &pb.CSFormationOff{Id: gear.EquipId, Guid: gear.Guid, FormatPos: proto.Int32(2)}
	if out := copyOverflowWire(t, f, partnerGearOff); len(out) == 0 {
		t.Fatal("empty partner unequip response")
	} else if _, ok := out[0].(*pb.SCFormationOff); !ok {
		t.Fatal("partner unequip after rejected removal", out)
	}
	beforeLegacyP, beforeLegacyG := f.profile(t)
	beforeLegacy, err := json.Marshal(beforeLegacyG)
	if err != nil {
		t.Fatal(err)
	}
	legacyOn := &pb.CSFormationOn{Id: replacement.Id, Guid: replacement.Guid, FormatPos: proto.Int32(2)}
	if _, err := f.s.formationOn(ctx(legacyOn), f.id, legacyOn); err == nil || err.Error() != constUtil.CopyContinuityLineupChanged {
		t.Fatal("legacy formation entry accepted a different member", err)
	}
	afterLegacyP, afterLegacyG := f.profile(t)
	afterLegacy, err := json.Marshal(afterLegacyG)
	if err != nil || !proto.Equal(beforeLegacyP, afterLegacyP) || string(beforeLegacy) != string(afterLegacy) {
		t.Fatal("legacy formation rejection changed profile", err)
	}

	// A deployment may encounter a preexisting mismatch. Restore its original
	// identities one step at a time without rewriting its accepted battle state.
	if err := updateProfile(f.ctx, f.id, func(_ *pb.Player, state *model.GameState) error {
		for _, slot := range state.Formation {
			if slot.GetFormatPos() == 2 {
				slot.Things = []*pb.FormationThing{{Pos: proto.Int32(0), Id: replacement.Id, Guid: replacement.Guid}}
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	assertRejectedWithoutMutation(&pb.CSFormationOn{Id: other.Id, Guid: other.Guid, FormatPos: proto.Int32(2)})
	assertRejectedWithoutMutation(&pb.CSFormationOff{Id: main.Id, Guid: main.Guid, FormatPos: proto.Int32(1)})
	direct, err := f.s.p5FormationOnNative(ctx(partnerOn), f.id, partnerOn, 710004)
	if err != nil || len(direct) == 0 {
		t.Fatal("directly replacing an extra with a missing historical member", err)
	}
	_, g = f.profile(t)
	if !proto.Equal(g.Run.Party, partyBefore) || !proto.Equal(g.Run.Gain, gainBefore) {
		t.Fatal("direct historical repair changed the accepted run")
	}
	if err := updateProfile(f.ctx, f.id, func(_ *pb.Player, state *model.GameState) error {
		for _, slot := range state.Formation {
			if slot.GetFormatPos() == 2 {
				slot.Things = []*pb.FormationThing{{Pos: proto.Int32(0), Id: replacement.Id, Guid: replacement.Guid}}
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	removed := copyOverflowWire(t, f, &pb.CSFormationOff{Id: replacement.Id, Guid: replacement.Guid, FormatPos: proto.Int32(2)})
	if len(removed) == 0 {
		t.Fatal("empty historical removal response")
	} else if _, ok := removed[0].(*pb.SCFormationOff); !ok {
		t.Fatal("removing an extra historical member", removed)
	}
	restored := copyOverflowWire(t, f, partnerOn)
	if len(restored) == 0 {
		t.Fatal("empty historical restoration response")
	} else if _, ok := restored[0].(*pb.SCFormationOn); !ok {
		t.Fatal("restoring a missing historical member", restored)
	}
	_, g = f.profile(t)
	if !proto.Equal(g.Run.Party, partyBefore) || !proto.Equal(g.Run.Gain, gainBefore) || len(g.Run.Passed) != 1 {
		t.Fatal("historical formation recovery changed the accepted run")
	}
	// Existing gear and position operations retain their original contracts.
	equipOn := &pb.CSFormationOn{Id: gear.EquipId, Guid: gear.Guid, FormatPos: proto.Int32(1)}
	if out := copyOverflowWire(t, f, equipOn); len(out) == 0 {
		t.Fatal("empty equip response")
	} else if _, ok := out[0].(*pb.SCFormationOn); !ok {
		t.Fatal("equip while run active", out)
	}
	equipOff := &pb.CSFormationOff{Id: gear.EquipId, Guid: gear.Guid, FormatPos: proto.Int32(1)}
	if out := copyOverflowWire(t, f, equipOff); len(out) == 0 {
		t.Fatal("empty unequip response")
	} else if _, ok := out[0].(*pb.SCFormationOff); !ok {
		t.Fatal("unequip while run active", out)
	}
	exchange := &pb.CSFormationExChange{FromPos: proto.Int32(1), ToPos: proto.Int32(2)}
	exchanged, err := f.s.formationExchange(ctx(exchange), f.id, exchange, 710003)
	if err != nil {
		t.Fatal("exchange while run active", err)
	}
	f.restart(t)
	if replay, err := f.s.formationExchange(ctx(exchange), f.id, exchange, 710003); err != nil || !proto.Equal(exchanged[0], replay[0]) {
		t.Fatal("cold exchange receipt", err)
	}
	beforeReplayP, beforeReplayG := f.profile(t)
	firstEnd := &pb.CSCopyFightEnd{FightId: start.FightId, Result: proto.Int32(1), BattlePlayer: battle}
	if out := copyOverflowWire(t, f, firstEnd); len(out) == 0 || !proto.Equal(out[0], settled) {
		t.Fatal("cold first-fight receipt", out)
	}
	afterReplayP, afterReplayG := f.profile(t)
	if !proto.Equal(beforeReplayP, afterReplayP) || !proto.Equal(beforeReplayG.Run.Gain, afterReplayG.Run.Gain) || !proto.Equal(afterReplayG.Run.Gain, gainBefore) {
		t.Fatal("cold receipt duplicated or changed accepted rewards")
	}
	beforeP, _ := f.profile(t)
	if out := copyOverflowWire(t, f, enter); len(out) == 0 {
		t.Fatal("empty resume")
	} else if _, ok := out[0].(*pb.SCCopyEnter); !ok {
		t.Fatal("resume same paid run", out)
	}
	afterP, _ := f.profile(t)
	if afterP.GetStamina() != beforeP.GetStamina() {
		t.Fatal("resume charged stamina twice")
	}
	for i, task := range tasks[1:] {
		start = startFight(task["TaskId"])
		if i == 0 {
			if start.BattlePlayer.GetInst().GetAnger() != partyBefore.GetInst().GetAnger() {
				t.Fatal("cold resume lost anger")
			}
			for _, old := range partyBefore.GetFormation() {
				found := false
				for _, hero := range start.BattlePlayer.GetFormation() {
					if hero.GetGuid() == old.GetGuid() {
						found = true
						if hero.GetInst().GetCurHp() != old.GetInst().GetCurHp() {
							t.Fatal("cold resume lost accepted HP", hero.GetGuid())
						}
					}
				}
				if !found {
					t.Fatal("cold resume lost party identity", old.GetGuid())
				}
			}
		}
		endFight(start, proto.Clone(start.BattlePlayer).(*pb.BattlePlayer))
	}
	_, g = f.profile(t)
	if !g.Run.Complete {
		t.Fatal("fixture did not finish the run")
	}
	// The first fight's committed reward is not granted again by this guard.
	if settled.GetGain() == nil {
		t.Fatal("expected original first-fight reward response")
	}
	afterComplete := copyOverflowWire(t, f, &pb.CSFormationOn{Id: replacement.Id, Guid: replacement.Guid, FormatPos: proto.Int32(1)})
	if len(afterComplete) == 0 {
		t.Fatal("empty post-run formation response")
	} else if _, ok := afterComplete[0].(*pb.SCFormationOn); !ok {
		t.Fatal("finished run still blocked formation changes", afterComplete)
	}
}

func TestCopyFormationContinuityLegacyPendingIdentity(t *testing.T) {
	player := &pb.Player{PlayerId: proto.Int64(7)}
	main := &pb.Warrior{Guid: proto.String("owned"), Id: proto.String("hero_043")}
	extra := &pb.Warrior{Guid: proto.String("other"), Id: proto.String("hero_020")}
	state := &model.GameState{
		Warriors: []*pb.Warrior{main, extra},
		Formation: []*pb.FormationItem{{FormatPos: proto.Int32(1), Things: []*pb.FormationThing{
			{Pos: proto.Int32(0), Guid: main.Guid, Id: main.Id},
		}}},
		Run: &model.CopyRun{Fight: &model.CopyFight{Start: &pb.SCCopyFightStart{BattlePlayer: &pb.BattlePlayer{
			PlayerId: player.PlayerId,
			Formation: []*pb.BattleHero{
				{Guid: main.Guid, Id: main.Id},
				{Guid: proto.String("demo-scripted"), Id: proto.String("hero_011")},
			},
		}}}},
	}
	guard := copyContinuityGuardFormation(player, state)
	if guard == nil || len(guard.target) != 1 {
		t.Fatal("legacy pending fight failed to isolate its owned hero")
	}
	state.Formation[0].Things[0] = &pb.FormationThing{Pos: proto.Int32(0), Guid: extra.Guid, Id: extra.Id}
	if err := guard.check(state); err == nil {
		t.Fatal("legacy pending fight accepted a different hero")
	}
	state.Run.Fight.Start.BattlePlayer.Formation = append(state.Run.Fight.Start.BattlePlayer.Formation, &pb.BattleHero{Guid: main.Guid, Id: main.Id})
	if copyContinuityGuardFormation(player, state) != nil {
		t.Fatal("ambiguous legacy snapshot must retain its prior behavior")
	}
	state.Run.Fight = nil
	if copyContinuityGuardFormation(player, state) != nil {
		t.Fatal("old run without a party or pending fight changed behavior")
	}
}
