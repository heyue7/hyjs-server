package service

import (
	pb "hyjs-server/model/protocol"
	"strings"
)

// Original BootHero/RecruitShowHero carry complete three-slot skill sequences.
// Accept only an exact original sequence for the named hero, never mixed variants.
func (e *combatRoundEngine) originalScriptedSkillSequence(hero *pb.BattleHero) bool {
	slots := strings.Split(hero.GetSkillOrder(), ",")
	if len(slots) != 3 || hero.GetAwakenSkill() != "" || hero.GetAwakeStage() != 0 {
		return false
	}
	// Original type8 enemies use WantedEnemy's exact skill triples. Preserve
	// the original Hero defaults for players and every unrelated mode.
	if e.input != nil && e.input.BattleType == 8 && e.input.Defender == nil && e.input.Monster != nil {
		monster := false
		for _, actor := range e.input.Monster.Formation {
			if actor == hero {
				monster = true
				break
			}
		}
		if monster {
			for _, row := range e.service.Tables["WantedEnemy"] {
				if row["HeroId"] == hero.GetId() && row["SkillId1"] == slots[0] && row["SkillId2"] == slots[1] && row["UltSkillId"] == slots[2] {
					return true
				}
			}
		}
	}
	for _, name := range []string{"BootHero", "RecruitShowHero"} {
		for _, row := range e.service.Tables[name] {
			if row["HeroId"] == hero.GetId() && row["SkillId1"] == slots[0] && row["SkillId2"] == slots[1] && row["UltSkillId"] == slots[2] {
				return true
			}
		}
	}
	return false
}
