package service

import "math"

// Effect overwrites the single native attribute snapshot; every UnEffect clears it entirely.
func (e *combatRoundEngine) originalShareEffect(target *combatRoundActor, row map[string]string) {
	target.shareFactor = tableNumber(row, "Args1") * .01
	target.shareActors = nil
	maxHP := 0.0
	for _, actor := range e.players[target.camp].actors {
		if actor == target || actor.hp <= 0 {
			continue
		}
		if actor.hp > maxHP {
			target.shareActors = nil
			maxHP = actor.hp
		}
		if actor.hp == maxHP {
			target.shareActors = append(target.shareActors, actor)
		}
	}
}

// FRelease computes each share before the primary variance draw, with its own defense/reduction.
// Shared recipients do not receive critical/SPE/block factors or recursive release processing.
func (e *combatRoundEngine) originalSharedDamage(skill *combatRoundSkill, target *combatRoundActor, typ int32, factor, power, bonus float64, exportOrigin ...combatRoundHPExportOrigin) error {
	portion := factor * target.shareFactor / float64(len(target.shareActors))
	for _, actor := range target.shareActors {
		defense, reduce := actor.attribute("pdef"), actor.attribute("pdamageReduce")
		if typ == 2 {
			defense, reduce = actor.attribute("mdef"), actor.attribute("mdamageReduce")
		}
		// Original retains even dead cached recipients and consumes their variance/dodge draws.
		damage := (power - defense) * portion * math.Max(0, 1+bonus-actor.originalFraction("damageReduce")-reduce*.01) * (1 + e.rng.Range(-.1, .1))
		if !skill.actor.hasState("mustHit") && actor.attribute("dodge") > skill.actor.attribute("hit") {
			damage *= 1 - e.rng.Value()*math.Min(.5, (actor.attribute("dodge")-skill.actor.attribute("hit"))*.01)*2
		}
		killer := (*combatRoundActor)(nil)
		if actor == skill.target {
			killer = skill.actor
		}
		nominal, visible, shielded := e.damageHPKind(skill.actor, actor, math.Max(1, damage), typ, "share_damage", killer)
		e.originalExtraHPExport(actor, nominal, visible, shielded, 8, exportOrigin)
		if e.failure != nil {
			return e.failure
		}
	}
	return nil
}
