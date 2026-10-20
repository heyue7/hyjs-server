package service

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	gormlogger "gorm.io/gorm/logger"

	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

// adminActivityQueryRecorder 记录实际执行的 SQL，用于确认奖池奖励读取不再逐池查询。
type adminActivityQueryRecorder struct {
	gormlogger.Interface
	mu   sync.Mutex
	sqls []string
}

func (r *adminActivityQueryRecorder) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, _ := fc()
	r.mu.Lock()
	r.sqls = append(r.sqls, sql)
	r.mu.Unlock()
}

func (r *adminActivityQueryRecorder) countContaining(fragment string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, sql := range r.sqls {
		if strings.Contains(sql, fragment) {
			n++
		}
	}
	return n
}

func (r *adminActivityQueryRecorder) reset() {
	r.mu.Lock()
	r.sqls = nil
	r.mu.Unlock()
}

// adminActivitiesSQLite 建立真实 SQLite 表，供奖池奖励读取的行为与查询数检查复用。
func adminActivitiesSQLite(t *testing.T) context.Context {
	t.Helper()
	ctx := context.Background()
	if err := db.InitGame(ctx, filepath.Join(t.TempDir(), "admin-activities.sqlite")); err != nil {
		t.Fatal(err)
	}
	connection, err := db.GetIns().DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { connection.Close() })
	if err := db.GetIns().WithContext(ctx).AutoMigrate(&db.GameplayPool{}, &db.GameplayPoolReward{}, &db.P4CommerceRule{}, &db.P4WelfareRule{}, &db.PeriodInstance{}, &db.P4ActivityRegistry{}); err != nil {
		t.Fatal(err)
	}
	return ctx
}

// legacyAdminActivities 保留逐池查询的旧算法，作为响应等价检查的独立基准。
func legacyAdminActivities(t *testing.T, ctx context.Context) []model.AdminActivityDTO {
	t.Helper()
	d := db.GetIns().WithContext(ctx)
	out := []model.AdminActivityDTO{}
	var pools []db.GameplayPool
	var commerce []db.P4CommerceRule
	var welfare []db.P4WelfareRule
	var periods []db.PeriodInstance
	var registry []db.P4ActivityRegistry
	if err := d.Order("id").Find(&pools).Error; err != nil {
		t.Fatal(err)
	}
	for _, r := range pools {
		var rewards []db.GameplayPoolReward
		if err := d.Where("pool_id=?", r.ID).Order("id").Find(&rewards).Error; err != nil {
			t.Fatal(err)
		}
		if rewards == nil {
			rewards = []db.GameplayPoolReward{}
		}
		out = append(out, adminActivityProjection("pool", r, rewards))
	}
	if err := d.Order("id").Find(&commerce).Error; err != nil {
		t.Fatal(err)
	}
	for _, r := range commerce {
		out = append(out, adminActivityProjection("commerce", r, nil))
	}
	if err := d.Order("id").Find(&welfare).Error; err != nil {
		t.Fatal(err)
	}
	for _, r := range welfare {
		out = append(out, adminActivityProjection("welfare", r, nil))
	}
	if err := d.Order("id").Find(&periods).Error; err != nil {
		t.Fatal(err)
	}
	for _, r := range periods {
		out = append(out, adminActivityProjection("period", r, nil))
	}
	if err := d.Order("id").Find(&registry).Error; err != nil {
		t.Fatal(err)
	}
	for _, r := range registry {
		out = append(out, adminActivityProjection("registry", r, nil))
	}
	return out
}

func adminActivityRewardsOf(t *testing.T, dto model.AdminActivityDTO) []db.GameplayPoolReward {
	t.Helper()
	var cfg struct {
		Rewards []db.GameplayPoolReward `json:"rewards"`
	}
	if err := json.Unmarshal(dto.Config, &cfg); err != nil {
		t.Fatal(err)
	}
	return cfg.Rewards
}

