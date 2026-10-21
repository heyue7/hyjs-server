package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p3OrderProduct(o *db.PaymentOrder) (db.PaymentProduct, error) {
	var p db.PaymentProduct
	err := json.Unmarshal([]byte(o.Snapshot), &p)
	if err == nil {
		err = validateP3Product(&p)
	}
	return p, err
}

func p3ActivityDescription(r *db.PaymentActivityRule) string {
	switch r.Kind {
	case "first":
		return "完成一笔测试购买后，可领取一次首充礼包。"
	case "single":
		return fmt.Sprintf("活动期间，单笔测试购买达到%g元，可领取一次奖励。", float64(r.Threshold)/100)
	case "total":
		return fmt.Sprintf("活动期间，测试购买累计达到%g元，可领取一次奖励。", float64(r.Threshold)/100)
	case "continued":
		return "连续3天完成测试购买，可分别领取每日奖励和最终礼包。"
	case "consume":
		return fmt.Sprintf("活动期间，实际花费%d钻石可领取一次返利。", r.Threshold)
	}
	return "测试活动"
}
func loadP3Activity(tx *gorm.DB, id int64, activity string) (*db.PaymentActivityState, *db.PaymentActivityRule, error) {
	allowed, err := localPlayActivityAccess(tx, activity, false)
	if err != nil {
		return nil, nil, err
	}
	if !allowed {
		return nil, nil, errLocalPlayActivitiesInvalid
	}
	state := &db.PaymentActivityState{PlayerID: id, ActivityID: activity}
	rule := &db.PaymentActivityRule{}
	err = tx.First(state, "player_id = ? AND activity_id = ?", id, activity).Error
	if err == nil {
		err = json.Unmarshal([]byte(state.Snapshot), rule)
	} else if errors.Is(err, gorm.ErrRecordNotFound) {
		if err = tx.First(rule, "id = ?", activity).Error; err != nil {
			return nil, nil, err
		}
		raw, e := json.Marshal(rule)
		if e != nil {
			return nil, nil, e
		}
		state.Snapshot = string(raw)
		err = tx.Create(state).Error
	}
	if err != nil {
		return nil, nil, err
	}
	// 首充保留奖励与已领快照，开放状态跟随当前运营开关。
	if rule.Kind == "first" {
		var live db.PaymentActivityRule
		if err := tx.Select("id", "enabled").First(&live, "id = ?", activity).Error; err != nil {
			return nil, nil, err
		}
		rule.Enabled = live.Enabled
	}
	if rule.ID != activity || !rule.Enabled || rule.StartAt <= 0 || rule.StartAt >= rule.EndAt || rule.EndAt > rule.RewardEndAt || rule.Threshold <= 0 || rule.Source == "" {
		return nil, nil, errors.New(constUtil.P3PaymentConfigError)
	}
	if _, err = p3Resources(rule.Reward); err != nil {
		return nil, nil, err
	}
	return state, rule, nil
}

func p3ActivityMetric(tx *gorm.DB, id int64, r *db.PaymentActivityRule, asOf ...time.Time) (int64, error) {
	now := LocalPlayNow(tx.Statement.Context)
	if len(asOf) > 0 {
		now = asOf[0]
	}
	end := min(r.EndAt, now.UnixMilli()+1)
	metricStart := r.StartAt
	// 首充礼包按终身首笔确认订单计算，不再因旧活动窗口结束失去资格。
	if r.Kind == "first" {
		metricStart, end = 0, now.UnixMilli()+1
	}
	start, err := localPlayActivityMetricStart(tx, r.ID, metricStart)
	if err != nil {
		return 0, err
	}
	if r.Kind == "consume" {
		var events []db.EventFact
		query, e := localPlayActivityFactScope(tx, r.ID)
		if e != nil {
			return 0, e
		}
		if err := query.Where("player_id = ? AND kind = ? AND occurred_at >= ? AND occurred_at < ?", id, constUtil.P1EventDiamondSpent, start, end).Find(&events).Error; err != nil {
			return 0, err
		}
		var value int64
		for _, e := range events {
			if e.Amount <= 0 || value > math.MaxInt64-e.Amount {
				return 0, p3Error()
			}
			value += e.Amount
		}
		return value, nil
	}
	orders, err := localPlayActivityPaymentOrders(tx, id, r.ID, start, end)
	if err != nil {
		return 0, err
	}
	var value int64
	days := map[string]bool{}
	var lastDay string
	var streak int64
	for _, o := range orders {
		if o.Cents <= 0 {
			return 0, p3Error()
		}
		switch r.Kind {
		case "first":
			value = 1
		case "single":
			value = max(value, o.Cents)
		case "total":
			if value > math.MaxInt64-o.Cents {
				return 0, p3Error()
			}
			value += o.Cents
		case "continued":
			day := p3Day(time.UnixMilli(o.ConfirmedAt))
			if days[day] {
				continue
			}
			days[day] = true
			local := time.UnixMilli(o.ConfirmedAt).In(time.FixedZone("UTC+8", 8*3600))
			prior := local.AddDate(0, 0, -1).Format("2006-01-02")
			if lastDay == prior {
				streak++
			} else {
				streak = 1
			}
			lastDay = day
			value = max(value, streak)
		default:
			return 0, errors.New(constUtil.P3PaymentConfigError)
		}
	}
	return value, nil
}

