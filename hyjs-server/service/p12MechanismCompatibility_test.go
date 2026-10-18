package service

import (
	"encoding/json"
	"fmt"
	"math"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestP12MechanismOriginalBossConfigurationPins(t *testing.T) {
	s := p6OriginalTables(t)
	world, _ := p12WorldBossBoostOriginalTable(t, "WorldBoss")
	factors, _ := p12WorldBossBoostOriginalTable(t, "WorldBossFactor")
	def := &p12WorldBossDefinition{Tables: map[string][]map[string]string{"WorldBoss": world, "WorldBossFactor": factors}}
	for level := int32(1); level <= 50; level++ {
		monster, _, err := s.p12WorldBossBattleMonster(def, int64(level))
		if err != nil {
			t.Fatal(level, err)
		}
		for index, actor := range monster.Formation {
			raw, _ := proto.MarshalOptions{Deterministic: true}.Marshal(actor)
			if p6Digest(raw) != p12MechanismWorldBossNPCPins[level][index] {
				t.Fatal("original row*factor static PB differs", level, index)
			}
		}
	}
	t.Log("50 original level configurations/200 NPC PBs built and matched; zero battles and no added native claim")
}

func TestP12MechanismTypedModeRepresentativesAndColdReceipts(t *testing.T) {
	s := p6OriginalTables(t)
	grown := p6GenericGrownActors(t)
	actors := func(ids []string, camp int) []*pb.BattleHero {
		out := []*pb.BattleHero{}
		for index, id := range ids {
			actor := proto.Clone(grown[id]).(*pb.BattleHero)
			actor.Guid = proto.String(fmt.Sprintf("00000000-0000-4000-8000-%012d", camp*10+index+1))
			actor.Pose = proto.Int32([]int32{3, 7, 9, 13}[index])
			actor.Prop.Att = proto.Float64(actor.Prop.GetAtt() + .25)
			out = append(out, actor)
		}
		return out
	}
	for _, mode := range []int32{18, 8, 5} {
		t.Run(fmt.Sprint(mode), func(t *testing.T) {
			var input *model.FrozenBattleInput
			var compute func(*model.FrozenBattleInput) (*model.CombatFinalHPOutcome, error)
			var receipt func(*model.FrozenBattleInput, *model.CombatFinalHPOutcome) error
			switch mode {
			case 18:
				input = p12WorldBossPartyScopeInput(t, s)
				input.Attacker.Formation = actors([]string{"hero_025", "hero_011", "hero_001"}, 0)
				input.Attacker.Inst.Anger = proto.Float64(100)
				input.Attacker.Formation[0].Inst.CurHp = proto.Float64(input.Attacker.Formation[0].Prop.GetHp() * .8)
				compute = func(in *model.FrozenBattleInput) (*model.CombatFinalHPOutcome, error) {
					return s.computeP12WorldBossCertifiedFinalHP(t.Context(), in)
				}
				receipt = validateP12WorldBossCertifiedReceipt
			case 8:
				_, input = p12WantedOriginalFixture(t)
				input.Attacker.Formation = actors([]string{"hero_071", "hero_011", "hero_001"}, 0)
				input.Monster.Formation[0].Inst.CurHp = proto.Float64(0)
				input.Monster.Formation[1].Inst.CurHp = proto.Float64(21.125)
				input.AttackerDamageFactor = proto.Float64(2.5)
				compute = func(in *model.FrozenBattleInput) (*model.CombatFinalHPOutcome, error) {
					return s.ComputeP12WantedProtocolFinalHP(t.Context(), in)
				}
				receipt = ValidateP12WantedCertifiedReceipt
			case 5:
				wire := p12FieldPVPNativeTypedInput(t, p12FieldPVPNativeCases(t)[0])
				wire.Attacker.Inst.FirstOrder = proto.Int32(0)
				wire.Defender1.Inst.FirstOrder = proto.Int32(1)
				var err error
				input, err = s.FreezeBattleInput(model.FrozenBattleInput{Attacker: wire.Attacker, Defender: wire.Defender1, Seed: 42, MaxRound: 40, BattleType: 5, DefenderFirst: true, AttackerDamageFactor: proto.Float64(1), DefenderDamageFactor: proto.Float64(1)})
				if err != nil {
					t.Fatal(err)
				}
				input.Attacker.Formation = actors([]string{"hero_025", "hero_011"}, 0)
				input.Defender.Formation = actors([]string{"hero_071", "hero_001"}, 1)
				input.Attacker.ServerId = proto.Int32(1)
				input.Defender.ServerId = proto.Int32(1)
				input.Attacker.Inst.FirstOrder = proto.Int32(0)
				input.Defender.Inst.FirstOrder = proto.Int32(1)
				input.Attacker.Inst.Anger = proto.Float64(100)
				input.Defender.Inst.Anger = proto.Float64(200)
				for _, party := range []*pb.BattlePlayer{input.Attacker, input.Defender} {
					for _, actor := range party.Formation {
						actor.Inst.CurHp = proto.Float64(actor.Prop.GetHp() * .8)
					}
				}
				compute = func(in *model.FrozenBattleInput) (*model.CombatFinalHPOutcome, error) {
					return s.ComputeP12FieldPVPProtocolFinalHP(t.Context(), in)
				}
				receipt = ValidateP12FieldPVPCertifiedReceipt
			}
			proof, _ := p12MechanismIdentity(mode)
			out, err := compute(input)
			if err != nil {
				t.Fatal("original typed mechanism computation", err)
			}
			if out.Outcome.CapabilityHash != proof || !out.Outcome.Authoritative {
				t.Fatal("named local compatibility not selected")
			}
			if err = receipt(input, out); err != nil {
				t.Fatal("cold original receipt", err)
			}
			before, _ := json.Marshal(input)
			raw, _ := json.Marshal(out)
			for _, variant := range []string{"HP", "AP", "source", "foreign-proof", "actor-source"} {
				var bad model.CombatFinalHPOutcome
				if json.Unmarshal(raw, &bad) != nil {
					t.Fatal("cold JSON")
				}
				switch variant {
				case "HP":
					bad.FinalActors[0].CurrentHP++
				case "AP":
					bad.Outcome.Attacker.Inst.Anger = proto.Float64(301)
				case "source":
					bad.CalculationSourceSHA256 = "foreign"
				case "foreign-proof":
					bad.Outcome.CapabilityHash = constUtil.P6StarterNativeProof
				case "actor-source":
					bad.FinalActors[0].InitialActorSHA256 = ""
				}
				if receipt(input, &bad) == nil {
					t.Fatal("forged typed receipt accepted", variant)
				}
			}
			if mode == 18 {
				if _, err = p12WorldBossBattleDamage(input, out); err != nil {
					t.Fatal("original shared damage extraction", err)
				}
			}
			if mode == 5 {
				for camp, party := range []*pb.BattlePlayer{input.Attacker, input.Defender} {
					side := &pb.PlayerSide{CurHp: proto.Float64(.8)}
					if err = p12FieldPVPApplyTerminal(party, side, out, int32(camp)); err != nil {
						t.Fatal("all actual character HP/AP persistence", err)
					}
					if len(side.BattlePosHps) != 2 || !combatFinite(side.GetCurHp()) || side.GetCurHp() < 0 || side.GetCurHp() > 1 {
						t.Fatal("field persistent percentage slots")
					}
					var weighted, maximum float64
					for index, actor := range party.Formation {
						weighted += actor.Prop.GetHp() * side.BattlePosHps[index].GetHpPercent()
						maximum += actor.Prop.GetHp()
					}
					if math.Abs(weighted/maximum-side.GetCurHp()) > 1e-15 {
						t.Fatal("existing P8 aggregate changed")
					}
				}
			}
			after, _ := json.Marshal(input)
			if string(before) != string(after) {
				t.Fatal("frozen source HP/AP/initiative mutated")
			}
			t.Logf("typed%d actors=%d localProof=%s winner=%d round=%d; sourceHP/AP retained; cold+forgeries verified", mode, len(out.FinalActors), proof, out.Outcome.Winner, out.Outcome.FinalRound)
		})
	}
}
