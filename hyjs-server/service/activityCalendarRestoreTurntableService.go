package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
)

// RestoreActivityCalendarTurntableWindow joins the caller's clock/profile-locked
// transaction. The caller must stop competing settlement workers and abort the
// whole transaction on error. Only the pool's three calendar columns are written;
// the caller handles NextFree once through the existing clock timer migration.
func RestoreActivityCalendarTurntableWindow(tx *gorm.DB, poolID string, before, after FrozenJSONWindow) error {
	fail := func(reason string) error {
		return fmt.Errorf("turntable %s calendar restore: %s: %w", poolID, reason, constUtil.ErrAdminConfiguration)
	}
	valid := func(w FrozenJSONWindow) bool {
		return w.StartAt > 0 && w.EndAt > w.StartAt && w.ClaimEndAt >= w.EndAt && w.ClaimEndAt <= constUtil.ActivityCalendarRestoreMaxMillis
	}
	if tx == nil || tx.Statement == nil || poolID == "" || len(poolID) > 64 || !valid(before) || !valid(after) {
		return fail("invalid transaction, identity or window")
	}
	if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return fail("caller transaction required")
	}
	var pool db.GameplayPool
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?", poolID).Take(&pool).Error; err != nil {
		return err
	}
	if (FrozenJSONWindow{pool.StartAt, pool.EndAt, pool.RewardEndAt}) != before {
		return constUtil.ErrAdminConfigConflict
	}
	if pool.FreePerDay < -1 || pool.FreePerDay > 0 || pool.FreeSeconds < 1 || pool.FreeSeconds > 31536000 || pool.MinLevel < 1 || pool.SingleCost != "diamond,50" || pool.TenCost != "diamond,488" || pool.CoinPerDraw != 10000 || pool.GuaranteeEvery < 0 || pool.GuaranteeEvery > 10000 || pool.InitialDiamondPool < 0 || pool.InitialDiamondPool > math.MaxInt32 || pool.JackpotPerMillion < 0 || pool.JackpotPerMillion > 1000000 || pool.PoolContributionBps < 0 || pool.PoolContributionBps > 10000 {
		return fail("unsupported pool rules or daily free quota")
	}
	var rules db.TurntableRules
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("pool_id=?", poolID).Take(&rules).Error; err != nil {
		return err
	}
	if rules.ScorePerDraw <= 0 {
		return fail("invalid score rules")
	}
	if _, err := turntablePityRules(rules.PrizePityRules, pool.JackpotPerMillion); err != nil {
		return err
	}
	var rewards []db.GameplayPoolReward
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("pool_id=?", poolID).Order("id").Limit(10).Find(&rewards).Error; err != nil {
		return err
	}
	if len(rewards) != 9 {
		return fail("expected nine reward positions")
	}
	for i, reward := range rewards {
		if reward.ID != int32(i) || reward.ResourceID == "" || reward.Amount <= 0 || reward.Weight <= 0 || reward.Weight > 1000000000000 || reward.Stock < 0 {
			return fail("invalid reward position")
		}
	}
	stockValid := func(raw []byte) bool {
		var stock map[int32]int32
		if turntableRestoreDecode(raw, &stock) != nil || stock == nil {
			return false
		}
		for key, used := range stock {
			if key < 0 || key >= int32(len(rewards)) || used < 0 || (rewards[key].Stock > 0 && used > rewards[key].Stock) {
				return false
			}
		}
		return true
	}
	amountsValid := func(pool, remainder int64) bool {
		return pool >= 0 && pool <= math.MaxInt32 && remainder >= 0 && remainder < 10000
	}
	var global db.TurntableGlobal
	globalErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("pool_id=?", poolID).Take(&global).Error
	if globalErr != nil && !errors.Is(globalErr, gorm.ErrRecordNotFound) {
		return globalErr
	}
	// Character pools legitimately have no shared row: runtime lazily creates a
	// Personal row from the owner's records and projects that as its global view.
	// Do not create a shared inventory or settlement authority during migration.
	if globalErr == nil && (global.Settled || !amountsValid(global.DiamondPool, global.ContributionRemainder) || !stockValid(global.UsedStock)) {
		return fail("settled or invalid global state")
	}
	// Unsettled pools must not contain a prior final ranking. Both historical nil
	// and the explicit empty array are legitimate empty representations.
	if globalErr == nil && len(bytes.TrimSpace(global.FinalRanks)) > 0 {
		var ranks []json.RawMessage
		if turntableRestoreDecode(global.FinalRanks, &ranks) != nil || len(ranks) > 0 {
			return fail("unexpected final ranking")
		}
	}
	var personals []db.TurntablePersonal
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("pool_id=?", poolID).Order("player_id").Limit(10001).Find(&personals).Error; err != nil {
		return err
	}
	if len(personals) > 10000 {
		return fail("personal state limit exceeded")
	}
	for _, personal := range personals {
		if personal.PlayerID <= 0 || personal.Settled || personal.LegacyScore < 0 || personal.CreatedAt <= 0 || !amountsValid(personal.DiamondPool, personal.ContributionRemainder) || !stockValid(personal.UsedStock) {
			return fail("settled or invalid personal state")
		}
	}
	var states []db.TurntableState
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("pool_id=?", poolID).Order("player_id").Limit(10001).Find(&states).Error; err != nil {
		return err
	}
	if len(states) > 10000 {
		return fail("draw state limit exceeded")
	}
	for _, state := range states {
		if state.PlayerID <= 0 || state.Score < 0 || state.BoughtCoins < 0 || state.NextFree < 0 || state.FreeUsed < 0 || state.Timestamp < 0 || (state.RequestCount != 0 && state.RequestCount != 1 && state.RequestCount != 10) {
			return fail("invalid draw state")
		}
		if state.FreeDay != "" && !fieldCalendarDayValid75(state.FreeDay) {
			return fail("invalid free day")
		}
		if state.PrizePityState != "" {
			var progress map[int32]turntablePityProgress
			if turntableRestoreDecode([]byte(state.PrizePityState), &progress) != nil || progress == nil {
				return fail("invalid pity state")
			}
			for key, saved := range progress {
				if key < 0 || key > 9 || saved.Since < 0 || saved.Key == "" {
					return fail("invalid pity progress")
				}
			}
		}
	}
	if before == after {
		return nil
	}
	result := tx.Model(&db.GameplayPool{}).Where("id=? AND start_at=? AND end_at=? AND reward_end_at=?", poolID, before.StartAt, before.EndAt, before.ClaimEndAt).Updates(map[string]any{"start_at": after.StartAt, "end_at": after.EndAt, "reward_end_at": after.ClaimEndAt})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return constUtil.ErrAdminConfigConflict
	}
	return nil
}

func turntableRestoreDecode(raw []byte, target any) error {
	// Reuse the strict token reader to reject duplicate keys before decoding the
	// typed payload; unknown fields and trailing JSON are rejected as well.
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
