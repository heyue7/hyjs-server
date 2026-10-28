package service

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"

	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/model"
)

func ggoTradeActionHash(action string, req model.GGOFieldTradeActionRequest) string {
	raw, _ := json.Marshal(struct {
		Action      string  `json:"action"`
		TradeID     string  `json:"tradeId"`
		InitiatorID int64   `json:"initiatorId"`
		ReceiverID  int64   `json:"receiverId"`
		Version     int64   `json:"version"`
		Offer       []int64 `json:"offer"`
	}{action, req.TradeID, req.InitiatorID, req.ReceiverID, req.Version, req.Offer})
	return p6Digest(raw)
}

// GGOFieldTradeAction processes accept, offer, confirm and cancel. Every
// request UUID is durable; the two settlement facts and both profiles commit
// in one transaction once both parties confirm the same quote version.
func (s *GameService) GGOFieldTradeAction(ctx context.Context, action string, req model.GGOFieldTradeActionRequest, session []byte) (*model.GGOFieldTradeView, error) {
	var view *model.GGOFieldTradeView
	changed := false
	err := s.ggoWithTradePair(ctx, req.GGOFieldAuth, session, req.InitiatorID, req.ReceiverID, func(tx *gorm.DB, pair *ggoTradePair) error {
		hash := ggoTradeActionHash(action, req)
		var old struct {
			TradeID string `json:"tradeId"`
		}
		replayed, err := ggoReadReceipt(tx, req.PlayerID, req.RequestID, constUtil.GGOFieldOpTradeAction, hash, &old)
		if err != nil {
			return err
		}
		row, state, err := ggoTradeRead(tx, req.TradeID, req.InitiatorID, req.ReceiverID)
		if err != nil {
			return err
		}
		if !replayed && action != "cancel" && (state.Status == "invited" || state.Status == "open") {
			if err = ggoCheckClientBuildLocked(tx, req.PlayerID, session, pair.Actors[0].Service.GGOFieldConfig); err != nil {
				return err
			}
		}
		changed, err = ggoTradeRefresh(tx, row, state, pair)
		if err != nil {
			return err
		}
		if replayed {
			view, err = ggoTradeView(state, pair, req.PlayerID)
			return err
		}
		index := 0
		if req.PlayerID == req.ReceiverID {
			index = 1
		}
		stale := action != "cancel" && req.Version != state.Version
		if !stale && (state.Status == "invited" || state.Status == "open") {
			switch action {
			case "accept":
				if index != 1 || state.Status != "invited" {
					return ggoFieldError(constUtil.GGOFieldBusy, "该交易无法接受")
				}
				state.Status, state.Reason = "open", ""
				changed = true
			case "offer":
				if state.Status != "open" || len(req.Offer) != 6 {
					return model.NewErrParamsError()
				}
				var offer [6]int64
				for i, amount := range req.Offer {
					if amount < 0 || amount > pair.Actors[index].Game.GGOField.Chips[i] {
						return ggoFieldError(constUtil.GGOFieldInsufficientLife, "交易芯片不足")
					}
					offer[i] = amount
				}
				if offer != state.Offers[index] {
					if state.Version == math.MaxInt64 {
						return errors.New("交易版本溢出")
					}
					state.Offers[index] = offer
					state.Version++
					state.Confirmed = [2]bool{}
					state.Reason = ""
					changed = true
				}
			case "confirm":
				if state.Status != "open" {
					return ggoFieldError(constUtil.GGOFieldBusy, "交易尚未接受")
				}
				cfg := pair.Actors[0].Service.GGOFieldConfig
				if cfg == nil || cfg.Runtime == nil {
					return ggoFieldError(constUtil.GGOFieldUnavailable, "交易暂未开放")
				}
				allowGift := cfg.Rules.AllowGift != nil && *cfg.Rules.AllowGift
				states := [2]*model.GGOFieldState{pair.Actors[0].Game.GGOField, pair.Actors[1].Game.GGOField}
				after, e := ggoTradeAfter(states, state.Offers, allowGift)
				if e != nil {
					if strings.Contains(e.Error(), "经济版本") {
						return ggoFieldError(constUtil.GGOFieldEconomyConflict, e.Error())
					}
					return ggoFieldError(constUtil.GGOFieldBusy, e.Error())
				}
				state.Confirmed[index] = true
				state.Reason = ""
				changed = true
				if state.Confirmed[0] && state.Confirmed[1] {
					for i, actor := range pair.Actors {
						state.SettledValues[i], e = ggoTradeOfferValue(states[i], state.Offers[i])
						if e != nil {
							return e
						}
						state.SettledChips[i] = after[i].Chips
						state.SettledProtected[i], _, e = ggoProtectedChips(after[i].Chips, cfg.Runtime.ProtectedChips)
						if e != nil {
							return e
						}
						*actor.Game.GGOField = after[i]
					}
					state.Status = "settled"
					ggoTradeClearLinks(pair, state)
					for i, actor := range pair.Actors {
						var delta [6]int64
						for color := range delta {
							delta[color] = state.Offers[1-i][color] - state.Offers[i][color]
						}
						key := state.TradeID
						if e = ggoSaveReceipt(tx, actor.Row.Id, ggoSystemReceiptID("trade-settlement", key), constUtil.GGOFieldOpTradeSettlement, hash, actor.Game.GGOField.EconomyVersion, &key, delta, pair.Actors[1-i].Row.Id, pair.Now, state); e != nil {
							return e
						}
					}
					if e = ggoTradeSaveProfiles(tx, pair); e != nil {
						return e
					}
				}
			case "cancel":
				state.Status, state.Reason = "cancelled", "participant_cancelled"
				state.Confirmed = [2]bool{}
				ggoTradeClearLinks(pair, state)
				if err = ggoTradeSaveProfiles(tx, pair); err != nil {
					return err
				}
				changed = true
			default:
				return model.NewErrParamsError()
			}
		}
		if changed {
			if err = ggoTradeSave(tx, row, state); err != nil {
				return err
			}
		}
		view, err = ggoTradeView(state, pair, req.PlayerID)
		if err != nil {
			return err
		}
		if stale && view.Active {
			view.Reason = "version_changed"
		}
		return ggoSaveReceipt(tx, req.PlayerID, req.RequestID, constUtil.GGOFieldOpTradeAction, hash, "", nil, [6]int64{}, 0, pair.Now, map[string]any{"tradeId": state.TradeID})
	})
	if err != nil {
		return nil, err
	}
	if changed {
		ggoTradeNotify(s, ctx, req.InitiatorID, req.ReceiverID)
	}
	return view, nil
}
