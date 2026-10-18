package service

// Each original Buff HP export owns its Buff identity and origin independently
// from the cast that loaded it. Existing calculation events retain their audit kind.
func (e *combatRoundEngine) originalBuffHPExport(source, target *combatRoundActor, nominal float64, visible, shielded bool, hpType int32, buffs []*combatRoundBuff) {
	if !visible || len(buffs) == 0 || buffs[0] == nil {
		return
	}
	if shielded {
		hpType = 16
	}
	b := buffs[0]
	e.trace("buff_hp_export", source, target, "", "", nominal)
	event := &e.events[len(e.events)-1]
	event.Buff = b.row["Id"]
	event.OriginalBuffExport = b.originalExportGroup
	event.OriginalBuffOwner = b.target.uid
	event.OriginalHPValue = &nominal
	event.OriginalHPType = &hpType
	e.originalBuffSnapshots = append(e.originalBuffSnapshots, combatRoundPendingBuffSnapshot{index: len(e.events) - 1, buff: b})
}
func (e *combatRoundEngine) originalBuffChangeHP(source, target *combatRoundActor, change float64, kind string, hpType int32, buffs ...*combatRoundBuff) float64 {
	valueParent := e.originalBuffValueDeath
	if len(buffs) > 0 && buffs[0] != nil && e.originalRoundBuffDeath == nil {
		e.originalBuffValueDeath = buffs[0]
	}
	defer func() { e.originalBuffValueDeath = valueParent }()
	shielded := change < 0 && target.attribute("shield") > 0
	if change > 0 && target.hasState("forbidAddHp") {
		hpType = 6
	}
	actual, nominal, visible := e.changeHPWithOriginalExport(source, target, change, kind)
	e.originalBuffHPExport(source, target, nominal, visible, shielded && nominal == 0, hpType, buffs)
	return actual
}
func (e *combatRoundEngine) originalBuffDamageHP(source, target *combatRoundActor, damage float64, typ, hpType int32, buffs ...*combatRoundBuff) {
	valueParent := e.originalBuffValueDeath
	if len(buffs) > 0 && buffs[0] != nil && e.originalRoundBuffDeath == nil {
		e.originalBuffValueDeath = buffs[0]
	}
	defer func() { e.originalBuffValueDeath = valueParent }()
	nominal, visible, shielded := e.damageHP(source, target, damage, typ)
	e.originalBuffHPExport(source, target, nominal, visible, shielded, hpType, buffs)
}

// AP values stay attached to the original Buff-owned FExportOnceData source.
func (e *combatRoundEngine) originalBuffAnger(source, target *combatRoundActor, value float64, buffs ...*combatRoundBuff) {
	visible := value != 0 && (value < 0 || value*(1+target.attribute("angerRate")*.01) > 0)
	before := e.players[target.camp].anger
	e.addAnger(target, value)
	if !visible || len(buffs) == 0 || buffs[0] == nil {
		return
	}
	e.trace("buff_value_export", source, target, "", "", e.players[target.camp].anger-before)
	event := &e.events[len(e.events)-1]
	event.Buff = buffs[0].row["Id"]
	event.OriginalBuffExport = buffs[0].originalExportGroup
	event.OriginalBuffOwner = buffs[0].target.uid
	event.OriginalValueClass = 2
	e.originalBuffSnapshots = append(e.originalBuffSnapshots, combatRoundPendingBuffSnapshot{index: len(e.events) - 1, buff: buffs[0]})
}
