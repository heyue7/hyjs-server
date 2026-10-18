package service

import (
	"context"
	"errors"
	"fmt"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"strconv"
	"time"
)

// p4Rebate 从幂等实际扣费事实计费，在领取事务里冻结当日金额；跨入口共用P1投递。
func (s *GameService) p4Rebate(ctx context.Context, id int64, request proto.Message, stamp int64, now time.Time) ([]proto.Message, error) {
	var key string
	claim := false
	switch b := request.(type) {
	case *pb.CSActivityInfo:
		key = b.GetActivityId()
	case *pb.CSActivityReward:
		key = b.GetActivityId()
		claim = true
	case *pb.CSConsumeReturnReward:
		key = b.GetActivityId()
		claim = true
	default:
		return nil, errors.New("返利请求无效")
	}
	var response proto.Message
	err := updateProfileWithTx(ctx, id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		if claim {
			old, e := p4ReadReceipt(tx, id, key, stamp, request)
			if e != nil {
				return e
			}
			if old != nil {
				response = old
				return nil
			}
		}
		row, cfg, rules, state, err := s.loadP4Welfare(tx, id, constUtil.P4WelfareRebate, now)
		if err != nil {
			return err
		}
		expected := "consumeRet_p4_" + time.UnixMilli(cfg.StartAt).In(time.FixedZone("UTC+8", 8*3600)).Format("200601")
		expected, err = localPlayWelfareActivityKey(tx, constUtil.P4WelfareRebate, expected)
		if err != nil {
			return err
		}
		if key != expected {
			return errors.New("返利实例不属于当前窗口")
		}
		day := p1Day(now)
		source := "rebate:" + key + ":" + day
		var delivery db.RewardDelivery
		err = tx.Where("player_id=? AND source_key=?", id, source).First(&delivery).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		amount := int64(0)
		status := int32(0)
		if delivery.ID > 0 {
			reward, e := p1RewardResources(delivery.Rewards)
			if e != nil || len(reward) != 1 || reward[0].GetId() != "diamond" {
				return errors.New("返利投递快照损坏")
			}
			amount = int64(reward[0].GetNum())
			status = 1
			if delivery.ClaimedAt > 0 {
				status = 2
			}
		} else {
			var facts []db.EventFact
			query, e := localPlayActivityFactScope(tx, cfg.ID)
			if e != nil {
				return e
			}
			if err := query.Where("player_id=? AND kind=? AND day=? AND occurred_at>=? AND occurred_at<? AND occurred_at<=?", id, constUtil.P1EventDiamondSpent, day, cfg.StartAt, cfg.EndAt, now.UnixMilli()).Find(&facts).Error; err != nil {
				return err
			}
			// 上限只需1000钻消费，累计提前截断避免审计大数溢出。
			basis := int64(0)
			needed := (rules.RebateCap*10000 + rules.RebateBps - 1) / rules.RebateBps
			for _, f := range facts {
				if f.Amount < 1 {
					return errors.New("返利消费事实无效")
				}
				basis += min(f.Amount, needed-basis)
				if basis >= needed {
					break
				}
			}
			amount = min(rules.RebateCap, basis*rules.RebateBps/10000)
			if amount > 0 {
				status = 1
			}
		}
		resources := []*pb.BaseResource{{Id: proto.String("diamond"), Num: proto.Int32(int32(amount))}}
		gain := &pb.ResultObject{}
		if claim {
			if amount < 1 {
				return errors.New("今日实际消费不足返利")
			}
			if b, ok := request.(*pb.CSActivityReward); ok && (b.GetIndex() != 1 || !p1SameResources(b.GetGetResources(), resources)) {
				return errors.New("返利选项或金额变化，请刷新")
			}
			if delivery.ID == 0 {
				reward, e := s.EnqueueRewardNotice(tx, id, source, "消费返利", cfg.Source, "diamond,"+strconv.FormatInt(amount, 10), cfg.EndAt)
				if e != nil {
					return e
				}
				delivery = *reward
			}
			if delivery.ClaimedAt == 0 {
				gain, err = s.GrantFixedReward(p, g, delivery.Rewards)
				if err != nil {
					return err
				}
				delivery.Result, err = proto.Marshal(gain)
				if err != nil {
					return err
				}
				delivery.ClaimedAt = now.UnixMilli()
				if err = tx.Save(&delivery).Error; err != nil {
					return err
				}
			}
			status = 2
		}
		item := &pb.ActivityItem{Index: proto.Int32(1), Value: proto.String("1"), RewardList: resources, Status: proto.Int32(status), Left: proto.Int32(int32(amount)), Total: proto.Int32(int32(rules.RebateCap)), PickOneOrGetAll: proto.Int32(1)}
		if claim {
			response = &pb.SCActivityReward{ActivityId: proto.String(key), ItemList: []*pb.ActivityItem{item}, Get: gain}
			if err = p4SaveReceipt(tx, id, key, stamp, request, response); err != nil {
				return err
			}
		} else {
			response = &pb.SCActivityInfo{ActivityId: proto.String(key), ItemList: []*pb.ActivityItem{item}, StartTime: proto.Int64(cfg.StartAt), EndTime: proto.Int64(cfg.EndAt), RewardEndTime: proto.Int64(cfg.EndAt), Param: proto.String(fmt.Sprintf("%d%%", rules.RebateBps/100)), Information: proto.String(cfg.Source)}
		}
		return saveP4Welfare(tx, row, cfg, state)
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{response}, nil
}
