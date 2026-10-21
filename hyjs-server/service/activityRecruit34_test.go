package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func activityRecruitPatch34(t *testing.T, f *p1Fixture, actor *model.AdminPrincipal, kind, daily, pity int64) *model.AdminRecruitSettingsSaveRequest {
	t.Helper()
	rules, err := f.s.AdminRecruitSettings(f.ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	items := append([]model.AdminRecruitSetting(nil), rules.Items...)
	for i := range items {
		if items[i].Kind == kind {
			items[i].DailyLimit = daily
			items[i].GuaranteeEvery = pity
		}
	}
	return &model.AdminRecruitSettingsSaveRequest{Version: rules.Version, Reason: "isolated activity rule save", Items: items}
}

// An invalid activity must roll back its embedded rules and audit in the same transaction.
func TestActivityRecruit34KingTemplateAtomicSaveAndConflict(t *testing.T) {
	f, actor, form := adminKingFixture(t)
	definition := adminKingDefinition(t, f, actor, form)
	patch := activityRecruitPatch34(t, f, actor, 3, 60, 4)
	broken := form
	broken.Prizes = append([]model.AdminActivityPrize(nil), form.Prizes...)
	broken.Prizes[0].ResourceID = "item_005"
	_, err := f.s.AdminSaveActivityDefinition(f.ctx, actor, definition.ID, model.AdminActivityDefinitionSaveRequest{Name: form.Name, Form: broken, Version: definition.Version, DefaultDays: 1, Enabled: true, RecruitSettings: patch})
	if err == nil {
		t.Fatal("invalid activity accepted")
	}
	rules, err := f.s.AdminRecruitSettings(f.ctx, actor)
	if err != nil || rules.Version != patch.Version {
		t.Fatal("failed activity changed recruit rules", rules, err)
	}
	form.Alias = "atomic king"
	saved, err := f.s.AdminSaveActivityDefinition(f.ctx, actor, definition.ID, model.AdminActivityDefinitionSaveRequest{Name: form.Name, Form: form, Version: definition.Version, DefaultDays: 1, Enabled: true, RecruitSettings: patch})
	if err != nil {
		t.Fatal(err)
	}
	rules, err = f.s.AdminRecruitSettings(f.ctx, actor)
	if err != nil || rules.Items[1].DailyLimit != 60 || rules.Items[1].GuaranteeEvery != 4 || rules.Items[0] != patch.Items[0] {
		t.Fatal("embedded king save mismatch", rules, err)
	}
	form.Alias = "must roll back"
	_, err = f.s.AdminSaveActivityDefinition(f.ctx, actor, saved.ID, model.AdminActivityDefinitionSaveRequest{Name: form.Name, Form: form, Version: saved.Version, DefaultDays: 1, Enabled: true, RecruitSettings: patch})
	if !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatal("stale runtime version accepted", err)
	}
	var row db.AdminActivityDefinition
	if err = db.GetIns().First(&row, "id=?", saved.ID).Error; err != nil || strings.Contains(string(row.FormJSON), "must roll back") {
		t.Fatal("conflicting save changed template", err)
	}
	patch = activityRecruitPatch34(t, f, actor, 2, 99, 2)
	_, err = f.s.AdminSaveActivityDefinition(f.ctx, actor, saved.ID, model.AdminActivityDefinitionSaveRequest{Name: form.Name, Form: form, Version: saved.Version, DefaultDays: 1, Enabled: true, RecruitSettings: patch})
	if !errors.Is(err, constUtil.ErrAdminInput) {
		t.Fatal("king editor changed orange rules", err)
	}
}

// The older typed editor uses the same atomic operation and keeps failed drafts out of storage.
func TestActivityRecruit34KingTypedSaveAndRollback(t *testing.T) {
	f, actor, form := adminKingFixture(t)
	form.StartAt = time.Now().Add(-time.Hour).UnixMilli()
	form.EndAt = time.Now().Add(time.Hour).UnixMilli()
	form.ClaimEndAt = form.EndAt
	patch := activityRecruitPatch34(t, f, actor, 3, 55, 5)
	saved, err := f.s.AdminSaveTypedActivity(f.ctx, actor, "", model.AdminTypedActivityRequest{AdminActivityForm: form, Enabled: false, RecruitSettings: patch})
	if err != nil {
		t.Fatal(err)
	}
	rules, err := f.s.AdminRecruitSettings(f.ctx, actor)
	if err != nil || rules.Items[1].DailyLimit != 55 || rules.Items[1].GuaranteeEvery != 5 {
		t.Fatal("typed activity rules not committed", rules, err)
	}
	patch = activityRecruitPatch34(t, f, actor, 3, 58, 8)
	broken := saved.Form
	broken.Prizes = append([]model.AdminActivityPrize(nil), saved.Form.Prizes...)
	broken.Prizes[0].ResourceID = "item_005"
	_, err = f.s.AdminSaveTypedActivity(f.ctx, actor, saved.ID, model.AdminTypedActivityRequest{AdminActivityForm: broken, Version: saved.Version, Enabled: false, RecruitSettings: patch})
	if err == nil {
		t.Fatal("broken typed pool accepted")
	}
	rules, err = f.s.AdminRecruitSettings(f.ctx, actor)
	if err != nil || rules.Version != patch.Version {
		t.Fatal("failed typed activity changed runtime rules", err)
	}
}

