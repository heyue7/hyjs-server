package service

import (
	"encoding/json"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestGGOMonsterNativeStartAcceptsRecentPatrolContact(t *testing.T) {
	f, ctx, _, _ := ggoBossIntegrationFixture(t)
	cfg := f.s.GGOFieldConfig
	f.s.GGOFieldRevision = "monster-contact-fixture"
	var selected *model.AdminFieldGGOPoint
	for i := range cfg.Runtime.Points {
		if cfg.Runtime.Points[i].Kind == "monster" && cfg.Runtime.Points[i].Patrol != nil {
			selected = &cfg.Runtime.Points[i]
			selected.Radius = .2
			break
		}
	}
	if selected == nil {
		t.Fatal("missing patrol fixture")
	}
	var uuid string
	err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		now := time.Now().UnixMilli()
		row, state, _, err := ggoLoadLine(tx, f.s.OriginalServerID(), 1, cfg, f.s.GGOFieldRevision, now)
		if err != nil {
			return err
		}
		for i := range state.Monsters {
			if state.Monsters[i].PointID == selected.ID {
				state.Monsters[i].PatrolEpochMS = now - 14200
				uuid = state.Monsters[i].UUID
				break
			}
		}
		if uuid == "" {
			t.Fatal("missing shared patrol node")
		}
		return ggoLineSave(tx, row, state, now)
	})
	if err != nil {
		t.Fatal(err)
	}
	contact, err := ggoWirePoint(selected.Position)
	if err != nil {
		t.Fatal(err)
	}
	setPosition := func(point *pb.PointNew) {
		t.Helper()
		if err := updateProfile(ctx, f.id, func(_ *pb.Player, game *model.GameState) error {
			game.Position.Current = point
			game.GGOField.ProtectionUntil = 0
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	far := proto.Clone(contact).(*pb.PointNew)
	far.X = proto.Float32(contact.GetX() + 5)
	setPosition(far)
	request := &pb.CSSidePveFightStart{MonsterId: proto.String(uuid)}
	if _, err = f.s.fieldFightStart(ctx, f.id, request); err == nil {
		t.Fatal("distant actor started combat")
	}
	setPosition(contact)
	started, err := f.s.fieldFightStart(ctx, f.id, request)
	if err != nil || len(started) != 1 || started[0].(*pb.SCSidePveFightStart).GetCode() != 1 {
		t.Fatal("recent actual contact did not start native combat", started, err)
	}
	retry, err := f.s.fieldFightStart(ctx, f.id, request)
	if err != nil || len(retry) != 1 || !proto.Equal(started[0], retry[0]) {
		t.Fatal("start retry changed fight", err)
	}
}

// Uses the real native type-6 start, original engine result and end transaction.
// Coordinates and actor progress are bounded fixtures, not device evidence.
func TestGGOMonsterNativeStartEndFrozenPolicy(t *testing.T) {
	t.Setenv("HYJS_P1_MYSQL_DATABASE", "")
	f, ctx, _, _ := ggoBossIntegrationFixture(t)
	cfg := f.s.GGOFieldConfig
	cfg.Rules.DeathLossPercent = ggoInt64(25)
	f.s.GGOFieldRevision = "monster-native-fixture"
	var node ggoMonsterNode
	var position *model.AdminFieldGGOPosition
	err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		_, state, _, e := ggoLoadLine(tx, f.s.OriginalServerID(), 1, cfg, f.s.GGOFieldRevision, time.Now().UnixMilli())
		if e != nil {
			return e
		}
		for _, candidate := range state.Monsters {
			for _, point := range cfg.Runtime.Points {
				if point.ID == candidate.PointID && point.Patrol == nil {
					node, position = candidate, point.Position
					return nil
				}
			}
		}
		t.Fatal("missing stationary monster fixture")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	point, err := ggoWirePoint(position)
	if err != nil {
		t.Fatal(err)
	}
	if err = updateProfile(ctx, f.id, func(_ *pb.Player, g *model.GameState) error {
		g.Position.Current = point
		g.GGOField.ProtectionUntil = 0
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	startMessages, err := f.s.fieldFightStart(ctx, f.id, &pb.CSSidePveFightStart{MonsterId: proto.String(node.UUID)})
	if err != nil {
		t.Fatal("native start", err)
	}
	start := startMessages[0].(*pb.SCSidePveFightStart)
	if start.GetCode() != 1 || len(start.BattleMonsterList) != 1 || len(start.BattleMonsterList[0].Formation) != 4 {
		t.Fatal("invalid native start")
	}
	_, game := f.profile(t)
	if game.GGOField.MonsterFight == nil || game.GGOField.MonsterFight.DeathLossPercent == nil || *game.GGOField.MonsterFight.DeathLossPercent != 25 {
		t.Fatal("death policy not frozen")
	}
	input, err := fieldBossFrozenInput(game.Field.Fight)
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(start.BattleMonsterList[0], input.Monster) {
		t.Fatal("native start and frozen monster formation differ")
	}
	// Published tuning affects future fights only, including after profile reload.
	for i := range cfg.Runtime.MonsterTiers {
		if cfg.Runtime.MonsterTiers[i].Level == node.Level && cfg.Runtime.MonsterTiers[i].HeroID == node.HeroID {
			cfg.Runtime.MonsterTiers[i].HP++
		}
	}
	_, reloaded := f.profile(t)
	restored, err := fieldBossFrozenInput(reloaded.Field.Fight)
	if err != nil || !proto.Equal(restored.Monster, input.Monster) {
		t.Fatal("frozen monster formation changed after configuration edit", err)
	}
	retried, err := f.s.fieldFightStart(ctx, f.id, &pb.CSSidePveFightStart{MonsterId: proto.String(node.UUID)})
	if err != nil || len(retried) != 1 || !proto.Equal(retried[0], start) {
		t.Fatal("active battle start changed after configuration edit", err)
	}
	cfg.Rules.DeathLossPercent = ggoInt64(100)
	beforeRaw, _ := json.Marshal(input)
	terminal, _, err := f.s.ComputeP6GenericMechanismBattle(ctx, input, nil)
	if err != nil || terminal == nil || terminal.OriginalOperates == nil {
		t.Fatal("original battle replay", err)
	}
	rechecked, err := f.s.ComputeOriginalP6ProtocolFinalHP(ctx, input, terminal.OriginalOperates)
	if err != nil {
		t.Fatal("original record replay", err)
	}
	afterRaw, _ := json.Marshal(input)
	if p6Digest(beforeRaw) != p6Digest(afterRaw) {
		t.Fatal("frozen input mutated during replay")
	}
	if !proto.Equal(terminal.OriginalOperates, rechecked.OriginalOperates) {
		want, got := terminal.OriginalOperates, rechecked.OriginalOperates
		t.Logf("round/hp before=%d/%g/%g after=%d/%g/%g; skill counts=%d/%d", want.GetFinalRoundNum(), want.GetAttackerTotalLeftHp(), want.GetDefenderTotalLeftHp(), got.GetFinalRoundNum(), got.GetAttackerTotalLeftHp(), got.GetDefenderTotalLeftHp(), len(want.SkillOpts), len(got.SkillOpts))
		for i := 0; i < len(want.SkillOpts) && i < len(got.SkillOpts); i++ {
			if !proto.Equal(want.SkillOpts[i], got.SkillOpts[i]) {
				t.Logf("skill %d before=%v after=%v", i, want.SkillOpts[i], got.SkillOpts[i])
			}
		}
		if !p6OriginalProtocolRecordEqual(want, got) {
			t.Fatal("replayed record violates original comparator")
		}
	}
	reported := proto.Clone(input.Attacker).(*pb.BattlePlayer)
	for _, hero := range reported.Formation {
		found := false
		for _, actor := range terminal.FinalActors {
			if actor.Camp == 0 && actor.GUID == hero.GetGuid() {
				hero.Inst.CurHp = proto.Float64(actor.CurrentHP)
				if actor.RuntimePose != 0 {
					hero.Pose = proto.Int32(actor.RuntimePose)
				}
				found = true
				break
			}
		}
		if !found {
			t.Fatal("terminal actor missing")
		}
	}
	reported.Inst.Anger = proto.Float64(terminal.Outcome.Attacker.GetInst().GetAnger())
	result := int32(0)
	if terminal.Outcome.Winner == 0 {
		result = 1
	}
	end := &pb.CSSidePveFightEnd{FightId: start.FightId, BattlePlayer: reported, Result: proto.Int32(result), OptRecord: terminal.OriginalOperates}
	invalid := proto.Clone(end).(*pb.CSSidePveFightEnd)
	invalid.Result = proto.Int32(2)
	if _, err = f.s.fieldFightEnd(ctx, f.id, invalid); err == nil {
		t.Fatal("invalid result enum accepted")
	}
	messages, err := f.s.fieldFightEnd(ctx, f.id, end)
	if err != nil {
		t.Fatal("native end", err)
	}
	native := messages[0].(*pb.SCSidePveFightEnd)
	if native.GetCode() != 1 || native.GetFightResult() != result {
		t.Fatal("unexpected native end")
	}
	replayed, err := f.s.fieldFightEnd(ctx, f.id, end)
	if err != nil || len(replayed) != 1 || !proto.Equal(replayed[0], native) {
		t.Fatal("native end retry", err)
	}
	changed := proto.Clone(end).(*pb.CSSidePveFightEnd)
	changed.Result = proto.Int32(1 - result)
	if _, err = f.s.fieldFightEnd(ctx, f.id, changed); err == nil {
		t.Fatal("different retry accepted")
	}
	changed = proto.Clone(end).(*pb.CSSidePveFightEnd)
	clockChanged := false
	for _, op := range changed.OptRecord.SkillOpts {
		if op.GetCamp() == 0 {
			op.RoundTime = proto.Float64(op.GetRoundTime() + 0.1)
			clockChanged = true
			break
		}
	}
	if !clockChanged || !p6OriginalProtocolRecordEqual(end.OptRecord, changed.OptRecord) {
		t.Fatal("fixture cannot model player replay clock difference")
	}
	if _, err = f.s.fieldFightEnd(ctx, f.id, changed); err == nil {
		t.Fatal("same fight accepted changed source record")
	}
	_, after := f.profile(t)
	if after.GGOField.MonsterFight != nil || after.Field.Fight == nil || !after.Field.Fight.Settled {
		t.Fatal("in-flight state not cleared")
	}
	if result == 0 && terminal.Outcome.Attacker.GetFormation()[0].GetInst().GetCurHp() == 0 {
		var receipt model.GGOFieldMonsterReceipt
		found, e := ggoFindBusinessReceipt(db.GetIns(), f.id, constUtil.GGOFieldOpMonsterResult, start.GetFightId(), &receipt)
		if e != nil || !found || receipt.Lost != ([6]int64{2}) {
			t.Fatal("frozen 25 percent not applied", receipt.Lost, e)
		}
	}
}
