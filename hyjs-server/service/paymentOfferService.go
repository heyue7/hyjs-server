package service

import (
	"context"
	"errors"
	"math"
	"sort"
	"strconv"
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

// paymentOffer 原6-1神装触发需要真实过关；原20资质套装与红箱测试商品独立。
// 礼包码仅哈希保存，单角色及共享次数和发奖同一事务；外部激活服务不模拟成功。
func (s *GameService) paymentOffer(ctx context.Context, id int64, request proto.Message, now time.Time) ([]proto.Message, error) {
	var response proto.Message
	err := s.p3Profile(ctx, id, func(tx *gorm.DB, row *db.Profile, p *pb.Player, g *model.GameState) error {
		switch b := request.(type) {
		case *pb.CSConvertGiftCode:
			value := strings.TrimSpace(b.GetGiftCode())
			if len(value) < 1 || len(value) > 128 {
				return p3Error()
			}
			var code db.PaymentGiftCode
			lookup := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&code, "hash = ?", p3Hash([]byte(value))).Error
			if errors.Is(lookup, gorm.ErrRecordNotFound) && value != strings.ToUpper(value) {
				lookup = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&code, "hash = ?", p3Hash([]byte(strings.ToUpper(value)))).Error
				if lookup == nil && !strings.HasPrefix(code.Source, "admin-cdk:") {
					lookup = gorm.ErrRecordNotFound
				}
			}
			if lookup != nil {
				return errors.New(constUtil.P3PaymentUnavailable)
			}
			if err := s.adminCDKEligible(tx, code, now); err != nil {
				return err
			}
			// Only operator CDKs opt into the zero-quota policy; existing legacy zero-limit codes remain closed.
			unlimitedClaims := code.Limit == 0 && strings.HasPrefix(code.Source, "admin-cdk:")
			if code.Source == "" || code.Limit < 0 || code.Limit == 0 && !unlimitedClaims || code.Used < 0 || code.Used == math.MaxInt64 || !unlimitedClaims && code.Used >= code.Limit || now.UnixMilli() >= code.ExpiresAt || (code.DemoOnly && (!strings.HasPrefix(row.Uid, "demo-") || row.Id == 1)) {
				return errors.New(constUtil.P3PaymentUnavailable)
			}
			gain, err := s.p3Claim(tx, p, g, "gift:"+code.Hash, "lifetime", code.Reward, now)
			if err != nil {
				return err
			}
			code.Used++
			if err = tx.Save(&code).Error; err != nil {
				return err
			}
			if err = saveProfile(tx, row, p, g); err != nil {
				return err
			}
			response = &pb.SCConvertGiftCode{Get: gain}
		case *pb.CSOpenLuxuryEquipment:
			var config map[string]string
			for _, r := range s.Tables["LuxuryEquip"] {
				if r["chapter"] == strconv.Itoa(int(b.GetChapter())) && r["stage"] == strconv.Itoa(int(b.GetMission())) {
					config = r
				}
			}
			if config == nil {
				response = &pb.SCOpenLuxuryEquipment{OpenOrNot: proto.Int32(0)}
				return nil
			}
			// 原请求的mission是章节内从1起的序号；存档使用全局MissionId。
			// 沿原UIChapterData的数字排序映射，不能将6-1错误匹配为chapter6/MissionId1。
			missionID, err := s.luxuryOfferMission(config["chapter"], b.GetMission())
			if err != nil {
				return err
			}
			passed := false
			for _, info := range g.Copy.GetPassedCopyInfo() {
				passed = passed || (info.GetChapterId() == config["chapter"] && info.GetMissionId() == missionID && info.GetStar() > 0)
			}
			if !passed {
				return errors.New(constUtil.P3PaymentUnavailable)
			}
			offerID, e := strconv.ParseInt(config["id"], 10, 64)
			if e != nil {
				return e
			}
			money, e := strconv.ParseInt(config["money"], 10, 64)
			if e != nil {
				return e
			}
			hours, e := strconv.ParseInt(config["limitHours"], 10, 64)
			if e != nil || hours < 1 || hours > 720 || money < 1 || money > 100000 {
				return errors.New(constUtil.P3PaymentConfigError)
			}
			offer := db.PaymentLuxuryOffer{PlayerID: id, OfferID: offerID, StartAt: now.UnixMilli(), EndAt: now.UnixMilli() + hours*3600*1000, Cents: money * 100, Reward: config["prize"]}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&offer).Error; err != nil {
				return err
			}
			// An existing offer keeps its original window and claim, including when
			// the game calendar moves backwards. Never advertise it outside that window.
			if err := tx.First(&offer, "player_id = ? AND offer_id = ?", id, offerID).Error; err != nil {
				return err
			}
			open := int32(0)
			if !offer.Claimed && now.UnixMilli() >= offer.StartAt && now.UnixMilli() < offer.EndAt {
				open = 1
			}
			response = &pb.SCOpenLuxuryEquipment{OpenOrNot: proto.Int32(open)}
		case *pb.CSLuxuryEquipmentInfo, *pb.CSLuxuryEquipmentReward:
			var offer db.PaymentLuxuryOffer
			found := tx.Where("player_id = ?", id).Order("start_at DESC").First(&offer).Error
			if errors.Is(found, gorm.ErrRecordNotFound) {
				if _, ok := request.(*pb.CSLuxuryEquipmentReward); ok {
					return errors.New(constUtil.P3PaymentUnavailable)
				}
				response = &pb.SCLuxuryEquipmentInfo{Id: proto.Int32(0), CurrentTime: proto.Int64(now.UnixMilli()), EndTime: proto.Int64(now.UnixMilli()), RewardState: proto.Int32(0)}
				return nil
			}
			if found != nil {
				return found
			}
			if now.UnixMilli() < offer.StartAt || now.UnixMilli() >= offer.EndAt {
				if _, ok := request.(*pb.CSLuxuryEquipmentReward); ok {
					return errors.New(constUtil.P3PaymentUnavailable)
				}
				response = &pb.SCLuxuryEquipmentInfo{Id: proto.Int32(0), CurrentTime: proto.Int64(now.UnixMilli()), EndTime: proto.Int64(now.UnixMilli()), RewardState: proto.Int32(0)}
				return nil
			}
			end := offer.EndAt
			// 个人日历回拨后，只展示当前游玩时刻已经发生的充值资格。
			if GameCalendarContextBound(ctx) && now.UnixMilli() < end {
				end = now.UnixMilli() + 1
			}
			total, _, _, err := PaymentTotals(tx, id, offer.StartAt, end)
			if err != nil {
				return err
			}
			status := int32(0)
			if total >= offer.Cents {
				status = 1
			}
			if offer.Claimed {
				status = 2
			}
			if _, ok := request.(*pb.CSLuxuryEquipmentReward); ok {
				if now.UnixMilli() < offer.StartAt || now.UnixMilli() >= offer.EndAt || status != 1 {
					return errors.New(constUtil.P3PaymentUnavailable)
				}
				gain, err := s.GrantFixedReward(p, g, offer.Reward)
				if err != nil {
					return err
				}
				offer.Claimed = true
				if err = tx.Save(&offer).Error; err != nil {
					return err
				}
				if err = saveProfile(tx, row, p, g); err != nil {
					return err
				}
				response = &pb.SCLuxuryEquipmentReward{Get: gain}
			} else {
				response = &pb.SCLuxuryEquipmentInfo{Id: proto.Int32(int32(offer.OfferID)), CurrentTime: proto.Int64(now.UnixMilli()), EndTime: proto.Int64(offer.EndAt), RewardState: proto.Int32(status)}
			}
		default:
			return p3Error()
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{response}, nil
}

// luxuryOfferMission 复用原UI按关卡数字ID排序后赋level_seq的章节内序号。
// 返回真实原表ID，礼包报价、触发窗口及已创建实例保持原有合同。
func (s *GameService) luxuryOfferMission(chapter string, stage int32) (string, error) {
	var missions []int
	for _, row := range s.Tables["CopyMission"] {
		if row["ChapterId"] != chapter {
			continue
		}
		id, err := strconv.Atoi(row["MissionId"])
		if err != nil || id < 1 {
			return "", errors.New(constUtil.P3PaymentConfigError)
		}
		missions = append(missions, id)
	}
	if stage < 1 || int(stage) > len(missions) {
		return "", errors.New(constUtil.P3PaymentConfigError)
	}
	sort.Ints(missions)
	return strconv.Itoa(missions[stage-1]), nil
}

// luxurySignReward 读取后台配置；缺少原奖励时保持关闭，不补设钻石奖励。
func luxurySignReward(tx *gorm.DB, now time.Time) (string, error) {
	if !gameSchemaHasTable(tx, &db.ActivityRules{}) || !gameSchemaHasTable(tx, &db.ActivityReward{}) {
		return "", nil
	}
	var rule db.ActivityRules
	err := tx.First(&rule, "id=?", constUtil.P3LuxurySignActivityID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if !rule.Enabled || now.UnixMilli() < rule.StartAt || now.UnixMilli() >= rule.EndAt {
		return "", nil
	}
	var reward db.ActivityReward
	err = tx.Where("activity_id=? AND reward_type=?", rule.ID, constUtil.P3LuxurySignRewardType).First(&reward).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if _, err := activityLoginResources(reward.Reward); err != nil {
		return "", err
	}
	return reward.Reward, nil
}

// luxurySignStatus 独立当日支付资格与领奖；确认事实累计达到原6元门槛。
func (s *GameService) luxurySignStatus(tx *gorm.DB, id int64, now time.Time) (int32, error) {
	reward, err := luxurySignReward(tx, now)
	if err != nil || reward == "" {
		return 0, err
	}
	// 旧版本及独立月签到测试尚未启用支付域时保留关闭状态；缺表不能产生付费资格。
	if !gameSchemaHasTable(tx, &db.PaymentOrder{}) || !gameSchemaHasTable(tx, &db.MembershipClaim{}) {
		return 0, nil
	}
	local := now.In(time.FixedZone("UTC+8", 8*3600))
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
	end := start.AddDate(0, 0, 1).UnixMilli()
	if GameCalendarContextBound(tx.Statement.Context) && now.UnixMilli() < end {
		end = now.UnixMilli() + 1
	}
	cents, _, _, err := PaymentTotals(tx, id, start.UnixMilli(), end)
	if err != nil {
		return 0, err
	}
	need, e := strconv.ParseFloat(s.table("GameConfigure", "StaticName", "LuxurySignRechargeAmount")["NameValue"], 64)
	if e != nil || need <= 0 {
		return 0, errors.New(constUtil.P3PaymentConfigError)
	}
	claimed, err := p3ClaimExists(tx, id, "luxury-sign", p3Day(now))
	if err != nil {
		return 0, err
	}
	if claimed {
		return 2, nil
	}
	if float64(cents) >= need*100 {
		return 1, nil
	}
	return 0, nil
}
func (s *GameService) receiveLuxurySignTx(tx *gorm.DB, row *db.Profile, p *pb.Player, g *model.GameState, now time.Time) (proto.Message, error) {
	status, err := s.luxurySignStatus(tx, p.GetPlayerId(), now)
	if err != nil {
		return nil, err
	}
	if status != 1 {
		return nil, errors.New(constUtil.P3PaymentUnavailable)
	}
	reward, err := luxurySignReward(tx, now)
	if err != nil {
		return nil, err
	}
	if reward == "" {
		return nil, errors.New(constUtil.P3PaymentUnavailable)
	}
	gain, err := s.p3Claim(tx, p, g, "luxury-sign", p3Day(now), reward, now)
	if err != nil {
		return nil, err
	}
	if err = saveProfile(tx, row, p, g); err != nil {
		return nil, err
	}
	return &pb.SCReceiveLuxurySign{Get: gain}, nil
}
