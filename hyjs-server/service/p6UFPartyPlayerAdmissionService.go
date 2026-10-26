package service

import (
	"context"
	"encoding/json"
	"errors"

	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Only this typed server-source factory can mint the fresh admission envelope.
// No exported API accepts a frozen snapshot, table set, seal or qualification.
// A byte seal also detects accidental mutation before its durable capture.
type p6UFPartyFreshAdmission struct {
	owner     *GameService
	input     *model.FrozenBattleInput
	inputHash string
	fightID   string
}

func (s *GameService) newP6UFPartyPlayerAdmission(ctx context.Context, typed *pb.SCUFFightRecordReplay, properties map[string][]string) (*p6UFPartyFreshAdmission, error) {
	if ctx == nil || constUtil.P6UFPartyPlayerNativeProof == "" {
		return nil, errors.New(constUtil.CombatUnsupported)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	source, err := p6UFPlayerTypedSource(typed)
	if err != nil {
		return nil, err
	}
	source.AppliedProperties = properties
	frozen, err := s.FreezeBattleInput(*source)
	if err != nil {
		return nil, err
	}
	if err = validateP6UFPartyPlayerScope(frozen); err != nil {
		return nil, err
	}
	// Canonical rows were just copied from this server's source, and have not
	// escaped. Qualify their full actor/skill/talent closure without refreezing.
	if _, err = qualifyP6GenericMechanisms(s, frozen, frozen); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(frozen)
	if err != nil {
		return nil, err
	}
	return &p6UFPartyFreshAdmission{owner: s, input: frozen, inputHash: p6Digest(raw), fightID: typed.GetFightId()}, nil
}

func (s *GameService) captureP6UFPartyPlayerAdmission(ctx context.Context, tx *gorm.DB, playerID int64, fightID string, fresh *p6UFPartyFreshAdmission) (string, string, *model.FrozenBattleInput, error) {
	if fresh == nil || fresh.owner != s || fresh.input == nil || fresh.inputHash == "" || fresh.fightID != fightID {
		return "", "", nil, errors.New(constUtil.CombatInvalid)
	}
	raw, err := json.Marshal(fresh.input)
	if err != nil || p6Digest(raw) != fresh.inputHash {
		return "", "", nil, errors.New(constUtil.P6ReplayConflict)
	}
	id, matchID, previous, saved, err := p6ReplayCaptureIdentity(ctx, tx, playerID, constUtil.P13UFPlayerBattleSource, fightID, *fresh.input)
	if err != nil {
		return "", "", nil, err
	}
	if previous != nil {
		return previous.ID, previous.InputHash, saved, nil
	}
	record, err := p6WriteCapturedReplayEncoded(ctx, tx, id, matchID, playerID, constUtil.P13UFPlayerBattleSource, fightID, fresh.input, raw)
	if err != nil {
		return "", "", nil, err
	}
	return record.ID, record.InputHash, fresh.input, nil
}
