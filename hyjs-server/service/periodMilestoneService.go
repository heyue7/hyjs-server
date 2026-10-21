package service

import (
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// p1PeriodMilestone keeps each original reward threshold in the frozen period contract.
type p1PeriodMilestone struct {
	Goal    int64  `json:"goal"`
	Rewards string `json:"rewards"`
}

func p1PeriodMilestones(cfg db.PeriodInstance) []p1PeriodMilestone {
	if len(cfg.Milestones) == 0 {
		return []p1PeriodMilestone{{Goal: cfg.Goal, Rewards: cfg.Rewards}}
	}
	var tiers []p1PeriodMilestone
	_ = json.Unmarshal(cfg.Milestones, &tiers)
	return tiers
}

func (s *GameService) validateP1Milestones(cfg db.PeriodInstance) error {
	tiers := p1PeriodMilestones(cfg)
	if len(tiers) < 1 || len(tiers) > 31 {
		return errors.New("活动奖励档数无效")
	}
	previous := int64(0)
	for _, tier := range tiers {
		if tier.Goal <= previous || tier.Goal > math.MaxInt32 {
			return errors.New("活动目标须依次递增")
		}
		ok, err := s.openServerRewardSupported(tier.Rewards)
		if err != nil {
			return err
		}
		if !ok {
			return errors.New("活动奖励资源未恢复")
		}
		previous = tier.Goal
	}
	if tiers[0].Goal != cfg.Goal || tiers[0].Rewards != cfg.Rewards {
		return errors.New("活动首档与兼容字段不符")
	}
	return nil
}

func p1PeriodSources(cfg db.PeriodInstance) []string {
	out := []string{}
	for index := range p1PeriodMilestones(cfg) {
		identity := cfg.ID
		if cfg.RewardSourceID != "" {
			identity = cfg.RewardSourceID
		}
		key := "activity:" + identity
		if index > 0 {
			key += ":tier:" + strconv.Itoa(index+1)
		}
		out = append(out, key)
	}
	return out
}

// p1PeriodItems creates only earned deliveries from the recorded progress. Each
// tier has a stable receipt shared with the reward center, including after restart.
func (s *GameService) p1PeriodItems(tx *gorm.DB, playerID int64, progress *db.PeriodProgress, cfg db.PeriodInstance, now int64) ([]*pb.ActivityItem, []db.RewardDelivery, error) {
	value := progress.Value
	if cfg.Mode == "daily" && progress.LastDay != p1Day(time.UnixMilli(now)) && now < cfg.EndAt {
		value = 0
	}
	items := []*pb.ActivityItem{}
	deliveries := []db.RewardDelivery{}
	sources := p1PeriodSources(cfg)
	for index, tier := range p1PeriodMilestones(cfg) {
		resources, err := p1RewardResources(tier.Rewards)
		if err != nil {
			return nil, nil, err
		}
		var delivery db.RewardDelivery
		query := tx.Where("player_id=? AND source_key=?", playerID, sources[index]).First(&delivery).Error
		if query != nil && !errors.Is(query, gorm.ErrRecordNotFound) {
			return nil, nil, query
		}
		if errors.Is(query, gorm.ErrRecordNotFound) && value >= tier.Goal && now < cfg.RewardEndAt {
			row, e := s.EnqueueRewardNotice(tx, playerID, sources[index], cfg.Title, cfg.Source, tier.Rewards, cfg.RewardEndAt)
			if e != nil {
				return nil, nil, e
			}
			delivery = *row
		}
		// A migrated personal entitlement reuses the historical delivery verbatim.
		// Only an unclaimed expiry may move to this still-open personal deadline.
		if delivery.ID > 0 && cfg.RewardSourceID != "" {
			if err := personalOpening40RenewDelivery(tx, playerID, cfg, &delivery, now); err != nil {
				return nil, nil, err
			}
			resources, err = p1RewardResources(delivery.Rewards)
			if err != nil {
				return nil, nil, err
			}
		}
		status := int32(0)
		if delivery.ID > 0 {
			status = 1
			if delivery.ClaimedAt > 0 {
				status = 2
			}
			if index == 0 {
				progress.RewardID = delivery.ID
			}
		}
		items = append(items, &pb.ActivityItem{Index: proto.Int32(int32(index + 1)), Value: proto.String(strconv.FormatInt(tier.Goal, 10)), RewardList: resources, Status: proto.Int32(status), Left: proto.Int32(int32(min(value, tier.Goal))), Total: proto.Int32(int32(tier.Goal)), PickOneOrGetAll: proto.Int32(1)})
		deliveries = append(deliveries, delivery)
	}
	return items, deliveries, nil
}
