package service

import (
	"encoding/json"
	"os"
	"testing"
)

func p6ReadActualBreachPaths(t *testing.T) []p6ActualGrowthPath {
	t.Helper()
	file := "p5-breach-highest-actors-20261014.json"
	raw, err := readTestAnalysis75("../../analysis/p6/" + file)
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Paths []struct {
			p6ActualGrowthPath
			Visited  int32   `json:"visitedLevels"`
			Requests int64   `json:"actualTenStepRequests"`
			Cost     int64   `json:"actualItem006Cost"`
			Percent  float64 `json:"terminalPropertyPercent"`
		} `json:"characterPaths"`
		Requests   int64             `json:"actualTenStepRequests"`
		TableSHA   map[string]string `json:"originalTableSha256"`
		Native     bool              `json:"dynamicCombatCertified"`
		Production bool              `json:"productionModified"`
	}
	if err = json.Unmarshal(raw, &fixture); err != nil || len(fixture.Paths) != 76 || fixture.Requests != 305896 || fixture.Native || fixture.Production {
		t.Fatal("actual breach25 fixture drift", err)
	}
	for table, digest := range fixture.TableSHA {
		original, e := readTestAnalysis75("../../analysis/tables/Table" + table + ".tsv")
		if e != nil || p6Digest(original) != digest {
			t.Fatal("actual breach original table drift", table, e)
		}
	}
	s := &GameService{}
	if err = s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	terminal := s.table("HeroBreached", "id", "25")
	if terminal == nil || tableNumber(terminal, "Exp") != -1 || s.table("HeroBreached", "id", "26") != nil {
		t.Fatal("original reachable terminal breach row changed")
	}
	for _, key := range []string{"AttAddPercent", "HpAddPercent", "PDefAddPercent", "MDefAddPercent"} {
		if tableNumber(terminal, key) != 0 {
			t.Fatal("original terminal empty property percent invented", key)
		}
	}
	var paths []p6ActualGrowthPath
	seen := map[string]bool{}
	var requests int64
	for _, row := range fixture.Paths {
		w, a := row.Warrior, row.BattleHero
		if w == nil || a == nil || seen[row.HeroID] || row.HeroID != w.GetId() || row.HeroID != a.GetId() || w.GetGuid() != a.GetGuid() || w.GetLevel() != 100 || a.GetLevel() != 100 || w.GetBreached().GetLevel() != 25 || a.GetBreachedLevel() != 25 || w.GetTalent() != 0 || len(w.GetTalentBranch()) != 0 || w.GetAwake().GetStage() != 0 || a.GetAwakeStage() != 0 || row.Visited != 25 || row.Requests < 1 || row.Cost < 1 || row.Percent != 0 {
			t.Fatal("real original terminal breach stage changed", row.HeroID)
		}
		training := w.GetTraining()
		if training.GetHp() != 0 || training.GetAtt() != 0 || training.GetPdef() != 0 || training.GetMdef() != 0 {
			t.Fatal("breach-only fixture gained unrecorded training", row.HeroID)
		}
		seen[row.HeroID] = true
		requests += row.Requests
		row.Fixture = file
		paths = append(paths, row.p6ActualGrowthPath)
	}
	if requests != fixture.Requests {
		t.Fatal("actual breach request ledger mismatch", requests)
	}
	return paths
}

func TestP6ActualOriginalBreach25AllActorsWholeReplay(t *testing.T) {
	if os.Getenv("HYJS_P6_ACTUAL_BREACH_TEST") != "1" {
		t.Skip("explicit actual breach25 stage matrix disabled")
	}
	p6ActualStageAllActorsWholeReplay(t, p6ReadActualBreachPaths(t), "HYJS_P6_ACTUAL_BREACH_OUTPUT", "actual original reachable breach25/level100/training0/talent0/awake0; terminal property percentages0 preserved, skill growth uses actual25; complete branch/native/combined highest growth pending")
}
