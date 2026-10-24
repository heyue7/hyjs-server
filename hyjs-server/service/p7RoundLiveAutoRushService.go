package service

import (
	"context"
	"errors"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p7RoundLiveAutoControls(series *model.FrozenP6RushSeries, record *pb.BattleOperate) ([]p6RoundAutoControl, error) {
	if err := p7GenericMechanismRushValidateControls(series, record); err != nil {
		return nil, err
	}
	if record.AttackerTransOpt != nil || record.DefenderTransOpt != nil || record.GetFinalRoundNum() < 1 || record.GetFinalRoundNum() > series.Template.MaxRound+1 {
		return nil, errors.New(constUtil.P6ReplayConflict)
	}
	var controls []p6RoundAutoControl
	for _, op := range record.SkillOpts {
		if op.GetCamp() != 0 {
			continue
		}
		if op.GetRoundNum()%2 != 1 {
			return nil, errors.New(constUtil.P6ReplayConflict)
		}
		if len(controls) > 0 && controls[len(controls)-1].RoundNum == op.GetRoundNum() {
			continue
		}
		controls = append(controls, p6RoundAutoControl{op.GetRoundNum(), op.GetRoundTime()})
	}
	maximum := min(record.GetFinalRoundNum(), series.Template.MaxRound)
	if len(controls) != int((maximum+1)/2) {
		return nil, errors.New(constUtil.P6ReplayConflict)
	}
	if err := validateP6RoundLiveAutoControls(series.Template, controls); err != nil {
		return nil, err
	}
	return controls, nil
}

func p7RoundLiveAutoRushComputeEnd(ctx context.Context, engine *GameService, series *model.FrozenP6RushSeries, record *pb.BattleOperate, reported int32) (*model.CombatFinalHPOutcome, error) {
	controls, err := p7RoundLiveAutoControls(series, record)
	if err != nil {
		return nil, err
	}
	out, err := engine.computeP6RoundLiveAuto(ctx, series.Template, record, controls)
	if err != nil {
		return nil, err
	}
	out.Outcome.Authoritative = true
	out.Outcome.CapabilityHash = constUtil.P7RoundLiveAutoProof
	if err = p7GenericMechanismRushValidateEnd(series, out, record, reported); err != nil {
		return nil, err
	}
	return out, nil
}

// New receipts have their own driver/capability and digest domain. Historical
// manual, single-Auto and formation receipt paths retain their existing hashes.
func p7RoundLiveAutoRushValidateEnd(series *model.FrozenP6RushSeries, out *model.CombatFinalHPOutcome, record *pb.BattleOperate, reported int32) error {
	controls, err := p7RoundLiveAutoControls(series, record)
	if err != nil {
		return err
	}
	input := series.Template
	reject := func() error { return errors.New(constUtil.P6ReplayConflict) }
	if out == nil || out.Outcome == nil || !out.Outcome.Authoritative || out.Outcome.DiagnosticAIDriverPolicy != constUtil.P6RoundLiveAutoDriver || out.Outcome.CapabilityHash != constUtil.P7RoundLiveAutoProof || out.Schema != constUtil.P6FinalActorHPSchema || out.EngineVersion != input.EngineVersion || out.ClientVersion != input.ClientVersion || out.ConfigurationHash != input.ConfigurationHash || !proto.Equal(record, out.OriginalOperates) {
		return reject()
	}
	recordHash, sourceHash, err := p6RoundLiveAutoSourceDigests(input, record, controls)
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
