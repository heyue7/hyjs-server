package service

import (
	"context"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
)

// Prospective source fixtures exercise scope rejection only. No test supplies
// a nonempty native certificate or upgrades a diagnostic outcome's authority.
func TestP6UFGuardMirrorProofIndependentClosedRoutes(t *testing.T) {
	if constUtil.P6UFGuardNativeProof != "" {
		t.Fatal("legacy diagnostic scope unexpectedly opened")
	}
	for _, rule := range []string{"", constUtil.P13UFGuardLocalRule} {
		if _, err := p6UFGuardProofForRule(rule); err == nil || err.Error() != constUtil.P13UFGuardProofMissing {
			t.Fatal("old certificate route opened", rule, err)
		}
	}
	if _, err := p6UFGuardProofForRule("unknown"); err == nil {
		t.Fatal("unknown rule selected a proof")
	}
	for _, rule := range []string{constUtil.P13UFGuardLocalRule, constUtil.P13UFGuardMirrorRule, "unknown"} {
		for _, proof := range []string{"", "foreign-native-domain", constUtil.P6UFPlayerNativeProof} {
			rules := &p13UFGuardBattleRules{LocalRule: rule, Proof: proof}
			if p13UFGuardRulesProof(rules) == nil {
				t.Fatal("foreign or empty proof admitted", rule)
			}
			if validateP6UFGuardTerminalForRule(nil, nil, rules) == nil {
				t.Fatal("rule forgery admitted terminal", rule)
			}
		}
	}
}

func TestP6UFGuardMirrorProofFullTemplateAndUnapprovedCompute(t *testing.T) {
	for _, seed := range p6GuardMechanismFixtures(t) {
		t.Run(seed.Offer.GuardID, func(t *testing.T) {
			input := p6GuardMechanismClone(t, seed).Input
			engine := &GameService{Tables: input.Tables, ServerID: int64(input.Monster.GetServerId())}
			expected, err := p6UFGuardMirrorTemplate(input)
			if err != nil {
				t.Fatal(err)
			}
			expected.MonsterId = input.Monster.MonsterId
			expected.Formation[0].Guid = input.Monster.Formation[0].Guid
			input.Monster = expected
			if err = validateP6UFGuardMirrorScope(input); err != nil {
				t.Fatal("complete prospective mirror source", err)
			}
			if constUtil.P6UFGuardMirrorNativeProof == "" {
				if _, err = engine.ComputeP6UFGuardMirrorSettlement(context.Background(), input); err == nil || err.Error() != constUtil.P13UFGuardProofMissing {
					t.Fatal("unapproved mirror compute opened", err)
				}
				if err = validateP6UFGuardMirrorTerminal(input, &model.CombatFinalHPOutcome{}); err == nil {
					t.Fatal("unapproved mirror terminal opened")
				}
			} else {
				out, e := engine.ComputeP6UFGuardMirrorSettlement(context.Background(), input)
				if e != nil || out == nil || out.Outcome == nil || !out.Outcome.Authoritative || out.Outcome.CapabilityHash != constUtil.P6UFGuardMirrorNativeProof {
					t.Fatal("independent approved mirror route", e)
				}
				if e = validateP6UFGuardMirrorTerminal(input, out); e != nil {
					t.Fatal(e)
				}
			}
			if _, err = engine.ComputeP6UFGuardSettlement(context.Background(), input); err == nil {
				t.Fatal("mirror borrowed legacy certificate")
			}
			mutations := map[string]func(*model.FrozenBattleInput){
				"attack": func(v *model.FrozenBattleInput) {
					h := v.Monster.Formation[0]
					h.Prop.Att = proto.Float64(h.Prop.GetAtt() + 1)
				},
				"defense": func(v *model.FrozenBattleInput) {
					h := v.Monster.Formation[0]
					h.Prop.Pdef = proto.Float64(h.Prop.GetPdef() + 1)
				},
				"level":  func(v *model.FrozenBattleInput) { v.Monster.Formation[0].Level = proto.Int32(2) },
				"pose":   func(v *model.FrozenBattleInput) { v.Monster.Formation[0].Pose = proto.Int32(7) },
				"skills": func(v *model.FrozenBattleInput) { v.Monster.Formation[0].SkillOrder = proto.String("injected") },
				"blood":  func(v *model.FrozenBattleInput) { v.Monster.Formation[0].BloodNumber = proto.Int32(2) },
				"wounded-npc": func(v *model.FrozenBattleInput) {
					h := v.Monster.Formation[0]
					h.Inst.CurHp = proto.Float64(h.Prop.GetHp() - 1)
				},
				"npc-ap": func(v *model.FrozenBattleInput) { v.Monster.Inst.Anger = proto.Float64(1) },
				"other-template": func(v *model.FrozenBattleInput) {
					if strings.HasPrefix(v.Monster.GetTaskId(), "baseCamp") {
						v.Monster.TaskId = proto.String("tower_guard_001")
					} else {
						v.Monster.TaskId = proto.String("baseCamp_guard_001")
					}
				},
				"table-source": func(v *model.FrozenBattleInput) { v.Tables["Hero"][0]["hp"] = "999999" },
			}
			for name, mutate := range mutations {
				t.Run(name, func(t *testing.T) {
					changed := p6GuardMechanismClone(t, p6GuardMechanismFixture{Input: input}).Input
					mutate(changed)
					if validateP6UFGuardMirrorScope(changed) == nil {
						t.Fatal("incomplete or changed NPC source admitted")
					}
				})
			}
		})
	}
}
