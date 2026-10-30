package service

import (
	"context"
	"encoding/json"
	"errors"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
)

// The fallback always starts the server's automatic driver at round one. Its
// input does not contain the client's commands, reported HP or winner.
func (s *GameService) p7Awake16ServerFallback(ctx context.Context, input *model.FrozenBattleInput) (*model.CombatFinalHPOutcome, error) {
	if _, err := s.p7Awake16GenericProof(ctx, input); err != nil {
		return nil, err
	}
	auto, err := p7Awake16FallbackInput(input)
	if err != nil {
		return nil, err
	}
	// Type 16 has its own admitted Awake driver; the shared PVE wrapper only
	// accepts its separately audited modes.
	out, err := s.computeFrozenBattleFinalHP(ctx, auto, nil, true)
	if err != nil {
		return nil, err
	}
	out.Outcome.Authoritative = true
	out.Outcome.CapabilityHash = constUtil.P7Awake16ServerFallbackProof
	out.Outcome.DiagnosticAIDriverPolicy = constUtil.P6DiagnosticManualAutoStart
	if err = validateP7Awake16ServerFallback(ctx, input, out); err != nil {
		return nil, err
	}
	return out, nil
}

func p7Awake16FallbackInput(input *model.FrozenBattleInput) (*model.FrozenBattleInput, error) {
	auto, err := cloneP6CalculationInput(input)
	if err != nil {
		return nil, err
	}
	auto.DiagnosticAIDriverPolicy = constUtil.P6DiagnosticManualAutoStart
	auto.DiagnosticManualAutoStartRoundTime = proto.Float64(0)
	if err = ValidateP6AIDriverPolicy(auto); err != nil {
		return nil, err
	}
	return auto, nil
}

// Cold reads verify the frozen source and complete server outcome without
// interpreting the defective client recording a second time.
func validateP7Awake16ServerFallback(ctx context.Context, input *model.FrozenBattleInput, out *model.CombatFinalHPOutcome) error {
	if ctx == nil || input == nil {
		return errors.New(constUtil.P6ReplayConflict)
	}
	validator := &GameService{Tables: input.Tables}
	if _, err := validator.p7Awake16GenericProof(ctx, input); err != nil {
		return err
	}
	auto, err := p7Awake16FallbackInput(input)
	if err != nil {
		return err
	}
	if out == nil || out.Outcome == nil || !out.Outcome.Authoritative || out.Outcome.CapabilityHash != constUtil.P7Awake16ServerFallbackProof || out.Outcome.DiagnosticAIDriverPolicy != constUtil.P6DiagnosticManualAutoStart || out.OriginalRecordSHA256 != "" || out.Schema != constUtil.P6FinalActorHPSchema || out.EngineVersion != auto.EngineVersion || out.ClientVersion != auto.ClientVersion || out.ConfigurationHash != auto.ConfigurationHash {
		return errors.New(constUtil.P6ReplayConflict)
	}
	if err = validateP6GenericMechanismTerminal(auto, out); err != nil {
		return err
	}
	for _, actor := range out.FinalActors {
		if actor.RuntimePose != 0 {
			return errors.New(constUtil.P6ReplayConflict)
		}
	}
	closure := *auto
	closure.Tables = nil
	raw, err := json.Marshal(&closure)
	if err != nil || p6Digest(raw) != out.CalculationSourceSHA256 {
		return errors.New(constUtil.P6ReplayConflict)
	}
	return nil
}
