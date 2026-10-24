package service

import (
	"context"
	"fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/db"
	"strings"
)

type adminOpeningScopeKey struct{}
type adminOpeningScope struct {
	OccurrenceID string
	PlayerID     int64
}

func adminOpeningPeriod(tx *gorm.DB) *adminOpeningScope {
	scope, _ := tx.Statement.Context.Value(adminOpeningScopeKey{}).(*adminOpeningScope)
	return scope
}
func adminBindOpeningPeriod(tx *gorm.DB, issued db.AdminCharacterActivityInstance) {
	var scope *adminOpeningScope
	if strings.HasPrefix(issued.ConsumerKey, "opening_period_") {
		scope = &adminOpeningScope{OccurrenceID: issued.OccurrenceID, PlayerID: issued.PlayerID}
	}
	tx.Statement.Context = context.WithValue(tx.Statement.Context, adminOpeningScopeKey{}, scope)
}

// A repeatable opening source uses its own native task ledger. The first legacy
// once source retains all existing claims; subsequent periods never clear them.
func adminResolveOpeningSchedule(tx *gorm.DB, cfg *db.OpenServerActivityRule, now, playerID int64, rows []db.AdminActivityOccurrence) error {
	tx.Statement.Context = context.WithValue(tx.Statement.Context, adminOpeningScopeKey{}, (*adminOpeningScope)(nil))
	baseStart, baseEnabled := cfg.StartAt, cfg.Enabled
	// Do not range-lock an empty issued set: different first-time roles may
	// share its InnoDB gap. The profile lock and unique insert/current read
	// below serialize each actual acceptance without that cross-role cycle.
	var accepted []db.AdminCharacterActivityInstance
	if playerID > 0 && gameSchemaHasTable(tx, &db.AdminCharacterActivityInstance{}) {
		if err := tx.Where("player_id=? AND server_id=? AND mode=?", playerID, rows[0].ServerID, "opening-14").Order("start_at,occurrence_id").Find(&accepted).Error; err != nil {
			return err
		}
	}
	byID := map[string]db.AdminCharacterActivityInstance{}
	for _, issued := range accepted {
		byID[issued.OccurrenceID] = issued
		// Backfill acceptance for an already-issued pre-policy opening identity.
		for _, source := range rows {
			if source.ID == issued.OccurrenceID {
				source.FormJSON = issued.FormJSON // the accepted source snapshot is authoritative
				if _, err := adminAcceptActivityTemplate(tx, playerID, source); err != nil {
					return err
				}
				break
			}
		}
	}
	cfg.Enabled = false
	for _, source := range rows {
		if !source.Enabled {
			continue
		}
		start, duration, grace := adminCharacterScheduleDays(source)
		candidate := db.AdminCharacterActivityInstance{PlayerID: playerID, OccurrenceID: source.ID, ServerID: source.ServerID, Mode: "opening-14", StartAt: baseStart + (start-1)*86400000, SourceVersion: adminVersion(source), FormJSON: append([]byte{}, source.FormJSON...)}
		candidate.EndAt = candidate.StartAt + duration*86400000
		candidate.ClaimEndAt = candidate.EndAt + grace*86400000
		if old, ok := byID[source.ID]; ok {
			candidate = old
		}
		if now < candidate.StartAt || now >= candidate.ClaimEndAt {
			continue
		}
		if playerID > 0 && candidate.ConsumerKey == "" {
			allowed, err := adminAcceptActivityTemplate(tx, playerID, source)
			if err != nil {
				return err
			}
			if !allowed {
				continue
			}
			policy, err := adminActivityOccurrenceRepeatPolicy(source)
			if err != nil {
				return err
			}
			key := fmt.Sprintf("opening_character_%d_%d", source.ServerID, playerID)
			if policy == "repeat" || len(accepted) > 0 {
				key = "opening_period_" + p6Digest([]byte(fmt.Sprintf("%d/%d/%s", source.ServerID, playerID, source.ID)))[:24]
			}
			candidate.ConsumerKey = key
			candidate.ConsumerID = "opening:" + key
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&candidate).Error; err != nil {
				return err
			}
			if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).Where("player_id=? AND occurrence_id=?", playerID, source.ID).Take(&candidate).Error; err != nil {
				return err
			}
		}
		cfg.StartAt, cfg.EndAt, cfg.RewardEndAt = candidate.StartAt, candidate.EndAt, candidate.ClaimEndAt
		cfg.Enabled = baseEnabled
		if playerID > 0 {
			adminBindOpeningPeriod(tx, candidate)
		}
		return nil
	}
	// Keep stopped/expired accepted TTL visible to callers. A new source blocked
	// by once policy cannot silently rewrite the old participation window.
	if len(accepted) > 0 {
		last := accepted[len(accepted)-1]
		cfg.StartAt, cfg.EndAt, cfg.RewardEndAt = last.StartAt, last.EndAt, last.ClaimEndAt
		adminBindOpeningPeriod(tx, last)
	}
	return nil
}

