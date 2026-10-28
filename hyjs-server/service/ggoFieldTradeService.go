package service

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"math"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

const ggoTradeLifetimeMS int64 = 120000

type ggoTradeState struct {
	TradeID          string      `json:"tradeId"`
	InitiatorID      int64       `json:"initiatorId"`
	ReceiverID       int64       `json:"receiverId"`
	Status           string      `json:"status"`
	Reason           string      `json:"reason,omitempty"`
	Version          int64       `json:"version"`
	Offers           [2][6]int64 `json:"offers"`
	Confirmed        [2]bool     `json:"confirmed"`
	Revisions        [2]int64    `json:"revisions"`
	ExpiresAtMS      int64       `json:"expiresAtMs"`
	SettledValues    [2]string   `json:"settledValues,omitempty"`
	SettledChips     [2][6]int64 `json:"settledChips,omitempty"`
	SettledProtected [2][6]int64 `json:"settledProtected,omitempty"`
}

type ggoTradePair struct {
	Actors [2]*ggoFieldActor // invitation order, not lock order
	Now    int64
}

func ggoTradeIDsValid(actor, initiator, receiver int64) bool {
	return initiator > 0 && receiver > 0 && initiator != receiver && (actor == initiator || actor == receiver)
}

// ggoWithTradePair uses the same global scene mutex and ascending player lock
// order as native PvP. No request ever locks its own player before the peer.
func (s *GameService) ggoWithTradePair(ctx context.Context, auth model.GGOFieldAuth, session []byte, initiator, receiver int64, apply func(*gorm.DB, *ggoTradePair) error) error {
	if !ggoTradeIDsValid(auth.PlayerID, initiator, receiver) || len(session) != 16 {
		return model.NewErrParamsError()
	}
	profile, err := AuthorizeAccount(ctx, auth.PlayerID, session)
	if err != nil {
		return ggoClientAuthError(err)
	}
	if AdminProfileServerID(*profile) != s.OriginalServerID() {
		return constUtil.ErrAdminPermission
	}
	ctx, err = s.BindLocalPlayContext(ctx, profile)
	if err != nil {
		return err
	}
	snapshot, err := s.adminFieldRuntimeSnapshotForRequest(ctx, s.OriginalServerID())
	if err != nil {
		return err
	}
	runtime := s.withRuntimeTables(snapshot.tables)
	runtime.GGOFieldConfig, runtime.GGOFieldEconomy, runtime.GGOFieldRevision = snapshot.ggo, snapshot.ggoEconomy, snapshot.ggoRevision
	ctx = context.WithValue(ctx, constUtil.GameServiceContextKey, runtime)
	p10SceneMutation.Lock()
	defer p10SceneMutation.Unlock()
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}
		ids := []int64{initiator, receiver}
		if ids[0] > ids[1] {
			ids[0], ids[1] = ids[1], ids[0]
		}
		actors := map[int64]*ggoFieldActor{}
		now := time.Now().UnixMilli()
		for _, id := range ids {
			row := &db.Profile{}
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(row, id).Error; err != nil {
				return err
			}
			if AdminProfileServerID(*row) != s.OriginalServerID() || id == auth.PlayerID && subtle.ConstantTimeCompare(row.Session, session) != 1 {
				return ggoFieldError(constUtil.GGOFieldInvalidLocation, "角色或会话已变化")
			}
			player, game, err := decodeProfileForCalendarUpdate(tx, row)
			if err != nil {
				return err
			}
			actors[id] = &ggoFieldActor{Service: runtime, Row: row, Player: player, Game: game, Now: now}
		}
		return apply(tx, &ggoTradePair{Actors: [2]*ggoFieldActor{actors[initiator], actors[receiver]}, Now: now})
	})
}

func ggoTradeLinkMatches(link *model.GGOFieldTradeLink, state *ggoTradeState) bool {
	return link != nil && state != nil && link.TradeID == state.TradeID && link.InitiatorID == state.InitiatorID && link.ReceiverID == state.ReceiverID && link.ExpiresAtMS == state.ExpiresAtMS
}

