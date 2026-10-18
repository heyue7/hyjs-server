package service

import "math"

// combatOriginalAdditionalAttackDamage 保留 BuffOfProperty 138d–1490 的原 scalar 和乘法顺序。
func combatOriginalAdditionalAttackDamage(source, target *combatRoundActor, row map[string]string) float64 {
	extra := source.attribute("patt")
	defense := target.attribute("pdef")
	typedReduce := float64(target.attribute("pdamageReduce") * .01)
	if source.attackType != 1 {
		extra = source.attribute("matt")
		defense = target.attribute("mdef")
		typedReduce = float64(target.attribute("mdamageReduce") * .01)
	}
	sharedReduce := target.originalFraction("damageReduce")
	if row["Factor"] == "additionAttRealDam" {
		defense, sharedReduce, typedReduce = 0, 0, 0
	}
	attenuation := float64(1 + source.originalFraction("damageAdd"))
	attenuation = float64(attenuation - sharedReduce)
	attenuation = math.Max(0, float64(attenuation-typedReduce))
	damage := float64(source.attribute("att") + extra)
	damage = float64(damage - defense)
	// 原 1461–1474 先乘原 Args1，再乘0.01，最后乘减伤，不先合并百分比。
	damage = float64(damage * tableNumber(row, "Args1"))
	damage = float64(damage * .01)
	damage = float64(damage * attenuation)
	return math.Max(1, damage)
}
