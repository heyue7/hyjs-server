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
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"strconv"
	"strings"
	"time"
)

type copyResetInput struct {
	Scope  string  `json:"scope"`
	Prices []int32 `json:"prices"`
	Source string  `json:"source"`
}
type copyResetProgress struct {
	Input copyResetInput   `json:"input"`
	Used  map[string]int32 `json:"used"`
}

func (s *GameService) initializeCopyReset(tx *gorm.DB) error {
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.CopyResetRule{ID: constUtil.CopyResetRuleID, Enabled: true, Scope: constUtil.CopyResetPlayerDay, Source: constUtil.CopyResetSource}).Error
}
func (s *GameService) copyResetQuote(tx *gorm.DB, id int64, p *pb.Player, chapter, mission string, now time.Time) (*db.CopyResetDaily, *copyResetProgress, string, int32, *pb.BaseResource, error) {
	var cfg db.CopyResetRule
	if e := tx.Clauses(clause.Locking{Strength: "SHARE"}).First(&cfg, constUtil.CopyResetRuleID).Error; e != nil {
		return nil, nil, "", 0, nil, e
	}
	if !cfg.Enabled {
		return nil, nil, "", 0, nil, errors.New("主线关卡重置暂未开放")
	}
	row := &db.CopyResetDaily{PlayerID: id, Day: p1Day(now)}
	state := &copyResetProgress{Used: map[string]int32{}}
	e := tx.First(row, "player_id=? AND day=?", id, row.Day).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		if cfg.Scope != constUtil.CopyResetPlayerDay && cfg.Scope != constUtil.CopyResetMissionDay {
			return nil, nil, "", 0, nil, errors.New("重置额度范围无效")
		}
		state.Input.Scope = cfg.Scope
		state.Input.Source = cfg.Source
		for _, v := range strings.Split(s.table("GameConfigure", "StaticName", "CopyResetDiamondCost")["NameValue"], ";") {
			n, e := strconv.ParseInt(v, 10, 32)
			if e != nil || n < 1 {
				return nil, nil, "", 0, nil, errors.New("原关卡重置价格未恢复")
			}
			state.Input.Prices = append(state.Input.Prices, int32(n))
		}
		frozen, _ := json.Marshal(state.Input)
		row.InputHash = p6Digest(frozen)
	} else if e != nil {
		return nil, nil, "", 0, nil, e
	} else {
		if json.Unmarshal(row.Payload, state) != nil {
			return nil, nil, "", 0, nil, errors.New("重置日存档损坏")
		}
		frozen, _ := json.Marshal(state.Input)
		if p6Digest(frozen) != row.InputHash {
			return nil, nil, "", 0, nil, errors.New("重置冻结输入损坏")
		}
	}
	if len(state.Input.Prices) != 4 || state.Input.Source == "" || (state.Input.Scope != constUtil.CopyResetPlayerDay && state.Input.Scope != constUtil.CopyResetMissionDay) {
		return nil, nil, "", 0, nil, errors.New("重置原价格或范围存档无效")
	}
	for _, price := range state.Input.Prices {
		if price < 1 {
			return nil, nil, "", 0, nil, errors.New("重置原价格无效")
		}
	}
	if state.Used == nil {
		state.Used = map[string]int32{}
	}
	for _, used := range state.Used {
		if used < 0 || used > 4 {
			return nil, nil, "", 0, nil, errors.New("重置使用次数无效")
		}
	}
	vip := s.table("VipPerks", "vip", strconv.Itoa(int(p.GetVip())))
	limit := int32(tableNumber(vip, "numPerk1"))
	if vip == nil || limit < 1 || limit > 4 {
		return nil, nil, "", 0, nil, errors.New("原VIP重置额度未恢复")
	}
	key := "global"
	if state.Input.Scope == constUtil.CopyResetMissionDay {
		key = chapter + "/" + mission
	}
	used := state.Used[key]
	cost := &pb.BaseResource{Id: proto.String("diamond"), Num: proto.Int32(state.Input.Prices[min(int(used), len(state.Input.Prices)-1)])}
	return row, state, key, max(0, limit-used), cost, nil
}
func saveCopyResetDaily(tx *gorm.DB, row *db.CopyResetDaily, state *copyResetProgress) error {
	raw, e := json.Marshal(state)
	if e != nil {
		return e
	}
	row.Payload = raw
	return tx.Save(row).Error
}
func (s *GameService) copyResetMission(p *pb.Player, g *model.GameState, chapter, mission string) (*pb.CopySimpleInfo, error) {
	cfg := s.table("CopyMission", "MissionId", mission)
	if cfg == nil || cfg["ChapterId"] != chapter || !s.copyMissionOpen(g, cfg) {
		return nil, errors.New("关卡或前置条件未开放")
	}
	chapterRow := s.table("CopyChapter", "ChapterId", chapter)
	if chapterRow == nil || int32(tableNumber(chapterRow, "LevelLimit")) > p.GetLevel() {
		return nil, errors.New("角色等级不足")
	}
	for _, info := range g.Copy.GetPassedCopyInfo() {
		if info.GetChapterId() == chapter && info.GetMissionId() == mission {
			return info, nil
		}
	}
	return nil, errors.New("尚无本关已完成挑战")
}

