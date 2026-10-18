package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"math"
	"strconv"
	"time"
)

// LoadP11Tables 复用现有TSV读取器加载原累计积分全奖档，初始化明确校验完整11档。
func (s *GameService) LoadP11Tables(root string) error {
	if err := s.loadP7Table(root, "LimitPvpRankReward"); err != nil {
		return err
	}
	if err := s.loadP7Table(root, "GrabTimeReward"); err != nil {
		return err
	}
	return s.LoadP11LimitDailyTables(root)
}

// p11LimitRules 完整冻结原配置；经济价格和筛选窗口不随活动中的表更新漂移。
type p11LimitReward struct {
	Key    int32
	Score  int64
	Reward string
}
type p11LimitRules struct {
	Rewards    []p11LimitReward
	MatchScore int32
	MatchLevel int32
	MatchNum   int32
	FreeTimes  int32
	LoginScore int32
	RankShow   int32
	BuyMax     int32
	BuyPrice   int32
}

func (s *GameService) p11LimitOriginalRules() (*p11LimitRules, error) {
	out := &p11LimitRules{}
	fields := []struct {
		name string
		dst  *int32
	}{
		{"limitPvpMatchScore", &out.MatchScore}, {"limitPvpMatchLevel", &out.MatchLevel}, {"limitPvpMatchNum", &out.MatchNum},
		{"limitPvpMaxFreeTimes", &out.FreeTimes}, {"limitPvpFirstLogin", &out.LoginScore}, {"limitPvpRankShowNum", &out.RankShow},
		{"limitPvpMaxBuyTimes", &out.BuyMax}, {"limitPvpCostDiamond", &out.BuyPrice},
	}
	for _, field := range fields {
		n, err := s.p10Setting(field.name)
		if err != nil || n < 1 || n > 100000 {
			return nil, errors.New("限时竞技原配置缺失或越界")
		}
		*field.dst = n
	}
	if out.MatchNum > 50 || out.RankShow > 500 {
		return nil, errors.New("限时竞技原列表数量越界")
	}
	seen := map[int32]bool{}
	for _, row := range s.Tables["LimitPvpRankReward"] {
		key, err := strconv.ParseInt(row["id"], 10, 32)
		if err != nil || key < 1 || seen[int32(key)] {
			return nil, errors.New("限时竞技原累计奖励键无效")
		}
		score, err := strconv.ParseInt(row["score"], 10, 64)
		if err != nil || score < 1 || score > math.MaxInt32 {
			return nil, errors.New("限时竞技原累计奖励积分无效")
		}
		supported, err := s.openServerRewardSupported(row["reward"])
		if err != nil || !supported {
			return nil, errors.New("限时竞技完整原累计奖档资源未恢复")
		}
		if _, err := p1RewardResources(row["reward"]); err != nil {
			return nil, err
		}
		seen[int32(key)] = true
		out.Rewards = append(out.Rewards, p11LimitReward{Key: int32(key), Score: score, Reward: row["reward"]})
	}
	if len(out.Rewards) != 11 {
		return nil, errors.New("限时竞技原11个累计积分奖档不完整")
	}
	return out, nil
}
func (s *GameService) InitializeP11Limit(ctx context.Context) error {
	if _, err := s.p11LimitOriginalRules(); err != nil {
		return err
	}
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.P11LimitPolicy{}, &db.P11LimitPeriod{}, &db.P11LimitMember{}, &db.P11LimitDaily{}, &db.P11LimitRewardClaim{}); err != nil {
		return err
	}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.P11LimitPolicy{ID: 1, Source: constUtil.P11LimitSource}).Error; err != nil {
		return err
	}
	if err := s.InitializeP11LimitFight(ctx); err != nil {
		return err
	}
	return s.InitializeP11LimitDaily(ctx)
}

