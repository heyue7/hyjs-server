package service

import (
	"testing"
	"time"

	"hyjs-server/db"
	"hyjs-server/model"
)

// TestAdminActivityIdentityPreservesHistoricalSnapshot checks the public read/write boundary,
// including fixed names, separately stored operator aliases and immutable issued history.
func TestAdminActivityIdentityPreservesHistoricalSnapshot(t *testing.T) {
	f, actor := adminModesFixture(t)
	if err := f.s.InitializeAdminGameServers(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeAdminActivitySchedules(f.ctx); err != nil {
		t.Fatal(err)
	}
	form := adminModeForm("login-total")
	form.Alias = "十月登录"
	form.Milestones = []model.AdminActivityMilestone{{Goal: 1, Rewards: []model.AdminResourceAmount{{ResourceID: "coin", Amount: 17}}}}
	created, err := f.s.AdminSaveActivityDefinition(f.ctx, actor, "", model.AdminActivityDefinitionSaveRequest{Name: "任意名称不能覆盖原标题", Form: form, DefaultDays: 2, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if created.Name != "登录奖励" || created.Form.Name != created.Name || created.Form.Alias != "十月登录" {
		t.Fatal("fixed title and management alias mixed", created)
	}
	calendar, err := f.s.AdminActivitySchedule(f.ctx, actor, 1)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	result, err := f.s.AdminSaveActivitySchedule(f.ctx, actor, 1, model.AdminActivityScheduleSaveRequest{Version: calendar.Version, Items: []model.AdminActivityOccurrenceDTO{{TemplateID: created.ID, TemplateVersion: created.Version, StartAt: now - 1000, EndAt: now + 86400000, ClaimEndAt: now + 86400000, Enabled: true}}})
	if err != nil || len(result.Items) != 1 {
		t.Fatal("publish", err)
	}
	if result.Items[0].Name != "登录奖励" || result.Items[0].Form.Alias != "十月登录" {
		t.Fatal("calendar lost fixed name/alias", result.Items[0])
	}
	// A legacy issued record is shown canonically without rewriting its original bytes or version.
	var row db.AdminActivityOccurrence
	if err = db.GetIns().First(&row, "id=?", result.Items[0].ID).Error; err != nil {
		t.Fatal(err)
	}
	legacy := form
	legacy.Alias = ""
	legacy.Name = "旧活动自定义名"
	row.Name = legacy.Name
	row.FormJSON = adminJSON(legacy)
	if err = db.GetIns().Save(&row).Error; err != nil {
		t.Fatal(err)
	}
	before := string(row.FormJSON)
	projected, err := f.s.adminActivityOccurrenceDTO(db.GetIns().WithContext(f.ctx), row)
	if err != nil || projected.Name != "登录奖励" || projected.Form.Alias != legacy.Name {
		t.Fatal("legacy projection", projected, err)
	}
	var after db.AdminActivityOccurrence
	if err = db.GetIns().First(&after, "id=?", row.ID).Error; err != nil || string(after.FormJSON) != before || adminVersion(after) != adminVersion(row) {
		t.Fatal("read changed issued history", err)
	}
}

func TestAdminKingIdentityIgnoresWorldSuffix(t *testing.T) {
	f, actor, form := adminKingFixture(t)
	form.Name = "王者招募·SAO"
	form.Alias = "SAO常驻"
	created := adminKingDefinition(t, f, actor, form)
	if created.Name != "王者招募" || created.Form.Name != "王者招募" || created.Form.Alias != "SAO常驻" {
		t.Fatal("king suffix leaked into fixed title", created)
	}
	var saved db.AdminActivityDefinition
	if err := db.GetIns().First(&saved, "id=?", created.ID).Error; err != nil {
		t.Fatal(err)
	}
	if saved.Name != "王者招募" {
		t.Fatal("custom king name persisted", saved.Name)
	}
}
