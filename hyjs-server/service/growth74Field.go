package service

import (
	"context"
	"encoding/json"
	"fmt"
	"hyjs-server/db"
	"hyjs-server/model"
)

func growth74Drop(id string, n int64) model.AdminFieldDrop {
	return model.AdminFieldDrop{ResourceID: id, Quantities: []model.AdminFieldQuantity{{Amount: n, Weight: 1}}}
}
func growth74SetDrop(old []model.AdminFieldDrop, id string, n int64) []model.AdminFieldDrop {
	out := []model.AdminFieldDrop{}
	for _, v := range old {
		if v.ResourceID != id {
			out = append(out, v)
		}
	}
	return append(out, growth74Drop(id, n))
}
func (s *GameService) growth74Field(ctx context.Context, a *model.AdminPrincipal) error {
	fields, e := s.AdminFields(ctx, a)
	if e != nil {
		return e
	}
	for _, scene := range []string{"field_001", "field_002", "field_003"} {
		var cfg model.AdminFieldConfiguration
		found := false
		var target *model.AdminFieldTemplateDTO
		for i := range fields.Items {
			v := &fields.Items[i]
			if v.Configuration.Scene == scene && growth74Source(v.Name) {
				target = v
				break
			}
		}
		if target == nil {
			var dep db.AdminFieldDeployment
			if er := db.GetIns().WithContext(ctx).First(&dep, "server_id=? AND scene=?", s.OriginalServerID(), scene).Error; er == nil {
				e = json.Unmarshal([]byte(dep.Configuration), &cfg)
				found = e == nil
			}
			if !found {
				cfg, e = s.AdminFieldOriginalReference(scene)
				if e != nil {
					continue
				}
			}
			for i := range cfg.GatherGroups {
				g := &cfg.GatherGroups[i]
				for j := range g.Drops {
					for k := range g.Drops[j].Quantities {
						q := &g.Drops[j].Quantities[k]
						if q.Amount > 0 {
							factor := int64(20 + g.Level*2)
							q.Amount = (q.Amount*factor + 9) / 10
						}
					}
				}
			}
			if scene == "field_001" || scene == "field_002" {
				diamond, coin, rank1, rank23, rank410, tail := int64(500), int64(2000000), int64(500), int64(250), int64(100), int64(300)
				if scene == "field_002" {
					diamond, coin, rank1, rank23, rank410, tail = 800, 4000000, 300, 150, 60, 200
				}
				enabled := true
				cfg.OrdinaryBossEnabled = &enabled
				for i := range cfg.OrdinaryBossGroups {
					b := &cfg.OrdinaryBossGroups[i]
					b.RebornHours = 1
					b.DeathSettlement = true
					b.SproutBonusBps = 2500
					b.SproutMax = 2
					b.Drops = growth74SetDrop(b.Drops, "diamond", diamond)
					b.Drops = growth74SetDrop(b.Drops, "coin", coin)
					b.KillDrops = growth74SetDrop(b.KillDrops, "diamond", tail)
					b.RankDrops = []model.AdminFieldBossRank73{{From: 1, To: 1, Drops: []model.AdminFieldDrop{growth74Drop("diamond", rank1)}}, {From: 2, To: 3, Drops: []model.AdminFieldDrop{growth74Drop("diamond", rank23)}}, {From: 4, To: 10, Drops: []model.AdminFieldDrop{growth74Drop("diamond", rank410)}}}
				}
			}
			if cfg.GGO != nil {
				for i := range cfg.GGO.Chips {
					v := &cfg.GGO.Chips[i]
					n := []int64{3, 9, 30, 90, 300, 900}[i]
					w := []int64{550000, 250000, 130000, 50000, 18000, 2000}[i]
					one := int64(1)
					v.ExchangeValue = &n
					v.DropWeight = &w
					v.DropAmount = &one
				}
			}
			name := map[string]string{"field_001": "草原", "field_002": "雪地", "field_003": "沙漠"}[scene] + "·成长投放V12"
			target, e = s.AdminSaveField(ctx, a, model.AdminFieldSaveRequest{Name: name, Configuration: cfg})
			if e != nil {
				return fmt.Errorf("%s: %w", name, e)
			}
		}
		if target.Available {
			deps, err := s.AdminFieldDeployments(ctx, a, s.OriginalServerID())
			if err != nil {
				return err
			}
			version := ""
			for _, v := range deps.Items {
				if v.Scene == scene {
					version = v.Version
				}
			}
			if _, e = s.AdminDeployField(ctx, a, model.AdminFieldDeployRequest{ServerID: s.OriginalServerID(), Scene: scene, TemplateID: target.ID, TemplateVersion: target.Version, Version: version, Enabled: true}); e != nil {
				return e
			}
		}
	}
	r, e := s.AdminFieldResources(ctx, a)
	if e != nil {
		return e
	}
	r.BossDailyRewards = 3
	_, e = s.AdminSaveFieldResources(ctx, a, model.AdminFieldResourcesSaveRequest{AdminFieldResourcesResponse: *r, Reason: "成长投放：各图共享每日3次首领奖励"})
	return e
}