// p11LimitPeriod 锁定显式发布日历，首次进入冻结完整原参数；既有周期窗口及规则禁止覆盖。
func (s *GameService) p11LimitPeriod(tx *gorm.DB, now time.Time) (*db.P11LimitPeriod, *p11LimitRules, error) {
	var cfg db.P11LimitPolicy
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&cfg, 1).Error; err != nil {
		return nil, nil, err
	}
	if !cfg.Enabled || cfg.PeriodID == "" || cfg.Source == "" || cfg.StartsAt <= 0 || cfg.EndsAt <= cfg.StartsAt || now.UnixMilli() < cfg.StartsAt || now.UnixMilli() >= cfg.EndsAt {
		return nil, nil, errors.New(constUtil.P11LimitClosed)
	}
	rules, err := s.p11LimitOriginalRules()
	if err != nil {
		return nil, nil, err
	}
	raw, err := json.Marshal(rules)
	if err != nil {
		return nil, nil, err
	}
	var period db.P11LimitPeriod
	err = tx.First(&period, "id=?", cfg.PeriodID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		period = db.P11LimitPeriod{ID: cfg.PeriodID, StartsAt: cfg.StartsAt, EndsAt: cfg.EndsAt, RulesJSON: string(raw), RulesHash: p6Digest(raw), Source: cfg.Source, CreatedAt: now.UnixMilli()}
		if err = tx.Create(&period).Error; err != nil {
			return nil, nil, err
		}
	} else if err != nil {
		return nil, nil, err
	}
	if period.StartsAt != cfg.StartsAt || period.EndsAt != cfg.EndsAt || period.Source != cfg.Source || period.RulesHash != p6Digest(raw) || period.RulesHash != p6Digest([]byte(period.RulesJSON)) {
		return nil, nil, errors.New("限时竞技已冻结周期/原配置不可覆盖")
	}
	return &period, rules, nil
}

