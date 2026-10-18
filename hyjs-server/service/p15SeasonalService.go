package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

type p15SeasonalChild struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Domain string `json:"domain"`
}

// isP15SeasonalChild preserves original first-prefix parsing while isolating root tabs.
func isP15SeasonalChild(id string) bool {
	first := strings.SplitN(id, "_", 2)[0]
	return first == "totalLoginFestival" || first == "totalLoginUpdate" || first == "limitExchangeFestival" || first == "limitExchangeUpdate"
}
func isP15SeasonalParent(id string) bool {
	return strings.HasPrefix(id, "festival_p4_") || strings.HasPrefix(id, "update_p4_")
}

// seedP15Seasonal creates real event and commerce instances; it never overwrites edits.
func (s *GameService) seedP15Seasonal(tx *gorm.DB, now time.Time) error {
	local := now.In(time.FixedZone("UTC+8", 8*3600))
	start := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, local.Location())
	end := start.AddDate(0, 1, 0)
	suffix := start.Format("200601")
	var existing int64
	if err := tx.Model(&db.P15SeasonalBundle{}).Where("id IN ?", []string{"festival_p4_" + suffix, "update_p4_" + suffix}).Count(&existing).Error; err != nil {
		return err
	}
	if existing == 2 {
		return nil
	}
	periods := []db.PeriodInstance{
		{ID: "celebrationActivity_p4_" + suffix, Title: "庆典登录补给", Enabled: false, Kind: constUtil.P1EventLoginDay, Mode: "cumulative", Goal: 1, Rewards: "item_005,10;item_047,5", StartAt: start.UnixMilli(), EndAt: end.UnixMilli(), RewardEndAt: end.AddDate(0, 0, 7).UnixMilli(), Source: constUtil.P15SeasonalSource},
		{ID: "totalLoginFestival_p4_festival_" + suffix, Title: "节日精灵补给", Enabled: false, Kind: constUtil.P1EventLoginDay, Mode: "cumulative", Goal: 1, Rewards: "item_124,15", StartAt: start.UnixMilli(), EndAt: end.UnixMilli(), RewardEndAt: end.AddDate(0, 0, 7).UnixMilli(), Source: constUtil.P15SeasonalSource},
		{ID: "totalLoginUpdate_p4_update_" + suffix, Title: "新版三日补给", Enabled: false, Kind: constUtil.P1EventLoginDay, Mode: "cumulative", Goal: 3, Rewards: "item_013,2;item_139,2", StartAt: start.UnixMilli(), EndAt: end.UnixMilli(), RewardEndAt: end.AddDate(0, 0, 7).UnixMilli(), Source: constUtil.P15SeasonalSource},
	}
	for _, cfg := range periods {
		if err := s.validateP1Period(cfg); err != nil {
			return err
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&cfg).Error; err != nil {
			return err
		}
	}
	rules := p4CommerceRules{Products: []p4Product{p4BaseProduct("seasonalSpecialty", "item_470,10", "coin,200000", 1, 90)}}
	rules.Products[0].Name = "专精石补给"
	rules.Products[0].DailyLimit = 3
	rules.Products[0].TotalLimit = 30
	raw, err := json.Marshal(rules)
	if err != nil {
		return err
	}
	commerce := db.P4CommerceRule{ID: "limitExchangeFestival_p4_festival_" + suffix, Kind: "exchange", ShopType: 0, Enabled: false, StartAt: start.UnixMilli(), EndAt: end.UnixMilli(), ClaimEndAt: end.UnixMilli(), Rules: raw, Source: constUtil.P15SeasonalSource}
	if _, err := s.validateP4Commerce(commerce); err != nil {
		return err
	}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&commerce).Error; err != nil {
		return err
	}
	for _, spec := range []struct {
		kind, title string
		children    []p15SeasonalChild
	}{
		{"festival", "节日补给", []p15SeasonalChild{{periods[1].ID, periods[1].Title, "period"}, {commerce.ID, "专精材料兑换", "commerce"}}},
		// The original Panel loads Festival for Update; its prefix controls the update decoration.
		{"update", "新版补给", []p15SeasonalChild{{periods[2].ID, periods[2].Title, "period"}}},
	} {
		children, err := json.Marshal(spec.children)
		if err != nil {
			return err
		}
		bundle := db.P15SeasonalBundle{ID: spec.kind + "_p4_" + suffix, Kind: spec.kind, Title: spec.title, Enabled: false, StartAt: start.UnixMilli(), EndAt: end.UnixMilli(), RewardEndAt: end.AddDate(0, 0, 7).UnixMilli(), Children: children, Source: constUtil.P15SeasonalSource}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&bundle).Error; err != nil {
			return err
		}
	}
	return nil
}

