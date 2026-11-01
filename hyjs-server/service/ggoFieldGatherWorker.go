package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/elog"
	"hyjs-server/model"
)

type ggoGatherTimer struct {
	token string
	timer *time.Timer
}

func (s *GameService) ggoCancelGatherFromAction(tx *gorm.DB, id int64, g *model.GameState, attempt *db.P8GatherAttempt, frozen *ggoGatherFrozen, requestID string) (*model.GGOFieldGatherCancelResponse, error) {
	return ggoCancelAttempt(tx, s, id, g, attempt, requestID, ggoCancelHash(frozen.StartRequestID, ""), time.Now().UnixMilli())
}

// ggoScheduleGather handles the normal deadline. The durable attempt remains
// authoritative; the recovery worker handles a process restart or missed timer.
func (s *GameService) ggoScheduleGather(ctx context.Context, id int64, token string, readyAt int64) {
	base := s.p10ClientOwner()
	task := &ggoGatherTimer{token: token}
	delay := time.Until(time.UnixMilli(readyAt))
	if delay < time.Millisecond {
		delay = time.Millisecond
	}
	task.timer = time.AfterFunc(delay, func() {
		if !base.ggoGatherTimers.CompareAndDelete(id, task) {
			return
		}
		callCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		if err := base.ggoFinishDue(callCtx, id, token); err != nil {
			elog.Warn("ggo gather deadline failed", zap.Error(err))
		}
	})
	if old, ok := base.ggoGatherTimers.Swap(id, task); ok {
		old.(*ggoGatherTimer).timer.Stop()
	}
}

func (s *GameService) ggoFinishDue(ctx context.Context, id int64, token string) error {
	ctx = p1GatherCatalogContext(context.WithValue(ctx, ggoGatherCurrentReadKey{}, true))
	snapshot, err := s.adminFieldRuntimeSnapshotForRequest(ctx, s.OriginalServerID())
	if err != nil {
		return err
	}
	runtime := s.withRuntimeTables(snapshot.tables)
	runtime.GGOFieldConfig = snapshot.ggo
	runtime.GGOFieldEconomy = snapshot.ggoEconomy
	runtime.GGOFieldRevision = snapshot.ggoRevision
	ctx = context.WithValue(ctx, constUtil.GameServiceContextKey, runtime)
	run := func(attemptCtx context.Context) error {
		database := db.GetIns()
		return database.WithContext(attemptCtx).Transaction(func(tx *gorm.DB) error {
			if err := ServerGameClockGuardTransaction(tx); err != nil {
				return err
			}
			var row db.Profile
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error; err != nil {
				return err
			}
			guarded, err := p1AcquireGatherCatalog(tx)
			if err != nil {
				return err
			}
			tx = guarded
			player, game, err := decodeProfileForCalendarUpdate(tx, &row)
			if err != nil {
				return err
			}
			var attempt db.P8GatherAttempt
			err = tx.First(&attempt, "player_id=? AND token=? AND active=?", id, token, true).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			if err != nil {
				return err
			}
			now := time.Now().UnixMilli()
			if now < attempt.ReadyAt {
				return nil
			}
			if !strings.Contains(attempt.Snapshot, "ggoStartRequestId") {
				return nil
			}
			frozen, err := ggoFrozenAttempt(&attempt)
			if err != nil {
				return err
			}
			actor := &ggoFieldActor{Service: runtime, Row: &row, Player: player, Game: game, Now: now}
			canFinish, err := ggoGatherCanFinish(tx, actor, &attempt, frozen)
			if err != nil {
				return err
			}
			if !canFinish {
				requestID := ggoSystemReceiptID("cancel", token)
				if _, err = ggoCancelAttempt(tx, runtime, id, game, &attempt, requestID, ggoCancelHash(frozen.StartRequestID, ""), now); err != nil {
					return err
				}
				return saveProfile(tx, &row, player, game)
			}
			_, _, err = ggoEndAttempt(tx, actor, &attempt, ggoSystemReceiptID("claim", token))
			return err
		}, ggoGatherTransactionOptions(database, attemptCtx)...)
	}
	shared := context.WithValue(ctx, constUtil.P1SharedReadContextKey{}, true)
	err = run(shared)
	if errors.Is(err, constUtil.ErrP1ExclusiveRetry) {
		// The attempted settlement and every associated write have rolled back.
		err = run(context.WithValue(ctx, constUtil.P1SharedReadContextKey{}, false))
	}
	return err
}

// ServeGGOFieldGather recovers due rows through the indexed bounded query.
func (s *GameService) ServeGGOFieldGather(ctx context.Context) error {
	if !s.ggoExtractionEnabled() {
		<-ctx.Done()
		return nil
	}
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	defer s.ggoStopNoticeTimers()
	seen := make(map[int64]int64)
	lastRecovery := int64(0)
	var observedDeployment string
	deploymentKnown := false
	warnings := make(map[string]*ggoWorkerWarning)
	warn := func(kind string, err error) {
		if err == nil {
			return
		}
		w := warnings[kind]
		if w == nil {
			w = &ggoWorkerWarning{}
			warnings[kind] = w
		}
		if time.Since(w.last) < 30*time.Second {
			w.suppressed++
			return
		}
		elog.Warn("ggo background recovery failed", zap.String("kind", kind), zap.Int64("suppressed", w.suppressed), zap.Error(err))
		w.last, w.suppressed = time.Now(), 0
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			now := time.Now().UnixMilli()
			warn("line recovery", s.ggoRecoverLines(ctx, now))
			if snapshot := s.p10ClientOwner().adminFieldRuntimeCache.Load(); snapshot != nil && snapshot.serverID == s.OriginalServerID() {
				if deploymentKnown && snapshot.ggoRevision != observedDeployment {
					s.ggoNotifyFieldChange(ctx, 0)
				}
				deploymentKnown, observedDeployment = true, snapshot.ggoRevision
			}
			warn("line notification", s.ggoObserveLineChanges(ctx, seen))
			if now-lastRecovery < 1000 {
				continue
			}
			lastRecovery = now
			var attempts []db.P8GatherAttempt
			if err := db.GetIns().WithContext(ctx).Select("player_id", "token", "snapshot").Where("scene=? AND active=? AND ready_at<=?", constUtil.GGOFieldScene, true, now).Order("ready_at").Limit(64).Find(&attempts).Error; err != nil {
				warn("gather query", err)
			}
			for _, attempt := range attempts {
				var frozen ggoGatherFrozen
				if json.Unmarshal([]byte(attempt.Snapshot), &frozen) != nil || frozen.StartRequestID == "" {
					continue
				}
				if err := s.ggoFinishDue(ctx, attempt.PlayerID, attempt.Token); err != nil {
					warn("gather settlement", err)
				}
			}
			if err := s.ggoRecoverExtractions(ctx, now); err != nil {
				warn("extraction", err)
			}
			if err := s.ggoRecoverTrades(ctx, now); err != nil {
				warn("trade", err)
			}
		}
	}
}
