package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// p13UFMissionDefinition is an explicitly published local task catalog. Its kinds
// identify real natural producers; the APK does not contain the private catalog.
type p13UFMissionDefinition struct {
	Kind   int32 `json:"kind"`
	Target int64 `json:"target"`
	Score  int32 `json:"score"`
}

func p13UFMissionCatalog(rule *db.P13UnionFightSettlementPolicy) ([]p13UFMissionDefinition, error) {
	if rule.PersonalMissions == "" {
		if rule.TaskRewards != "" {
			return nil, errors.New("公会战任务奖励缺提前发布的任务目录")
		}
		return nil, nil
	}
	var defs []p13UFMissionDefinition
	if json.Unmarshal([]byte(rule.PersonalMissions), &defs) != nil || len(defs) != 2 {
		return nil, errors.New("本地公会战任务目录须包含真实采矿与运矿两项")
	}
	seen := map[int32]bool{}
	var total int64
	for _, def := range defs {
		if (def.Kind != constUtil.P13UnionFightMissionGather && def.Kind != constUtil.P13UnionFightMissionTransport) || seen[def.Kind] || def.Target <= 0 || def.Target > math.MaxInt32 || def.Score <= 0 {
			return nil, errors.New("公会战本地自然任务类型/阈值/分值无效")
		}
		seen[def.Kind] = true
		total += int64(def.Score)
	}
	if total > math.MaxInt32 {
		return nil, errors.New("公会战本地任务总分不能精确表示")
	}
	return defs, nil
}

// p13UFMissionState derives progress from committed full-lifecycle sources rather
// than spendable inventory, tokens, camp balances or client supplied counts.
func p13UFMissionState(tx *gorm.DB, match *db.P13UnionFightMatch, actor *db.P13UnionFightFieldPlayer, rule *db.P13UnionFightSettlementPolicy) ([]*pb.InstUFPlayerMission, int32, int, error) {
	defs, err := p13UFMissionCatalog(rule)
	if err != nil || len(defs) == 0 {
		return nil, 0, 0, err
	}
	var scope db.P13UnionFightTransportScope
	if err = tx.First(&scope, "match_id=?", match.ID).Error; err != nil {
		return nil, 0, 0, err
	}
	var field db.P13UnionFightField
	if err = tx.First(&field, "match_id=?", match.ID).Error; err != nil {
		return nil, 0, 0, err
	}
	if scope.Source != constUtil.P13UnionFightTransportSource || scope.StartedAt != field.CreatedAt || scope.StartedAt <= 0 {
		return nil, 0, 0, errors.New("任务缺本场完整自然资源来源")
	}
	var tasks []db.P13UnionFightResourceTask
	if err = tx.Where("match_id=? AND player_id=? AND status=?", match.ID, actor.PlayerID, constUtil.P13UnionFightResourceDone).Find(&tasks).Error; err != nil {
		return nil, 0, 0, err
	}
	var gathered int64
	for _, task := range tasks {
		var detail pb.SCUFPlayerCollectEndDetail
		if task.StartedAt < scope.StartedAt || task.StartedAt < match.StartAt || task.DueAt > match.EndAt || task.FinishedAt < task.DueAt || task.Granted <= 0 || task.Source == "" || p6Digest(task.Result) != task.ResultHash || proto.Unmarshal(task.Result, &detail) != nil || int64(detail.GetAddResourceNum()) != task.Granted || detail.GetPlayer().GetGuid() != actor.PlayerID || gathered > math.MaxInt32-task.Granted {
			return nil, 0, 0, errors.New("任务实际采集记录来源无效")
		}
		gathered += task.Granted
	}
	var stat db.P13UnionFightTransportStat
	var transported int64
	err = tx.First(&stat, "match_id=? AND player_id=?", match.ID, actor.PlayerID).Error
	if err == nil {
		if stat.GuildID != actor.GuildID || stat.Ore <= 0 || stat.Ore > math.MaxInt32 || stat.Deposits <= 0 || stat.UpdatedAt < scope.StartedAt || stat.UpdatedAt >= match.EndAt {
			return nil, 0, 0, errors.New("任务实际运矿来源无效")
		}
		transported = stat.Ore
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, 0, 0, err
	}
	frames := []*pb.InstUFPlayerMission{}
	var score int32
	complete := 0
	for index, def := range defs {
		progress := gathered
		desc := "union_fight_mine"
		if def.Kind == constUtil.P13UnionFightMissionTransport {
			progress = transported
			desc = "union_fight_trans_resourse"
		}
		done := int32(0)
		if progress >= def.Target {
			done = 1
			complete++
			score += def.Score
		}
		frames = append(frames, &pb.InstUFPlayerMission{PlayerGuid: proto.Int64(actor.PlayerID), MissionType: proto.Int32(def.Kind), MissionIndex: proto.Int32(int32(index)), Progress: proto.String(fmt.Sprintf("%d/%d", min(progress, def.Target), def.Target)), IsComplete: proto.Int32(done), RewardNum: proto.Int32(def.Score), MissionDesc: proto.String(desc), MissionIcon: proto.String("")})
	}
	return frames, score, complete, nil
}

