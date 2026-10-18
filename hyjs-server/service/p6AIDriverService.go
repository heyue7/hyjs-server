package service

import (
	"errors"

	"hyjs-server/constUtil"
	"hyjs-server/model"
)

// originalP6CopyEnemyType follows BattleManager.SetFightEngineData0057..02ab.
// Fighter payload shape does not select AI: UF14 Monster still uses PlayerAI.
func originalP6CopyEnemyType(battleType int32) bool {
	switch battleType {
	case 0, 1, 4, 6, 7, 8, 9, 10, 13, 16:
		return true
	default:
		return false
	}
}

// ValidateP6AIDriverPolicy keeps the old explicitly observed diagnostic driver
// separate from the current original mode wrapper. It never enables authority.
func ValidateP6AIDriverPolicy(input *model.FrozenBattleInput) error {
	if input == nil {
		return errors.New(constUtil.CombatInvalid)
	}
	if input.DiagnosticAIDriverPolicy == constUtil.P6DiagnosticManualAutoStart || input.DiagnosticManualAutoStartRoundTime != nil {
		if input.DiagnosticAIDriverPolicy != constUtil.P6DiagnosticManualAutoStart || input.DiagnosticManualAutoStartRoundTime == nil || !combatFinite(*input.DiagnosticManualAutoStartRoundTime) || *input.DiagnosticManualAutoStartRoundTime < 0 || *input.DiagnosticManualAutoStartRoundTime > 110 || (input.BattleType != 9 && input.BattleType != 10 && input.BattleType != 16) || input.Monster == nil || input.Defender != nil || input.KFReplay != nil || input.PVECompatibilityRule != "" || input.IsAttToDef || input.DefenderFirst {
			return errors.New(constUtil.CombatUnsupported)
		}
		return nil
	}
	if input.DiagnosticAIDriverPolicy == "" {
		return nil
	}
	if input.DiagnosticAIDriverPolicy != constUtil.P6DiagnosticUF14CopyAI || input.BattleType != 14 || input.Monster == nil || input.Defender != nil || input.KFReplay != nil || input.PVECompatibilityRule != "" {
		return errors.New(constUtil.CombatUnsupported)
	}
	return nil
}

func (e *combatRoundEngine) usesCopyEnemyAI(camp int32) bool {
	return camp == 1 && (originalP6CopyEnemyType(e.input.BattleType) || e.input.DiagnosticAIDriverPolicy == constUtil.P6DiagnosticUF14CopyAI)
}

// A manual player leaves its original AI unrun until the observed Auto control.
// The engine's existing ordered actor/buff ticks and repeated time additions
// remain active; this never offsets emitted actions or rewrites a source record.
func (e *combatRoundEngine) originalManualAutoWaiting() bool {
	return e.input.DiagnosticAIDriverPolicy == constUtil.P6DiagnosticManualAutoStart && e.input.DiagnosticManualAutoStartRoundTime != nil && e.round == 1 && e.camp == 0 && e.time < *e.input.DiagnosticManualAutoStartRoundTime
}
