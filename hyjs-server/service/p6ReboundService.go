package service

import "strings"

// reboundPerMaxhp is native teammate revival, with separate source and recipient one-use flags.
// Its Effect clears even its just-inserted marker, then heals and loads talent-origin Param buffs.
func (e *combatRoundEngine) originalReboundEffect(source, target *combatRoundActor, buff *combatRoundBuff) error {
	target.state = 0
	target.hpLossAccumulator = 0
	target.hpChangeInRound = 0
	target.current, target.waiting = nil, nil
	target.movedTarget, target.movedRule, target.waitUntil = "", -1, 0
	target.speState, target.speUntil = 1, 0
	for _, skill := range target.skills {
		skill.state, skill.stage, skill.releaseIndex = 0, 0, 0
		skill.first, skill.target, skill.targets, skill.sameDamTargets = false, nil, nil, nil
		// FSkill.ResetRound retains cooldown and per-actor skill template overrides.
	}
	source.reboundUsed, target.rebounded = true, true
	for _, old := range append([]*combatRoundBuff(nil), target.buffs...) {
		e.unloadBuff(target, old)
	}
	buff.value = target.attribute("maxHp") * tableNumber(buff.row, "Args1") * .01
	e.originalBuffChangeHP(source, target, buff.value, "teammate_revive", 17, buff)
	dead := e.players[target.camp].deadActors
	for i, actor := range dead {
		if actor == target {
			e.players[target.camp].deadActors = append(dead[:i], dead[i+1:]...)
			break
		}
	}
	for _, id := range strings.Split(buff.row["Param"], ",") {
		if id == "" {
			continue
		}
		if err := e.loadBuffWithOrigin(source, target, e.table("Buff", "Id", id), buff.timer, buff.origin); err != nil {
			return err
		}
	}
	return e.failure
}
