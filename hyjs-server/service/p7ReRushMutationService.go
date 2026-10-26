package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// p7ReRushMutation 复用原消息和回执原子性，仅消费已显式发布的 Re 来源。
func (s *GameService) p7ReRushMutation(ctx context.Context, id int64, request proto.Message, stamp int64) ([]proto.Message, bool, error) {
	var response proto.Message
	mutation := false
	switch request.(type) {
	case *pb.CSRushRewardPreview:
		response = &pb.SCRushRewardPreview{}
	case *pb.CSRushGetSectionReward:
		response = &pb.SCRushGetSectionReward{}
		mutation = true
	case *pb.CSRushGetSectionAddition:
		response = &pb.SCRushGetSectionAddition{}
	case *pb.CSRushAddition:
		response = &pb.SCRushAddition{}
		mutation = true
	case *pb.CSRushAdditionView:
		response = &pb.SCRushAdditionView{}
	case *pb.CSRush3Hit:
		response = &pb.SCRush3Hit{}
		mutation = true
	case *pb.CSRushToTop:
		response = &pb.SCRushToTop{}
		mutation = true
	case *pb.CSRushMazeTrader:
		response = &pb.SCRushMazeTrader{}
	case *pb.CSRushMazeTraderBuy:
		response = &pb.SCRushMazeTraderBuy{}
		mutation = true
	default:
		return nil, false, nil
	}
	enabled, err := p7ReRushEnabled(db.GetIns().WithContext(ctx))
	if err != nil || !enabled {
		return nil, enabled, err
	}
	legacyRequest := false
	err = updateProfileWithTx(ctx, id, func(tx *gorm.DB, player *pb.Player, game *model.GameState) error {
		domain := "re-rush:" + string(request.ProtoReflect().Descriptor().Name())
		if mutation {
			if found, err := p7ReadReceipt(tx, id, domain, stamp, request, response); found || err != nil {
				return err
			}
		}
		state, err := p7State(tx, id, LocalPlayNow(ctx))
		if err != nil {
			return err
		}
		switch request.(type) {
		case *pb.CSRushRewardPreview, *pb.CSRushGetSectionReward, *pb.CSRushGetSectionAddition, *pb.CSRushAddition, *pb.CSRushAdditionView, *pb.CSRush3Hit, *pb.CSRushToTop:
			tx, err = s.p7BindSettledQueryProjection(tx, state)
			if err != nil {
				return err
			}
			switch request.(type) {
			case *pb.CSRush3Hit, *pb.CSRushToTop:
				p7EliteProjectionFor(tx).refreshSweeps = true
			}
		}
		legacyRequest, err = s.p7ReRushLegacyRequest(tx, state, request, stamp)
		if err != nil || legacyRequest {
			return err
		}
		if player.GetLevel() < 22 || mutation && state.ActiveFight != "" {
			return errors.New("请先满足试炼条件或完成当前战斗")
		}
		if _, ok := request.(*pb.CSRushMazeTrader); ok {
			response, err = s.p7ReRushTrader(tx, player, game, state, false, stamp)
		} else if _, ok := request.(*pb.CSRushMazeTraderBuy); ok {
			response, err = s.p7ReRushTrader(tx, player, game, state, true, stamp)
		} else if _, ok := request.(*pb.CSRush3Hit); ok {
			response, err = s.p7ReRushSweep(tx, player, game, state, true)
		} else if _, ok := request.(*pb.CSRushToTop); ok {
			response, err = s.p7ReRushSweep(tx, player, game, state, false)
		} else {
			response, err = s.p7ReRushSectionMutation(tx, player, game, state, request)
		}
		if err != nil {
			return err
		}
		if !mutation {
			return nil
		}
		if err = saveP7RushCalendar(tx, state, LocalPlayNow(ctx)); err != nil {
			return err
		}
		return p7SaveReceipt(tx, id, domain, stamp, request, response)
	})
	if err != nil {
		return nil, true, err
	}
	if legacyRequest {
		return nil, false, nil
	}
	return []proto.Message{response}, true, nil
}

