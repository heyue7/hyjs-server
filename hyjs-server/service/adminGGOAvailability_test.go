package service

import (
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	"testing"
)

func TestAdminGGOAvailabilityMatchesRuntimeGate(t *testing.T) {
	t.Setenv("HYJS_P1_MYSQL_DATABASE", "")
	f := p1Setup(t)
	if err := f.s.LoadP8Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.LoadP12WorldBossTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeAdminFields(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().AutoMigrate(&db.AdminUser{}, &db.AdminAudit{}); err != nil {
		t.Fatal(err)
	}
	user := db.AdminUser{Username: "isolated-desert-availability", Role: constUtil.AdminRoleOperator, Enabled: true, PasswordHash: "isolated-no-login"}
	if err := db.GetIns().Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	actor := &model.AdminPrincipal{UserID: user.ID, Username: user.Username, Role: user.Role}
	for _, integration := range []bool{false, true} {
		f.s.GGOIntegrationReady = integration
		list, err := f.s.AdminFields(f.ctx, actor)
		if err != nil {
			t.Fatal(err)
		}
		deployments, err := f.s.AdminFieldDeployments(f.ctx, actor, 1)
		if err != nil {
			t.Fatal(err)
		}
		seen := false
		for _, item := range list.Items {
			want := constUtil.P8PackagedFieldScene(item.Configuration.Scene) || item.Configuration.Scene == constUtil.GGOFieldScene && f.s.ggoExtractionEnabled()
			if item.Available != want || (item.UnavailableReason == "") != want {
				t.Fatalf("template gate mismatch: scene=%s integration=%t", item.Configuration.Scene, integration)
			}
			if item.Configuration.Scene == constUtil.GGOFieldScene {
				seen = true
			}
		}
		if !seen {
			t.Fatal("desert template absent")
		}
		for _, item := range deployments.Items {
			want := constUtil.P8PackagedFieldScene(item.Scene) || item.Scene == constUtil.GGOFieldScene && f.s.ggoExtractionEnabled()
			if item.Available != want {
				t.Fatalf("deployment gate mismatch: scene=%s integration=%t", item.Scene, integration)
			}
			if item.Enabled != constUtil.P8PackagedFieldScene(item.Scene) {
				t.Fatal("availability silently enabled unpublished map")
			}
		}
	}
}
