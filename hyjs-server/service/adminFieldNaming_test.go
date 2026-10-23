package service

import (
	"hyjs-server/db"
	"testing"
)

func TestDesertNameMigrationPreservesCustomTemplatesAndDeployments(t *testing.T) {
	f := p1Setup(t)
	tx := db.GetIns()
	if err := tx.AutoMigrate(&db.AdminFieldTemplate{}, &db.AdminFieldDeployment{}); err != nil {
		t.Fatal(err)
	}
	rows := []db.AdminFieldTemplate{
		{ID: "legacy", Scene: "field_003", Name: "野外三区 · 原资源方案", Revision: 7, Configuration: `{"ggo":{"custom":true},"scene":"field_003"}`},
		{ID: "custom", Scene: "field_003", Name: "沙漠测试方案", Revision: 3, Configuration: `{"scene":"field_003"}`},
		{ID: "other", Scene: "field_001", Name: "野外三区", Revision: 2, Configuration: `{"scene":"field_001"}`},
	}
	for _, row := range rows {
		if err := tx.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	before := rows[0].Configuration
	deployment := db.AdminFieldDeployment{ServerID: 1, Scene: "field_003", TemplateID: "legacy", Enabled: false, Configuration: before, Revision: 5, UpdatedAt: 42}
	if err := tx.Create(&deployment).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.initializeDesertFieldName(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.initializeDesertFieldName(f.ctx); err != nil {
		t.Fatal(err)
	}
	var after db.AdminFieldDeployment
	if err := tx.First(&after, "server_id=? AND scene=?", 1, "field_003").Error; err != nil {
		t.Fatal(err)
	}
	if after != deployment {
		t.Fatal("name migration changed published snapshot")
	}
	for _, original := range rows {
		var got db.AdminFieldTemplate
		if err := tx.First(&got, "id=?", original.ID).Error; err != nil {
			t.Fatal(err)
		}
		if got.Configuration != original.Configuration {
			t.Fatal("naming migration changed map configuration")
		}
		if original.ID == "legacy" {
			if got.Name != "沙漠野外 · 原资源方案" || got.Revision != 8 || got.Configuration != before {
				t.Fatalf("legacy name or repeat migration: %+v", got)
			}
		} else if got.Name != original.Name || got.Revision != original.Revision {
			t.Fatal("custom or other-scene name overwritten")
		}
	}
}
