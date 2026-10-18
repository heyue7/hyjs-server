package service

// Native recover always uses the origin's physical bonus, including a magical source.
func (e *combatRoundEngine) originalRecover(source, target *combatRoundActor, row map[string]string, buffs ...*combatRoundBuff) {
	start := len(e.events)
	heal := (source.attribute("att")+source.attribute("patt"))*tableNumber(row, "Args1")*.01*(1+float64(source.attribute("treatAdd")*.01)) + source.attribute("cure") + target.attribute("heal")
	e.originalBuffChangeHP(source, target, heal, "buff_heal", 11, buffs...)
	if len(buffs) > 0 && buffs[0] != nil {
		visible := false
		for _, event := range e.events[start:] {
			if event.Kind == "buff_hp_export" && event.OriginalBuffExport == buffs[0].originalExportGroup {
				visible = true
				break
			}
		}
		if !visible {
			e.originalEmptyBuffExport(buffs[0])
		}
	}
}

// Native doHpChangeInRound accumulates signed nonlethal MinusHP and nominal healing
// after poison/minimum adjustment. Shield-only, lethal and forbidden recovery skip it.
func (e *combatRoundEngine) originalHPChangeInRound(target *combatRoundActor, change float64) {
	parent := e.originalStandaloneLoadParent
	group := e.originalStandaloneLoadGroup
	e.originalStandaloneLoadParent = nil
	e.originalStandaloneLoadGroup = 0
	defer func() { e.originalStandaloneLoadParent = parent; e.originalStandaloneLoadGroup = group }()
	parentScope := e.originalCastScope
	e.originalCastScope = nil
	defer func() { e.originalCastScope = parentScope }()
	target.hpChangeInRound += change
	if e.camp == 1-target.camp && target.hpChangeInRound/target.attribute("maxHp") <= -.4 {
		// LoadBuffToTargetByTimer uses the injured actor directly. It checks each condition
		// before LoadTalentBuff and preserves formation/talent/slot order without selection.
		for _, owner := range e.players[target.camp].actors {
			if owner.state == 4 || owner.uid == target.uid {
				continue
			}
			currentTalent := ""
			for _, trigger := range owner.talents {
				if trigger.timer != 19 || !e.conditionTrue(trigger, owner, target) {
					continue
				}
				if currentTalent != trigger.origin.talent {
					currentTalent = trigger.origin.talent
					e.originalNewStandaloneLoadGroup()
				}
				trigger.condition = 0
				if err := e.applyTrigger(owner, target, trigger, []*combatRoundActor{target}); err != nil {
					e.failure = err
					return
				}
			}
		}
	}
	if change > 0 {
		// Positive recovery also calls EnemyAddHp on every living, non-weak enemy actor.
		for _, actor := range e.players[1-target.camp].actors {
			if actor.state == 3 || actor.state == 4 {
				continue
			}
			if err := e.talentTimer(actor, nil, 34); err != nil {
				e.failure = err
				return
			}
		}
	}
}
