package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

// TestAdminNativePetRecruitFixedPoolAndSave 覆盖真实规则投影、权限、原子保存和固定名单校验。
func TestAdminNativePetRecruitFixedPoolAndSave(t *testing.T) {
	f, operator := adminNativeFixture(t, false)
	if err := f.s.LoadP9Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().AutoMigrate(&db.P9PetRecruitRule{}); err != nil {
		t.Fatal(err)
	}
	var pool []p9RecruitOption
	for _, pet := range f.s.Tables["Pet"] {
		switch pet["quality"] {
		case "18":
			pool = append(pool, p9RecruitOption{PetID: pet["id"], Weight: 3762})
		case "20":
			pool = append(pool, p9RecruitOption{PetID: pet["id"], Weight: 21, Guarantee: true})
		}
	}
	raw, err := json.Marshal(pool)
	if err != nil {
		t.Fatal(err)
	}
	rule := db.P9PetRecruitRule{ID: constUtil.P9RecruitRuleID, Enabled: true, Pool: raw, Source: "test fixture"}
	if err := db.GetIns().Create(&rule).Error; err != nil {
		t.Fatal(err)
	}
	item := adminNativeFind(t, f, operator, "pet-recruit")
	if item.ID != "pet-recruit:permanent" || item.Name != "精灵扭蛋" || !item.Permanent || !item.Enabled || item.Frozen || len(item.Form.Prizes) != 59 {
		t.Fatalf("unexpected pet recruit projection: %+v", item)
	}
	var purple, orange int64
	for _, prize := range item.Form.Prizes {
		if prize.ID < 0 || int(prize.ID) >= len(f.s.Tables["Pet"]) {
			t.Fatalf("invalid original table index: %+v", prize)
		}
		pet := f.s.Tables["Pet"][int(prize.ID)]
		if prize.ResourceID != pet["id"] || prize.Amount != 1 || prize.Stock != 0 || prize.Guaranteed != (pet["quality"] == "20") {
			t.Fatalf("invalid fixed pet projection: %+v", prize)
		}
		if prize.Guaranteed {
			orange += prize.Weight
		} else {
			purple += prize.Weight
		}
	}
	if purple != orange*99 {
		t.Fatalf("ordinary quality ratio: purple=%d orange=%d", purple, orange)
	}
	viewer := db.AdminUser{Username: "pet-recruit-viewer", PasswordHash: "unused", Role: constUtil.AdminRoleViewer, Enabled: true}
	if err := db.GetIns().Create(&viewer).Error; err != nil {
		t.Fatal(err)
	}
	viewerPrincipal := &model.AdminPrincipal{UserID: viewer.ID, Username: viewer.Username, Role: viewer.Role}
	if _, err := f.s.AdminSaveNativeActivity(f.ctx, viewerPrincipal, item.ID, model.AdminNativeActivitySaveRequest{Version: item.Version, Enabled: false, Form: item.Form}); !errors.Is(err, constUtil.ErrAdminPermission) {
		t.Fatalf("viewer save accepted: %v", err)
	}
	item.Form.Prizes[0].Weight++
	saved, err := f.s.AdminSaveNativeActivity(f.ctx, operator, item.ID, model.AdminNativeActivitySaveRequest{Version: item.Version, Enabled: false, Alias: "运营配置", Form: item.Form})
	if err != nil {
		t.Fatal(err)
	}
	if saved.Version == item.Version || saved.Enabled || saved.Alias != "运营配置" || saved.Form.Prizes[0].Weight != item.Form.Prizes[0].Weight {
		t.Fatalf("save did not persist: %+v", saved)
	}
	if err := db.GetIns().First(&rule, "id = ?", constUtil.P9RecruitRuleID).Error; err != nil {
		t.Fatal(err)
	}
	if rule.Enabled || rule.Source != constUtil.P9RecruitSource || json.Unmarshal(rule.Pool, &pool) != nil || pool[0].Weight != saved.Form.Prizes[0].Weight {
		t.Fatalf("source, pool or switch did not persist atomically: %+v", rule)
	}
	if _, err := f.s.AdminSaveNativeActivity(f.ctx, operator, item.ID, model.AdminNativeActivitySaveRequest{Version: item.Version, Enabled: true, Form: item.Form}); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatalf("stale version accepted: %v", err)
	}
	var auditCount int64
	if err := db.GetIns().Model(&db.AdminAudit{}).Where("target = ? AND action = ?", item.ID, "native-activity.update").Count(&auditCount).Error; err != nil || auditCount != 1 {
		t.Fatalf("save audit count=%d err=%v", auditCount, err)
	}
	if err := f.s.AdminRemoveNativeActivity(f.ctx, operator, item.ID, saved.Version); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatalf("permanent pool removal accepted: %v", err)
	}
	redID := ""
	for _, pet := range f.s.Tables["Pet"] {
		if pet["quality"] == "23" {
			redID = pet["id"]
			break
		}
	}
	if redID == "" {
		t.Fatal("missing red pet fixture")
	}
	cases := map[string]func(*model.AdminNativeActivityForm){
		"missing":    func(form *model.AdminNativeActivityForm) { form.Prizes = form.Prizes[1:] },
		"duplicate":  func(form *model.AdminNativeActivityForm) { form.Prizes[1] = form.Prizes[0] },
		"red":        func(form *model.AdminNativeActivityForm) { form.Prizes[0].ResourceID = redID },
		"unknown":    func(form *model.AdminNativeActivityForm) { form.Prizes[0].ResourceID = "pet_unknown" },
		"id":         func(form *model.AdminNativeActivityForm) { form.Prizes[0].ID = 9999 },
		"amount":     func(form *model.AdminNativeActivityForm) { form.Prizes[0].Amount = 2 },
		"stock":      func(form *model.AdminNativeActivityForm) { form.Prizes[0].Stock = 1 },
		"guarantee":  func(form *model.AdminNativeActivityForm) { form.Prizes[0].Guaranteed = !form.Prizes[0].Guaranteed },
		"weight":     func(form *model.AdminNativeActivityForm) { form.Prizes[0].Weight = 0 },
		"weight-max": func(form *model.AdminNativeActivityForm) { form.Prizes[0].Weight = 1000000000001 },
		"price":      func(form *model.AdminNativeActivityForm) { form.DiamondCost = 1 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			data, _ := json.Marshal(saved.Form)
			var form model.AdminNativeActivityForm
			if err := json.Unmarshal(data, &form); err != nil {
				t.Fatal(err)
			}
			mutate(&form)
			if _, err := f.s.AdminSaveNativeActivity(f.ctx, operator, item.ID, model.AdminNativeActivitySaveRequest{Version: saved.Version, Enabled: true, Alias: saved.Alias, Form: form}); !errors.Is(err, constUtil.ErrAdminInput) {
				t.Fatalf("invalid %s accepted: %v", name, err)
			}
		})
	}
	still := adminNativeFind(t, f, operator, "pet-recruit")
	if still.Version != saved.Version || still.Enabled {
		t.Fatal("failed saves changed the persisted pool")
	}
	poolBefore := append([]byte(nil), rule.Pool...)
	if err := db.GetIns().Model(&rule).Update("source", "other operator's source").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.AdminSaveNativeActivity(f.ctx, operator, item.ID, model.AdminNativeActivitySaveRequest{Version: saved.Version, Enabled: true, Alias: saved.Alias, Form: saved.Form}); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatalf("source change did not conflict: %v", err)
	}
	still = adminNativeFind(t, f, operator, "pet-recruit")
	enabled, err := f.s.AdminSaveNativeActivity(f.ctx, operator, item.ID, model.AdminNativeActivitySaveRequest{Version: still.Version, Enabled: true, Alias: still.Alias, Form: still.Form})
	if err != nil || !enabled.Enabled {
		t.Fatalf("enable failed: %v", err)
	}
	if err := db.GetIns().First(&rule, "id = ?", constUtil.P9RecruitRuleID).Error; err != nil {
		t.Fatal(err)
	}
	if !rule.Enabled || rule.Source != "other operator's source" || !bytes.Equal(poolBefore, rule.Pool) {
		t.Fatal("enable changed the pool or source")
	}
	var legacy []p9RecruitOption
	for _, pet := range f.s.Tables["Pet"] {
		if pet["quality"] == "18" || pet["quality"] == "20" || pet["quality"] == "23" {
			legacy = append(legacy, p9RecruitOption{PetID: pet["id"], Weight: 1, Guarantee: pet["quality"] == "20" || pet["quality"] == "23"})
		}
	}
	legacyRaw, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&rule).Updates(map[string]any{"pool": legacyRaw, "source": "old fixture"}).Error; err != nil {
		t.Fatal(err)
	}
	old := adminNativeFind(t, f, operator, "pet-recruit")
	if !old.Frozen || len(old.Form.Prizes) != 78 || old.Note == "" {
		t.Fatalf("legacy pool should be visible and frozen: %+v", old)
	}
	if _, err := f.s.AdminSaveNativeActivity(f.ctx, operator, item.ID, model.AdminNativeActivitySaveRequest{Version: old.Version, Enabled: false, Alias: old.Alias, Form: old.Form}); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatalf("legacy pool accepted a save: %v", err)
	}
}
