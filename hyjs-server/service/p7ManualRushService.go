package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"time"
)

func initializeP7ManualRush(tx *gorm.DB) error {
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&[]db.P7RushManualPolicy{{ID: constUtil.P7RushManualDriverPolicyID, Source: constUtil.P7RushManualDriverSource}, {ID: constUtil.P7RushEliteManualDriverPolicyID, Source: constUtil.P7RushEliteManualDriverSource}}).Error
}

// Freeze permission only on this new captured source. A preview may admit the
// roster before payment, but never changes a pending record into a real result.
func (s *GameService) p7PrepareManualRush(ctx context.Context, tx *gorm.DB, id int64, replayID string, start *pb.SCRushFightStart, now time.Time) (*model.CombatOutcome, bool, error) {
	policyID, driverSource, scopeProof, trial := p7RushManualIdentity(start.GetRushType())
	if policyID == 0 || scopeProof == "" {
		return nil, false, nil
	}
	var policy db.P7RushManualPolicy
	if err := tx.First(&policy, policyID).Error; err != nil {
		return nil, true, err
	}
	if !policy.Enabled || policy.Source != driverSource {
		return nil, false, nil
	}
	series, err := s.FreezeP6RushSeriesInput(start, trial, false, nil, constUtil.P6OriginalClientHash)
	if err != nil {
		return nil, true, err
	}
	family, err := p7ManualRushFamilyForSeries(series)
	if err != nil {
		return nil, true, err
	}
	driverSource = family.DriverSource
	scopeProof = family.ScopeProof
	var record db.P6BattleRecord
	if err = tx.First(&record, "id=?", replayID).Error; err != nil {
		return nil, true, err
	}
	input, err := decodeP6Record(&record)
	if err != nil {
		return nil, true, err
	}
	a, _ := json.Marshal(input)
	b, _ := json.Marshal(series.Template)
	if !bytes.Equal(a, b) || record.PlayerID != id || record.Source != constUtil.P7RushReplaySource || record.SourceFight != start.GetFightId() || record.Status != constUtil.P6BattleCaptured || record.Authoritative {
		return nil, true, errors.New(constUtil.P7ScopedRushConflict)
	}
	preview, err := family.Preview(ctx, s, series)
	if err != nil {
		return nil, true, err
	}
	body, err := json.Marshal(series)
	if err != nil {
		return nil, true, err
	}
	stored, err := encodeP6StoredInput(body)
	if err != nil {
		return nil, true, err
	}
	previewBody, err := json.Marshal(preview)
	if err != nil {
		return nil, true, err
	}
	rawStart, err := proto.Marshal(start)
	if err != nil {
		return nil, true, err
	}
	row := db.P7RushManualCalculation{FightID: start.GetFightId(), PlayerID: id, P6RecordID: replayID, DriverPolicy: driverSource, ScopeProof: scopeProof, Series: stored, SeriesHash: p6Digest(body), StartHash: p6Digest(rawStart), Preview: previewBody, PreviewHash: p6Digest(previewBody), CreatedAt: now.UnixMilli()}
	if err = tx.Create(&row).Error; err != nil {
		return nil, true, err
	}
	return preview.Outcome, true, nil
}

