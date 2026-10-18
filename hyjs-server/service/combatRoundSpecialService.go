package service

import (
	"errors"
	"hyjs-server/constUtil"
	"math"
	"strings"
)

// specialHit restores every typed hit's float/fall change for normal default-state actors.
func (e *combatRoundEngine) specialHit(source, target *combatRoundActor, cause int32) float64 {
	if target.hasState("immuneFloatBreak") || target.shield("shield") > 0 {
		return 0
	}
	duration := 0.0
	if (cause == 2 || target.speState == 2 && cause != 3) && target.speState != 3 && !target.unableStates[2] {
		target.speState = 2
		duration = e.number("FloatDuration")
	}
	if cause == 3 && target.speState != 3 && !target.unableStates[3] {
		target.speState = 3
		duration = e.number("BreakDuration")
	}
	if duration > 0 {
		target.speUntil = e.time + duration
		e.trace("special_begin", source, target, "", "", float64(target.speState))
	}
	return duration
}
func (e *combatRoundEngine) specialFactor(advance, state int32, kind string) float64 {
	names := []string{"None", "Normal", "Float", "Break"}
	if advance < 0 || advance > 3 || state < 0 || state > 3 {
		return 1
	}
	row := e.table("GameConfigure", "StaticName", names[advance]+"2"+names[state]+"2"+kind)
	if row == nil {
		return 1
	}
	return tableNumber(row, "NameValue")
}
func (e *combatRoundEngine) magicStone(source, target *combatRoundActor, origin combatRoundBuffOrigin, exports ...combatRoundHPExportOrigin) error {
	if source.camp != 0 && e.input.Defender == nil {
		return nil
	}
	if e.rng.Value() > e.number("BreakGetMagicStonePercent") {
		return nil
	}
	minimum := int32(math.MaxInt32)
	var candidates []*combatRoundActor
	for _, a := range e.players[source.camp].actors {
		if a.state == 4 {
			continue
		}
		if a.magicStones < minimum {
			minimum = a.magicStones
			candidates = nil
		}
		if a.magicStones == minimum {
			candidates = append(candidates, a)
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	var ready []*combatRoundActor
	for _, a := range candidates {
		if s := a.skill(2); s != nil && s.cooldown == 0 {
			ready = append(ready, a)
		}
	}
	if len(ready) > 0 {
		candidates = ready
	}
	recipient := candidates[0]
	for _, a := range candidates {
		if a.position() < recipient.position() {
			recipient = a
		}
	}
	row := e.table("GameConfigure", "StaticName", "MagicStoneBuff")
	buff := e.table("Buff", "Id", row["NameValue"])
	if buff == nil {
		return errors.New(constUtil.CombatUnsupported)
	}
	for _, id := range strings.Split(buff["Param"], ",") {
		if r := e.table("Buff", "Id", id); r == nil || !supportedP6Factor(r["Factor"]) {
			return errors.New(constUtil.CombatUnsupported)
		}
	}
	recipient.magicStones++
	e.trace("magic_stone", source, recipient, "", "", 1)
	e.events[len(e.events)-1].Skill = origin.skill
	e.events[len(e.events)-1].Cast = origin.cast
	e.events[len(e.events)-1].OriginalValueClass = 5
	if len(exports) > 0 {
		e.events[len(e.events)-1].OriginalValueOrder = exports[0].nextOrder()
	}
	e.events[len(e.events)-1].OriginalValueSource = target.uid
	e.events[len(e.events)-1].OriginalValueTarget = recipient.uid
	return e.loadBuffWithOrigin(source, recipient, buff, 4, origin)
}

// counterStrike restores FRelease's normal-hit retaliation without another skill or RNG draw.
// The original cap uses incoming pre-shield damage; real retaliation still spends typed shields.
func (e *combatRoundEngine) counterStrike(owner, attacker *combatRoundActor, incoming float64, exportOrigin ...combatRoundHPExportOrigin) error {
	percent := owner.attribute("counterStrikeDam") * .01
	if percent <= 0 || owner.hp <= 0 || owner.state >= 3 {
		return nil
	}
	targets, err := e.selectTargets(owner, owner.counterSelect, attacker, nil)
	if err != nil {
		return err
	}
	for _, target := range targets {
		if target.hp <= 0 || target.state >= 3 || target.hasState("invisible") || target.hasState("invincible") {
			continue
		}
		damage := combatOriginalCounterstrikeDamage(owner, attacker, target, incoming)
		if !combatFinite(damage) {
			return errors.New(constUtil.CombatInvalid)
		}
		nominal, visible, shielded := e.damageHP(owner, target, damage, owner.attackType)
		e.originalExtraHPExport(target, nominal, visible, shielded, 7, exportOrigin)
		e.trace("counterstrike", owner, target, "", "", damage)
	}
	return e.failure
}

// bleedingHit applies FRelease physical extra damage after retaliation and before healing.
// Current attacker attributes supply the physical formula, including for magical normal hits.
func (e *combatRoundEngine) bleedingHit(source, target *combatRoundActor, exportOrigin ...combatRoundHPExportOrigin) error {
	percent := target.attribute("bleeding") * .01
	if percent <= 0 || target.state >= 4 {
		return nil
	}
	damage := (source.attribute("att") + source.attribute("patt") - target.attribute("pdef")) * percent * math.Max(0, 1+source.originalFraction("damageAdd")-target.originalFraction("damageReduce")-target.attribute("pdamageReduce")*.01)
	damage = math.Max(1, damage)
	if !combatFinite(damage) {
		return errors.New(constUtil.CombatInvalid)
	}
	nominal, visible, shielded := e.damageHP(source, target, damage, 1)
	e.originalExtraHPExport(target, nominal, visible, shielded, 12, exportOrigin)
	e.trace("bleeding", source, target, "", "", damage)
	return e.failure
}

// additionalAttackDamage mirrors immediate BuffOfProperty attack damage without an RNG draw.
// Real damage bypasses defense and reduction, retaining the source typed shield consumption.
func (e *combatRoundEngine) additionalAttackDamage(source, target *combatRoundActor, row map[string]string, buffs ...*combatRoundBuff) error {
	if target.state >= 4 {
		return nil
	}
	damage := combatOriginalAdditionalAttackDamage(source, target, row)
	if !combatFinite(damage) {
		return errors.New(constUtil.CombatInvalid)
	}
	e.originalBuffDamageHP(source, target, damage, source.attackType, 8, buffs...)
	e.trace("additional_attack_damage", source, target, "", "", damage)
	return e.failure
}

// finishPendingDeaths waits for original released attacks to reach LAST before DoDead.
// Clearing old Buffs precedes Die talents, allowing the one-use native revival to restore HP.
// finishReleaseDeaths applies FRelease.Do's last-cast boundary before attack-end
// Buffs. A finishing actor no longer reserves its held weak target; another
// attacker's live release still reserves it through GetNumActorAttack.
func (e *combatRoundEngine) finishReleaseDeaths(ending *combatRoundSkill) error {
	for _, target := range e.players[1-ending.actor.camp].actors {
		if target.state != 3 {
			continue
		}
		pending := false
		for _, actor := range e.players[ending.actor.camp].actors {
			skill := actor.current
			if skill == nil || actor.state == 4 || skill.state >= 3 || skill == ending && skill.target == target {
				continue
			}
			if skill.first {
				pending = pending || skill.target == target
			} else {
				for _, selected := range skill.targets {
					pending = pending || selected == target
				}
			}
		}
		if !pending {
			if err := e.completeActorDeath(target); err != nil {
				return err
			}
		}
	}
	return nil
}

func (e *combatRoundEngine) finishPendingDeaths() error {
	for _, player := range e.players {
		for _, target := range player.actors {
			if target.state != 3 {
				continue
			}
			pending := target.current != nil && target.current.first && target.current.state < 3
			for _, attacker := range e.players[1-target.camp].actors {
				skill := attacker.current
				if skill == nil || attacker.state == 4 || skill.state >= 3 {
					continue
				}
				if skill.first {
					pending = pending || skill.target == target
				} else {
					for _, actor := range skill.targets {
						pending = pending || actor == target
					}
				}
			}
			if pending {
				continue
			}
			if err := e.completeActorDeath(target); err != nil {
				return err
			}
		}
	}
	return nil
}

// completeActorDeath mirrors DoDead, including direct round-end deaths without a release wait.
func (e *combatRoundEngine) completeActorDeath(target *combatRoundActor) error {
	target.hp = 0
	target.state = 4
	if target.current == e.pendingUltimate && target.current != nil {
		e.pendingUltimate = nil
		e.ultimateRunning = false
	}
	target.current = nil
	target.waiting = nil
	for _, skill := range target.skills {
		skill.state = 4
	}
	// Original death ClearAllBuff walks persistent identities from the tail.
	for i := len(target.buffs) - 1; i >= 0; i-- {
		e.unloadBuff(target, target.buffs[i])
	}
	e.trace("dead", target, nil, "", "", 0)
	parentScope := e.originalCastScope
	e.originalCastScope = nil
	valueParent := e.originalBuffValueDeath
	e.originalBuffValueDeath = nil
	roundBuff := e.originalRoundBuffDeath
	e.originalRoundBuffDeath = nil
	e.deathTriggers(target.deathSource, target)
	e.originalRoundBuffDeath = roundBuff
	e.originalBuffValueDeath = valueParent
	e.originalCastScope = parentScope
	if target.state == 4 {
		if valueParent == nil {
			e.originalCastDeadActor(target)
		} else {
			e.trace("buff_value_dead_export", valueParent.source, target, "", "", 0)
			event := &e.events[len(e.events)-1]
			event.Buff = valueParent.row["Id"]
			event.OriginalBuffExport = valueParent.originalExportGroup
			event.OriginalBuffOwner = valueParent.target.uid
			e.originalBuffSnapshots = append(e.originalBuffSnapshots, combatRoundPendingBuffSnapshot{index: len(e.events) - 1, buff: valueParent})
		}
		if roundBuff != nil {
			e.trace("buff_round_dead_export", roundBuff.source, target, "", "", 0)
			e.events[len(e.events)-1].Buff = roundBuff.row["Id"]
			e.events[len(e.events)-1].OriginalBuffExport = roundBuff.originalExportGroup
			e.events[len(e.events)-1].OriginalBuffOwner = roundBuff.target.uid
			e.originalBuffSnapshots = append(e.originalBuffSnapshots, combatRoundPendingBuffSnapshot{index: len(e.events) - 1, buff: roundBuff})
		}
		// Original DoDead adds to the chronological list only after Die talents finish.
		e.players[target.camp].deadActors = append(e.players[target.camp].deadActors, target)
	}
	return e.failure
}
