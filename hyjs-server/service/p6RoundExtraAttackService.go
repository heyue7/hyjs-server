package service

import (
	"fmt"
	"hyjs-server/constUtil"
	"math"
)

// ProcMyRoundStart uses the Buff recipient's current attack, not the Buff source.
// Visual kind8 is followed by owner.attackType; AddHp still consumes typed shields.
// There is no release hit/crit/variance/block, AP gain, retaliation or recursive share.
func (e *combatRoundEngine) originalRoundExtraAttacks(b *combatRoundBuff) error {
	if b.suppressed {
		return nil
	}
	owner := b.target
	index := -1
	for i, candidate := range owner.buffs {
		if candidate == b {
			index = i
			break
		}
	}
	if index < 0 {
		return fmt.Errorf("%s: detached round extra-attack Buff", constUtil.CombatUnsupported)
	}
	for i := 0; float64(i) < tableNumber(b.row, "Args3"); i++ {
		targets, err := e.selectTargetsWithConditions(owner, int32(tableNumber(b.row, "Args2")), nil, nil, true, true, true)
		if err != nil {
			return err
		}
		for _, target := range targets {
			attack, defense, reduce := owner.attribute("patt"), target.attribute("pdef"), target.attribute("pdamageReduce")
			if owner.attackType == 2 {
				attack, defense, reduce = owner.attribute("matt"), target.attribute("mdef"), target.attribute("mdamageReduce")
			}
			factor := math.Max(0, 1+owner.originalFraction("damageAdd")-target.originalFraction("damageReduce")-reduce*.01)
			damage := math.Max(1, (owner.attribute("att")+attack-defense)*tableNumber(b.row, "Args1")*.01*factor)
			e.damageHPKind(owner, target, damage, owner.attackType, "round_extra_damage", nil)
			e.trace("round_extra_attack", owner, target, "", "", damage)
			e.events[len(e.events)-1].Buff = b.row["Id"]
			if e.failure != nil {
				return e.failure
			}
			// Native rereads buffs[index] for subsequent fields/iterations. Its missing
			// content mapping gives no justified recovery when a nested death removes it.
			if index >= len(owner.buffs) || owner.buffs[index] != b {
				return fmt.Errorf("%s: round extra-attack Buff index mutated", constUtil.CombatUnsupported)
			}
		}
		// Native appends one FExportOnceData per numeric iteration, even with no target.
		if len(targets) == 0 {
			e.trace("round_extra_attempt", owner, nil, "", "", 0)
			e.events[len(e.events)-1].Buff = b.row["Id"]
		}
	}
	return nil
}
