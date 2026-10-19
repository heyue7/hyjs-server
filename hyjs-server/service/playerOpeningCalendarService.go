package service

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"time"
)

// InitializePlayerOpeningCalendar migrates the date anchor without altering previous rewards.
// Existing roles use their persisted creation day; future clock jumps never rewrite this anchor.
func (s *GameService) InitializePlayerOpeningCalendar(ctx context.Context) error {
	d := db.GetIns().WithContext(ctx)
	if err := d.AutoMigrate(&db.PlayerOpeningCalendar{}); err != nil {
		return err
	}
	var roles []db.Profile
	if err := d.Select("id", "created_at").Where("player IS NOT NULL AND created_at > 0").Find(&roles).Error; err != nil {
		return err
	}
	for _, row := range roles {
		// Historic creation timestamps are real dates; their old offset was not recorded.
		// Keep that evidence rather than shifting the birthday every deployment.
		if err := insertPlayerOpeningCalendar(d, row.Id, time.UnixMilli(*row.CreatedAt), time.Now()); err != nil {
			return err
		}
	}
	return nil
}

func insertPlayerOpeningCalendar(tx *gorm.DB, id int64, gameAt, realAt time.Time) error {
	if GameAccountProtectedProfile(tx, id) {
		return nil
	}
	zone := time.FixedZone("UTC+8", 8*3600)
	local := gameAt.In(zone)
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, zone)
	row := db.PlayerOpeningCalendar{PlayerID: id, StartAt: start.UnixMilli(), CreatedAt: realAt.UnixMilli(), Source: constUtil.PlayerOpeningCalendarSource}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error
}

// loadPlayerOpenServerRule projects the same original task windows onto this character's day one.
// Old personal activity cycles retain their frozen windows and receipts.
func loadPlayerOpenServerRule(tx *gorm.DB, id int64) (*db.OpenServerActivityRule, error) {
	cfg, err := loadOpenServerRule(tx)
	if err != nil {
		return nil, err
	}
	cycle, err := localPlayOpenServerCycle(tx)
	if err != nil {
		return nil, err
	}
	if cycle != nil || GameAccountProtectedProfile(tx, id) || !tx.Migrator().HasTable(&db.PlayerOpeningCalendar{}) {
		return cfg, nil
	}
	var anchor db.PlayerOpeningCalendar
	err = tx.First(&anchor, "player_id=?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return cfg, nil
	}
	if err != nil {
		return nil, err
	}
	if anchor.Source != constUtil.PlayerOpeningCalendarSource || anchor.StartAt <= 0 {
		return nil, constUtil.ErrAdminConfiguration
	}
	duration, grace := cfg.EndAt-cfg.StartAt, cfg.RewardEndAt-cfg.EndAt
	cfg.StartAt = anchor.StartAt
	cfg.EndAt = anchor.StartAt + duration
	cfg.RewardEndAt = cfg.EndAt + grace
	if err := AdminScheduledOpeningRule(tx, cfg, LocalPlayNow(tx.Statement.Context).UnixMilli(), id); err != nil {
		return nil, err
	}
	return cfg, nil
}