// p11LimitAt 原查询和买次同持角色/周期行锁，首次有效进入、日额度、扣费及HTTP回执原子提交。
func (s *GameService) p11LimitAt(ctx context.Context, id int64, request proto.Message, stamp int64, now time.Time) ([]proto.Message, error) {
	var reply proto.Message
	operation := ""
	switch request.(type) {
	case *pb.CSLimitPvpFindMatch:
		reply = &pb.SCLimitPvpFindMatch{}
		operation = "limit-match"
	case *pb.CSLimitPvpRank:
		reply = &pb.SCLimitPvpRank{}
		operation = "limit-rank"
	case *pb.CSBuyLimitPvpTimes:
		reply = &pb.SCBuyLimitPvpTimes{}
		operation = "limit-buy"
	case *pb.CSLimitPvpRewardInfo:
		reply = &pb.SCLimitPvpRewardInfo{}
		operation = "limit-reward-info"
	case *pb.CSGetLimitPvpRewardInfo:
		reply = &pb.SCGetLimitPvpRewardInfo{}
		operation = "limit-reward-get"
	case *pb.CSLimitPvpFightResult:
		return s.p11LimitFightAt(ctx, id, request.(*pb.CSLimitPvpFightResult), stamp, now)
	default:
		return nil, errors.New("限时竞技奖励/签到依赖原规则及权威结算，尚未开放")
	}
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var profile db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&profile, id).Error; err != nil {
			return err
		}
		p, g, err := decodeProfile(&profile)
		if err != nil {
			return err
		}
		raw, err := proto.Marshal(request)
		if err != nil {
			return err
		}
		// 已支付HTTP回执先恢复，周期结束和重启不会再次扣费或转移到新活动。
		if stamp > 0 {
			var receipt db.P11ArenaReceipt
			err = tx.First(&receipt, "player_id=? AND operation=? AND stamp=?", id, operation, stamp).Error
			if err == nil {
				if receipt.RequestHash != p6Digest(raw) {
					return errors.New("限时竞技回执请求冲突")
				}
				return proto.Unmarshal(receipt.Response, reply)
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}
		period, rules, err := s.p11LimitPeriod(tx, now)
		if err != nil {
			return err
		}
		// Seal the last real ranking before entry points or other member mutations.
		if _, err = s.PrepareP11LimitDailyMutation(ctx, tx, period, now); err != nil {
			return err
		}
		// 原资格等级未知，继续使用现有真实竞技PVPLevel准入，运营默认关闭且具名记载。
		level, err := s.p10Setting("PVPLevel")
		if err != nil {
			return err
		}
		if p.GetLevel() < level {
			return errors.New("限时竞技本地真实名册准入等级未满足")
		}
		if _, _, err = s.p11ArenaProjection(&profile, tx); err != nil {
			return err
		}
		var member db.P11LimitMember
		err = tx.First(&member, "period_id=? AND player_id=?", period.ID, id).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			member = db.P11LimitMember{PeriodID: period.ID, PlayerID: id, CreatedAt: now.UnixMilli()}
		} else if err != nil {
			return err
		}
		if member.Score < 0 {
			return errors.New("限时竞技积分存档无效")
		}
		daily := db.P11LimitDaily{PeriodID: period.ID, PlayerID: id, Day: dailyCopyDay(now)}
		firstScore := int32(0)
		err = tx.First(&daily, "period_id=? AND player_id=? AND day=?", period.ID, id, daily.Day).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if member.Score > math.MaxInt64-int64(rules.LoginScore) {
				return errors.New("限时竞技真实积分溢出")
			}
			firstScore = rules.LoginScore
			member.Score += int64(firstScore)
			daily.LoginScore = firstScore
			daily.CreatedAt = now.UnixMilli()
			if s.P1Ready {
				source := fmt.Sprintf("p11-limit-entry:%s:%d:%s", period.ID, id, daily.Day)
				if err = s.RecordP1Event(tx, id, source, "limit_arena_daily_entry", int64(firstScore), now); err != nil {
					return err
				}
			}
		} else if err != nil {
			return err
		}
		if member.Score < 0 || daily.Used < 0 || daily.Bought < 0 || daily.Bought > rules.BuyMax || daily.Used > rules.FreeTimes+daily.Bought || daily.LoginScore != rules.LoginScore {
			return errors.New("限时竞技积分/日次数存档无效")
		}
		member.UpdatedAt = now.UnixMilli()
		if err = tx.Save(&member).Error; err != nil {
			return err
		}
		switch b := request.(type) {
		case *pb.CSLimitPvpRewardInfo:
			mine, _, err := s.p11LimitList(tx, period.ID, id, rules, false)
			if err != nil {
				return err
			}
			out := &pb.SCLimitPvpRewardInfo{MyRank: proto.Int32(int32(mine.GetPvpRank()))}
			for _, reward := range rules.Rewards {
				state := int32(0)
				var policy db.P11LimitPolicy
				if err = tx.First(&policy, 1).Error; err != nil {
					return err
				}
				if policy.RewardEnabled && member.Score >= reward.Score {
					state = 1
				}
				var claims int64
				if err = tx.Model(&db.P11LimitRewardClaim{}).Where("period_id=? AND player_id=? AND reward_key=?", period.ID, id, reward.Key).Count(&claims).Error; err != nil {
					return err
				}
				if claims > 0 {
					state = 2
				}
				resources, err := p1RewardResources(reward.Reward)
				if err != nil {
					return err
				}
				out.Items = append(out.Items, &pb.LimitPvpRewardItem{Integral: proto.Int32(int32(reward.Score)), Rewards: resources, State: proto.Int32(state), RewardKey: proto.Int32(reward.Key)})
			}
			reply = out
		case *pb.CSGetLimitPvpRewardInfo:
			if stamp <= 0 {
				return errors.New("限时竞技累计奖缺少原HTTP回执时间戳")
			}
			var cfg db.P11LimitPolicy
			if err = tx.First(&cfg, 1).Error; err != nil {
				return err
			}
			if !cfg.RewardEnabled {
				return errors.New("限时竞技累计积分奖励运营本批尚未开放")
			}
			key, err := strconv.ParseInt(b.GetItemId(), 10, 32)
			if err != nil {
				return errors.New("限时竞技原累计奖键无效")
			}
			var selected *p11LimitReward
			for i := range rules.Rewards {
				if rules.Rewards[i].Key == int32(key) {
					selected = &rules.Rewards[i]
					break
				}
			}
			if selected == nil || member.Score < selected.Score {
				return errors.New("限时竞技累计积分档未达到")
			}
			var previous db.P11LimitRewardClaim
			err = tx.First(&previous, "period_id=? AND player_id=? AND reward_key=?", period.ID, id, key).Error
			if err == nil {
				// 新编号重复领已领档返回零增量，同原HTTP编号由上方回执还原第一次结果。
				reply = &pb.SCGetLimitPvpRewardInfo{Get: &pb.ResultObject{}}
			} else {
				if !errors.Is(err, gorm.ErrRecordNotFound) {
					return err
				}
				gain, err := s.GrantFixedReward(p, g, selected.Reward)
				if err != nil {
					return err
				}
				reply = &pb.SCGetLimitPvpRewardInfo{Get: gain}
				blob, err := proto.Marshal(reply)
				if err != nil {
					return err
				}
				claim := db.P11LimitRewardClaim{PeriodID: period.ID, PlayerID: id, RewardKey: key, Score: member.Score, Reward: selected.Reward, RulesHash: period.RulesHash, Response: blob, ClaimedAt: now.UnixMilli()}
				if err = tx.Create(&claim).Error; err != nil {
					return err
				}
				if err = saveProfile(tx, &profile, p, g); err != nil {
					return err
				}
				if s.P1Ready {
					source := fmt.Sprintf("p11-limit-stage:%s:%d:%d", period.ID, id, key)
					if err = s.RecordP1Event(tx, id, source, "limit_arena_score_reward", 1, now); err != nil {
						return err
					}
				}
			}
		case *pb.CSBuyLimitPvpTimes:
			// 原客户端购买走HTTP且num=1；缺少稳定请求编号不得扣费，防止断线重试追加购买。
			if stamp <= 0 {
				return errors.New("限时竞技买次缺少原HTTP回执时间戳")
			}
			n := b.GetNum()
			if n < 1 || n > rules.BuyMax-daily.Bought {
				return errors.New("限时竞技购买次数超过剩余原上限")
			}
			total := int64(n) * int64(rules.BuyPrice)
			if total > math.MaxInt32 {
				return errors.New("限时竞技原购买金额溢出")
			}
			cost := &pb.BaseResource{Id: proto.String("diamond"), Num: proto.Int32(int32(total))}
			if err = deductPoolCost(p, g, cost); err != nil {
				return err
			}
			daily.Bought += n
			if err = saveProfile(tx, &profile, p, g); err != nil {
				return err
			}
			if s.P1Ready {
				source := fmt.Sprintf("p11-limit-buy:%d:%d", id, stamp)
				if err = s.RecordP1Cost(tx, id, source, []*pb.BaseResource{cost}, now); err != nil {
					return err
				}
			}
			reply = &pb.SCBuyLimitPvpTimes{Cost: []*pb.BaseResource{cost}, FightTimes: proto.Int32(daily.Used), AllTimes: proto.Int32(rules.FreeTimes + daily.Bought)}
		case *pb.CSLimitPvpFindMatch, *pb.CSLimitPvpRank:
			mine, list, err := s.p11LimitList(tx, period.ID, id, rules, operation == "limit-match")
			if err != nil {
				return err
			}
			if operation == "limit-match" {
				// Freeze complete original actors only for a new real query. The defaultoff publication adds no scores or results.
				if err = s.prepareP11LimitMatches(ctx, tx, id, period, mine, list, now); err != nil {
					return err
				}
				reply = &pb.SCLimitPvpFindMatch{Myrank: mine, RankList: list, FightTimes: proto.Int32(daily.Used), AllTimes: proto.Int32(rules.FreeTimes + daily.Bought), FirstRewardScore: proto.Int32(firstScore)}
			} else {
				reply = &pb.SCLimitPvpRank{Myrank: mine, RankList: list}
			}
		}
		if err = tx.Save(&daily).Error; err != nil {
			return err
		}
		if stamp > 0 {
			blob, err := proto.Marshal(reply)
			if err != nil {
				return err
			}
			return tx.Create(&db.P11ArenaReceipt{PlayerID: id, Operation: operation, Stamp: stamp, RequestHash: p6Digest(raw), Response: blob, BatchID: period.ID, CreatedAt: now.UnixMilli()}).Error
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{reply}, nil
}