func (s *GameService) p7ReRushSectionMutation(tx *gorm.DB, player *pb.Player, game *model.GameState, state *db.P7RushState, request proto.Message) (proto.Message, error) {
	bonus, err := p7ReRushBonus(tx, state)
	if err != nil {
		return nil, err
	}
	if _, ok := request.(*pb.CSRushAdditionView); ok {
		return &pb.SCRushAdditionView{Prop: p7ReRushBonusProjection(bonus)}, nil
	}
	section := max(1, (state.Floor-1)/3)
	switch request.(type) {
	case *pb.CSRushGetSectionAddition, *pb.CSRushAddition:
		if _, selecting := request.(*pb.CSRushAddition); selecting && state.AdditionOfferSection != 0 {
			section = state.AdditionOfferSection
		} else {
			pending, err := p7ReRushPendingAddition(tx, state)
			if err != nil {
				return nil, err
			}
			if pending == nil {
				return nil, errors.New("没有已领奖且待选择加成的试炼段")
			}
			section = pending.Section
		}
	}
	stars, hash, complete, err := p7ReRushSectionProof(tx, state, section)
	if err != nil {
		return nil, err
	}
	row, err := p7ReRushSectionRow(tx, state, section)
	if err != nil {
		return nil, err
	}
	if row.AcceptanceHash != "" && row.AcceptanceHash != hash {
		return nil, errors.New("Re试炼分段来源冲突")
	}
	switch req := request.(type) {
	case *pb.CSRushRewardPreview:
		if row.Claimed && row.Selected != "" {
			section++
			stars, _, _, err = p7ReRushSectionProof(tx, state, section)
			if err != nil {
				return nil, err
			}
		}
		three, err := p1RewardResources("point,300;item_005,6")
		if err != nil {
			return nil, err
		}
		six, err := p1RewardResources("point,600;item_005,12")
		if err != nil {
			return nil, err
		}
		nine, err := p1RewardResources("point,900;item_005,18")
		if err != nil {
			return nil, err
		}
		return &pb.SCRushRewardPreview{SectionStar: proto.Int32(stars), SectionFrom: proto.Int32(section*3 - 2), SectionTo: proto.Int32(section * 3), ThreeStarReward: three, SixStarReward: six, NineStarReward: nine}, nil
	case *pb.CSRushGetSectionAddition:
		if !complete || !row.Claimed || row.Selected != "" {
			return nil, errors.New("本段试炼未完成或加成已选择")
		}
		if err = p7ReRushSaveAdditionOffer(tx, state, section); err != nil {
			return nil, err
		}
		return &pb.SCRushGetSectionAddition{StarResult3: proto.String("perhp,0.03"), StarResult6: proto.String("peratt,0.06"), StarResult9: proto.String("perpdef,0.09"), CostStar: proto.Int32(stars), Prop: p7ReRushBonusProjection(bonus)}, nil
	case *pb.CSRushGetSectionReward:
		if !complete || row.Claimed {
			return nil, errors.New("本段试炼未完成或奖励已领取")
		}
		gain, err := s.GrantFixedReward(player, game, fmt.Sprintf("point,%d;item_005,%d", stars*100, stars*2))
		if err != nil {
			return nil, err
		}
		row.Claimed, row.AcceptanceHash = true, hash
		if err = tx.Save(row).Error; err != nil {
			return nil, err
		}
		return &pb.SCRushGetSectionReward{Gain: gain}, nil
	case *pb.CSRushAddition:
		if !complete || !row.Claimed || row.Selected != "" || req.GetStar() > stars {
			return nil, errors.New("请先领取本段奖励，且不能重复选择加成")
		}
		options := map[int32]string{3: "perhp,0.03", 6: "peratt,0.06", 9: "perpdef,0.09"}
		if options[req.GetStar()] == "" || options[req.GetStar()] != req.GetSelect() {
			return nil, errors.New("Re试炼加成选项与星数不匹配")
		}
		if err = p7ReRushSaveAdditionOffer(tx, state, section); err != nil {
			return nil, err
		}
		row.Selected, row.SpentStars = req.GetSelect(), req.GetStar()
		if err = tx.Save(row).Error; err != nil {
			return nil, err
		}
		bonus, err = p7ReRushBonus(tx, state)
		return &pb.SCRushAddition{Prop: p7ReRushBonusProjection(bonus)}, err
	}
	return nil, errors.New("Re试炼区段消息不匹配")
}

