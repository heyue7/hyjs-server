package service

import "math"

// Native treatHurtAndAddperAtt freezes an attack increment from the raw wound heal.
// Healing is immediate and never rolled back; unloading removes only this increment.
func (e *combatRoundEngine) originalWoundTreat(buff *combatRoundBuff) {
	heal := (buff.target.attribute("maxHp") - buff.target.hp) * tableNumber(buff.row, "Args1") * .01
	buff.value = heal * tableNumber(buff.row, "Args2") * .01
	e.changeHP(buff.source, buff.target, heal, "wound_treat")
}

// FRelease applies cripple after both camps' onceCast clearing, before LAST/weak processing.
// Its self damage uses physical shields, with no self-killer talent.
func (e *combatRoundEngine) originalCrippleCastEnd(actor *combatRoundActor) error {
	rate := actor.attribute("cripple") * .01
	if rate >= 0 {
		return nil
	}
	damage := math.Max(1, -rate*actor.attribute("maxHp"))
	e.damageHP(nil, actor, damage, 1)
	e.trace("cripple_tick", actor, actor, "", "", damage)
	return e.failure
}
