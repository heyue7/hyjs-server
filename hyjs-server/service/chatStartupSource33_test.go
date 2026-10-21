package service

import (
	"encoding/json"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestChat33StartupSkillCorrectionsPreserveUnrelatedDuel(t *testing.T) {
	s := p6OriginalTables(t)
	input := p6Input(t, s, p6Actor("hero_047", "left", "047_1,047_2,047_3", 3, 1000), p6Actor("hero_047", "right", "047_1,047_2,047_3", 3, 1000), 40)
	input.BattleType = 11
	input, err := s.FreezeBattleInput(*input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ComputeP11GenericMechanismSettlement(t.Context(), input); err != nil {
		t.Fatal("original source", err)
	}
	if err = s.ApplyP15GreenHeroCompatibility(); err != nil {
		t.Fatal(err)
	}
	if err = s.ApplyThreeOrangeSkillFactors(); err != nil {
		t.Fatal(err)
	}
	startup, err := s.FreezeBattleInput(model.FrozenBattleInput{Attacker: input.Attacker, Defender: input.Defender, BattleType: 11, Seed: input.Seed, MaxRound: 40})
	if err != nil || startup.ConfigurationHash != input.ConfigurationHash {
		t.Fatal("unrelated startup catalog invalidated reviewed duel", err)
	}
	if _, err = s.ComputeP11GenericMechanismSettlement(t.Context(), startup); err != nil {
		t.Fatal("startup source", err)
	}
	cold, err := s.FreezeBattleInput(*startup)
	if err != nil || cold.ConfigurationHash != startup.ConfigurationHash {
		t.Fatal("cold source changed", err)
	}
	for _, spec := range threeOrangeSkillFactorSpecs() {
		if s.table("SkillCast", "SkillCastId", spec.id)["ExportFactor"] != spec.factor {
			t.Fatal("shared authored source mutated", spec.id)
		}
	}
	// An active authored actor retains its actual corrected skills and the old
	// capability gate; this fix cannot certify a new actor or erase its damage.
	active := *startup
	active.ConfigurationHash = ""
	active.Attacker = proto.Clone(startup.Attacker).(*pb.BattlePlayer)
	active.Attacker.Formation[0].Id = proto.String("hero_069")
	active.Attacker.Formation[0].SkillOrder = proto.String("069_1,069_2,069_3")
	authored, err := s.FreezeBattleInput(active)
	if err != nil {
		t.Fatal(err)
	}
	if authored.ConfigurationHash == constUtil.P6LinearPhysicalTables || (&GameService{Tables: authored.Tables}).table("SkillCast", "SkillCastId", "1560")["ExportFactor"] != "33" {
		t.Fatal("active authored actor lost real factors")
	}
	// Keep an unrelated edit visible to the full source validator.
	s.table("Buff", "Id", s.Tables["Buff"][0]["Id"])["Factor"] = "unreviewed33"
	tampered, err := s.FreezeBattleInput(model.FrozenBattleInput{Attacker: input.Attacker, Defender: input.Defender, BattleType: 11, Seed: input.Seed, MaxRound: 40})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p11GenericMechanismProof(tampered); err == nil {
		t.Fatal("unrelated source edit accepted")
	}
	before, _ := json.Marshal(input)
	after, _ := json.Marshal(startup)
	if string(before) != string(after) {
		t.Fatal("unrelated frozen battle changed")
	}
}
