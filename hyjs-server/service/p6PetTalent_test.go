package service

import (
	"encoding/csv"
	"encoding/json"
	"os"
	"sort"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p6PetAreaTalentInput(t *testing.T, s *GameService) *model.FrozenBattleInput {
	input := p6CompleteBowInput(t, s)
	input.Attacker.Formation[0].TalentList = []string{"tal_3084"}
	input.Seed = 38
	return input
}

func TestP6OriginalPetNormalCastAreaTalentRollsAndFilters(t *testing.T) {
	s := p6OriginalTables(t)
	input := p6PetAreaTalentInput(t, s)
	e := &combatRoundEngine{service: &GameService{Tables: input.Tables}, input: input, rng: NewCombatRandom(38), operate: &pb.BattleOperate{}}
	if err := e.initialize(); err != nil {
		t.Fatal(err)
	}
	a, targets := e.players[0].actors[0], e.players[1].actors
	skill := a.skill(1)
	skill.target, skill.targets = targets[0], targets
	if err := e.castTalentTimers(skill, targets[0], 5); err != nil {
		t.Fatal(err)
	}
	// Original tal3084 has exactly 2% probability and Select2. Seed38's three
	// draws are .727922, .0050459, .0275298: only the second cast target succeeds.
	if e.rng.Draws() != 3 || targets[0].hasState("dizzy") || !targets[1].hasState("dizzy") || targets[2].hasState("dizzy") {
		t.Fatal("before-cast talent used only its main reference", e.rng.Draws())
	}
	e.unloadBuff(targets[1], targets[1].buffs[0])
	if err := e.loadBuff(a, targets[0], map[string]string{"Id": "pet-scope-invincible", "Factor": "invincible", "TotalRound": "1"}, 4); err != nil {
		t.Fatal(err)
	}
	e.rng = NewCombatRandom(38)
	if err := e.castTalentTimers(skill, targets[0], 5); err != nil {
		t.Fatal(err)
	}
	if e.rng.Draws() != 2 || targets[1].hasState("dizzy") || !targets[2].hasState("dizzy") {
		t.Fatal("invincible receiver consumed an original debuff chance", e.rng.Draws())
	}
	skill.targets = nil
	e.rng = NewCombatRandom(38)
	if err := e.castTalentTimers(skill, targets[1], 5); err != nil || e.rng.Draws() != 0 {
		t.Fatal("empty cast collection fell back to a single reference", err, e.rng.Draws())
	}
	// LoadAttBuff preserves original talent/slot order across NormalAtt5 and AllAtt4.
	// The normal area talent comes first; the guaranteed self buff draws fourth.
	mixed := p6PetAreaTalentInput(t, s)
	mixed.Attacker.Formation[0].TalentList = []string{"tal_3084", "tal_3000"}
	m := &combatRoundEngine{service: &GameService{Tables: mixed.Tables}, input: mixed, rng: NewCombatRandom(38), operate: &pb.BattleOperate{}}
	if err := m.initialize(); err != nil {
		t.Fatal(err)
	}
	ma, mt := m.players[0].actors[0], m.players[1].actors
	ms := ma.skill(1)
	ms.target, ms.targets = mt[0], mt
	if err := m.castTalentTimers(ms, mt[0], 4, 5); err != nil || m.rng.Draws() != 4 || mt[0].hasState("dizzy") || !mt[1].hasState("dizzy") || mt[2].hasState("dizzy") {
		t.Fatal("mixed before-cast timers changed original talent order", err, m.rng.Draws())
	}
	closeCombat(t, ma.attribute("damageAdd"), 2)
}

func TestP6OriginalPetAreaTalentFullBowWholeBattleAndReplay(t *testing.T) {
	s := p6OriginalTables(t)
	input := p6PetAreaTalentInput(t, s)
	out, err := s.ComputeFrozenBattle(t.Context(), input, nil)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, event := range out.Events {
		if event.Kind == "release" && event.Actor == "0_complete-bow" {
			seen[event.Skill] = true
		}
	}
	if len(seen) != 3 || out.Authoritative || len(out.Attacker.Formation[0].GetTalentList()) != 1 {
		t.Fatal("original complete actor or guardian talent lost", seen)
	}
	replayed, err := s.ComputeFrozenBattle(t.Context(), input, out.Operates)
	if err != nil || !proto.Equal(out.Operates, replayed.Operates) || !proto.Equal(out.Attacker, replayed.Attacker) || !proto.Equal(out.Defender, replayed.Defender) || out.RandomDraws != replayed.RandomDraws {
		t.Fatal("guardian talent full original replay diverged", err)
	}
}

// This explicit diagnostic retains every original pet/level mapping and records
// exact factory/replay boundaries; it never equates fixture replay with device parity.
func TestP6OriginalPetTalentSkillFactoryScopeDiagnostic(t *testing.T) {
	if os.Getenv("HYJS_P6_PET_DIAGNOSTIC") != "1" {
		t.Skip("explicit original pet talent coverage diagnostic disabled")
	}
	f, err := os.Open("../../analysis/tables/TablePetTalentSkill.tsv")
	if err != nil {
		t.Fatal(err)
	}
	r := csv.NewReader(f)
	r.Comma, r.FieldsPerRecord = '\t', -1
	rows, err := r.ReadAll()
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	var original []map[string]string
	unique := map[string]bool{}
	for _, row := range rows[5:] {
		entry := map[string]string{}
		for i, v := range row {
			if i < len(rows[1]) {
				entry[rows[1][i]] = v
			}
		}
		original = append(original, entry)
		unique[entry["talent"]] = true
	}
	var ids []string
	for id := range unique {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	s := p6OriginalTables(t)
	results := map[string]any{}
	passed, rejected := 0, 0
	for _, id := range ids {
		a := p6Actor("hero_001", "pet-scope-source", "001_1,001_2,001_3", 1, 80000)
		a.TalentList = []string{id}
		input := p6Input(t, s, a, p6Actor("hero_047", "pet-scope-target", "047_1,047_2,047_3", 1, 80000), 8)
		out, computeErr := s.ComputeFrozenBattle(t.Context(), input, nil)
		entry := map[string]any{"nativeCompared": false, "authoritative": false}
		if computeErr != nil {
			entry["computeError"] = computeErr.Error()
			rejected++
		} else {
			replayed, replayErr := s.ComputeFrozenBattle(t.Context(), input, out.Operates)
			if replayErr != nil || out.Authoritative || !proto.Equal(out.Operates, replayed.Operates) || !proto.Equal(out.Attacker, replayed.Attacker) || !proto.Equal(out.Defender, replayed.Defender) || out.RandomDraws != replayed.RandomDraws {
				t.Fatal("original pet talent replay or authority gate changed", id, replayErr)
			}
			entry["wholeReplayPassed"], entry["finalRound"], entry["randomDraws"] = true, out.FinalRound, out.RandomDraws
			passed++
		}
		results[id] = entry
	}
	report := map[string]any{"engineVersion": constUtil.P6EngineVersion, "originalPetTalentSkill": original, "uniqueTalentCount": len(ids), "computedAndReplayed": passed, "rejected": rejected, "results": results, "nativeCompared": false, "authoritative": false}
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile("../../analysis/p6/pet-talent-scope-20261013.json", raw, 0644); err != nil {
		t.Fatal(err)
	}
	t.Logf("original pet rows=%d unique=%d full replay=%d rejected=%d; native gate closed", len(original), len(ids), passed, rejected)
}
