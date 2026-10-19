package service

import (
	"encoding/json"
	"errors"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"testing"
)

func TestAdminActivityDefinitionDeletePreservesIssuedHistory(t *testing.T) {
	f, actor := adminModesFixture(t)
	if err := f.s.InitializeAdminActivitySchedules(f.ctx); err != nil {
		t.Fatal(err)
	}
	var row db.AdminActivityDefinition
	if err := db.GetIns().First(&row, "id=?", "original_opening_14").Error; err != nil {
		t.Fatal(err)
	}
	issued := db.AdminActivityOccurrence{ID: "delete-proof", ServerID: 1, TemplateID: row.ID, TemplateVersion: adminVersion(row), Mode: row.Mode, Name: row.Name, ConsumerID: "opening:delete-proof", ConsumerKey: "delete-proof", FormJSON: row.FormJSON, Revision: 1, Enabled: true}
	if err := db.GetIns().Create(&issued).Error; err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(issued)
	if e := f.s.AdminDeleteActivityDefinition(f.ctx, actor, row.ID, "wrong"); !errors.Is(e, constUtil.ErrAdminConfigConflict) {
		t.Fatal("stale delete", e)
	}
	readOnly := *actor
	readOnly.Role = constUtil.AdminRoleViewer
	// Resolve permissions from the persisted identity, rather than trusting the request role field.
	var user db.AdminUser
	if e := db.GetIns().First(&user, actor.UserID).Error; e != nil {
		t.Fatal(e)
	}
	originalRole := user.Role
	user.Role = constUtil.AdminRoleViewer
	db.GetIns().Save(&user)
	if e := f.s.AdminDeleteActivityDefinition(f.ctx, &readOnly, row.ID, adminVersion(row)); !errors.Is(e, constUtil.ErrAdminPermission) {
		t.Fatal("viewer deleted template", e)
	}
	user.Role = originalRole
	db.GetIns().Save(&user)
	if e := f.s.AdminDeleteActivityDefinition(f.ctx, actor, row.ID, adminVersion(row)); e != nil {
		t.Fatal(e)
	}
	if e := f.s.InitializeAdminActivitySchedules(f.ctx); e != nil {
		t.Fatal(e)
	}
	var archived db.AdminActivityDefinition
	if e := db.GetIns().First(&archived, "id=?", row.ID).Error; e != nil || !archived.Archived || archived.Enabled {
		t.Fatal("template resurrected", e)
	}
	var after db.AdminActivityOccurrence
	if e := db.GetIns().First(&after, "id=?", issued.ID).Error; e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(after)
	if string(raw) != string(before) {
		t.Fatal("delete rewrote issued contract")
	}
	listed, e := f.s.AdminActivityDefinitions(f.ctx, actor)
	if e != nil {
		t.Fatal(e)
	}
	for _, x := range listed {
		if x.ID == row.ID {
			t.Fatal("archived still in template library")
		}
	}
	var audit db.AdminAudit
	if e = db.GetIns().First(&audit, "action=? AND target=?", "activity.template.archive", row.ID).Error; e != nil {
		t.Fatal("missing audit", e)
	}
}
