package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
)

// ActivityRechargeCarryFact retains both the original calendar date and the
// restored eligibility date. An EffectiveAt of zero retains audit history only.
type ActivityRechargeCarryFact struct {
	OrderID     string `json:"orderId"`
	Cents       int64  `json:"cents"`
	OriginalAt  int64  `json:"originalAt"`
	EffectiveAt int64  `json:"effectiveAt"`
}

// PrepareActivityRechargeCarry reads the old accepted order set before any date
// mutation. The caller holds clock/profile/period locks in the restore transaction.
// oldNow is the pre-restore business clock; realCutoff is the frozen real instant.
func PrepareActivityRechargeCarry(tx *gorm.DB, playerID int64, before db.PeriodInstance, requestID string, oldNow, realCutoff, openingAt int64) (*db.ActivityRechargeCarry, error) {
	if tx == nil || tx.Statement == nil {
		return nil, constUtil.ErrAdminConfiguration
	}
	if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return nil, fmt.Errorf("recharge carry preparation requires caller transaction: %w", constUtil.ErrAdminConfiguration)
	}
	if playerID <= 0 || before.ID == "" || before.Kind != "payment_amount" || requestID == "" || len(requestID) > 128 || oldNow <= 0 || realCutoff <= 0 || openingAt <= 0 {
		return nil, constUtil.ErrAdminInput
	}
	if before.Mode != "cumulative" && before.Mode != "single" && before.Mode != "daily" {
		return nil, constUtil.ErrAdminConfiguration
	}
	current := tx.Clauses(clause.Locking{Strength: "SHARE"}).Session(&gorm.Session{})
	baselineOrders, err := openingCalendarBoundedPayments(current.Where("player_id=? AND status=?", playerID, constUtil.P3PaymentConfirmed).Order("id"))
	if err != nil {
		return nil, err
	}
	baseline := make([]ActivityRechargeCarryFact, 0, len(baselineOrders))
	baselineIDs := map[string]db.PaymentOrder{}
	for _, order := range baselineOrders {
		if order.ID == "" || order.Cents <= 0 || order.ConfirmedAt <= 0 {
			return nil, constUtil.ErrAdminConfiguration
		}
		baselineIDs[order.ID] = order
		baseline = append(baseline, ActivityRechargeCarryFact{OrderID: order.ID, Cents: order.Cents, OriginalAt: order.ConfirmedAt})
	}
	orders, err := localPlayActivityPaymentOrders(current, playerID, before.ID, before.StartAt, min(before.EndAt, oldNow+1))
	if err != nil {
		return nil, err
	}
	facts := make([]ActivityRechargeCarryFact, 0, len(orders))
	activeDay := p1Day(time.UnixMilli(oldNow))
	// After the old window closes, its last recorded day remains the original
	// daily consumer's frozen qualification. Within a window only today is valid.
	if oldNow >= before.EndAt && len(orders) > 0 {
		activeDay = p1Day(time.UnixMilli(orders[len(orders)-1].ConfirmedAt))
	}
	for _, order := range orders {
		original, exists := baselineIDs[order.ID]
		if !exists || original.Cents != order.Cents || order.PlayerID != playerID || order.Cents <= 0 || order.Status != constUtil.P3PaymentConfirmed {
			return nil, constUtil.ErrAdminConfiguration
		}
		effective := openingAt
		if before.Mode == "daily" && p1Day(time.UnixMilli(order.ConfirmedAt)) != activeDay {
			effective = 0
		}
		facts = append(facts, ActivityRechargeCarryFact{order.ID, order.Cents, order.ConfirmedAt, effective})
	}
	sort.Slice(facts, func(i, j int) bool { return facts[i].OrderID < facts[j].OrderID })
	raw, err := json.Marshal(facts)
	if err != nil {
		return nil, err
	}
	row := &db.ActivityRechargeCarry{InstanceID: before.ID, PlayerID: playerID, RequestID: requestID, RealCutoff: realCutoff, OpeningAt: openingAt, Mode: before.Mode, Facts: raw, Baseline: adminJSON(baseline)}
	row.Digest = activityRechargeCarryDigest(*row)
	return row, nil
}

