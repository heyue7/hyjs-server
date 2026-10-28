package service

import (
	"context"
	"errors"
	"math"
	"time"

	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

// ggoSettleTerminal uses the six prices frozen with the carried chips. It
// mutates only the caller's locked profile; save and receipt commit together.
func ggoSettleTerminal(game *model.GameState, requestID string) (*model.GGOFieldSettlementResponse, error) {
	if game == nil || game.GGOField == nil {
		return nil, ggoFieldError(constUtil.GGOFieldInvalidLocation, "沙漠终端不存在")
	}
	terminal := game.GGOField
	out := &model.GGOFieldSettlementResponse{RequestID: requestID, EconomyVersion: terminal.EconomyVersion, Chips: terminal.Chips, TerminalRevision: terminal.Revision}
	var total int64
	for i, count := range terminal.Chips {
		if count < 0 || count > math.MaxInt64-total {
			return nil, errors.New("沙漠芯片数量无效")
		}
		total += count
		price := terminal.ExchangeValues[i]
		if count > 0 && (price < 1 || count > math.MaxInt64/price || out.CreditAdded > math.MaxInt64-count*price) {
			return nil, ggoFieldError(constUtil.GGOFieldCreditOverflow, "信用余额超出上限，芯片仍保留")
		}
		out.CreditAdded += count * price
	}
	if total > 0 {
		if terminal.EconomyVersion == "" || terminal.Revision == math.MaxInt64 {
			return nil, errors.New("沙漠终端版本无效")
		}
		if err := ggoCreditAdd(game, out.CreditAdded); err != nil {
			return nil, ggoFieldError(constUtil.GGOFieldCreditOverflow, "信用余额超出上限，芯片仍保留")
		}
		terminal.Chips = [6]int64{}
		terminal.Revision++
		out.TerminalRevision = terminal.Revision
	}
	out.CreditBalance = ggoCreditBalance(game)
	if out.CreditBalance < 0 {
		return nil, errors.New("信用余额无效")
	}
	return out, nil
}

// GGOFieldSettlement can complete for an already-present character while new
// desert entry is closed. The NPC position comes from the published snapshot,
// while chip prices come from the terminal's frozen economy version.
func (s *GameService) GGOFieldSettlement(ctx context.Context, req model.GGOFieldSettlementRequest, session []byte) (*model.GGOFieldSettlementResponse, error) {
	var out model.GGOFieldSettlementResponse
	err := s.ggoWithProfile(ctx, req.GGOFieldAuth, session, false, false, func(tx *gorm.DB, a *ggoFieldActor) error {
		hash := p6Digest([]byte(constUtil.GGOFieldOpSettlement))
		if found, err := ggoReadReceipt(tx, a.Row.Id, req.RequestID, constUtil.GGOFieldOpSettlement, hash, &out); err != nil {
			return err
		} else if found {
			// A replay carries the original delta but must not roll back a newer balance.
			out.CreditBalance = ggoCreditBalance(a.Game)
			if out.CreditBalance < 0 {
				return errors.New("信用余额无效")
			}
			if a.Game.GGOField != nil {
				out.TerminalRevision = a.Game.GGOField.Revision
			}
			return nil
		}
		cfg := a.Service.GGOFieldEconomy
		if cfg == nil || cfg.Runtime == nil {
			return ggoFieldError(constUtil.GGOFieldUnavailable, "沙漠结算配置不可用")
		}
		if err := ggoCurrentPresence(tx, a, session); err != nil {
			return err
		}
		if !ggoPointInsideSafe(a.Game.Position.GetCurrent(), cfg.Runtime.SafeArea) {
			return ggoFieldError(constUtil.GGOFieldInvalidLocation, "请回到安全区结算")
		}
		var npc *model.AdminFieldGGONPC
		for i := range cfg.NPCs {
			if cfg.NPCs[i].Role == "settlement" {
				npc = &cfg.NPCs[i]
				break
			}
		}
		if npc == nil {
			return errors.New("沙漠结算员未配置")
		}
		point, err := ggoWirePoint(npc.Position)
		if err != nil {
			return err
		}
		if !p8Near(a.Game.Position.GetCurrent(), &p8GatherConfig{Point: point, Radius: 3}) {
			return ggoFieldError(constUtil.GGOFieldInvalidLocation, "请靠近终端结算员")
		}
		if a.Game.Field.Side.GetCurHp() <= 0 || a.Game.Field.Fight != nil && !a.Game.Field.Fight.Settled || a.Game.Run != nil && !a.Game.Run.Complete {
			return ggoFieldError(constUtil.GGOFieldBusy, "当前无法结算")
		}
		if err = P12FieldPVPRequireIdle(tx, a.Row.Id, time.UnixMilli(a.Now)); err != nil {
			return err
		}
		var active db.P8GatherAttempt
		if err = tx.Where("player_id=? AND active=?", a.Row.Id, true).First(&active).Error; err == nil {
			return ggoFieldError(constUtil.GGOFieldBusy, "当前无法结算")
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		late, err := a.Service.ggoSettleLateBossAwardsLocked(tx, a.Row.Id, a.Game)
		if err != nil {
			return err
		}
		result, err := ggoSettleTerminal(a.Game, req.RequestID)
		if err != nil {
			return err
		}
		if late < 0 || result.CreditAdded > math.MaxInt64-late {
			return ggoFieldError(constUtil.GGOFieldCreditOverflow, "信用余额超出上限，芯片仍保留")
		}
		result.CreditAdded += late
		out = *result
		if err = saveProfile(tx, a.Row, a.Player, a.Game); err != nil {
			return err
		}
		var delta [6]int64
		for i, count := range out.Chips {
			delta[i] = -count
		}
		return ggoSaveReceipt(tx, a.Row.Id, req.RequestID, constUtil.GGOFieldOpSettlement, hash, out.EconomyVersion, nil, delta, 0, a.Now, out)
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}
