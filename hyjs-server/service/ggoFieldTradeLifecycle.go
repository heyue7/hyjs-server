package service

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"

	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/elog"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Clearing the player's link makes a concurrent bilateral confirmation fail
// under its ordered profile locks, without acquiring the peer's profile here.
func ggoCancelTradeForActor(id int64, g *model.GameState) int64 {
	if g == nil || g.GGOField == nil || g.GGOField.Trade == nil {
		return 0
	}
	link := g.GGOField.Trade
	g.GGOField.Trade = nil
	if id == link.InitiatorID {
		return link.ReceiverID
	}
	if id == link.ReceiverID {
		return link.InitiatorID
	}
	return 0
}

func (s *GameService) ggoTradeCancelSession(ctx context.Context, id int64, session []byte) error {
	var peer int64
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error; err != nil {
			return err
		}
		if len(session) != 16 || subtle.ConstantTimeCompare(row.Session, session) != 1 {
			return nil
		}
		if row.Player == nil || row.Game == nil {
			return nil
		}
		player, game, err := decodeProfile(&row)
		if err != nil {
			return err
		}
		if game.GGOField == nil {
			return nil
		}
		changed := game.GGOField.PositionConfirmed != 0 || game.GGOField.PositionSession != ""
		game.GGOField.PositionConfirmed, game.GGOField.PositionSession = 0, ""
		if link := game.GGOField.Trade; link != nil {
			if id == link.InitiatorID {
				peer = link.ReceiverID
			} else if id == link.ReceiverID {
				peer = link.InitiatorID
			} else {
				return errors.New("交易角色引用损坏")
			}
			game.GGOField.Trade = nil
			changed = true
		}
		if !changed {
			return nil
		}
		return saveProfile(tx, &row, player, game)
	})
	if err == nil && peer > 0 {
		ggoTradeNotify(s, ctx, peer)
	}
	return err
}

// The existing receipt due index also bounds cleanup of timed-out quotes.
// Each quote locks both profiles in the same order as live actions.
func (s *GameService) ggoRecoverTrades(ctx context.Context, now int64) error {
	var rows []db.GGOFieldReceipt
	query := db.GetIns().WithContext(ctx).Model(&db.GGOFieldReceipt{}).
		Select("ggo_field_receipts.player_id, ggo_field_receipts.request_id, ggo_field_receipts.response").
		Joins("JOIN profiles ON profiles.id = ggo_field_receipts.player_id").
		Where("ggo_field_receipts.operation=? AND ggo_field_receipts.active=? AND ggo_field_receipts.ready_at<=?", constUtil.GGOFieldOpTradeState, true, now).
		Where("(profiles.server_id=? OR (?=? AND profiles.server_id<=0))", s.OriginalServerID(), s.OriginalServerID(), constUtil.GameDefaultServerID).
		Order("ggo_field_receipts.ready_at").Limit(64)
	if err := query.Find(&rows).Error; err != nil {
		return err
	}
	for _, candidate := range rows {
		var initial ggoTradeState
		if err := json.Unmarshal(candidate.Response, &initial); err != nil || initial.InitiatorID != candidate.PlayerID || initial.TradeID != candidate.RequestID || initial.ReceiverID < 1 || initial.ReceiverID == initial.InitiatorID {
			elog.Warn("ggo trade recovery found an invalid quote")
			continue
		}
		p10SceneMutation.Lock()
		changed := false
		err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			ids := []int64{initial.InitiatorID, initial.ReceiverID}
			if ids[0] > ids[1] {
				ids[0], ids[1] = ids[1], ids[0]
			}
			actors := map[int64]*ggoFieldActor{}
			for _, id := range ids {
				row := &db.Profile{}
				if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(row, id).Error; err != nil {
					return err
				}
				if AdminProfileServerID(*row) != s.OriginalServerID() {
					return nil
				}
				player, game, err := decodeProfile(row)
				if err != nil {
					return err
				}
				actors[id] = &ggoFieldActor{Service: s, Row: row, Player: player, Game: game, Now: now}
			}
			pair := &ggoTradePair{Actors: [2]*ggoFieldActor{actors[initial.InitiatorID], actors[initial.ReceiverID]}, Now: now}
			row, state, err := ggoTradeRead(tx, initial.TradeID, initial.InitiatorID, initial.ReceiverID)
			if err != nil {
				return err
			}
			if (state.Status != "invited" && state.Status != "open") || state.ExpiresAtMS > now {
				return nil
			}
			state.Status, state.Reason, state.Confirmed = "expired", "timeout", [2]bool{}
			ggoTradeClearLinks(pair, state)
			if err = ggoTradeSave(tx, row, state); err != nil {
				return err
			}
			if err = ggoTradeSaveProfiles(tx, pair); err != nil {
				return err
			}
			changed = true
			return nil
		})
		p10SceneMutation.Unlock()
		if err != nil {
			elog.Warn("ggo trade expiry failed", zap.Error(err))
			continue
		}
		if changed {
			ggoTradeNotify(s, ctx, initial.InitiatorID, initial.ReceiverID)
		}
	}
	return nil
}

// A cancelled quote is recovered by its trade ID; no asset is changed here.
func ggoCancelTradeOnPosition(id int64, g *model.GameState, area *model.AdminFieldGGOSafeArea, next *pb.PointNew) int64 {
	if g != nil && g.GGOField != nil && g.GGOField.Trade != nil && !ggoPointInsideSafe(next, area) {
		return ggoCancelTradeForActor(id, g)
	}
	return 0
}