// p7ReRushSweep 复用历史真实最高难度资格；三连要求三星，至最高每次最多三层。
// Re 的扫荡属于已经通关内容的领取入口，不调用固定胜者战斗或制造新的原引擎记录。
func (s *GameService) p7ReRushSweep(tx *gorm.DB, player *pb.Player, game *model.GameState, state *db.P7RushState, three bool) (proto.Message, error) {
	legacy, err := s.p7ReRushLegacyFirstSection(tx, state)
	if err != nil {
		return nil, err
	}
	if legacy {
		return nil, errors.New("请先完成本轮已冻结的原试炼首组")
	}
	failed, err := p7TrialMustReset(tx, state)
	if err != nil {
		return nil, err
	}
	reward, _, err := p7ReRushSectionFlags(tx, state)
	if err != nil {
		return nil, err
	}
	if failed || state.Floor > constUtil.P7ReRushLastFloor || reward != 0 || state.ActiveFight != "" {
		return nil, errors.New("请先重置已结束的试炼、完成当前战斗或领取本段奖励")
	}
	wins, err := p7ReRushAcceptedWins(tx, state.PlayerID, 0, state.Floor, min(state.Floor+2, constUtil.P7ReRushLastFloor))
	if err != nil {
		return nil, err
	}
	result3, resultTop := &pb.SCRush3Hit{}, &pb.SCRushToTop{}
	for i := 0; i < 3 && state.Floor <= constUtil.P7ReRushLastFloor; i++ {
		floor := state.Floor
		win := wins[floor]
		if win.Star < 1 || three && win.Star != 3 {
			break
		}
		gain, err := s.GrantFixedReward(player, game, fmt.Sprintf("coin,%d;point,%d", (2000+floor*500)*win.Star, (50+floor*10)*win.Star))
		if err != nil {
			return nil, err
		}
		if err = tx.Create(&db.P7ReRushSweep{PlayerID: state.PlayerID, RunID: state.RunID, Floor: floor, Star: win.Star, SourceFight: win.Fight, SourceHash: win.Hash}).Error; err != nil {
			return nil, err
		}
		state.Floor++
		state.TotalStars += win.Star
		state.RecordStars = max(state.RecordStars, state.TotalStars)
		// 扫荡仍计入本轮真实获得的星数，事件来源独立于新的战斗结算。
		source := fmt.Sprintf("re-rush-sweep:%d:%d", state.RunID, floor)
		if err = s.RecordP1Event(tx, state.PlayerID, source, constUtil.P7RushStarEvent, int64(state.TotalStars), LocalPlayNow(tx.Statement.Context)); err != nil {
			return nil, err
		}
		if err = s.RecordP1Event(tx, state.PlayerID, source, constUtil.P7RushEarnedStarEvent, int64(win.Star), LocalPlayNow(tx.Statement.Context)); err != nil {
			return nil, err
		}
		result3.Floor = append(result3.Floor, floor)
		result3.IsWin = append(result3.IsWin, 1)
		result3.Gain = append(result3.Gain, gain)
		info := &pb.RushSweepInfo{Floor: proto.Int32(floor), Gain: gain}
		if floor%3 == 0 {
			sectionGain, err := s.p7ReRushSectionMutation(tx, player, game, state, &pb.CSRushGetSectionReward{})
			if err != nil {
				return nil, err
			}
			info.IsSectionEnd, info.SectionGain, info.SectionProp = proto.Int32(1), sectionGain.(*pb.SCRushGetSectionReward).Gain, proto.String("")
			// 原三连回调只导入 gain；至最高回调另外导入 sectionGain。
			// 三连将同次真实到账的分段资源并入末层展示，避免客户端库存漏刷新。
			if three {
				merged := proto.Clone(gain).(*pb.ResultObject)
				redMergeGain(merged, info.SectionGain)
				result3.Gain[len(result3.Gain)-1] = merged
			}
		}
		resultTop.SweepInfo = append(resultTop.SweepInfo, info)
	}
	result3.IsValid = proto.Int32(0)
	if len(result3.Floor) > 0 {
		result3.IsValid = proto.Int32(1)
	}
	if three {
		return result3, nil
	}
	return resultTop, nil
}

