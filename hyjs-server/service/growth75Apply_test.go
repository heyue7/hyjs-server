package service

import (
	"fmt"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	"testing"
)

func growth75DeploymentFixture(t *testing.T, slots int) (*p1Fixture, *model.AdminPrincipal) {
	t.Helper()
	f, a := growth75Fixture(t)
	if err := f.s.InitializeClientUpdates(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().AutoMigrate(&db.ServerGameClock{}, &db.AdminActivityOccurrence{}, &db.AdminActivityDefinition{}, &db.AdminCharacterActivityInstance{}); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Create(&db.ServerGameClock{ServerID: 1, Revision: 1}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.AdminUser{}).Where("id=?", a.UserID).Update("role", constUtil.AdminRoleAdmin).Error; err != nil {
		t.Fatal(err)
	}
	a.Role = constUtil.AdminRoleAdmin
	proof := db.AdminAudit{ID: constUtil.Growth75SourceAuditID, Time: constUtil.Growth75SourcePublishedAt, Action: "activity.schedule.save", Target: "1", Detail: constUtil.Growth75SourceDetail}
	if err := db.GetIns().Create(&proof).Error; err != nil {
		t.Fatal(err)
	}
	rows := make([]db.AdminActivityOccurrence, 0, slots)
	for i := 0; i < slots; i++ {
		rows = append(rows, db.AdminActivityOccurrence{ID: fmt.Sprintf("growth75-proof-%04d", i), ServerID: 1, ConsumerKey: fmt.Sprintf("growth75-proof-consumer-%04d", i), ConsumerID: fmt.Sprintf("period:growth75-proof-consumer-%04d", i), Mode: "login-total", FormJSON: adminJSON(model.AdminActivityForm{Source: "成长投放V12 / 2026-10-26", Mode: "login-total"})})
	}
	if err := db.GetIns().CreateInBatches(rows, 100).Error; err != nil {
		t.Fatal(err)
	}
	return f, a
}

func TestGrowth75DeploymentRejectsLiveOrIncompleteEvidence(t *testing.T) {
	f, _ := growth75DeploymentFixture(t, 819)
	if err := f.s.ApplyGrowth75(f.ctx); err == nil {
		t.Fatal("live game accepted maintenance apply")
	}
	if err := db.GetIns().Model(&db.ClientUpdatePolicy{}).Where("id=1").Update("maintenance", true).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.ApplyGrowth75(f.ctx); err == nil {
		t.Fatal("819 rows accepted audited820 boundary")
	}
	if n := p1Count(t, &db.AdminAudit{}, "action=?", constUtil.Growth75ApplyAction); n != 0 {
		t.Fatal("incomplete publication marked complete")
	}
	var any db.AdminActivityOccurrence
	if err := db.GetIns().First(&any).Error; err != nil || any.FirstPublishedAt != 0 {
		t.Fatal("publication mutated before full evidence validation", err)
	}
}

func TestGrowth75DeploymentAppliesInMaintenanceAndResumesWithoutDuplicateRelease(t *testing.T) {
	f, _ := growth75DeploymentFixture(t, 820)
	if err := db.GetIns().Model(&db.ClientUpdatePolicy{}).Where("id=1").Updates(map[string]any{"maintenance": true, "maintenance_start_at": 123, "maintenance_epoch": 9}).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := f.s.ApplyGrowth75(f.ctx); err != nil {
			t.Fatal("explicit candidate command", i, err)
		}
	}
	var p db.ClientUpdatePolicy
	if err := db.GetIns().First(&p, 1).Error; err != nil {
		t.Fatal(err)
	}
	if !p.Maintenance || p.MaintenanceEpoch != 9 || p.MaintenanceStartAt != 123 || p.VersionName != constUtil.Growth75Version || p.LatestBuild != constUtil.Growth75Build || p.MinLoginBuild != constUtil.Growth75Build || p.MinRunBuild != constUtil.Growth75Build {
		t.Fatalf("wrong release or maintenance changed %+v", p)
	}
	if n := p1Count(t, &db.ClientUpdateRelease{}, "build_code=?", constUtil.Growth75Build); n != 1 {
		t.Fatal("retry published duplicate releases", n)
	}
	if n := p1Count(t, &db.AdminActivityOccurrence{}, "first_published_at=?", constUtil.Growth75SourcePublishedAt); n != 820 {
		t.Fatal("audited publication set incomplete", n)
	}
	if n := p1Count(t, &db.AdminAudit{}, "action=?", "calendar75.anchors"); n != 1 {
		t.Fatal("calendar repair replayed", n)
	}
}