// Verify presence in ascending player order. The caller already owns both rows.
func ggoTradeLive(tx *gorm.DB, pair *ggoTradePair) (bool, error) {
	cfg := pair.Actors[0].Service.GGOFieldConfig
	if cfg == nil || cfg.Runtime == nil || !cfg.Rules.SafeTradeEnabled {
		return false, nil
	}
	ordered := []*ggoFieldActor{pair.Actors[0], pair.Actors[1]}
	if ordered[0].Row.Id > ordered[1].Row.Id {
		ordered[0], ordered[1] = ordered[1], ordered[0]
	}
	for _, actor := range ordered {
		if actor.Game.GGOField == nil || actor.Game.Field == nil || actor.Game.Field.Side == nil || actor.Game.Position == nil || actor.Game.PositionLoading || actor.Game.Field.Side.GetCurHp() <= 0 || actor.Game.Field.Side.GetState() != constUtil.P8FieldIdle || actor.Game.Field.Fight != nil && !actor.Game.Field.Fight.Settled || actor.Game.Run != nil && !actor.Game.Run.Complete || actor.Game.GGOField.Extraction != nil || !ggoPointInsideSafe(actor.Game.Position.GetCurrent(), cfg.Runtime.SafeArea) {
			return false, nil
		}
		value, ok := p10ClientSinks.Load(p10ClientKey{actor.Service.p10ClientOwner(), actor.Row.Id})
		if !ok {
			return false, nil
		}
		sink := value.(*p10ClientSink)
		if err := ggoCheckClientBuildLocked(tx, actor.Row.Id, actor.Row.Session, cfg); err != nil {
			var biz *model.BizCode
			if errors.As(err, &biz) && biz.Code == constUtil.GGOFieldClientUpdateRequired {
				return false, nil
			}
			return false, err
		}
		if !sink.tradeReady.Load() || len(sink.session) != 16 || subtle.ConstantTimeCompare(sink.session, actor.Row.Session) != 1 {
			return false, nil
		}
		if actor.Row.AccountID > 0 {
			account, err := AccountForProfile(tx, actor.Row)
			if err != nil {
				return false, err
			}
			if !account.Enabled || account.AuthRevision != actor.Row.AccountAuthRevision {
				return false, nil
			}
		}
		if err := ggoCurrentPresence(tx, actor, sink.session); err != nil {
			var biz *model.BizCode
			if errors.As(err, &biz) && biz.Code == constUtil.GGOFieldInvalidLocation {
				return false, nil
			}
			return false, err
		}
		if err := P12FieldPVPRequireIdle(tx, actor.Row.Id, time.UnixMilli(pair.Now)); err != nil {
			if err.Error() == constUtil.P12FieldPVPBusy || err.Error() == constUtil.P12FieldPVPJailed {
				return false, nil
			}
			return false, err
		}
		var active db.P8GatherAttempt
		if err := tx.Where("player_id=? AND active=?", actor.Row.Id, true).First(&active).Error; err == nil {
			return false, nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return false, err
		}
	}
	return pair.Actors[0].Presence.Scene == pair.Actors[1].Presence.Scene && pair.Actors[0].Presence.Line == pair.Actors[1].Presence.Line, nil
}

func ggoTradeRead(tx *gorm.DB, tradeID string, initiator, receiver int64) (*db.GGOFieldReceipt, *ggoTradeState, error) {
	var row db.GGOFieldReceipt
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "player_id=? AND request_id=?", initiator, tradeID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ggoFieldError(constUtil.GGOFieldStaleGeneration, "交易不存在")
		}
		return nil, nil, err
	}
	var state ggoTradeState
	if row.Operation != constUtil.GGOFieldOpTradeState || json.Unmarshal(row.Response, &state) != nil || state.TradeID != tradeID || state.InitiatorID != initiator || state.ReceiverID != receiver || state.Version < 1 {
		return nil, nil, ggoFieldError(constUtil.GGOFieldRequestConflict, "交易编号或参与者不一致")
	}
	return &row, &state, nil
}

