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

const p12WantedLiveRecordedDriver = "original-wanted8-recorded-player-live-npc-exact-v1"
const p12WantedLiveRecordedProof = "87598dfe7341455b589f327139aa0e3930f53fd48997e6aaebb6f6aa7b93db66"

func p12WantedLiveRecordedSource(input *model.FrozenBattleInput, record *pb.BattleOperate) (string, string, error) {
	if _, err := P12WantedProofForInput(input); err != nil {
		return "", "", err
	}
	if record == nil || record.AttackerTransOpt != nil || record.DefenderTransOpt != nil || len(record.SkillOpts) == 0 || len(record.SkillOpts) > 16000 || p6ScopedUnknown(record) {
		return "", "", errors.New(constUtil.P6ReplayConflict)
	}
	for _, op := range record.SkillOpts {
		if op == nil || p6ScopedUnknown(op) || op.GetCamp() < 0 || op.GetCamp() > 1 {
			return "", "", errors.New(constUtil.P6ReplayConflict)
		}
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
	bound, err := json.Marshal([]string{p6Digest(frozen), hash, p12WantedLiveRecordedDriver, p12WantedLiveRecordedProof})
	if err != nil {
		return "", "", err
	}
	return hash, p6Digest(bound), nil
}

// Recorded player commands drive only the player's camp. NPC AI, legal skill
// execution, RNG and all damage/HP/round calculations remain independent.
func (s *GameService) computeP12WantedLiveRecorded(ctx context.Context, input *model.FrozenBattleInput, record *pb.BattleOperate) (*model.CombatFinalHPOutcome, error) {
	if len(p12WantedLiveRecordedProof) != 64 {
		return nil, errors.New(constUtil.CombatUnsupported)
	}
	if _, _, err := p12WantedLiveRecordedSource(input, record); err != nil {
		return nil, err
	}
	if _, err := s.ValidateP6GenericMechanisms(ctx, input); err != nil {
		return nil, err
	}
	out, err := s.computeFrozenBattleFinalHPLiveControlled(ctx, input, record, true, nil, nil, true)
	if err != nil {
		return nil, err
	}
	if !proto.Equal(record, out.OriginalOperates) {
		return nil, errors.New(constUtil.P6ReplayConflict)
	}
	if err = validateP6GenericMechanismTerminal(input, out); err != nil {
		return nil, err
	}
	out.OriginalRecordSHA256, out.CalculationSourceSHA256, err = p12WantedLiveRecordedSource(input, record)
	if err != nil {
		return nil, err
	}
	out.Outcome.DiagnosticAIDriverPolicy = p12WantedLiveRecordedDriver
	out.Outcome.CapabilityHash = p12WantedLiveRecordedProof
	out.Outcome.Authoritative = true
	return out, validateP12WantedLiveRecorded(input, record, out)
}
func validateP12WantedLiveRecorded(input *model.FrozenBattleInput, record *pb.BattleOperate, out *model.CombatFinalHPOutcome) error {
	reject := func() error { return errors.New(constUtil.P6ReplayConflict) }
	hash, source, err := p12WantedLiveRecordedSource(input, record)
	if err != nil {
		return err
	}
	if out == nil || out.Outcome == nil || !out.Outcome.Authoritative || out.Outcome.DiagnosticAIDriverPolicy != p12WantedLiveRecordedDriver || out.Outcome.CapabilityHash != p12WantedLiveRecordedProof || out.Schema != constUtil.P6FinalActorHPSchema || out.EngineVersion != input.EngineVersion || out.ClientVersion != input.ClientVersion || out.ConfigurationHash != input.ConfigurationHash || out.OriginalRecordSHA256 != hash || out.CalculationSourceSHA256 != source || !proto.Equal(record, out.OriginalOperates) {
		return reject()
	}
	return validateP6GenericMechanismTerminal(input, out)
}
