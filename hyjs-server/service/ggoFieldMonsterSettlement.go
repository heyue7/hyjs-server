package service

import (
	"context"
	"errors"
	"math"
	"strconv"
	"strings"

	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

type ggoPendingMonster struct {
	FightID        string   `json:"fightId"`
	Reward         string   `json:"reward"`
	Chips          [6]int64 `json:"chips"`
	EconomyVersion string   `json:"economyVersion"`
	ExchangeValues [6]int64 `json:"exchangeValues"`
}

func (s *GameService) ggoMonsterRewardFits(p *pb.Player, g *model.GameState, reward string) (bool, error) {
	if reward == "" {
		return true, nil
	}
	supported, err := s.openServerRewardSupported(reward)
	if err != nil {
		return false, err
	}
	if !supported {
		return false, errors.New("沙漠小怪奖励资源不存在")
	}
	ok, err := ggoInventoryHasSpace(g, reward)
	if err != nil || !ok {
		return ok, err
	}
	var warriors int64
	for _, part := range strings.Split(reward, ";") {
		pair := strings.Split(part, ",")
		if len(pair) != 2 {
			return false, errors.New("沙漠小怪奖励无效")
		}
		amount, e := strconv.ParseInt(pair[1], 10, 32)
		if e != nil || amount < 1 {
			return false, errors.New("沙漠小怪奖励无效")
		}
		if strings.HasPrefix(pair[0], "hero_") {
			warriors += amount
		}
		if pair[0] == "coin" && p.GetCoin() > math.MaxInt64-amount {
			return false, nil
		}
		if pair[0] == constUtil.GGOCurrencyID && ggoCreditBalance(g) > math.MaxInt64-amount {
			return false, nil
		}
	}
	if warriors > 0 && warriors > int64(p.GetWarriorBagSize())-int64(len(g.Warriors)) {
		return false, nil
	}
	return true, nil
}

// ggoMonsterAward applies the already-scaled ordinary reward and one frozen
// chip outcome. The normal reward grant is internally atomic on a clone.
func (s *GameService) ggoMonsterAward(p *pb.Player, g *model.GameState, pending *ggoPendingMonster) (*model.GGOFieldMonsterReceipt, bool, error) {
	if pending == nil || pending.FightID == "" || g.GGOField == nil {
		return nil, false, errors.New("沙漠小怪冻结奖励无效")
	}
	fits, err := s.ggoMonsterRewardFits(p, g, pending.Reward)
	if err != nil || !fits {
		return nil, !fits, err
	}
	terminal := g.GGOField
	_, held, err := ggoProtectedChips(terminal.Chips, 3)
	if err != nil {
		return nil, false, err
	}
	var added int64
	for i, n := range pending.Chips {
		if n < 0 || n > 1 {
			return nil, false, errors.New("沙漠小怪芯片奖励无效")
		}
		if terminal.Chips[i] > math.MaxInt64-n || held > math.MaxInt64-n {
			return nil, true, nil
		}
		held += n
		added += n
	}
	if added > 1 {
		return nil, false, errors.New("沙漠小怪芯片奖励无效")
	}
	oldHeld := held - added
	if added > 0 && oldHeld > 0 && (terminal.EconomyVersion != pending.EconomyVersion || terminal.ExchangeValues != pending.ExchangeValues) {
		return nil, true, nil
	}
	if added > 0 && terminal.Revision == math.MaxInt64 {
		return nil, false, errors.New("沙漠终端版本溢出")
	}
	gain := &pb.ResultObject{}
	if pending.Reward != "" {
		gain, err = s.GrantFixedReward(p, g, pending.Reward)
		if err != nil {
			if strings.Contains(err.Error(), "背包已满") || strings.Contains(err.Error(), "溢出") {
				return nil, true, nil
			}
			return nil, false, err
		}
	}
	terminal = g.GGOField
	if added > 0 {
		if oldHeld == 0 {
			terminal.EconomyVersion = pending.EconomyVersion
			terminal.ExchangeValues = pending.ExchangeValues
		}
		for i, n := range pending.Chips {
			terminal.Chips[i] += n
		}
		terminal.Revision++
	}
	out := &model.GGOFieldMonsterReceipt{FightID: pending.FightID, Status: "settled", DeliveryStatus: "delivered", Resources: []model.GGOFieldResourceAmount{}, Warriors: []*pb.Warrior{}, Chips: pending.Chips, TerminalRevision: terminal.Revision, CreditBalance: ggoCreditBalance(g)}
	if out.CreditBalance < 0 {
		return nil, false, errors.New("信用余额无效")
	}
	for _, item := range gain.GetSimps() {
		out.Resources = append(out.Resources, model.GGOFieldResourceAmount{ID: item.GetId(), Amount: int64(item.GetNum())})
	}
	out.Warriors = append(out.Warriors, gain.GetWars()...)
	return out, false, nil
}

func (s *GameService) GGOFieldMonsterReceipt(ctx context.Context, req model.GGOFieldMonsterReceiptRequest, session []byte) (*model.GGOFieldMonsterReceipt, error) {
	var out model.GGOFieldMonsterReceipt
	err := s.ggoWithProfile(ctx, req.GGOFieldAuth, session, false, false, func(tx *gorm.DB, a *ggoFieldActor) error {
		found, err := ggoFindBusinessReceipt(tx, a.Row.Id, constUtil.GGOFieldOpMonsterClaim, req.FightID, &out)
		if err != nil {
			return err
		}
		if !found {
			found, err = ggoFindBusinessReceipt(tx, a.Row.Id, constUtil.GGOFieldOpMonsterResult, req.FightID, &out)
			if err != nil {
				return err
			}
		}
		if !found {
			out.FightID = req.FightID
			out.Status = "unknown"
			if a.Game.GGOField != nil && a.Game.GGOField.MonsterFight != nil && a.Game.GGOField.MonsterFight.FightID == req.FightID {
				out.Status = "active"
			}
		} else if out.Status == "settled" && (out.EndRequestHash == "" || len(out.NativeEnd) == 0) {
			var original model.GGOFieldMonsterReceipt
			ok, e := ggoFindBusinessReceipt(tx, a.Row.Id, constUtil.GGOFieldOpMonsterResult, req.FightID, &original)
			if e != nil {
				return e
			}
			if !ok || original.EndRequestHash == "" || len(original.NativeEnd) == 0 {
				return errors.New("沙漠小怪原始回执缺失")
			}
			out.InputHash, out.EndRequestHash, out.NativeEnd = original.InputHash, original.EndRequestHash, original.NativeEnd
		}
		out.CreditBalance = ggoCreditBalance(a.Game)
		if out.CreditBalance < 0 {
			return errors.New("信用余额无效")
		}
		if a.Game.GGOField != nil {
			out.TerminalRevision = a.Game.GGOField.Revision
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *GameService) GGOFieldMonsterClaim(ctx context.Context, req model.GGOFieldMonsterClaimRequest, session []byte) (*model.GGOFieldMonsterReceipt, error) {
	var out model.GGOFieldMonsterReceipt
	full := false
	err := s.ggoWithProfile(ctx, req.GGOFieldAuth, session, false, false, func(tx *gorm.DB, a *ggoFieldActor) error {
		hash := p6Digest([]byte(req.FightID))
		if found, err := ggoReadReceipt(tx, a.Row.Id, req.RequestID, constUtil.GGOFieldOpMonsterClaim, hash, &out); err != nil {
			return err
		} else if found {
			out.CreditBalance = ggoCreditBalance(a.Game)
			if out.CreditBalance < 0 {
				return errors.New("信用余额无效")
			}
			if a.Game.GGOField != nil {
				out.TerminalRevision = a.Game.GGOField.Revision
			}
			return nil
		}
		if found, err := ggoFindBusinessReceipt(tx, a.Row.Id, constUtil.GGOFieldOpMonsterClaim, req.FightID, &out); err != nil {
			return err
		} else if found {
			out.RequestID = req.RequestID
			out.CreditBalance = ggoCreditBalance(a.Game)
			if out.CreditBalance < 0 {
				return errors.New("信用余额无效")
			}
			if a.Game.GGOField != nil {
				out.TerminalRevision = a.Game.GGOField.Revision
			}
			return ggoSaveReceipt(tx, a.Row.Id, req.RequestID, constUtil.GGOFieldOpMonsterClaim, hash, "", nil, [6]int64{}, 0, a.Now, out)
		}
		var original model.GGOFieldMonsterReceipt
		found, err := ggoFindBusinessReceipt(tx, a.Row.Id, constUtil.GGOFieldOpMonsterResult, req.FightID, &original)
		if err != nil {
			return err
		}
		if !found {
			return ggoFieldError(constUtil.GGOFieldStaleGeneration, "沙漠小怪结果尚不存在")
		}
		if original.DeliveryStatus != "pending_delivery" {
			out = original
			out.RequestID = req.RequestID
			return ggoSaveReceipt(tx, a.Row.Id, req.RequestID, constUtil.GGOFieldOpMonsterClaim, hash, "", nil, [6]int64{}, 0, a.Now, out)
		}
		var pending ggoPendingMonster
		found, err = ggoFindBusinessReceipt(tx, a.Row.Id, constUtil.GGOFieldOpMonsterReady, req.FightID, &pending)
		if err != nil {
			return err
		}
		if !found {
			return errors.New("沙漠小怪待领奖励缺失")
		}
		result, isFull, err := a.Service.ggoMonsterAward(a.Player, a.Game, &pending)
		if err != nil {
			return err
		}
		if isFull {
			full = true
			return nil
		}
		out = *result
		out.RequestID = req.RequestID
		out.InputHash, out.EndRequestHash, out.NativeEnd = original.InputHash, original.EndRequestHash, original.NativeEnd
		if out.EndRequestHash == "" || len(out.NativeEnd) == 0 {
			return errors.New("沙漠小怪原始回执缺失")
		}
		if err = saveProfile(tx, a.Row, a.Player, a.Game); err != nil {
			return err
		}
		key := req.FightID
		return ggoSaveReceipt(tx, a.Row.Id, req.RequestID, constUtil.GGOFieldOpMonsterClaim, hash, pending.EconomyVersion, &key, pending.Chips, 0, a.Now, out)
	})
	if err != nil {
		return nil, err
	}
	if full {
		return nil, ggoFieldError(constUtil.GGOFieldInventoryFull, "背包或终端空间不足，原奖励已保留")
	}
	return &out, nil
}
