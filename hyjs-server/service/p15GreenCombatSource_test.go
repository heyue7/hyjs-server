package service

import (
	"encoding/json"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
)

func TestP15GreenCombatStrictSourceViewsAndLegacyFreeze(t *testing.T) {
	s := p6OriginalTables(t)
	original := p6GenericInput(t, s, p6Actor("hero_062", "source-062", "062_2,062_1,062_3", 3, 6000), 40)
	original.BattleType = 2
	if err := s.ApplyP15GreenHeroCompatibility(); err != nil {
		t.Fatal(err)
	}
	input, err := s.FreezeBattleInput(model.FrozenBattleInput{Attacker: original.Attacker, Defender: original.Defender, BattleType: 2, MaxRound: 40, Seed: 42})
	if err != nil || input.ConfigurationHash != constUtil.P6LinearPhysicalTables {
		t.Fatal("unused compatibility polluted source", err)
	}
	if s.table("Hero", "id", "hero_056")["SkillId1"] != "1068_1" {
		t.Fatal("runtime compatible source mutated")
	}
	before, _ := json.Marshal(input)
	again, err := s.FreezeBattleInput(*input)
	if err != nil || again.ConfigurationHash != input.ConfigurationHash {
		t.Fatal("original frozen canonical view", err)
	}
	after, _ := json.Marshal(input)
	if string(before) != string(after) {
		t.Fatal("frozen original mutated")
	}
	// Preserve an older frozen compatible view with no green actor, rather than
	// deriving its table identity from the newly normalized runtime projection.
	historical := *input
	historical.Tables = cloneP6OriginalTestTables(input.Tables)
	historical.Tables["Hero"] = cloneP6OriginalTestTables(s.Tables)["Hero"]
	raw, _ := json.Marshal(historical.Tables)
	historical.ConfigurationHash = p6Digest(raw)
	if _, err = s.ValidateP6GenericMechanisms(t.Context(), &historical); err != nil {
		t.Fatal("historical compatible view", err)
	}
	reFrozen, err := s.FreezeBattleInput(historical)
	if err != nil || reFrozen.ConfigurationHash != historical.ConfigurationHash {
		t.Fatal("historical compatible hash rewritten", err)
	}
	// Wanted8's additional original table survives; only the six irrelevant
	// catalog rows normalize, and the mode-specific final hash stays distinct.
	wanted, err := s.FreezeBattleInput(model.FrozenBattleInput{Attacker: input.Attacker, Defender: input.Defender, BattleType: 8, MaxRound: 40, Seed: 42})
	if err != nil {
		t.Fatal(err)
	}
	if wanted.ConfigurationHash == constUtil.P6LinearPhysicalTables || len(wanted.Tables["WantedEnemy"]) == 0 || wanted.Tables["Hero"][0]["id"] != input.Tables["Hero"][0]["id"] {
		t.Fatal("Wanted8 source collapsed")
	}
	var restored bool
	for _, row := range wanted.Tables["Hero"] {
		if row["id"] == "hero_056" {
			restored = row["SkillId1"] == ""
		}
	}
	if !restored {
		t.Fatal("Wanted8 unused green row remained compatible")
	}
	if _, err = p11GenericMechanismProof(wanted); err == nil {
		t.Fatal("cross-mode source admitted")
	}
	for _, column := range []string{"SkillId1", "BeFoundBone", "RushFlyTime", "ImposState", "attBase"} {
		t.Run("reject-mutated-"+column, func(t *testing.T) {
			bad := historical
			bad.Tables = cloneP6OriginalTestTables(historical.Tables)
			for _, row := range bad.Tables["Hero"] {
				if row["id"] == "hero_056" {
					row[column] = "unapproved-source-edit"
				}
			}
			encoded, _ := json.Marshal(bad.Tables)
			bad.ConfigurationHash = p6Digest(encoded)
			if _, err := p11GenericMechanismProof(&bad); err == nil {
				t.Fatal("unapproved green edit admitted")
			}
		})
	}
	bad := historical
	bad.Tables = cloneP6OriginalTestTables(historical.Tables)
	bad.Tables["Buff"][0]["Args1"] = "123456"
	encoded, _ := json.Marshal(bad.Tables)
	bad.ConfigurationHash = p6Digest(encoded)
	if _, err := p11GenericMechanismProof(&bad); err == nil {
		t.Fatal("other source table edit admitted")
	}
}

func TestP15GreenActiveArenaKeepsApprovedOriginalAttack(t *testing.T) {
	s := p6OriginalTables(t)
	if err := s.ApplyP15GreenHeroCompatibility(); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"hero_056", "hero_057", "hero_058", "hero_059", "hero_060", "hero_061"} {
		t.Run(id, func(t *testing.T) {
			row := s.table("Hero", "id", id)
			actor := p6Actor(id, "approved-green", row["SkillId1"]+",,", 3, 4000)
			source := p6GenericInput(t, s, actor, 40)
			source.BattleType = 2
			input, err := s.FreezeBattleInput(*source)
			if err != nil || input.ConfigurationHash == constUtil.P6LinearPhysicalTables || !p15GreenCombatSourceCompatible(input) {
				t.Fatal("green active source lost", err)
			}
			out, err := s.ComputeP11GenericMechanismSettlement(t.Context(), input)
			if err != nil {
				t.Fatal("approved green original attack", err)
			}
			if !out.Outcome.Authoritative || out.FinalActors[0].HeroID != id || input.Attacker.Formation[0].GetSkillOrder() != row["SkillId1"]+",," {
				t.Fatal("green actor rewritten")
			}
			raw, _ := proto.Marshal(input.Attacker)
			if len(raw) == 0 {
				t.Fatal("empty source")
			}
		})
	}
}