// TestAdminActivitiesBatchRewardResponseEquivalence 对照旧逐池算法检查多池、空奖励、交错奖励ID和孤立奖励。
func TestAdminActivitiesBatchRewardResponseEquivalence(t *testing.T) {
	ctx := adminActivitiesSQLite(t)
	d := db.GetIns().WithContext(ctx)
	pools := []db.GameplayPool{
		{ID: "pool-alpha", Name: "阿尔法", Source: "finite sqlite fixture", MinLevel: 1, FreeSeconds: 1, StartAt: 1, EndAt: 2, RewardEndAt: 2},
		{ID: "pool-bravo", Name: "布拉沃", Source: "finite sqlite fixture", MinLevel: 1, FreeSeconds: 1, StartAt: 1, EndAt: 2, RewardEndAt: 2},
		{ID: "pool-empty", Name: "空奖池", Source: "finite sqlite fixture", MinLevel: 1, FreeSeconds: 1, StartAt: 1, EndAt: 2, RewardEndAt: 2},
		{ID: "pool-delta", Name: "德尔塔", Source: "finite sqlite fixture", MinLevel: 1, FreeSeconds: 1, StartAt: 1, EndAt: 2, RewardEndAt: 2},
	}
	if err := d.Create(&pools).Error; err != nil {
		t.Fatal(err)
	}
	// 奖励ID刻意跨池交错，确认每池内部仍按原 id 升序返回。
	rewards := []db.GameplayPoolReward{
		{PoolID: "pool-alpha", ID: 2, ResourceID: "coin", Amount: 2, Weight: 1},
		{PoolID: "pool-delta", ID: 3, ResourceID: "coin", Amount: 3, Weight: 1},
		{PoolID: "pool-bravo", ID: 1, ResourceID: "coin", Amount: 1, Weight: 1},
		{PoolID: "pool-alpha", ID: 9, ResourceID: "diamond", Amount: 9, Weight: 2},
		{PoolID: "pool-delta", ID: 7, ResourceID: "coin", Amount: 7, Weight: 1},
		{PoolID: "pool-bravo", ID: 5, ResourceID: "diamond", Amount: 5, Weight: 1},
		{PoolID: "pool-orphan", ID: 6, ResourceID: "coin", Amount: 6, Weight: 1},
	}
	if err := d.Create(&rewards).Error; err != nil {
		t.Fatal(err)
	}
	commerce := db.P4CommerceRule{ID: "p4:ordinary:security", Kind: "ordinary", ShopType: 1, Enabled: true, Rules: adminJSON(map[string]int{"limit": 1}), Source: "finite sqlite fixture"}
	welfare := db.P4WelfareRule{ID: "p4:login:security", Kind: constUtil.P4WelfareLogin, Enabled: true, Rules: adminJSON(map[string]int{"days": 1}), Source: "finite sqlite fixture"}
	period := db.PeriodInstance{ID: "totalLogin_security", Kind: constUtil.P1EventLoginDay, Mode: "cumulative", Title: "等价检查", Goal: 1, Rewards: "coin,1", StartAt: 1, EndAt: 2, RewardEndAt: 2, Source: "finite sqlite fixture"}
	registry := db.P4ActivityRegistry{ID: 1, Name: "security_registry", Prefix: "p4", Protocols: "[]", Owner: "fixture", Status: "registered", Evidence: "finite sqlite fixture"}
	for _, row := range []any{&commerce, &welfare, &period, &registry} {
		if err := d.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}

	prevLogger := db.GetIns().Config.Logger
	recorder := &adminActivityQueryRecorder{Interface: gormlogger.Default.LogMode(gormlogger.Silent)}
	db.GetIns().Config.Logger = recorder
	defer func() { db.GetIns().Config.Logger = prevLogger }()

	actual, err := AdminActivities(ctx)
	if err != nil {
		t.Fatal(err)
	}
	batchRewardQueries := recorder.countContaining("gameplay_pool_rewards")
	recorder.reset()
	expected := legacyAdminActivities(t, ctx)
	legacyRewardQueries := recorder.countContaining("gameplay_pool_rewards")
	if batchRewardQueries != 1 || legacyRewardQueries != len(pools) {
		t.Fatalf("reward queries: batch %d (want 1), legacy %d (want %d pools)", batchRewardQueries, legacyRewardQueries, len(pools))
	}
	t.Logf("reward queries: legacy=%d batch=%d for %d pools", legacyRewardQueries, batchRewardQueries, len(pools))
	actualJSON, err := json.Marshal(actual)
	if err != nil {
		t.Fatal(err)
	}
	expectedJSON, err := json.Marshal(expected)
	if err != nil {
		t.Fatal(err)
	}
	if string(actualJSON) != string(expectedJSON) {
		t.Fatalf("batch rewards changed response\nactual: %s\nexpected: %s", actualJSON, expectedJSON)
	}
	again, err := AdminActivities(ctx)
	if err != nil {
		t.Fatal(err)
	}
	againJSON, err := json.Marshal(again)
	if err != nil {
		t.Fatal(err)
	}
	if string(againJSON) != string(actualJSON) {
		t.Fatalf("activity response or versions unstable across reads")
	}

	byID := map[string]model.AdminActivityDTO{}
	for _, row := range actual {
		byID[row.ID] = row
	}
	for id, wantIDs := range map[string][]int32{
		"pool:pool-alpha": {2, 9},
		"pool:pool-bravo": {1, 5},
		"pool:pool-empty": {},
		"pool:pool-delta": {3, 7},
	} {
		dto, ok := byID[id]
		if !ok {
			t.Fatalf("missing activity %s", id)
		}
		got := adminActivityRewardsOf(t, dto)
		if len(got) != len(wantIDs) {
			t.Fatalf("%s rewards = %+v, want ids %v", id, got, wantIDs)
		}
		for i, wantID := range wantIDs {
			if got[i].ID != wantID {
				t.Fatalf("%s reward order = %+v, want ids %v", id, got, wantIDs)
			}
		}
	}
	if _, ok := byID["pool:pool-orphan"]; ok {
		t.Fatal("orphan reward pool leaked into activities")
	}
	// 空奖励保持 []，与旧算法一致，不回退为 null。
	var emptyCfg map[string]json.RawMessage
	if err := json.Unmarshal(byID["pool:pool-empty"].Config, &emptyCfg); err != nil {
		t.Fatal(err)
	}
	if string(emptyCfg["rewards"]) != "[]" {
		t.Fatalf("empty pool rewards = %s, want []", emptyCfg["rewards"])
	}
	if len(actual) != 8 || byID["commerce:"+commerce.ID].ID == "" || byID["welfare:"+welfare.ID].ID == "" || byID["period:"+period.ID].ID == "" || byID["registry:"+registry.Name].ID == "" {
		t.Fatalf("activity ordering or other domains changed: %+v", actual)
	}
}

