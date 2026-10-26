package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// LocalPlayContextBound is true only after the server has authenticated and
// explicitly opted this demo character into the local play tools.
func LocalPlayContextBound(ctx context.Context) bool {
	return ctx.Value(constUtil.LocalPlayBoundKey) == true
}

// LocalPlayAt applies a calendar offset only to an already trusted context.
// Accounting clocks, session expiry and transport deadlines keep wall time.
func LocalPlayAt(ctx context.Context, real time.Time) time.Time {
	if shifted, ok := ServerGameClockAt(ctx, real); ok {
		return shifted
	}
	if !LocalPlayContextBound(ctx) {
		return real
	}
	days, _ := ctx.Value(constUtil.LocalPlayOffsetDaysKey).(int64)
	return real.AddDate(0, 0, int(days))
}

func LocalPlayNow(ctx context.Context) time.Time { return LocalPlayAt(ctx, time.Now()) }

func (s *GameService) InitializeLocalPlay(ctx context.Context) error {
	d := db.GetIns().WithContext(ctx)
	if err := d.AutoMigrate(&db.LocalPlayPolicy{}, &db.LocalPlayClockState{}, &db.LocalPlayClockOperation{}, &db.LocalPlayPaymentTime{}); err != nil {
		return err
	}
	return d.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.LocalPlayPolicy{ID: 1, Enabled: false, Version: constUtil.LocalPlayVersion, Source: constUtil.LocalPlaySource}).Error
}

func localPlayPolicyEnabled(tx *gorm.DB) (bool, error) {
	var policy db.LocalPlayPolicy
	if err := tx.First(&policy, 1).Error; err != nil {
		return false, err
	}
	if policy.Version != constUtil.LocalPlayVersion || policy.Source != constUtil.LocalPlaySource {
		return false, errors.New(constUtil.LocalPlayInvalidMessage)
	}
	return policy.Enabled, nil
}

// BindLocalPlayContext runs after original session authorization. Other users
// and default-disabled deployments keep their original clock and activity path.
func (s *GameService) BindLocalPlayContext(ctx context.Context, owner *db.Profile) (context.Context, error) {
	if s.ServerClockReady {
		return s.BindServerGameClockContext(ctx, owner)
	}
	if !s.LocalPlayToolsEnabled || !localWalletOwner(owner) {
		return ctx, nil
	}
	d := db.GetIns().WithContext(ctx)
	enabled, err := localPlayPolicyEnabled(d)
	if err != nil || !enabled {
		return ctx, err
	}
	var state db.LocalPlayClockState
	err = d.First(&state, "player_id=?", owner.Id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		state = db.LocalPlayClockState{PlayerID: owner.Id}
	} else if err != nil {
		return ctx, err
	}
	if state.OffsetDays < -3650 || state.OffsetDays > 3650 || state.Revision < 0 {
		return ctx, errors.New(constUtil.LocalPlayInvalidMessage)
	}
	ctx = context.WithValue(ctx, constUtil.LocalPlayBoundKey, true)
	ctx = context.WithValue(ctx, constUtil.LocalPlayOwnerKey, owner.Id)
	ctx = context.WithValue(ctx, constUtil.LocalPlayOffsetDaysKey, state.OffsetDays)
	return ctx, nil
}

func localPlayTimeView(state *db.LocalPlayClockState, real time.Time) *model.LocalPlayTimeView {
	return &model.LocalPlayTimeView{GameTimeMS: real.AddDate(0, 0, int(state.OffsetDays)).UnixMilli(), RealTimeMS: real.UnixMilli(), OffsetDays: state.OffsetDays, Revision: state.Revision, Timezone: "Asia/Shanghai", TestOnly: true}
}

func localPlayAPIAuthorized(ctx context.Context, owner *db.Profile) bool {
	id, _ := ctx.Value(constUtil.LocalPlayOwnerKey).(int64)
	return LocalPlayContextBound(ctx) && ctx.Value(constUtil.LocalWalletAPIContextKey) == true && localWalletOwner(owner) && owner.Id == id && localWalletSessionAllowed(ctx, owner)
}