// copyGetResetInfo always supplies the original cost[0], including quota zero.
func (s *GameService) copyGetResetInfo(ctx context.Context, id int64, b *pb.CSCopyGetResetInfo) ([]proto.Message, error) {
	out := &pb.SCCopyGetResetInfo{}
	e := s.p3Profile(ctx, id, func(tx *gorm.DB, row *db.Profile, p *pb.Player, g *model.GameState) error {
		s.refreshCopyProgress(g)
		if err := s.refreshCopyCalendar(ctx, tx, id, g, time.Now()); err != nil {
			return err
		}
		if _, e := s.copyResetMission(p, g, b.GetChapterId(), b.GetMissionId()); e != nil {
			return e
		}
		daily, state, _, remain, cost, e := s.copyResetQuote(tx, id, p, b.GetChapterId(), b.GetMissionId(), LocalPlayNow(ctx))
		if e != nil {
			return e
		}
		out.RemainResetCount = proto.Int32(remain)
		out.Cost = []*pb.BaseResource{cost}
		if e = saveCopyResetDaily(tx, daily, state); e != nil {
			return e
		}
		return saveProfile(tx, row, p, g)
	})
	return []proto.Message{out}, e
}

// copyResetChallenge clears only the exhausted target, under the original player lock.
func (s *GameService) copyResetChallenge(ctx context.Context, id int64, b *pb.CSCopyResetChallengeCount, stamp int64) ([]proto.Message, error) {
	out := &pb.SCCopyResetChallengeCount{}
	return s.p5Mutation(ctx, id, constUtil.CopyResetOperation, stamp, b, out, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		s.refreshCopyProgress(g)
		if err := s.refreshCopyCalendar(ctx, tx, id, g, time.Now()); err != nil {
			return err
		}
		info, e := s.copyResetMission(p, g, b.GetChapterId(), b.GetMissionId())
		if e != nil {
			return e
		}
		limit := int32(tableNumber(s.table("CopyMission", "MissionId", b.GetMissionId()), "ChallengeCount"))
		if limit < 1 || info.GetChallengeTimes() < limit {
			return errors.New("本关挑战次数尚未耗尽")
		}
		if g.Run != nil && !g.Run.Complete {
			return errors.New("请先离开正在进行的副本")
		}
		if e = s.ensureNoActiveDailyCopy(tx, id, time.Now()); e != nil {
			return e
		}
		daily, state, key, remain, cost, e := s.copyResetQuote(tx, id, p, b.GetChapterId(), b.GetMissionId(), LocalPlayNow(ctx))
		if e != nil {
			return e
		}
		if remain < 1 {
			return errors.New("今日VIP关卡重置次数已用完")
		}
		if b.GetCost().GetId() != cost.GetId() || b.GetCost().GetNum() != cost.GetNum() || b.GetCost().GetGuid() != "" {
			return errors.New("重置报价已变化，请重新查询")
		}
		if e = deductPoolCost(p, g, cost); e != nil {
			return e
		}
		info.ChallengeTimes = proto.Int32(0)
		state.Used[key]++
		if e = saveCopyResetDaily(tx, daily, state); e != nil {
			return e
		}
		if e = s.RecordP1Cost(tx, id, fmt.Sprintf("p5:copy-reset:%d", stamp), []*pb.BaseResource{cost}, LocalPlayNow(ctx)); e != nil {
			return e
		}
		out.CurrentSimpleInfo = proto.Clone(info).(*pb.CopySimpleInfo)
		return nil
	})
}
