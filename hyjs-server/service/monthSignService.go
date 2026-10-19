package service

import (
	"context"
	"errors"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"strconv"
	"time"
)

// InitializeMonthSign 补齐独立表与缺失日档，已有运营SQL配置保持原值。
func (s *GameService) InitializeMonthSign(ctx context.Context) error {
	d := db.GetIns().WithContext(ctx)
	if err := d.AutoMigrate(&db.MonthSignRule{}, &db.MonthSignState{}); err != nil {
		return err
	}
	return d.Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		for day := int32(1); day <= 31; day++ {
			r := db.MonthSignRule{Day: day, Reward: reMonthSignReward(day), DoubleVIP: 0, Enabled: true, Source: constUtil.ReMonthSignSource}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&r).Error; err != nil {
				return err
			}
			// 仅替换从未启用的旧自动奖历；运营保存和已领取快照保持。
			legacy := "coin," + strconv.FormatInt(int64(day)*1000, 10)
			if err := tx.Model(&db.MonthSignRule{}).Where("day=? AND reward=? AND double_vip=? AND enabled=? AND source=?", day, legacy, 2, false, constUtil.LegacyMonthSignSeedSource).Updates(map[string]any{"reward": r.Reward, "double_vip": r.DoubleVIP, "enabled": true, "source": r.Source}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// monthSign 使用真实服务器时间；测试通过独立时钟入口验证跨日跨月。
func (s *GameService) monthSign(ctx context.Context, id int64, request proto.Message, stamps ...int64) ([]proto.Message, error) {
	return s.monthSignAt(ctx, id, request, LocalPlayNow(ctx), stamps...)
}

// monthSignAt 锁角色行，将资源增量和签到快照在同一事务内保存。
func (s *GameService) monthSignAt(ctx context.Context, id int64, request proto.Message, now time.Time, stamps ...int64) ([]proto.Message, error) {
	var response proto.Message
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var row db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error; err != nil {
			return err
		}
		player, game, err := decodeProfileForCalendarUpdate(tx, &row)
		if err != nil {
			return err
		}
		if _, ok := request.(*pb.CSReceiveLuxurySign); ok {
			response, err = s.receiveLuxurySignTx(tx, &row, player, game, now)
			return err
		}
		switch request.(type) {
		case *pb.CSRewardMonthSignList, *pb.CSReceiveMonthSign:
		default:
			return errors.New("月签到请求类型无效")
		}
		var stamp int64
		if len(stamps) > 0 {
			stamp = stamps[0]
		}
		if claim, ok := request.(*pb.CSReceiveMonthSign); ok {
			prior, replayErr := localPlayMonthSignReplay(tx, id, claim, stamp)
			if replayErr != nil {
				return replayErr
			}
			if prior != nil {
				if err := s.ValidateCopyAwardReceiptCurrent(tx, id, game, &pb.SCCopyFightEnd{Gain: prior.GetGet()}); err != nil {
					return err
				}
				response = prior
				return nil
			}
		}
		local := now.In(time.FixedZone("UTC+8", 8*3600))
		month, day := local.Format("2006-01"), local.Format("2006-01-02")
		calendarDays := time.Date(local.Year(), local.Month()+1, 0, 0, 0, 0, 0, local.Location()).Day()
		rules, err := localPlayMonthSignRules(tx)
		if err != nil {
			return err
		}
		if rules == nil {
			if err := tx.Where("day <= ?", calendarDays).Order("day").Find(&rules).Error; err != nil {
				return err
			}
		} else {
			rules = rules[:calendarDays]
		}
		if len(rules) != calendarDays {
			return errors.New("月签到SQL日历缺失")
		}
		for i, r := range rules {
			if r.Day != int32(i+1) || !r.Enabled || r.DoubleVIP < 0 {
				return errors.New("月签到SQL日历无效或已关闭")
			}
			supported, err := s.openServerRewardSupported(r.Reward)
			if err != nil {
				return err
			}
			if !supported {
				return errors.New("签到奖励资源尚未恢复")
			}
		}
		state := db.MonthSignState{PlayerID: id, Month: month}
		scoped, err := localPlayMonthSignState(tx, id, month)
		if err != nil {
			return err
		}
		if scoped != nil {
			state = *scoped
		} else {
			err = tx.Where("player_id = ? AND month = ?", id, month).First(&state).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}
		if state.Days < 0 || state.Days > int32(calendarDays) || state.LastDay > day || (state.Days > 0 && (state.LastDay == "" || state.LastReward == "" || state.LastDoubleVIP < 0)) {
			return errors.New("月签到存档无效")
		}
		today := state.LastDay == day
		current := state.Days + 1
		if today || current > int32(calendarDays) {
			current = state.Days
		}
		if _, ok := request.(*pb.CSReceiveMonthSign); ok {
			content := ""
			double := false
			if today {
				if state.Doubled || state.LastDoubleVIP <= 0 || player.GetVip() < state.LastDoubleVIP {
					return errors.New("今日签到已领取或VIP不足")
				}
				content = state.LastReward
				state.Doubled = true
			} else {
				if state.Days >= int32(calendarDays) {
					return errors.New("本月签到已完成")
				}
				r := rules[state.Days]
				content = r.Reward
				double = r.DoubleVIP > 0 && player.GetVip() >= r.DoubleVIP
				state.Days++
				state.LastDay = day
				state.LastReward = r.Reward
				state.LastDoubleVIP = r.DoubleVIP
				state.Doubled = double || r.DoubleVIP == 0
			}
			// 双倍使用同一固定奖串的重复条目，基础资源引擎逐项校验并累加。
			if double {
				content += ";" + content
			}
			gain, err := s.GrantFixedReward(player, game, content)
			if err != nil {
				return err
			}
			if err := localPlaySaveMonthSign(tx, &state); err != nil {
				return err
			}
			if err := saveProfile(tx, &row, player, game); err != nil {
				return err
			}
			reply := &pb.SCReceiveMonthSign{Get: gain}
			if err := localPlayMonthSignSaveReceipt(tx, id, request.(*pb.CSReceiveMonthSign), stamp, reply); err != nil {
				return err
			}
			response = reply
			return nil
		}
		var items []*pb.MonthRewardItem
		for _, r := range rules {
			status := int32(0)
			if r.Day < current {
				status = 3
			}
			if r.Day == current {
				status = 1
				if today {
					status = 2
					if state.Doubled {
						status = 3
					}
					r.Reward = state.LastReward
					r.DoubleVIP = state.LastDoubleVIP
				}
			}
			items = append(items, &pb.MonthRewardItem{Day: proto.Int32(r.Day), GetState: proto.Int32(status), RewardId: proto.String(r.Reward), DoubleNeedVipLevel: proto.Int32(r.DoubleVIP)})
		}
		luxuryState, err := s.luxurySignStatus(tx, id, now)
		if err != nil {
			return err
		}
		luxuryReward, err := luxurySignReward(tx, now)
		if err != nil {
			return err
		}
		response = &pb.SCRewardMonthSignList{List: items, CurrentDay: proto.Int32(current), LuxuryState: proto.Int32(luxuryState), LuxuryReward: proto.String(luxuryReward), LuxuryNeedPay: proto.String("6")}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{response}, nil
}

// reMonthSignReward 复用Re现成七档，原服奖历未知的边界保留在来源中。
func reMonthSignReward(day int32) string {
	rewards := []string{"coin,100000", "item_007,40", "item_010,30", "item_006,60", "item_005,60", "diamond,300", "item_016,5"}
	return rewards[(day-1)%int32(len(rewards))]
}
