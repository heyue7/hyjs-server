package service

import (
	"encoding/json"

	"hyjs-server/constUtil"
	"hyjs-server/model"
)

// p15GreenCombatOriginalView recognizes exactly the six approved compatibility
// rows. All other fields, row order and source tables must still hash to the
// immutable original source. The returned Hero rows are independent copies.
func p15GreenCombatOriginalView(tables map[string][]map[string]string) ([]map[string]string, bool) {
	pairs := [][2]string{{"hero_056", "monster_068"}, {"hero_057", "monster_070"}, {"hero_058", "monster_070"}, {"hero_059", "monster_070"}, {"hero_060", "monster_067"}, {"hero_061", "monster_070"}}
	columns := []string{"SkillId1", "BeFoundBone", "RushFlyTime", "RushFallTime", "JumpBackFlyTime", "JumpBackFallTime", "ImposState"}
	indexed := map[string]map[string]string{}
	for _, row := range tables["Hero"] {
		if indexed[row["id"]] != nil {
			return nil, false
		}
		indexed[row["id"]] = row
	}
	for _, pair := range pairs {
		row, donor := indexed[pair[0]], indexed[pair[1]]
		if row == nil || donor == nil || row["quality"] != "12" || row["heroClass"] != "0" {
			return nil, false
		}
		for _, key := range columns {
			if row[key] != donor[key] {
				return nil, false
			}
		}
	}
	view := make(map[string][]map[string]string, len(tables))
	for name, rows := range tables {
		view[name] = rows
	}
	heroRows := make([]map[string]string, 0, len(tables["Hero"]))
	for _, row := range tables["Hero"] {
		cloned := make(map[string]string, len(row))
		for key, value := range row {
			cloned[key] = value
		}
		for _, pair := range pairs {
			if row["id"] == pair[0] {
				for _, key := range columns {
					cloned[key] = ""
				}
				break
			}
		}
		heroRows = append(heroRows, cloned)
	}
	view["Hero"] = heroRows
	// Wanted8 freezes one additional original table; retain it in the caller
	// while recognizing only the unchanged eleven-table combat source here.
	delete(view, "WantedEnemy")
	raw, err := json.Marshal(view)
	if err != nil || p6Digest(raw) != constUtil.P6LinearPhysicalTables {
		return nil, false
	}
	return heroRows, true
}

func p15GreenCombatHasActor(input *model.FrozenBattleInput) bool {
	for _, hero := range input.Attacker.GetFormation() {
		if p15GreenCombatActorID(hero.GetId()) {
			return true
		}
	}
	for _, hero := range input.Defender.GetFormation() {
		if p15GreenCombatActorID(hero.GetId()) {
			return true
		}
	}
	for _, hero := range input.Monster.GetFormation() {
		if p15GreenCombatActorID(hero.GetId()) {
			return true
		}
	}
	return false
}

func p15GreenCombatActorID(id string) bool {
	switch id {
	case "hero_056", "hero_057", "hero_058", "hero_059", "hero_060", "hero_061":
		return true
	default:
		return false
	}
}

// Unused catalog compatibility cannot change the original actor calculations.
// Active green actors retain their approved original donor attack source rows.
func p15NormalizeUnusedGreenCombatRows(input *model.FrozenBattleInput, priorHash string) {
	if p15GreenCombatHasActor(input) {
		return
	}
	// An already frozen compatible source must retain its exact table identity
	// when canonical validation re-freezes it; new inputs have no prior hash.
	if priorHash != "" {
		raw, err := json.Marshal(input.Tables)
		if err == nil && p6Digest(raw) == priorHash {
			return
		}
	}
	if rows, ok := p15GreenCombatOriginalView(input.Tables); ok {
		input.Tables["Hero"] = rows
	}
}

// Six-green actors already have independent original-engine comparison
// evidence (P15 native-r1 twelve actors). Historical compatible views retain
// their source identity even without a green actor. Check the exact source delta;
// it never accepts another source edit or bypasses mechanism/runtime validation.
func p15GreenCombatSourceCompatible(input *model.FrozenBattleInput) bool {
	if input == nil {
		return false
	}
	if _, extra := input.Tables["WantedEnemy"]; extra {
		return false
	}
	raw, err := json.Marshal(input.Tables)
	if err != nil || p6Digest(raw) != input.ConfigurationHash {
		return false
	}
	_, ok := p15GreenCombatOriginalView(input.Tables)
	return ok
}
