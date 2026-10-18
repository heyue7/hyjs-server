package service

import (
	"errors"
	"hyjs-server/constUtil"
	"math"
)

// Native Effect and UnEffect both read the then-current maxHp, rather than a frozen val.
// This intentionally preserves the original residual if maxHp changes between the two calls.
func (e *combatRoundEngine) originalHPAPEffect(target *combatRoundActor, row map[string]string, direction int32) {
	if row["Factor"] == "MangerToNperhp" {
		target.angerToHPAP += direction * int32(tableNumber(row, "Args1"))
		amount := float64(float64(tableNumber(row, "Args2")*.01) * target.attribute("maxHp"))
		target.angerToHPAmount += float64(float64(direction) * amount)
	} else {
		amount := float64(float64(tableNumber(row, "Args1")*.01) * target.attribute("maxHp"))
		target.lossToAPHP += float64(float64(direction) * amount)
		target.lossToAPAP += direction * int32(tableNumber(row, "Args2"))
	}
}

// MinusHP executes after shields. Enough AP cancels HP loss with ceiling cost; insufficient
// AP spends whole steps and leaves a remainder. Native minimum HP damage applies afterward.
func (e *combatRoundEngine) originalMinusHP(target *combatRoundActor, change float64) float64 {
	if change > 0 {
		return change
	}
	anger := e.players[target.camp].anger
	if target.angerToHPAP > 0 && anger > 0 {
		steps := math.Floor(anger / float64(target.angerToHPAP))
		covered := float64(steps * target.angerToHPAmount)
		if covered >= -change {
			cost := math.Ceil(-change/target.angerToHPAmount) * float64(target.angerToHPAP)
			e.addAnger(target, -cost)
			e.trace("anger_absorb_hp", target, target, "", "", -change)
			return 0
		}
		e.addAnger(target, -steps*float64(target.angerToHPAP))
		change += covered
		if covered != 0 {
			e.trace("anger_absorb_hp", target, target, "", "", covered)
		}
	}
	return math.Min(-1, change)
}

// Native accumulation records nonlethal losses even while the conversion Buff is absent.
// The comparison is strict, so exactly one negative HF threshold does not trigger yet.
func (e *combatRoundEngine) originalLossAnger(target *combatRoundActor, change float64) {
	target.hpLossAccumulator += change
	if target.lossToAPAP <= 0 || !(target.hpLossAccumulator < target.lossToAPHP) {
		return
	}
	if target.lossToAPHP == 0 {
		e.failure = errors.New(constUtil.CombatUnsupported)
		return
	}
	steps := math.Floor(target.hpLossAccumulator / target.lossToAPHP)
	target.hpLossAccumulator -= float64(steps * target.lossToAPHP)
	e.addAnger(target, steps*float64(target.lossToAPAP))
	e.trace("hp_loss_to_anger", target, target, "", "", steps*float64(target.lossToAPAP))
}
