package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
)

func openingCarryScope(tx *gorm.DB, playerID int64) (string, error) {
	if scope := adminOpeningPeriod(tx); scope != nil {
		if scope.PlayerID != playerID || scope.OccurrenceID == "" || len(scope.OccurrenceID) > 64 {
			return "", constUtil.ErrAdminConfiguration
		}
		return "occurrence:" + scope.OccurrenceID, nil
	}
	cycle, err := localPlayOpenServerCycle(tx)
	if err != nil {
		return "", err
	}
	if cycle != nil {
		if cycle.PlayerID != playerID || cycle.ID == "" || len(cycle.ID) > 64 {
			return "", constUtil.ErrAdminConfiguration
		}
		return "cycle:" + cycle.ID, nil
	}
	return "lifetime", nil
}

func openingCarryTransaction(tx *gorm.DB) bool {
	if tx == nil || tx.Statement == nil {
		return false
	}
	_, ok := tx.Statement.ConnPool.(gorm.TxCommitter)
	return ok
}

func openingCarryDatedKey(key string) (string, string, bool) {
	for _, prefix := range []string{constUtil.OpenServerActivityCounterLogin, constUtil.OpenServerActivityCounterCopyWin, constUtil.OpenServerActivityCounterPredatory} {
		if strings.HasPrefix(key, prefix) {
			return prefix, strings.TrimPrefix(key, prefix), true
		}
	}
	return "", "", false
}

func openingCarryDigest(row db.OpeningCalendarCarry) string {
	row.Digest = ""
	return p6Digest(adminJSON(row))
}

func openingCarryDecode(row db.OpeningCalendarCarry) (map[string]int32, map[string]bool, error) {
	valid := func(start, end, claim int64) bool {
		return start > 0 && end > start && claim >= end && claim <= constUtil.ActivityCalendarRestoreMaxMillis && (start+8*3600000)%86400000 == 0 && (end-start)%86400000 == 0
	}
	validScope := row.Scope == "lifetime" || (strings.HasPrefix(row.Scope, "occurrence:") && len(row.Scope) > len("occurrence:") && len(row.Scope) <= len("occurrence:")+64) || (strings.HasPrefix(row.Scope, "cycle:") && len(row.Scope) > len("cycle:") && len(row.Scope) <= len("cycle:")+64)
	if row.PlayerID <= 0 || !validScope || row.RequestID == "" || len(row.RequestID) > 128 || row.OldNow <= 0 || row.RealCutoff <= 0 || !valid(row.BeforeStartAt, row.BeforeEndAt, row.BeforeClaimEndAt) || !valid(row.AfterStartAt, row.AfterEndAt, row.AfterClaimEndAt) || row.Digest != openingCarryDigest(row) {
		return nil, nil, constUtil.ErrAdminConfiguration
	}
	var baseline map[string]int32
	var logins map[string]bool
	if openingCarryJSON(row.Baseline, &baseline) != nil || baseline == nil || openingCarryJSON(row.LoginDays, &logins) != nil || logins == nil {
		return nil, nil, constUtil.ErrAdminConfiguration
	}
	for key, value := range baseline {
		prefix, date, ok := openingCarryDatedKey(key)
		if !ok || !fieldCalendarDayValid75(date) || value < 0 || (prefix == constUtil.OpenServerActivityCounterLogin && value > 1) {
			return nil, nil, constUtil.ErrAdminConfiguration
		}
	}
	for date, yes := range logins {
		if !yes || !fieldCalendarDayValid75(date) {
			return nil, nil, constUtil.ErrAdminConfiguration
		}
		at, _ := time.ParseInLocation("2006-01-02", date, time.FixedZone("UTC+8", 8*3600))
		if at.UnixMilli() < row.AfterStartAt || at.UnixMilli() >= row.AfterEndAt {
			return nil, nil, constUtil.ErrAdminConfiguration
		}
	}
	return baseline, logins, nil
}

