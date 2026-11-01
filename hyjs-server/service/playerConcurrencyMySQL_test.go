package service

import (
	"context"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
)

// Two player catalogues must finish while an unrelated seed and another
// reader remain held. An operator edit still waits for the selected rule.
func TestPlayerConcurrencyMySQLCommerceReaders(t *testing.T) {
	if os.Getenv("HYJS_P4_MYSQL_DATABASE") == "" {
		t.Skip("requires isolated hyjs_test_p4_ MySQL")
	}
	f := p4Setup(t)
	now := time.Now()
	if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		_, _, _, _, err := f.s.loadP4Commerce(tx, f.id, "", 1, now)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(f.ctx, 8*time.Second)
	defer cancel()
	held := db.GetIns().WithContext(ctx).Begin()
	defer held.Rollback()
	for _, key := range []string{constUtil.ReRushShopID, constUtil.P4PopularityRankRulePrefix + "local-v1"} {
		var row db.P4CommerceRule
		if err := held.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "id=?", key).Error; err != nil {
			t.Fatal(err)
		}
	}
	cfg, _, _, _, err := f.s.loadP4Commerce(held, f.id, "", 1, now)
	if err != nil {
		t.Fatal(err)
	}
	peer := make(chan error, 1)
	go func() {
		peer <- db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			_, _, _, _, err := f.s.loadP4Commerce(tx, f.id+1000, "", 1, now)
			return err
		})
	}()
	select {
	case err = <-peer:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("independent player catalogue waited for shared or unrelated rules")
	}
	writer := make(chan error, 1)
	go func() {
		writer <- db.GetIns().WithContext(ctx).Model(&db.P4CommerceRule{}).Where("id=?", cfg.ID).Update("enabled", false).Error
	}()
	select {
	case err = <-writer:
		t.Fatalf("operator changed a rule still in use: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	if err = held.Commit().Error; err != nil {
		t.Fatal(err)
	}
	if err = <-writer; err != nil {
		t.Fatal(err)
	}
}

func TestPlayerConcurrencyUnchangedCustomSave(t *testing.T) {
	f := p1Setup(t)
	var writes atomic.Int64
	callback := "player-concurrency-profile-writes"
	if err := db.GetIns().Callback().Update().After("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "profiles" {
			writes.Add(1)
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer db.GetIns().Callback().Update().Remove(callback)
	err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		var row db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, f.id).Error; err != nil {
			return err
		}
		player, game, err := decodeProfileForCalendarUpdate(tx, &row)
		if err != nil {
			return err
		}
		if err = saveProfile(tx, &row, player, game); err != nil {
			return err
		}
		writes.Store(0)
		return saveProfile(tx, &row, player, game)
	})
	if err != nil {
		t.Fatal(err)
	}
	if writes.Load() != 0 {
		t.Fatalf("unchanged custom save wrote profile %d times", writes.Load())
	}
}
