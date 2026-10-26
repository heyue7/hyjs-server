package service

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
)

func TestGGOAccountWeekly73ScopeRollbackAndReset(t *testing.T) {
	f := p1Setup(t)
	if err := initializeGGOAccountWeekly(db.GetIns()); err != nil {
		t.Fatal(err)
	}
	when := time.Date(2026, 10, 26, 0, 0, 0, 0, time.FixedZone("test", 8*3600))
	consume := func(owner int64, count int64, at time.Time) error {
		return db.GetIns().Transaction(func(tx *gorm.DB) error { return useGGOAccountWeekly(tx, owner, "holy-fragment", 3, count, at) })
	}
	if err := consume(10, 2, when); err != nil {
		t.Fatal(err)
	}
	rollback := errors.New("rollback")
	if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		if e := useGGOAccountWeekly(tx, 10, "holy-fragment", 3, 1, when); e != nil {
			return e
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	if err := consume(10, 1, when.Add(6*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := consume(10, 1, when); err == nil {
		t.Fatal("shared account exceeded weekly quota")
	}
	if err := consume(11, 3, when); err != nil {
		t.Fatal("other account blocked", err)
	}
	if err := consume(10, 3, when.Add(7*24*time.Hour)); err != nil {
		t.Fatal("Monday reset", err)
	}
	if err := consume(0, 1, when); err == nil {
		t.Fatal("unlinked account bypass")
	}
	if err := db.GetIns().Model(&db.Profile{}).Where("id=?", f.id).Update("account_id", 10).Error; err != nil {
		t.Fatal(err)
	}
	counts, owner, err := ggoWeeklyCatalogue(db.GetIns(), f.id, []p4Product{{ID: "holy-fragment", AccountWeeklyLimit: 3}}, when)
	if err != nil || owner != 10 || counts["holy-fragment"] != 3 {
		t.Fatal(counts, owner, err)
	}
	if ggoAccountWeek(when.Add(-time.Millisecond)) == ggoAccountWeek(when) {
		t.Fatal("Monday boundary did not change")
	}
}

func TestGGOAccountWeekly73MySQLConcurrent(t *testing.T) {
	_ = p1Setup(t)
	if db.GetIns().Dialector.Name() != "mysql" {
		t.Skip("real InnoDB concurrency requires managed MySQL")
	}
	if err := initializeGGOAccountWeekly(db.GetIns()); err != nil {
		t.Fatal(err)
	}
	var accepted atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := db.GetIns().Transaction(func(tx *gorm.DB) error { return useGGOAccountWeekly(tx, 22, "shared", 3, 1, time.Now()) })
			if err == nil {
				accepted.Add(1)
			}
		}()
	}
	wg.Wait()
	if accepted.Load() != 3 {
		t.Fatalf("accepted %d, want3", accepted.Load())
	}
	var row db.GGOAccountWeekly
	if err := db.AccountsTable(db.GetIns(), constUtil.GGOAccountWeeklyTable).Where("account_id=? AND product_id=?", 22, "shared").First(&row).Error; err != nil || row.Bought != 3 {
		t.Fatal(row, err)
	}
}

func TestGGOTreasureFragmentMetadata73(t *testing.T) {
	f := p1Setup(t)
	for _, id := range []string{"frag_trea_015_1", "frag_trea_015_6", "frag_equip_037"} {
		row := f.s.ggoRewardMetadata(id)
		if row == nil || row["name"] == "" || row["icon"] == "" {
			t.Fatal(id, row)
		}
	}
}
