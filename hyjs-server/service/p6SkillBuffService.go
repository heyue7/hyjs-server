package service

import "strings"

// Native changeSkillBuff mutates FSkillBuff triggers in this actor's skill instances.
// UnEffect does nothing: removal, suppression, death and round reset never undo replacement.
func (e *combatRoundEngine) originalChangeSkillBuff(target *combatRoundActor, row map[string]string) {
	parts := strings.Split(row["Param"], ",")
	if len(parts) != 3 {
		return
	}
	for _, skill := range target.skills {
		if skill.id != parts[0] {
			continue
		}
		if skill.buffOverrides == nil {
			skill.buffOverrides = map[string]string{}
		}
		for _, cast := range skill.casts {
			for _, slot := range []string{"0", "1", "2"} {
				key := cast["SkillCastId"] + "." + slot
				id := cast["BufferId"+slot]
				if current, ok := skill.buffOverrides[key]; ok {
					id = current
				}
				if id != parts[1] {
					continue
				}
				skill.buffOverrides[key] = parts[2]
				e.trace("skill_buff_replace", target, nil, skill.id, cast["SkillCastId"], 0)
				e.events[len(e.events)-1].Buff = parts[2]
			}
		}
	}
}
func (skill *combatRoundSkill) originalCastBuffID(cast map[string]string, slot string) string {
	if id, ok := skill.buffOverrides[cast["SkillCastId"]+"."+slot]; ok {
		return id
	}
	return cast["BufferId"+slot]
}
