package service

import (
	"errors"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	"testing"
	"time"
)

func TestFieldRefresh69ScheduleAndColdNodes(t *testing.T) {
	f := p1Setup(t)
	ctx := f.ctx
	s := f.s
	for _, load := range []func(string) error{s.LoadTables, s.LoadP8Tables} {
		if err := load("../../analysis/tables"); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.InitializeP8Gather(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.InitializeFieldResources(ctx); err != nil {
		t.Fatal(err)
	}
	at := func(value string) int64 {
		v, e := time.Parse(time.RFC3339, value)
		if e != nil {
			t.Fatal(e)
		}
		return v.UnixMilli()
	}
	before := at("2026-10-25T11:59:59+08:00")
	noon := at("2026-10-25T12:00:00+08:00")
	midnight := at("2026-10-26T00:00:00+08:00")
	if fieldGatherBoundary(before, []int64{0, 12}) != at("2026-10-25T00:00:00+08:00") || fieldGatherBoundary(noon, []int64{0, 12}) != noon || fieldGatherBoundary(midnight-1, []int64{0, 12}) != noon || fieldGatherBoundary(midnight, []int64{0, 12}) != midnight {
		t.Fatal("Beijing boundary drift")
	}
	for _, bad := range [][]int64{nil, {}, {0, 0}, {-1}, {24}} {
		if validFieldRefreshHours(bad) {
			t.Fatal("accepted invalid schedule", bad)
		}
	}
	nodes, _, err := s.p8Nodes(db.GetIns(), f.id, "field_001", before)
	if err != nil || len(nodes) < 2 {
		t.Fatal(err)
	}
	target := nodes[0]
	attempt := db.P8GatherAttempt{PlayerID: f.id, Token: "refresh69", Scene: target.Scene, PosID: target.PosID, Generation: target.Generation, Active: true, Snapshot: target.Snapshot}
	if err := db.GetIns().Create(&attempt).Error; err != nil {
		t.Fatal(err)
	}
	refreshed, changed, err := s.p8Nodes(db.GetIns(), f.id, "field_001", noon)
	if err != nil || !changed {
		t.Fatal(err)
	}
	for _, n := range refreshed {
		want := int64(2)
		if n.PosID == target.PosID {
			want = 1
		}
		if n.Generation != want {
			t.Fatal("frozen node protection", n.PosID, n.Generation, want)
		}
	}
	if err := db.GetIns().Model(&attempt).Update("active", false).Error; err != nil {
		t.Fatal(err)
	}
	// A new service instance reads persisted boundaries and catches the deferred point.
	cold := &GameService{Tables: s.Tables, FieldResourcePolicyReady: true}
	refreshed, changed, err = cold.p8Nodes(db.GetIns(), f.id, "field_001", noon)
	if err != nil || !changed {
		t.Fatal(err)
	}
	for _, n := range refreshed {
		if n.Generation != 2 || n.ScheduledAt != noon {
			t.Fatal("deferred or persisted refresh", n)
		}
	}
	if _, changed, err = cold.p8Nodes(db.GetIns(), f.id, "field_001", noon+1); err != nil || changed {
		t.Fatal("repeat refreshed", err)
	}
	refreshed, changed, err = cold.p8Nodes(db.GetIns(), f.id, "field_001", midnight)
	if err != nil || !changed {
		t.Fatal(err)
	}
	for _, n := range refreshed {
		if n.Generation != 3 || n.ScheduledAt != midnight {
			t.Fatal("midnight refresh", n)
		}
	}
	// Startup only fills missing defaults and preserves operator schedules.
	if err := db.GetIns().Model(&db.FieldResourcePolicy{}).Where("id=1").Update("gather_refresh_hours", "[6,18]").Error; err != nil {
		t.Fatal(err)
	}
	if err := s.InitializeFieldResources(ctx); err != nil {
		t.Fatal(err)
	}
	var policy db.FieldResourcePolicy
	if err := db.GetIns().First(&policy).Error; err != nil || policy.GatherRefreshHours != "[6,18]" {
		t.Fatal("operator schedule overwritten", err, policy)
	}
}

func TestFieldRefresh69ConfigLegacyAndConflict(t *testing.T) {
	f := p1Setup(t)
	if err := f.s.InitializeFieldResources(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := AdminInitialize(f.ctx, ""); err != nil {
		t.Fatal(err)
	}
	user := db.AdminUser{Username: "field69", PasswordHash: "isolated-fixture", Role: constUtil.AdminRoleOperator, Enabled: true}
	if err := db.GetIns().Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	actor := &model.AdminPrincipal{UserID: user.ID, Username: user.Username, Role: user.Role}
	current, err := f.s.AdminFieldResources(f.ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	req := model.AdminFieldResourcesSaveRequest{AdminFieldResourcesResponse: *current, Reason: "刷新时刻验证"}
	req.GatherRefreshHours = []int64{18, 6}
	req.BossDailyRewards = current.BossDailyRewards + 1
	saved, err := f.s.AdminSaveFieldResources(f.ctx, actor, req)
	if err != nil || saved.GatherRefreshHours[0] != 6 {
		t.Fatal(err)
	}
	if _, err = f.s.AdminSaveFieldResources(f.ctx, actor, req); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatal("stale schedule accepted", err)
	}
	req.AdminFieldResourcesResponse = *saved
	req.GatherRefreshHours = nil
	req.BossDailyRewards = 1
	saved, err = f.s.AdminSaveFieldResources(f.ctx, actor, req)
	if err != nil || len(saved.GatherRefreshHours) != 2 || saved.GatherRefreshHours[0] != 6 || saved.GatherRefreshHours[1] != 18 {
		t.Fatal("legacy PUT changed schedule", err)
	}
	req.AdminFieldResourcesResponse = *saved
	req.GatherRefreshHours = []int64{}
	if _, err = f.s.AdminSaveFieldResources(f.ctx, actor, req); !errors.Is(err, constUtil.ErrAdminInput) {
		t.Fatal("empty schedule accepted", err)
	}
}
