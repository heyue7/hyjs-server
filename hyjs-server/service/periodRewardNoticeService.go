package service

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
)

// periodRewardNoticeCopy 用已冻结的达标条件解释奖励，不向玩家展示配置来源。
func periodRewardNoticeCopy(cfg db.PeriodInstance, index int) (string, string) {
	title := strings.TrimSpace(strings.TrimPrefix(cfg.Title, "Re"))
	if title == "" {
		title = "活动奖励"
	}
	tiers := p1PeriodMilestones(cfg)
	if index < 0 || index >= len(tiers) {
		return title, title + "已达标，奖励已发放。"
	}
	goal := tiers[index].Goal
	var condition string
	switch cfg.Kind {
	case constUtil.P1EventLoginDay:
		condition = fmt.Sprintf("累计登录达到%d天", goal)
	case constUtil.P1EventCopyWin:
		condition = fmt.Sprintf("主线通关达到%d次", goal)
	case constUtil.P1EventDiamondSpent:
		condition = fmt.Sprintf("累计消耗达到%d钻石", goal)
	case constUtil.P1EventCoinSpent:
		condition = fmt.Sprintf("累计消耗达到%d金币", goal)
	case constUtil.P4ArenaWinKind:
		condition = fmt.Sprintf("竞技场获胜达到%d次", goal)
	case constUtil.P4PlunderActivityKind:
		condition = fmt.Sprintf("夺宝达到%d次", goal)
	case constUtil.P7RushStarEvent:
		condition = fmt.Sprintf("试炼星数达到%d星", goal)
	case constUtil.P1EventRecruit:
		condition = fmt.Sprintf("招募达到%d次", goal)
	case constUtil.P1EventEquipStrengthen:
		condition = fmt.Sprintf("装备强化达到%d次", goal)
	case constUtil.P1EventPurchase:
		condition = fmt.Sprintf("累计购买达到%d件", goal)
	case constUtil.P4KindCollect:
		condition = fmt.Sprintf("收集角色达到%d种", goal)
	case constUtil.P4KindPower:
		condition = fmt.Sprintf("战力达到%d", goal)
	case "payment_amount":
		amount := strings.TrimRight(strings.TrimRight(fmt.Sprintf("%d.%02d", goal/100, goal%100), "0"), ".")
		condition = "累计充值达到" + amount + "元"
	case "wanted_attack":
		condition = fmt.Sprintf("悬赏挑战达到%d次", goal)
	case "wanted_kill":
		condition = fmt.Sprintf("悬赏击杀达到%d次", goal)
	default:
		condition = title + "已达标"
	}
	if cfg.Mode == "daily" {
		condition = "今日" + strings.TrimPrefix(condition, "累计")
	} else if cfg.Mode == "single" && (cfg.Kind == constUtil.P1EventDiamondSpent || cfg.Kind == constUtil.P1EventCoinSpent) {
		condition = "单次" + strings.TrimPrefix(condition, "累计")
	}
	return title, condition + "，奖励已发放。"
}

// projectPeriodRewardNotices 批量读取角色快照，同时修正历史和新投递的展示副本。
// 来源、数量、领取状态及原始记录保持原值，配置编辑也不会改写旧奖励的达标门槛。
func projectPeriodRewardNotices(tx *gorm.DB, playerID int64, rows []db.RewardDelivery) error {
	keys := map[string]bool{}
	indices := map[string]int{}
	instances := map[string]string{}
	for _, row := range rows {
		if !strings.HasPrefix(row.SourceKey, "activity:") {
			continue
		}
		key := strings.TrimPrefix(row.SourceKey, "activity:")
		index := 0
		if offset := strings.LastIndex(key, ":tier:"); offset >= 0 {
			tier, err := strconv.Atoi(key[offset+6:])
			if err != nil || tier < 2 {
				continue
			}
			index = tier - 1
			key = key[:offset]
		}
		keys[key] = true
		indices[row.SourceKey] = index
		instances[row.SourceKey] = key
	}
	if len(keys) == 0 {
		return nil
	}
	ids := make([]string, 0, len(keys))
	for key := range keys {
		ids = append(ids, key)
	}
	var progress []db.PeriodProgress
	if err := tx.Where("player_id=? AND instance_id IN ?", playerID, ids).Find(&progress).Error; err != nil {
		return err
	}
	configs := map[string]db.PeriodInstance{}
	for _, row := range progress {
		var cfg db.PeriodInstance
		if json.Unmarshal(row.Snapshot, &cfg) == nil && cfg.ID == row.InstanceID {
			configs[row.InstanceID] = cfg
		}
	}
	missing := []string{}
	for _, id := range ids {
		if _, ok := configs[id]; !ok {
			missing = append(missing, id)
		}
	}
	if len(missing) > 0 {
		var periods []db.PeriodInstance
		if err := tx.Where("id IN ?", missing).Find(&periods).Error; err != nil {
			return err
		}
		for _, cfg := range periods {
			// 缺少角色快照时只采用已冻结规则，避免拿后来编辑的配置解释旧奖励。
			var frozen db.PeriodInstance
			if json.Unmarshal(cfg.Frozen, &frozen) != nil || frozen.ID != cfg.ID {
				continue
			}
			configs[cfg.ID] = frozen
		}
	}
	for i := range rows {
		if key, ok := instances[rows[i].SourceKey]; ok {
			if cfg, found := configs[key]; found {
				rows[i].Title, rows[i].Content = periodRewardNoticeCopy(cfg, indices[rows[i].SourceKey])
			} else {
				// 已归档或缺失快照时不推测门槛，仍保留玩家可用的领取入口。
				rows[i].Title = strings.TrimSpace(strings.TrimPrefix(rows[i].Title, "Re"))
				rows[i].Content = "活动目标已达成，奖励已发放。"
			}
		}
	}
	return nil
}
