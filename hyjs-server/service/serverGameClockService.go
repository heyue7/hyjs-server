package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// ServerGameClockContextBound never enables the legacy demo activity tools.
func ServerGameClockContextBound(ctx context.Context) bool {
	return ctx.Value(constUtil.ServerGameClockBoundKey) == true
}

// GameCalendarContextBound covers calendar accounting without granting demo-tool access.
func GameCalendarContextBound(ctx context.Context) bool {
	return ServerGameClockContextBound(ctx) || LocalPlayContextBound(ctx)
}

// GameCalendarOwner identifies the original authenticated owner of this request.
func GameCalendarOwner(ctx context.Context) int64 {
	if ServerGameClockContextBound(ctx) {
		id, _ := ctx.Value(constUtil.ServerGameClockOwnerKey).(int64)
		return id
	}
	id, _ := ctx.Value(constUtil.LocalPlayOwnerKey).(int64)
	return id
}

// GameCalendarOffsetDays selects the server offset before any historic personal offset.
func GameCalendarOffsetDays(ctx context.Context) int64 {
	if ServerGameClockContextBound(ctx) {
		days, _ := ctx.Value(constUtil.ServerGameClockOffsetKey).(int64)
		return days
	}
	days, _ := ctx.Value(constUtil.LocalPlayOffsetDaysKey).(int64)
	return days
}

func (s *GameService) serverGameClockID() int64 {
	if s.ServerID > 0 {
		return s.ServerID
	}
	return constUtil.ServerGameClockDefaultID
}

// InitializeServerGameClock inserts the zero-offset clock without altering existing server history.
func (s *GameService) InitializeServerGameClock(ctx context.Context) error {
	d := db.GetIns().WithContext(ctx)
	if err := d.AutoMigrate(&db.ServerGameClock{}, &db.ServerGameClockOperation{}, &db.ServerGameClockRecovery{}, &db.LocalPlayPaymentTime{}, &db.AdminAudit{}); err != nil {
		return err
	}
	row := db.ServerGameClock{ServerID: s.serverGameClockID()}
	if err := d.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
		return err
	}
	var persisted db.ServerGameClock
	if err := d.First(&persisted, "server_id=?", row.ServerID).Error; err != nil {
		return err
	}
	if !serverGameClockValid(&persisted) {
		return constUtil.ErrAdminConfiguration
	}
	s.cacheServerGameClock(serverGameClockView(&persisted, time.Now()))
	s.ServerClockReady = true
	return nil
}

func serverGameClockValid(row *db.ServerGameClock) bool {
	return row.ServerID > 0 && row.OffsetDays >= -constUtil.ServerGameClockMaxDays && row.OffsetDays <= constUtil.ServerGameClockMaxDays && row.Revision >= 0 && row.HighDays >= 0 && row.HighDays <= constUtil.ServerGameClockMaxDays && row.HighDays >= row.OffsetDays
}

func serverGameClockView(row *db.ServerGameClock, real time.Time) *model.ServerGameClockView {
	return &model.ServerGameClockView{Enabled: true, Scope: constUtil.ServerGameClockScope, ServerID: row.ServerID, GameTimeMS: real.AddDate(0, 0, int(row.OffsetDays)).UnixMilli(), RealTimeMS: real.UnixMilli(), OffsetDays: row.OffsetDays, Revision: row.Revision, Timezone: "Asia/Shanghai", TestOnly: true}
}

// cacheServerGameClock prevents an older completed request from replacing a newer committed revision.
func (s *GameService) cacheServerGameClock(view *model.ServerGameClockView) {
	for {
		prior := s.ServerClockState.Load()
		if prior != nil && prior.Revision > view.Revision {
			return
		}
		if s.ServerClockState.CompareAndSwap(prior, view) {
			return
		}
	}
}

// ServerGameClockAt supplies authenticated request snapshots and server-owned worker contexts.
func ServerGameClockAt(ctx context.Context, real time.Time) (time.Time, bool) {
	if ServerGameClockContextBound(ctx) {
		return real.AddDate(0, 0, int(GameCalendarOffsetDays(ctx))), true
	}
	if svc, ok := ctx.Value(constUtil.GameServiceContextKey).(*GameService); ok && svc.ServerClockReady {
		if state := svc.ServerClockState.Load(); state != nil {
			return real.AddDate(0, 0, int(state.OffsetDays)), true
		}
	}
	return real, false
}

