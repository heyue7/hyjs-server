package service

import (
	"context"
	"errors"
	"math"
	"strings"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Caller holds both profile rows in ascending ID order, as well as the native
// certified PvP result. This function never locks another player's profile.
func (s *GameService) ggoSettlePVP(tx *gorm.DB, winnerID, loserID int64, winner, loser *model.GameState, fightID string, now int64) error {
	cfg := s.GGOFieldConfig
	if cfg == nil || cfg.Runtime == nil || winner == nil || loser == nil || winner.GGOField == nil || loser.GGOField == nil || loser.Field == nil || loser.Field.Side == nil || fightID == "" {
		return errors.New("沙漠PK终局无效")
	}
	if loser.GGOField.PVPReturn != nil {
		return errors.New("前次沙漠死亡尚未返回")
	}
	if _, err := ggoWirePoint(cfg.Runtime.SafeReturn); err != nil {
		return err
	}
	heal := tableNumber(s.table("GameConfigure", "StaticName", "Resurrection2Safe"), "NameValue")
	if math.IsNaN(heal) || math.IsInf(heal, 0) || heal <= 0 || heal > 1 {
		return errors.New("沙漠返回恢复值无效")
	}
	_, winnerTotal, err := ggoProtectedChips(winner.GGOField.Chips, cfg.Runtime.ProtectedChips)
	if err != nil {
		return err
	}
	_, loserTotal, err := ggoProtectedChips(loser.GGOField.Chips, cfg.Runtime.ProtectedChips)
	if err != nil {
		return err
	}
	lost, err := ggoDeathLoss(loser.GGOField.Chips, cfg.Runtime.ProtectedChips, nil)
	if err != nil {
		return err
	}
	var count int64
	for _, n := range lost {
		count += n
	}
	if count > 0 {
		if winnerTotal > math.MaxInt64-count {
			return errors.New("胜者芯片总数溢出")
		}
		if loser.GGOField.EconomyVersion == "" {
			return errors.New("沙漠芯片经济版本缺失")
		}
		for i, n := range lost {
			if n > 0 && loser.GGOField.ExchangeValues[i] < 1 {
				return errors.New("沙漠芯片冻结价格缺失")
			}
		}
		if winnerTotal > 0 && (winner.GGOField.EconomyVersion != loser.GGOField.EconomyVersion || winner.GGOField.ExchangeValues != loser.GGOField.ExchangeValues) {
			return ggoFieldError(constUtil.GGOFieldEconomyConflict, "双方芯片经济版本不同，请先结算")
		}
		if loser.GGOField.Revision == math.MaxInt64 || winner.GGOField.Revision == math.MaxInt64 {
			return errors.New("沙漠终端版本溢出")
		}
		if err = ggoApplyChipTransfer(&loser.GGOField.Chips, &winner.GGOField.Chips, lost); err != nil {
			return err
		}
		if winnerTotal == 0 {
			winner.GGOField.EconomyVersion = loser.GGOField.EconomyVersion
			winner.GGOField.ExchangeValues = loser.GGOField.ExchangeValues
		}
		loser.GGOField.Revision++
		winner.GGOField.Revision++
	}
	// A certified winner makes the other side the defeated side even when the
	// original round limit leaves nonzero HP. Safe return waits for presentation.
	loser.Field.Side.CurHp = proto.Float64(0)
	loser.Field.Side.State = proto.Int32(constUtil.P8FieldDead)
	loser.GGOField.PVPReturn = &model.GGOFieldPVPReturn{FightID: fightID, Position: cfg.Runtime.SafeReturn, HealRatio: heal}
	receipt := model.GGOFieldPVPReceipt{FightID: fightID, WinnerID: winnerID, LoserID: loserID, Lost: lost}
	version := loser.GGOField.EconomyVersion
	var debit [6]int64
	for i, n := range lost {
		debit[i] = -n
	}
	key := fightID
	if err = ggoSaveReceipt(tx, loserID, ggoSystemReceiptID("pvp-loss", fightID), constUtil.GGOFieldOpPVPLoss, p6Digest([]byte(fightID)), version, &key, debit, winnerID, now, receipt); err != nil {
		return err
	}
	if err = ggoSaveReceipt(tx, winnerID, ggoSystemReceiptID("pvp-win", fightID), constUtil.GGOFieldOpPVPWin, p6Digest([]byte(fightID)), version, &key, lost, loserID, now, receipt); err != nil {
		return err
	}
	_ = loserTotal
	return nil
}

func (s *GameService) GGOFieldPVPReceipt(ctx context.Context, req model.GGOFieldPVPReceiptRequest, session []byte) (*model.GGOFieldPVPReceipt, error) {
	var out model.GGOFieldPVPReceipt
	found := false
	err := s.ggoWithProfile(ctx, req.GGOFieldAuth, session, false, false, func(tx *gorm.DB, a *ggoFieldActor) error {
		for _, operation := range []string{constUtil.GGOFieldOpPVPWin, constUtil.GGOFieldOpPVPLoss} {
			yes, e := ggoFindBusinessReceipt(tx, a.Row.Id, operation, req.FightID, &out)
			if e != nil {
				return e
			}
			if yes {
				found = true
				break
			}
		}
		if !found {
			return ggoFieldError(constUtil.GGOFieldStaleGeneration, "沙漠PK回执不存在")
		}
		if a.Game.GGOField != nil {
			out.TerminalRevision = a.Game.GGOField.Revision
		}
		out.CreditBalance = ggoCreditBalance(a.Game)
		if out.CreditBalance < 0 {
			return errors.New("信用余额无效")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// ggoPVPFinishReturn runs under the profile lock after the native animation lease.
func (s *GameService) ggoPVPFinishReturn(tx *gorm.DB, p *pb.Player, g *model.GameState, fightID string, now int64) (bool, error) {
	if g.GGOField == nil || g.GGOField.PVPReturn == nil {
		return false, nil
	}
	pending := g.GGOField.PVPReturn
	if pending.FightID != fightID || strings.TrimSpace(fightID) == "" {
		return false, errors.New("沙漠PK返回记录不匹配")
	}
	point, err := ggoWirePoint(pending.Position)
	if err != nil {
		return false, err
	}
	if g.Field == nil || g.Field.Side == nil || pending.HealRatio <= 0 || pending.HealRatio > 1 || math.IsNaN(pending.HealRatio) || math.IsInf(pending.HealRatio, 0) {
		return false, errors.New("沙漠PK返回状态无效")
	}
	if err = s.p8Heal(p, g, pending.HealRatio, true); err != nil {
		return false, err
	}
	if err = ggoPositionBump(g); err != nil {
		return false, err
	}
	g.Position.Current = point
	g.PositionLoading = true
	g.Field.Side.State = proto.Int32(constUtil.P8FieldIdle)
	g.Field.Side.Anger = proto.Float64(0)
	state, err := p8LifeState(tx, p.GetPlayerId())
	if err != nil {
		return false, err
	}
	state.DeathAt = 0
	state.SafeRecoveryAt = now
	state.InSafe = true
	state.LastFightID = fightID
	if err = tx.Save(state).Error; err != nil {
		return false, err
	}
	g.GGOField.PVPReturn = nil
	return true, nil
}
