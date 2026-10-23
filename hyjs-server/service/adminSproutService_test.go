package service

import (
	"errors"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	"testing"
)

func TestAdminSproutSettingsGlobalVersion55(t *testing.T) {
	f := p1Setup(t)
	actor := adminGuildTestActor(t, constUtil.AdminRoleOperator)
	if err := f.s.InitializeAdminSproutSettings(f.ctx); err != nil {
		t.Fatal(err)
	}
	a, err := f.s.AdminSproutSettings(f.ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	if a.Enabled || a.Days != nil || a.Icon != "ggo_sprout" {
		t.Fatal("unconfigured default", a)
	}
	days := int64(14)
	req := model.AdminSproutSettingsSaveRequest{Enabled: true, Days: &days, Version: a.Version, Reason: "测试"}
	b, err := f.s.AdminSaveSproutSettings(f.ctx, actor, req)
	if err != nil {
		t.Fatal(err)
	}
	if !b.Enabled || b.Days == nil || *b.Days != 14 || b.Version == a.Version {
		t.Fatal("save", b)
	}
	if _, err := f.s.AdminSaveSproutSettings(f.ctx, actor, req); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatal("stale version", err)
	}
	if err := f.s.InitializeAdminSproutSettings(f.ctx); err != nil {
		t.Fatal(err)
	}
	c, err := f.s.AdminSproutSettings(f.ctx, actor)
	if err != nil || c.Version != b.Version {
		t.Fatal("restart changed settings", err)
	}
	viewer := adminGuildTestActor(t, constUtil.AdminRoleViewer)
	req.Version = b.Version
	if _, err := f.s.AdminSaveSproutSettings(f.ctx, viewer, req); !errors.Is(err, constUtil.ErrAdminPermission) {
		t.Fatal("viewer saved", err)
	}
	var n int64
	if err := db.GetIns().Model(&db.AdminAudit{}).Where("action=?", "sprout.settings.update").Count(&n).Error; err != nil || n != 1 {
		t.Fatal("audit", n, err)
	}
}
