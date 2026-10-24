package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// computeP12WantedRecorded keeps submitted commands as calculation input. The
// independent native recorded-driver proof is required before settlement opens.
func (s *GameService) computeP12WantedRecorded(ctx context.Context, input *model.FrozenBattleInput, supplied *pb.BattleOperate) (*model.CombatFinalHPOutcome, error) {
	if !constUtil.P12WantedRecordedCertified || len(constUtil.P12WantedRecordedProof) != 64 || supplied == nil || p6ScopedUnknown(supplied) || supplied.AttackerTransOpt != nil || supplied.DefenderTransOpt != nil {
		return nil, errors.New(constUtil.CombatUnsupported)
	}
	if _, err := P12WantedProofForInput(input); err != nil {
		return nil, err
	}
	for _, op := range supplied.SkillOpts {
		if op == nil || p6ScopedUnknown(op) || op.GetCamp() < 0 || op.GetCamp() > 1 {
			return nil, errors.New(constUtil.P6ReplayConflict)
		}
	}
	out, err := s.computeP12WantedLiveAutoDiagnostic(ctx, input, supplied)
	if err != nil {
		return nil, err
	}
	if err = validateP12WantedRecorded(input, supplied, out); err != nil {
		return nil, err
	}
	out.Outcome.Authoritative = true
	out.Outcome.CapabilityHash = constUtil.P12WantedRecordedProof
	return out, nil
}

// The admitted subfamily starts manual, then enables live AI at the first
// successful player command in round one. Every emitted command must match;
// other manual schedules remain unsupported rather than becoming playback.
func p12WantedLiveAutoClock(input *model.FrozenBattleInput, supplied *pb.BattleOperate) (float64, error) {
	if _, err := P12WantedProofForInput(input); err != nil {
		return 0, err
	}
	if supplied == nil || p6ScopedUnknown(supplied) || supplied.AttackerTransOpt != nil || supplied.DefenderTransOpt != nil {
		return 0, errors.New(constUtil.P6ReplayConflict)
	}
	for _, op := range supplied.SkillOpts {
		if op == nil || p6ScopedUnknown(op) || op.GetCamp() < 0 || op.GetCamp() > 1 {
			return 0, errors.New(constUtil.P6ReplayConflict)
		}
		if op.GetCamp() == 0 {
			clock := op.GetRoundTime()
			if op.GetRoundNum() != 1 || !combatFinite(clock) || clock < 0 || clock > 110 {
				return 0, errors.New(constUtil.CombatUnsupported)
			}
			return clock, nil
		}
	}
	return 0, errors.New(constUtil.CombatUnsupported)
}

func p12WantedLiveAutoDigests(input *model.FrozenBattleInput, supplied *pb.BattleOperate, clock float64) (string, string, error) {
	raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(supplied)
	if err != nil {
		return "", "", err
	}
	view := *input
	view.Tables = nil
	frozen, err := json.Marshal(&view)
	if err != nil {
		return "", "", err
	}
	bound, err := json.Marshal(struct {
		FrozenInput    string  `json:"frozenInput"`
		OriginalRecord string  `json:"originalRecord"`
		Driver         string  `json:"driver"`
		Clock          float64 `json:"clock"`
	}{p6Digest(frozen), p6Digest(raw), constUtil.P12WantedRecordedDriver, clock})
	if err != nil {
		return "", "", err
	}
	return p6Digest(raw), p6Digest(bound), nil
}

func (s *GameService) computeP12WantedLiveAutoDiagnostic(ctx context.Context, input *model.FrozenBattleInput, supplied *pb.BattleOperate) (*model.CombatFinalHPOutcome, error) {
	clock, err := p12WantedLiveAutoClock(input, supplied)
	if err != nil {
		return nil, err
	}
	derived, err := cloneP6CalculationInput(input)
	if err != nil {
		return nil, err
	}
	derived.DiagnosticAIDriverPolicy = constUtil.P6DiagnosticManualAutoStart
	derived.DiagnosticManualAutoStartRoundTime = proto.Float64(clock)
	out, err := s.computeFrozenBattleFinalHP(ctx, derived, nil, true)
	if err != nil {
		return nil, err
	}
	out.Outcome.DiagnosticAIDriverPolicy = constUtil.P12WantedRecordedDriver
	if !proto.Equal(supplied, out.OriginalOperates) {
		return nil, errors.New(constUtil.P6ReplayConflict)
	}
	out.OriginalRecordSHA256, out.CalculationSourceSHA256, err = p12WantedLiveAutoDigests(input, supplied, clock)
	if err != nil {
		return nil, err
	}
	if err = validateP12WantedRecorded(input, supplied, out); err != nil {
		return nil, err
	}
	return out, nil
}