// ServerGameClockGuardTransaction locks the shared clock before any Profile lock.
// Stale request snapshots fail atomically; the client can retry against the current server date.
func ServerGameClockGuardTransaction(tx *gorm.DB) error {
	ctx := tx.Statement.Context
	if !ServerGameClockContextBound(ctx) {
		return nil
	}
	serverID, _ := ctx.Value(constUtil.ServerGameClockServerKey).(int64)
	revision, _ := ctx.Value(constUtil.ServerGameClockRevisionKey).(int64)
	var state db.ServerGameClock
	if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).First(&state, "server_id=?", serverID).Error; err != nil {
		return err
	}
	if !serverGameClockValid(&state) {
		return constUtil.ErrAdminConfiguration
	}
	if state.Revision != revision {
		return fmt.Errorf("%w: 全服日期刚刚调整，请重试当前操作", constUtil.ErrAdminConfigConflict)
	}
	return nil
}

// BindServerGameClockContext is a read-only snapshot after original account authentication.
func (s *GameService) BindServerGameClockContext(ctx context.Context, owner *db.Profile) (context.Context, error) {
	if !s.ServerClockReady || owner == nil || owner.Id < 1 {
		return ctx, nil
	}
	var row db.ServerGameClock
	if err := db.GetIns().WithContext(ctx).First(&row, "server_id=?", s.serverGameClockID()).Error; err != nil {
		return ctx, err
	}
	if !serverGameClockValid(&row) {
		return ctx, constUtil.ErrAdminConfiguration
	}
	s.cacheServerGameClock(serverGameClockView(&row, time.Now()))
	ctx = context.WithValue(ctx, constUtil.ServerGameClockBoundKey, true)
	ctx = context.WithValue(ctx, constUtil.ServerGameClockOwnerKey, owner.Id)
	ctx = context.WithValue(ctx, constUtil.ServerGameClockServerKey, row.ServerID)
	ctx = context.WithValue(ctx, constUtil.ServerGameClockOffsetKey, row.OffsetDays)
	ctx = context.WithValue(ctx, constUtil.ServerGameClockRevisionKey, row.Revision)
	ctx = context.WithValue(ctx, constUtil.ServerGameClockHighDaysKey, row.HighDays)
	return ctx, nil
}

// Workers share the persisted calendar snapshot and transaction revision guard.
// No role identity or permission is created by this binding.
func (s *GameService) bindServerGameClockWorker75(ctx context.Context) (context.Context, bool, error) {
	if !s.ServerClockReady || ServerGameClockContextBound(ctx) {
		return ctx, false, nil
	}
	var row db.ServerGameClock
	if err := db.GetIns().WithContext(ctx).First(&row, "server_id=?", s.serverGameClockID()).Error; err != nil {
		return ctx, false, err
	}
	if !serverGameClockValid(&row) {
		return ctx, false, constUtil.ErrAdminConfiguration
	}
	ctx = context.WithValue(ctx, constUtil.ServerGameClockBoundKey, true)
	ctx = context.WithValue(ctx, constUtil.ServerGameClockServerKey, row.ServerID)
	ctx = context.WithValue(ctx, constUtil.ServerGameClockOffsetKey, row.OffsetDays)
	ctx = context.WithValue(ctx, constUtil.ServerGameClockRevisionKey, row.Revision)
	ctx = context.WithValue(ctx, constUtil.ServerGameClockHighDaysKey, row.HighDays)
	return ctx, true, nil
}

// AdminServerGameClock reads the same persistent calendar used by ordinary game requests.
func (s *GameService) AdminServerGameClock(ctx context.Context, p *model.AdminPrincipal) (*model.ServerGameClockView, error) {
	if !s.ServerClockReady {
		return nil, constUtil.ErrAdminConfiguration
	}
	if err := adminOperationUser(db.GetIns().WithContext(ctx), p, false); err != nil {
		return nil, err
	}
	var row db.ServerGameClock
	if err := db.GetIns().WithContext(ctx).First(&row, "server_id=?", s.serverGameClockID()).Error; err != nil {
		return nil, err
	}
	if !serverGameClockValid(&row) {
		return nil, constUtil.ErrAdminConfiguration
	}
	return serverGameClockView(&row, time.Now()), nil
}

