package service

import "math"

// BuffOfProperty.Effect1d4b-1f07 snapshots target maxHp, optionally bounded by
// the source current skill's typed attack. ProcMyRoundStart applies it as type1.
func (e *combatRoundEngine) roundMaxHPValue(source, target *combatRoundActor, row map[string]string) float64 {
	value := target.attribute("maxHp") * tableNumber(row, "Args1") * .01
	capFactor := tableNumber(row, "Args2")
	if capFactor <= 0 {
		return value
	}
	kind := source.attackType
	if source.current != nil {
		kind = source.current.attackType
	}
	power := source.attribute("att")
	if kind == 1 {
		power += source.attribute("patt")
	} else if kind == 2 {
		power += source.attribute("matt")
	}
	limit := power * capFactor * .01
	if value > 0 {
		return math.Min(value, limit)
	}
	return math.Max(value, -limit)
}
func (e *combatRoundEngine) applyRoundMaxHP(b *combatRoundBuff) {
	if b.target.state >= 3 {
		return
	}
	if b.value < 0 {
		// ProcMyRoundStart exports the Buff-owned HP change. It does not run
		// the cast's LoadKillBuff path, even though its source remains visible.
		nominal, visible, shielded := e.damageHPKind(b.source, b.target, -b.value, 1, "damage", nil)
		e.originalBuffHPExport(b.source, b.target, nominal, visible, shielded, 15, []*combatRoundBuff{b})
	} else if b.value > 0 {
		e.originalBuffChangeHP(b.source, b.target, b.value, "round_maxhp_heal", 15, b)
	}
	e.trace("round_maxhp_change", b.source, b.target, "", "", b.value)
	e.events[len(e.events)-1].Buff = b.row["Id"]
}