func activityRechargeCarryDigest(row db.ActivityRechargeCarry) string {
	row.Digest = ""
	return p6Digest(adminJSON(row))
}

// SaveActivityRechargeCarry joins the restore transaction and rejects replacing
// an earlier restore's entitlement. Same-request retries require identical input.
func SaveActivityRechargeCarry(tx *gorm.DB, row *db.ActivityRechargeCarry) error {
	if tx == nil || tx.Statement == nil {
		return constUtil.ErrAdminConfiguration
	}
	if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return fmt.Errorf("recharge carry save requires caller transaction: %w", constUtil.ErrAdminConfiguration)
	}
	if row == nil || len(row.Baseline) == 0 || row.Digest != activityRechargeCarryDigest(*row) {
		return constUtil.ErrAdminConfiguration
	}
	var prior db.ActivityRechargeCarry
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("instance_id=? AND player_id=?", row.InstanceID, row.PlayerID).Take(&prior).Error
	if err == nil {
		if prior.Digest == row.Digest && prior.RequestID == row.RequestID {
			return nil
		}
		return constUtil.ErrAdminConfigConflict
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return tx.Create(row).Error
}

// rechargeCarryValue is shared by direct activities and reward-center eligibility
// through rechargePeriodValue75. New orders use immutable real confirmation time.
func rechargeCarryValue(tx *gorm.DB, playerID int64, cfg db.PeriodInstance, now int64) (int64, string, bool, error) {
	if !gameSchemaHasTable(tx, &db.ActivityRechargeCarry{}) {
		return 0, "", false, nil
	}
	var row db.ActivityRechargeCarry
	err := tx.Where("instance_id=? AND player_id=?", cfg.ID, playerID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, "", false, nil
	}
	if err != nil {
		return 0, "", true, err
	}
	if row.Mode != cfg.Mode || row.RealCutoff <= 0 || row.OpeningAt <= 0 || len(row.Baseline) == 0 || row.Digest != activityRechargeCarryDigest(row) {
		return 0, "", true, constUtil.ErrAdminConfiguration
	}
	var facts []ActivityRechargeCarryFact
	if json.Unmarshal(row.Facts, &facts) != nil {
		return 0, "", true, constUtil.ErrAdminConfiguration
	}
	realStart, err := rechargeActivityRealStart75(tx, playerID, cfg.ID)
	if err != nil {
		return 0, "", true, err
	}
	var orders []db.PaymentOrder
	// Keep the real cutoff separate from the immutable business-calendar projection.
	// A restored calendar may be days behind the real confirmation instant.
	query := tx.Model(&db.PaymentOrder{}).Where("player_id=? AND status=? AND confirmed_at>=? AND confirmed_at>=? AND confirmed_at<?", playerID, constUtil.P3PaymentConfirmed, max(row.RealCutoff, realStart), cfg.StartAt, min(cfg.EndAt, now+1)).Order("confirmed_at,id")
	if gameSchemaHasTable(tx, &db.LocalPlayPaymentTime{}) {
		query = tx.Table("p3_payment_orders AS pay").Select("pay.*, COALESCE(play.game_at,pay.confirmed_at) AS confirmed_at").Joins("LEFT JOIN local_play_payment_times AS play ON play.order_id=pay.id AND play.player_id=pay.player_id").Where("pay.player_id=? AND pay.status=? AND pay.confirmed_at>=? AND COALESCE(play.game_at,pay.confirmed_at)>=? AND COALESCE(play.game_at,pay.confirmed_at)<?", playerID, constUtil.P3PaymentConfirmed, max(row.RealCutoff, realStart), cfg.StartAt, min(cfg.EndAt, now+1)).Order("COALESCE(play.game_at,pay.confirmed_at),pay.id")
	}
	err = query.Find(&orders).Error
	if err != nil {
		return 0, "", true, err
	}
	value, day, err := activityRechargeCarryValue(row, cfg, facts, orders, now)
	return value, day, true, err
}

