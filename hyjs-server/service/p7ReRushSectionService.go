package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

type p7ReRushWin struct {
	Floor int32  `json:"floor"`
	Star  int32  `json:"star"`
	Fight string `json:"fight"`
	Hash  string `json:"hash"`
}

// p7ReRushBonusProjection 将Re内部比例适配回原UI.Refresh读取的per字段。
// 原界面读取perhp/peratt/perpdef/permdef；战斗展开继续使用内部比例。
func p7ReRushBonusProjection(bonus *pb.BaseProp) *pb.BaseProp {
	return &pb.BaseProp{Perhp: proto.Float64(bonus.GetHp()), Peratt: proto.Float64(bonus.GetAtt()), Perpdef: proto.Float64(bonus.GetPdef()), Permdef: proto.Float64(bonus.GetMdef())}
}

// p7ReRushAcceptedWins 只接受实际结束且与原引擎权威记录一致的胜利。
// runID为0用于历史扫荡资格，当前轮分段严格匹配轮次；查询不生成通关状态。
func p7ReRushAcceptedWins(tx *gorm.DB, id, runID int64, from, to int32, sourceFight ...string) (map[int32]p7ReRushWin, error) {
	var tasks []string
	for floor := from; floor <= to; floor++ {
		tasks = append(tasks, fmt.Sprintf("local_rush_%d", floor))
	}
	query := tx.Where("player_id=? AND kind=? AND status=? AND diagnostic=? AND task_id IN ?", id, constUtil.P7RushTrial, constUtil.P7RushSettled, false, tasks)
	if runID != 0 {
		query = query.Where("run_id=?", runID)
	}
	if len(sourceFight) == 1 {
		query = query.Where("id=?", sourceFight[0])
	}
	var fights []db.P7RushFight
	if err := query.Find(&fights).Error; err != nil {
		return nil, err
	}
	wins := map[int32]p7ReRushWin{}
	for _, fight := range fights {
		var stage db.P7RushStageRule
		if json.Unmarshal(fight.Rule, &stage) != nil || p6Digest(fight.Rule) != fight.RuleHash || p6Digest(fight.Start) != fight.StartHash || stage.Source != constUtil.P7ReRushSource {
			return nil, errors.New("Re试炼历史战斗来源损坏")
		}
		end := &pb.CSRushFightEnd{}
		result := &pb.SCRushFightEnd{}
		if proto.Unmarshal(fight.EndRequest, end) != nil || proto.Unmarshal(fight.Result, result) != nil || end.GetFightId() != fight.ID || end.GetResult() != result.GetResult() || stage.Floor < from || stage.Floor > to {
			return nil, errors.New("Re试炼历史结算回执损坏")
		}
		if result.GetResult() != 1 {
			continue
		}
		outcome, scoped, err := p7ReadScopedRush(tx, &fight, end)
		if err != nil {
			return nil, err
		}
		if !scoped {
			var record db.P6BattleRecord
			if err := tx.First(&record, "id=?", fight.ReplayID).Error; err != nil {
				return nil, err
			}
			outcome = &model.CombatOutcome{}
			if record.PlayerID != id || record.SourceFight != fight.ID || record.Source != constUtil.P7RushReplaySource || !record.Authoritative || p6Digest(record.Outcome) != record.OutcomeHash || json.Unmarshal(record.Outcome, outcome) != nil {
				return nil, errors.New("Re试炼历史权威记录损坏")
			}
		}
		if outcome == nil || !outcome.Authoritative || outcome.Winner != 0 || stage.Star < 1 || stage.Star > 3 || result.GetStar() != stage.Star {
			return nil, errors.New("Re试炼历史胜利与实际计算冲突")
		}
		if wins[stage.Floor].Star < stage.Star {
			wins[stage.Floor] = p7ReRushWin{Floor: stage.Floor, Star: stage.Star, Fight: fight.ID, Hash: p6Digest(append(append([]byte(fight.StartHash+fight.RuleHash), fight.Result...), fight.EndRequest...))}
		}
	}
	return wins, nil
}

// p7ReRushRunWins 合并同轮实际挑战和逐条绑定历史权威胜利的合法扫荡。
func p7ReRushRunWins(tx *gorm.DB, state *db.P7RushState, from, to int32) (map[int32]p7ReRushWin, error) {
	wins, err := p7ReRushAcceptedWins(tx, state.PlayerID, state.RunID, from, to)
	if err != nil {
		return nil, err
	}
	var sweeps []db.P7ReRushSweep
	if err = tx.Where("player_id=? AND run_id=? AND floor BETWEEN ? AND ?", state.PlayerID, state.RunID, from, to).Find(&sweeps).Error; err != nil {
		return nil, err
	}
	for _, sweep := range sweeps {
		history, err := p7ReRushAcceptedWins(tx, state.PlayerID, 0, sweep.Floor, sweep.Floor, sweep.SourceFight)
		if err != nil {
			return nil, err
		}
		win := history[sweep.Floor]
		if win.Star != sweep.Star || win.Fight != sweep.SourceFight || win.Hash != sweep.SourceHash || wins[sweep.Floor].Star != 0 {
			return nil, errors.New("Re试炼扫荡资格来源冲突")
		}
		wins[sweep.Floor] = win
	}
	return wins, nil
}

