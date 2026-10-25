package service

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

func TestAdminDailyTask69AtomicRewardsAndFrozenDay(t *testing.T) {
	ctx := context.Background()
	if err := db.InitGame(ctx, filepath.Join(t.TempDir(), "admin-daily69.db")); err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.GetIns().DB()
	defer sqlDB.Close()
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.AdminUser{}, &db.AdminAudit{}); err != nil {
		t.Fatal(err)
	}
	s := &GameService{}
	if err := s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := s.InitializeDailyTasks(ctx); err != nil {
		t.Fatal(err)
	}
	user := db.AdminUser{Username: "daily69-operator", Role: constUtil.AdminRoleOperator, Enabled: true, PasswordHash: "isolated"}
	if err := tx.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	actor := &model.AdminPrincipal{UserID: user.ID, Username: user.Username, Role: user.Role}
	initial, err := s.AdminDailyTasks69(ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	request := func() model.AdminDailyTask69SaveRequest {
		req := model.AdminDailyTask69SaveRequest{Version: initial.Version, Reason: "隔离验证"}
		for _, row := range initial.Items {
			req.Items = append(req.Items, model.AdminDailyTask69Reward{ID: row.ID, Reward: model.AdminResourceAmount{ResourceID: "diamond", Amount: 55}})
		}
		return req
	}
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	freeze := func(at time.Time) *db.DailyTaskState {
		t.Helper()
		var row *db.DailyTaskState
		err := tx.Transaction(func(q *gorm.DB) error {
			var e error
			var state *dailyTaskSnapshot
			row, state, e = s.loadDailyTaskState(q, 12345, 1, at)
			if e != nil {
				return e
			}
			return saveDailyTaskState(q, row, state)
		})
		if err != nil {
			t.Fatal(err)
		}
		return row
	}
	frozen := freeze(now)
	if _, err := s.AdminDailyTasks69(ctx, nil); !errors.Is(err, constUtil.ErrAdminPermission) {
		t.Fatal("unauthenticated", err)
	}
	if err := tx.Model(&db.AdminUser{}).Where("id=?", user.ID).Update("role", constUtil.AdminRoleViewer).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := s.AdminDailyTasks69(ctx, actor); err != nil {
		t.Fatal("viewer read", err)
	}
	if _, err := s.AdminSaveDailyTasks69(ctx, actor, request()); !errors.Is(err, constUtil.ErrAdminPermission) {
		t.Fatal("viewer write", err)
	}
	tx.Model(&db.AdminUser{}).Where("id=?", user.ID).Update("role", constUtil.AdminRoleOperator)
	cases := []struct {
		name   string
		mutate func(*model.AdminDailyTask69SaveRequest)
	}{
		{"missing", func(r *model.AdminDailyTask69SaveRequest) { r.Items = r.Items[:6] }},
		{"duplicate", func(r *model.AdminDailyTask69SaveRequest) { r.Items[6].ID = r.Items[0].ID }},
		{"unknown", func(r *model.AdminDailyTask69SaveRequest) { r.Items[6].ID = "unknown" }},
		{"zero", func(r *model.AdminDailyTask69SaveRequest) { r.Items[6].Reward.Amount = 0 }},
		{"overflow", func(r *model.AdminDailyTask69SaveRequest) { r.Items[6].Reward.Amount = 2147483648 }},
		{"unsupported", func(r *model.AdminDailyTask69SaveRequest) { r.Items[6].Reward.ResourceID = "unknown" }},
		{"multiple", func(r *model.AdminDailyTask69SaveRequest) { r.Items[6].Reward.ResourceID = "coin;diamond" }},
		{"reason", func(r *model.AdminDailyTask69SaveRequest) { r.Reason = " " }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := request()
			c.mutate(&req)
			if _, err := s.AdminSaveDailyTasks69(ctx, actor, req); !errors.Is(err, constUtil.ErrAdminInput) {
				t.Fatal(err)
			}
			read, err := s.AdminDailyTasks69(ctx, actor)
			if err != nil || read.Version != initial.Version {
				t.Fatal("partial save", read, err)
			}
		})
	}
	saved, err := s.AdminSaveDailyTasks69(ctx, actor, request())
	if err != nil {
		t.Fatal(err)
	}
	if saved.Version == initial.Version {
		t.Fatal("version unchanged")
	}
	for _, row := range saved.Items {
		if row.Reward.Amount != 55 || row.Reward.ResourceID != "diamond" {
			t.Fatal(row)
		}
	}
	if _, err := s.AdminSaveDailyTasks69(ctx, actor, request()); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatal("stale version", err)
	}
	same := freeze(now)
	if same.Snapshot != frozen.Snapshot {
		t.Fatal("existing day rewritten")
	}
	next := freeze(now.AddDate(0, 0, 1))
	if !strings.Contains(next.Snapshot, "diamond,55") {
		t.Fatal("next day ignored new reward")
	}
	var audit int64
	tx.Model(&db.AdminAudit{}).Where("action=?", constUtil.AdminDailyTask69AuditAction).Count(&audit)
	if audit != 1 {
		t.Fatal("audit count", audit)
	}
}