func (s *GameService) LocalPlayTimeState(ctx context.Context, id int64) (*model.LocalPlayTimeView, error) {
	if !s.LocalPlayToolsEnabled || !LocalPlayContextBound(ctx) {
		return nil, errors.New(constUtil.LocalPlayDisabledMessage)
	}
	var view *model.LocalPlayTimeView
	err := s.p3Profile(ctx, id, func(tx *gorm.DB, owner *db.Profile, _ *pb.Player, _ *model.GameState) error {
		if !localPlayAPIAuthorized(ctx, owner) {
			return errors.New(constUtil.LocalPlayInvalidMessage)
		}
		enabled, err := localPlayPolicyEnabled(tx)
		if err != nil {
			return err
		}
		if !enabled {
			return errors.New(constUtil.LocalPlayDisabledMessage)
		}
		var state db.LocalPlayClockState
		err = tx.First(&state, "player_id=?", id).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			state = db.LocalPlayClockState{PlayerID: id}
		} else if err != nil {
			return err
		}
		view = localPlayTimeView(&state, time.Now())
		return nil
	})
	return view, err
}

func (s *GameService) LocalPlayTimeChange(ctx context.Context, id int64, action, requestID string) (*model.LocalPlayTimeView, error) {
	return s.localPlayTimeChange(ctx, id, action, requestID, nil, nil)
}

// localPlayTimeChange 共用原日历事务；管理员变更同事务鉴权与审计。
func (s *GameService) localPlayTimeChange(ctx context.Context, id int64, action, requestID string, expected *int64, actor *model.AdminPrincipal) (*model.LocalPlayTimeView, error) {
	if !s.LocalPlayToolsEnabled || !LocalPlayContextBound(ctx) {
		return nil, errors.New(constUtil.LocalPlayDisabledMessage)
	}
	var result *model.LocalPlayTimeView
	err := s.p3Profile(ctx, id, func(tx *gorm.DB, owner *db.Profile, player *pb.Player, game *model.GameState) error {
		if actor != nil {
			if err := adminOperationUser(tx, actor, true); err != nil {
				return err
			}
			if !localWalletOwner(owner) {
				return constUtil.ErrAdminPermission
			}
		} else if !localPlayAPIAuthorized(ctx, owner) {
			return errors.New(constUtil.LocalPlayInvalidMessage)
		}
		enabled, err := localPlayPolicyEnabled(tx)
		if err != nil {
			return err
		}
		if !enabled {
			return errors.New(constUtil.LocalPlayDisabledMessage)
		}
		var prior db.LocalPlayClockOperation
		err = tx.First(&prior, "player_id=? AND request_id=?", id, requestID).Error
		if err == nil {
			if prior.Action != action {
				return errors.New(constUtil.LocalPlayConflictMessage)
			}
			return json.Unmarshal(prior.Response, &result)
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if game.Run != nil {
			return errors.New(constUtil.LocalPlayBusyMessage)
		}
		state := db.LocalPlayClockState{PlayerID: id}
		if err = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&state).Error; err != nil {
			return err
		}
		if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&state, "player_id=?", id).Error; err != nil {
			return err
		}
		if expected != nil && state.Revision != *expected {
			return constUtil.ErrAdminConfigConflict
		}
		beforeDays := state.OffsetDays
		switch action {
		case "previousDay":
			state.OffsetDays--
		case "nextDay":
			state.OffsetDays++
		case "reset":
			state.OffsetDays = 0
		default:
			return errors.New(constUtil.LocalPlayInvalidMessage)
		}
		if state.OffsetDays < -3650 || state.OffsetDays > 3650 {
			return errors.New(constUtil.LocalPlayRangeMessage)
		}
		if state.Revision == math.MaxInt64 {
			return errors.New(constUtil.LocalPlayInvalidMessage)
		}
		real := time.Now()
		recovered, err := applyLocalPlayRecoveryJump(tx, player, id, beforeDays, state.OffsetDays, real)
		if err != nil {
			return err
		}
		if recovered {
			if err = saveProfile(tx, owner, player, game); err != nil {
				return err
			}
		}
		state.Revision++
		state.UpdatedAt = real.UnixMilli()
		if err = tx.Save(&state).Error; err != nil {
			return err
		}
		result = localPlayTimeView(&state, real)
		data, err := json.Marshal(result)
		if err != nil {
			return err
		}
		if err = tx.Create(&db.LocalPlayClockOperation{PlayerID: id, RequestID: requestID, Action: action, Response: data, CreatedAt: real.UnixMilli()}).Error; err != nil {
			return err
		}
		if actor != nil {
			return adminAudit(tx, actor, "player.clock", strconv.FormatInt(id, 10), fmt.Sprintf("%s; %d → %d; request=%s", action, beforeDays, state.OffsetDays, requestID))
		}
		return nil
	})
	return result, err
}

