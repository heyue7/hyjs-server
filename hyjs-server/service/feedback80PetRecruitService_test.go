package service

import (
	"bytes"
	"encoding/json"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"testing"
)

func TestFeedback80PetRecruitMigrationPreservesHistory(t *testing.T) {
	f, actor := adminNativeFixture(t, false)
	if err := f.s.LoadP9Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().AutoMigrate(&db.P9PetRecruitRule{}, &db.ClientUpdatePolicy{}); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.AdminUser{}).Where("id=?", actor.UserID).Update("role", constUtil.AdminRoleAdmin).Error; err != nil {
		t.Fatal(err)
	}
	actor.Role = constUtil.AdminRoleAdmin
	legacy := []p9RecruitOption{}
	for _, pet := range f.s.Tables["Pet"] {
		legacy = append(legacy, p9RecruitOption{PetID: pet["id"], Weight: 1, Guarantee: pet["quality"] != "18"})
	}
	raw, _ := json.Marshal(legacy)
	rule := db.P9PetRecruitRule{ID: "ordinary", Enabled: false, Pool: raw, Source: constUtil.P9RecruitLegacySource}
	if err := db.GetIns().Create(&rule).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Create(&db.ClientUpdatePolicy{ID: 1}).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.ApplyFeedback80Config(f.ctx, actor); err == nil {
		t.Fatal("non-maintenance migration accepted")
	}
	if err := db.GetIns().Model(&db.ClientUpdatePolicy{}).Where("id=1").Update("maintenance", true).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.ApplyFeedback80Config(f.ctx, actor); err != nil {
		t.Fatal(err)
	}
	var current db.P9PetRecruitRule
	if err := db.GetIns().First(&current, "id=?", "ordinary").Error; err != nil {
		t.Fatal(err)
	}
	input, err := f.s.p9RecruitInput(current)
	if err != nil || current.Enabled || len(input.Pool) != 59 || current.Source != constUtil.P9RecruitSource {
		t.Fatal("migration changed enable or pool", err)
	}
	input.Pool[0].Weight++
	custom, _ := json.Marshal(input.Pool)
	if err := db.GetIns().Model(&current).Update("pool", custom).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.ApplyFeedback80Config(f.ctx, actor); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().First(&current, "id=?", "ordinary").Error; err != nil || !bytes.Equal(current.Pool, custom) {
		t.Fatal("retry overwrote operator change", err)
	}
}
