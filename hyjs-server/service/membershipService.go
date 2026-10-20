package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p3ClaimExists(tx *gorm.DB, id int64, kind, period string) (bool, error) {
	var count int64
	err := tx.Model(&db.MembershipClaim{}).Where("player_id = ? AND kind = ? AND period = ?", id, kind, period).Count(&count).Error
	return count != 0, err
}
func (s *GameService) p3Claim(tx *gorm.DB, p *pb.Player, g *model.GameState, kind, period, reward string, now time.Time) (*pb.ResultObject, error) {
	exists, err := p3ClaimExists(tx, p.GetPlayerId(), kind, period)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, errors.New(constUtil.P3PaymentUnavailable)
	}
	gain, err := s.GrantFixedReward(p, g, reward)
	if err != nil {
		return nil, err
	}
	err = tx.Create(&db.MembershipClaim{PlayerID: p.GetPlayerId(), Kind: kind, Period: period, Reward: reward, At: now.UnixMilli()}).Error
	return gain, err
}

// cardPaidReward 从确认订单的冻结快照取每日奖励，裸Player资格不能伪造支付事实。
func cardPaidReward(tx *gorm.DB, id int64, kind string, now time.Time) (int64, int64, error) {
	orders, err := localPlayPaymentOrders(tx, id, 0, now.UnixMilli()+1)
	if err != nil {
		return 0, 0, err
	}
	var reward, paidEnd int64
	for _, o := range orders {
		p, err := p3OrderProduct(&o)
		if err != nil {
			return 0, 0, err
		}
		if p.Kind == kind {
			reward = p.Free
			if kind == "month" {
				paidEnd = max(paidEnd, o.ConfirmedAt) + constUtil.P3MonthCardDurationMS
			}
		}
	}
	return reward, paidEnd, nil
}
func (s *GameService) cardStatus(tx *gorm.DB, p *pb.Player, kind string, now time.Time) (string, int64, error) {
	reward, paidEnd, err := cardPaidReward(tx, p.GetPlayerId(), kind, now)
	if err != nil {
		return "", 0, err
	}
	end := p.GetMonthCardEndTime()
	// Rewinding the local calendar cannot borrow a purchase or renewal from a
	// later play date. The original persisted entitlement remains untouched.
	if GameCalendarContextBound(tx.Statement.Context) {
		end = paidEnd
	}
	valid := (kind == "month" && end > now.UnixMilli()) || (kind == "permanent" && p.GetPermanentCardValid() == 1)
	if !valid || reward <= 0 {
		return "notValid", reward, nil
	}
	claimed, err := p3ClaimExists(tx, p.GetPlayerId(), "card:"+kind, p3Day(now))
	if err != nil {
		return "", 0, err
	}
	if claimed {
		return "alreadyClaim", reward, nil
	}
	return "canClaim", reward, nil
}
func p3Week(now time.Time) string {
	local := now.In(time.FixedZone("UTC+8", 8*3600))
	year, week := local.ISOWeek()
	return fmt.Sprintf("%04d-W%02d", year, week)
}
func (s *GameService) vipBoon(tx *gorm.DB, p *pb.Player, now time.Time) ([]*pb.ActivityDailyBoonItem, []*pb.ActivityShopItem, error) {
	_, _, count, err := PaymentTotals(tx, p.GetPlayerId(), 0, now.UnixMilli()+1)
	if err != nil {
		return nil, nil, err
	}
	// 已保存档位优先；未建表或缺行时沿用当前硬编码默认，关闭策略时不开放领取。
	form, policyEnabled, _, err := s.adminNativeVIPPolicy(tx, false)
	if err != nil {
		return nil, nil, err
	}
	tier, ok := adminNativeVIPTierFor(form, int64(p.GetVip()))
	if !ok {
		return nil, nil, errors.New(constUtil.P3PaymentConfigError)
	}
	dailyStatus := int32(0)
	weekStatus := int32(0)
	left := int32(0)
	if policyEnabled && count > 0 && p.GetVip() > 0 {
		dailyStatus = 1
		weekStatus = 1
		left = 1
	}
	claimed, err := p3ClaimExists(tx, p.GetPlayerId(), "vip:daily", p3Day(now))
	if err != nil {
		return nil, nil, err
	}
	if claimed {
		dailyStatus = 2
	}
	bought, err := p3ClaimExists(tx, p.GetPlayerId(), "vip:week", p3Week(now))
	if err != nil {
		return nil, nil, err
	}
	if bought {
		weekStatus = 2
		left = 0
	}
	dailyRewards, err := adminNativePBResources(tier.DailyRewards)
	if err != nil {
		return nil, nil, err
	}
	weekRewards, err := adminNativePBResources(tier.WeekRewards)
	if err != nil {
		return nil, nil, err
	}
	weekCost, err := adminNativePBResources([]model.AdminResourceAmount{tier.WeekCost})
	if err != nil || len(weekCost) != 1 || weekCost[0].GetId() != "diamond" {
		return nil, nil, errors.New(constUtil.P3PaymentConfigError)
	}
	daily := []*pb.ActivityDailyBoonItem{{Vip: proto.Int32(p.GetVip()), Reward: dailyRewards, Status: proto.Int32(dailyStatus), NeedRecharge: proto.Int64(60)}}
	week := []*pb.ActivityShopItem{{Index: proto.Int32(1), GetResource: weekRewards, CostResource: weekCost, Total: proto.Int32(1), Left: proto.Int32(left), Status: proto.Int32(weekStatus), PickOneOrGetAll: proto.Int32(1), Discount: proto.String("1"), Name: proto.String("会员每周福利")}}
	return daily, week, nil
}

