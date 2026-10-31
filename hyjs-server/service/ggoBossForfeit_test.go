package service

import (
	"encoding/hex"
	"encoding/json"
	"math"
	"reflect"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestGGOBossForfeitRecordBoundary(t *testing.T) {
	valid := &pb.CSSidePveBossFightEnd{OptRecord: &pb.BattleOperate{FinalRoundNum: proto.Int32(4)}}
	cases := []struct {
		name string
		edit func(*pb.CSSidePveBossFightEnd)
		want bool
	}{
		{"omitted zero values", func(*pb.CSSidePveBossFightEnd) {}, true},
		{"explicit zero values", func(r *pb.CSSidePveBossFightEnd) {
			r.Result = proto.Int32(0)
			r.OptRecord.AttackerTotalLeftHp = proto.Float64(0)
		}, true},
		{"win", func(r *pb.CSSidePveBossFightEnd) { r.Result = proto.Int32(1) }, false},
		{"missing record", func(r *pb.CSSidePveBossFightEnd) { r.OptRecord = nil }, false},
		{"empty record", func(r *pb.CSSidePveBossFightEnd) { r.OptRecord = &pb.BattleOperate{} }, false},
		{"round too large", func(r *pb.CSSidePveBossFightEnd) { r.OptRecord.FinalRoundNum = proto.Int32(22) }, false},
		{"surviving timeout", func(r *pb.CSSidePveBossFightEnd) {
			r.OptRecord.FinalRoundNum = proto.Int32(21)
			r.OptRecord.AttackerTotalLeftHp = proto.Float64(1)
		}, false},
		{"negative hp", func(r *pb.CSSidePveBossFightEnd) { r.OptRecord.AttackerTotalLeftHp = proto.Float64(-1) }, false},
		{"nan hp", func(r *pb.CSSidePveBossFightEnd) { r.OptRecord.AttackerTotalLeftHp = proto.Float64(math.NaN()) }, false},
		{"infinite hp", func(r *pb.CSSidePveBossFightEnd) { r.OptRecord.AttackerTotalLeftHp = proto.Float64(math.Inf(1)) }, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request := proto.Clone(valid).(*pb.CSSidePveBossFightEnd)
			tc.edit(request)
			if got := ggoBossRequestedForfeit(request, 20); got != tc.want {
				t.Fatalf("forfeit=%v want=%v", got, tc.want)
			}
		})
	}
}

// Original End with a client-reported zero-HP loss is a self-funded forfeit
// even when the independent server terminal wins with surviving heroes.
func TestGGOBossForfeitKeepsServerDamageAndColdReceipt(t *testing.T) {
	t.Setenv("HYJS_P1_MYSQL_DATABASE", "")
	t.Setenv("HYJS_GAME_ACCOUNTS_DATABASE", "")
	f, ctx, session, event := ggoBossIntegrationFixture(t)
	messages, err := f.s.fieldBossFightStart(ctx, f.id, &pb.CSSidePveBossFightStart{Uuid: proto.String(event.ID)})
	if err != nil {
		t.Fatal(err)
	}
	start := messages[0].(*pb.SCSidePveBossFightStart)
	if err = updateProfile(ctx, f.id, func(_ *pb.Player, game *model.GameState) error {
		game.GGOField.Chips = [6]int64{10}
		game.GGOField.PositionRevision = 7
		fight := game.Field.Fight
		input, err := fieldBossFrozenInput(fight)
		if err != nil {
			return err
		}
		for _, hero := range input.Attacker.Formation {
			hero.Prop.Att = proto.Float64(1e9)
		}
		for _, enemy := range input.Monster.Formation {
			enemy.Inst.CurHp = proto.Float64(1)
			enemy.Prop.Att = proto.Float64(1)
		}
		input.MaxRound = 20
		fight.BossStart.Maxround, fight.Start.Maxround = proto.Int32(20), proto.Int32(20)
		fight.BossStart.BattlePlayer, fight.Start.BattlePlayer = input.Attacker, input.Attacker
		fight.BossStart.BattleMonsterList, fight.Start.BattleMonsterList = []*pb.BattleMonster{input.Monster}, []*pb.BattleMonster{input.Monster}
		fight.GGOBoss.ProtectedChips = 0
		fight.GGOBoss.DeathLossPercent = proto.Int64(50)
		raw, err := json.Marshal(input)
		if err != nil {
			return err
		}
		fight.BossInput, fight.BossInputStored, fight.BossInputHash = input, nil, ""
		fight.GGOBoss.InputHash = p6Digest(raw)
		return nil
	}); err != nil {
		t.Fatal("prepare isolated automatic win", err)
	}
	_, before := f.profile(t)
	input, err := fieldBossFrozenInput(before.Field.Fight)
	if err != nil {
		t.Fatal(err)
	}
	terminal, _, err := f.s.ComputeP6GenericMechanismBattle(ctx, input, nil)
	if err != nil {
		t.Fatal(err)
	}
	var remaining float64
	for _, actor := range terminal.FinalActors {
		if actor.Camp == 0 {
			remaining += actor.CurrentHP
		}
	}
	if terminal.Outcome.Winner != 0 || remaining <= 0 {
		t.Fatal("fixture must win with survivors", terminal.Outcome.Winner, remaining)
	}
	wantDamage, err := ggoBossLeaderDamage(before.Field.Fight, input, terminal)
	if err != nil || wantDamage <= 0 {
		t.Fatal("expected authoritative damage", wantDamage, err)
	}
	clientRecord := proto.Clone(terminal.OriginalOperates).(*pb.BattleOperate)
	clientRecord.AttackerTotalLeftHp = nil // protobuf-net omits default zero.
	request := &pb.CSSidePveBossFightEnd{FightId: start.FightId, Uuid: proto.String(event.ID), OptRecord: clientRecord}
	ended, err := f.s.fieldBossFightEnd(ctx, f.id, request)
	if err != nil || len(ended) == 0 || ended[0].(*pb.SCSidePveBossFightEnd).GetFightResult() != 0 {
		t.Fatal("native forfeit result", err)
	}
	_, after := f.profile(t)
	if after.GGOField.Chips != ([6]int64{5}) || after.GGOField.PositionRevision != 8 || !after.PositionLoading {
		t.Fatal("forfeit did not apply loss and safe return", after.GGOField.Chips, after.GGOField.PositionRevision)
	}
	read := func() *model.GGOFieldBossBattleReceipt {
		t.Helper()
		receipt, e := f.s.GGOFieldBossReceipt(ctx, model.GGOFieldBossReceiptRequest{GGOFieldAuth: model.GGOFieldAuth{PlayerID: f.id, SessionHex: hex.EncodeToString(session)}, FightID: start.GetFightId()}, session)
		if e != nil {
			t.Fatal(e)
		}
		return receipt
	}
	receipt := read()
	p := after.Position.GetCurrent()
	if !receipt.Defeated || !receipt.Forfeit || receipt.Lost != ([6]int64{5}) || receipt.SafeReturn == nil || receipt.PositionRevision != 8 || *receipt.SafeReturn.X != float64(p.GetX()) || *receipt.SafeReturn.Y != float64(p.GetY()) || *receipt.SafeReturn.Z != float64(p.GetZ()) || receipt.Damage != wantDamage || receipt.RewardStatus != "pending_boss_defeat" || len(receipt.RewardResources) != 0 || receipt.RewardChips != nil {
		t.Fatal("forfeit receipt or reward changed", receipt)
	}
	f.restart(t)
	replay, err := f.s.fieldBossFightEnd(ctx, f.id, request)
	if err != nil || !proto.Equal(ended[0], replay[0]) || !reflect.DeepEqual(receipt, read()) {
		t.Fatal("cold forfeit replay changed", err)
	}
	_, cold := f.profile(t)
	if cold.GGOField.Chips != ([6]int64{5}) || cold.GGOField.PositionRevision != 8 {
		t.Fatal("replay repeated loss or safe return")
	}
}

