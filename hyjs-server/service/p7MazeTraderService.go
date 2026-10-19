package service

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
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

// InitializeP7MazeTrader creates only optional operational tables; publication stays disabled.
func (s *GameService) InitializeP7MazeTrader(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.P7MazeTraderPolicy{}, &db.P7MazeTraderOffer{}); err != nil {
		return err
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.P7MazeTraderPolicy{
		ID: constUtil.P7MazeTraderPolicyID, Version: "local-maze-trader-v1", Source: constUtil.P7MazeTraderSource,
		MinFloor: 1, AppearPPM: 1000000, ResourceID: "item_001", Quantity: 2, OriginalPrice: 20, DiscountPrice: 10,
	}).Error
}

func (s *GameService) p7ValidateMazePolicy(p *db.P7MazeTraderPolicy) error {
	if p.ID != constUtil.P7MazeTraderPolicyID || len(p.Version) < 1 || len(p.Version) > 64 || p.Source != constUtil.P7MazeTraderSource || p.PublishedAt <= 0 || p.PublishedAt > p.StartAt || p.StartAt >= p.EndAt || p.MinFloor < 1 || p.AppearPPM < 0 || p.AppearPPM > 1000000 || !strings.HasPrefix(p.ResourceID, "item_") || p.Quantity < 1 || p.Quantity > 1000 || p.DiscountPrice < 1 || p.OriginalPrice < p.DiscountPrice {
		return errors.New("迷宫商人预发布目录无效")
	}
	// The original response has one simple item and two diamond price labels.
	if ok, err := s.openServerRewardSupported(fmt.Sprintf("%s,%d", p.ResourceID, p.Quantity)); err != nil || !ok {
		return errors.New("迷宫商人商品尚未恢复用途")
	}
	return nil
}

// p7AcceptedTrialFailure verifies the original accepted End and immutable P6 witness.
// SQL-only result flags, computed-but-unaccepted inputs and diagnostic fights grant nothing.
func p7AcceptedTrialFailure(tx *gorm.DB, fight *db.P7RushFight) (bool, error) {
	if fight.Kind != constUtil.P7RushTrial || fight.Diagnostic || fight.Status != constUtil.P7RushSettled {
		return false, nil
	}
	end := &pb.CSRushFightEnd{}
	result := &pb.SCRushFightEnd{}
	if proto.Unmarshal(fight.EndRequest, end) != nil || proto.Unmarshal(fight.Result, result) != nil || end.GetFightId() != fight.ID {
		return false, errors.New("试炼失败回执损坏")
	}
	if result.GetResult() != 0 {
		return false, nil
	}
	if end.GetResult() != 0 {
		return false, errors.New("试炼失败来源冲突")
	}
	out, scoped, err := p7ReadScopedRush(tx, fight, end)
	if err != nil {
		return false, err
	}
	if !scoped || out == nil || !out.Authoritative || out.Winner == 0 {
		return false, errors.New("试炼失败缺少独立接受见证")
	}
	return true, nil
}

// p7TrialMustReset reads only this current run/floor. Old runs never lock a new run.
func p7TrialMustReset(tx *gorm.DB, state *db.P7RushState) (bool, error) {
	var rows []db.P7RushFight
	if err := tx.Where("player_id=? AND run_id=? AND kind=? AND status=? AND diagnostic=?", state.PlayerID, state.RunID, constUtil.P7RushTrial, constUtil.P7RushSettled, false).Find(&rows).Error; err != nil {
		return false, err
	}
	for i := range rows {
		var rule db.P7RushStageRule
		if p6Digest(rows[i].Rule) != rows[i].RuleHash || json.Unmarshal(rows[i].Rule, &rule) != nil {
			return false, errors.New("试炼任务快照损坏")
		}
		if rule.Floor != state.Floor {
			continue
		}
		failed, err := p7AcceptedTrialFailure(tx, &rows[i])
		if err != nil {
			return false, err
		}
		if failed {
			return true, nil
		}
	}
	return false, nil
}

// p7FreezeMazeTrader runs after an actual accepted defeat, in the same profile transaction.
// The offer and even an unsuccessful appearance draw freeze once, before any query.
func (s *GameService) p7FreezeMazeTrader(tx *gorm.DB, fight *db.P7RushFight, now time.Time) error {
	failed, err := p7AcceptedTrialFailure(tx, fight)
	if err != nil || !failed {
		return err
	}
	var stage db.P7RushStageRule
	if json.Unmarshal(fight.Rule, &stage) != nil {
		return errors.New("迷宫商人试炼来源损坏")
	}
	if stage.Source == constUtil.P7ReRushSource {
		return nil
	}
	var count int64
	if err = tx.Model(&db.P7MazeTraderOffer{}).Where("player_id=? AND run_id=?", fight.PlayerID, fight.RunID).Count(&count).Error; err != nil || count > 0 {
		return err
	}
	var policy db.P7MazeTraderPolicy
	if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&policy, constUtil.P7MazeTraderPolicyID).Error; err != nil {
		return err
	}
	if !policy.Enabled {
		return nil
	}
	if err = s.p7ValidateMazePolicy(&policy); err != nil {
		return err
	}
	var record db.P6BattleRecord
	if err = tx.First(&record, "id=?", fight.ReplayID).Error; err != nil {
		return err
	}
	var rule db.P7RushStageRule
	if json.Unmarshal(fight.Rule, &rule) != nil {
		return errors.New("试炼任务快照损坏")
	}
	// No retrospective offers: the version must already be published and open at paid Start.
	if policy.PublishedAt > record.CreatedAt || record.CreatedAt < policy.StartAt || now.UnixMilli() >= policy.EndAt || rule.Floor < policy.MinFloor {
		return nil
	}
	draw, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return err
	}
	raw, err := json.Marshal(policy)
	if err != nil {
		return err
	}
	offer := db.P7MazeTraderOffer{PlayerID: fight.PlayerID, RunID: fight.RunID, FightID: fight.ID, StartHash: fight.StartHash, ResultHash: p6Digest(fight.Result), Policy: raw, PolicyHash: p6Digest(raw), Draw: int32(draw.Int64()), Appeared: draw.Int64() < int64(policy.AppearPPM), FrozenAt: now.UnixMilli()}
	return tx.Create(&offer).Error
}

