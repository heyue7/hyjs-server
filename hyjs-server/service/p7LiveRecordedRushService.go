package service

import (
	"context"
	"errors"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p7LiveRecordedRushComputeEnd(ctx context.Context, engine *GameService, series *model.FrozenP6RushSeries, record *pb.BattleOperate, reported int32) (*model.CombatFinalHPOutcome, error) {
	input, err := p7GenericMechanismRushCalculationInput(series, record)
	if err != nil {
		return nil, err
	}
	out, err := engine.computeP6LiveRecorded(ctx, input, record)
	if err != nil {
		return nil, err
	}
	out.Outcome.Authoritative = true
	out.Outcome.CapabilityHash = constUtil.P7LiveRecordedRushProof
	if err = p7GenericMechanismRushValidateEnd(series, out, record, reported); err != nil {
		return nil, err
	}
	return out, nil
}

func p7LiveRecordedRushValidateEnd(series *model.FrozenP6RushSeries, out *model.CombatFinalHPOutcome, record *pb.BattleOperate, reported int32) error {
	input := series.Template
	reject := func() error { return errors.New(constUtil.P6ReplayConflict) }
	if err := validateP6LiveRecordedInput(input, record); err != nil {
		return err
	}
	if out == nil || out.Outcome == nil || !out.Outcome.Authoritative || out.Outcome.DiagnosticAIDriverPolicy != constUtil.P6LiveRecordedDriver || out.Outcome.CapabilityHash != constUtil.P7LiveRecordedRushProof || out.Schema != constUtil.P6FinalActorHPSchema || out.EngineVersion != input.EngineVersion || out.ClientVersion != input.ClientVersion || out.ConfigurationHash != input.ConfigurationHash || !proto.Equal(record, out.OriginalOperates) {
		return reject()
	}
	recordHash, sourceHash, err := p6LiveRecordedSourceDigests(input, record)
	if err != nil || recordHash != out.OriginalRecordSHA256 || sourceHash != out.CalculationSourceSHA256 {
		return reject()
	}
	if err = validateP6GenericMechanismTerminal(input, out); err != nil {
		return err
	}
	for _, actor := range out.FinalActors {
		if actor.RuntimePose != 0 {
			return reject()
		}
	}
	expected := int32(0)
	if out.Outcome.Winner == 0 {
		expected = 1
	}
	if reported != expected {
		return reject()
	}
	return nil
}