// The original round limit has a failed result despite surviving heroes.
func TestGGOBossSurvivingTimeoutDoesNotForfeit(t *testing.T) {
	t.Setenv("HYJS_P1_MYSQL_DATABASE", "")
	t.Setenv("HYJS_GAME_ACCOUNTS_DATABASE", "")
	f, ctx, session, event := ggoBossIntegrationFixture(t)
	messages, err := f.s.fieldBossFightStart(ctx, f.id, &pb.CSSidePveBossFightStart{Uuid: proto.String(event.ID)})
	if err != nil {
		t.Fatal(err)
	}
	start := messages[0].(*pb.SCSidePveBossFightStart)
	if err = updateProfile(ctx, f.id, func(_ *pb.Player, game *model.GameState) error {
		fight := game.Field.Fight
		input, e := fieldBossFrozenInput(fight)
		if e != nil {
			return e
		}
		for _, hero := range input.Attacker.Formation {
			hero.Prop.Hp = proto.Float64(1e9)
			hero.Inst.CurHp = proto.Float64(1e9)
			hero.Prop.Att = proto.Float64(1)
		}
		for _, enemy := range input.Monster.Formation {
			enemy.Prop.Att = proto.Float64(1)
		}
		fight.BossStart.BattlePlayer, fight.Start.BattlePlayer = input.Attacker, input.Attacker
		fight.BossStart.BattleMonsterList, fight.Start.BattleMonsterList = []*pb.BattleMonster{input.Monster}, []*pb.BattleMonster{input.Monster}
		raw, e := json.Marshal(input)
		if e != nil {
			return e
		}
		fight.BossInput, fight.BossInputStored, fight.BossInputHash = input, nil, ""
		fight.GGOBoss.InputHash = p6Digest(raw)
		return nil
	}); err != nil {
		t.Fatal("prepare isolated timeout", err)
	}
	_, before := f.profile(t)
	input, err := fieldBossFrozenInput(before.Field.Fight)
	if err != nil {
		t.Fatal(err)
	}
	terminal, _, err := f.s.ComputeP6GenericMechanismBattle(ctx, input, nil)
	if err != nil || terminal.Outcome.Winner != 1 || terminal.OriginalOperates.GetFinalRoundNum() != input.MaxRound+1 || terminal.OriginalOperates.GetAttackerTotalLeftHp() <= 0 {
		t.Fatal("fixture must time out with survivors", terminal, err)
	}
	request := &pb.CSSidePveBossFightEnd{FightId: start.FightId, Uuid: proto.String(event.ID), OptRecord: terminal.OriginalOperates}
	ended, err := f.s.fieldBossFightEnd(ctx, f.id, request)
	if err != nil || ended[0].(*pb.SCSidePveBossFightEnd).GetFightResult() != 0 {
		t.Fatal("native timeout", err)
	}
	_, after := f.profile(t)
	receipt, err := f.s.GGOFieldBossReceipt(ctx, model.GGOFieldBossReceiptRequest{GGOFieldAuth: model.GGOFieldAuth{PlayerID: f.id, SessionHex: hex.EncodeToString(session)}, FightID: start.GetFightId()}, session)
	if err != nil || receipt.Defeated || receipt.Forfeit || receipt.SafeReturn != nil || receipt.Lost != ([6]int64{}) || after.GGOField.Chips != before.GGOField.Chips || after.GGOField.PositionRevision != before.GGOField.PositionRevision {
		t.Fatal("surviving timeout treated as defeat", receipt, err)
	}
}