func validateP12WantedRecorded(input *model.FrozenBattleInput, supplied *pb.BattleOperate, out *model.CombatFinalHPOutcome) error {
	reject := func() error { return errors.New(constUtil.P6ReplayConflict) }
	if _, err := P12WantedProofForInput(input); err != nil {
		return err
	}
	if supplied == nil || out == nil || out.Outcome == nil || out.OriginalOperates == nil || supplied.AttackerTransOpt != nil || supplied.DefenderTransOpt != nil || out.Schema != constUtil.P6FinalActorHPSchema || out.ConfigurationHash != input.ConfigurationHash || out.EngineVersion != input.EngineVersion || out.ClientVersion != input.ClientVersion || out.Outcome.DiagnosticAIDriverPolicy != constUtil.P12WantedRecordedDriver {
		return reject()
	}
	if p6ScopedUnknown(supplied) {
		return reject()
	}
	for _, op := range supplied.SkillOpts {
		if op == nil || p6ScopedUnknown(op) || op.GetCamp() < 0 || op.GetCamp() > 1 {
			return reject()
		}
	}
	clock, err := p12WantedLiveAutoClock(input, supplied)
	if err != nil {
		return err
	}
	recordHash, sourceHash, err := p12WantedLiveAutoDigests(input, supplied, clock)
	if err != nil || out.OriginalRecordSHA256 != recordHash || out.CalculationSourceSHA256 != sourceHash || !proto.Equal(supplied, out.OriginalOperates) {
		return reject()
	}
	return validateP6GenericMechanismTerminal(input, out)
}

func p12WantedStoreRecorded(fight *db.P12WantedFight, request *pb.CSRebelFightEnd, terminal *model.CombatFinalHPOutcome) error {
	if terminal == nil || terminal.Outcome == nil || !terminal.Outcome.Authoritative || terminal.Outcome.CapabilityHash != constUtil.P12WantedRecordedProof {
		return errors.New(constUtil.P6ReplayConflict)
	}
	raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(request)
	if err != nil {
		return err
	}
	body, err := json.Marshal(terminal)
	if err != nil {
		return err
	}
	fight.RecordedEndRequest = raw
	fight.RecordedTerminalJSON = string(body)
	fight.RecordedTerminalHash = p6Digest(body)
	fight.RecordedRecordHash = terminal.OriginalRecordSHA256
	fight.RecordedProof = constUtil.P12WantedRecordedProof
	return nil
}

func p12WantedRecordedReceipt(fight *db.P12WantedFight, request *pb.CSRebelFightEnd) error {
	if fight.RecordedProof == "" && len(fight.RecordedEndRequest) == 0 && fight.RecordedTerminalJSON == "" && fight.RecordedTerminalHash == "" && fight.RecordedRecordHash == "" {
		// Legacy automatic rows have no recorded columns. Their exact frozen
		// automatic record must still match, so losing all five recorded
		// columns cannot turn an accepted manual End into a legacy receipt.
		if request == nil || p6Digest([]byte(fight.InputJSON)) != fight.InputHash || p6Digest([]byte(fight.TerminalJSON)) != fight.TerminalHash || p6Digest(fight.StartProto) != fight.StartHash {
			return errors.New(constUtil.P6ReplayConflict)
		}
		var input model.FrozenBattleInput
		var original model.CombatFinalHPOutcome
		if json.Unmarshal([]byte(fight.InputJSON), &input) != nil || json.Unmarshal([]byte(fight.TerminalJSON), &original) != nil || ValidateP12WantedCertifiedReceipt(&input, &original) != nil || !proto.Equal(request.OptRecord, original.OriginalOperates) {
			return errors.New(constUtil.P6ReplayConflict)
		}
		return nil
	}
	reject := func() error { return errors.New(constUtil.P6ReplayConflict) }
	raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(request)
	if err != nil || !bytes.Equal(raw, fight.RecordedEndRequest) || p6Digest(raw) != fight.EndRequestHash || fight.RecordedProof != constUtil.P12WantedRecordedProof || p6Digest([]byte(fight.RecordedTerminalJSON)) != fight.RecordedTerminalHash || p6Digest([]byte(fight.InputJSON)) != fight.InputHash || p6Digest([]byte(fight.TerminalJSON)) != fight.TerminalHash || p6Digest(fight.StartProto) != fight.StartHash {
		return reject()
	}
	var input model.FrozenBattleInput
	var original, recorded model.CombatFinalHPOutcome
	if json.Unmarshal([]byte(fight.InputJSON), &input) != nil || json.Unmarshal([]byte(fight.TerminalJSON), &original) != nil || json.Unmarshal([]byte(fight.RecordedTerminalJSON), &recorded) != nil || ValidateP12WantedCertifiedReceipt(&input, &original) != nil || recorded.Outcome == nil || !recorded.Outcome.Authoritative || recorded.Outcome.CapabilityHash != fight.RecordedProof || recorded.OriginalRecordSHA256 != fight.RecordedRecordHash {
		return reject()
	}
	return validateP12WantedRecorded(&input, request.OptRecord, &recorded)
}