// TestAdminActivitiesRewardQueryCountStaysBounded 检查奖励读取按块增长，并在块边界后继续合并结果。
func TestAdminActivitiesRewardQueryCountStaysBounded(t *testing.T) {
	ctx := adminActivitiesSQLite(t)
	d := db.GetIns().WithContext(ctx)
	chunk := constUtil.AdminActivityRewardQueryChunkSize
	poolCount := chunk + 1
	pools := make([]db.GameplayPool, 0, poolCount)
	for i := 0; i < poolCount; i++ {
		pools = append(pools, db.GameplayPool{
			ID:          fmt.Sprintf("perf-pool-%04d", i),
			Name:        "性能池",
			Source:      "finite sqlite fixture",
			MinLevel:    1,
			FreeSeconds: 1,
			StartAt:     1,
			EndAt:       2,
			RewardEndAt: 2,
		})
	}
	if err := d.CreateInBatches(&pools, 200).Error; err != nil {
		t.Fatal(err)
	}
	// 只给首块末位和次块首位的池写入奖励，确认跨块归组没有丢失或串池。
	boundary := []db.GameplayPoolReward{
		{PoolID: pools[chunk-1].ID, ID: 2, ResourceID: "coin", Amount: 2, Weight: 1},
		{PoolID: pools[chunk].ID, ID: 1, ResourceID: "diamond", Amount: 1, Weight: 1},
	}
	if err := d.Create(&boundary).Error; err != nil {
		t.Fatal(err)
	}

	prevLogger := db.GetIns().Config.Logger
	recorder := &adminActivityQueryRecorder{Interface: gormlogger.Default.LogMode(gormlogger.Silent)}
	db.GetIns().Config.Logger = recorder
	defer func() { db.GetIns().Config.Logger = prevLogger }()

	started := time.Now()
	rows, err := AdminActivities(ctx)
	if err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(started)
	if len(rows) != poolCount {
		t.Fatalf("activity count = %d, want %d", len(rows), poolCount)
	}
	// 池增长只增加奖励查询块数，池主表仍为单次读取。
	rewardQueries := recorder.countContaining("gameplay_pool_rewards")
	poolQueries := recorder.countContaining("gameplay_pools")
	t.Logf("pools=%d rewardQueries=%d poolQueries=%d elapsed=%s", poolCount, rewardQueries, poolQueries, elapsed)
	if rewardQueries != 2 {
		t.Fatalf("reward queries = %d, want 2 for %d pools (chunk %d)", rewardQueries, poolCount, chunk)
	}
	if poolQueries != 1 {
		t.Fatalf("pool queries = %d, want 1", poolQueries)
	}
	first := adminActivityRewardsOf(t, rows[chunk-1])
	second := adminActivityRewardsOf(t, rows[chunk])
	if len(first) != 1 || first[0].ID != 2 || len(second) != 1 || second[0].ID != 1 {
		t.Fatalf("cross-chunk grouping = first %+v second %+v", first, second)
	}
	for i, row := range rows {
		if i == chunk-1 || i == chunk {
			continue
		}
		if got := adminActivityRewardsOf(t, row); len(got) != 0 {
			t.Fatalf("pool %s received unrelated rewards: %+v", row.ID, got)
		}
	}
}