// p7ReRushSectionProof 以三层当前轮的实际挑战或合法历史扫荡重算星数。
func p7ReRushSectionProof(tx *gorm.DB, state *db.P7RushState, section int32) (int32, string, bool, error) {
	from, to := section*3-2, section*3
	wins, err := p7ReRushRunWins(tx, state, from, to)
	if err != nil {
		return 0, "", false, err
	}
	var proof []p7ReRushWin
	stars := int32(0)
	for floor := from; floor <= to; floor++ {
		win, found := wins[floor]
		if !found {
			continue
		}
		stars += win.Star
		proof = append(proof, win)
	}
	raw, err := json.Marshal(proof)
	return stars, p6Digest(raw), len(proof) == 3 && state.Floor > to, err
}

func p7ReRushSectionRow(tx *gorm.DB, state *db.P7RushState, section int32) (*db.P7ReRushSection, error) {
	row := &db.P7ReRushSection{PlayerID: state.PlayerID, RunID: state.RunID, Section: section}
	err := tx.First(row, "player_id=? AND run_id=? AND section=?", state.PlayerID, state.RunID, section).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return row, nil
	}
	return row, err
}

func p7ReRushBonus(tx *gorm.DB, state *db.P7RushState) (*pb.BaseProp, error) {
	var sections []db.P7ReRushSection
	if err := tx.Where("player_id=? AND run_id=? AND selected<>?", state.PlayerID, state.RunID, "").Find(&sections).Error; err != nil {
		return nil, err
	}
	prop := &pb.BaseProp{Hp: proto.Float64(0), Att: proto.Float64(0), Pdef: proto.Float64(0), Mdef: proto.Float64(0)}
	// 已购旧首组加成继续归属于本轮；校验冻结三场，不回填或补发历史权益。
	legacy, policy, err := p7ReadRushAdditionOffer(tx, state)
	if err != nil {
		return nil, err
	}
	if legacy != nil && legacy.Selected != "" {
		stars, complete, hash, err := p7RushAdditionProgress(tx, legacy, policy)
		if err != nil || !complete || stars < legacy.SpentStars || hash != legacy.AcceptanceHash {
			return nil, errors.New("旧试炼加成冻结来源冲突")
		}
		key, rate, err := p7RushAdditionOption(legacy.Selected)
		if err != nil {
			return nil, err
		}
		switch key {
		case "perhp":
			prop.Hp = proto.Float64(rate)
		case "peratt":
			prop.Att = proto.Float64(rate)
		case "perpdef":
			prop.Pdef = proto.Float64(rate)
		case "permdef":
			prop.Mdef = proto.Float64(rate)
		}
	}
	for _, section := range sections {
		stars, hash, complete, err := p7ReRushSectionProof(tx, state, section.Section)
		if err != nil || !complete || !section.Claimed || section.AcceptanceHash != hash || section.SpentStars > stars {
			return nil, errors.New("Re试炼已选加成缺少真实分段来源")
		}
		switch section.Selected {
		case "perhp,0.03":
			if section.SpentStars != 3 {
				return nil, errors.New("Re试炼生命加成星数冲突")
			}
			prop.Hp = proto.Float64(prop.GetHp() + .03)
		case "peratt,0.06":
			if section.SpentStars != 6 {
				return nil, errors.New("Re试炼攻击加成星数冲突")
			}
			prop.Att = proto.Float64(prop.GetAtt() + .06)
		case "perpdef,0.09":
			if section.SpentStars != 9 {
				return nil, errors.New("Re试炼防御加成星数冲突")
			}
			prop.Pdef = proto.Float64(prop.GetPdef() + .09)
			prop.Mdef = proto.Float64(prop.GetMdef() + .09)
		default:
			return nil, errors.New("Re试炼加成选项损坏")
		}
	}
	return prop, nil
}

func (s *GameService) p7ApplyReRushBonus(tx *gorm.DB, state *db.P7RushState, party *pb.BattlePlayer) (*pb.BattlePlayer, error) {
	bonus, err := p7ReRushBonus(tx, state)
	if err != nil {
		return nil, err
	}
	result := proto.Clone(party).(*pb.BattlePlayer)
	for _, actor := range result.Formation {
		if actor.Prop == nil || actor.Inst == nil {
			return nil, errors.New("Re试炼加成角色缺少实例")
		}
		actor.Prop.Hp = proto.Float64(math.RoundToEven(actor.Prop.GetHp() * (1 + bonus.GetHp())))
		actor.Inst.CurHp = proto.Float64(actor.Prop.GetHp())
		actor.Prop.Att = proto.Float64(math.RoundToEven(actor.Prop.GetAtt() * (1 + bonus.GetAtt())))
		actor.Prop.Pdef = proto.Float64(math.RoundToEven(actor.Prop.GetPdef() * (1 + bonus.GetPdef())))
		actor.Prop.Mdef = proto.Float64(math.RoundToEven(actor.Prop.GetMdef() * (1 + bonus.GetMdef())))
	}
	return result, nil
}

// p7ReRushSectionFlags 保留每三层先领奖、再选择加成的原顺序。
func p7ReRushSectionFlags(tx *gorm.DB, state *db.P7RushState) (int32, int32, error) {
	section := (state.Floor - 1) / 3
	if section < 1 {
		return 0, 0, nil
	}
	_, hash, complete, err := p7ReRushSectionProof(tx, state, section)
	if err != nil || !complete {
		return 0, 0, err
	}
	row, err := p7ReRushSectionRow(tx, state, section)
	if err != nil {
		return 0, 0, err
	}
	if row.AcceptanceHash != "" && row.AcceptanceHash != hash {
		return 0, 0, errors.New("Re试炼分段领奖来源冲突")
	}
	reward, addition := int32(0), int32(0)
	if !row.Claimed {
		reward = 1
	}
	if row.Selected == "" {
		addition = 1
	}
	return reward, addition, nil
}
