package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func (s *GameService) InitializeP10Worship(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.P10WorshipPolicy{}, &db.P10WorshipDaily{}, &db.P10WorshipAction{}); err != nil {
		return err
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.P10WorshipPolicy{ID: 1, Source: constUtil.P10WorshipSource}).Error
}

func p10WorshipDailyHash(row *db.P10WorshipDaily) string {
	copy := *row
	copy.SourceHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}
func p10WorshipActionHash(row *db.P10WorshipAction) string {
	copy := *row
	copy.SourceHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}

// p10WorshipRanks 从本服已创建真实角色计算原战力与等级榜，空账号不生成占位玩家。
func (s *GameService) p10WorshipRanks(tx *gorm.DB, kind int32) ([]*pb.SimplePlayerInfo, error) {
	var rows []db.Profile
	if err := tx.Order("id").Find(&rows).Error; err != nil {
		return nil, err
	}
	var ranked []*pb.SimplePlayerInfo
	for _, row := range rows {
		if row.Player == nil && row.Game == nil {
			continue
		}
		p, g, err := decodeProfile(&row)
		if err != nil {
			return nil, err
		}
		if p.GetName() == "" || p.GetHeroWarId() == "" || len(g.Warriors) == 0 {
			continue
		}
		info, err := s.p10Player(tx, row.Id)
		if err != nil {
			return nil, err
		}
		// 名人榜模型跟随当前队伍的第一位角色，读取阵容顺序而不依赖账号主角。
		var first *pb.FormationItem
		for _, slot := range g.Formation {
			hero := formationHero(slot)
			if hero != nil && hero.GetId() != "" && hero.GetGuid() != "" && (first == nil || slot.GetFormatPos() < first.GetFormatPos()) {
				first = slot
			}
		}
		if first != nil {
			info.HeroWarId = proto.String(formationHero(first).GetId())
		}
		ranked = append(ranked, info)
	}
	sort.Slice(ranked, func(i, j int) bool {
		left, right := ranked[i].GetFightPower(), ranked[j].GetFightPower()
		if kind == 1 {
			left, right = ranked[i].GetLevel(), ranked[j].GetLevel()
		}
		if left != right {
			return left > right
		}
		return ranked[i].GetPlayerId() < ranked[j].GetPlayerId()
	})
	return ranked[:min(len(ranked), constUtil.P10WorshipTopCount)], nil
}

