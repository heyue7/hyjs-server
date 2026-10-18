package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Seed a separate off-by-default operational switch. Global full-engine parity
// remains independent and SQL alone can never supply an original runtime proof.
func initializeP7ScopedRush(tx *gorm.DB) error {
	row := db.P7RushScopedPolicy{ID: constUtil.P7ScopedRushPolicyID, Source: constUtil.P7ScopedRushPolicySource}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error
}

func p7ScopedRushAvailable(tx *gorm.DB) (bool, error) {
	if constUtil.P6RushStarterNativeProof == "" {
		return false, nil
	}
	var policy db.P7RushScopedPolicy
	if err := tx.First(&policy, constUtil.P7ScopedRushPolicyID).Error; err != nil {
		return false, err
	}
	return policy.Enabled && policy.Source == constUtil.P7ScopedRushPolicySource, nil
}

// A query probes the actual current roster and this frozen stage without making
// a paid fight, changing the formation, or claiming arbitrary stage coverage.
func (s *GameService) p7ScopedStageAvailable(ctx context.Context, tx *gorm.DB, p *pb.Player, g *model.GameState, cfg db.P7RushStageRule, now time.Time) (bool, error) {
	if allowed, err := p7ManualRushStageAllowed(tx, cfg.Kind); err != nil || !allowed {
		return false, err
	}
	enabled, err := p7ScopedRushAvailable(tx)
	if err != nil || !enabled {
		return false, err
	}
	monster, err := s.validateP7Stage(cfg)
	if err != nil {
		return false, nil
	}
	player := proto.Clone(p).(*pb.Player)
	raw, err := json.Marshal(g)
	if err != nil {
		return false, err
	}
	var game model.GameState
	if err = json.Unmarshal(raw, &game); err != nil {
		return false, err
	}
	if err = s.PrepareP5GuildPrayerProperties(ctx, tx, player, &game, now); err != nil {
		return false, err
	}
	attacker, err := s.battlePlayer(player, &game)
	if err != nil {
		return false, nil
	}
	if cfg.Kind == constUtil.P7RushTrial {
		state, e := p7State(tx, player.GetPlayerId(), now)
		if e != nil {
			return false, e
		}
		attacker, e = s.p7ApplyRushAddition(tx, state, attacker)
		if e != nil {
			return false, e
		}
	}
	start := &pb.SCRushFightStart{FightId: proto.String("scope-query"), RandSeed: proto.Int64(0), BattlePlayer: attacker, BattleMonsterList: []*pb.BattleMonster{monster}, FightScene: proto.String(cfg.Scene), MaxRound: proto.Int32(cfg.MaxRound), WinKey: proto.Int32(cfg.WinKey), WinValue: proto.Int32(cfg.WinValue), RushType: proto.Int32(cfg.Kind)}
	series, err := s.FreezeP6RushSeriesInput(start, cfg.Kind == constUtil.P7RushTrial, true, nil, constUtil.P6OriginalClientHash)
	if err != nil {
		return false, nil
	}
	if validateP6RushStarterScope(series) == nil {
		return true, nil
	}
	return p7PartyRushSourceAvailable(series) || p7NaturalRushSourceAvailable(series) || p7GenericMechanismRushSourceAvailable(series), nil
}

// Compute only a new captured record in the owner's existing transaction.
// Complete series/terminal evidence is retained before the paid run is exposed.
func (s *GameService) p7ComputeScopedRush(ctx context.Context, tx *gorm.DB, id int64, replayID string, start *pb.SCRushFightStart, now time.Time) (*model.CombatOutcome, error) {
	if out, handled, err := s.p7PrepareManualRush(ctx, tx, id, replayID, start, now); handled || err != nil {
		return out, err
	}
	series, err := s.FreezeP6RushSeriesInput(start, start.GetRushType() == constUtil.P7RushTrial, true, nil, constUtil.P6OriginalClientHash)
	if err != nil {
		return nil, err
	}
	var record db.P6BattleRecord
	if err = tx.First(&record, "id=?", replayID).Error; err != nil {
		return nil, err
	}
	input, err := decodeP6Record(&record)
	if err != nil {
		return nil, err
	}
	a, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	b, err := json.Marshal(series.Template)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(a, b) || record.PlayerID != id || record.Source != constUtil.P7RushReplaySource || record.SourceFight != start.GetFightId() || record.Status != constUtil.P6BattleCaptured || record.Authoritative {
		return nil, errors.New(constUtil.P7ScopedRushConflict)
	}
	terminal, err := s.ComputeP6RushStarterSettlement(ctx, series)
	if err != nil {
		return nil, err
	}
	result := int32(0)
	if terminal.Outcome.Winner == 0 {
		result = 1
	}
	if err = ValidateP6RushStarterEnd(series, terminal, terminal.OriginalOperates, result); err != nil {
		return nil, err
	}
	seriesBody, err := json.Marshal(series)
	if err != nil {
		return nil, err
	}
	stored, err := encodeP6StoredInput(seriesBody)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(terminal)
	if err != nil {
		return nil, err
	}
	rawStart, err := proto.Marshal(start)
	if err != nil {
		return nil, err
	}
	row := db.P7RushScopedCalculation{FightID: start.GetFightId(), PlayerID: id, P6RecordID: replayID, Series: stored, SeriesHash: p6Digest(seriesBody), Terminal: body, TerminalHash: p6Digest(body), StartHash: p6Digest(rawStart), ScopeProof: terminal.Outcome.CapabilityHash, CreatedAt: now.UnixMilli()}
	if err = tx.Create(&row).Error; err != nil {
		return nil, err
	}
	operations, err := proto.MarshalOptions{Deterministic: true}.Marshal(terminal.Outcome.Operates)
	if err != nil {
		return nil, err
	}
	outcome, err := json.Marshal(terminal.Outcome)
	if err != nil {
		return nil, err
	}
	replayResult := p6PlaybackResult(terminal.Outcome.Winner)
	changed := tx.Model(&db.P6BattleRecord{}).Where("id=? AND status=? AND authoritative=?", record.ID, constUtil.P6BattleCaptured, false).Updates(map[string]any{"status": constUtil.P6BattleComputed, "operates": operations, "outcome": outcome, "outcome_hash": p6Digest(outcome), "client_result": replayResult, "server_result": replayResult, "replay_hash": p6Digest(append(append([]byte(nil), operations...), byte(replayResult))), "authoritative": true, "finished_at": now.UnixMilli()})
	if changed.Error != nil {
		return nil, changed.Error
	}
	if changed.RowsAffected != 1 {
		return nil, errors.New(constUtil.P7ScopedRushConflict)
	}
	return terminal.Outcome, nil
}

