package service

// FActor.MinusHP loads WillDie before consulting the counted state flag.
// AddHp skips later HP/shield changes once this state already exists.
// The original protection is 0.01 HP and keeps skill execution available.
func (e *combatRoundEngine) originalWillDie(target *combatRoundActor) {
	parentScope := e.originalCastScope
	e.originalCastScope = nil
	defer func() { e.originalCastScope = parentScope }()
	if err := e.talentTimer(target, nil, 24); err != nil {
		e.failure = err
		return
	}
	if target.hasState("dieAfterLastAttack") {
		target.hp = .01
		e.trace("last_attack_protected", target, nil, "", "", .01)
	}
}

// ProcMyRoundEnd runs only for the active camp before global Buff.minusRound.
// DoDead bypasses shields, invulnerability, pending attacks, and killer talents.
func (e *combatRoundEngine) originalLastAttackRoundEnd(actor *combatRoundActor) error {
	if actor.camp != e.camp || actor.state >= 3 || !actor.hasState("dieAfterLastAttack") {
		return nil
	}
	var origin *combatRoundBuff
	for _, buff := range actor.buffs {
		if buff.row["Factor"] == "dieAfterLastAttack" {
			origin = buff
			break
		}
	}
	actor.deathSource = nil
	e.trace("last_attack_round_death", actor, nil, "", "", 0)
	e.events[len(e.events)-1].Buff = "buff_196"
	if err := e.completeActorDeath(actor); err != nil {
		return err
	}
	if actor.state == 4 && origin != nil {
		e.trace("buff_dead_export", origin.source, actor, "", "", 0)
		e.events[len(e.events)-1].Buff = origin.row["Id"]
		e.events[len(e.events)-1].OriginalBuffExport = origin.originalExportGroup
		e.events[len(e.events)-1].OriginalBuffOwner = origin.target.uid
		e.originalBuffSnapshots = append(e.originalBuffSnapshots, combatRoundPendingBuffSnapshot{index: len(e.events) - 1, buff: origin})
	}
	return nil
}