// p10WorshipQuote 首次实际膜拜才冻结当天原报价；后续不因配置漂移重新定奖励。
func (s *GameService) p10WorshipQuote(tx *gorm.DB, id int64, day string, freeze bool) (*db.P10WorshipDaily, error) {
	var daily db.P10WorshipDaily
	err := tx.First(&daily, "player_id=? AND day=?", id, day).Error
	if err == nil {
		if daily.Limit < 1 || daily.Limit > 3 || daily.Reward == "" || len(daily.ConfigurationHash) != 64 || daily.SourceHash != p10WorshipDailyHash(&daily) {
			return nil, errors.New("原每日膜拜次数/奖励来源损坏")
		}
		return &daily, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	limit, err := s.p10Setting("AddGoodTimes")
	if err != nil || limit != 3 {
		return nil, errors.New("原每日膜拜3次配置缺失")
	}
	reward := s.table("GameConfigure", "StaticName", "AddGoodReward")["NameValue"]
	resources, err := p1RewardResources(reward)
	if err != nil || len(resources) != 1 || resources[0].GetId() != "coin" || resources[0].GetNum() != 10000 {
		return nil, errors.New("原膜拜一万金币配置缺失")
	}
	raw, err := json.Marshal(s.Tables["GameConfigure"])
	if err != nil {
		return nil, err
	}
	daily = db.P10WorshipDaily{PlayerID: id, Day: day, Limit: int64(limit), Reward: reward, ConfigurationHash: p6Digest(raw)}
	daily.SourceHash = p10WorshipDailyHash(&daily)
	if freeze {
		if err := tx.Create(&daily).Error; err != nil {
			return nil, err
		}
	}
	return &daily, nil
}

func p10WorshipActions(tx *gorm.DB, daily *db.P10WorshipDaily) ([]int64, error) {
	var rows []db.P10WorshipAction
	if err := tx.Where("player_id=? AND day=?", daily.PlayerID, daily.Day).Order("target_id").Find(&rows).Error; err != nil {
		return nil, err
	}
	if int64(len(rows)) > daily.Limit {
		return nil, errors.New("膜拜事实超过原每日次数")
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		info := &pb.SimplePlayerInfo{}
		if row.DailyHash != daily.SourceHash || row.SourceHash != p10WorshipActionHash(&row) || row.RankType < 0 || row.RankType > 1 || row.RankPosition < 1 || row.RankPosition > 3 || proto.Unmarshal(row.TargetInfo, info) != nil || info.GetPlayerId() != row.TargetID || p1Day(time.UnixMilli(row.CreatedAt)) != daily.Day {
			return nil, errors.New("真实膜拜资格/奖励事实来源SHA损坏")
		}
		ids = append(ids, row.TargetID)
	}
	return ids, nil
}

// p10Worship 同一已锁观察者串行化每日3次/两个榜单/唯一目标与金币/P1事实/原回执。
func (s *GameService) p10Worship(ctx context.Context, id int64, request proto.Message, stamp int64, now time.Time) ([]proto.Message, error) {
	var response proto.Message
	var player *pb.Player
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var profile db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&profile, id).Error; err != nil {
			return err
		}
		p, g, err := decodeProfileForCalendarUpdate(tx, &profile)
		if err != nil {
			return err
		}
		player = p
		if _, ok := request.(*pb.CSGood); ok {
			reply := &pb.SCGood{}
			found, err := p10ReadReceipt(tx, id, constUtil.P10WorshipOperation, stamp, request, reply)
			if err != nil {
				return err
			}
			if found {
				response = reply
				return nil
			}
		}
		var policy db.P10WorshipPolicy
		if err := tx.First(&policy, 1).Error; err != nil {
			return err
		}
		day := p1Day(now)
		daily, err := s.p10WorshipQuote(tx, id, day, false)
		if err != nil {
			return err
		}
		ids, err := p10WorshipActions(tx, daily)
		if err != nil {
			return err
		}
		left := int32(daily.Limit - int64(len(ids)))
		switch b := request.(type) {
		case *pb.CSFamous:
			ranks, err := s.p10WorshipRanks(tx, b.GetType())
			if err != nil {
				return err
			}
			if !policy.Enabled {
				left = 0
			}
			response = &pb.SCFamous{LeftAddGood: proto.Int32(left), GoodList: ids, PlayerList: ranks}
		case *pb.CSGood:
			if !policy.Enabled {
				return errors.New(constUtil.P10WorshipClosed)
			}
			if left <= 0 {
				return errors.New("原每日三次膜拜次数已用完")
			}
			for _, prior := range ids {
				if prior == b.GetPlayerId() {
					return errors.New("同日该玩家已实际膜拜")
				}
			}
			var target *pb.SimplePlayerInfo
			var rankType int32
			var position int64
			for _, kind := range []int32{0, 1} {
				ranks, err := s.p10WorshipRanks(tx, kind)
				if err != nil {
					return err
				}
				for i, info := range ranks {
					if info.GetPlayerId() == b.GetPlayerId() {
						target, rankType, position = info, kind, int64(i+1)
						break
					}
				}
				if target != nil {
					break
				}
			}
			if target == nil {
				return errors.New("目标不在真实等级或战力前三膜拜榜")
			}
			daily, err = s.p10WorshipQuote(tx, id, day, true)
			if err != nil {
				return err
			}
			gain, err := s.GrantFixedReward(p, g, daily.Reward)
			if err != nil {
				return err
			}
			raw, err := proto.Marshal(target)
			if err != nil {
				return err
			}
			action := db.P10WorshipAction{PlayerID: id, Day: day, TargetID: target.GetPlayerId(), RankType: rankType, RankPosition: position, TargetInfo: raw, DailyHash: daily.SourceHash, CreatedAt: now.UnixMilli()}
			action.SourceHash = p10WorshipActionHash(&action)
			if err := tx.Create(&action).Error; err != nil {
				return err
			}
			source := fmt.Sprintf("p10-worship:%s:%d", day, action.TargetID)
			if err := s.RecordP1Event(tx, id, source, constUtil.P10WorshipEvent, 1, now); err != nil {
				return err
			}
			if err := s.RecordP1Event(tx, id, source, constUtil.P10WorshipCoinEvent, 10000, now); err != nil {
				return err
			}
			if err := saveProfile(tx, &profile, p, g); err != nil {
				return err
			}
			ids = append(ids, action.TargetID)
			sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
			response = &pb.SCGood{Get: gain, LeftAddGood: proto.Int32(left - 1), GoodList: ids}
			return p10SaveReceipt(tx, id, constUtil.P10WorshipOperation, stamp, request, response)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if _, ok := request.(*pb.CSGood); ok {
		return []proto.Message{response, &pb.SCPlayer{Player: player}}, nil
	}
	return []proto.Message{response}, nil
}
