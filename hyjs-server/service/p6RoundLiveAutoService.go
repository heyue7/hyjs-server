package service

import (
	"context"
	"encoding/json"
	"errors"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Each control is one original SetAuto/Run candidate, never a skill playback queue.
type p6RoundAutoControl struct {
	RoundNum  int32   `json:"roundNum"`
	RoundTime float64 `json:"roundTime"`
}

func validateP6RoundLiveAutoControls(input *model.FrozenBattleInput, controls []p6RoundAutoControl) error {
	if input == nil || (input.BattleType != 9 && input.BattleType != 10) || input.Attacker == nil || input.Monster == nil || input.Defender != nil || input.DefenderFirst || input.IsAttToDef || input.KFReplay != nil || input.KFReplayViewer != nil || len(input.WinConditions) != 0 || input.PVECompatibilityRule != "" || input.DiagnosticAIDriverPolicy != "" || input.DiagnosticManualAutoStartRoundTime != nil || len(controls) == 0 || len(controls) > int((input.MaxRound+1)/2) {
		return errors.New(constUtil.CombatUnsupported)
	}
	for i, control := range controls {
		if control.RoundNum != int32(2*i+1) || control.RoundNum > input.MaxRound || !combatFinite(control.RoundTime) || control.RoundTime < 0 || control.RoundTime > 110 {
			return errors.New(constUtil.P6ReplayConflict)
		}
	}
	return nil
}

func (e *combatRoundEngine) applyRoundLiveAutoControl() error {
	if len(e.roundLiveAutoControls) == 0 || e.camp != 0 || e.roundLiveAutoStarted {
		return nil
	}
	index := int((e.round - 1) / 2)
	if index >= len(e.roundLiveAutoControls) || e.roundLiveAutoControls[index].RoundNum != e.round {
		return errors.New(constUtil.P6ReplayConflict)
	}
	if e.time < e.roundLiveAutoControls[index].RoundTime {
		return nil
	}
	e.roundLiveAutoStarted = true
	return e.buildOriginalRoundAICommands()
}

func p6RoundLiveAutoSourceDigests(input *model.FrozenBattleInput, record *pb.BattleOperate, controls []p6RoundAutoControl) (string, string, error) {
	if err := validateP6RoundLiveAutoControls(input, controls); err != nil {
		return "", "", err
	}
	closure := *input
	closure.Tables = nil
	frozen, err := json.Marshal(&closure)
	if err != nil {
		return "", "", err
	}
	actual, err := proto.MarshalOptions{Deterministic: true}.Marshal(record)
	if err != nil {
		return "", "", err
	}
	schedule, err := json.Marshal(controls)
	if err != nil {
		return "", "", err
	}
	recordHash := p6Digest(actual)
	envelope, err := json.Marshal(struct {
		FrozenInput    string `json:"frozenInput"`
		OriginalRecord string `json:"originalRecord"`
		Driver         string `json:"driver"`
		Controls       string `json:"controls"`
	}{p6Digest(frozen), recordHash, constUtil.P6RoundLiveAutoDriver, p6Digest(schedule)})
	if err != nil {
		return "", "", err
	}
	return recordHash, p6Digest(envelope), nil
}

func (s *GameService) computeP6RoundLiveAuto(ctx context.Context, input *model.FrozenBattleInput, expected *pb.BattleOperate, controls []p6RoundAutoControl) (*model.CombatFinalHPOutcome, error) {
	if err := validateP6RoundLiveAutoControls(input, controls); err != nil {
		return nil, err
	}
	if _, err := s.ValidateP6GenericMechanisms(ctx, input); err != nil {
		return nil, err
	}
	// No supplied record reaches the engine. It runs live PlayerAI and independent NPC AI.
	out, err := s.computeFrozenBattleFinalHPRoundControlled(ctx, input, nil, true, nil, controls)
	if err != nil {
		return nil, err
	}
	if !proto.Equal(expected, out.OriginalOperates) {
		return nil, errors.New(constUtil.P6ReplayConflict)
	}
	if err = validateP6GenericMechanismTerminal(input, out); err != nil {
		return nil, err
	}
	out.OriginalRecordSHA256, out.CalculationSourceSHA256, err = p6RoundLiveAutoSourceDigests(input, expected, controls)
	if err != nil {
		return nil, err
	}
	out.Outcome.DiagnosticAIDriverPolicy = constUtil.P6RoundLiveAutoDriver
	return out, nil
}
