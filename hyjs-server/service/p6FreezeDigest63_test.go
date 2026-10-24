package service

import (
	"encoding/json"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	"testing"
)

func TestP6FreezeDigest63NormalizationAndFrozenIdentity(t *testing.T) {
	for _, mode := range []string{"original", "orange", "green", "both", "both-frozen", "unknown-edit"} {
		t.Run(mode, func(t *testing.T) {
			s := p6OriginalTables(t)
			original := p6GenericInput(t, s, p6Actor("hero_062", "digest63", "062_2,062_1,062_3", 3, 6000), 40)
			original.BattleType = 2
			if mode == "orange" || mode == "both" || mode == "both-frozen" || mode == "unknown-edit" {
				if err := s.ApplyThreeOrangeSkillFactors(); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "green" || mode == "both" || mode == "both-frozen" || mode == "unknown-edit" {
				if err := s.ApplyP15GreenHeroCompatibility(); err != nil {
					t.Fatal(err)
				}
			}
			expected := *original
			expected.Tables = cloneP6OriginalTestTables(original.Tables)
			runtimeTables := cloneP6OriginalTestTables(s.Tables)
			for name := range expected.Tables {
				expected.Tables[name] = runtimeTables[name]
			}
			if mode == "unknown-edit" {
				expected.Tables["Buff"][0]["Args1"] = "123456"
				s.Tables["Buff"][0]["Args1"] = "123456"
			}
			prior := ""
			if mode == "both-frozen" {
				raw, _ := json.Marshal(expected.Tables)
				prior = p6Digest(raw)
			}
			expected.ConfigurationHash = prior
			// External strict wrappers retain the old normalization order and behavior.
			normalizeUnusedThreeOrangeCombatRows(&expected, prior)
			p15NormalizeUnusedGreenCombatRows(&expected, prior)
			want, _ := json.Marshal(expected.Tables)
			request := *original
			request.ConfigurationHash = prior
			actual, err := s.FreezeBattleInput(request)
			if err != nil {
				t.Fatal(err)
			}
			got, _ := json.Marshal(actual.Tables)
			if string(want) != string(got) || actual.ConfigurationHash != p6Digest(want) {
				t.Fatal("normalization/order/hash changed")
			}
			if mode == "unknown-edit" {
				if actual.ConfigurationHash == constUtil.P6LinearPhysicalTables {
					t.Fatal("unknown source edit hidden")
				}
				if _, err = s.ValidateP6GenericMechanisms(t.Context(), original); err == nil {
					t.Fatal("different canonical source admitted")
				}
			} else if mode == "both-frozen" {
				if actual.ConfigurationHash != prior {
					t.Fatal("accepted historical source rewritten")
				}
			} else if actual.ConfigurationHash != constUtil.P6LinearPhysicalTables {
				t.Fatal("unused approved rows failed normalization")
			}
		})
	}
}

func TestP6FreezeDigest63BothAssignmentsRequireNewHash(t *testing.T) {
	s := p6OriginalTables(t)
	input := p6GenericInput(t, s, p6Actor("hero_062", "digest63", "062_2,062_1,062_3", 3, 6000), 40)
	if err := s.ApplyThreeOrangeSkillFactors(); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyP15GreenHeroCompatibility(); err != nil {
		t.Fatal(err)
	}
	view := model.FrozenBattleInput{Attacker: input.Attacker, Defender: input.Defender, Tables: cloneP6OriginalTestTables(input.Tables)}
	runtimeTables := cloneP6OriginalTestTables(s.Tables)
	for name := range view.Tables {
		view.Tables[name] = runtimeTables[name]
	}
	digest := func() string { raw, _ := json.Marshal(view.Tables); return p6Digest(raw) }
	before := digest()
	if !normalizeUnusedThreeOrangeCombatRowsWithHash(&view, "", before) {
		t.Fatal("orange assignment unreported")
	}
	orange := digest()
	if orange == before {
		t.Fatal("orange mutation not observed")
	}
	if !p15NormalizeUnusedGreenCombatRowsWithHash(&view, "", orange) {
		t.Fatal("green assignment unreported")
	}
	green := digest()
	if green == orange || green != constUtil.P6LinearPhysicalTables {
		t.Fatal("second mutation not independently digested")
	}
	if normalizeUnusedThreeOrangeCombatRowsWithHash(&view, green, green) || p15NormalizeUnusedGreenCombatRowsWithHash(&view, green, green) {
		t.Fatal("matching frozen identity changed")
	}
}