func (s *GameService) p3ActivityItems(tx *gorm.DB, p *pb.Player, state *db.PaymentActivityState, r *db.PaymentActivityRule, now time.Time) ([]*pb.ActivityItem, int64, error) {
	value, err := p3ActivityMetric(tx, p.GetPlayerId(), r, now)
	if err != nil {
		return nil, 0, err
	}
	reward, err := p3Resources(r.Reward)
	if err != nil {
		return nil, 0, err
	}
	status := int32(0)
	if value >= r.Threshold {
		status = 1
	}
	if state.Claimed {
		status = 2
	}
	if r.Kind != "continued" {
		text := strconv.FormatInt(r.Threshold, 10)
		if r.Kind == "single" || r.Kind == "total" {
			text = strconv.FormatFloat(float64(r.Threshold)/100, 'f', -1, 64)
		}
		return []*pb.ActivityItem{{Index: proto.Int32(1), Value: proto.String(text), RewardList: reward, Status: proto.Int32(status), Left: proto.Int32(int32(min(value, r.Threshold))), Total: proto.Int32(int32(r.Threshold)), PickOneOrGetAll: proto.Int32(1), Message: proto.String(p3ActivityDescription(r))}}, value, nil
	}
	var items []*pb.ActivityItem
	local := now.In(time.FixedZone("UTC+8", 8*3600))
	dayStart := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
	orders, err := localPlayActivityPaymentOrders(tx, p.GetPlayerId(), r.ID, max(dayStart.UnixMilli(), r.StartAt), min(dayStart.AddDate(0, 0, 1).UnixMilli(), r.EndAt, now.UnixMilli()+1))
	if err != nil {
		return nil, 0, err
	}
	_, todayCharge, _, err := paymentTotalsFromOrders(orders)
	if err != nil {
		return nil, 0, err
	}
	for i := int32(1); i <= 3; i++ {
		dailyStatus := int32(0)
		if value >= int64(i) {
			dailyStatus = 1
		}
		claimed, e := p3ClaimExists(tx, p.GetPlayerId(), "continued:"+r.ID, string(rune('0'+i)))
		if e != nil {
			return nil, 0, e
		}
		if claimed {
			dailyStatus = 2
		}
		items = append(items, &pb.ActivityItem{Index: proto.Int32(i), Value: proto.String(strconv.Itoa(int(i))), Param: proto.String(strconv.FormatInt(todayCharge, 10)), RewardList: []*pb.BaseResource{{Id: proto.String("diamond"), Num: proto.Int32(10)}}, Status: proto.Int32(dailyStatus), Left: proto.Int32(int32(min(value, int64(i)))), Total: proto.Int32(i), PickOneOrGetAll: proto.Int32(1)})
	}
	return items, value, nil
}

