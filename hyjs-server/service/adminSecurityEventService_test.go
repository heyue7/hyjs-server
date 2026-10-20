package service

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

// TestAdminSecurityEventsAggregateRBACAndRetention 覆盖分钟聚合、字段清洗、admin复核和30天保留。
func TestAdminSecurityEventsAggregateRBACAndRetention(t *testing.T) {
	ctx := context.Background()
	if err := db.InitGame(ctx, filepath.Join(t.TempDir(), "security-events.sqlite")); err != nil {
		t.Fatal(err)
	}
	connection, _ := db.GetIns().DB()
	t.Cleanup(func() { connection.Close() })
	if err := db.GetIns().WithContext(ctx).AutoMigrate(&db.AdminUser{}); err != nil {
		t.Fatal(err)
	}
	if err := InitializeSecurityEvents(ctx); err != nil {
		t.Fatal(err)
	}
	users := []db.AdminUser{
		{Username: "security-admin", PasswordHash: "unused-fixture", Role: constUtil.AdminRoleAdmin, Enabled: true},
		{Username: "security-operator", PasswordHash: "unused-fixture", Role: constUtil.AdminRoleOperator, Enabled: true},
		{Username: "security-viewer", PasswordHash: "unused-fixture", Role: constUtil.AdminRoleViewer, Enabled: true},
		{Username: "security-disabled", PasswordHash: "unused-fixture", Role: constUtil.AdminRoleAdmin, Enabled: false},
	}
	for i := range users {
		if err := db.GetIns().WithContext(ctx).Create(&users[i]).Error; err != nil {
			t.Fatal(err)
		}
	}
	admin := &model.AdminPrincipal{UserID: users[0].ID, Username: users[0].Username, Role: users[0].Role}
	operator := &model.AdminPrincipal{UserID: users[1].ID, Username: users[1].Username, Role: users[1].Role}
	viewer := &model.AdminPrincipal{UserID: users[2].ID, Username: users[2].Username, Role: users[2].Role}
	disabled := &model.AdminPrincipal{UserID: users[3].ID, Username: users[3].Username, Role: users[3].Role}

	base := int64(1_767_225_600_000)
	login := model.SecurityEventInput{Actor: "alice", SourceIP: "203.0.113.7", Action: "auth.login", Target: "acct-1", Outcome: constUtil.AdminSecurityEventOutcomeSuccess, ReasonCode: "ok", RequestID: "req-r1"}
	for i := 0; i < 2; i++ {
		if err := adminSecurityEventRecordAt(ctx, login, base+int64(i)*1000); err != nil {
			t.Fatal(err)
		}
	}
	denied := model.SecurityEventInput{Actor: "bob", SourceIP: "198.51.100.9", Action: "auth.password.reset", Target: "acct-2", Outcome: constUtil.AdminSecurityEventOutcomeDenied, ReasonCode: "permission", RequestID: "req-r2"}
	if err := adminSecurityEventRecordAt(ctx, denied, base+2000); err != nil {
		t.Fatal(err)
	}
	cleaned := model.SecurityEventInput{Actor: "eve\nadmin", SourceIP: "2001:db8::1", Action: "auth.logout", Target: "acct\x003", Outcome: constUtil.AdminSecurityEventOutcomeFailure, ReasonCode: "expired"}
	if err := adminSecurityEventRecordAt(ctx, cleaned, base+3000); err != nil {
		t.Fatal(err)
	}

	var aggregated db.AdminSecurityEvent
	if err := db.GetIns().WithContext(ctx).Where("actor = ?", "alice").Take(&aggregated).Error; err != nil {
		t.Fatal(err)
	}
	if aggregated.Occurrences != 2 || aggregated.FirstAt != base || aggregated.LastAt != base+1000 {
		t.Fatalf("minute aggregation = %+v", aggregated)
	}
	if aggregated.SourceIP != "203.0.113.7" || aggregated.EventKey == "" || aggregated.RequestID != "req-r1" {
		t.Fatalf("stored security event = %+v", aggregated)
	}
	var sanitized db.AdminSecurityEvent
	if err := db.GetIns().WithContext(ctx).Where("actor = ?", "eveadmin").Take(&sanitized).Error; err != nil {
		t.Fatal(err)
	}
	if sanitized.Target != "acct3" || sanitized.SourceIP != "2001:db8::1" {
		t.Fatalf("sanitized security event = %+v", sanitized)
	}

	badIP := login
	badIP.SourceIP = "X-Forwarded-For: 203.0.113.7"
	if err := adminSecurityEventRecordAt(ctx, badIP, base+4000); !errors.Is(err, constUtil.ErrAdminSecurityEventInput) {
		t.Fatalf("invalid source IP error = %v", err)
	}
	badReason := login
	badReason.ReasonCode = "password=secret"
	if err := adminSecurityEventRecordAt(ctx, badReason, base+4000); !errors.Is(err, constUtil.ErrAdminSecurityEventInput) {
		t.Fatalf("invalid reason code error = %v", err)
	}
	var count int64
	if err := db.GetIns().WithContext(ctx).Model(&db.AdminSecurityEvent{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("invalid inputs created rows: %d", count)
	}

	page, err := AdminSecurityEvents(ctx, admin, model.AdminSecurityEventsRequest{Page: 1, PageSize: 25})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 3 || len(page.Items) != 3 {
		t.Fatalf("admin security page = %+v", page)
	}
	if page.Items[0].Action != "auth.logout" || page.Items[1].Outcome != constUtil.AdminSecurityEventOutcomeDenied {
		t.Fatalf("last_at ordering = %+v", page.Items)
	}
	raw, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "eventKey") || strings.Contains(string(raw), aggregated.EventKey) {
		t.Fatalf("internal event key leaked: %s", raw)
	}
	filtered, err := AdminSecurityEvents(ctx, admin, model.AdminSecurityEventsRequest{Page: 1, PageSize: 25, Action: "auth.login", Outcome: constUtil.AdminSecurityEventOutcomeSuccess, Actor: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	if filtered.Total != 1 || len(filtered.Items) != 1 || filtered.Items[0].Occurrences != 2 {
		t.Fatalf("filtered security page = %+v", filtered)
	}

	for name, principal := range map[string]*model.AdminPrincipal{"operator": operator, "viewer": viewer, "disabled": disabled} {
		if _, err := AdminSecurityEvents(ctx, principal, model.AdminSecurityEventsRequest{Page: 1, PageSize: 25}); !errors.Is(err, constUtil.ErrAdminPermission) {
			t.Fatalf("%s security read error = %v", name, err)
		}
	}
	staleAdmin := &model.AdminPrincipal{UserID: operator.UserID, Username: operator.Username, Role: constUtil.AdminRoleAdmin}
	if _, err := AdminSecurityEvents(ctx, staleAdmin, model.AdminSecurityEventsRequest{Page: 1, PageSize: 25}); !errors.Is(err, constUtil.ErrAdminPermission) {
		t.Fatalf("stale admin role read error = %v", err)
	}

	old := db.AdminSecurityEvent{EventKey: strings.Repeat("a", 64), FirstAt: 1, LastAt: base - constUtil.AdminSecurityEventRetentionMillis - 1, Actor: "old", SourceIP: "203.0.113.9", Action: "auth.login", Target: "acct-old", Outcome: constUtil.AdminSecurityEventOutcomeFailure, ReasonCode: "old", Occurrences: 1}
	if err := db.GetIns().WithContext(ctx).Create(&old).Error; err != nil {
		t.Fatal(err)
	}
	if err := adminSecurityEventCleanup(ctx, base); err != nil {
		t.Fatal(err)
	}
	var remaining int64
	if err := db.GetIns().WithContext(ctx).Model(&db.AdminSecurityEvent{}).Where("id = ?", old.ID).Count(&remaining).Error; err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Fatalf("retention cleanup kept old row: %d", remaining)
	}
}