func adminOpeningReadStates(tx *gorm.DB, id int64) ([]db.OpenServerActivityState, error) {
	scope := adminOpeningPeriod(tx)
	var rows []db.AdminOpeningState
	err := tx.Where("occurrence_id=? AND player_id=?", scope.OccurrenceID, id).Find(&rows).Error
	out := make([]db.OpenServerActivityState, 0, len(rows))
	for _, r := range rows {
		out = append(out, db.OpenServerActivityState{PlayerID: id, TaskID: r.TaskID, Achieved: r.Achieved, Progress: r.Progress, Claims: r.Claims})
	}
	return out, err
}
func adminOpeningSaveState(tx *gorm.DB, state *db.OpenServerActivityState) error {
	scope := adminOpeningPeriod(tx)
	return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "occurrence_id"}, {Name: "player_id"}, {Name: "task_id"}}, DoUpdates: clause.AssignmentColumns([]string{"achieved", "progress", "claims"})}).Create(&db.AdminOpeningState{OccurrenceID: scope.OccurrenceID, PlayerID: state.PlayerID, TaskID: state.TaskID, Achieved: state.Achieved, Progress: state.Progress, Claims: state.Claims}).Error
}
func adminOpeningReadCounters(tx *gorm.DB, id int64) ([]db.OpenServerActivityCounter, error) {
	scope := adminOpeningPeriod(tx)
	var rows []db.AdminOpeningCounter
	err := tx.Where("occurrence_id=? AND player_id=?", scope.OccurrenceID, id).Find(&rows).Error
	out := make([]db.OpenServerActivityCounter, 0, len(rows))
	for _, r := range rows {
		out = append(out, db.OpenServerActivityCounter{PlayerID: id, CounterKey: r.CounterKey, Value: r.Value})
	}
	return out, err
}
func adminOpeningSaveCounter(tx *gorm.DB, row *db.OpenServerActivityCounter) error {
	scope := adminOpeningPeriod(tx)
	return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "occurrence_id"}, {Name: "player_id"}, {Name: "counter_key"}}, DoUpdates: clause.AssignmentColumns([]string{"value"})}).Create(&db.AdminOpeningCounter{OccurrenceID: scope.OccurrenceID, PlayerID: row.PlayerID, CounterKey: row.CounterKey, Value: row.Value}).Error
}
func adminOpeningTaskStock(tx *gorm.DB, tasks []db.OpenServerActivityTask, locking ...bool) error {
	scope := adminOpeningPeriod(tx)
	if len(locking) > 0 && locking[0] {
		// Called only after the original task FOR UPDATE. Materialize an absent
		// row under that serialization, then use a current read after any wait;
		// a plain RR snapshot here could sell the final unit twice.
		for i := range tasks {
			stock := db.AdminOpeningStock{OccurrenceID: scope.OccurrenceID, TaskID: tasks[i].ID}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&stock).Error; err != nil {
				return err
			}
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&stock, "occurrence_id=? AND task_id=?", scope.OccurrenceID, tasks[i].ID).Error; err != nil {
				return err
			}
			tasks[i].UsedStock = stock.UsedStock
		}
		return nil
	}
	var stocks []db.AdminOpeningStock
	if err := tx.Where("occurrence_id=?", scope.OccurrenceID).Find(&stocks).Error; err != nil {
		return err
	}
	amounts := map[int32]int32{}
	for _, r := range stocks {
		amounts[r.TaskID] = r.UsedStock
	}
	for i := range tasks {
		tasks[i].UsedStock = amounts[tasks[i].ID]
	}
	return nil
}
func adminOpeningSaveStock(tx *gorm.DB, task db.OpenServerActivityTask) error {
	scope := adminOpeningPeriod(tx)
	return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "occurrence_id"}, {Name: "task_id"}}, DoUpdates: clause.AssignmentColumns([]string{"used_stock"})}).Create(&db.AdminOpeningStock{OccurrenceID: scope.OccurrenceID, TaskID: task.ID, UsedStock: task.UsedStock}).Error
}

// Receipt stamps remain globally unique per role, including across periods.
// Lookup happens before selecting today's programme, so old stamps only replay.
func adminOpeningReadReceipt(tx *gorm.DB, id, stamp int64) (db.OpenServerActivityReceipt, error) {
	var row db.AdminOpeningReceipt
	err := tx.First(&row, "player_id=? AND timestamp=?", id, stamp).Error
	return db.OpenServerActivityReceipt{PlayerID: id, Timestamp: stamp, Request: row.Request, Response: row.Response}, err
}
func adminOpeningCreateReceipt(tx *gorm.DB, row *db.OpenServerActivityReceipt) error {
	scope := adminOpeningPeriod(tx)
	return tx.Create(&db.AdminOpeningReceipt{PlayerID: row.PlayerID, Timestamp: row.Timestamp, OccurrenceID: scope.OccurrenceID, Request: row.Request, Response: row.Response}).Error
}
