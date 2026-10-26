package service

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	pb "hyjs-server/model/protocol"
)

func p6ReadActualTrainingPaths(t *testing.T) []p6ActualGrowthPath {
	t.Helper()
	file := "p5-training-highest-actors-20261014.json"
	raw, err := readTestAnalysis75("../../analysis/p6/" + file)
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Paths []struct {
			p6ActualGrowthPath
			Maximum []int32 `json:"trainingMaximum"`
			Draws   int     `json:"actualDraws"`
		} `json:"characterPaths"`
		TableSHA map[string]string `json:"originalTableSha256"`
		Draws    int               `json:"actualDraws"`
		Native   bool              `json:"dynamicCombatCertified"`
	}
	if err = json.Unmarshal(raw, &fixture); err != nil || len(fixture.Paths) != 76 || fixture.Draws != 27790 || fixture.Native {
		t.Fatal("actual training fixture drift", err)
	}
	for table, digest := range fixture.TableSHA {
		original, e := readTestAnalysis75("../../analysis/tables/Table" + table + ".tsv")
		if e != nil || p6Digest(original) != digest {
			t.Fatal("actual training original table drift", table, e)
		}
	}
	s := &GameService{}
	if err = s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	var paths []p6ActualGrowthPath
	seen := map[string]bool{}
	totalDraws := 0
	for _, row := range fixture.Paths {
		w, a := row.Warrior, row.BattleHero
		if w == nil || a == nil || seen[row.HeroID] || row.HeroID != w.GetId() || row.HeroID != a.GetId() || w.GetGuid() != a.GetGuid() || w.GetLevel() != 100 || a.GetLevel() != 100 || w.GetBreached().GetLevel() != 1 || a.GetBreachedLevel() != 1 || w.GetTalent() != 0 || len(w.GetTalentBranch()) != 0 || w.GetAwake().GetStage() != 0 || a.GetAwakeStage() != 0 || row.Draws < 1 {
			t.Fatal("actual training individual stage changed", row.HeroID)
		}
		var maximum []int32
		for _, original := range s.Tables["HeroTrainingMaxMin"] {
			if original["warId"] == row.HeroID && original["warLv"] == "100" {
				maximum = []int32{int32(tableNumber(original, "hpMax")), int32(tableNumber(original, "attMax")), int32(tableNumber(original, "pdefMax")), int32(tableNumber(original, "mdefMax"))}
				break
			}
		}
		actual := []int32{w.GetTraining().GetHp(), w.GetTraining().GetAtt(), w.GetTraining().GetPdef(), w.GetTraining().GetMdef()}
		if len(maximum) != 4 || !slices.Equal(maximum, row.Maximum) || !slices.Equal(maximum, actual) {
			t.Fatal("real training did not reach each original level100 maximum", row.HeroID)
		}
		seen[row.HeroID] = true
		totalDraws += row.Draws
		row.Fixture = file
		paths = append(paths, row.p6ActualGrowthPath)
	}
	if totalDraws != fixture.Draws {
		t.Fatal("actual training draw ledger mismatch", totalDraws)
	}
	return paths
}

func TestP6ActualOriginalTrainingMaximumAllActorsWholeReplay(t *testing.T) {
	if os.Getenv("HYJS_P6_ACTUAL_TRAINING_TEST") != "1" {
		t.Skip("explicit actual training stage matrix disabled")
	}
	p6ActualStageAllActorsWholeReplay(t, p6ReadActualTrainingPaths(t), "HYJS_P6_ACTUAL_TRAINING_OUTPUT", "actual individual four original training maxima/level100/breach1/talent0/awake0; complete branch/native/combined highest growth pending")
}

func p6ActualStageAllActorsWholeReplay(t *testing.T, paths []p6ActualGrowthPath, outputEnv, scope string) {
	t.Helper()
	originals := map[string]*pb.BattleHero{}
	for _, path := range paths {
		originals[path.HeroID] = path.BattleHero
	}
	s := p6OriginalTables(t)
	var mutex sync.Mutex
	var rows []map[string]any
	t.Cleanup(func() {
		if t.Failed() || len(rows) != len(paths)*2 {
			return
		}
		if output := os.Getenv(outputEnv); output != "" {
			slices.SortFunc(rows, func(a, b map[string]any) int { return strings.Compare(a["case"].(string), b["case"].(string)) })
			raw, e := json.MarshalIndent(map[string]any{"engineVersion": constUtil.P6EngineVersion, "cases": rows, "nativeCompared": false, "authoritative": false, "scope": scope}, "", "  ")
			if e != nil {
				t.Error(e)
				return
			}
			if e = os.MkdirAll(filepath.Dir(output), 0755); e != nil {
				t.Error(e)
				return
			}
			if e = os.WriteFile(output, append(raw, '\n'), 0644); e != nil {
				t.Error(e)
			}
		}
	})
	for _, path := range paths {
		for _, camp := range []int{0, 1} {
			label := path.Fixture + "/" + path.HeroID + "/camp" + strconv.Itoa(camp)
			t.Run(label, func(t *testing.T) {
				t.Parallel()
				input := p6ActualGrowthInput(t, s, path, camp, originals)
				before, _ := json.Marshal(input)
				out, e := s.ComputeFrozenBattle(t.Context(), input, nil)
				if e != nil {
					t.Fatal(label, e)
				}
				replay, e := s.ComputeFrozenBattle(t.Context(), input, out.Operates)
				if e != nil || out.Authoritative || out.Winner != replay.Winner || out.RandomDraws != replay.RandomDraws || !proto.Equal(out.Operates, replay.Operates) || !proto.Equal(out.Attacker, replay.Attacker) || !proto.Equal(out.Defender, replay.Defender) || !reflect.DeepEqual(out.Events, replay.Events) {
					t.Fatal("actual training entire replay drift", label, e)
				}
				after, _ := json.Marshal(input)
				if string(before) != string(after) {
					t.Fatal("actual training frozen panels changed")
				}
				var started []string
				for _, event := range out.Events {
					if event.Actor == fmt.Sprintf("%d_%s", camp, path.BattleHero.GetGuid()) && event.Kind == "skill_begin" && !slices.Contains(started, event.Skill) {
						started = append(started, event.Skill)
					}
				}
				if len(started) == 0 {
					t.Fatal("actual trained actor did not release an owned skill", label)
				}
				panel, _ := json.Marshal(path.BattleHero.Prop)
				mutex.Lock()
				rows = append(rows, map[string]any{"case": label, "heroId": path.HeroID, "camp": camp, "inputSHA256": p6Digest(before), "panelSHA256": p6Digest(panel), "warrior": path.Warrior, "battleHero": path.BattleHero, "skillsStarted": started, "eventCount": len(out.Events), "randomDraws": out.RandomDraws, "finalRound": out.FinalRound, "nativeAndAllConditionBranches": "pending"})
				mutex.Unlock()
			})
		}
	}
}
