package service

import (
	"encoding/json"
	"strings"

	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// normalizeUnusedThreeOrangeCombatRows keeps an unrelated battle on its exact
// original source. Startup corrections for three absent actors cannot invalidate
// every existing mode certificate. Their actual battles retain all authored rows.
func normalizeUnusedThreeOrangeCombatRows(input *model.FrozenBattleInput, priorHash string) {
	normalizeUnusedThreeOrangeCombatRowsWithHash(input, priorHash, "")
}

// 返回true仅表示已赋值表；调用者必须重算摘要后再交给后续normalizer。
func normalizeUnusedThreeOrangeCombatRowsWithHash(input *model.FrozenBattleInput, priorHash, currentHash string) bool {
	for _, formation := range [][]*pb.BattleHero{input.Attacker.GetFormation(), input.Defender.GetFormation(), input.Monster.GetFormation()} {
		for _, actor := range formation {
			if actor == nil {
				return false
			}
			for _, id := range []string{"069", "070", "073"} {
				if actor.GetId() == "hero_"+id || strings.Contains(actor.GetSkillOrder(), id+"_") || strings.Contains(actor.GetAwakenSkill(), id+"_") {
					return false
				}
			}
		}
	}
	// Re-freezing an already matching receipt preserves its table identity.
	if currentHash == "" {
		raw, err := json.Marshal(input.Tables)
		if err != nil {
			return false
		}
		currentHash = p6Digest(raw)
	}
	if priorHash != "" && currentHash == priorHash {
		return false
	}
	view := make(map[string][]map[string]string, len(input.Tables))
	for name, rows := range input.Tables {
		view[name] = rows
	}
	rows := adminGuildCloneRows(input.Tables["SkillCast"])
	indexed := map[string]map[string]string{}
	for _, row := range rows {
		if indexed[row["SkillCastId"]] != nil {
			return false
		}
		indexed[row["SkillCastId"]] = row
	}
	for _, spec := range threeOrangeSkillFactorSpecs() {
		row := indexed[spec.id]
		if row == nil || row["SkillId"] != spec.skill || row["ExportFactor"] != "1" && row["ExportFactor"] != spec.factor {
			return false
		}
		row["ExportFactor"] = "1"
	}
	view["SkillCast"] = rows
	// Accept only the exact original collection or the independently recognized
	// green-row projection. An unrelated table edit keeps the existing rejection.
	raw, err := json.Marshal(view)
	if err != nil {
		return false
	}
	if p6Digest(raw) != constUtil.P6LinearPhysicalTables {
		if _, ok := p15GreenCombatOriginalView(view); !ok {
			return false
		}
	}
	input.Tables["SkillCast"] = rows
	return true
}
