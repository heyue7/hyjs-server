package service

import (
	"errors"

	"hyjs-server/constUtil"
	"hyjs-server/model"
)

type combatRoundNoTargetPress struct {
	round int32
	time  float64
	camp  int32
}

// ValidateP6PVECompatibility reserves the named controller rule for original
// monster PVE inputs. Empty policy retains the original automatic-AI contract.
func ValidateP6PVECompatibility(input model.FrozenBattleInput) error {
	if input.PVECompatibilityRule == "" {
		return nil
	}
	if input.PVECompatibilityRule != constUtil.P6LocalPVENoTargetFinish || input.Monster == nil || input.Defender != nil || input.KFReplay != nil {
		return errors.New(constUtil.CombatInvalid)
	}
	switch input.BattleType {
	case 1, 4, 6, 8, 9, 10, 16:
		return nil
	default:
		return errors.New(constUtil.CombatInvalid)
	}
}

func (e *combatRoundEngine) recordPVENoTargetPress(skill *combatRoundSkill) {
	if e.input.Monster == nil || e.camp != 1 || skill.state != 0 || skill.target != nil || skill.row["IsUltimate"] == "1" {
		return
	}
	opponents := e.players[0].actors
	if len(opponents) == 0 {
		return
	}
	for _, actor := range opponents {
		if actor.state != 3 && actor.state != 4 {
			return
		}
	}
	if e.pveNoTargetPresses == nil {
		e.pveNoTargetPresses = map[*combatRoundSkill]combatRoundNoTargetPress{}
	}
	e.pveNoTargetPresses[skill] = combatRoundNoTargetPress{round: e.round, time: e.time, camp: e.camp}
}

// validateOriginalCopyEnemyRoundFinish mirrors IsAllNormalMsgDone's state4
// requirement. An incomplete original AI never manufactures an end result.
// The opt-in controller rule requires actual same-round no-target presses and
// already-empty queues; it records its public round-over action separately.
func (e *combatRoundEngine) validateOriginalCopyEnemyRoundFinish() error {
	if e.input.Monster == nil || e.camp != 1 {
		return nil
	}
	var failed []*combatRoundSkill
	for _, skill := range e.copyEnemyDealt {
		if skill.state == 4 {
			continue
		}
		proof, proven := e.pveNoTargetPresses[skill]
		if e.input.PVECompatibilityRule != constUtil.P6LocalPVENoTargetFinish || skill.state != 0 || skill.target != nil || !proven || proof.round != e.round || proof.camp != e.camp || skill.actor.current != nil || len(skill.actor.waiting) != 0 {
			return errors.New(constUtil.P6OriginalAINoTargetStall)
		}
		failed = append(failed, skill)
	}
	if len(failed) == 0 {
		return nil
	}
	if e.anyRunning() || e.pendingUltimate != nil || e.commandIndex < len(e.commands) && !e.originalUsesSuppliedCamp() {
		return errors.New(constUtil.P6OriginalAINoTargetStall)
	}
	for _, actor := range e.players[e.camp].actors {
		for _, skill := range actor.skills {
			if skill.state == 1 || skill.state == 2 || skill.state == 3 {
				return errors.New(constUtil.P6OriginalAINoTargetStall)
			}
		}
	}
	for _, skill := range failed {
		// The event marks a controller action after original Update, not a
		// synthetic skill completion. All original data and RNG stay intact.
		e.trace("compat_no_target_finish", skill.actor, nil, skill.id, "", e.pveNoTargetPresses[skill].time)
		// Core.DoUpdate advances the original public roundTime after the actor
		// Tick. The controller action runs after Update returns at that boundary.
		e.events[len(e.events)-1].Time = e.time + 0.1
	}
	return nil
}
