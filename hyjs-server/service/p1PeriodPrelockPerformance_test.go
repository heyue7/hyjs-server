package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
)

func TestP1PeriodPrelockMySQLInactiveRowsRemainLocked(t *testing.T) {
	if os.Getenv("HYJS_P1_MYSQL_DATABASE") == "" {
		t.Skip("requires an isolated hyjs_test_p1_ MySQL database")
	}
	f := p1Setup(t)
	period := p1Instance(t, f, "inactive-prelock", "prelock", "cumulative", 1, time.Now().Add(time.Hour), 0)
	if err := db.GetIns().Model(&period).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(f.ctx, 10*time.Second)
	defer cancel()
	ctx = context.WithValue(ctx, constUtil.P1SharedReadContextKey{}, true)
	tx := db.GetIns().WithContext(ctx).Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer tx.Rollback()
	if err := p1LockPeriodIDs(tx); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		done <- db.GetIns().WithContext(ctx).Model(&db.PeriodInstance{}).Where("id=?", period.ID).Update("title", "changed").Error
	}()
	select {
	case err := <-done:
		t.Fatalf("inactive activity escaped the shared prelock: %v", err)
	case <-time.After(250 * time.Millisecond):
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("inactive activity remained locked after commit")
	}
}

func TestP1PeriodPrelockMySQLInsertGuardMatchesFullRead(t *testing.T) {
	if os.Getenv("HYJS_P1_MYSQL_DATABASE") == "" {
		t.Skip("requires an isolated hyjs_test_p1_ MySQL database")
	}
	f := p1Setup(t)
	ctx, cancel := context.WithTimeout(f.ctx, 10*time.Second)
	defer cancel()
	ctx = context.WithValue(ctx, constUtil.P1SharedReadContextKey{}, true)
	probe := func(full bool, id string) bool {
		t.Helper()
		tx := db.GetIns().WithContext(ctx).Begin()
		if tx.Error != nil {
			t.Fatal(tx.Error)
		}
		defer tx.Rollback()
		var err error
		if full {
			_, err = p1LockPeriods(tx)
		} else {
			err = p1LockPeriodIDs(tx)
		}
		if err != nil {
			t.Fatal(err)
		}
		rollback := errors.New("rollback isolated insert")
		done := make(chan error, 1)
		go func() {
			done <- db.GetIns().WithContext(ctx).Transaction(func(writer *gorm.DB) error {
				row := db.PeriodInstance{ID: id, Title: "isolated", Kind: "prelock", Mode: "cumulative", Source: "isolated"}
				if err := writer.Create(&row).Error; err != nil {
					return err
				}
				return rollback
			})
		}()
		blocked := false
		select {
		case err := <-done:
			if !errors.Is(err, rollback) {
				t.Fatal(err)
			}
		case <-time.After(250 * time.Millisecond):
			blocked = true
		}
		if err := tx.Commit().Error; err != nil {
			t.Fatal(err)
		}
		if blocked {
			select {
			case err := <-done:
				if !errors.Is(err, rollback) {
					t.Fatal(err)
				}
			case <-ctx.Done():
				t.Fatal("insert remained locked after commit")
			}
		}
		return blocked
	}
	full := probe(true, "zz-prelock-full")
	narrow := probe(false, "zz-prelock-narrow")
	if full != narrow {
		t.Fatalf("new-row lock changed: full blocked=%v, ID-only blocked=%v", full, narrow)
	}
}

// 同一隔离库、同一活动集对比完整预锁与仅主键预锁，结果仅描述本地样本。
func TestP1PeriodPrelockMySQLPayloadAndLatency(t *testing.T) {
	if os.Getenv("HYJS_P1_MYSQL_DATABASE") == "" {
		t.Skip("requires an isolated hyjs_test_p1_ MySQL database")
	}
	f := p1Setup(t)
	now := time.Now()
	rows := make([]db.PeriodInstance, 765)
	for i := range rows {
		rows[i] = db.PeriodInstance{ID: fmt.Sprintf("prelock-perf-%04d", i), Title: "isolated", Kind: "prelock", Mode: "cumulative", Goal: 1, Source: strings.Repeat("x", 620), Rewards: "coin,1", StartAt: now.UnixMilli(), EndAt: now.Add(time.Hour).UnixMilli(), RewardEndAt: now.Add(2 * time.Hour).UnixMilli()}
	}
	if err := db.GetIns().CreateInBatches(rows, 50).Error; err != nil {
		t.Fatal(err)
	}
	measure := func(full bool) time.Duration {
		start := time.Now()
		err := db.GetIns().WithContext(f.ctx).Transaction(func(tx *gorm.DB) error {
			if full {
				_, err := p1LockPeriods(tx)
				return err
			}
			return p1LockPeriodIDs(tx)
		})
		if err != nil {
			t.Fatal(err)
		}
		return time.Since(start)
	}
	measure(true)
	measure(false)
	var full, narrow []time.Duration
	for i := 0; i < 20; i++ {
		if i%2 == 0 {
			full, narrow = append(full, measure(true)), append(narrow, measure(false))
		} else {
			narrow, full = append(narrow, measure(false)), append(full, measure(true))
		}
	}
	sort.Slice(full, func(i, j int) bool { return full[i] < full[j] })
	sort.Slice(narrow, func(i, j int) bool { return narrow[i] < narrow[j] })
	t.Logf("isolated 765 periods: full p50=%s p95=%s; ID-only p50=%s p95=%s", full[9], full[18], narrow[9], narrow[18])
}