// paymentActivity 独立首充/单充/累计/连续/消费实例；确认订单按人民币分，消费取P1真实事实。
// 首次观察冻结整实例；连续日档每档一次，最终奖励单独领取，宽限仅领取已达条件。
func (s *GameService) paymentActivity(ctx context.Context, id int64, request proto.Message, now time.Time) ([]proto.Message, error) {
	var response proto.Message
	err := s.p3Profile(ctx, id, func(tx *gorm.DB, row *db.Profile, p *pb.Player, g *model.GameState) error {
		activity := ""
		index := int32(1)
		kind := int32(0)
		var selected []*pb.BaseResource
		claim := false
		continued := false
		switch b := request.(type) {
		case *pb.CSActivityInfo:
			activity = b.GetActivityId()
		case *pb.CSActivityReward:
			activity = b.GetActivityId()
			index = b.GetIndex()
			selected = b.GetGetResources()
			claim = true
		case *pb.CSContinuedRechargeInfo:
			activity = b.GetActivityId()
			continued = true
		case *pb.CSContinuedRechargeReward:
			activity = b.GetActivityId()
			index = b.GetIndex()
			kind = b.GetType()
			selected = b.GetGetResources()
			claim = true
			continued = true
		default:
			return p3Error()
		}
		state, r, err := loadP3Activity(tx, id, activity)
		if err != nil {
			return err
		}
		if continued != (r.Kind == "continued") {
			return p3Error()
		}
		if r.Kind != "first" && (now.UnixMilli() < r.StartAt || now.UnixMilli() >= r.RewardEndAt) {
			return errors.New(constUtil.P3PaymentUnavailable)
		}
		items, value, err := s.p3ActivityItems(tx, p, state, r, now)
		if err != nil {
			return err
		}
		if !claim {
			if continued {
				big, e := p3Resources(r.Reward)
				if e != nil {
					return e
				}
				status := int32(0)
				if value >= r.Threshold {
					status = 1
				}
				if state.Claimed {
					status = 2
				}
				response = &pb.SCContinuedRechargeInfo{ActivityId: proto.String(activity), ItemList: items, StartTime: proto.Int64(r.StartAt), EndTime: proto.Int64(r.EndAt), RewardEndTime: proto.Int64(r.RewardEndAt), Information: proto.String(p3ActivityDescription(r)), Param: proto.String("3"), BigReward: big, Status: proto.Int32(status), Level: proto.Int32(1), CurrentIndex: proto.Int32(int32(min(value+1, 3)))}
			} else {
				response = &pb.SCActivityInfo{ActivityId: proto.String(activity), ItemList: items, StartTime: proto.Int64(r.StartAt), EndTime: proto.Int64(r.EndAt), RewardEndTime: proto.Int64(r.RewardEndAt), Information: proto.String(p3ActivityDescription(r)), Param: proto.String("1")}
			}
			return nil
		}
		reward := r.Reward
		var gain *pb.ResultObject
		if continued && kind == 0 {
			if index < 1 || index > 3 || items[index-1].GetStatus() != 1 || !p3ResourcesEqual(selected, items[index-1].RewardList) {
				return errors.New(constUtil.P3PaymentUnavailable)
			}
			gain, err = s.p3Claim(tx, p, g, "continued:"+activity, string(rune('0'+index)), "diamond,10", now)
		} else {
			if (continued && (kind != 1 || index != -1)) || (!continued && index != 1) || state.Claimed || value < r.Threshold {
				return errors.New(constUtil.P3PaymentUnavailable)
			}
			expected, e := p3Resources(reward)
			if e != nil {
				return e
			}
			// 原首充面板只提交活动ID和档位，固定奖品由服务端快照决定。
			// 非空清单仍须完全匹配，其他活动继续验证原选择结果。
			if !(r.Kind == "first" && len(selected) == 0) && !p3ResourcesEqual(selected, expected) {
				return p3Error()
			}
			gain, err = s.GrantFixedReward(p, g, reward)
			state.Claimed = true
		}
		if err != nil {
			return err
		}
		if err = tx.Save(state).Error; err != nil {
			return err
		}
		if err = saveProfile(tx, row, p, g); err != nil {
			return err
		}
		items, value, err = s.p3ActivityItems(tx, p, state, r, now)
		if err != nil {
			return err
		}
		if continued {
			status := int32(0)
			if value >= r.Threshold {
				status = 1
			}
			if state.Claimed {
				status = 2
			}
			response = &pb.SCContinuedRechargeReward{ActivityId: proto.String(activity), ItemList: items, Get: gain, Status: proto.Int32(status)}
		} else {
			response = &pb.SCActivityReward{ActivityId: proto.String(activity), ItemList: items, Get: gain}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{response}, nil
}

// PaymentActivityTabs 交由共享活动列表聚合，不覆盖已恢复累计登录等活动。
func (s *GameService) PaymentActivityTabs(ctx context.Context, now time.Time) ([]*pb.ActivityTab, error) {
	var rules []db.PaymentActivityRule
	err := db.GetIns().WithContext(ctx).Where("enabled = ? AND (kind = ? OR (start_at <= ? AND reward_end_at > ?))", true, "first", now.UnixMilli(), now.UnixMilli()).Order("id").Find(&rules).Error
	result := []*pb.ActivityTab{{ActivityId: proto.String("monthCard_local_p3"), Status: proto.Int32(1), Order: proto.Int32(45)}, {ActivityId: proto.String("vipBoon_local_p3"), Status: proto.Int32(1), Order: proto.Int32(46)}}
	for i, r := range rules {
		allowed, accessErr := localPlayActivityVisible(db.GetIns().WithContext(ctx), r.ID)
		if accessErr != nil {
			return nil, accessErr
		}
		if !allowed {
			continue
		}
		result = append(result, &pb.ActivityTab{ActivityId: proto.String(r.ID), Status: proto.Int32(1), Order: proto.Int32(int32(50 + i))})
	}
	return result, err
}
