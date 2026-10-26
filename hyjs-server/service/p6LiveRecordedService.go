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

func validateP6LiveRecordedInput(input *model.FrozenBattleInput, record *pb.BattleOperate) error {
	if input == nil || (input.BattleType != 9 && input.BattleType != 10) || input.Attacker == nil || input.Monster == nil || input.Defender != nil || input.DefenderFirst || input.IsAttToDef || input.KFReplay != nil || input.KFReplayViewer != nil || len(input.WinConditions) != 0 || input.PVECompatibilityRule != "" || input.DiagnosticAIDriverPolicy != "" || input.DiagnosticManualAutoStartRoundTime != nil {
		return errors.New(constUtil.CombatUnsupported)
	}
	if record == nil || record.AttackerTransOpt != nil || record.DefenderTransOpt != nil || len(record.SkillOpts) == 0 || len(record.SkillOpts) > 16000 || p6ScopedUnknown(record) {
		return errors.New(constUtil.P6ReplayConflict)
	}
	return nil
}

func p6LiveRecordedSourceDigests(input *model.FrozenBattleInput, record *pb.BattleOperate) (string, string, error) {
	if err := validateP6LiveRecordedInput(input, record); err != nil {
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
	recordHash := p6Digest(actual)
	envelope, err := json.Marshal(struct {
		FrozenInput    string `json:"frozenInput"`
		OriginalRecord string `json:"originalRecord"`
		Driver         string `json:"driver"`
		Proof          string `json:"proof"`
	}{p6Digest(frozen), recordHash, constUtil.P6LiveRecordedDriver, constUtil.P7LiveRecordedRushProof})
	if err != nil {
		return "", "", err
	}
	return recordHash, p6Digest(envelope), nil
}

// Only the original recorded player camp is supplied. NPC decisions and every
// skill legality/anger/damage calculation remain independent; all resulting PB
// fields, including NPC commands and original clocks, must match exactly.
func (s *GameService) computeP6LiveRecorded(ctx context.Context, input *model.FrozenBattleInput, expected *pb.BattleOperate) (*model.CombatFinalHPOutcome, error) {
	if err := validateP6LiveRecordedInput(input, expected); err != nil {
		return nil, err
	}
	if _, err := s.ValidateP6GenericMechanisms(ctx, input); err != nil {
		return nil, err
	}
	out, err := s.computeFrozenBattleFinalHPLiveControlled(ctx, input, expected, true, nil, nil, true)
	if err != nil {
		return nil, err
	}
	if !proto.Equal(expected, out.OriginalOperates) {
		return nil, errors.New(constUtil.P6ReplayConflict)
	}
	if err = validateP6GenericMechanismTerminal(input, out); err != nil {
		return nil, err
	}
	out.OriginalRecordSHA256, out.CalculationSourceSHA256, err = p6LiveRecordedSourceDigests(input, expected)
	if err != nil {
		return nil, err
	}
	out.Outcome.DiagnosticAIDriverPolicy = constUtil.P6LiveRecordedDriver
	return out, nil
}