func ggoTradeSave(tx *gorm.DB, row *db.GGOFieldReceipt, state *ggoTradeState) error {
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	active := state.Status == "invited" || state.Status == "open"
	return tx.Model(row).Updates(map[string]any{"response": raw, "active": active, "ready_at": state.ExpiresAtMS}).Error
}

func ggoTradeSaveProfiles(tx *gorm.DB, pair *ggoTradePair) error {
	for _, actor := range pair.Actors {
		if err := saveProfile(tx, actor.Row, actor.Player, actor.Game); err != nil {
			return err
		}
	}
	return nil
}

func ggoTradeClearLinks(pair *ggoTradePair, state *ggoTradeState) {
	for _, actor := range pair.Actors {
		if actor.Game.GGOField != nil && ggoTradeLinkMatches(actor.Game.GGOField.Trade, state) {
			actor.Game.GGOField.Trade = nil
		}
	}
}

// A changed profile revision invalidates both confirmations and creates a new
// quote version, including when an unrelated safe-area settlement intervened.
func ggoTradeRefresh(tx *gorm.DB, row *db.GGOFieldReceipt, state *ggoTradeState, pair *ggoTradePair) (bool, error) {
	if state.Status != "invited" && state.Status != "open" {
		return false, nil
	}
	live, err := ggoTradeLive(tx, pair)
	if err != nil {
		return false, err
	}
	linked := pair.Actors[0].Game.GGOField != nil && pair.Actors[1].Game.GGOField != nil && ggoTradeLinkMatches(pair.Actors[0].Game.GGOField.Trade, state) && ggoTradeLinkMatches(pair.Actors[1].Game.GGOField.Trade, state)
	if pair.Now >= state.ExpiresAtMS || !live || !linked {
		state.Status, state.Reason = "cancelled", "location_or_session_changed"
		if pair.Now >= state.ExpiresAtMS {
			state.Status, state.Reason = "expired", "timeout"
		}
		state.Confirmed = [2]bool{}
		ggoTradeClearLinks(pair, state)
		if err = ggoTradeSave(tx, row, state); err != nil {
			return false, err
		}
		if err = ggoTradeSaveProfiles(tx, pair); err != nil {
			return false, err
		}
		return true, nil
	}
	current := [2]int64{pair.Actors[0].Game.GGOField.Revision, pair.Actors[1].Game.GGOField.Revision}
	if current != state.Revisions {
		if state.Version == math.MaxInt64 {
			return false, errors.New("交易版本溢出")
		}
		state.Version++
		state.Revisions = current
		state.Confirmed = [2]bool{}
		state.Reason = "balances_changed"
		if err = ggoTradeSave(tx, row, state); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

func ggoTradeView(state *ggoTradeState, pair *ggoTradePair, viewer int64) (*model.GGOFieldTradeView, error) {
	view := &model.GGOFieldTradeView{Status: "none", ServerTimeMS: pair.Now}
	index := 0
	if pair.Actors[1].Row.Id == viewer {
		index = 1
	}
	own := pair.Actors[index].Game
	if own.GGOField != nil {
		view.Terminal.Chips = own.GGOField.Chips
		view.Terminal.Revision = own.GGOField.Revision
		cfg := pair.Actors[index].Service.GGOFieldEconomy
		slots := int64(3)
		if cfg != nil && cfg.Runtime != nil {
			slots = cfg.Runtime.ProtectedChips
		}
		protected, _, err := ggoProtectedChips(own.GGOField.Chips, slots)
		if err != nil {
			return nil, err
		}
		view.Terminal.Protected = protected
	}
	view.Terminal.CreditBalance = ggoCreditBalance(own)
	if view.Terminal.CreditBalance < 0 {
		return nil, errors.New("信用余额无效")
	}
	if state == nil {
		return view, nil
	}
	view.TradeID, view.InitiatorID, view.ReceiverID = state.TradeID, state.InitiatorID, state.ReceiverID
	view.Status, view.Reason, view.Version = state.Status, state.Reason, state.Version
	view.Active = state.Status == "invited" || state.Status == "open"
	view.Offers, view.Confirmed, view.ExpiresAtMS = state.Offers, state.Confirmed, state.ExpiresAtMS
	states := [2]*model.GGOFieldState{pair.Actors[0].Game.GGOField, pair.Actors[1].Game.GGOField}
	for i := range states {
		if value, err := ggoTradeOfferValue(states[i], state.Offers[i]); err == nil {
			view.OfferCreditValues[i] = value
		}
	}
	if state.Status == "settled" {
		view.OfferCreditValues = state.SettledValues
		view.AfterChips, view.AfterProtected, view.PreviewValid = state.SettledChips[index], state.SettledProtected[index], true
	} else if states[0] != nil && states[1] != nil && state.Offers == ([2][6]int64{}) {
		view.AfterChips, view.AfterProtected, view.PreviewValid = view.Terminal.Chips, view.Terminal.Protected, true
	} else if states[0] != nil && states[1] != nil {
		cfg := pair.Actors[index].Service.GGOFieldEconomy
		allowGift, slots := false, int64(3)
		if cfg != nil && cfg.Runtime != nil {
			allowGift, slots = cfg.Rules.AllowGift != nil && *cfg.Rules.AllowGift, cfg.Runtime.ProtectedChips
		}
		after, err := ggoTradeAfter(states, state.Offers, allowGift)
		if err == nil {
			view.AfterChips = after[index].Chips
			view.AfterProtected, _, err = ggoProtectedChips(view.AfterChips, slots)
			view.PreviewValid = err == nil
		}
	}
	return view, nil
}

func ggoTradeNotify(s *GameService, ctx context.Context, ids ...int64) {
	for _, id := range ids {
		s.p10SceneSend(ctx, id, &pb.SCFieldResourceInfo{})
	}
}

func (s *GameService) GGOFieldTradeInvite(ctx context.Context, req model.GGOFieldTradeInviteRequest, session []byte) (*model.GGOFieldTradeView, error) {
	var view *model.GGOFieldTradeView
	changed := false
	err := s.ggoWithTradePair(ctx, req.GGOFieldAuth, session, req.PlayerID, req.TargetPlayerID, func(tx *gorm.DB, pair *ggoTradePair) error {
		hash := p6Digest([]byte(fmtTradeInvite(req.TargetPlayerID)))
		var old ggoTradeState
		if found, err := ggoReadReceipt(tx, req.PlayerID, req.RequestID, constUtil.GGOFieldOpTradeState, hash, &old); err != nil {
			return err
		} else if found {
			row, state, err := ggoTradeRead(tx, req.RequestID, req.PlayerID, req.TargetPlayerID)
			if err != nil {
				return err
			}
			changed, err = ggoTradeRefresh(tx, row, state, pair)
			if err != nil {
				return err
			}
			view, err = ggoTradeView(state, pair, req.PlayerID)
			return err
		}
		if err := ggoCheckClientBuildLocked(tx, req.PlayerID, session, pair.Actors[0].Service.GGOFieldConfig); err != nil {
			return err
		}
		live, err := ggoTradeLive(tx, pair)
		if err != nil {
			return err
		}
		if !live {
			return ggoFieldError(constUtil.GGOFieldInvalidLocation, "双方须在同线安全区并保持在线")
		}
		for _, actor := range pair.Actors {
			link := actor.Game.GGOField.Trade
			if link != nil && pair.Now < link.ExpiresAtMS {
				return ggoFieldError(constUtil.GGOFieldBusy, "已有交易进行中")
			}
			actor.Game.GGOField.Trade = nil
		}
		if pair.Now > math.MaxInt64-ggoTradeLifetimeMS {
			return errors.New("交易时间溢出")
		}
		state := &ggoTradeState{TradeID: req.RequestID, InitiatorID: req.PlayerID, ReceiverID: req.TargetPlayerID, Status: "invited", Version: 1, Revisions: [2]int64{pair.Actors[0].Game.GGOField.Revision, pair.Actors[1].Game.GGOField.Revision}, ExpiresAtMS: pair.Now + ggoTradeLifetimeMS}
		link := model.GGOFieldTradeLink{TradeID: state.TradeID, InitiatorID: state.InitiatorID, ReceiverID: state.ReceiverID, ExpiresAtMS: state.ExpiresAtMS}
		for _, actor := range pair.Actors {
			copy := link
			actor.Game.GGOField.Trade = &copy
		}
		if err = ggoTradeSaveProfiles(tx, pair); err != nil {
			return err
		}
		key := state.TradeID
		if err = ggoSaveReceipt(tx, req.PlayerID, req.RequestID, constUtil.GGOFieldOpTradeState, hash, "", &key, [6]int64{}, req.TargetPlayerID, pair.Now, state); err != nil {
			return err
		}
		if err = tx.Model(&db.GGOFieldReceipt{}).Where("player_id=? AND request_id=?", req.PlayerID, req.RequestID).Updates(map[string]any{"active": true, "ready_at": state.ExpiresAtMS}).Error; err != nil {
			return err
		}
		changed = true
		view, err = ggoTradeView(state, pair, req.PlayerID)
		return err
	})
	if err != nil {
		return nil, err
	}
	if changed {
		ggoTradeNotify(s, ctx, req.PlayerID, req.TargetPlayerID)
	}
	return view, nil
}

func fmtTradeInvite(target int64) string {
	raw, _ := json.Marshal(struct {
		Target int64 `json:"target"`
	}{target})
	return string(raw)
}

func (s *GameService) GGOFieldTradeStatus(ctx context.Context, req model.GGOFieldTradeStatusRequest, session []byte) (*model.GGOFieldTradeView, error) {
	var view *model.GGOFieldTradeView
	changed := false
	err := s.ggoWithTradePair(ctx, req.GGOFieldAuth, session, req.InitiatorID, req.ReceiverID, func(tx *gorm.DB, pair *ggoTradePair) error {
		row, state, err := ggoTradeRead(tx, req.TradeID, req.InitiatorID, req.ReceiverID)
		if err != nil {
			return err
		}
		changed, err = ggoTradeRefresh(tx, row, state, pair)
		if err != nil {
			return err
		}
		view, err = ggoTradeView(state, pair, req.PlayerID)
		return err
	})
	if err != nil {
		return nil, err
	}
	if changed {
		ggoTradeNotify(s, ctx, req.InitiatorID, req.ReceiverID)
	}
	return view, nil
}

func (s *GameService) GGOFieldTradeCurrent(ctx context.Context, auth model.GGOFieldAuth, session []byte) (*model.GGOFieldTradeView, error) {
	profile, err := AuthorizeAccount(ctx, auth.PlayerID, session)
	if err != nil {
		return nil, ggoClientAuthError(err)
	}
	if AdminProfileServerID(*profile) != s.OriginalServerID() {
		return nil, constUtil.ErrAdminPermission
	}
	_, game, err := decodeProfile(profile)
	if err != nil {
		return nil, err
	}
	if game.GGOField == nil || game.GGOField.Trade == nil {
		return &model.GGOFieldTradeView{Status: "none", ServerTimeMS: time.Now().UnixMilli()}, nil
	}
	link := game.GGOField.Trade
	return s.GGOFieldTradeStatus(ctx, model.GGOFieldTradeStatusRequest{GGOFieldAuth: auth, TradeID: link.TradeID, InitiatorID: link.InitiatorID, ReceiverID: link.ReceiverID}, session)
}