// AdminChangeServerGameClock serializes the server calendar and refuses any active original battle.
func (s *GameService) AdminChangeServerGameClock(ctx context.Context, p *model.AdminPrincipal, req model.ServerGameClockChangeRequest) (*model.ServerGameClockView, error) {
	if !s.ServerClockReady {
		return nil, constUtil.ErrAdminConfiguration
	}
	var result *model.ServerGameClockView
	var committed *model.ServerGameClockView
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 先取得全服日期锁，再建立MySQL一致性快照，保证重试读取已提交回执。
		var row db.ServerGameClock
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "server_id=?", s.serverGameClockID()).Error; err != nil {
			return err
		}
		if err := adminOperationUser(tx, p, true); err != nil {
			return err
		}
		var actor db.AdminUser
		if err := tx.First(&actor, p.UserID).Error; err != nil {
			return err
		}
		if actor.Role != constUtil.AdminRoleAdmin {
			return constUtil.ErrAdminPermission
		}
		if !serverGameClockValid(&row) {
			return constUtil.ErrAdminConfiguration
		}
		var prior db.ServerGameClockOperation
		err := tx.First(&prior, "server_id=? AND request_id=?", row.ServerID, req.RequestID).Error
		if err == nil {
			if prior.ActorID != p.UserID || prior.DeltaDays != req.DeltaDays {
				return constUtil.ErrAdminConfigConflict
			}
			return json.Unmarshal(prior.Response, &result)
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if row.Revision != req.Revision {
			return constUtil.ErrAdminConfigConflict
		}
		if row.Revision == math.MaxInt64 {
			return constUtil.ErrAdminConfiguration
		}
		// Profile locks keep in-flight original battle creation ordered against this calendar change.
		var profiles []db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("player IS NOT NULL AND game IS NOT NULL AND (server_id=? OR server_id=0)", s.OriginalServerID()).Order("id").Find(&profiles).Error; err != nil {
			return err
		}
		for _, profile := range profiles {
			var game model.GameState
			if err := json.Unmarshal([]byte(*profile.Game), &game); err != nil {
				return err
			}
			if game.Run != nil {
				return fmt.Errorf("%w: 有玩家尚未结束战斗，请结束后调整全服日期", constUtil.ErrAdminConfigConflict)
			}
		}
		calendarReal := time.Now()
		if err := expireCalendarWanted75(tx, s.OriginalServerID(), calendarReal.AddDate(0, 0, int(row.OffsetDays)).UnixMilli()); err != nil {
			return err
		}
		if err := serverGameClockPendingBattles(tx, calendarReal, row.OffsetDays); err != nil {
			return err
		}
		if err := s.shiftCalendarDurationTimers75(tx, profiles, req.DeltaDays); err != nil {
			return err
		}
		row.OffsetDays += req.DeltaDays
		if row.OffsetDays < -constUtil.ServerGameClockMaxDays || row.OffsetDays > constUtil.ServerGameClockMaxDays {
			return fmt.Errorf("%w: 全服日期最多前后调整3650天", constUtil.ErrAdminInput)
		}
		real := time.Now()
		row.Revision++
		row.UpdatedAtMillis = real.UnixMilli()
		if row.OffsetDays > row.HighDays {
			row.HighDays = row.OffsetDays
			row.HighReachedAt = real.UnixMilli()
		}
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
		result = serverGameClockView(&row, real)
		committed = result
		raw, err := json.Marshal(result)
		if err != nil {
			return err
		}
		operation := db.ServerGameClockOperation{ServerID: row.ServerID, RequestID: req.RequestID, ActorID: p.UserID, DeltaDays: req.DeltaDays, Response: raw, CreatedAt: real.UnixMilli()}
		if err := tx.Create(&operation).Error; err != nil {
			return err
		}
		return adminAudit(tx, p, "server.clock", fmt.Sprint(row.ServerID), fmt.Sprintf("delta=%d; offset=%d; revision=%d; request=%s", req.DeltaDays, row.OffsetDays, row.Revision, req.RequestID))
	})
	if err == nil && committed != nil {
		s.cacheServerGameClock(committed)
	}
	return result, err
}

