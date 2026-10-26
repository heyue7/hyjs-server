package service

import (
	"encoding/json"
	pb "hyjs-server/model/protocol"
	"os"
	"slices"
	"testing"
)

func p6ReadActualCombinedPaths(t *testing.T) []p6ActualGrowthPath {
	t.Helper()
	file := "p5-combined-red-awake-highest-actors-20261014.json"
	count, scope := 19, "red-awake"
	if os.Getenv("HYJS_P6_ACTUAL_COMBINED_SCOPE") == "all" {
		file = "p5-combined-all76-highest-actors-20261014.json"
		count, scope = 76, "all76 original configured identities, actual consecutive combined highest"
	}
	raw, err := readTestAnalysis75("../../analysis/p6/" + file)
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Paths []struct {
			p6ActualGrowthPath
			Combined          bool              `json:"actualContinuouslyCombinedPaidGrowth"`
			TrainingDraws     int64             `json:"actualTrainingDraws"`
			BreachRequests    int64             `json:"actualTenStepBreachRequests"`
			SpecialtyUpgrades int64             `json:"actualSpecialtyUpgrades"`
			Maximum           []int32           `json:"trainingMaximum"`
			Equipment         []*pb.Equipment   `json:"equipment"`
			Treasures         []*pb.Treasure    `json:"treasures"`
			Formation         *pb.FormationItem `json:"formation"`
		} `json:"characterPaths"`
		TableSHA       map[string]string `json:"originalTableSha256"`
		Native         bool              `json:"dynamicCombatCertified"`
		Production     bool              `json:"productionModified"`
		Scope          string            `json:"scope"`
		BreachTotal    int64             `json:"actualTenStepBreachRequests"`
		TrainingTotal  int64             `json:"actualTrainingDraws"`
		SpecialtyTotal int64             `json:"actualSpecialtyUpgrades"`
	}
	if err = json.Unmarshal(raw, &fixture); err != nil || len(fixture.Paths) != count || fixture.Native || fixture.Production || fixture.Scope != scope {
		t.Fatal("real consecutive highest fixture drift", err)
	}
	for table, digest := range fixture.TableSHA {
		original, e := readTestAnalysis75("../../analysis/tables/Table" + table + ".tsv")
		if e != nil || p6Digest(original) != digest {
			t.Fatal("combined original table drift", table, e)
		}
	}
	original := p6ReadActualGrowthPaths(t)
	highest := map[string]p6ActualGrowthPath{}
	awake := map[string]bool{}
	for _, p := range original[:76] {
		highest[p.HeroID] = p
	}
	for _, p := range original[76:] {
		awake[p.HeroID] = true
	}
	var paths []p6ActualGrowthPath
	s := &GameService{}
	if err = s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	awakeCount := 0
	var breachTotal, trainingTotal, specialtyTotal int64
	for _, row := range fixture.Paths {
		w, a := row.Warrior, row.BattleHero
		if w == nil || a == nil || seen[row.HeroID] || w.GetId() != row.HeroID || a.GetId() != row.HeroID || w.GetGuid() != a.GetGuid() || w.GetLevel() != 100 || a.GetLevel() != 100 || w.GetBreached().GetLevel() != 25 || a.GetBreachedLevel() != 25 || !row.Combined || row.TrainingDraws < 1 || row.BreachRequests < 1 {
			t.Fatal("real consecutive combined stage mismatch", row.HeroID)
		}
		h := highest[row.HeroID]
		if w.GetTalent() != h.HighestTrunk || !slices.Equal(w.GetTalentBranch(), h.BranchIDs) || !slices.Equal(row.NodeIDs, h.NodeIDs) {
			t.Fatal("combined highest actual talent path mismatch", row.HeroID)
		}
		tr := w.GetTraining()
		if len(row.Maximum) != 4 || !slices.Equal(row.Maximum, []int32{tr.GetHp(), tr.GetAtt(), tr.GetPdef(), tr.GetMdef()}) {
			t.Fatal("combined highest actual training mismatch", row.HeroID)
		}
		var originalMaximum []int32
		for _, v := range s.Tables["HeroTrainingMaxMin"] {
			if v["warId"] == row.HeroID && v["warLv"] == "100" {
				originalMaximum = []int32{int32(tableNumber(v, "hpMax")), int32(tableNumber(v, "attMax")), int32(tableNumber(v, "pdefMax")), int32(tableNumber(v, "mdefMax"))}
				break
			}
		}
		if !slices.Equal(originalMaximum, row.Maximum) {
			t.Fatal("combined original training cap mismatch", row.HeroID)
		}
		if awake[row.HeroID] {
			if w.GetAwake().GetStage() != 2 || w.GetAwake().GetFuseLevel() != 100 || a.GetAwakeStage() != 2 {
				t.Fatal("combined enabled awakening mismatch", row.HeroID)
			}
			awakeCount++
		} else if w.GetAwake().GetStage() != 0 || a.GetAwakeStage() != 0 {
			t.Fatal("combined unenabled awakening invented", row.HeroID)
		}
		specialty := map[int32]bool{}
		for _, v := range w.GetSkillRefine() {
			if v.GetRoleGuid() != w.GetGuid() || v.GetLevel() != 100 || v.GetStrengthLevel() != 10 || v.GetType() < 1 || v.GetType() > 5 || specialty[v.GetType()] {
				t.Fatal("combined highest specialty mismatch", row.HeroID)
			}
			specialty[v.GetType()] = true
		}
		hero := s.table("Hero", "id", row.HeroID)
		if hero["quality"] == "20" || hero["quality"] == "23" {
			if len(specialty) != 5 || row.SpecialtyUpgrades != 540 {
				t.Fatal("combined original enabled specialty mismatch", row.HeroID)
			}
		} else if len(specialty) != 0 || row.SpecialtyUpgrades != 0 {
			t.Fatal("combined original disabled specialty invented", row.HeroID)
		}
		if len(row.Equipment) != 4 || len(row.Treasures) != 2 {
			t.Fatal("combined actual equipped count", row.HeroID)
		}
		for i, v := range row.Equipment {
			if v.GetEquipId() != []string{"equip_037", "equip_038", "equip_039", "equip_040"}[i] || v.GetLevel() != 260 || v.GetRefineLv() != 80 || v.GetStar() != 5 || v.GetForgeLevel() != 15 {
				t.Fatal("combined highest red equipment mismatch", row.HeroID)
			}
		}
		for i, v := range row.Treasures {
			if v.GetTreasureId() != []string{"trea_015", "trea_018"}[i] || v.GetStrengthLevel() != 100 || v.GetRefineLevel() != 30 {
				t.Fatal("combined highest treasure mismatch", row.HeroID)
			}
		}
		if len(row.Formation.GetThings()) != 7 || row.Formation.GetThings()[0].GetGuid() != w.GetGuid() {
			t.Fatal("combined real wearing formation mismatch", row.HeroID)
		}
		for i, v := range row.Equipment {
			if row.Formation.GetThings()[i+1].GetGuid() != v.GetGuid() {
				t.Fatal("combined worn equipment GUID mismatch", row.HeroID)
			}
		}
		for i, v := range row.Treasures {
			if row.Formation.GetThings()[i+5].GetGuid() != v.GetGuid() {
				t.Fatal("combined worn treasure GUID mismatch", row.HeroID)
			}
		}
		seen[row.HeroID] = true
		breachTotal += row.BreachRequests
		trainingTotal += row.TrainingDraws
		specialtyTotal += row.SpecialtyUpgrades
		row.Fixture = file
		paths = append(paths, row.p6ActualGrowthPath)
	}
	if awakeCount != 15 {
		t.Fatal("combined original enabled awakening denominator changed", awakeCount)
	}
	if count == 76 && (fixture.BreachTotal != 305914 || fixture.TrainingTotal != 27880 || fixture.SpecialtyTotal != 26460 || breachTotal != fixture.BreachTotal || trainingTotal != fixture.TrainingTotal || specialtyTotal != fixture.SpecialtyTotal) {
		t.Fatal("combined actual paid ledger mismatch", breachTotal, trainingTotal, specialtyTotal)
	}
	return paths
}

func TestP6ActualOriginalCombinedHighestRedAwakeWholeReplay(t *testing.T) {
	if os.Getenv("HYJS_P6_ACTUAL_COMBINED_TEST") != "1" {
		t.Skip("explicit real consecutive combined highest stage disabled")
	}
	p6ActualStageAllActorsWholeReplay(t, p6ReadActualCombinedPaths(t), "HYJS_P6_ACTUAL_COMBINED_OUTPUT", "all selected actors real consecutive original highest growth and worn four red gear/two treasures; whole native/all reachable branches pending")
}
