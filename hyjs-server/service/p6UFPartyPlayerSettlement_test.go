package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// A diagnostic composition of actual original mode14 source actors. These
// additions exercise code contracts only; they are not native party evidence.
func p6UFPartyFixture(t *testing.T, a, b int) (*GameService, *model.FrozenBattleInput) {
	t.Helper()
	s := p6OriginalTables(t)
	raw, err := os.ReadFile("../../analysis/p6/native-actual-war-wounded14-inputs-v196/000.SCUFFightRecordReplay.pb")
	if err != nil {
		t.Fatal(err)
	}
	typed := &pb.SCUFFightRecordReplay{}
	if err = proto.Unmarshal(raw, typed); err != nil {
		t.Fatal(err)
	}
	for camp, size := range []int{a, b} {
		player := typed.PlayerList[camp]
		source := proto.Clone(player.Formation[0]).(*pb.BattleHero)
		player.Formation = nil
		player.Inst.FirstOrder = proto.Int32(int32(1 - camp))
		player.Inst.Anger = proto.Float64(37.5 + float64(camp))
		for index := 0; index < size; index++ {
			hero := proto.Clone(source).(*pb.BattleHero)
			hero.Guid = proto.String(fmt.Sprintf("diagnostic-party-%d-%d", camp, index))
			hero.Pose = proto.Int32(int32(index + 1))
			if index == 0 && size > 1 {
				hero.Inst.CurHp = proto.Float64(0)
			}
			player.Formation = append(player.Formation, hero)
		}
	}
	input, err := s.FreezeP6UFPlayerInput(typed)
	if err != nil {
		t.Fatal(err)
	}
	return s, input
}

func TestP6UFPartyPlayerOneToFourCurrentScopeAndNoAuthority(t *testing.T) {
	for _, sizes := range [][2]int{{1, 1}, {2, 2}, {3, 3}, {4, 4}, {1, 4}, {4, 1}} {
		t.Run(fmt.Sprint(sizes), func(t *testing.T) {
			s, input := p6UFPartyFixture(t, sizes[0], sizes[1])
			before, _ := json.Marshal(input)
			q, err := s.ValidateP6UFPartyPlayerMechanisms(context.Background(), input)
			if err != nil || q == nil || q.NativeCertified || q.ActorCount != sizes[0]+sizes[1] {
				t.Fatal(q, err)
			}
			if constUtil.P6UFPartyPlayerNativeProof == "" {
				if out, err := s.ComputeP6UFPartyPlayerSettlement(context.Background(), input); err == nil || out != nil {
					t.Fatal("empty native certificate opened party authority", out, err)
				}
			} else {
				out, e := s.ComputeP6UFPartyPlayerSettlement(context.Background(), input)
				if e != nil || out == nil || out.Outcome == nil || !out.Outcome.Authoritative || out.Outcome.CapabilityHash != constUtil.P6UFPartyPlayerNativeProof {
					t.Fatal("independent approved party route", e)
				}
				if e = validateP6UFPartyPlayerTerminal(input, out); e != nil {
					t.Fatal(e)
				}
			}
			for _, foreign := range []string{"", constUtil.P6UFPlayerNativeProof, constUtil.P6UFPartialPlayerNativeProof, constUtil.P6ActualPartyNativeProof, constUtil.P6UFGuardNativeProof} {
				if foreign != constUtil.P6UFPartyPlayerNativeProof && p6KnownUFPartyPlayerSettlementProof(input, foreign) {
					t.Fatal("borrowed unrelated native proof")
				}
			}
			after, _ := json.Marshal(input)
			if string(before) != string(after) {
				t.Fatal("qualification normalized current HP/AP or wrapper")
			}
		})
	}
}

func TestP6UFPartyPlayerRejectsUnsupportedInputs(t *testing.T) {
	s, input := p6UFPartyFixture(t, 4, 4)
	cases := map[string]func(*model.FrozenBattleInput){
		"all-dead": func(i *model.FrozenBattleInput) {
			for _, h := range i.Attacker.Formation {
				h.Inst.CurHp = proto.Float64(0)
			}
		},
		"negative-hp": func(i *model.FrozenBattleInput) { i.Attacker.Formation[0].Inst.CurHp = proto.Float64(-1) },
		"over-hp": func(i *model.FrozenBattleInput) {
			i.Attacker.Formation[1].Inst.CurHp = proto.Float64(i.Attacker.Formation[1].Prop.GetHp() + 1)
		},
		"nan-hp":         func(i *model.FrozenBattleInput) { i.Attacker.Formation[1].Inst.CurHp = proto.Float64(math.NaN()) },
		"ap":             func(i *model.FrozenBattleInput) { i.Defender.Inst.Anger = proto.Float64(301) },
		"duplicate-guid": func(i *model.FrozenBattleInput) { i.Attacker.Formation[1].Guid = i.Attacker.Formation[0].Guid },
		"duplicate-pose": func(i *model.FrozenBattleInput) { i.Attacker.Formation[1].Pose = i.Attacker.Formation[0].Pose },
		"five": func(i *model.FrozenBattleInput) {
			i.Attacker.Formation = append(i.Attacker.Formation, proto.Clone(i.Attacker.Formation[0]).(*pb.BattleHero))
		},
		"foreign-wrapper": func(i *model.FrozenBattleInput) { i.BattleType = 13 },
		"initiative":      func(i *model.FrozenBattleInput) { i.Attacker.Inst.FirstOrder = proto.Int32(0) },
		"factor":          func(i *model.FrozenBattleInput) { i.DefenderDamageFactor = proto.Float64(0) },
		"skill-order": func(i *model.FrozenBattleInput) {
			i.Attacker.Formation[0].SkillOrder = proto.String("foreign,skill,order")
		},
		"unknown-source": func(i *model.FrozenBattleInput) { i.Attacker.Formation[0].Id = proto.String("hero_unknown") },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			altered, err := cloneP6CalculationInput(input)
			if err != nil {
				t.Fatal(err)
			}
			change(altered)
			if _, err = s.ValidateP6UFPartyPlayerMechanisms(context.Background(), altered); err == nil {
				t.Fatal("unsupported source admitted")
			}
		})
	}
}

