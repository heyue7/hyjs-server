package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"math"
	"testing"
	"time"
)

func TestP12FieldPVPFive55ActualTransactions(t *testing.T) {
	for _, sizes := range [][2]int{{5, 5}, {1, 5}, {5, 1}, {1, 1}} {
		for _, reverse := range []bool{false, true} {
			t.Run(fmt.Sprintf("%dv%d/reverse=%v", sizes[0], sizes[1], reverse), func(t *testing.T) {
				f, peer, _, sessions := p12FieldPVPSetup(t)
				p12FieldPVPEnable(t)
				f.s.P5Ready = true
				ids := []int64{f.id, peer}
				for camp, id := range ids {
					if err := updateProfile(f.ctx, id, func(p *pb.Player, g *model.GameState) error {
						p.Level = proto.Int32(100)
						g.Field.Side.CurHp = proto.Float64(.8)
						g.Field.Side.Anger = proto.Float64(37)
						_, err := f.s.GrantFixedReward(p, g, "item_373,6")
						return err
					}); err != nil {
						t.Fatal(err)
					}
					if sizes[camp] == 5 {
						if _, err := f.s.tacticalFormationUnlock(f.ctx, id, &pb.CSTaticalFormationUnLock{TacticalId: proto.String("2")}, 550001); err != nil {
							t.Fatal("unlock five-person formation", err)
						}
						if _, err := f.s.tacticalFormationUse(f.ctx, id, &pb.CSUseTaticalFormation{Id: proto.String("2")}); err != nil {
							t.Fatal("use five-person formation", err)
						}
					}
					for i, hero := range []string{"hero_011", "hero_025", "hero_003", "hero_018"} {
						if i+1 >= sizes[camp] {
							break
						}
						p11ClaimAndFormPartner(t, f, id, hero, int32(i+2))
					}
				}
				attacker, defender := f.id, peer
				attCount, defCount := sizes[0], sizes[1]
				if reverse {
					attacker, defender = defender, attacker
					attCount, defCount = defCount, attCount
				}
				request := &pb.CSSidePvpFightResult{PlayerId: proto.Int64(defender), FightType: proto.Int32(0)}
				now := time.Now()
				stamp := int64(550050)
				if reverse {
					stamp = 0
				}
				replies, err := f.s.p12FieldPVPFightAt(f.ctx, attacker, sessions[attacker], request, stamp, now)
				response := p12FieldPVPResponse(t, replies, err)
				if response.GetCode() != 1 {
					t.Fatal("legal five-person PK rejected", response.GetCode())
				}
				var receipt db.P12FieldPVPReceipt
				if err = db.GetIns().First(&receipt, "player_id=?", attacker).Error; err != nil {
					t.Fatal(err)
				}
				_, replay, err := f.s.p12FieldPVPReadReceipt(db.GetIns(), &receipt)
				if err != nil {
					t.Fatal(err)
				}
				corrupt := receipt
				corrupt.ScopeProof = "foreign-proof"
				if _, _, err = f.s.p12FieldPVPReadReceipt(db.GetIns(), &corrupt); err == nil {
					t.Fatal("receipt proof was not bound to certified outcome")
				}
				if replay.GetAttFirstPlayerId() != attacker || replay.AttPlayer.GetPlayerId() != attacker || replay.AttPlayer.Inst.GetFirstOrder() != 1 || replay.DefPlayer.Inst.GetFirstOrder() != 0 || len(replay.AttPlayer.Formation) != attCount || len(replay.DefPlayer.Formation) != defCount {
					t.Fatal("initiator order or owned formation lost")
				}
				for _, party := range []*pb.BattlePlayer{replay.AttPlayer, replay.DefPlayer} {
					if party.Inst.GetAnger() != 37 {
						t.Fatal("input AP reset")
					}
					for _, actor := range party.Formation {
						if actor.Inst.GetCurHp() != actor.Prop.GetHp()*.8 {
							t.Fatal("input injury reset")
						}
					}
				}
				var record db.P6BattleRecord
				if err = db.GetIns().First(&record, "id=?", receipt.P6RecordID).Error; err != nil {
					t.Fatal(err)
				}
				frozen, err := decodeP6Record(&record)
				if err != nil || frozen.DefenderFirst || ValidateP6FirstOrder(frozen) != nil {
					t.Fatal("frozen initiating order", err)
				}
				// Oversized and inconsistent input stays rejected without changing real actors or skills.
				six := *frozen
				six.Attacker = proto.Clone(frozen.Attacker).(*pb.BattlePlayer)
				extra := proto.Clone(six.Attacker.Formation[0]).(*pb.BattleHero)
				extra.Guid = proto.String("sixth55")
				extra.Pose = proto.Int32(15)
				six.Attacker.Formation = append(six.Attacker.Formation, extra)
				for len(six.Attacker.Formation) < 6 {
					copy := proto.Clone(extra).(*pb.BattleHero)
					copy.Guid = proto.String(fmt.Sprint("extra", len(six.Attacker.Formation)))
					six.Attacker.Formation = append(six.Attacker.Formation, copy)
				}
				if _, err = P12FieldPVPProofForInput(&six); err == nil {
					t.Fatal("six-person input accepted")
				}
				bad := *frozen
				bad.DefenderFirst = true
				if _, err = P12FieldPVPProofForInput(&bad); err == nil {
					t.Fatal("contradictory first order accepted")
				}
				var terminal model.CombatFinalHPOutcome
				if err = json.Unmarshal(receipt.Terminal, &terminal); err != nil {
					t.Fatal(err)
				}
				for camp, id := range []int64{attacker, defender} {
					var profile db.Profile
					if err = db.GetIns().First(&profile, id).Error; err != nil {
						t.Fatal(err)
					}
					p, g, e := decodeProfile(&profile)
					if e != nil {
						t.Fatal(e)
					}
					party := terminal.Outcome.Attacker
					if camp == 1 {
						party = terminal.Outcome.Defender
					}
					if g.Field.Side.GetAnger() != party.Inst.GetAnger() {
						t.Fatal("terminal AP not persisted")
					}
					byPose := map[int32]float64{}
					for _, hp := range g.Field.Side.BattlePosHps {
						byPose[hp.GetBattlePos()] = hp.GetHpPercent()
					}
					for _, actor := range terminal.FinalActors {
						if int(actor.Camp) == camp && math.Abs(byPose[actor.Pose]-actor.CurrentHP/actor.MaxHP) > 1e-12 {
							t.Fatal("terminal per-actor HP not persisted")
						}
					}
					expected := int64(100000)
					if camp == 0 {
						expected += receipt.LootCoin
					} else {
						expected -= receipt.LootCoin
					}
					if p.GetCoin() != expected {
						t.Fatal("coin settlement", p.GetCoin(), expected)
					}
				}
				assertMV := func() {
					for _, viewer := range []int64{attacker, defender} {
						messages, e := f.s.p12FieldPVPMV(f.ctx, viewer, sessions[viewer], &pb.CSSideGetPvpMV{FightId: response.FightId})
						if e != nil || len(messages) != 1 || !proto.Equal(messages[0].(*pb.SCSideGetPvpMV).Data, replay) {
							t.Fatal("actual MV playback data unavailable", e)
						}
					}
				}
				assertMV()
				saved := p12FieldPVPProfiles(t, attacker, defender)
				for i := 0; i < 2; i++ {
					if i == 1 {
						f.restart(t)
						assertMV()
					}
					out, e := f.s.p12FieldPVPFightAt(f.ctx, attacker, sessions[attacker], request, stamp, now.Add(time.Second))
					if !proto.Equal(response, p12FieldPVPResponse(t, out, e)) || !bytes.Equal(saved, p12FieldPVPProfiles(t, attacker, defender)) {
						t.Fatal("retry/cold changed assets or terminal")
					}
				}
				var count int64
				if err = db.GetIns().Model(&db.P12FieldPVPReceipt{}).Where("player_id=?", attacker).Count(&count).Error; err != nil || count != 1 {
					t.Fatal("duplicate transaction", count, err)
				}
			})
		}
	}
}
