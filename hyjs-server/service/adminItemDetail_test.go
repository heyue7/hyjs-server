package service

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

func adminItemDetailTestService(t *testing.T) *GameService {
	t.Helper()
	s := &GameService{}
	if err := s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := s.LoadAdminCatalogue("../../analysis/tables", "../../admin-web/public/media/content-manifest.json"); err != nil {
		t.Fatal(err)
	}
	return s
}

// TestAdminItemDetailOriginalCatalogue verifies exact original facts without creating a player or database.
func TestAdminItemDetailOriginalCatalogue(t *testing.T) {
	s := adminItemDetailTestService(t)
	before, _ := json.Marshal(s.AdminCatalogueData.SourceTables)
	cases := []struct{ id, label, value string }{
		{"MineItem_019", "固定属性攻击", "200"},
		{"equip_001", "基础攻击", "33"},
		{"trea_001", "最大强化等级", "100"},
		{"frag_hero_001", "合成结果", "桐人"},
	}
	for _, tc := range cases {
		out, err := s.AdminItemDetail(context.Background(), model.AdminItemDetailRequest{ID: tc.id})
		if err != nil {
			t.Fatal(tc.id, err)
		}
		found := false
		for _, section := range out.Sections {
			for _, field := range section.Fields {
				if field.Label == tc.label && field.Value == tc.value {
					found = true
				}
			}
		}
		if !found {
			t.Fatalf("%s missing exact %s=%s", tc.id, tc.label, tc.value)
		}
	}
	if _, err := s.AdminItemDetail(context.Background(), model.AdminItemDetailRequest{ID: "unknown_item"}); !errors.Is(err, constUtil.ErrAdminInput) {
		t.Fatal("unknown catalogue item accepted")
	}
	after, _ := json.Marshal(s.AdminCatalogueData.SourceTables)
	if string(before) != string(after) {
		t.Fatal("details changed original source rows")
	}
}

// TestAdminItemDetailCurrentCookingRule verifies persisted serving values, invalid rules and context cancellation.
func TestAdminItemDetailCurrentCookingRule(t *testing.T) {
	ctx := context.Background()
	if err := db.InitGame(ctx, filepath.Join(t.TempDir(), "item-detail.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		conn, _ := db.GetIns().DB()
		if conn != nil {
			conn.Close()
		}
	})
	if err := db.GetIns().AutoMigrate(&db.CookingRule{}); err != nil {
		t.Fatal(err)
	}
	s := adminItemDetailTestService(t)
	rule := db.CookingRule{ID: 1, Enabled: true, Source: "隔离验收：当前运营规则", Gains: `{"gemScale":0.5,"requireLimits":1,"limit10.att":160000,"limit10.hp":608000,"limit10.pdef":48000,"limit10.mdef":48000,"limit10.cri":240,"limit10.antiCri":240}`}
	if err := db.GetIns().Create(&rule).Error; err != nil {
		t.Fatal(err)
	}
	out, err := s.AdminItemDetail(ctx, model.AdminItemDetailRequest{ID: "FishItem_050"})
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]string{}
	for _, section := range out.Sections {
		if section.Title == "当前生效数值" {
			for _, field := range section.Fields {
				values[field.Label] = field.Value
			}
		}
	}
	for label, want := range map[string]string{"每份攻击增幅": "2000", "攻击累计上限": "160000", "每份生命增幅": "7600", "生命累计上限": "608000", "每份暴击增幅": "3", "暴击累计上限": "240"} {
		if values[label] != want {
			t.Fatalf("%s got %q want %q", label, values[label], want)
		}
	}
	var unchanged db.CookingRule
	if err := db.GetIns().Take(&unchanged, 1).Error; err != nil || unchanged != rule {
		t.Fatal("details mutated serving rule", err)
	}
	if err := db.GetIns().Model(&rule).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	out, err = s.AdminItemDetail(ctx, model.AdminItemDetailRequest{ID: "FishItem_050"})
	if err != nil {
		t.Fatal(err)
	}
	for _, section := range out.Sections {
		if section.Title == "当前生效数值" && (len(section.Fields) != 1 || section.Fields[0].Value != "未配置有效的永久料理规则") {
			t.Fatal("disabled rule exposed active values")
		}
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := s.AdminItemDetail(cancelled, model.AdminItemDetailRequest{ID: "FishItem_050"}); err == nil {
		t.Fatal("current-rule read ignored cancelled request context")
	}
}
