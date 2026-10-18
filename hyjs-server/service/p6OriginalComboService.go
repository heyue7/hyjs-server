package service

// Original combo counts every successful target, including healing and matching advance states.
func (e *combatRoundEngine) originalAddCombo(source, target *combatRoundActor, advance int32) {
	count := int32(1)
	if advance != 0 && advance == target.speState {
		count++
	}
	source.selfCombo += count
	p := e.players[source.camp]
	p.combo += count
	p.lastCombo = e.time
}

func (e *combatRoundEngine) originalBreakCombo(player *combatRoundPlayer) {
	player.lastCombo = e.time
	if player.combo == 0 {
		return
	}
	player.combo = 0
	for _, actor := range player.actors {
		actor.selfCombo = 0
	}
}

func (e *combatRoundEngine) originalMaybeBreakCombo(player *combatRoundPlayer) {
	if e.time-player.lastCombo < e.number("comboBreakTime") {
		return
	}
	// FPlayer.Update ignores an ultimate that is queued but has not begun.
	// A normal skill ending this tick can therefore break an expired combo.
	for _, actor := range player.actors {
		if actor.current != nil {
			return
		}
	}
	e.originalBreakCombo(player)
}
