package service

import (
	"math"
	"reflect"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Exercise the native field endpoint, durable loss receipt, safe return and
// CurrentPosition handshake with an isolated role. Modified battle stats are
// finite fixtures used only to produce each terminal condition.
func TestGGOMonsterDefeatReturnsToSafeOnce(t *testing.T) {
	for _, scenario := range []string{"client-loss-fallback", "round-limit", "all-heroes-defeated"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := ggoResume86Setup(t, constUtil.GGOBattleResumeMonster)
			if err := updateProfile(fixture.ctx, fixture.f.id, func(_ *pb.Player, g *model.GameState) error {
				g.GGOField.Chips = [6]int64{5, 0, 0, 0, 0, 3}
				g.GGOField.Revision = 10
				g.GGOField.MonsterFight.ProtectedChips = 3
				g.GGOField.MonsterFight.DeathLossPercent = ggoInt64(50)
				if scenario == "client-loss-fallback" {
					return nil
				}
				fight := g.Field.Fight
				input, err := fieldBossFrozenInput(fight)
				if err != nil {
					return err
				}
				for _, enemy := range input.Monster.Formation {
					enemy.Prop.Hp = proto.Float64(1e12)
					enemy.Inst.CurHp = proto.Float64(1e12)
					enemy.Prop.Att = proto.Float64(1e9)
				}
				input.MaxRound = 1
				if scenario == "all-heroes-defeated" {
					input.MaxRound = 2
					for _, hero := range input.Attacker.Formation {
						hero.Inst.CurHp = proto.Float64(1)
					}
				}
				fight.Start.Maxround = proto.Int32(input.MaxRound)
				fight.Start.BattlePlayer = proto.Clone(input.Attacker).(*pb.BattlePlayer)
				fight.Start.BattleMonsterList = []*pb.BattleMonster{proto.Clone(input.Monster).(*pb.BattleMonster)}
				fight.BossInput, fight.BossInputStored, fight.BossInputHash = input, nil, ""
				return nil
			}); err != nil {
				t.Fatal("prepare owned role and terminal fixture", err)
			}
			_, before := fixture.f.profile(t)
			frozen := *before.GGOField.MonsterFight
			input, err := fieldBossFrozenInput(before.Field.Fight)
			if err != nil {
				t.Fatal(err)
			}
			terminal, _, err := fixture.f.s.ComputeP6GenericMechanismBattle(fixture.ctx, input, nil)
			if err != nil {
				t.Fatal("automatic terminal", err)
			}
			var attackerHP float64
			for _, actor := range terminal.FinalActors {
				if actor.Camp == 0 {
					attackerHP += actor.CurrentHP
				}
			}
			if scenario == "client-loss-fallback" && (terminal.Outcome.Winner != 0 || attackerHP <= 0) {
				t.Fatal("fixture must have a server automatic win with survivors", terminal.Outcome.Winner, attackerHP)
			}
			if scenario == "round-limit" && (terminal.Outcome.Winner != 1 || attackerHP <= 0) {
				t.Fatal("fixture must lose at the round limit with survivors", terminal.Outcome.Winner, attackerHP)
			}
			if scenario == "all-heroes-defeated" && (terminal.Outcome.Winner != 1 || attackerHP != 0) {
				t.Fatal("fixture must lose with no surviving HP", terminal.Outcome.Winner, attackerHP)
			}
			request := ggoResume86EndRequest(t, fixture).(*pb.CSSidePveFightEnd)
			if scenario == "client-loss-fallback" {
				// ProtoBuf-net omits the original client's default zero on the wire.
				request.Result = nil
				request.OptRecord = nil
			}
			var messages []proto.Message
			if scenario == "client-loss-fallback" && db.GetIns().Dialector.Name() == "mysql" {
				type reply struct {
					messages []proto.Message
					err      error
				}
				start := make(chan struct{})
				replies := make(chan reply, 4)
				for i := 0; i < cap(replies); i++ {
					go func() {
						<-start
						got, e := fixture.f.s.fieldFightEnd(fixture.ctx, fixture.f.id, request)
						replies <- reply{messages: got, err: e}
					}()
				}
				close(start)
				var all []reply
				for i := 0; i < cap(replies); i++ {
					all = append(all, <-replies)
				}
				var committed int
				for _, response := range all {
					if response.err != nil || len(response.messages) == 0 {
						t.Fatal("concurrent native loss", response.err)
					}
					if len(response.messages) == 3 {
						messages = response.messages
						committed++
					} else if len(response.messages) != 1 {
						t.Fatal("unexpected concurrent response", len(response.messages))
					}
				}
				if committed != 1 {
					t.Fatal("loss must commit once", committed)
				}
				for _, response := range all {
					if !proto.Equal(response.messages[0], messages[0]) {
						t.Fatal("concurrent retry returned a different receipt")
					}
				}
			} else {
				messages, err = fixture.f.s.fieldFightEnd(fixture.ctx, fixture.f.id, request)
			}
			if err != nil || len(messages) != 3 {
				t.Fatal("native loss settlement and refresh", len(messages), err)
			}
			result, ok := messages[0].(*pb.SCSidePveFightEnd)
			if !ok || result.GetCode() != 1 || result.GetFightResult() != 0 || result.GetState() != constUtil.P8FieldDead || len(result.GetGetn().GetSimps()) != 0 {
				t.Fatal("native loss response", messages[0])
			}
			if _, ok = messages[2].(*pb.SCRefreshScene); !ok {
				t.Fatal("missing native refresh signal", messages)
			}
			_, game := fixture.f.profile(t)
			point, err := ggoWirePoint(frozen.SafeReturn)
			if err != nil {
				t.Fatal(err)
			}
			if game.GGOField.Chips != ([6]int64{1, 0, 0, 0, 0, 3}) || game.GGOField.Revision != 11 || !game.PositionLoading || !proto.Equal(game.Position.GetCurrent(), point) || game.Field.Side.GetState() != constUtil.P8FieldIdle || math.Abs(game.Field.Side.GetCurHp()-frozen.HealRatio) > 1e-9 || game.Field.Side.GetKilleds() != before.Field.Side.GetKilleds()+1 {
				t.Fatal("loss, safe position or revival state incorrect", game.GGOField.Chips, game.GGOField.Revision, game.Position, game.Field.Side)
			}
			life, err := p8LifeState(db.GetIns(), fixture.f.id)
			if err != nil || !life.InSafe || life.DeathAt != 0 || life.InvincibleUntil != 0 || life.SafeRecoveryAt <= 0 || life.LastFightID != request.GetFightId() {
				t.Fatal("safe-area life state", life, err)
			}
			var receipt model.GGOFieldMonsterReceipt
			found, err := ggoFindBusinessReceipt(db.GetIns(), fixture.f.id, constUtil.GGOFieldOpMonsterResult, request.GetFightId(), &receipt)
			if err != nil || !found || receipt.Lost != ([6]int64{4, 0, 0, 0, 0, 0}) {
				t.Fatal("durable loss receipt", receipt, err)
			}
			settled := ggoResume86SnapshotFor(t, fixture.f)
			repeated, err := fixture.f.s.fieldFightEnd(fixture.ctx, fixture.f.id, request)
			if err != nil || len(repeated) != 1 || !proto.Equal(repeated[0], result) || !reflect.DeepEqual(settled, ggoResume86SnapshotFor(t, fixture.f)) {
				t.Fatal("retry changed the settled loss", err)
			}
			fixture.f.restart(t)
			repeated, err = fixture.f.s.fieldFightEnd(fixture.ctx, fixture.f.id, request)
			if err != nil || len(repeated) != 1 || !proto.Equal(repeated[0], result) || !reflect.DeepEqual(settled, ggoResume86SnapshotFor(t, fixture.f)) {
				t.Fatal("cold retry changed the settled loss", err)
			}
			scene, err := fixture.f.s.fieldScene(fixture.ctx, fixture.f.id, false)
			if err != nil || len(scene) == 0 {
				t.Fatal("native current-position handshake", err)
			}
			position, ok := scene[0].(*pb.SCCurrentPosition)
			if !ok || !proto.Equal(position.GetSelf().GetPosition().GetCurrent(), point) || position.GetSelf().GetInfo().GetPlayerSide().GetState() != constUtil.P8FieldIdle {
				t.Fatal("current-position response did not restore the safe role", scene[0])
			}
			_, game = fixture.f.profile(t)
			if game.PositionLoading || game.GGOField.Chips != ([6]int64{1, 0, 0, 0, 0, 3}) {
				t.Fatal("position handshake changed chips or remained pending")
			}
		})
	}
}
