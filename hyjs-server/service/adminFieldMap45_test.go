package service

import "testing"

// TestAdminFieldMapPresentationUsesOriginalScene 验证大地图参数与首领点位只取自原表。
func TestAdminFieldMapPresentationUsesOriginalScene(t *testing.T) {
	s := &GameService{}
	for _, load := range []func(string) error{s.LoadTables, s.LoadP8Tables, s.LoadP12WorldBossTables} {
		if err := load("../../analysis/tables"); err != nil {
			t.Fatal(err)
		}
	}
	presentation := s.adminFieldPresentation()
	forest := presentation["field_001"]
	if forest.Map == nil {
		t.Fatal("森林野外缺少原Scene小地图参数")
	}
	if forest.Map.Width != 512 || forest.Map.Height != 512 || forest.Map.Scale != 4.096 || forest.Map.OffsetX != 32.1 || forest.Map.OffsetY != 39.2 {
		t.Fatalf("森林野外小地图参数未按原Scene读取: %+v", *forest.Map)
	}
	if len(forest.Map.BossPoints) != 1 || forest.Map.BossPoints[0].ID != "boss_pos_001" || forest.Map.BossPoints[0].X != 64 || forest.Map.BossPoints[0].Z != 66 {
		t.Fatalf("森林野外普通首领点位不匹配: %+v", forest.Map.BossPoints)
	}
	if forest.Map.BossPoints[0].Name == "" {
		t.Fatal("普通首领点位缺少原候选名称")
	}
	snow := presentation["field_002"]
	if snow.Map == nil || len(snow.Map.WorldBossPoints) != 10 {
		t.Fatalf("雪原野外世界首领点位不匹配: %+v", snow.Map)
	}
	first := snow.Map.WorldBossPoints[0]
	if first.ID != "1" || first.X != 66.3 || first.Z != 22.42 || first.Name == "" {
		t.Fatalf("世界首领原点位或名称不匹配: %+v", first)
	}
	desert := presentation["field_003"]
	if desert.Map == nil || desert.Map.OffsetX != 30.4 || desert.Map.OffsetY != 35.5 {
		t.Fatalf("沙漠野外小地图偏移未按原Scene读取: %+v", desert.Map)
	}
}