func p7ReRushTraderValid(tx *gorm.DB, state *db.P7RushState) (bool, error) {
	failed := false
	var fights []db.P7RushFight
	err := tx.Where("player_id=? AND run_id=? AND task_id=? AND kind=? AND status=? AND diagnostic=?", state.PlayerID, state.RunID, fmt.Sprintf("local_rush_%d", state.Floor), constUtil.P7RushTrial, constUtil.P7RushSettled, false).Find(&fights).Error
	if err != nil {
		return false, err
	}
	for i := range fights {
		var stage db.P7RushStageRule
		if p6Digest(fights[i].Rule) != fights[i].RuleHash || json.Unmarshal(fights[i].Rule, &stage) != nil || stage.Source != constUtil.P7ReRushSource {
			return false, errors.New("Re迷宫商人失败来源损坏")
		}
		accepted, err := p7AcceptedTrialFailure(tx, &fights[i])
		if err != nil {
			return false, err
		}
		failed = failed || accepted
	}
	// Re把102层完成也记为stopped；商人仍需最后一个真实完整区段见证。
	if err == nil && !failed && state.Floor > constUtil.P7ReRushLastFloor {
		_, _, failed, err = p7ReRushSectionProof(tx, state, constUtil.P7ReRushLastFloor/3)
	}
	if err != nil || !failed {
		return false, err
	}
	var run db.P7ReRushRun
	err = tx.First(&run, "player_id=? AND run_id=?", state.PlayerID, state.RunID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return true, nil
	}
	return !run.TraderBought, err
}

func (s *GameService) p7ReRushTrader(tx *gorm.DB, player *pb.Player, game *model.GameState, state *db.P7RushState, buy bool, stamp int64) (proto.Message, error) {
	valid, err := p7ReRushTraderValid(tx, state)
	if err != nil {
		return nil, err
	}
	if !valid {
		return nil, errors.New("本轮商人尚未出现或已经购买")
	}
	if !buy {
		return &pb.SCRushMazeTrader{Result: &pb.BaseResource{Id: proto.String("item_005"), Num: proto.Int32(10)}, OriginalPrice: proto.Int32(50), DiscountPrice: proto.Int32(20)}, nil
	}
	cost := []*pb.BaseResource{{Id: proto.String("diamond"), Num: proto.Int32(20)}}
	if err = deductPoolCost(player, game, cost[0]); err != nil {
		return nil, err
	}
	if err = s.RecordP1Cost(tx, state.PlayerID, fmt.Sprintf("re-rush-trader:%d", stamp), cost, LocalPlayNow(tx.Statement.Context)); err != nil {
		return nil, err
	}
	gain, err := s.GrantFixedReward(player, game, "item_005,10")
	if err != nil {
		return nil, err
	}
	if err = tx.Save(&db.P7ReRushRun{PlayerID: state.PlayerID, RunID: state.RunID, TraderBought: true}).Error; err != nil {
		return nil, err
	}
	return &pb.SCRushMazeTraderBuy{Gain: gain, Cost: cost}, nil
}

// p7ReRushCompletedInfo 在第102层后仍保留原领奖、加成和重置入口。
func (s *GameService) p7ReRushCompletedInfo(tx *gorm.DB, p *pb.Player, state *db.P7RushState, policy db.P7RushPolicy, now time.Time) (*pb.SCGetRushInfo, error) {
	free, err := s.p7VIP(p, "numPerk2")
	if err != nil {
		return nil, err
	}
	paid, err := s.p7VIP(p, "numPerk23")
	if err != nil {
		return nil, err
	}
	reward, addition, err := p7ReRushSectionFlags(tx, state)
	if err != nil {
		return nil, err
	}
	valid, err := p7ReRushTraderValid(tx, state)
	if err != nil {
		return nil, err
	}
	maze := int32(0)
	if valid {
		maze = 1
	}
	return &pb.SCGetRushInfo{CurFloor: proto.Int32(min(state.Floor, constUtil.P7ReRushLastFloor)), CurTotalStar: proto.Int32(state.TotalStars), HighRecordFloor: proto.Int32(state.HighFloor), RecordTotalStar: proto.Int32(state.RecordStars), ResetLeftTime: proto.Int32(max(0, free-state.FreeResetUsed)), DiamondRestLeftTime: proto.Int32(max(0, paid-state.PaidResetUsed)), DiamondRestCostNum: proto.Int32(policy.TrialResetCost), CurFloorSceneName: proto.String(p7ReRushNavigation(state.Floor)), IsMustReset: proto.Int32(1), IsRewardNeedGet: proto.Int32(reward), IsAdditionNeedGet: proto.Int32(addition), IsMazeTraderValid: proto.Int32(maze), WinKey: proto.Int32(0), WinValue: proto.Int32(0)}, nil
}