func p15SeasonalSnapshot(live db.P15SeasonalBundle) (db.P15SeasonalBundle, []p15SeasonalChild, error) {
	cfg := live
	if len(live.Frozen) > 0 {
		if len(live.Frozen) > 65536 || json.Unmarshal(live.Frozen, &cfg) != nil || cfg.ID != live.ID {
			return cfg, nil, errors.New(constUtil.P15SeasonalError)
		}
	}
	children := []p15SeasonalChild{}
	if !isP15SeasonalParent(cfg.ID) || (cfg.Kind != "festival" && cfg.Kind != "update") || cfg.Title == "" || cfg.Source == "" || cfg.StartAt >= cfg.EndAt || cfg.EndAt > cfg.RewardEndAt || len(cfg.Children) > 16384 || json.Unmarshal(cfg.Children, &children) != nil || len(children) < 1 || len(children) > 16 {
		return cfg, nil, errors.New(constUtil.P15SeasonalError)
	}
	seen := map[string]bool{}
	for _, child := range children {
		prefix := strings.SplitN(child.ID, "_", 2)[0]
		if seen[child.ID] || child.Title == "" || !isP15SeasonalChild(child.ID) || !strings.Contains(child.ID, "_p4_"+cfg.Kind+"_") || (child.Domain != "period" && child.Domain != "commerce") || (cfg.Kind == "update" && (prefix != "totalLoginUpdate" || child.Domain != "period")) || (child.Domain == "commerce" && !strings.HasPrefix(prefix, "limitExchange")) || (child.Domain == "period" && !strings.HasPrefix(prefix, "totalLogin")) {
			return cfg, nil, errors.New(constUtil.P15SeasonalError)
		}
		seen[child.ID] = true
	}
	return cfg, children, nil
}

func (s *GameService) p15SeasonalTabs(tx *gorm.DB, now time.Time) ([]*pb.ActivityTab, error) {
	var rows []db.P15SeasonalBundle
	if err := tx.Where("enabled=?", true).Order("id").Find(&rows).Error; err != nil {
		return nil, err
	}
	tabs := []*pb.ActivityTab{}
	for _, live := range rows {
		allowed, err := localPlayActivityVisible(tx, live.ID)
		if err != nil {
			return nil, err
		}
		if !allowed {
			continue
		}
		cfg, _, err := p15SeasonalSnapshot(live)
		if err != nil {
			return nil, err
		}
		if now.UnixMilli() < cfg.StartAt || now.UnixMilli() >= cfg.RewardEndAt {
			continue
		}
		tabs = append(tabs, &pb.ActivityTab{ActivityId: proto.String(cfg.ID), Name: proto.String(cfg.Title), Order: proto.Int32(int32(45 + len(tabs))), Status: proto.Int32(0), Param: proto.String(cfg.Source)})
	}
	return tabs, nil
}