// Accepted paid runs use their immutable source after policy/roster changes.
// Diagnostic records and malformed partial rows cannot fall back to global mode.
func p7ReadScopedRush(tx *gorm.DB, fight *db.P7RushFight, request *pb.CSRushFightEnd) (*model.CombatOutcome, bool, error) {
	if out, handled, err := p7ReadManualRush(tx, fight, request); handled || err != nil {
		return out, handled, err
	}
	reject := func() (*model.CombatOutcome, bool, error) {
		return nil, true, errors.New(constUtil.P7ScopedRushConflict)
	}
	var row db.P7RushScopedCalculation
	err := tx.First(&row, "fight_id=?", fight.ID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, true, err
	}
	if fight.Diagnostic || row.PlayerID != fight.PlayerID || row.P6RecordID != fight.ReplayID || row.StartHash != fight.StartHash || p6Digest(fight.Start) != fight.StartHash || p6Digest(fight.Rule) != fight.RuleHash || p6Digest(row.Terminal) != row.TerminalHash {
		return reject()
	}
	var record db.P6BattleRecord
	if err = tx.First(&record, "id=?", row.P6RecordID).Error; err != nil {
		return nil, true, err
	}
	input, err := decodeP6Record(&record)
	if err != nil {
		return nil, true, err
	}
	raw, err := decodeP6StoredInput(row.Series)
	var series model.FrozenP6RushSeries
	var terminal model.CombatFinalHPOutcome
	var cfg db.P7RushStageRule
	start := &pb.SCRushFightStart{}
	if err != nil || p6Digest(raw) != row.SeriesHash || json.Unmarshal(raw, &series) != nil || json.Unmarshal(row.Terminal, &terminal) != nil || json.Unmarshal(fight.Rule, &cfg) != nil || proto.Unmarshal(fight.Start, start) != nil || terminal.Outcome == nil {
		return reject()
	}
	a, err := json.Marshal(input)
	if err != nil {
		return nil, true, err
	}
	b, err := json.Marshal(series.Template)
	if err != nil {
		return nil, true, err
	}
	if !bytes.Equal(a, b) || !proto.Equal(start, series.Start) || start.GetFightId() != fight.ID || start.GetRushType() != fight.Kind || cfg.Kind != fight.Kind || cfg.TaskID != fight.TaskID || cfg.Scene != start.GetFightScene() || cfg.MaxRound != start.GetMaxRound() || cfg.WinKey != start.GetWinKey() || cfg.WinValue != start.GetWinValue() || input.Attacker.GetPlayerId() != fight.PlayerID || record.PlayerID != fight.PlayerID || record.Source != constUtil.P7RushReplaySource || record.SourceFight != fight.ID || record.Status != constUtil.P6BattleComputed || !record.Authoritative || !p6KnownRushStarterSettlementProof(input, row.ScopeProof) || row.ScopeProof != terminal.Outcome.CapabilityHash {
		return reject()
	}
	outcome, err := json.Marshal(terminal.Outcome)
	if err != nil {
		return nil, true, err
	}
	operations, err := proto.MarshalOptions{Deterministic: true}.Marshal(terminal.Outcome.Operates)
	if err != nil {
		return nil, true, err
	}
	if !bytes.Equal(outcome, record.Outcome) || p6Digest(outcome) != record.OutcomeHash || !bytes.Equal(operations, record.Operates) || record.ReplayHash != p6Digest(append(append([]byte(nil), operations...), byte(record.ServerResult))) || record.ServerResult != p6PlaybackResult(terminal.Outcome.Winner) || record.ClientResult != record.ServerResult {
		return reject()
	}
	supplied := terminal.OriginalOperates
	reported := int32(0)
	if terminal.Outcome.Winner == 0 {
		reported = 1
	}
	if request != nil {
		supplied = request.GetOptRecord()
		reported = request.GetResult()
	}
	if err = ValidateP6RushStarterEnd(&series, &terminal, supplied, reported); err != nil {
		return nil, true, err
	}
	return terminal.Outcome, true, nil
}

// The original client imports full reward bodies again when a receipt is replayed.
// Reuse the shared owned-item check before exposing an already accepted reward.
func (s *GameService) p7ValidateScopedAward(tx *gorm.DB, id int64, g *model.GameState, result *pb.SCRushFightEnd) error {
	return s.ValidateCopyAwardReceiptCurrent(tx, id, g, &pb.SCCopyFightEnd{Gain: result.ResultObj})
}