func p13UFMissionRules(tx *gorm.DB, match *db.P13UnionFightMatch) (*db.P13UnionFightSettlementPolicy, error) {
	var offer db.P13UnionFightSettlementOffer
	err := tx.First(&offer, "match_id=?", match.ID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return p13UFSettlementRules(&offer, match)
}

// p13UFMissionSync changes only the exact difference of verified natural mission
// points. Other battle points remain visible and continue to exclude zero-fight.
func p13UFMissionSync(tx *gorm.DB, match *db.P13UnionFightMatch, actor *db.P13UnionFightFieldPlayer, frame *pb.InstUFPlayer, previous int32) (*pb.SCUFMissionPush, error) {
	rule, err := p13UFMissionRules(tx, match)
	if err != nil || rule == nil || rule.PersonalMissions == "" {
		return nil, err
	}
	_, score, _, err := p13UFMissionState(tx, match, actor, rule)
	if err != nil {
		return nil, err
	}
	if score < previous {
		return nil, errors.New("公会战实际任务进度发生逆转")
	}
	if score == previous {
		return nil, nil
	}
	value := frame.GetOwnScore() + float64(score-previous)
	if math.IsNaN(value) || math.IsInf(value, 0) || value > math.MaxInt32 {
		return nil, errors.New("公会战实际任务积分溢出")
	}
	frame.OwnScore = proto.Float64(value)
	if err = p13UFFieldSaveActor(tx, actor, frame); err != nil {
		return nil, err
	}
	return &pb.SCUFMissionPush{PlayerFightScore: proto.Float64(value)}, nil
}

func p13UFMissionScore(tx *gorm.DB, match *db.P13UnionFightMatch, actor *db.P13UnionFightFieldPlayer) (int32, error) {
	rule, err := p13UFMissionRules(tx, match)
	if err != nil || rule == nil {
		return 0, err
	}
	_, score, _, err := p13UFMissionState(tx, match, actor, rule)
	return score, err
}

func (s *GameService) p13UFMissionQuery(ctx context.Context, id int64, request proto.Message, now time.Time) ([]proto.Message, error) {
	var messages []proto.Message
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		actor, match, _, _, err := s.p13UFFieldCurrent(tx, id, now)
		if err != nil {
			return err
		}
		// Return only prepublished guild tasks backed by actual same-transaction opening facts.
		if _, ok := request.(*pb.CSUFUnionMissionList); ok {
			infos, err := p13UFGuildMissionFrames(tx, match, actor)
			if err != nil {
				return err
			}
			messages = []proto.Message{&pb.SCUFUnionMissionList{MissionInfos: infos}}
			return nil
		}
		rule, err := p13UFMissionRules(tx, match)
		if err != nil {
			return err
		}
		var infos []*pb.InstUFPlayerMission
		if rule != nil {
			infos, _, _, err = p13UFMissionState(tx, match, actor, rule)
			if err != nil {
				return err
			}
		}
		messages = []proto.Message{&pb.SCUFPlayerMissionList{MissionInfos: infos}}
		return nil
	})
	return messages, err
}
