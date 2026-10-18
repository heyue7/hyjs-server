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

// validateP6RoundOneLiveAutoFormation bounds one explicit control event. The
// expected complete record is an assertion, never a PlayerAI command source.
func validateP6RoundOneLiveAutoFormation(input *model.FrozenBattleInput, expected *pb.BattleOperate, formation *pb.BattleTransOperate) error {
	if input == nil || (input.BattleType != 9 && input.BattleType != 10 && input.BattleType != 16) || input.Attacker == nil || input.Monster == nil || input.Defender != nil || input.DefenderFirst || input.IsAttToDef || input.KFReplay != nil || input.KFReplayViewer != nil || len(input.WinConditions) != 0 || input.PVECompatibilityRule != "" || input.DiagnosticAIDriverPolicy != "" || input.DiagnosticManualAutoStartRoundTime != nil {
		return errors.New(constUtil.CombatUnsupported)
	}
	if expected == nil || formation == nil || expected.DefenderTransOpt != nil || !proto.Equal(expected.AttackerTransOpt, formation) || p6ScopedUnknown(expected) || p6ScopedUnknown(formation) || formation.GetRoundNum() != 1 || len(formation.ActorUids) != len(input.Attacker.Formation) || len(formation.ActorUids) == 0 || len(formation.Fpids) != len(formation.ActorUids) || len(expected.SkillOpts) == 0 || len(expected.SkillOpts) > 16000 {
		return errors.New(constUtil.P6ReplayConflict)
	}
	seen, slots := map[string]bool{}, map[int32]bool{}
	for i, uid := range formation.ActorUids {
		found := false
		for _, actor := range input.Attacker.Formation {
			found = found || actor.GetGuid() == uid
		}
		if !found || seen[uid] || formation.Fpids[i] < 1 || formation.Fpids[i] > 15 || slots[formation.Fpids[i]] {
			return errors.New(constUtil.P6ReplayConflict)
		}
		seen[uid], slots[formation.Fpids[i]] = true, true
	}
	for _, op := range expected.SkillOpts {
		if op == nil || p6ScopedUnknown(op) {
			return errors.New(constUtil.P6ReplayConflict)
		}
	}
	return nil
}

// P6RoundOneLiveAutoFormationSourceDigests binds the unchanged frozen source,
// complete expected PB, and explicit round1/time0 control under a separate domain.
// Old manual/playback/Auto source digest functions remain unchanged.
func P6RoundOneLiveAutoFormationSourceDigests(input *model.FrozenBattleInput, expected *pb.BattleOperate, formation *pb.BattleTransOperate) (string, string, error) {
	if err := validateP6RoundOneLiveAutoFormation(input, expected, formation); err != nil {
		return "", "", err
	}
	view := *input
	view.Tables = nil
	frozen, err := json.Marshal(&view)
	if err != nil {
		return "", "", err
	}
	record, err := proto.MarshalOptions{Deterministic: true}.Marshal(expected)
	if err != nil {
		return "", "", err
	}
	control, err := proto.MarshalOptions{Deterministic: true}.Marshal(formation)
	if err != nil {
		return "", "", err
	}
	recordHash := p6Digest(record)
	bound, err := json.Marshal(struct {
		FrozenInput, OriginalRecord, ControlDriver, FormationControl string
		AutoStartRound                                               int32
		AutoStartRoundTime                                           float64
	}{p6Digest(frozen), recordHash, constUtil.P6RoundOneLiveAutoFormationDriver, p6Digest(control), 1, 0})
	if err != nil {
		return "", "", err
	}
	return recordHash, p6Digest(bound), nil
}

// ComputeP6RoundOneLiveAutoFormation executes one original round1 DoTrans before
// live PlayerAI.Run at time0. It neither replays expected.SkillOpts nor rewrites
// Start, source slots, clocks, recorded commands, or comparison tolerances.
// Authority is assigned only by a separately reviewed mode consumer.
func (s *GameService) ComputeP6RoundOneLiveAutoFormation(ctx context.Context, input *model.FrozenBattleInput, expected *pb.BattleOperate, formation *pb.BattleTransOperate) (*model.CombatFinalHPOutcome, error) {
	if err := validateP6RoundOneLiveAutoFormation(input, expected, formation); err != nil {
		return nil, err
	}
	if _, err := s.ValidateP6GenericMechanisms(ctx, input); err != nil {
		return nil, err
	}
	result, err := s.computeFrozenBattleFinalHPControlled(ctx, input, expected, true, formation)
	if err != nil {
		return nil, err
	}
	if !proto.Equal(expected, result.OriginalOperates) {
		return nil, errors.New(constUtil.P6ReplayConflict)
	}
	if err = validateP6GenericMechanismTerminal(input, result); err != nil {
		return nil, err
	}
	result.OriginalRecordSHA256, result.CalculationSourceSHA256, err = P6RoundOneLiveAutoFormationSourceDigests(input, expected, formation)
	if err != nil {
		return nil, err
	}
	result.Outcome.DiagnosticAIDriverPolicy = constUtil.P6RoundOneLiveAutoFormationDriver
	return result, nil
}

// applyRoundOneLiveAutoFormation runs after original RoundBegin triggers, before
// the new path's first PlayerAI.Run. No elapsed time, actor stats or RNG is assigned.
func (e *combatRoundEngine) applyRoundOneLiveAutoFormation() error {
	if e.roundOneLiveAutoFormation == nil || e.originalFormationApplied {
		return nil
	}
	if e.round != 1 || e.camp != 0 || e.time != 0 {
		return errors.New(constUtil.P6ReplayConflict)
	}
	return e.applyOriginalFormationChange(e.roundOneLiveAutoFormation)
}