func TestP6UFPartyPlayerCompleteTerminalFrozenDeadPartners(t *testing.T) {
	s, input := p6UFPartyFixture(t, 4, 4)
	out, err := s.ComputeOriginalP6UFPlayerProtocolFinalHP(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if out.Outcome.Authoritative {
		t.Fatal("diagnostic computation acquired settlement authority")
	}
	if err = validateP6UFPartyPlayerTerminal(input, out); err != nil {
		t.Fatal(err)
	}
	for _, actor := range out.FinalActors {
		if actor.Pose == 1 && actor.CurrentHP != 0 {
			t.Fatal("initially dead original047 partner was resurrected", actor)
		}
	}
	cases := map[string]func(*model.CombatFinalHPOutcome){
		"missing-actor": func(o *model.CombatFinalHPOutcome) { o.FinalActors = o.FinalActors[1:] },
		"initial-sha":   func(o *model.CombatFinalHPOutcome) { o.FinalActors[0].InitialActorSHA256 = "foreign" },
		"guid":          func(o *model.CombatFinalHPOutcome) { o.FinalActors[0].GUID = "foreign" },
		"pose":          func(o *model.CombatFinalHPOutcome) { o.FinalActors[0].Pose++ },
		"max":           func(o *model.CombatFinalHPOutcome) { o.FinalActors[0].MaxHP++ },
		"whole-player": func(o *model.CombatFinalHPOutcome) {
			o.Outcome.Defender.Formation[0].Prop.Att = proto.Float64(o.Outcome.Defender.Formation[0].Prop.GetAtt() + 1)
		},
		"ap":           func(o *model.CombatFinalHPOutcome) { o.Outcome.Attacker.Inst.Anger = proto.Float64(301) },
		"original-ops": func(o *model.CombatFinalHPOutcome) { o.OriginalOperates.AttackerTotalLeftHp = proto.Float64(-1) },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			raw, _ := json.Marshal(out)
			var changed model.CombatFinalHPOutcome
			if err := json.Unmarshal(raw, &changed); err != nil {
				t.Fatal(err)
			}
			change(&changed)
			if validateP6UFPartyPlayerTerminal(input, &changed) == nil {
				t.Fatal("terminal drift accepted")
			}
		})
	}
}

func TestP6UFPartyPlayerProducerRetainsZeroSlotsAndRejectsForeignAuthority(t *testing.T) {
	_, input := p6UFPartyFixture(t, 4, 4)
	battle := input.Attacker
	frame := &pb.InstUFPlayer{Guid: proto.Int64(battle.GetPlayerId()), Camp: proto.Int32(0), State: proto.Int32(0), FightInfo: &pb.InstFightable{Guid: proto.Int64(battle.GetPlayerId()), Anger: proto.Float64(battle.Inst.GetAnger())}}
	for _, hero := range battle.Formation {
		frame.FightInfo.Index = append(frame.FightInfo.Index, hero.GetPose())
		frame.FightInfo.HpPos = append(frame.FightInfo.HpPos, hero.Inst.GetCurHp())
		frame.FightInfo.MaxHpPos = append(frame.FightInfo.MaxHpPos, hero.Prop.GetHp())
	}
	battleRaw, _ := proto.Marshal(battle)
	frameRaw, _ := proto.Marshal(frame)
	actor := &db.P13UnionFightFieldPlayer{PlayerID: battle.GetPlayerId(), Camp: 0, Battle: battleRaw, BattleHash: p6Digest(battleRaw), Frame: frameRaw, FrameHash: p6Digest(frameRaw)}
	before, _ := json.Marshal(input)
	current, err := p13UFPlayerBattleCurrentSource(actor, frame, battle, 1, true)
	if err != nil || !proto.Equal(current, battle) || current.Formation[0].Inst.GetCurHp() != 0 {
		t.Fatal("producer lost frozen dead partner/current AP", err)
	}
	if _, err = p13UFPlayerBattleCurrentSource(actor, frame, battle, 1, false); err == nil {
		t.Fatal("legacy producer accepted dead partner")
	}
	after, _ := json.Marshal(input)
	if string(before) != string(after) {
		t.Fatal("producer mutated original frozen party")
	}
	rules := p13UFPlayerBattle172Rules()
	rules.Policy.ID = constUtil.P13UFPlayerPartyPolicyID
	rules.Policy.Version = constUtil.P13UFPlayerPartyPolicyVersion
	rules.FullHPOnly = false
	rules.PartialCapabilityProof = ""
	for _, proof := range []string{"", constUtil.P6UFPlayerNativeProof, constUtil.P6UFPartialPlayerNativeProof, constUtil.P6ActualPartyNativeProof} {
		if proof == constUtil.P6UFPartyPlayerNativeProof && proof != "" {
			continue
		}
		rules.CapabilityProof = proof
		if p13UFPlayerBattleRulesValid(rules) {
			t.Fatal("new policy borrowed an old or missing certificate")
		}
		if _, err = p13UFPlayerBattleSourceForRules(actor, frame, battle, 1, rules); err == nil {
			t.Fatal("unpublished party authority admitted capture")
		}
	}
}
