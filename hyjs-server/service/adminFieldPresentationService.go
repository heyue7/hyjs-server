package service

import (
	"fmt"
	"strings"

	"hyjs-server/model"
)

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
	for _, r := range s.Tables["WorldBoss"] {
		if r["show"] != "1" {
			continue
		}
		hero := s.table("Hero", "id", r["HeroId"])
		out[r["BossId"]] = model.AdminFieldPresentation{Name: text(r["name"], text(hero["name"], "世界首领")), IconURL: heroIcon(hero, r["HeroId"]), Detail: "全服共享首领周期"}
	}
	return out
}