// ServerGameClockPrepareProfile runs after the existing Profile row lock and before game business logic.
// The recovery cursor and player update commit together; rewinding and re-entering old days give no extra recovery.
func ServerGameClockPrepareProfile(tx *gorm.DB, row *db.Profile, player *pb.Player) (bool, error) {
	ctx := tx.Statement.Context
	if !ServerGameClockContextBound(ctx) {
		return false, nil
	}
	if row == nil || player == nil || GameCalendarOwner(ctx) != row.Id {
		return false, constUtil.ErrAdminConfiguration
	}
	serverID, _ := ctx.Value(constUtil.ServerGameClockServerKey).(int64)
	revision, _ := ctx.Value(constUtil.ServerGameClockRevisionKey).(int64)
	var state db.ServerGameClock
	if err := tx.First(&state, "server_id=?", serverID).Error; err != nil {
		return false, err
	}
	if !serverGameClockValid(&state) {
		return false, constUtil.ErrAdminConfiguration
	}
	if state.Revision != revision {
		return false, constUtil.ErrAdminConfigConflict
	}
	target := max(int64(0), state.OffsetDays)
	var cursor db.ServerGameClockRecovery
	err := tx.First(&cursor, "server_id=? AND player_id=?", serverID, row.Id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		cursor = db.ServerGameClockRecovery{ServerID: serverID, PlayerID: row.Id}
		// Newly created players never receive recovery for server days before their creation.
		if row.CreatedAt != nil && *row.CreatedAt > state.HighReachedAt {
			cursor.HighDays = state.HighDays
		}
	} else if err != nil {
		return false, err
	}
	if cursor.HighDays < 0 || cursor.HighDays > constUtil.ServerGameClockMaxDays {
		return false, constUtil.ErrAdminConfiguration
	}
	if target <= cursor.HighDays {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, tx.Create(&cursor).Error
		}
		return false, nil
	}
	real := time.Now()
	changed, err := settleResourceRecovery(player, real.UnixMilli())
	if err != nil {
		return false, err
	}
	virtual := real.AddDate(0, 0, int(target-cursor.HighDays)).UnixMilli()
	energy, _ := recoverResourceValue(player.GetEnergy(), player.GetEnergyLastRefreshTime(), constUtil.EnergyRecoveryLimit, constUtil.EnergyRecoveryIntervalMS, virtual)
	stamina, _ := recoverResourceValue(player.GetStamina(), player.GetStaminaLastRefreshTime(), constUtil.StaminaRecoveryLimit, constUtil.StaminaRecoveryIntervalMS, virtual)
	if energy != player.GetEnergy() {
		player.Energy = proto.Int32(energy)
		player.EnergyLastRefreshTime = proto.Int64(real.UnixMilli())
		changed = true
	}
	if stamina != player.GetStamina() {
		player.Stamina = proto.Int32(stamina)
		player.StaminaLastRefreshTime = proto.Int64(real.UnixMilli())
		changed = true
	}
	cursor.HighDays = target
	cursor.UpdatedAtMillis = real.UnixMilli()
	if err := tx.Save(&cursor).Error; err != nil {
		return false, err
	}
	return changed, nil
}

// serverGameClockPendingBattles preserves independent accepted battle leases as well as Profile.Run.
// Expired or committed historical rows never permanently block server calendar operations.
func serverGameClockPendingBattles(tx *gorm.DB, real time.Time, offset int64) error {
	gameMS := real.AddDate(0, 0, int(offset)).UnixMilli()
	busy := func(q *gorm.DB) error {
		var count int64
		if err := q.Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return fmt.Errorf("%w: 有玩家尚未结束有效战斗，请结束后调整全服日期", constUtil.ErrAdminConfigConflict)
		}
		return nil
	}
	if gameSchemaHasTable(tx, &db.P12WantedFight{}) && gameSchemaHasTable(tx, &db.P12WantedInstance{}) {
		q := tx.Table("p12_wanted_fights AS fight").Joins("JOIN p12_wanted_instances AS instance ON instance.id=fight.instance_id").Where("fight.committed=? AND fight.abandoned_at=0 AND instance.state=? AND instance.created_at<=? AND instance.expires_at>? AND instance.current_hp>0", false, constUtil.P12WantedActive, gameMS, gameMS)
		if err := busy(q); err != nil {
			return err
		}
	}
	if gameSchemaHasTable(tx, &db.P12WorldBossBattleReceipt{}) && gameSchemaHasTable(tx, &db.P12WorldBossCycle{}) {
		q := tx.Table("p12_world_boss_battle_receipts AS fight").Joins("JOIN p12_world_boss_cycles AS cycle ON cycle.id=fight.instance_id").Where("fight.committed=? AND fight.abandoned_at=0 AND cycle.starts_at<=? AND cycle.ends_at>?", false, gameMS, gameMS)
		if err := busy(q); err != nil {
			return err
		}
	}
	// PK is a real six-second lease, independent from the adjusted business calendar.
	if gameSchemaHasTable(tx, &db.P12FieldPVPReceipt{}) {
		if err := busy(tx.Model(&db.P12FieldPVPReceipt{}).Where("busy_until>?", real.UnixMilli())); err != nil {
			return err
		}
	}
	// P11对手报价尚未接纳战斗，不阻挡调日；实际结算与日期变更共享事务锁序。
	if gameSchemaHasTable(tx, &db.P13UnionFightPlayerBattleAttempt{}) && gameSchemaHasTable(tx, &db.P13UnionFightMatch{}) {
		q := tx.Table("p13_union_fight_player_battle_attempts AS fight").Joins("JOIN p13_union_fight_matches AS battle ON battle.id=fight.match_id").Where("fight.status=? AND battle.start_at<=? AND battle.end_at>?", constUtil.P13UFPlayerBattlePending, gameMS, gameMS)
		if err := busy(q); err != nil {
			return err
		}
	}
	return nil
}
