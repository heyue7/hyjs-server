package service

import (
	"encoding/json"
	"strconv"
	"testing"
)

func TestThreeOrangeFactorsMatchOrangeTargetAndDamageMechanics(t *testing.T) {
	s := &GameService{}
	if err := s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if s.table("Hero", "id", "hero_010")["quality"] != "20" {
		t.Fatal("reference must be orange")
	}
	before := map[string]string{}
	for _, row := range s.Tables["SkillCast"] {
		b, _ := json.Marshal(row)
		before[row["SkillCastId"]] = string(b)
	}
	if err := s.ApplyThreeOrangeSkillFactors(); err != nil {
		t.Fatal(err)
	}
	total := func(key string) float64 {
		v := float64(0)
		for _, row := range s.Tables["SkillCast"] {
			if row["SkillId"] == key {
				x, _ := strconv.ParseFloat(row["ExportFactor"], 64)
				v += x
			}
		}
		return v
	}
	changed := 0
	for _, row := range s.Tables["SkillCast"] {
		b, _ := json.Marshal(row)
		if string(b) == before[row["SkillCastId"]] {
			continue
		}
		changed++
		var old map[string]string
		json.Unmarshal([]byte(before[row["SkillCastId"]]), &old)
		old["ExportFactor"] = row["ExportFactor"]
		b, _ = json.Marshal(old)
		fresh, _ := json.Marshal(row)
		if string(b) != string(fresh) {
			t.Fatal("unrelated cast field changed")
		}
		if row["SelectStrategy"] != "3" || row["ExportType"] != "1" {
			t.Fatal("front single physical mechanism changed")
		}
	}
	if changed != 23 {
		t.Fatal("expected 23 changed and 7 preserved one-percent lead ticks", changed)
	}
	for _, hero := range []string{"069", "070", "073"} {
		for _, slot := range []string{"1", "2", "3"} {
			donor := "010_1"
			if slot == "3" {
				donor = "010_3"
			}
			if total(hero+"_"+slot) != total(donor) {
				t.Fatal("orange total mismatch", hero, slot)
			}
		}
		if s.table("Skill", "SkillId", hero+"_3")["LevelGrowFactor"] != s.table("Skill", "SkillId", "010_3")["LevelGrowFactor"] {
			t.Fatal("ultimate growth differs from orange reference")
		}
	}
	if err := s.ApplyThreeOrangeSkillFactors(); err != nil {
		t.Fatal("repeat startup projection", err)
	}
}