// Preserve the original stored input. A new explicit permission row alone may
// authorize the pending-to-computed transition after whole End comparison.
func p7ReadManualRush(tx *gorm.DB, fight *db.P7RushFight, request *pb.CSRushFightEnd) (*model.CombatOutcome, bool, error) {
	reject := func() (*model.CombatOutcome, bool, error) {
		return nil, true, errors.New(constUtil.P7ScopedRushConflict)
	}
	var row db.P7RushManualCalculation
	err := tx.First(&row, "fight_id=?", fight.ID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, true, err
	}
	family, familyErr := p7ManualRushFamilyForReceipt(fight.Kind, row.DriverPolicy, row.ScopeProof)
	if familyErr != nil {
		return reject()
	}
	driverSource := family.DriverSource
	scopeProof := family.ScopeProof
	if fight.Diagnostic || row.PlayerID != fight.PlayerID || row.P6RecordID != fight.ReplayID || row.StartHash != fight.StartHash || p6Digest(fight.Start) != fight.StartHash || p6Digest(fight.Rule) != fight.RuleHash || row.DriverPolicy != driverSource || row.ScopeProof == "" || row.ScopeProof != scopeProof {
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
	var cfg db.P7RushStageRule
	start := &pb.SCRushFightStart{}
	if err != nil || p6Digest(raw) != row.SeriesHash || json.Unmarshal(raw, &series) != nil || json.Unmarshal(fight.Rule, &cfg) != nil || proto.Unmarshal(fight.Start, start) != nil || family.ValidateScope(&series) != nil {
		return reject()
	}
	a, _ := json.Marshal(input)
	b, _ := json.Marshal(series.Template)
	if !bytes.Equal(a, b) || !proto.Equal(start, series.Start) || start.GetFightId() != fight.ID || cfg.Kind != fight.Kind || cfg.TaskID != fight.TaskID || cfg.Scene != start.GetFightScene() || cfg.MaxRound != start.GetMaxRound() || cfg.WinKey != start.GetWinKey() || cfg.WinValue != start.GetWinValue() || input.Attacker.GetPlayerId() != fight.PlayerID || record.PlayerID != fight.PlayerID || record.Source != constUtil.P7RushReplaySource || record.SourceFight != fight.ID {
		return reject()
	}
	if row.Status == constUtil.P7RushManualPending {
		if fight.Status == constUtil.P7RushSettled || record.Status != constUtil.P6BattleCaptured || record.Authoritative || len(row.Terminal) != 0 || row.TerminalHash != "" || row.DerivedCalculationSourceSHA256 != "" || row.EndRequestHash != "" || row.AcceptedAt != 0 {
			return reject()
		}
		if request == nil {
			var preview model.CombatFinalHPOutcome
			if p6Digest(row.Preview) != row.PreviewHash || json.Unmarshal(row.Preview, &preview) != nil || preview.Outcome == nil {
				return reject()
			}
			if family.ValidatePreview(&series, &preview) != nil {
				return reject()
			}
			return preview.Outcome, true, nil
		}
		terminal, computeErr := family.ComputeEnd(tx.Statement.Context, &GameService{}, &series, request.GetOptRecord(), request.GetResult())
		if computeErr != nil {
			return nil, true, computeErr
		}
		body, encodeErr := json.Marshal(terminal)
		if encodeErr != nil {
			return nil, true, encodeErr
		}
		outcome, encodeErr := json.Marshal(terminal.Outcome)
		if encodeErr != nil {
			return nil, true, encodeErr
		}
		operations, encodeErr := proto.MarshalOptions{Deterministic: true}.Marshal(terminal.Outcome.Operates)
		if encodeErr != nil {
			return nil, true, encodeErr
		}
		end, encodeErr := proto.Marshal(request)
		if encodeErr != nil {
			return nil, true, encodeErr
		}
		now := time.Now().UnixMilli()
		result := p6PlaybackResult(terminal.Outcome.Winner)
		changed := tx.Model(&db.P6BattleRecord{}).Where("id=? AND status=? AND authoritative=?", record.ID, constUtil.P6BattleCaptured, false).Updates(map[string]any{"status": constUtil.P6BattleComputed, "operates": operations, "outcome": outcome, "outcome_hash": p6Digest(outcome), "client_result": result, "server_result": result, "replay_hash": p6Digest(append(append([]byte(nil), operations...), byte(result))), "authoritative": true, "finished_at": now})
		if changed.Error != nil {
			return nil, true, changed.Error
		}
		if changed.RowsAffected != 1 {
			return reject()
		}
		changed = tx.Model(&db.P7RushManualCalculation{}).Where("fight_id=? AND status=? AND scope_proof=?", row.FightID, constUtil.P7RushManualPending, row.ScopeProof).Updates(map[string]any{"status": constUtil.P7RushManualAccepted, "terminal": body, "terminal_hash": p6Digest(body), "derived_calculation_source_sha256": terminal.CalculationSourceSHA256, "end_request_hash": p6Digest(end), "accepted_at": now})
		if changed.Error != nil {
			return nil, true, changed.Error
		}
		if changed.RowsAffected != 1 {
			return reject()
		}
		return terminal.Outcome, true, nil
	}
	if row.Status != constUtil.P7RushManualAccepted || row.AcceptedAt <= 0 || row.TerminalHash == "" || p6Digest(row.Terminal) != row.TerminalHash || row.EndRequestHash == "" || record.Status != constUtil.P6BattleComputed || !record.Authoritative {
		return reject()
	}
	var terminal model.CombatFinalHPOutcome
	if json.Unmarshal(row.Terminal, &terminal) != nil || terminal.Outcome == nil || terminal.CalculationSourceSHA256 != row.DerivedCalculationSourceSHA256 {
		return reject()
	}
	supplied := terminal.OriginalOperates
	reported := int32(0)
	if terminal.Outcome.Winner == 0 {
		reported = 1
	}
	if request != nil {
		end, marshalErr := proto.Marshal(request)
		if marshalErr != nil || p6Digest(end) != row.EndRequestHash {
			return reject()
		}
		supplied = request.GetOptRecord()
		reported = request.GetResult()
	}
	if err = family.ValidateEnd(&series, &terminal, supplied, reported); err != nil {
		return nil, true, err
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
	return terminal.Outcome, true, nil
}

// New admission requires the matching kind's policy. A separately enabled Elite
// policy cannot enable Trial, and frozen old permissions remain readable after closure.
func p7ManualRushStageAllowed(tx *gorm.DB, kind int32) (bool, error) {
	policyID, source, proof, _ := p7RushManualIdentity(kind)
	if policyID == 0 || proof == "" {
		return false, nil
	}
	var policies []db.P7RushManualPolicy
	if err := tx.Where("id IN ?", []int64{constUtil.P7RushManualDriverPolicyID, constUtil.P7RushEliteManualDriverPolicyID}).Find(&policies).Error; err != nil {
		return false, err
	}
	active := false
	own := false
	for _, policy := range policies {
		_, expected, _, _ := p7RushManualIdentity(constUtil.P7RushTrial)
		if policy.ID == constUtil.P7RushEliteManualDriverPolicyID {
			_, expected, _, _ = p7RushManualIdentity(constUtil.P7RushElite)
		}
		if policy.Enabled && policy.Source == expected {
			active = true
			if policy.ID == policyID && policy.Source == source {
				own = true
			}
		}
	}
	if active {
		return own, nil
	}
	return true, nil
}

func p7RushManualIdentity(kind int32) (int64, string, string, bool) {
	switch kind {
	case constUtil.P7RushTrial:
		return constUtil.P7RushManualDriverPolicyID, constUtil.P7RushManualDriverSource, constUtil.P6RushManualNativeProof, true
	case constUtil.P7RushElite:
		return constUtil.P7RushEliteManualDriverPolicyID, constUtil.P7RushEliteManualDriverSource, constUtil.P6RushEliteManualNativeProof, false
	default:
		return 0, "", "", false
	}
}
