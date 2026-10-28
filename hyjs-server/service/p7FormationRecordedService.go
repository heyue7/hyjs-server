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

const p7FormationRecordedDriver = "original-pve-live-recorded-round1-formation-strict-v1"
const p7FormationRecordedProof = "0c6e61f38312234462b5391a855956024b430f0f840dca5c788faddfb74f6637"

func validateP7FormationRecordedInput(input *model.FrozenBattleInput, record *pb.BattleOperate) error {
	if input == nil || (input.BattleType != 9 && input.BattleType != 10) || record == nil || record.AttackerTransOpt == nil {
		return errors.New(constUtil.CombatUnsupported)
	}
	if err := validateP6RoundOneLiveAutoFormation(input, record, record.AttackerTransOpt); err != nil {
		return err
	}
	// Keep the existing formation-free driver unchanged. Only this named source
	// admits one complete original round-one formation and live player commands.
	view := proto.Clone(record).(*pb.BattleOperate)
	view.AttackerTransOpt = nil
	return validateP6LiveRecordedInput(input, view)
}
func p7FormationRecordedSource(input *model.FrozenBattleInput, record *pb.BattleOperate) (string, string, error) {
	if err := validateP7FormationRecordedInput(input, record); err != nil {
		return "", "", err
	}
	view := *input
	view.Tables = nil
	frozen, err := json.Marshal(&view)
	if err != nil {
		return "", "", err
	}
	raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(record)
	if err != nil {
		return "", "", err
	}
	hash := p6Digest(raw)
	bound, err := json.Marshal([]string{p6Digest(frozen), hash, p7FormationRecordedDriver, p7FormationRecordedProof})
	if err != nil {
		return "", "", err
	}
	return hash, p6Digest(bound), nil
}
func p7FormationRecordedComputeEnd(ctx context.Context, engine *GameService, series *model.FrozenP6RushSeries, record *pb.BattleOperate, reported int32) (*model.CombatFinalHPOutcome, error) {
	input, err := p7GenericMechanismRushCalculationInput(series, record)
	if err != nil {
		return nil, err
	}
	if err = validateP7FormationRecordedInput(input, record); err != nil {
		return nil, err
	}
	if _, err = engine.ValidateP6GenericMechanisms(ctx, input); err != nil {
		return nil, err
	}
	out, err := engine.computeFrozenBattleFinalHPLiveControlled(ctx, input, record, true, nil, nil, true)
	if err != nil {
		return nil, err
	}
	if !proto.Equal(record, out.OriginalOperates) {
		return nil, errors.New(constUtil.P6ReplayConflict)
	}
	if err = validateP6GenericMechanismTerminal(input, out); err != nil {
		return nil, err
	}
	out.OriginalRecordSHA256, out.CalculationSourceSHA256, err = p7FormationRecordedSource(input, record)
	if err != nil {
		return nil, err
	}
	out.Outcome.DiagnosticAIDriverPolicy = p7FormationRecordedDriver
	out.Outcome.CapabilityHash = p7FormationRecordedProof
	out.Outcome.Authoritative = true
	return out, p7FormationRecordedValidateEnd(series, out, record, reported)
}
func p7FormationRecordedValidateEnd(series *model.FrozenP6RushSeries, out *model.CombatFinalHPOutcome, record *pb.BattleOperate, reported int32) error {
	if series == nil {
		return errors.New(constUtil.P6ReplayConflict)
	}
	input := series.Template
	hash, source, err := p7FormationRecordedSource(input, record)
	if err != nil {
		return err
	}
	if out == nil || out.Outcome == nil || !out.Outcome.Authoritative || out.Outcome.DiagnosticAIDriverPolicy != p7FormationRecordedDriver || out.Outcome.CapabilityHash != p7FormationRecordedProof || out.Schema != constUtil.P6FinalActorHPSchema || out.EngineVersion != input.EngineVersion || out.ClientVersion != input.ClientVersion || out.ConfigurationHash != input.ConfigurationHash || out.OriginalRecordSHA256 != hash || out.CalculationSourceSHA256 != source || !proto.Equal(record, out.OriginalOperates) {
		return errors.New(constUtil.P6ReplayConflict)
	}
	if err = validateP6GenericMechanismTerminal(input, out); err != nil {
		return err
	}
	expected := int32(0)
	if out.Outcome.Winner == 0 {
		expected = 1
	}
	if reported != expected {
		return errors.New(constUtil.P6ReplayConflict)
	}
	return nil
}