// p11LimitList 只展示实际周期积分和可解析实际队伍；胜利积分与竞技初入列位置互不混用。
func (s *GameService) p11LimitList(tx *gorm.DB, period string, id int64, rules *p11LimitRules, match bool) (*pb.LimitPvpRankPlayerInfo, []*pb.LimitPvpRankPlayerInfo, error) {
	var members []db.P11LimitMember
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("period_id=?", period).Order("score DESC,player_id").Find(&members).Error; err != nil {
		return nil, nil, err
	}
	var mine *pb.LimitPvpRankPlayerInfo
	all := []*pb.LimitPvpRankPlayerInfo{}
	for _, member := range members {
		var profile db.Profile
		if err := tx.First(&profile, member.PlayerID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			return nil, nil, err
		}
		info, _, err := s.p11ArenaProjection(&profile, tx)
		if err != nil {
			continue
		}
		item := &pb.LimitPvpRankPlayerInfo{Info: info, PvpIntegral: proto.Int64(member.Score), PvpRank: proto.Int64(int64(len(all) + 1))}
		all = append(all, item)
		if member.PlayerID == id {
			mine = item
		}
	}
	if mine == nil {
		return nil, nil, errors.New("限时竞技当前实际队伍无效")
	}
	result := []*pb.LimitPvpRankPlayerInfo{}
	for _, item := range all {
		if match {
			if item.Info.GetPlayerId() == id {
				continue
			}
			diff := item.GetPvpIntegral() - mine.GetPvpIntegral()
			level := int64(item.Info.GetLevel()) - int64(mine.Info.GetLevel())
			if diff < 0 {
				diff = -diff
			}
			if level < 0 {
				level = -level
			}
			if diff > int64(rules.MatchScore) || level > int64(rules.MatchLevel) {
				continue
			}
			blocked, err := p10Blocked(tx, id, item.Info.GetPlayerId())
			if err != nil {
				return nil, nil, err
			}
			if blocked {
				continue
			}
		}
		result = append(result, item)
		cap := rules.RankShow
		if match {
			cap = rules.MatchNum
		}
		if len(result) >= int(cap) {
			break
		}
	}
	return mine, result, nil
}
