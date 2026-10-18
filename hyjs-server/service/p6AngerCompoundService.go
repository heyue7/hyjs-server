package service

import "math"

// Original class0 immediate AP steal and AP/HP compound effects. HP uses the raw
// consumed amount, before source angerRate or AP maximum truncates the gain.
func (e *combatRoundEngine) originalAngerCompoundEffect(source, target *combatRoundActor, row map[string]string, buffs ...*combatRoundBuff) (float64, error) {
	amount := math.Min(e.players[target.camp].anger, tableNumber(row, "Args1"))
	if row["Factor"] == "angerAndhp" {
		amount = math.Min(e.players[target.camp].anger, math.Abs(tableNumber(row, "Args1")))
	}
	if amount <= 0 {
		return 0, nil
	}
	if row["Factor"] == "angerAndhp" {
		delta := amount
		if tableNumber(row, "Args1") <= 0 {
			delta = -amount
		}
		e.originalBuffAnger(source, target, delta, buffs...)
		value := target.attribute("maxHp") * tableNumber(row, "Args2") * .01
		if x := tableNumber(row, "Args3"); x != 0 {
			cap := source.attribute("att") * x * .01
			if value > 0 && value > cap || value <= 0 && value < cap {
				value = cap
			}
		}
		value *= amount
		if value < 0 {
			e.originalBuffDamageHP(source, target, -value, 1, 15, buffs...)
		} else if value > 0 {
			e.originalBuffChangeHP(source, target, value, "anger_hp_heal", 15, buffs...)
		}
		e.trace("anger_hp_change", source, target, "", "", value)
		return value, e.failure
	}
	e.originalBuffAnger(source, target, -amount, buffs...)
	e.originalBuffAnger(source, source, amount, buffs...)
	e.trace("anger_steal", source, target, "", "", amount)
	if row["Factor"] == "angerChangeAndAddHp" {
		// Original LimitConditions(0,1,1); selection happens after both AP mutations.
		recipients, err := e.selectTargetsWithConditions(source, int32(tableNumber(row, "Args2")), nil, nil, false, true, true)
		if err != nil {
			return 0, err
		}
		for _, recipient := range recipients {
			heal := (source.attribute("att")+source.attribute("patt"))*tableNumber(row, "Args3")*amount*.01*(1+float64(source.attribute("treatAdd")*.01)) + source.attribute("cure") + recipient.attribute("heal")
			e.originalBuffChangeHP(source, recipient, heal, "anger_steal_heal", 11, buffs...)
		}
	}
	return amount, e.failure
}
