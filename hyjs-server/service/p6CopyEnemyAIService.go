package service

import "sort"

// originalCopyNormalDone follows EngineCopyEnemyAI.IsAllNormalMsgDone: every
// queued normal must have been consumed and every dealt skill must be Done4.
func (e *combatRoundEngine) originalCopyNormalDone() bool {
	if e.commandIndex < len(e.commands) {
		return false
	}
	for _, skill := range e.copyEnemyDealt {
		if skill.state != 4 {
			return false
		}
	}
	return true
}

// stepOriginalCopyEnemyAI keeps original Normal and Ultimate phases separate.
// Update0124..0136 changes phase and returns; ready hero ultimates are selected
// only by the next Ultimate update, after all normal messages complete.
func (e *combatRoundEngine) stepOriginalCopyEnemyAI() {
	if !e.copyEnemyUltimatePhase {
		if e.aiDelay > 0 {
			e.aiDelay -= 0.1
			return
		}
		if e.commandIndex < len(e.commands) {
			command := e.commands[e.commandIndex]
			e.commandIndex++
			for _, skill := range command.skills {
				if skill.state == 0 {
					e.issue(skill)
					// Original DoSkill's return does not prevent the dealt message append.
					e.copyEnemyDealt = append(e.copyEnemyDealt, skill)
				}
			}
			// Original Update00f5/0110 writes literal1.0, independent of UI spacing.
			e.aiDelay = 1
		}
		if e.originalCopyNormalDone() {
			e.copyEnemyUltimatePhase = true
		}
		return
	}
	if e.copyEnemyCurrentUltimate == nil {
		if len(e.copyEnemyUltimateQueue) > 0 {
			e.copyEnemyCurrentUltimate = e.copyEnemyUltimateQueue[0]
			e.copyEnemyUltimateQueue = e.copyEnemyUltimateQueue[1:]
		} else {
			ready := append([]*combatRoundActor(nil), e.players[e.camp].actors...)
			sort.SliceStable(ready, func(i, j int) bool { return ready[i].position() < ready[j].position() })
			for _, actor := range ready {
				if actor.heroClass != 1 && e.canSkill(actor.skill(2)) {
					e.copyEnemyCurrentUltimate = actor.skill(2)
					break
				}
			}
			if e.copyEnemyCurrentUltimate == nil {
				e.copyEnemyFinishNotified = true
				return
			}
		}
	}
	skill := e.copyEnemyCurrentUltimate
	switch skill.state {
	case 0:
		e.issue(skill)
	case 4:
		// Original clears the current message and returns before selecting another.
		e.copyEnemyCurrentUltimate = nil
	}
}

// The legacy complete diagnostic replay supplies both camps' presses itself.
// Keep the same CopyEnemyAI completion phases without issuing a second press.
func (e *combatRoundEngine) stepOriginalCopyEnemyReplayPhase(normalIssued bool) {
	if !e.copyEnemyUltimatePhase {
		if normalIssued {
			return
		}
		if e.aiDelay > 0 {
			e.aiDelay -= 0.1
			return
		}
		if e.originalCopyNormalDone() {
			e.copyEnemyUltimatePhase = true
		}
		return
	}
	if e.copyEnemyCurrentUltimate != nil {
		if e.copyEnemyCurrentUltimate.state == 4 {
			e.copyEnemyCurrentUltimate = nil
		}
		return
	}
	e.copyEnemyFinishNotified = true
}

// Original ComputeCore.SkillEnd checks enabled active/ready normals and emits a
// view hint. The recorded-playback caller accepts this hint for its automatic
// enemy after Update; the automatic caller waits for the AI's own notification.
func (e *combatRoundEngine) originalHasActiveNormalSkill() bool {
	for _, actor := range e.players[e.camp].actors {
		if actor.state >= 3 {
			continue
		}
		for _, skill := range actor.skills {
			if skill.row["IsUltimate"] == "1" {
				continue
			}
			// CopyEnemyAI.Run disables unselected class1 normals before execution.
			if e.usesCopyEnemyAI(actor.camp) && actor.heroClass == 1 {
				enabled := false
				for _, command := range e.commands {
					for _, queued := range command.skills {
						if queued == skill {
							enabled = true
						}
					}
				}
				if !enabled {
					continue
				}
			}
			if skill.state != 0 && skill.state != 4 || e.canSkill(skill) {
				return true
			}
		}
	}
	return false
}
