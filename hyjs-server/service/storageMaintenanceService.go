package service

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/db"
	"hyjs-server/model"
)

// These publications cannot be selected after expiry. Accepted fights create their
// settlement in the same transaction as their frozen replay; keep every such reference.
var unusedMatchTables = []struct{ match, settlement string }{
	{"p11_arena_matches", "p11_arena_fight_settlements"},
	{"p11_plunder_fight_matches", "p11_plunder_fight_settlements"},
	{"p11_limit_fight_matches", "p11_limit_fight_settlements"},
}

// CleanupUnusedExpiredMatches uses one fixed cutoff and a small locked batch. The
// caller explicitly selects apply; receipts, profiles and replays are never deleted.
func (s *GameService) CleanupUnusedExpiredMatches(ctx context.Context, cutoff time.Time, apply bool) ([]model.ExpiredMatchCleanup, error) {
	if ctx == nil || cutoff.IsZero() || cutoff.After(time.Now()) {
		return nil, errors.New("invalid cleanup cutoff")
	}
	results := make([]model.ExpiredMatchCleanup, 0, len(unusedMatchTables))
	for _, table := range unusedMatchTables {
		// Names are compile-time allowlisted, never request input.
		where := "expires_at < ? AND NOT EXISTS (SELECT 1 FROM " + table.settlement + " AS receipt WHERE receipt.match_id=" + table.match + ".id)"
		result := model.ExpiredMatchCleanup{Table: table.match}
		if err := db.GetIns().WithContext(ctx).Table(table.match).Where(where, cutoff.UnixMilli()).Count(&result.Eligible).Error; err != nil {
			return results, err
		}
		if apply {
			for {
				var removed int64
				err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
					// Acceptance locks these same publication rows before creating receipts.
					var ids []string
					if err := tx.Table(table.match).Clauses(clause.Locking{Strength: "UPDATE"}).Where(where, cutoff.UnixMilli()).Order("id ASC").Limit(32).Pluck("id", &ids).Error; err != nil {
						return err
					}
					if len(ids) == 0 {
						return nil
					}
					change := tx.Table(table.match).Where("id IN ?", ids).Where(where, cutoff.UnixMilli()).Delete(&db.P11ArenaMatch{})
					removed = change.RowsAffected
					return change.Error
				})
				result.Removed += removed
				if err != nil {
					return append(results, result), err
				}
				if removed < 32 {
					break
				}
			}
		}
		results = append(results, result)
	}
	return results, nil
}
