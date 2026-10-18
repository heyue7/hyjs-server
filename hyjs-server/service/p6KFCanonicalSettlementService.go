package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
)

// This audit names the independently prepared local KF wrapper. Generic type17
// and ordinary SCKuafuMatchPlayer APIs retain their separate original boundary.
func (s *GameService) ComputeOriginalP6KFRealmProtocolFinalHP(ctx context.Context, input *model.FrozenBattleInput) (*model.CombatFinalHPOutcome, error) {
	if ctx == nil || input == nil {
		return nil, errors.New(constUtil.CombatInvalid)
	}
	if input.BattleType != 17 || input.KFReplay == nil || input.KFReplayViewer == nil || input.KFReplayViewer.CompatibilityRule != constUtil.P6KFJointViewerCompatibility || input.Monster != nil || len(input.WinConditions) != 0 || input.PVECompatibilityRule != "" || input.DiagnosticAIDriverPolicy != "" || input.DiagnosticManualAutoStartRoundTime != nil {
		return nil, errors.New(constUtil.CombatUnsupported)
	}
	if err := validateP6KFProjection(input); err != nil {
		return nil, err
	}
	if err := validateP6KFCalculationBoundary(input); err != nil {
		return nil, err
	}
	return s.computeFrozenBattleFinalHP(ctx, input, nil, true)
}

// Canonical means the actual source0 left joint identity. The display-only
// opposite viewer may never add a second series win or redefine global first order.
func validateP6KFCanonicalScope(input *model.FrozenBattleInput) error {
	reject := func() error { return errors.New(constUtil.CombatUnsupported) }
	if input == nil {
		return errors.New(constUtil.CombatInvalid)
	}
	raw := input.KFReplay
	viewer := input.KFReplayViewer
	if raw == nil || viewer == nil || len(raw.PlayerList) != 2 || raw.GetWarId() == "" || raw.WinPlayerId != nil || raw.Score != nil || input.IsAttToDef || input.DefenderFirst || input.BattleType != 17 || input.MaxRound != 40 || input.DiagnosticManualAutoStartRoundTime != nil || (raw.GetMaxRoundNum() > 0 && raw.GetMaxRoundNum() != 40) || (raw.GetRoomId() != "" && raw.GetRoomId() != "map_forest_001") || viewer.CompatibilityRule != constUtil.P6KFJointViewerCompatibility {
		return reject()
	}
	left, right := raw.PlayerList[0], raw.PlayerList[1]
	if left == nil || right == nil || left.GetServerId() <= 0 || right.GetServerId() <= 0 || left.GetServerId() == right.GetServerId() || left.GetPlayerId() <= 0 || right.GetPlayerId() <= 0 || viewer.ServerID != int64(left.GetServerId()) || viewer.PlayerID != left.GetPlayerId() || left.Inst == nil || right.Inst == nil || left.Inst.GetFirstOrder() != 0 || right.Inst.GetFirstOrder() != 0 || len(left.Inst.ExtBattleAdds) != 0 || len(right.Inst.ExtBattleAdds) != 0 {
		return reject()
	}
	if err := validateP6KFProjection(input); err != nil {
		return err
	}
	if err := validateP6KFCalculationBoundary(input); err != nil {
		return err
	}
	// Reuse only the certified actor/table validator. Neither normalized first
	// order nor the validation view is ever a calculation source.
	view, err := cloneP6CalculationInput(input)
	if err != nil {
		return err
	}
	view.KFReplay = nil
	view.KFReplayViewer = nil
	view.BattleType = 11
	view.Attacker.Inst.FirstOrder = proto.Int32(1)
	view.Defender.Inst.FirstOrder = proto.Int32(0)
	return validateP6Starter047Scope(view)
}

func (s *GameService) ComputeP6KFCanonicalSettlement(ctx context.Context, input *model.FrozenBattleInput) (*model.CombatFinalHPOutcome, error) {
	if ctx == nil || input == nil {
		return nil, errors.New(constUtil.CombatInvalid)
	}
	frozen, err := cloneP6CalculationInput(input)
	if err != nil {
		return nil, err
	}
	if err = validateP6KFCanonicalScope(frozen); err != nil {
		return nil, err
	}
	if constUtil.P6KFCanonicalNativeProof == "" {
		return nil, errors.New(constUtil.CombatUnsupported)
	}
	out, err := s.ComputeOriginalP6KFRealmProtocolFinalHP(ctx, frozen)
	if err != nil {
		return nil, err
	}
	if err = validateP6KFCanonicalTerminal(frozen, out); err != nil {
		return nil, err
	}
	out.Outcome.Authoritative = true
	out.Outcome.CapabilityHash = constUtil.P6KFCanonicalNativeProof
	return out, nil
}

func p6KnownKFCanonicalSettlementProof(input *model.FrozenBattleInput, proof string) bool {
	return constUtil.P6KFCanonicalNativeProof != "" && proof == constUtil.P6KFCanonicalNativeProof && validateP6KFCanonicalScope(input) == nil
}

func validateP6KFCanonicalTerminal(input *model.FrozenBattleInput, out *model.CombatFinalHPOutcome) error {
	if err := validateP6KFCanonicalScope(input); err != nil {
		return err
	}
	if err := validateP6Starter047Terminal(input, out); err != nil {
		return err
	}
	if out.Schema != constUtil.P6FinalActorHPSchema || out.EngineVersion != input.EngineVersion || out.ClientVersion != input.ClientVersion || out.ConfigurationHash != input.ConfigurationHash {
		return errors.New(constUtil.P6ReplayConflict)
	}
	closure := *input
	closure.Tables = nil
	raw, err := json.Marshal(&closure)
	if err != nil || p6Digest(raw) != out.CalculationSourceSHA256 {
		return errors.New(constUtil.P6ReplayConflict)
	}
	for _, actor := range out.FinalActors {
		if actor.UID != fmt.Sprintf("%d_%s", actor.Camp, actor.GUID) || actor.BloodNumber != 1 {
			return errors.New(constUtil.P6ReplayConflict)
		}
	}
	return nil
}

// P6KFCanonicalWinner is a read-only receipt validator and joint identity map.
// Numeric WinPlayerId alone cannot distinguish equal local IDs across realms.
func P6KFCanonicalWinner(input *model.FrozenBattleInput, out *model.CombatFinalHPOutcome) (int64, int64, error) {
	if out == nil || out.Outcome == nil || !out.Outcome.Authoritative || !p6KnownKFCanonicalSettlementProof(input, out.Outcome.CapabilityHash) {
		return 0, 0, errors.New(constUtil.CombatUnsupported)
	}
	if err := validateP6KFCanonicalTerminal(input, out); err != nil {
		return 0, 0, err
	}
	if out.Outcome.Winner < 0 || out.Outcome.Winner > 1 {
		return 0, 0, errors.New(constUtil.P6ReplayConflict)
	}
	winner := input.KFReplay.PlayerList[out.Outcome.Winner]
	return int64(winner.GetServerId()), winner.GetPlayerId(), nil
}
