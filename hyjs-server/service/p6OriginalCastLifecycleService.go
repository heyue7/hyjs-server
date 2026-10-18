package service

import "hyjs-server/model"

// Original FBuffManager.LoadBuff records persistent identities after Effect;
// one-use and class5 helpers are excluded from the caller's loadBuffs list.
func (e *combatRoundEngine) originalCastLoadedBuff(b *combatRoundBuff) {
	if b.once || int(tableNumber(b.row, "BuffClass"))%10 == 5 || e.originalCastScope == nil && !e.originalExportsStarted {
		return
	}
	event := &e.events[len(e.events)-1]
	event.OriginalStandaloneLoadGroup = e.originalStandaloneLoadGroup
	if e.originalCastScope != nil {
		event.OriginalExportSource = e.originalCastScope.source.uid
		event.OriginalExportSkill = e.originalCastScope.skill
		event.OriginalExportCast = e.originalCastScope.cast
	}
	event.OriginalBuffTalent = b.origin.talent
	event.OriginalBuffSkill = b.origin.skill
	event.OriginalBuffCast = b.origin.cast
	e.originalBuffSnapshots = append(e.originalBuffSnapshots, combatRoundPendingBuffSnapshot{index: len(e.events) - 1, buff: b, parent: e.originalStandaloneLoadParent})
}
func (e *combatRoundEngine) originalCastDeadActor(target *combatRoundActor) {
	if e.originalCastScope == nil {
		return
	}
	e.trace("cast_dead_export", e.originalCastScope.source, target, e.originalCastScope.skill, e.originalCastScope.cast, 0)
}

type combatRoundPendingBuffSnapshot struct {
	index  int
	buff   *combatRoundBuff
	parent *combatRoundBuff
}

// Original callback serialization observes Buff objects after all actors have
// updated. Freeze those fields at completed Tick, preserving transient objects.
func (e *combatRoundEngine) originalFinalizeBuffSnapshots() {
	for _, pending := range e.originalBuffSnapshots {
		e.events[pending.index].OriginalBuffSnapshot = originalBuffMetadataSnapshot(pending.buff)
		e.events[pending.index].OriginalBuffInstance = pending.buff.originalExportGroup
		if pending.parent != nil {
			e.events[pending.index].OriginalBuffParentSnapshot = originalBuffMetadataSnapshot(pending.parent)
			e.events[pending.index].OriginalBuffParentInstance = pending.parent.originalExportGroup
		}
	}
	e.originalBuffSnapshots = nil
}

// UnLoadBuff00c0 creates a standalone Buff export only for persistent objects.
func (e *combatRoundEngine) originalBuffUnloaded(b *combatRoundBuff) {
	if b.once || !e.originalExportsStarted && e.originalCastScope == nil {
		return
	}
	event := &e.events[len(e.events)-1]
	event.OriginalBuffExport = b.originalExportGroup
	event.OriginalBuffOwner = b.target.uid
	e.originalBuffSnapshots = append(e.originalBuffSnapshots, combatRoundPendingBuffSnapshot{index: len(e.events) - 1, buff: b})
}

func originalBuffMetadataSnapshot(b *combatRoundBuff) *model.CombatBuffExportSnapshot {
	class := int32(tableNumber(b.row, "BuffClass"))
	val, val2 := 0.0, 0.0
	if p6HPTransferKey(b.row["Factor"]) != "" {
		val = b.initialValue * .01
	} else if class%10 == 1 {
		val = b.initialValue
	} else {
		switch b.row["Factor"] {
		case "angerAndhp", "poisonDam", "fireDam", "recoverBySelfRound", "reliveAndinvisible", "permaxHpBySelfRound", "perhp", "permaxHp", "minusMaxHpPercent":
			val = b.initialValue
		case "iceYNT":
			val = tableNumber(parseP6Condition(b.row["Param"]), "damageAdd") * .01
			val2 = tableNumber(parseP6Condition(b.row["Param"]), "treatAdd") * .01
		}
	}
	return &model.CombatBuffExportSnapshot{ID: b.row["Id"], Factor: b.row["Factor"], Args1: tableNumber(b.row, "Args1"), Args2: tableNumber(b.row, "Args2"), Args3: tableNumber(b.row, "Args3"), BuffClass: class % 10, BuffFlag: class / 10, Group: b.row["Group"], IsDebuff: b.row["IsDebuff"] == "1", CanBeCleared: b.row["CanBeCleared"] == "1", OnceData: b.once, ShouldCover: b.row["ShouldCover"] == "1", Param: b.row["Param"], TotalCount: tableNumber(b.row, "TotalCount"), TotalTime: tableNumber(b.row, "TotalTime"), TotalRound: b.rounds, AttCount: b.attacks, DefCount: b.defenses, Val: val, Val2: val2, LoadBuffRound: b.loadedRound, LoadBuffTime: b.loaded, Timer: b.timer, Target: b.target.uid, Source: b.source.uid, Talent: b.origin.talent, Skill: b.origin.skill, Cast: b.origin.cast}
}

func (e *combatRoundEngine) originalNewStandaloneLoadGroup() {
	e.originalStandaloneLoadSequence++
	e.originalStandaloneLoadGroup = e.originalStandaloneLoadSequence
}

// Original delayed Effect appends its FExport even when no persistent child survives.
func (e *combatRoundEngine) originalEmptyBuffExport(b *combatRoundBuff) {
	if b == nil || !e.originalExportsStarted {
		return
	}
	e.trace("buff_empty_export", b.source, nil, "", "", 0)
	event := &e.events[len(e.events)-1]
	event.Buff = b.row["Id"]
	event.OriginalBuffExport = b.originalExportGroup
	event.OriginalBuffOwner = b.target.uid
	e.originalBuffSnapshots = append(e.originalBuffSnapshots, combatRoundPendingBuffSnapshot{index: len(e.events) - 1, buff: b})
}

// Original LoadBuffByTimer/DelayEffect appends its group after all child Effect
// calls, including immediate one-use numerical exports after the last load.
func (e *combatRoundEngine) originalCompleteStandaloneLoadGroup(group int64) {
	if group <= 0 || e.originalCastScope != nil {
		return
	}
	for _, pending := range e.originalBuffSnapshots {
		loaded := e.events[pending.index]
		if loaded.Kind == "buff_load" && loaded.OriginalStandaloneLoadGroup == group && loaded.OriginalExportCast == "" {
			e.trace("standalone_load_export", pending.buff.source, nil, "", "", 0)
			event := &e.events[len(e.events)-1]
			event.OriginalStandaloneLoadGroup = group
			event.OriginalBuffTalent = loaded.OriginalBuffTalent
			return
		}
	}
}
