package service

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"hyjs-server/db"
)

var errP11ArenaDailyMaintenance = errors.New("arena daily maintenance required")

const p11ArenaTransactionMaxAttempts = 3

// A maintenance request rolls the entire attempt back before Tick acquires
// the exclusive arena lock. The caller must reset attempt-local output itself.
func (s *GameService) p11ArenaTransaction(ctx context.Context, now time.Time, fn func(*gorm.DB) error) error {
	for attempt := 0; attempt < p11ArenaTransactionMaxAttempts; attempt++ {
		err := db.GetIns().WithContext(ctx).Transaction(fn)
		if !errors.Is(err, errP11ArenaDailyMaintenance) || attempt+1 == p11ArenaTransactionMaxAttempts {
			return err
		}
		if err := s.TickP11ArenaDaily(ctx, now); err != nil {
			return err
		}
	}
	return errP11ArenaDailyMaintenance
}
