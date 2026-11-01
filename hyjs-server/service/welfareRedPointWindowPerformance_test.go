package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm/logger"
	"hyjs-server/constUtil"
	"hyjs-server/db"
)

type redPointWindowSQLTrace struct {
	logger.Interface
	count int
}

func (trace *redPointWindowSQLTrace) Trace(_ context.Context, _ time.Time, statement func() (string, int64), _ error) {
	query, _ := statement()
	if strings.Contains(strings.ToLower(query), "admin_activity_occurrences") {
		trace.count++
	}
}

// 过期窗口数量接近当前线上目录，同时覆盖冻结时间与现行时间不一致。
func TestWelfareRedPointWindowSnapshotAndQueryCount(t *testing.T) {
	f := p1Setup(t)
	if err := db.GetIns().AutoMigrate(&db.AdminActivityOccurrence{}, &db.AdminActivityScheduleRevision{}); err != nil {
		t.Fatal(err)
	}
	f.s.adminActivitySchemaReady.Store(true)
	f.ctx = context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	now := time.Now()
	active := db.PeriodInstance{ID: "kaiGongActivity_windowtest", Enabled: true, Title: "window test", Kind: constUtil.P1EventCopyWin, Mode: "cumulative", Goal: 1, Rewards: "coin,1", StartAt: now.Add(-time.Hour).UnixMilli(), EndAt: now.Add(time.Hour).UnixMilli(), RewardEndAt: now.Add(2 * time.Hour).UnixMilli(), Source: "isolated window test"}
	frozen, err := json.Marshal(active)
	if err != nil {
		t.Fatal(err)
	}
	// 当前行过期，但已经冻结的活动仍在窗口内。
	live := active
	live.StartAt, live.EndAt, live.RewardEndAt, live.Frozen = now.Add(-4*time.Hour).UnixMilli(), now.Add(-3*time.Hour).UnixMilli(), now.Add(-2*time.Hour).UnixMilli(), frozen
	rows := []db.PeriodInstance{live}
	// 实时行仍有效、但冻结窗口过期时不得点亮；领奖宽限期仍须点亮。
	expired := active
	expired.ID, expired.StartAt, expired.EndAt, expired.RewardEndAt = "pvpWinTimes_windowtest", live.StartAt, live.EndAt, live.RewardEndAt
	expiredSnapshot, err := json.Marshal(expired)
	if err != nil {
		t.Fatal(err)
	}
	expiredLive := active
	expiredLive.ID, expiredLive.Frozen = expired.ID, expiredSnapshot
	grace := active
	grace.ID, grace.EndAt = "robTreasureTimes_windowtest", now.Add(-time.Minute).UnixMilli()
	graceSnapshot, err := json.Marshal(grace)
	if err != nil {
		t.Fatal(err)
	}
	rows = append(rows, expiredLive, grace)
	for i := 0; i < 700; i++ {
		row := active
		row.ID = fmt.Sprintf("windowexpired_%04d", i)
		row.StartAt, row.EndAt, row.RewardEndAt = live.StartAt, live.EndAt, live.RewardEndAt
		rows = append(rows, row)
	}
	if err = db.GetIns().CreateInBatches(rows, 100).Error; err != nil {
		t.Fatal(err)
	}
	progress := []db.PeriodProgress{{InstanceID: active.ID, PlayerID: f.id, Snapshot: frozen, Value: 1}, {InstanceID: expired.ID, PlayerID: f.id, Snapshot: expiredSnapshot, Value: 1}, {InstanceID: grace.ID, PlayerID: f.id, Snapshot: graceSnapshot, Value: 1}}
	if err = db.GetIns().Create(&progress).Error; err != nil {
		t.Fatal(err)
	}
	previous := db.GetIns().Config.Logger
	trace := &redPointWindowSQLTrace{Interface: previous}
	db.GetIns().Config.Logger = trace
	started := time.Now()
	values := map[string]bool{}
	err = f.s.periodReadyPoints(db.GetIns().WithContext(f.ctx), f.id, values)
	elapsed := time.Since(started)
	db.GetIns().Config.Logger = previous
	if err != nil || !values["kaiGongActivity"] || !values["robTreasureTimes"] || values["pvpWinTimes"] {
		t.Fatalf("frozen active period lost: %v %v", values, err)
	}
	if trace.count > 4 {
		t.Fatalf("expired windows caused %d activity occurrence queries", trace.count)
	}
	t.Logf("700 expired windows: activity occurrence queries=%d elapsed=%s", trace.count, elapsed)
}
