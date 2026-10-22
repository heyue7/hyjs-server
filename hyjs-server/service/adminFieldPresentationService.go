package service

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"hyjs-server/model"
)

// adminFieldSceneRows reuses the tables loaded once at startup. Missing source
// data leaves the map preview unavailable instead of probing other projects.
func (s *GameService) adminFieldSceneRows() map[string]map[string]string {
	rows := s.Tables["Scene"]
	if len(rows) == 0 && s.AdminCatalogueData != nil {
		rows = s.AdminCatalogueData.SourceTables["Scene"]
	}
	out := make(map[string]map[string]string, len(rows))
	for _, row := range rows {
		if row["SceneId"] != "" {
			out[row["SceneId"]] = row
		}
	}
	return out
}

// adminFieldMapCoordinate 读取原坐标的X与Z；Y轴不参与小地图投影。
func adminFieldMapCoordinate(raw string) (float64, float64, bool) {
	parts := strings.Split(raw, ",")
	if len(parts) < 3 {
		return 0, 0, false
	}
	x, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	if err != nil {
		return 0, 0, false
	}
	z, err := strconv.ParseFloat(strings.TrimSpace(parts[2]), 64)
	if err != nil {
		return 0, 0, false
	}
	return x, z, !math.IsNaN(x) && !math.IsInf(x, 0) && !math.IsNaN(z) && !math.IsInf(z, 0)
}

