package service

// Original FAttribute scalar percentages are stateful fractions. Effect and
// UnEffect apply each delta in their actual order; summing only live Buff rows
// loses the original rounding residue after stacks are removed.
func (a *combatRoundActor) originalFraction(key string) float64 {
	if key != "damageAdd" && key != "damageReduce" && key != "lifestealHp" {
		return float64(a.attribute(key) * .01)
	}
	if a.originalPercent == nil {
		a.originalPercent = map[string]float64{}
		a.originalPercentBase = map[string]float64{}
	}
	base := a.base[key]
	prior, exists := a.originalPercentBase[key]
	if !exists || prior != base {
		a.originalPercentBase[key] = base
		a.originalPercent[key] = float64(base * .01)
	}
	return a.originalPercent[key]
}

func (a *combatRoundActor) originalPercentChange(key string, delta float64) {
	value := a.originalFraction(key)
	a.originalPercent[key] = value + delta
}

func (e *combatRoundEngine) originalPercentEffect(b *combatRoundBuff, apply bool) {
	if b.originalPercentApplied == apply || p6OriginalFactoryNoEffect(b.row) {
		return
	}
	sign := 1.0
	if !apply {
		sign = -1
	}
	x := float64(tableNumber(b.row, "Args1") * .01)
	change := func(key string, value float64) { b.target.originalPercentChange(key, sign*value) }
	switch b.row["Factor"] {
	case "damageAdd", "damageReduce", "lifestealHp":
		change(b.row["Factor"], x)
	case "damageAddAndReduce":
		change("damageAdd", x)
		change("damageReduce", float64(tableNumber(b.row, "Args2")*.01))
	case "damageReduceChange":
		change("damageReduce", -x)
		b.source.originalPercentChange("damageReduce", sign*x)
	case "perhpTodamageAdd":
		change("damageAdd", float64(b.value*.01))
	case "iceYNT":
		change("damageAdd", float64(tableNumber(parseP6Condition(b.row["Param"]), "damageAdd")*.01))
	}
	b.originalPercentApplied = apply
}
