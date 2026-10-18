package service

import (
	"errors"
	"hyjs-server/constUtil"
	pb "hyjs-server/model/protocol"
	"strconv"
	"strings"
)

// p6AwakenSkillSlots mirrors BattleActorData's insertion into 0/1/3, preserving ultimate 2.
// Awakening panel bonuses are already frozen by the owning profile builder and stay unchanged.
func (e *combatRoundEngine) p6AwakenSkillSlots(hero *pb.BattleHero, row map[string]string) ([]string, string, error) {
	base := strings.Split(hero.GetSkillOrder(), ",")
	stage := hero.GetAwakeStage()
	if stage < 0 || stage > 2 {
		return nil, "", errors.New(constUtil.CombatInvalid)
	}
	if stage > 0 {
		for level := int32(1); level <= stage; level++ {
			var found map[string]string
			for _, candidate := range e.service.Tables["HeroAwakenAdd"] {
				if candidate["heroId"] == hero.GetId() && tableNumber(candidate, "stage") == float64(level) {
					if found != nil {
						return nil, "", errors.New(constUtil.CombatInvalid)
					}
					found = candidate
				}
			}
			if found == nil {
				return nil, "", errors.New(constUtil.CombatUnsupported)
			}
			if level == 1 && found["talentId"] != "" {
				present := false
				for _, id := range hero.GetTalentList() {
					present = present || id == found["talentId"]
				}
				if !present {
					return nil, "", errors.New(constUtil.CombatInvalid)
				}
			}
		}
	}
	if stage < 2 {
		if hero.GetAwakenSkill() != "" {
			return nil, "", errors.New(constUtil.CombatInvalid)
		}
		return base, "", nil
	}
	parts := strings.Split(strings.SplitN(hero.GetAwakenSkill(), ";", 2)[0], ",")
	if len(parts) != 2 || row["IsOpenAwake"] != "1" || parts[0] == "" || parts[0] != row["AwakeSkillId"] || len(base) != 3 {
		return nil, "", errors.New(constUtil.CombatInvalid)
	}
	position, err := strconv.Atoi(parts[1])
	if err != nil || position < 0 || position > 2 {
		return nil, "", errors.New(constUtil.CombatInvalid)
	}
	// The native input contains two ordered basic skills and the fixed ultimate.
	if base[2] != row["UltSkillId"] || !((base[0] == row["SkillId1"] && base[1] == row["SkillId2"]) || (base[0] == row["SkillId2"] && base[1] == row["SkillId1"])) {
		return nil, "", errors.New(constUtil.CombatInvalid)
	}
	slots := make([]string, 4)
	insert := position
	if position == 2 {
		insert = 3
	}
	slots[insert] = parts[0]
	for index, skill := range base {
		if slots[index] == "" {
			slots[index] = skill
		} else {
			slots[index+1] = skill
		}
	}
	if position < 2 {
		slots[2], slots[3] = slots[3], slots[2]
	}
	return slots, parts[0], nil
}
