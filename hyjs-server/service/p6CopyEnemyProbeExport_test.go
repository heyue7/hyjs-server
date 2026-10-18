package service

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Export exact original task groups as explicitly named table mechanism probes.
// Enemy rows, poses, growth properties and skills never change. A source-bound
// real cold P5 attacker and explicit diagnostic damage0/1 allow NPC branches to
// run; this is not an original paid chapter or private-server parameter proof.
func TestP6ExportOriginalCopyEnemyMechanismProbes(t *testing.T) {
	folder := os.Getenv("HYJS_P6_COPY_ENEMY_PROBE_OUTPUT")
	if folder == "" {
		t.Skip("explicit original table probe export disabled")
	}
	analysisRoot, err := filepath.EvalSymlinks("../../analysis/p6")
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Dir(filepath.Dir(analysisRoot))
	absolute, err := filepath.Abs(folder)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(absolute, filepath.Join(root, "analysis/p6")+string(os.PathSeparator)) {
		t.Fatal("keep probe source under analysis/p6")
	}
	if _, err = os.Stat(filepath.Join(absolute, "manifest.json")); err == nil {
		t.Fatal("preserve existing immutable probe source")
	}
	if err = os.MkdirAll(absolute, 0755); err != nil {
		t.Fatal(err)
	}
	s := p6OriginalTables(t)
	sourcePath := filepath.Join(root, "analysis/p6/native-next-real-skills-inputs-v95/000.pb")
	raw, err := os.ReadFile(sourcePath)
	if err != nil || p6Digest(raw) != "398c6819d7b8d5d1b4f06287610fdd2d34b8e81e791b3b35c0f61c48330f257f" {
		t.Fatal("real cold actor source SHA", err)
	}
	playerSource := &pb.SCFightPlayback{}
	if err = proto.Unmarshal(raw, playerSource); err != nil {
		t.Fatal(err)
	}
	remainingRaw, err := os.ReadFile(filepath.Join(root, "analysis/p6/native-next-real-skills-inputs-v97/manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var coverage struct {
		Remaining []string `json:"allRemainingOriginalSkillIDs"`
	}
	if err = json.Unmarshal(remainingRaw, &coverage); err != nil {
		t.Fatal(err)
	}
	remaining := map[string]bool{}
	for _, id := range coverage.Remaining {
		remaining[id] = true
	}
	type candidate struct {
		id     string
		rows   []map[string]string
		skills map[string]bool
		level  float64
	}
	groups := map[string]*candidate{}
	for _, row := range s.Tables["CopyEnemy"] {
		if row["GroupIndex"] != "0" {
			t.Fatal("original source grouping changed; preserve waves explicitly")
		}
		id := row["TaskId"]
		c := groups[id]
		if c == nil {
			c = &candidate{id: id, skills: map[string]bool{}}
			groups[id] = c
		}
		c.rows = append(c.rows, row)
		c.level = max(c.level, tableNumber(row, "Level"))
		for _, key := range []string{"SkillId1", "SkillId2", "UltSkillId"} {
			if id := row[key]; id != "" {
				c.skills[id] = true
			}
		}
	}
	candidates := []*candidate{}
	for _, c := range groups {
		candidates = append(candidates, c)
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].id < candidates[j].id })
	cases := []map[string]any{}
	failures := []map[string]string{}
	predicted := map[string]bool{}
	for len(cases) < 14 {
		selected := -1
		score := 0
		for i, c := range candidates {
			gain := 0
			for id := range c.skills {
				if remaining[id] && !predicted[id] {
					gain++
				}
			}
			if gain > score || gain == score && gain > 0 && selected >= 0 && c.level < candidates[selected].level {
				selected, score = i, gain
			}
		}
		if selected < 0 || score == 0 {
			break
		}
		c := candidates[selected]
		candidates = append(candidates[:selected], candidates[selected+1:]...)
		monster := &pb.BattleMonster{Inst: &pb.BattleMonsterInst{}}
		enemyIDs := []string{}
		for _, row := range c.rows {
			actor, e := s.actor(row, "monster-"+row["CopyEnemyId"], int32(tableNumber(row, "Location")), true)
			if e != nil {
				t.Fatal(e)
			}
			monster.Formation = append(monster.Formation, actor)
			enemyIDs = append(enemyIDs, row["CopyEnemyId"])
		}
		input, freezeErr := s.FreezeBattleInput(model.FrozenBattleInput{Attacker: playerSource.Attacker, Monster: monster, Seed: playerSource.GetRandSeed(), MaxRound: 40, BattleType: 1, AttackerDamageFactor: proto.Float64(0), DefenderDamageFactor: proto.Float64(1), AppliedProperties: map[string][]string{"probeScope": {"original CopyEnemy fullTaskGroup0 + real cold P5 attacker + explicit diagnostic factors0/1; not naturalpaid or private-source"}}})
		if freezeErr != nil {
			failures = append(failures, map[string]string{"task": c.id, "error": freezeErr.Error()})
			continue
		}
		computed, computeErr := s.ComputeFrozenBattle(t.Context(), input, nil)
		if computeErr != nil {
			failures = append(failures, map[string]string{"task": c.id, "error": computeErr.Error()})
			continue
		}
		started := map[string]bool{}
		for _, event := range computed.Events {
			if event.Kind == "skill_begin" && strings.HasPrefix(event.Actor, "1_") {
				started[event.Skill] = true
			}
		}
		gained := []string{}
		for id := range started {
			if remaining[id] && !predicted[id] {
				gained = append(gained, id)
			}
		}
		sort.Strings(gained)
		if len(gained) == 0 {
			continue
		}
		for _, id := range gained {
			predicted[id] = true
		}
		name := fmt.Sprintf("%03d", len(cases))
		wire := &pb.SCFightPlayback{Fightid: proto.String("original-copy-table-probe-task-" + c.id), RandSeed: proto.Int64(input.Seed), BattleType: proto.Int32(1), Attacker: proto.Clone(input.Attacker).(*pb.BattlePlayer), Defender2: proto.Clone(monster).(*pb.BattleMonster), AttackerFactor: proto.Float64(0), DefenderFactor: proto.Float64(1), MaxRound: proto.Int32(40)}
		encoded, e := proto.MarshalOptions{Deterministic: true}.Marshal(wire)
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(absolute, name+".pb"), encoded, 0644); e != nil {
			t.Fatal(e)
		}
		out, e := json.Marshal(computed)
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(absolute, name+".go.json"), out, 0644); e != nil {
			t.Fatal(e)
		}
		cases = append(cases, map[string]any{"case": "original-copy-table-task-" + c.id + "/explicit-factors0-1", "wire": name + ".pb", "wireSHA256": p6Digest(encoded), "seed": input.Seed, "battleType": 1, "firstCamp": 0, "sourceTable": "analysis/tables/TableCopyEnemy.tsv", "taskID": c.id, "originalEnemyRowIDs": enemyIDs, "goPredictedNewSkillIDs": gained, "goOutcome": name + ".go.json", "goOutcomeSHA256": p6Digest(out), "status": "awaiting-original-automatic-ai"})
	}
	if len(cases) == 0 {
		t.Fatal("no actually computed new NPC skill candidate", failures)
	}
	table, err := os.ReadFile(filepath.Join(root, "analysis/tables/TableCopyEnemy.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	manifest := map[string]any{"scope": "Explicit original-table mechanism probes, never naturalpaid chapter/private parameter/whole authority. Entire actual Task Group0 enemies/Location/Level/Props/Skills preserved. Attacker from SHA-bound real cold P5 and explicit diagnostic replay damage factors0/1; Go predictions only select original runtime inputs.", "nativeCompared": false, "authoritative": false, "sourceTableSHA256": p6Digest(table), "sourceAttackerPB": "analysis/p6/native-next-real-skills-inputs-v95/000.pb", "sourceAttackerSHA256": p6Digest(raw), "cases": cases, "candidateFailures": failures, "goPredictedDistinctNewNPCSkills": len(predicted), "originalTasks": len(groups)}
	out, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(absolute, "manifest.json"), append(out, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
}