func (s *GameService) p7CurrentMazeOffer(tx *gorm.DB, state *db.P7RushState, now time.Time) (*db.P7MazeTraderOffer, *db.P7MazeTraderPolicy, error) {
	offer := &db.P7MazeTraderOffer{}
	err := tx.First(offer, "player_id=? AND run_id=?", state.PlayerID, state.RunID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	policy := &db.P7MazeTraderPolicy{}
	if p6Digest(offer.Policy) != offer.PolicyHash || json.Unmarshal(offer.Policy, policy) != nil || s.p7ValidateMazePolicy(policy) != nil || offer.Draw < 0 || offer.Draw >= 1000000 || offer.Appeared != (offer.Draw < policy.AppearPPM) {
		return nil, nil, errors.New("迷宫商人冻结目录损坏")
	}
	if !offer.Appeared || now.UnixMilli() >= policy.EndAt {
		return nil, nil, nil
	}
	var fight db.P7RushFight
	if err = tx.First(&fight, "id=? AND player_id=? AND run_id=?", offer.FightID, state.PlayerID, state.RunID).Error; err != nil {
		return nil, nil, err
	}
	failed, err := p7AcceptedTrialFailure(tx, &fight)
	if err != nil {
		return nil, nil, err
	}
	if !failed || fight.StartHash != offer.StartHash || p6Digest(fight.Result) != offer.ResultHash {
		return nil, nil, errors.New("迷宫商人失败资格损坏")
	}
	return offer, policy, nil
}

// p7MazeTraderFlags projects the exact original reset/merchant flags without fabricating failure.
func (s *GameService) p7MazeTraderFlags(tx *gorm.DB, state *db.P7RushState, now time.Time) (int32, int32, error) {
	failed, err := p7TrialMustReset(tx, state)
	if err != nil || !failed {
		return 0, 0, err
	}
	offer, _, err := s.p7CurrentMazeOffer(tx, state, now)
	if err != nil {
		return 0, 0, err
	}
	available := int32(0)
	if offer != nil && offer.PurchasedAt == 0 {
		available = 1
	}
	return 1, available, nil
}

// P7MazeTrader keeps the original empty query/buy messages and existing P7 receipt identity.
func (s *GameService) P7MazeTrader(ctx context.Context, id int64, request proto.Message, stamp int64, now time.Time) ([]proto.Message, error) {
	var result proto.Message
	err := updateProfileWithTx(ctx, id, func(tx *gorm.DB, player *pb.Player, game *model.GameState) error {
		buy := false
		switch request.(type) {
		case *pb.CSRushMazeTrader:
		case *pb.CSRushMazeTraderBuy:
			buy = true
		default:
			return errors.New("迷宫商人协议无效")
		}
		if buy {
			previous := &pb.SCRushMazeTraderBuy{}
			found, err := p7ReadReceipt(tx, id, constUtil.P7MazeTraderReceipt, stamp, request, previous)
			if err != nil {
				return err
			}
			if found {
				result = previous
				return nil
			}
		}
		state, err := p7State(tx, id, now)
		if err != nil {
			return err
		}
		failed, err := p7TrialMustReset(tx, state)
		if err != nil {
			return err
		}
		if !failed || state.ActiveFight != "" {
			return errors.New("迷宫商人只在本轮试炼失败后出现")
		}
		offer, policy, err := s.p7CurrentMazeOffer(tx, state, now)
		if err != nil {
			return err
		}
		if offer == nil {
			return errors.New("本轮没有开放的迷宫商人")
		}
		if !buy {
			result = &pb.SCRushMazeTrader{Result: &pb.BaseResource{Id: proto.String(policy.ResourceID), Num: proto.Int32(policy.Quantity)}, OriginalPrice: proto.Int32(policy.OriginalPrice), DiscountPrice: proto.Int32(policy.DiscountPrice)}
			return nil
		}
		if offer.PurchasedAt != 0 {
			return errors.New("本轮迷宫商人商品已购买")
		}
		cost := []*pb.BaseResource{{Id: proto.String("diamond"), Num: proto.Int32(policy.DiscountPrice)}}
		if err = deductPoolCost(player, game, cost[0]); err != nil {
			return err
		}
		gain, err := s.GrantFixedReward(player, game, fmt.Sprintf("%s,%d", policy.ResourceID, policy.Quantity))
		if err != nil {
			return err
		}
		if err = s.RecordP1Cost(tx, id, fmt.Sprintf("p7-maze:%d", stamp), cost, now); err != nil {
			return err
		}
		offer.PurchasedAt = now.UnixMilli()
		offer.BuyStamp = stamp
		if err = tx.Save(offer).Error; err != nil {
			return err
		}
		result = &pb.SCRushMazeTraderBuy{Gain: gain, Cost: cost}
		return p7SaveReceipt(tx, id, constUtil.P7MazeTraderReceipt, stamp, request, result)
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{result}, nil
}
