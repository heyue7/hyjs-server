package service

type combatRoundHPExportOrigin struct {
	source *combatRoundActor
	skill  string
	cast   string
	cursor *int64
}

// Each original FExportOnceData advances independently; audit event chronology
// remains separate from this original value sequence.
func (o combatRoundHPExportOrigin) nextOrder() int64 {
	if o.cursor == nil {
		return 0
	}
	*o.cursor++
	return *o.cursor
}

// Retaliation and shared/bleeding HP values retain the parent FExportOnceData
// source even when the actor actually dealing that damage is different.
func (e *combatRoundEngine) originalExtraHPExport(target *combatRoundActor, nominal float64, visible, shielded bool, hpType int32, origin []combatRoundHPExportOrigin) {
	if !visible || len(origin) == 0 {
		return
	}
	if shielded {
		hpType = 16
	}
	o := origin[0]
	e.trace("hp_export", o.source, target, o.skill, o.cast, nominal)
	e.events[len(e.events)-1].OriginalHPValue = &nominal
	e.events[len(e.events)-1].OriginalHPType = &hpType
	e.events[len(e.events)-1].OriginalValueOrder = o.nextOrder()
}

// Non-HP display values retain their parent original cast source and extra duration.
func (e *combatRoundEngine) originalCastValueExport(target *combatRoundActor, class int32, value, extra float64, origin combatRoundHPExportOrigin) {
	e.trace("value_export", origin.source, target, origin.skill, origin.cast, value)
	e.events[len(e.events)-1].OriginalValueClass = class
	e.events[len(e.events)-1].OriginalExtraValue = extra
	e.events[len(e.events)-1].OriginalValueOrder = origin.nextOrder()
}

func (e *combatRoundEngine) originalCastAnger(target *combatRoundActor, value float64, origin combatRoundHPExportOrigin) {
	visible := value != 0 && (value < 0 || value*(1+target.attribute("angerRate")*.01) > 0)
	before := e.players[target.camp].anger
	e.addAnger(target, value)
	if visible {
		e.originalCastValueExport(target, 2, e.players[target.camp].anger-before, 0, origin)
	}
}
