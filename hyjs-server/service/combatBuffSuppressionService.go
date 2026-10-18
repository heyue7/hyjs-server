package service

import (
	"fmt"
	"hyjs-server/constUtil"
)

// FBuffManager only temporarily disables positive, clearable Buffs. Their original
// instances and lifetimes remain in its collection; reactivation calls Effect again.
func (e *combatRoundEngine) suppressBuffsOnce(target *combatRoundActor) ([]*combatRoundBuff, error) {
	var suppressed []*combatRoundBuff
	for _, b := range target.buffs {
		if b.row["IsDebuff"] == "1" || b.row["CanBeCleared"] != "1" || b.suppressed {
			continue
		}
		switch b.row["Factor"] {
		case "delayCreateBuffs", "reliveAndinvisible", "clearBuff", "clearDebuff", "clearOneBuff", "clearOneDebuff":
			return nil, fmt.Errorf("%s: temporary reactivation %s/%s", constUtil.CombatUnsupported, b.row["Id"], b.row["Factor"])
		}
		suppressed = append(suppressed, b)
	}
	for _, b := range suppressed {
		if b.row["Factor"] == "MangerToNperhp" || b.row["Factor"] == "MperhpreduceToNanger" {
			e.originalHPAPEffect(target, b.row, -1)
		}
		if b.row["Factor"] == "damShareByMaxHpTeammate" {
			target.shareFactor, target.shareActors = 0, nil
		}
		if b.row["Factor"] == "counterStrikeDam" || b.row["Factor"] == "counterStrikeRealDam" {
			target.counterReal = false
		}
		if kind := b.shieldKind(); kind != "" {
			// Original shield UnEffect resets the whole typed attribute, including
			// contributions from other shields. Effect later restores original Args1.
			for _, shield := range target.buffs {
				if shield.shieldKind() == kind {
					shield.value = 0
				}
			}
		}
		e.originalPercentEffect(b, false)
		b.suppressed = true
		e.trace("buff_suppress", b.source, target, "", "", 0)
		e.events[len(e.events)-1].Buff = b.row["Id"]
	}
	return suppressed, nil
}

func (e *combatRoundEngine) restoreBuffsOnce(target *combatRoundActor, suppressed []*combatRoundBuff) error {
	for _, b := range suppressed {
		b.suppressed = false
		present := false
		for _, current := range target.buffs {
			if current == b {
				present = true
				break
			}
		}
		// ClearAllBuff restores then empties its temporary list before Die talents.
		// A revived actor must not re-effect a cleared pre-death healing Buff later.
		if !present {
			continue
		}
		x := tableNumber(b.row, "Args1")
		switch b.row["Factor"] {
		case "MangerToNperhp", "MperhpreduceToNanger":
			e.originalHPAPEffect(target, b.row, 1)
		case "WeaponPerAtt":
			b.value = 0
			e.trace("native_buff_no_effect", b.source, target, "", "", 0)
			e.events[len(e.events)-1].Buff = b.row["Id"]
		case "treatHurtAndAddperAtt":
			e.originalWoundTreat(b)
		case "damShareByMaxHpTeammate":
			e.originalShareEffect(target, b.row)
		case "changeSkillBuff":
			e.originalChangeSkillBuff(target, b.row)
		case "angerChange", "angerChangeAndAddHp", "angerAndhp":
			var err error
			b.value, err = e.originalAngerCompoundEffect(b.source, target, b.row)
			if err != nil {
				return err
			}
		case "clearAllDebuff", "clearSomeBuffs":
			e.originalDispelEffect(target, b.row)
		case "permaxHpBySelfRound":
			b.value = e.roundMaxHPValue(b.source, target, b.row)
		case "blockDam":
			target.blockDam = tableNumber(b.row, "Args2")
		case "sourceNhitToperdef", "angerTocri", "comboNumTocri", "angerToperatt", "criTocriFactor", "sourcepermdefTomdef":
			// Effect runs after its own old contribution has been removed.
			b.suppressed = true
			b.value = e.propertyTransferValue(b.source, target, b.row)
			b.suppressed = false
		case "minusMaxHpPercent":
			b.value = e.maxHPPercentEffect(b.source, target, b.row)
		case "perhpTodamageAdd", "perhpTododge", "perhpTocri", "perhpToPeratt", "perhpToperdef":
			b.value = p6HPTransferValue(target, b.row)
		case "counterStrikeDam", "counterStrikeRealDam":
			target.counterReal = b.row["Factor"] == "counterStrikeRealDam"
			target.counterSelect = int32(tableNumber(b.row, "Args2"))
		case "sourceperattToshield", "sourceperattTopshield", "sourceperattTomshield":
			if p6OriginalFactoryNoEffect(b.row) {
				b.value = 0
				e.trace("native_buff_no_effect", b.source, target, "", "", 0)
				e.events[len(e.events)-1].Buff = b.row["Id"]
			} else {
				b.value = b.source.attribute("att") * x * .01
			}
			b.initialValue = b.value
		case "shield", "pshield", "mshield":
			b.value = x
		case "recoverBySelfRound":
			b.value = (b.source.attribute("att")+b.source.attribute("patt"))*x*.01*(1+b.source.attribute("treatAdd")*.01) + b.source.attribute("cure") + target.attribute("heal")
		case "poisonDam", "fireDam":
			power, resistance := "poison", "antiPoison"
			if b.row["Factor"] == "fireDam" {
				power, resistance = "fire", "antiFire"
			}
			b.value = (b.source.attribute("att")+b.source.attribute("matt"))*.6 + b.source.attribute(power)*.01 - target.attribute(resistance)*.01
		case "hp":
			e.changeHP(b.source, target, x, "buff_heal")
		case "perhp":
			e.changeHP(b.source, target, target.hp*x*.01, "buff_heal")
		case "permaxHp":
			e.changeHP(b.source, target, target.attribute("maxHp")*x*.01, "buff_heal")
		case "recover":
			e.originalRecover(b.source, target, b.row)
		case "anger":
			e.addAnger(target, x)
		case "minusSkillCoolDown":
			for _, skill := range target.skills {
				skill.cooldown = max(0, skill.cooldown-int32(x))
			}
		case "additionAttDam", "additionAttRealDam":
			if err := e.additionalAttackDamage(b.source, target, b.row, b); err != nil {
				return err
			}
		case "additionSourcepermaxHp":
			b.value = b.source.attribute("maxHp") * x * .01
			if b.value > 0 && target.state < 3 {
				e.damageHP(b.source, target, b.value, b.source.attackType)
			} else if b.value < 0 {
				e.changeHP(b.source, target, -b.value, "buff_heal")
			}
			e.trace("source_maxhp_damage", b.source, target, "", "", b.value)
		}
		e.originalPercentEffect(b, true)
		e.trace("buff_restore", b.source, target, "", "", 0)
		e.events[len(e.events)-1].Buff = b.row["Id"]
	}
	return nil
}