// p15SeasonalList freezes the exact native child directory and underlying rules together.
func (s *GameService) p15SeasonalList(ctx context.Context, id int64, request *pb.CSActivityList, now time.Time) ([]proto.Message, error) {
	out := &pb.SCActivityList{}
	err := s.p3Profile(ctx, id, func(tx *gorm.DB, _ *db.Profile, _ *pb.Player, _ *model.GameState) error {
		allowed, err := localPlayActivityAccess(tx, request.GetActivityId(), false)
		if err != nil {
			return err
		}
		if !allowed {
			return errors.New(constUtil.P15SeasonalError)
		}
		if _, err := p1LockPeriods(tx); err != nil {
			return err
		}
		var live db.P15SeasonalBundle
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&live, "id=?", request.GetActivityId()).Error; err != nil {
			return errors.New(constUtil.P15SeasonalError)
		}
		if !live.Enabled {
			return errors.New("季节活动已关闭")
		}
		cfg, children, err := p15SeasonalSnapshot(live)
		if err != nil {
			return err
		}
		if now.UnixMilli() < cfg.StartAt || now.UnixMilli() >= cfg.RewardEndAt {
			return errors.New("季节活动未开放或领奖截止")
		}
		for i, child := range children {
			visible := false
			if child.Domain == "period" {
				var rule db.PeriodInstance
				if err := tx.First(&rule, "id=?", child.ID).Error; err != nil {
					return err
				}
				if rule.Enabled {
					frozen, err := s.freezeP1Period(tx, rule, true)
					if err != nil {
						return err
					}
					if frozen.StartAt != cfg.StartAt || frozen.EndAt != cfg.EndAt || frozen.RewardEndAt != cfg.RewardEndAt {
						return errors.New(constUtil.P15SeasonalError)
					}
					visible = now.UnixMilli() < frozen.RewardEndAt
				}
			} else {
				var rule db.P4CommerceRule
				if err := tx.First(&rule, "id=?", child.ID).Error; err != nil {
					return err
				}
				if rule.Enabled {
					frozen, _, err := s.p4CommerceSnapshot(rule)
					if err != nil {
						return err
					}
					if frozen.StartAt != cfg.StartAt || frozen.EndAt != cfg.EndAt || frozen.ClaimEndAt > cfg.RewardEndAt {
						return errors.New(constUtil.P15SeasonalError)
					}
					if len(rule.Frozen) == 0 {
						raw, err := json.Marshal(frozen)
						if err != nil {
							return err
						}
						if err := tx.Model(&rule).Update("frozen", raw).Error; err != nil {
							return err
						}
					}
					visible = now.UnixMilli() < frozen.EndAt
				}
			}
			if visible {
				out.ActivityList = append(out.ActivityList, &pb.ActivityTab{ActivityId: proto.String(child.ID), Name: proto.String(child.Title), Order: proto.Int32(int32(i + 1)), Status: proto.Int32(0), Param: proto.String(cfg.Source)})
			}
		}
		if len(live.Frozen) == 0 {
			raw, err := json.Marshal(cfg)
			if err != nil {
				return err
			}
			if err := tx.Model(&live).Update("frozen", raw).Error; err != nil {
				return err
			}
		}
		out.Information = proto.String(cfg.Title + "：完成活动任务后领取奖励，兑换商品按页面条件购买。")
		return nil
	})
	return []proto.Message{out}, err
}

// p15SeasonalGuard applies parent shutdown and directory membership to direct child requests.
// Period locks precede bundle/catalogue locks on list, reward and commerce paths.
func (s *GameService) p15SeasonalGuard(tx *gorm.DB, key string, now time.Time) error {
	if !isP15SeasonalChild(key) {
		return nil
	}
	allowed, err := localPlayActivityAccess(tx, key, false)
	if err != nil {
		return err
	}
	if !allowed {
		return errors.New(constUtil.P15SeasonalError)
	}
	if _, err := p1LockPeriods(tx); err != nil {
		return err
	}
	parts := strings.Split(key, "_")
	if len(parts) != 4 || parts[1] != "p4" {
		return errors.New(constUtil.P15SeasonalError)
	}
	var live db.P15SeasonalBundle
	if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).First(&live, "id=?", parts[2]+"_p4_"+parts[3]).Error; err != nil {
		return err
	}
	if !live.Enabled {
		return errors.New("季节活动已关闭")
	}
	cfg, children, err := p15SeasonalSnapshot(live)
	if err != nil {
		return err
	}
	if now.UnixMilli() < cfg.StartAt || now.UnixMilli() >= cfg.RewardEndAt {
		return errors.New("季节活动未开放或领奖截止")
	}
	for _, child := range children {
		if child.ID == key {
			return nil
		}
	}
	return errors.New(constUtil.P15SeasonalError)
}
