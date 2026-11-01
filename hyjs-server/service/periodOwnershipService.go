package service

import (
	"context"
	"encoding/json"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
)

// Existing issued-instance and private-cycle records are the ownership source.
// Metadata stays on the activity row so event routing needs no new directory.
func (s *GameService) backfillP1Owners(ctx context.Context) error {
	d := db.GetIns().WithContext(ctx)
	if d.Migrator().HasTable(&db.AdminCharacterActivityInstance{}) {
		var last string
		for {
			var rows []db.AdminCharacterActivityInstance
			if err := d.Where("consumer_key>?", last).Order("consumer_key").Limit(256).Find(&rows).Error; err != nil {
				return err
			}
			if len(rows) == 0 {
				break
			}
			for _, row := range rows {
				if err := d.Model(&db.PeriodInstance{}).Where("id=? AND owner_player_id=0", row.ConsumerKey).
					Updates(map[string]any{"owner_player_id": row.PlayerID, "realm_id": row.ServerID}).Error; err != nil {
					return err
				}
			}
			last = rows[len(rows)-1].ConsumerKey
		}
	}
	if !d.Migrator().HasTable(&db.LocalPlayActivityCycle{}) {
		return nil
	}
	var last string
	for {
		var rows []db.LocalPlayActivityCycle
		if err := d.Where("id>?", last).Order("id").Limit(256).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		for _, row := range rows {
			var snapshot localPlayScopedSnapshot
			if json.Unmarshal(row.Snapshot, &snapshot) != nil {
				continue // Existing consumers retain their own malformed-snapshot handling.
			}
			for _, period := range snapshot.Periods {
				if localPlayPrivateCycleID(period.ID) != row.ID {
					continue
				}
				if err := d.Model(&db.PeriodInstance{}).Where("id=? AND owner_player_id=0", period.ID).
					Update("owner_player_id", row.PlayerID).Error; err != nil {
					return err
				}
			}
		}
		last = rows[len(rows)-1].ID
	}
}

// Candidate discovery uses this transaction's configuration snapshot. A newly
// published window is consumed by subsequent requests; selected existing rows
// are rechecked under their own lock before progress is changed.
func (s *GameService) p1EventPeriods(tx *gorm.DB, playerID int64, kind string) ([]db.PeriodInstance, error) {
	var candidates []db.PeriodInstance
	query := tx.Where("enabled=? AND owner_player_id IN ? AND realm_id IN ?", true, []int64{0, playerID}, []int64{0, s.OriginalServerID()}).
		Where("kind=? OR (frozen IS NOT NULL AND LENGTH(frozen)>0)", kind)
	if scope, _ := tx.Statement.Context.Value(constUtil.PeriodEventScopeContextKey{}).(string); scope != "" {
		query = query.Where("id=?", scope)
	}
	if err := query.Order("id").Find(&candidates).Error; err != nil {
		return nil, err
	}
	var result []db.PeriodInstance
	now := LocalPlayNow(tx.Statement.Context).UnixMilli()
	for _, candidate := range candidates {
		cfg, err := p1PeriodSnapshot(candidate)
		if err != nil {
			return nil, err
		}
		if cfg.Kind != kind || now < cfg.StartAt || now >= cfg.EndAt {
			continue
		}
		allowed, err := localPlayActivityVisible(tx, candidate.ID)
		if err != nil {
			return nil, err
		}
		if !allowed {
			continue
		}
		if p1GatherSnapshotRC(tx) && p1GatherCatalogKind(kind) {
			if len(candidate.Frozen) == 0 {
				return nil, constUtil.ErrP1ExclusiveRetry
			}
			result = append(result, candidate)
			continue
		}
		strength := "SHARE"
		if len(candidate.Frozen) == 0 {
			if p1SharedRead(tx) {
				return nil, constUtil.ErrP1ExclusiveRetry
			}
			strength = "UPDATE"
		}
		var live db.PeriodInstance
		if err = tx.Clauses(clause.Locking{Strength: strength}).First(&live, "id=?", candidate.ID).Error; err != nil {
			return nil, err
		}
		if strength == "SHARE" && len(live.Frozen) == 0 {
			return nil, constUtil.ErrConfigurationReadRetry
		}
		result = append(result, live)
	}
	return result, nil
}
