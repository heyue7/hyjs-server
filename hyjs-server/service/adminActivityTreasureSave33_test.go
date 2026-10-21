package service

import (
	"testing"
	"time"

	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

// TestAdminTreasureTemplateSave33 covers the reported enabled-template save,
// its cold read and the actual consumer's no-grace rule without issuing a live activity.
func TestAdminTreasureTemplateSave33(t *testing.T) {
	f, actor := adminModesFixture(t)
	if err := f.s.InitializeAdminActivitySchedules(f.ctx); err != nil {
		t.Fatal(err)
	}
	form := reOperationsWelfareForms()[0]
	form.Milestones[0].Rewards[0].Amount = 10000
	before := p1Count(t, &db.P4WelfareRule{}, "kind=?", constUtil.ReWelfareCatKind)
	saved, err := f.s.AdminSaveActivityDefinition(f.ctx, actor, "", model.AdminActivityDefinitionSaveRequest{Name: form.Name, Form: form, DefaultDays: 7, Enabled: true})
	if err != nil || !saved.Availability.Usable {
		t.Fatalf("valid template rejected: %+v %v", saved, err)
	}
	f.restart(t)
	rows, err := f.s.AdminActivityDefinitions(f.ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range rows {
		if row.ID == saved.ID {
			found = row.Version == saved.Version && row.Form.Milestones[0].Goal == 100 && row.Form.Milestones[0].Rewards[0].Amount == 10000 && row.Form.Milestones[0].Rewards[0].ResourceID == "coin"
		}
	}
	if !found || p1Count(t, &db.P4WelfareRule{}, "kind=?", constUtil.ReWelfareCatKind) != before {
		t.Fatal("template did not persist independently of live consumer rules")
	}
	// Template validation must not relax the real activity's closing time.
	form.StartAt = time.Now().Add(-time.Hour).UnixMilli()
	form.EndAt = time.Now().Add(time.Hour).UnixMilli()
	form.ClaimEndAt = form.EndAt + 86400000
	if _, err = f.s.AdminSaveTypedActivity(f.ctx, actor, "", model.AdminTypedActivityRequest{AdminActivityForm: form, Enabled: true}); err == nil {
		t.Fatal("actual treasure activity accepted a claim grace window")
	}
}
