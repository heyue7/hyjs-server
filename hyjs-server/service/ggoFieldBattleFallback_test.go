package service

import (
	"context"
	"errors"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestOrdinaryFieldEndTransportValidation(t *testing.T) {
	valid := &pb.CSSidePveFightEnd{
		FightId: proto.String("ordinary-fight"),
		BattlePlayer: &pb.BattlePlayer{PlayerId: proto.Int64(4), Inst: &pb.BattlePlayerInst{},
			Formation: []*pb.BattleHero{{Id: proto.String("hero_001"), Guid: proto.String("owned"), Pose: proto.Int32(1), Inst: &pb.BattleHeroInst{}}}},
		OptRecord: &pb.BattleOperate{FinalRoundNum: proto.Int32(41)},
	}
	if err := validateOrdinaryFieldEnd(valid); err != nil {
		t.Fatal("original zero-valued loss rejected", err)
	}
	for _, change := range []func(*pb.CSSidePveFightEnd){
		func(b *pb.CSSidePveFightEnd) { b.OptRecord = nil },
		func(b *pb.CSSidePveFightEnd) { b.OptRecord.FinalRoundNum = proto.Int32(42) },
		func(b *pb.CSSidePveFightEnd) { b.BattlePlayer.Inst = nil },
		func(b *pb.CSSidePveFightEnd) { b.BattlePlayer.Formation[0].Inst.CurHp = proto.Float64(math.NaN()) },
		func(b *pb.CSSidePveFightEnd) { b.BattlePlayer.Formation[0].Inst.CurHp = proto.Float64(-1) },
	} {
		bad := proto.Clone(valid).(*pb.CSSidePveFightEnd)
		change(bad)
		if err := validateOrdinaryFieldEnd(bad); err == nil {
			t.Fatal("ordinary field accepted invalid payload")
		}
	}
}

// The test drives the real native start/end transaction using an isolated
// fixture. It checks both a rejected record and settlement from frozen input.
func TestGGOBattleFallbackUsesFrozenResult(t *testing.T) {
	var shared *ggoResume86Base
	if os.Getenv("HYJS_P1_MYSQL_DATABASE") != "" {
		shared = ggoResume86BaseSetup(t)
	}
	for _, kind := range []int64{constUtil.GGOBattleResumeMonster, constUtil.GGOBattleResumeBoss} {
		name := "monster"
		if kind == constUtil.GGOBattleResumeBoss {
			name = "boss"
		}
		t.Run(name, func(t *testing.T) {
			var fixture *ggoResume86Fixture
			if shared == nil {
				fixture = ggoResume86Setup(t, kind)
			} else {
				fixture = ggoResume86SetupFromBase(t, kind, shared)
			}
			_, game := fixture.f.profile(t)
			input, err := fieldBossFrozenInput(game.Field.Fight)
			if err != nil {
				t.Fatal(err)
			}
			automatic, _, err := fixture.f.s.ComputeP6GenericMechanismBattle(fixture.ctx, input, nil)
			if err != nil {
				t.Fatal("automatic terminal", err)
			}
			expected := int32(0)
			if automatic.Outcome.Winner == 0 {
				expected = 1
			}
			badRecord := proto.Clone(automatic.OriginalOperates).(*pb.BattleOperate)
			if len(badRecord.SkillOpts) == 0 {
				t.Fatal("fixture has no recorded operation")
			}
			badRecord.SkillOpts[0].ActorUid = proto.String("no-such-battle-actor")
			if _, err = fixture.f.s.ComputeOriginalP6ProtocolFinalHP(fixture.ctx, input, badRecord); err == nil {
				t.Fatal("fixture did not reject the broken operation")
			}
			for _, record := range []*pb.BattleOperate{nil, badRecord} {
				got, result, e := fixture.f.s.ggoBattleTerminal(fixture.ctx, input, record, 1-expected, nil, fixture.f.id, true)
				if e != nil || result != expected || !reflect.DeepEqual(got.FinalActors, automatic.FinalActors) {
					t.Fatal("fallback trusted supplied record, result or HP", result, e)
				}
			}
			cancelled, cancel := context.WithCancel(fixture.ctx)
			cancel()
			if _, _, err = fixture.f.s.ggoBattleTerminal(cancelled, input, badRecord, expected, nil, fixture.f.id, true); !errors.Is(err, context.Canceled) {
				t.Fatal("cancelled request entered settlement", err)
			}
			end := ggoResume86EndRequest(t, fixture)
			switch request := end.(type) {
			case *pb.CSSidePveFightEnd:
				// A mismatched explicit monster loss has its own conservative
				// settlement test; this case isolates invalid-record fallback.
				request.OptRecord, request.Result, request.BattlePlayer = badRecord, proto.Int32(expected), nil
			case *pb.CSSidePveBossFightEnd:
				request.OptRecord, request.Result = nil, proto.Int32(1-expected)
			}
			before := ggoResume86SnapshotFor(t, fixture.f)
			wrong := proto.Clone(end)
			if request, ok := wrong.(*pb.CSSidePveFightEnd); ok {
				request.FightId = proto.String("other-fight")
			} else {
				wrong.(*pb.CSSidePveBossFightEnd).Uuid = proto.String("other-event")
			}
			if _, err = ggoResume86End(fixture, wrong); err == nil {
				t.Fatal("wrong fight identity accepted")
			}
			if !reflect.DeepEqual(before, ggoResume86SnapshotFor(t, fixture.f)) {
				t.Fatal("wrong identity changed state")
			}
			originalHash := game.Field.Fight.BossInputHash
			if originalHash == "" {
				t.Fatal("fixture has no frozen input digest")
			}
			setInputHash := func(value string) {
				t.Helper()
				if e := updateProfile(fixture.ctx, fixture.f.id, func(_ *pb.Player, state *model.GameState) error {
					state.Field.Fight.BossInputHash = value
					return nil
				}); e != nil {
					t.Fatal("set frozen digest", e)
				}
			}
			setInputHash(strings.Repeat("0", 64))
			corrupt := ggoResume86SnapshotFor(t, fixture.f)
			if _, err = ggoResume86End(fixture, end); err == nil || !reflect.DeepEqual(corrupt, ggoResume86SnapshotFor(t, fixture.f)) {
				t.Fatal("corrupt frozen input was settled", err)
			}
			setInputHash(originalHash)
			messages, err := ggoResume86End(fixture, end)
			if err != nil || len(messages) == 0 {
				t.Fatal("fallback settlement", err)
			}
			var result int32
			switch response := messages[0].(type) {
			case *pb.SCSidePveFightEnd:
				result = response.GetFightResult()
			case *pb.SCSidePveBossFightEnd:
				result = response.GetFightResult()
			}
			if result != expected {
				t.Fatal("client result decided settlement", result, expected)
			}
			_, game = fixture.f.profile(t)
			if game.Field == nil || game.Field.Fight == nil || !game.Field.Fight.Settled {
				t.Fatal("fight was not settled")
			}
			if kind == constUtil.GGOBattleResumeBoss {
				var receipt model.GGOFieldBossBattleReceipt
				found, e := ggoFindBusinessReceipt(db.GetIns(), fixture.f.id, constUtil.GGOFieldBossFightOperation, fixture.req.ExpectedFightID, &receipt)
				var bossHP float64
				for _, actor := range automatic.FinalActors {
					if actor.Camp == 1 {
						bossHP = actor.CurrentHP
					}
				}
				wantDamage := math.Max(0, input.Monster.Formation[0].GetInst().GetCurHp()-bossHP)
				if e != nil || !found || receipt.InputHash != fixture.req.ExpectedInputHash || math.Abs(receipt.Damage-wantDamage) > 0.00001 {
					t.Fatal("boss damage did not use server terminal", receipt.Damage, wantDamage, e)
				}
			} else {
				var receipt model.GGOFieldMonsterReceipt
				found, e := ggoFindBusinessReceipt(db.GetIns(), fixture.f.id, constUtil.GGOFieldOpMonsterResult, fixture.req.ExpectedFightID, &receipt)
				if e != nil || !found || receipt.InputHash != fixture.req.ExpectedInputHash || game.GGOField.MonsterFight != nil {
					t.Fatal("monster receipt or active fight invalid", e)
				}
			}
			settled := ggoResume86SnapshotFor(t, fixture.f)
			repeated, err := ggoResume86End(fixture, end)
			if err != nil || len(repeated) == 0 || !proto.Equal(messages[0], repeated[0]) || !reflect.DeepEqual(settled, ggoResume86SnapshotFor(t, fixture.f)) {
				t.Fatal("identical retry changed settlement", err)
			}
			changed := proto.Clone(end)
			if request, ok := changed.(*pb.CSSidePveFightEnd); ok {
				request.Result = proto.Int32(1 - expected)
			} else {
				changed.(*pb.CSSidePveBossFightEnd).Result = proto.Int32(expected)
			}
			if _, err = ggoResume86End(fixture, changed); err == nil || !reflect.DeepEqual(settled, ggoResume86SnapshotFor(t, fixture.f)) {
				t.Fatal("changed retry reused receipt or changed state", err)
			}
			fixture.f.restart(t)
			repeated, err = ggoResume86End(fixture, end)
			if err != nil || len(repeated) == 0 || !proto.Equal(messages[0], repeated[0]) {
				t.Fatal("cold receipt recovery", err)
			}
		})
	}
}