// A stopped rotation leaves the permanent pool working; enabling it must drive real draws.
func TestActivityRecruit34OrangeRotationProtocolAndAtomicSave(t *testing.T) {
	f, actor := adminNativeFixture(t, false)
	if err := f.s.InitializeP2(f.ctx); err != nil {
		t.Fatal(err)
	}
	startup := context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	if err := InitializeAdminActivityModes(startup); err != nil {
		t.Fatal(err)
	}
	item := adminNativeFind(t, f, actor, "recruit-rotation")
	if item.Enabled {
		t.Fatal("fixture rotation unexpectedly enabled")
	}
	// The base original message remains available while both seeded rotations are off.
	if _, err := f.s.recruitInfo(f.ctx, f.id, &pb.CSWarriorRecruitGet{CostType: []int32{2}}); err != nil {
		t.Fatal("permanent strong pool disconnected", err)
	}
	patch := activityRecruitPatch34(t, f, actor, 2, 1, 1)
	bad := item.Form
	bad.Prizes = append([]model.AdminActivityPrize(nil), item.Form.Prizes...)
	bad.Prizes[0].ResourceID = "item_005"
	_, err := f.s.AdminSaveNativeActivity(f.ctx, actor, item.ID, model.AdminNativeActivitySaveRequest{Version: item.Version, Enabled: false, Form: bad, RecruitSettings: patch})
	if err == nil {
		t.Fatal("invalid orange pool accepted")
	}
	rules, err := f.s.AdminRecruitSettings(f.ctx, actor)
	if err != nil || rules.Version != patch.Version {
		t.Fatal("invalid orange save partially committed rules", err)
	}
	// A finite one-hero orange pool makes the original draw prove which source won.
	var guaranteed model.AdminActivityPrize
	for _, p := range item.Form.Prizes {
		if p.Guaranteed {
			guaranteed = p
			break
		}
	}
	if guaranteed.ResourceID == "" {
		t.Fatal("no original orange candidate")
	}
	item.Form.Prizes = []model.AdminActivityPrize{guaranteed}
	saved, err := f.s.AdminSaveNativeActivity(f.ctx, actor, item.ID, model.AdminNativeActivitySaveRequest{Version: item.Version, Enabled: true, Form: item.Form, RecruitSettings: patch})
	if err != nil {
		t.Fatal(err)
	}
	if !saved.Enabled {
		t.Fatal("explicit rotation enable did not persist")
	}
	draw := &pb.CSWarriorRecruit{CostType: proto.Int32(2), NumType: proto.Int32(1), Free: proto.Int32(1)}
	messages, err := f.s.warriorRecruit(f.ctx, f.id, draw, 34001)
	if err != nil {
		t.Fatal("original free draw did not use enabled rotation", err)
	}
	award := messages[0].(*pb.SCWarriorRecruit).GetGet().GetWars()
	if len(award) != 1 || award[0].GetId() != guaranteed.ResourceID {
		t.Fatal("rotation pool not connected to actual draw", award)
	}
	before, _ := f.profile(t)
	if _, err = f.s.warriorRecruit(f.ctx, f.id, &pb.CSWarriorRecruit{CostType: proto.Int32(2), NumType: proto.Int32(1)}, 34002); err == nil {
		t.Fatal("embedded daily limit not enforced")
	}
	after, _ := f.profile(t)
	if !proto.Equal(before, after) {
		t.Fatal("rejected draw deducted resources")
	}
	var rotation db.P4RecruitRotation
	if err = db.GetIns().First(&rotation, "id=?", strings.TrimPrefix(item.ID, "recruit:")).Error; err != nil || len(rotation.Frozen) == 0 {
		t.Fatal("actual draw did not freeze rotation", err)
	}
	// Frozen award content stays protected while shared future daily/pity rules remain editable.
	patch = activityRecruitPatch34(t, f, actor, 2, 2, 3)
	frozen := adminNativeFind(t, f, actor, "recruit-rotation")
	_, err = f.s.AdminSaveNativeActivity(f.ctx, actor, frozen.ID, model.AdminNativeActivitySaveRequest{Version: frozen.Version, Enabled: frozen.Enabled, Form: frozen.Form, RecruitSettings: patch})
	if err != nil {
		t.Fatal("frozen pool blocked independent live rules", err)
	}
}
