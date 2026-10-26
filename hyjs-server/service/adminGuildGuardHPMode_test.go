package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

func TestAdminGuildTemplateGuardHPModeRoundTrip(t *testing.T) {
	f, _, _, _, _ := p13UFFieldSetup(t)
	actor := adminGuildTestActor(t, constUtil.AdminRoleOperator)
	if err := f.s.InitializeAdminGuildSettings(f.ctx); err != nil {
		t.Fatal(err)
	}
	ctx := context.WithValue(f.ctx, constUtil.AdminServerScopeContextKey, int64(1))
	var row db.P13UnionFightFieldPolicy
	if err := db.GetIns().First(&row, 1).Error; err != nil {
		t.Fatal(err)
	}
	row.Enabled = true
	row.PreparationMillis = 300000
	row.LeftRevivePointID, row.RightRevivePointID = 1, 3
	row.InitialGateHP, row.MoveTolerance = 10000, 3
	row.Source = "isolated complete field policy for real admin roundtrip"
	row.GuardHPSource = constUtil.P13UnionFightGuardHPSourceMonster
	row.GuardRule = constUtil.P13UFGuardMirrorRule
	row.MapProjection = "re-flat-v1"
	row.InitialGuardHP = 0
	if err := db.GetIns().Save(&row).Error; err != nil {
		t.Fatal(err)
	}
	before, err := f.s.AdminGuildSettings(ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	field := before.Policies.UnionFight.Field
	if field.GuardHPMode != "template" || field.InitialGuardHP != 0 {
		t.Fatal("actual template mode missing", field)
	}
	// Use the real read DTO as the write payload; only another editable value changes.
	field.MoveTolerance += 0.25
	request := model.AdminGuildSettingSaveRequest{Version: before.Version, Reason: "保留模板并调整移动容差", Policies: &model.AdminGuildPolicySave{UnionFight: &model.AdminGuildUnionFightPolicySave{Field: &field}}}
	after, err := f.s.AdminSaveGuildSettings(ctx, actor, request)
	if err != nil {
		t.Fatal(err)
	}
	var saved db.P13UnionFightFieldPolicy
	if err = db.GetIns().First(&saved, 1).Error; err != nil {
		t.Fatal(err)
	}
	if saved.GuardHPSource != row.GuardHPSource || saved.GuardRule != row.GuardRule || saved.MapProjection != row.MapProjection || saved.InitialGuardHP != 0 || saved.MoveTolerance != field.MoveTolerance || after.Policies.UnionFight.Field.GuardHPMode != "template" {
		t.Fatal("source/rule/projection lost", saved)
	}
	frozen, _ := json.Marshal(saved)
	// A field mode is a read-only observation, never a client-selected policy.
	for _, mode := range []string{"uniform", "arbitrary"} {
		field.GuardHPMode = mode
		request.Version = after.Version
		if _, err = f.s.AdminSaveGuildSettings(ctx, actor, request); !errors.Is(err, constUtil.ErrAdminInput) {
			t.Fatal("mode switch accepted", mode, err)
		}
		var unchanged db.P13UnionFightFieldPolicy
		if err = db.GetIns().First(&unchanged, 1).Error; err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(unchanged)
		if string(raw) != string(frozen) {
			t.Fatal("rejected request changed policy")
		}
	}
	field.GuardHPMode = "" // Older clients omit the added observation.
	request.Version = after.Version
	if _, err = f.s.AdminSaveGuildSettings(ctx, actor, request); err != nil {
		t.Fatal("legacy form rejected existing template", err)
	}
	// A uniform source retains its original positive-HP contract.
	saved.GuardHPSource, saved.GuardRule = "", ""
	saved.InitialGuardHP = 1000
	if err = db.GetIns().Save(&saved).Error; err != nil {
		t.Fatal(err)
	}
	uniform, err := f.s.AdminGuildSettings(ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	field = uniform.Policies.UnionFight.Field
	if field.GuardHPMode != "uniform" {
		t.Fatal("uniform observation missing")
	}
	field.InitialGuardHP = 0
	request.Version = uniform.Version
	if _, err = f.s.AdminSaveGuildSettings(ctx, actor, request); !errors.Is(err, constUtil.ErrAdminInput) {
		t.Fatal("uniform zero HP accepted", err)
	}
}