// membership 卡每日、VIP每日及每周独立周期，资源消耗和唯一领取同事务保存。
// VIP福利档位是可审计本地策略；原VipPerks的成长/容量及玩法门槛仍沿原表。
func (s *GameService) membership(ctx context.Context, id int64, request proto.Message, now time.Time) ([]proto.Message, error) {
	var response proto.Message
	err := s.p3Profile(ctx, id, func(tx *gorm.DB, row *db.Profile, p *pb.Player, g *model.GameState) error {
		changed := false
		switch b := request.(type) {
		case *pb.CSIsCardClaimed:
			month, _, err := s.cardStatus(tx, p, "month", now)
			if err != nil {
				return err
			}
			permanent, _, err := s.cardStatus(tx, p, "permanent", now)
			if err != nil {
				return err
			}
			response = &pb.SCIsCardClaimed{MonthStatus: proto.String(month), PermanentStatus: proto.String(permanent)}
		case *pb.CSClaimCard:
			if b.GetCardType() != "month" && b.GetCardType() != "permanent" {
				return p3Error()
			}
			status, reward, err := s.cardStatus(tx, p, b.GetCardType(), now)
			if err != nil {
				return err
			}
			if status != "canClaim" {
				return errors.New(constUtil.P3PaymentUnavailable)
			}
			gain, err := s.p3Claim(tx, p, g, "card:"+b.GetCardType(), p3Day(now), fmt.Sprintf("diamond,%d", reward), now)
			if err != nil {
				return err
			}
			response = &pb.SCClaimCard{Get: gain}
			changed = true
		case *pb.CSGetVipBoonInfo:
			daily, week, err := s.vipBoon(tx, p, now)
			if err != nil {
				return err
			}
			response = &pb.SCGetVipBoonInfo{Daily: daily, Week: week}
		case *pb.CSGetVipDailyBoonReward:
			daily, _, err := s.vipBoon(tx, p, now)
			if err != nil {
				return err
			}
			if daily[0].GetStatus() != 1 {
				return errors.New(constUtil.P3PaymentUnavailable)
			}
			// 领取内容来自同一已保存档位，不再固定为第一项金币。
			content, err := adminNativePBResourceSpec(daily[0].GetReward())
			if err != nil {
				return err
			}
			gain, err := s.p3Claim(tx, p, g, "vip:daily", p3Day(now), content, now)
			if err != nil {
				return err
			}
			daily[0].Status = proto.Int32(2)
			response = &pb.SCGetVipDailyBoonReward{Daily: daily, Get: gain}
			changed = true
		case *pb.CSGetVipWeekBoonReward:
			_, week, err := s.vipBoon(tx, p, now)
			if err != nil {
				return err
			}
			if b.GetIndex() != 1 || week[0].GetStatus() != 1 || !p3ResourcesEqual(b.GetCostResources(), week[0].CostResource) || !p3ResourcesEqual(b.GetGetResources(), week[0].GetResource) {
				return p3Error()
			}
			cost := week[0].GetCostResource()
			if len(cost) != 1 || cost[0].GetId() != "diamond" || cost[0].GetNum() <= 0 {
				return errors.New(constUtil.P3PaymentUnavailable)
			}
			costValue := int64(cost[0].GetNum())
			if p.GetDiamond() < costValue {
				return errors.New(constUtil.P3PaymentUnavailable)
			}
			content, err := adminNativePBResourceSpec(week[0].GetResource)
			if err != nil {
				return err
			}
			p.Diamond = proto.Int64(p.GetDiamond() - costValue)
			gain, err := s.p3Claim(tx, p, g, "vip:week", p3Week(now), content, now)
			if err != nil {
				return err
			}
			if err := s.RecordP1Event(tx, id, "vip:week:"+p3Week(now), constUtil.P1EventDiamondSpent, costValue, now); err != nil {
				return err
			}
			week[0].Status = proto.Int32(2)
			week[0].Left = proto.Int32(0)
			response = &pb.SCGetVipWeekBoonReward{Week: week, Get: gain, CostResources: week[0].CostResource}
			changed = true
		default:
			return p3Error()
		}
		if changed {
			return saveProfile(tx, row, p, g)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{response}, nil
}

func p3ResourcesEqual(a, b []*pb.BaseResource) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] == nil || b[i] == nil || a[i].GetId() != b[i].GetId() || a[i].GetNum() != b[i].GetNum() || a[i].GetGuid() != "" {
			return false
		}
	}
	return true
}
func p3Resources(reward string) ([]*pb.BaseResource, error) {
	var result []*pb.BaseResource
	for _, part := range strings.Split(reward, ";") {
		pair := strings.Split(part, ",")
		if len(pair) != 2 || pair[0] == "" {
			return nil, p3Error()
		}
		n, e := strconv.ParseInt(pair[1], 10, 32)
		if e != nil || n <= 0 {
			return nil, p3Error()
		}
		result = append(result, &pb.BaseResource{Id: proto.String(pair[0]), Num: proto.Int32(int32(n))})
	}
	return result, nil
}

// VIPPerks 查询只投影原表，购买次数的实际使用方依旧须各自持久化每日消费。
func (s *GameService) VIPPerks(vip int32) (map[string]int64, error) {
	row := s.table("VipPerks", "vip", strconv.Itoa(int(vip)))
	if row == nil {
		return nil, errors.New(constUtil.P3PaymentConfigError)
	}
	result := map[string]int64{}
	for key, value := range row {
		if n, err := strconv.ParseInt(value, 10, 64); err == nil {
			result[key] = n
		}
	}
	return result, nil
}
