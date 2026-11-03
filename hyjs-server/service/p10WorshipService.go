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

// p10WorshipRankLists reads each role and its current attribute sources once.
// Only public top-three projections escape this read transaction.
func (s *GameService) p10WorshipRankLists(tx *gorm.DB, serverID int64, now time.Time) (*p10FamousRankLists, error) {
	var ranked []*pb.SimplePlayerInfo
	var lastID int64
	for {
		var ids []int64
		if err := tx.Model(&db.Profile{}).Where("server_id=? AND id>? AND player IS NOT NULL AND game IS NOT NULL", serverID, lastID).Order("id").Limit(constUtil.PlayerRank48ReadPageSize).Pluck("id", &ids).Error; err != nil {
			return nil, err
		}
		if len(ids) == 0 {
			break
		}
		reads, err := s.p13GuildReadPlayers(tx, ids, now)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			read := reads[id]
			p, g := read.Player, read.Game
			if p.GetName() == "" || p.GetHeroWarId() == "" || len(g.Warriors) == 0 {
				continue
			}
			heroID := ""
			for _, w := range g.Warriors {
				if w.GetGuid() == p.GetHeroWarId() {
					heroID = w.GetId()
					break
				}
			}
			info := &pb.SimplePlayerInfo{PlayerId: proto.Int64(id), ServerId: proto.Int32(int32(serverID)), Level: p.Level, PlayerName: p.Name, FightPower: proto.Int32(read.Power), HeroWarId: proto.String(heroID)}
			var first *pb.FormationItem
			for _, slot := range g.Formation {
				hero := formationHero(slot)
				if hero != nil && hero.GetId() != "" && hero.GetGuid() != "" && (first == nil || slot.GetFormatPos() < first.GetFormatPos()) {
					first = slot
				}
			}
			if first != nil {
				hero, err := p5Warrior(g, formationHero(first).GetGuid())
				if err != nil {
					return nil, err
				}
				weapon, err := s.battleWeaponID(g, first)
				if err != nil {
					return nil, err
				}
				info.HeroWarId = hero.Id
				info.WeaponId = proto.String(weapon)
			}
			ranked = append(ranked, info)
		}
		lastID = ids[len(ids)-1]
		if len(ids) < constUtil.PlayerRank48ReadPageSize {
			break
		}
	}
	lists := &p10FamousRankLists{}
	for kind := 0; kind < 2; kind++ {
		list := append([]*pb.SimplePlayerInfo(nil), ranked...)
		sort.Slice(list, func(i, j int) bool {
			left, right := list[i].GetFightPower(), list[j].GetFightPower()
			if kind == 1 {
				left, right = list[i].GetLevel(), list[j].GetLevel()
			}
			if left != right {
				return left > right
			}
			return list[i].GetPlayerId() < list[j].GetPlayerId()
		})
		list = list[:min(len(list), constUtil.P10WorshipTopCount)]
		if kind == 0 {
			lists.Power = list
		} else {
			lists.Level = list
		}
	}
	return lists, nil
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
	if rank, ok := request.(*pb.CSFamous); ok {
		return s.p10FamousRead(ctx, id, rank.GetType(), now)
	}
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
			lists, err := s.p10WorshipRankLists(tx, AdminProfileServerID(profile), now)
			if err != nil {
				return err
			}
			for _, kind := range []int32{0, 1} {
				ranks := lists.Power
				if kind == 1 {
					ranks = lists.Level
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
