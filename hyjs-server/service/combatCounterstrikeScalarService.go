package service

import "math"

// combatOriginalCounterstrikeDamage 保留 FRelease 119a–129b 的原 fraction 读取与逐步运算。
// 减伤 Effect/UnEffect 已按原顺序累积，转为百分数再转回来会改变实际 IEEE 值。
func combatOriginalCounterstrikeDamage(owner, attacker, target *combatRoundActor, incoming float64) float64 {
	percent := float64(owner.attribute("counterStrikeDam") * .01)
	extra := owner.attribute("patt")
	defense := target.attribute("pdef")
	typedReduce := float64(target.attribute("pdamageReduce") * .01)
	if owner.attackType == 2 {
		extra = owner.attribute("matt")
		defense = target.attribute("mdef")
		typedReduce = float64(target.attribute("mdamageReduce") * .01)
	}
	// 原 IL 11a8 使用本次原攻击者的减伤；范围反击时也不切换为各被选目标。
	sharedReduce := attacker.originalFraction("damageReduce")
	if owner.counterReal {
		defense, sharedReduce, typedReduce = 0, 0, 0
	}
	attenuation := float64(1 + owner.originalFraction("damageAdd"))
	attenuation = float64(attenuation - sharedReduce)
	attenuation = math.Max(0, float64(attenuation-typedReduce))
	attack := float64(owner.attribute("att") + extra)
	attack = float64(attack - defense)
	damage := float64(attack * percent)
	damage = float64(damage * attenuation)
	cap := float64(incoming * percent)
	return math.Max(1, math.Min(damage, cap))
}
