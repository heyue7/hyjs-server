package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"hyjs-server/constUtil"
	"hyjs-server/db"
)

type p1LoginReconcileTrace struct {
	logger.Interface
	occurrences int
}

func (trace *p1LoginReconcileTrace) Trace(_ context.Context, _ time.Time, statement func() (string, int64), _ error) {
	query, _ := statement()
	if strings.Contains(strings.ToLower(query), "admin_activity_occurrences") {
		trace.occurrences++
	}
}

// 用真实登录事实和大量其他活动验证登录补算与日程查询量。
func TestP1LoginReconcileRelevantWindows(t *testing.T) {
	f := p1Setup(t)
	if err := db.GetIns().AutoMigrate(&db.AdminActivityOccurrence{}, &db.AdminActivityScheduleRevision{}); err != nil {
		t.Fatal(err)
	}
	f.s.adminActivitySchemaReady.Store(true)
	f.ctx = context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	now := time.Now()
	yesterday := now.AddDate(0, 0, -1)
	f.event(t, "login:"+p1Day(yesterday), constUtil.P1EventLoginDay, 1, yesterday)
	f.event(t, "login:"+p1Day(now), constUtil.P1EventLoginDay, 1, now)
	active := db.PeriodInstance{ID: "login_reconcile_latency", Enabled: true, Title: "login test", Kind: constUtil.P1EventLoginDay, Mode: "cumulative", Goal: 2, Rewards: "coin,1", StartAt: now.AddDate(0, 0, -3).UnixMilli(), EndAt: now.Add(time.Hour).UnixMilli(), RewardEndAt: now.Add(2 * time.Hour).UnixMilli(), Source: "isolated login test"}
	frozen, err := json.Marshal(active)
	if err != nil {
		t.Fatal(err)
	}
	// 运营实时时间已经关闭，旧快照仍有效，必须继续补算真实登录日。
	live := active
	live.StartAt, live.EndAt, live.RewardEndAt, live.Frozen = now.Add(-6*time.Hour).UnixMilli(), now.Add(-5*time.Hour).UnixMilli(), now.Add(-4*time.Hour).UnixMilli(), frozen
	rows := []db.PeriodInstance{live}
	for i := 0; i < 700; i++ {
		row := active
		row.ID = fmt.Sprintf("other_activity_%04d", i)
		row.Kind, row.Goal = constUtil.P1EventCopyWin, 1
		rows = append(rows, row)
	}
	if err = db.GetIns().CreateInBatches(rows, 100).Error; err != nil {
		t.Fatal(err)
	}
	previous := db.GetIns().Config.Logger
	trace := &p1LoginReconcileTrace{Interface: previous}
	db.GetIns().Config.Logger = trace
	started := time.Now()
	err = db.GetIns().WithContext(f.ctx).Transaction(func(tx *gorm.DB) error {
		return f.s.p1ReconcileLoginPeriods(tx, f.id, now)
	})
	elapsed := time.Since(started)
	db.GetIns().Config.Logger = previous
	if err != nil {
		t.Fatal(err)
	}
	progress := p1ProgressValue(t, f, active.ID)
	if progress.Value != 2 || progress.RewardID == 0 || p1Count(t, &db.RewardDelivery{}, "player_id=? AND source_key=?", f.id, "activity:"+active.ID) != 1 {
		t.Fatalf("real login days or reward changed: %+v", progress)
	}
	if trace.occurrences > 5 {
		t.Fatalf("unrelated windows caused %d activity occurrence queries", trace.occurrences)
	}
	t.Logf("700 unrelated windows: activity occurrence queries=%d elapsed=%s", trace.occurrences, elapsed)
}