// localPlayRecordPaymentTime preserves accounting's wall clock and records a
// separate activity-calendar projection inside the same original payment tx.
func localPlayRecordPaymentTime(tx *gorm.DB, order *db.PaymentOrder, real time.Time) error {
	ctx := tx.Statement.Context
	if order == nil || order.ID == "" || order.PlayerID <= 0 || real.UnixMilli() <= 0 || order.ConfirmedAt != real.UnixMilli() {
		return errors.New(constUtil.LocalPlayInvalidMessage)
	}
	if GameCalendarContextBound(ctx) && GameCalendarOwner(ctx) != order.PlayerID {
		return errors.New(constUtil.LocalPlayInvalidMessage)
	}
	game := LocalPlayAt(ctx, real)
	days := int64(game.Sub(real) / (24 * time.Hour))
	if GameCalendarContextBound(ctx) {
		days = GameCalendarOffsetDays(ctx)
	}
	// Freeze zero-offset confirmations too. A later clock binding or cold worker
	// must never reinterpret the same paid order using a different calendar.
	projection := db.LocalPlayPaymentTime{OrderID: order.ID, PlayerID: order.PlayerID, GameAt: game.UnixMilli(), RealAt: real.UnixMilli(), OffsetDays: days}
	return tx.Create(&projection).Error
}

// localPlayPaymentOrders is a read-only business-calendar projection. Returning
// projected ConfirmedAt values never rewrites the persisted payment orders.
func localPlayPaymentOrders(tx *gorm.DB, id, start, end int64) ([]db.PaymentOrder, error) {
	var orders []db.PaymentOrder
	if !gameSchemaHasTable(tx, &db.LocalPlayPaymentTime{}) {
		err := tx.Where("player_id=? AND status=? AND confirmed_at>=? AND confirmed_at<?", id, constUtil.P3PaymentConfirmed, start, end).Order("confirmed_at,id").Find(&orders).Error
		return orders, err
	}
	d := tx.Table("p3_payment_orders AS pay").Select("pay.*").Joins("LEFT JOIN local_play_payment_times AS play ON play.order_id=pay.id AND play.player_id=pay.player_id")
	err := d.Where("pay.player_id=? AND pay.status=? AND COALESCE(play.game_at,pay.confirmed_at)>=? AND COALESCE(play.game_at,pay.confirmed_at)<?", id, constUtil.P3PaymentConfirmed, start, end).Order("COALESCE(play.game_at,pay.confirmed_at),pay.id").Find(&orders).Error
	if err != nil || len(orders) == 0 {
		return orders, err
	}
	ids := make([]string, 0, len(orders))
	for _, order := range orders {
		ids = append(ids, order.ID)
	}
	var projections []db.LocalPlayPaymentTime
	if err = tx.Where("player_id=? AND order_id IN ?", id, ids).Find(&projections).Error; err != nil {
		return nil, err
	}
	at := map[string]int64{}
	for _, projection := range projections {
		at[projection.OrderID] = projection.GameAt
	}
	for i := range orders {
		if gameAt, ok := at[orders[i].ID]; ok {
			orders[i].ConfirmedAt = gameAt
		}
	}
	return orders, nil
}