// adminFieldBossPointName 使用原首领候选名称，缺失名称时保留真实候选编号。
func (s *GameService) adminFieldBossPointName(raw string, presentation map[string]model.AdminFieldPresentation) string {
	names := []string{}
	for _, candidate := range adminFieldWeights(raw) {
		name := presentation[candidate.ID].Name
		if name == "" && candidate.ID != "" {
			if boss := s.table("FieldBoss", "BossId", candidate.ID); boss != nil {
				name = s.table("Hero", "id", boss["HeroId"])["name"]
			}
		}
		if name == "" {
			name = candidate.ID
		}
		if name == "" {
			continue
		}
		exists := false
		for _, seen := range names {
			if seen == name {
				exists = true
				break
			}
		}
		if !exists {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return "普通首领候选"
	}
	return strings.Join(names, "、")
}

// adminFieldMapMetadata 将原Scene小地图参数和首领出生点转换为只读预览数据。
func (s *GameService) adminFieldMapMetadata(field map[string]string, scenes map[string]map[string]string, presentation map[string]model.AdminFieldPresentation) *model.AdminFieldMap {
	scene := scenes[field["FieldMap"]]
	if scene == nil {
		return nil
	}
	scale := tableNumber(scene, "miniMapScale")
	if scale <= 0 || math.IsNaN(scale) || math.IsInf(scale, 0) {
		return nil
	}
	out := &model.AdminFieldMap{
		Width:   512,
		Height:  512,
		Scale:   scale,
		OffsetX: tableNumber(scene, "miniMapOffsetX"),
		OffsetY: tableNumber(scene, "miniMapOffsetY"),
	}
	for _, id := range strings.Split(field["bossPosIds"], ";") {
		if id == "" {
			continue
		}
		row := s.table("FieldBossPos", "BoxxPosId", id)
		if row == nil {
			continue
		}
		x, z, ok := adminFieldMapCoordinate(row["BornPos"])
		if !ok {
			continue
		}
		out.BossPoints = append(out.BossPoints, model.AdminFieldMapPoint{ID: id, Name: s.adminFieldBossPointName(row["bossIdProbs"], presentation), X: x, Z: z})
	}
	for _, row := range s.Tables["WorldBossPos"] {
		if row["scene"] != field["FieldId"] {
			continue
		}
		x, z, ok := adminFieldMapCoordinate(row["pos"])
		if !ok {
			continue
		}
		name := presentation[row["BossId"]].Name
		if name == "" {
			name = row["BossId"]
		}
		out.WorldBossPoints = append(out.WorldBossPoints, model.AdminFieldMapPoint{ID: row["id"], Name: name, X: x, Z: z})
	}
	sort.Slice(out.BossPoints, func(i, j int) bool { return out.BossPoints[i].ID < out.BossPoints[j].ID })
	sort.Slice(out.WorldBossPoints, func(i, j int) bool { return out.WorldBossPoints[i].ID < out.WorldBossPoints[j].ID })
	return out
}

// adminFieldPresentation projects original names and pixels without changing the saved resource IDs.
func (s *GameService) adminFieldPresentation() map[string]model.AdminFieldPresentation {
	out := map[string]model.AdminFieldPresentation{}
	text := func(key, fallback string) string {
		if s.AdminCatalogueData != nil && s.AdminCatalogueData.DisplayNames[key] != "" {
			return s.AdminCatalogueData.DisplayNames[key]
		}
		return fallback
	}
	icon := func(key string) string {
		if s.AdminCatalogueData != nil {
			return s.AdminCatalogueData.SpriteURLs[key]
		}
		return ""
	}
	heroIcon := func(hero map[string]string, id string) string {
		if value := icon(hero["iconName"]); value != "" {
			return value
		}
		return icon(id)
	}
	item := func(id string) model.AdminFieldPresentation {
		if s.AdminCatalogueData != nil {
			for _, v := range s.AdminCatalogueData.Items {
				if v.ID == id {
					return model.AdminFieldPresentation{Name: v.Name, IconURL: v.IconURL}
				}
			}
		}
		return model.AdminFieldPresentation{}
	}
	for _, field := range s.Tables["Field"] {
		out[field["FieldId"]] = model.AdminFieldPresentation{Name: s.adminFieldName(field["FieldId"]), IconURL: icon(field["FieldMap"]), Detail: "场景与战斗地图"}
	}
	for _, r := range s.Tables["FieldResource"] {
		kind := "矿脉"
		if strings.Contains(r["id"], "fish") {
			kind = "鱼群"
		}
		v := model.AdminFieldPresentation{Name: fmt.Sprintf("%d 级%s", int64(tableNumber(r, "level")), kind)}
		var names []string
		for _, drop := range adminFieldDrops(r["rewardProbId"]) {
			p := item(drop.ResourceID)
			if p.Name != "" {
				names = append(names, p.Name)
			}
			if v.IconURL == "" {
				v.IconURL = p.IconURL
			}
		}
		v.Detail = strings.Join(names, "、")
		out[r["id"]] = v
	}
	for _, r := range s.Tables["FieldMonster"] {
		if r["TeamLeader"] != "1" {
			continue
		}
		hero := s.table("Hero", "id", r["HeroId"])
		name := text(hero["name"], "野外怪物")
		out[r["MonId"]] = model.AdminFieldPresentation{Name: name, IconURL: heroIcon(hero, r["HeroId"]), Detail: fmt.Sprintf("等级 %d", int64(tableNumber(r, "Level")))}
	}
	for _, r := range s.Tables["FieldBoss"] {
		if r["TeamLeader"] != "1" || r["GroupIndex"] != "1" {
			continue
		}
		hero := s.table("Hero", "id", r["HeroId"])
		out[r["BossId"]] = model.AdminFieldPresentation{Name: text(hero["name"], "野外首领"), IconURL: heroIcon(hero, r["HeroId"]), Detail: "普通野外首领"}
	}
	for _, r := range s.Tables["WorldBoss"] {
		if r["show"] != "1" {
			continue
		}
		hero := s.table("Hero", "id", r["HeroId"])
		out[r["BossId"]] = model.AdminFieldPresentation{Name: text(r["name"], text(hero["name"], "世界首领")), IconURL: heroIcon(hero, r["HeroId"]), Detail: "全服共享首领周期"}
	}
	// 首领名称先完成投影，再汇总每张地图的原小地图参数与出生点。
	scenes := s.adminFieldSceneRows()
	for _, field := range s.Tables["Field"] {
		id := field["FieldId"]
		v := out[id]
		v.Map = s.adminFieldMapMetadata(field, scenes, out)
		out[id] = v
	}
	return out
}
