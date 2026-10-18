package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"hyjs-server/model"
)

// TestAdminCatalogueOriginalTextMediaFacetsAndExactIDs 绑定真实五行元数据原表和APK导出的图片，覆盖曾显示文本key的目录。
func TestAdminCatalogueOriginalTextMediaFacetsAndExactIDs(t *testing.T) {
	original := map[string][]map[string]string{"Hero": {{"id": "sentinel_battle_row", "name": "保留战斗表"}}}
	s := &GameService{Tables: original}
	before, _ := json.Marshal(s.Tables)
	if err := s.LoadAdminCatalogue(filepath.Join("..", "..", "analysis", "tables"), filepath.Join("..", "..", "admin-web", "public", "media", "content-manifest.json")); err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(s.Tables)
	if !reflect.DeepEqual(before, after) || s.Tables["Hero"][0]["id"] != "sentinel_battle_row" {
		t.Fatal("管理目录加载修改了战斗Tables")
	}
	expected := map[string]string{"hero_001": "桐人", "equip_001": "先驱之剑", "trea_001": "狂暴指环", "frag_hero_001": "桐人碎片", "FishItem_026": "烤爆裂鱼", "MineItem_019": "1级红色宝石", "petequip_001": "狡诈之心"}
	ids := []string{"FishItem_001", "MineItem_001", "monster_001"}
	for id := range expected {
		ids = append(ids, id)
	}
	response := s.AdminCatalogueFiltered(model.AdminItemsRequest{IDs: ids, Page: 1, PageSize: 100})
	if response.Total != int64(len(expected)) {
		t.Fatalf("精确批量解析得到%d项，应为%d；矿点/钓点/怪物应排除", response.Total, len(expected))
	}
	items := map[string]model.AdminItemDTO{}
	for _, item := range response.Items {
		if expected[item.ID] != item.Name {
			t.Errorf("%s中文名=%q，期望%q", item.ID, item.Name, expected[item.ID])
		}
		if strings.HasPrefix(item.Name, "name_") {
			t.Errorf("未解析文本key泄露：%s", item.Name)
		}
		items[item.ID] = item
	}
	// 图片只接受真实清单里的导出地址，并确认最终静态源文件存在。
	for _, id := range []string{"hero_001", "equip_001", "FishItem_026", "MineItem_019"} {
		item := items[id]
		if !strings.HasPrefix(item.IconURL, "/admin/media/items/") || item.PreviewURL == "" {
			t.Errorf("%s未绑定原图：%+v", id, item)
			continue
		}
		path := filepath.Join("..", "..", "admin-web", "public", strings.TrimPrefix(item.IconURL, "/admin/"))
		if _, err := os.Stat(path); err != nil {
			t.Errorf("%s原图不存在：%v", id, err)
		}
	}
	if items["frag_hero_001"].OriginalQuality != 23 || items["frag_hero_001"].Quality != 6 || items["frag_hero_001"].IconURL != items["hero_001"].IconURL {
		t.Fatal("角色碎片未继承真实目标的资质与图片")
	}
	if items["FishItem_026"].QualityColor != "white" || items["MineItem_019"].QualityColor != "purple" || items["hero_001"].QualityColor != "red" {
		t.Fatal("混淆了原品级和资质")
	}
	quality := int64(2)
	equipment := s.AdminCatalogueFiltered(model.AdminItemsRequest{Category: "equipment", Quality: &quality, ApplicableType: "equip.type:1", Page: 1, PageSize: 100})
	found := false
	for _, item := range equipment.Items {
		if item.ID == "equip_001" {
			found = true
		}
		if item.Quality != 2 || item.OriginalQuality != 12 || item.Category != "equipment" {
			t.Fatalf("组合筛选返回不匹配条目：%+v", item)
		}
	}
	if !found {
		t.Fatal("组合筛选未找到先驱之剑")
	}
	chinese := s.AdminCatalogueFiltered(model.AdminItemsRequest{Q: "桐人", Page: 1, PageSize: 100})
	found = false
	for _, item := range chinese.Items {
		if item.ID == "hero_001" {
			found = true
		}
	}
	if !found {
		t.Fatal("中文名搜索未返回桐人")
	}
	unresolved := s.AdminCatalogueFiltered(model.AdminItemsRequest{Q: "name_hero_001", Page: 1, PageSize: 100})
	if unresolved.Total != 0 {
		t.Fatal("搜索仍暴露原文本索引")
	}
	item := s.AdminCatalogueFiltered(model.AdminItemsRequest{IDs: []string{"item_001"}, Page: 1, PageSize: 1})
	if len(item.Items) != 1 || item.Items[0].Quality != 5 || item.Items[0].QualityLabel != "橙色 · 传说" || item.Items[0].OriginalQuality != 5 {
		t.Fatal("道具原star未保留")
	}
	if len(response.Facets.Categories) == 0 || len(response.Facets.Qualities) == 0 || len(response.Facets.ApplicableTypes) == 0 {
		t.Fatal("未生成真实目录筛选项")
	}
	preview := s.AdminCatalogueData.ActivityPreviews["turntable"]
	if !strings.HasPrefix(preview, "/admin/media/") {
		t.Fatal("转盘原部件预览未绑定")
	}
	if _, err := os.Stat(filepath.Join("..", "..", "admin-web", "public", strings.TrimPrefix(preview, "/admin/"))); err != nil {
		t.Fatal(err)
	}
}

// TestAdminCatalogueKeepsApprovedGreenFragments 确认启动兼容后的六绿碎片在后台可搜索且原表保持只读。
func TestAdminCatalogueKeepsApprovedGreenFragments(t *testing.T) {
	root := filepath.Join("..", "..", "analysis", "tables")
	s := &GameService{Tables: map[string][]map[string]string{}}
	for _, name := range []string{"Hero", "Frag"} {
		rows, err := adminCatalogueReadTSV(filepath.Join(root, "Table"+name+".tsv"))
		if err != nil {
			t.Fatal(err)
		}
		s.Tables[name] = rows
	}
	if err := s.ApplyP15GreenHeroCompatibility(); err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(s.Tables)
	if err := s.LoadAdminCatalogue(root, filepath.Join("..", "..", "admin-web", "public", "media", "content-manifest.json")); err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(s.Tables)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("目录读取修改了已应用兼容的战斗表")
	}
	ids := []string{"frag_hero_056", "frag_hero_057", "frag_hero_058", "frag_hero_059", "frag_hero_060", "frag_hero_061"}
	response := s.AdminCatalogueFiltered(model.AdminItemsRequest{IDs: ids, Page: 1, PageSize: 100})
	if response.Total != 6 {
		t.Fatalf("六绿碎片目录只有%d项", response.Total)
	}
	for _, item := range response.Items {
		if item.Quality != 2 || item.OriginalQuality != 12 || !strings.HasSuffix(item.Name, "碎片") || item.Name == item.ID {
			t.Fatalf("六绿碎片展示不完整：%+v", item)
		}
	}
}