func openingCarryJSON(raw []byte, target any) error {
	reader := json.NewDecoder(bytes.NewReader(raw))
	if _, err := readFrozenJSONValue(reader, 0); err != nil {
		return err
	}
	if _, err := reader.Token(); err != io.EOF {
		return constUtil.ErrAdminConfiguration
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

// PrepareOpeningCalendarCarry freezes raw dated counters before any window
// mutation. The caller holds clock/profile locks and binds the accepted opening
// instance using adminBindOpeningPeriod; nil period means the lifetime ledger.
// Payment counters are intentionally excluded and retain their own qualification.
func PrepareOpeningCalendarCarry(tx *gorm.DB, playerID int64, requestID string, before, after FrozenJSONWindow, oldNow, realCutoff int64) (*db.OpeningCalendarCarry, error) {
	if !openingCarryTransaction(tx) {
		return nil, constUtil.ErrAdminConfiguration
	}
	scope, err := openingCarryScope(tx, playerID)
	if err != nil {
		return nil, err
	}
	// Read raw ledger rows directly: another carry may already project payment
	// qualification through the shared repository during this same outer restore.
	rows, err := openingCarryRawCounters(tx, playerID, scope)
	if err != nil {
		return nil, err
	}
	baseline := map[string]int32{}
	for _, counter := range rows {
		if _, _, dated := openingCarryDatedKey(counter.CounterKey); dated {
			if _, exists := baseline[counter.CounterKey]; exists {
				return nil, constUtil.ErrAdminConfiguration
			}
			baseline[counter.CounterKey] = counter.Value
		}
	}
	row := &db.OpeningCalendarCarry{PlayerID: playerID, Scope: scope, RequestID: requestID, BeforeStartAt: before.StartAt, BeforeEndAt: before.EndAt, BeforeClaimEndAt: before.ClaimEndAt, AfterStartAt: after.StartAt, AfterEndAt: after.EndAt, AfterClaimEndAt: after.ClaimEndAt, OldNow: oldNow, RealCutoff: realCutoff, Baseline: adminJSON(baseline), LoginDays: []byte("{}")}
	row.Digest = openingCarryDigest(*row)
	if _, _, err = openingCarryDecode(*row); err != nil {
		return nil, err
	}
	return row, nil
}

func openingCarryRawCounters(tx *gorm.DB, playerID int64, scope string) ([]db.OpenServerActivityCounter, error) {
	rows := []db.OpenServerActivityCounter{}
	if strings.HasPrefix(scope, "occurrence:") {
		var saved []db.AdminOpeningCounter
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("occurrence_id=? AND player_id=?", strings.TrimPrefix(scope, "occurrence:"), playerID).Order("counter_key").Find(&saved).Error; err != nil {
			return nil, err
		}
		for _, row := range saved {
			rows = append(rows, db.OpenServerActivityCounter{PlayerID: row.PlayerID, CounterKey: row.CounterKey, Value: row.Value})
		}
		return rows, nil
	}
	if strings.HasPrefix(scope, "cycle:") {
		var saved []db.LocalPlayOpenServerCounter
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("cycle_id=? AND player_id=?", strings.TrimPrefix(scope, "cycle:"), playerID).Order("counter_key").Find(&saved).Error; err != nil {
			return nil, err
		}
		for _, row := range saved {
			rows = append(rows, db.OpenServerActivityCounter{PlayerID: row.PlayerID, CounterKey: row.CounterKey, Value: row.Value})
		}
		return rows, nil
	}
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("player_id=?", playerID).Order("counter_key").Find(&rows).Error
	return rows, err
}

// SaveOpeningCalendarCarry never replaces a previous restore. Repeating the
// original prepared image is allowed even after post-restore login markers grow.
func SaveOpeningCalendarCarry(tx *gorm.DB, row *db.OpeningCalendarCarry) error {
	if !openingCarryTransaction(tx) || row == nil {
		return constUtil.ErrAdminConfiguration
	}
	if _, _, err := openingCarryDecode(*row); err != nil {
		return err
	}
	scope, err := openingCarryScope(tx, row.PlayerID)
	if err != nil {
		return err
	}
	if scope != row.Scope {
		return constUtil.ErrAdminConfiguration
	}
	var prior db.OpeningCalendarCarry
	err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("player_id=? AND scope=?", row.PlayerID, row.Scope).Take(&prior).Error
	if err == nil {
		if _, _, err := openingCarryDecode(prior); err != nil {
			return err
		}
		prior.LoginDays = append([]byte(nil), row.LoginDays...)
		prior.Digest = openingCarryDigest(prior)
		if prior.Digest != row.Digest {
			return constUtil.ErrAdminConfigConflict
		}
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return tx.Create(row).Error
}

func loadOpeningCalendarCarry(tx *gorm.DB, playerID int64, cfg *db.OpenServerActivityRule, lock bool) (*db.OpeningCalendarCarry, error) {
	if !gameSchemaHasTable(tx, &db.OpeningCalendarCarry{}) {
		return nil, nil
	}
	scope, err := openingCarryScope(tx, playerID)
	if err != nil {
		return nil, err
	}
	query := tx.Where("player_id=? AND scope=?", playerID, scope)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var row db.OpeningCalendarCarry
	err = query.Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if _, _, err = openingCarryDecode(row); err != nil {
		return nil, err
	}
	if cfg.StartAt != row.AfterStartAt || cfg.EndAt != row.AfterEndAt || cfg.RewardEndAt != row.AfterClaimEndAt {
		return nil, constUtil.ErrAdminConfigConflict
	}
	return &row, nil
}

// openingCalendarCarryCounters projects dated qualification without rewriting
// any raw ledger. All baseline keys are subtracted, including future history.
func openingCalendarCarryCounters(tx *gorm.DB, playerID int64, cfg *db.OpenServerActivityRule, rows []db.OpenServerActivityCounter) ([]db.OpenServerActivityCounter, error) {
	row, err := loadOpeningCalendarCarry(tx, playerID, cfg, false)
	if err != nil || row == nil {
		return rows, err
	}
	baseline, logins, err := openingCarryDecode(*row)
	if err != nil {
		return nil, err
	}
	current := map[string]int32{}
	out := []db.OpenServerActivityCounter{}
	for _, counter := range rows {
		if _, date, ok := openingCarryDatedKey(counter.CounterKey); ok {
			if !fieldCalendarDayValid75(date) || counter.Value < 0 {
				return nil, constUtil.ErrAdminConfiguration
			}
			if _, exists := current[counter.CounterKey]; exists {
				return nil, constUtil.ErrAdminConfiguration
			}
			current[counter.CounterKey] = counter.Value
		} else {
			out = append(out, counter)
		}
	}
	projected := map[string]int32{}
	add := func(key string, value int32) error {
		prefix, _, _ := openingCarryDatedKey(key)
		if prefix == constUtil.OpenServerActivityCounterLogin {
			projected[key] = max(projected[key], value)
			return nil
		}
		if value < 0 || projected[key] > math.MaxInt32-value {
			return constUtil.ErrAdminConfiguration
		}
		projected[key] += value
		return nil
	}
	zone := time.FixedZone("UTC+8", 8*3600)
	for key, old := range baseline {
		if current[key] < old {
			return nil, constUtil.ErrAdminConfigConflict
		}
		prefix, date, _ := openingCarryDatedKey(key)
		at, _ := time.ParseInLocation("2006-01-02", date, zone)
		millis := at.UnixMilli()
		if millis < row.BeforeStartAt || millis >= row.BeforeEndAt || millis > row.OldNow {
			continue
		}
		mapped := row.AfterStartAt + (millis - row.BeforeStartAt)
		if mapped >= row.AfterEndAt {
			return nil, constUtil.ErrAdminConfiguration
		}
		if err := add(prefix+time.UnixMilli(mapped).In(zone).Format("2006-01-02"), old); err != nil {
			return nil, err
		}
	}
	for key, value := range current {
		prefix, date, _ := openingCarryDatedKey(key)
		if prefix == constUtil.OpenServerActivityCounterLogin {
			if value > 1 {
				return nil, constUtil.ErrAdminConfiguration
			}
			continue
		}
		at, _ := time.ParseInLocation("2006-01-02", date, zone)
		if at.UnixMilli() < row.AfterStartAt || at.UnixMilli() >= row.AfterEndAt {
			continue
		}
		if err := add(key, value-baseline[key]); err != nil {
			return nil, err
		}
	}
	for date := range logins {
		if err := add(constUtil.OpenServerActivityCounterLogin+date, 1); err != nil {
			return nil, err
		}
	}
	for key, value := range projected {
		out = append(out, db.OpenServerActivityCounter{PlayerID: playerID, CounterKey: key, Value: value})
	}
	return out, nil
}

func openingCalendarCarryLogin(tx *gorm.DB, playerID int64, cfg *db.OpenServerActivityRule, day string) error {
	row, err := loadOpeningCalendarCarry(tx, playerID, cfg, true)
	if err != nil || row == nil {
		return err
	}
	if !openingCarryTransaction(tx) {
		return constUtil.ErrAdminConfiguration
	}
	_, logins, err := openingCarryDecode(*row)
	if err != nil {
		return err
	}
	if logins[day] {
		return nil
	}
	logins[day] = true
	row.LoginDays = adminJSON(logins)
	row.Digest = openingCarryDigest(*row)
	if _, _, err := openingCarryDecode(*row); err != nil {
		return err
	}
	return tx.Model(&db.OpeningCalendarCarry{}).Where("player_id=? AND scope=?", row.PlayerID, row.Scope).Updates(map[string]any{"login_days": row.LoginDays, "digest": row.Digest}).Error
}