// orders have already passed the immutable real cutoff; ConfirmedAt is business time.
func activityRechargeCarryValue(row db.ActivityRechargeCarry, cfg db.PeriodInstance, facts []ActivityRechargeCarryFact, orders []db.PaymentOrder, now int64) (int64, string, error) {
	var baseline []ActivityRechargeCarryFact
	baselineIDs := map[string]ActivityRechargeCarryFact{}
	if len(row.Baseline) > 0 {
		if json.Unmarshal(row.Baseline, &baseline) != nil {
			return 0, "", constUtil.ErrAdminConfiguration
		}
		for _, fact := range baseline {
			if fact.OrderID == "" || fact.Cents <= 0 || fact.OriginalAt <= 0 || fact.EffectiveAt != 0 || baselineIDs[fact.OrderID].OrderID != "" {
				return 0, "", constUtil.ErrAdminConfiguration
			}
			baselineIDs[fact.OrderID] = fact
		}
	}
	seen := map[string]int64{}
	accepted := make([]ActivityRechargeCarryFact, 0, len(facts)+len(orders))
	for _, fact := range facts {
		if fact.OrderID == "" || fact.Cents <= 0 || fact.OriginalAt <= 0 || fact.EffectiveAt < 0 || seen[fact.OrderID] != 0 {
			return 0, "", constUtil.ErrAdminConfiguration
		}
		if len(row.Baseline) > 0 {
			original, ok := baselineIDs[fact.OrderID]
			if !ok || original.Cents != fact.Cents {
				return 0, "", constUtil.ErrAdminConfiguration
			}
		}
		seen[fact.OrderID] = fact.Cents
		if fact.EffectiveAt > 0 && fact.EffectiveAt <= now {
			accepted = append(accepted, fact)
		}
	}
	for _, order := range orders {
		if order.ID == "" || order.PlayerID != row.PlayerID || order.Status != constUtil.P3PaymentConfirmed || order.Cents <= 0 {
			return 0, "", constUtil.ErrAdminConfiguration
		}
		if original, ok := baselineIDs[order.ID]; ok {
			if original.Cents != order.Cents {
				return 0, "", constUtil.ErrAdminConfiguration
			}
			continue
		}
		if cents := seen[order.ID]; cents != 0 {
			if cents != order.Cents {
				return 0, "", constUtil.ErrAdminConfiguration
			}
			continue
		}
		seen[order.ID] = order.Cents
		if order.ConfirmedAt < cfg.StartAt || order.ConfirmedAt >= min(cfg.EndAt, now+1) {
			continue
		}
		accepted = append(accepted, ActivityRechargeCarryFact{order.ID, order.Cents, order.ConfirmedAt, order.ConfirmedAt})
	}
	sort.Slice(accepted, func(i, j int) bool {
		if accepted[i].EffectiveAt == accepted[j].EffectiveAt {
			return accepted[i].OrderID < accepted[j].OrderID
		}
		return accepted[i].EffectiveAt < accepted[j].EffectiveAt
	})
	var value int64
	day := ""
	for _, fact := range accepted {
		atDay := p1Day(time.UnixMilli(fact.EffectiveAt))
		if cfg.Mode == "daily" && day != atDay {
			value = 0
		}
		if cfg.Mode == "single" {
			value = max(value, fact.Cents)
		} else {
			if value > math.MaxInt64-fact.Cents {
				return 0, "", constUtil.ErrAdminConfiguration
			}
			value += fact.Cents
		}
		day = atDay
	}
	if cfg.Mode == "daily" && now < cfg.EndAt && day != p1Day(time.UnixMilli(now)) {
		return 0, p1Day(time.UnixMilli(now)), nil
	}
	return value, day, nil
}
