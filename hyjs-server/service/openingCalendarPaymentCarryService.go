package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
)

// OpeningCalendarPaymentFact retains immutable money and both original clocks.
type OpeningCalendarPaymentFact struct {
	OrderID string `json:"orderId"`
	Cents   int64  `json:"cents"`
	RealAt  int64  `json:"realAt"`
	GameAt  int64  `json:"gameAt"`
}

type openingCalendarPaymentCandidate struct {
	ID             string
	PlayerID       int64
	Status         string
	Cents          int64
	RealAt         int64
	GameAt         int64
	ProjectionReal int64
}

func openingCalendarPaymentScope(tx *gorm.DB, playerID int64) (string, error) {
	if scope := adminOpeningPeriod(tx); scope != nil {
		if scope.PlayerID != playerID || scope.OccurrenceID == "" || len(scope.OccurrenceID) > 100 {
			return "", constUtil.ErrAdminConfiguration
		}
		return "occurrence:" + scope.OccurrenceID, nil
	}
	cycle, err := localPlayOpenServerCycle(tx)
	if err != nil {
		return "", err
	}
	if cycle != nil {
		if cycle.PlayerID != playerID || cycle.ID == "" || len(cycle.ID) > 100 {
			return "", constUtil.ErrAdminConfiguration
		}
		return "cycle:" + cycle.ID, nil
	}
	return "lifetime", nil
}

func openingCalendarPaymentTransaction(tx *gorm.DB) error {
	if tx == nil || tx.Statement == nil {
		return constUtil.ErrAdminConfiguration
	}
	if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return fmt.Errorf("opening payment carry requires caller transaction: %w", constUtil.ErrAdminConfiguration)
	}
	return nil
}

// Limit before materializing money history; exceeding the safety bound rejects
// the whole operation rather than silently truncating qualification or identity.
func openingCalendarBoundedPayments(query *gorm.DB) ([]db.PaymentOrder, error) {
	const maximumOrders = 10000
	var orders []db.PaymentOrder
	if err := query.Limit(maximumOrders + 1).Find(&orders).Error; err != nil {
		return nil, err
	}
	if len(orders) > maximumOrders {
		return nil, constUtil.ErrAdminConfiguration
	}
	return orders, nil
}

// PrepareOpeningCalendarPaymentCarry runs before the caller mutates dates, in
// its clock/profile/scope-locked transaction. The full confirmed-ID baseline
// excludes old out-of-window orders even when windows overlap or clocks repeat.
func PrepareOpeningCalendarPaymentCarry(tx *gorm.DB, playerID int64, before, after db.OpenServerActivityRule, requestID string, oldNow, realCutoff int64) (*db.OpeningCalendarPaymentCarry, error) {
	if err := openingCalendarPaymentTransaction(tx); err != nil {
		return nil, err
	}
	if playerID <= 0 || before.ID == "" || before.ID != after.ID || requestID == "" || len(requestID) > 128 || oldNow <= 0 || oldNow == math.MaxInt64 || realCutoff <= 0 {
		return nil, constUtil.ErrAdminInput
	}
	row := &db.OpeningCalendarPaymentCarry{PlayerID: playerID, RuleID: before.ID, RequestID: requestID, RealCutoff: realCutoff,
		BeforeStartAt: before.StartAt, BeforeEndAt: before.EndAt, BeforeRewardAt: before.RewardEndAt,
		AfterStartAt: after.StartAt, AfterEndAt: after.EndAt, AfterRewardAt: after.RewardEndAt}
	if !openingCalendarPaymentWindowsValid(*row) {
		return nil, constUtil.ErrAdminConfiguration
	}
	scope, err := openingCalendarPaymentScope(tx, playerID)
	if err != nil {
		return nil, err
	}
	row.ScopeID = scope
	// Current reads retain payments committed before the caller acquired its
	// profile lock even if an earlier maintenance read opened an RR snapshot.
	current := tx.Clauses(clause.Locking{Strength: "SHARE"}).Session(&gorm.Session{})
	baselineOrders, err := openingCalendarBoundedPayments(current.Select("id", "player_id", "status", "cents", "confirmed_at").Where("player_id=? AND status=?", playerID, constUtil.P3PaymentConfirmed).Order("id"))
	if err != nil {
		return nil, err
	}
	baseline := make([]OpeningCalendarPaymentFact, 0, len(baselineOrders))
	byID := map[string]db.PaymentOrder{}
	for _, order := range baselineOrders {
		if order.ID == "" || order.Cents <= 0 || order.ConfirmedAt <= 0 {
			return nil, constUtil.ErrAdminConfiguration
		}
		byID[order.ID] = order
		baseline = append(baseline, OpeningCalendarPaymentFact{OrderID: order.ID, Cents: order.Cents, RealAt: order.ConfirmedAt})
	}
	paymentStart := before.StartAt
	cycle, err := localPlayOpenServerCycle(tx)
	if err != nil {
		return nil, err
	}
	if cycle != nil {
		paymentStart = max(paymentStart, cycle.CreatedAt)
	}
	orders, err := localPlayActivityPaymentOrders(current, playerID, before.ID, paymentStart, min(before.EndAt, oldNow+1))
	if err != nil {
		return nil, err
	}
	facts := make([]OpeningCalendarPaymentFact, 0, len(orders))
	for _, order := range orders {
		real, ok := byID[order.ID]
		if !ok || real.Cents != order.Cents || order.PlayerID != playerID || order.Status != constUtil.P3PaymentConfirmed {
			return nil, constUtil.ErrAdminConfiguration
		}
		facts = append(facts, OpeningCalendarPaymentFact{order.ID, order.Cents, real.ConfirmedAt, order.ConfirmedAt})
	}
	sort.Slice(facts, func(i, j int) bool { return facts[i].OrderID < facts[j].OrderID })
	row.Facts, err = json.Marshal(facts)
	if err != nil {
		return nil, err
	}
	row.Baseline, err = json.Marshal(baseline)
	if err != nil {
		return nil, err
	}
	row.Digest = openingCalendarPaymentDigest(*row)
	return row, nil
}

func openingCalendarPaymentWindowsValid(row db.OpeningCalendarPaymentCarry) bool {
	return row.BeforeStartAt > 0 && row.BeforeEndAt > row.BeforeStartAt && row.BeforeRewardAt >= row.BeforeEndAt &&
		row.AfterStartAt > 0 && row.AfterEndAt > row.AfterStartAt && row.AfterRewardAt >= row.AfterEndAt &&
		row.BeforeEndAt-row.BeforeStartAt == row.AfterEndAt-row.AfterStartAt
}

func openingCalendarPaymentDigest(row db.OpeningCalendarPaymentCarry) string {
	row.Digest = ""
	return p6Digest(adminJSON(row))
}

// SaveOpeningCalendarPaymentCarry is one-shot per existing ledger identity;
// retries require the exact request and frozen input. It grants no property.
func SaveOpeningCalendarPaymentCarry(tx *gorm.DB, row *db.OpeningCalendarPaymentCarry) error {
	if err := openingCalendarPaymentTransaction(tx); err != nil {
		return err
	}
	if row == nil || row.PlayerID <= 0 || row.RuleID == "" || row.RequestID == "" || len(row.RequestID) > 128 || row.RealCutoff <= 0 || !openingCalendarPaymentWindowsValid(*row) || row.Digest != openingCalendarPaymentDigest(*row) {
		return constUtil.ErrAdminConfiguration
	}
	scope, err := openingCalendarPaymentScope(tx, row.PlayerID)
	if err != nil {
		return err
	}
	if row.ScopeID != scope {
		return constUtil.ErrAdminConfiguration
	}
	if _, err := openingCalendarPaymentValues(*row, nil, row.AfterEndAt); err != nil {
		return err
	}
	var prior db.OpeningCalendarPaymentCarry
	err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("player_id=? AND scope_id=?", row.PlayerID, scope).Take(&prior).Error
	if err == nil {
		if prior.Digest != openingCalendarPaymentDigest(prior) {
			return constUtil.ErrAdminConfiguration
		}
		if prior.RequestID == row.RequestID && prior.Digest == row.Digest {
			return nil
		}
		return constUtil.ErrAdminConfigConflict
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return tx.Create(row).Error
}

// Counter projection never writes the historical counter ledger. cfg is supplied
// by refresh for live-window validation; the read repository uses the frozen row.
func openingCalendarPaymentCounters(tx *gorm.DB, playerID int64, cfg *db.OpenServerActivityRule, now time.Time) (map[string]int32, bool, error) {
	if !gameSchemaHasTable(tx, &db.OpeningCalendarPaymentCarry{}) {
		return nil, false, nil
	}
	scope, err := openingCalendarPaymentScope(tx, playerID)
	if err != nil {
		return nil, false, err
	}
	var row db.OpeningCalendarPaymentCarry
	err = tx.Where("player_id=? AND scope_id=?", playerID, scope).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, true, err
	}
	if row.Digest != openingCalendarPaymentDigest(row) || !openingCalendarPaymentWindowsValid(row) || row.RealCutoff <= 0 ||
		cfg != nil && (cfg.ID != row.RuleID || cfg.StartAt != row.AfterStartAt || cfg.EndAt != row.AfterEndAt || cfg.RewardEndAt != row.AfterRewardAt) {
		return nil, true, constUtil.ErrAdminConfiguration
	}
	query := tx.Table("p3_payment_orders AS pay").Select("pay.id,pay.player_id,pay.status,pay.cents,pay.confirmed_at AS real_at,pay.confirmed_at AS game_at,pay.confirmed_at AS projection_real").Where("pay.player_id=? AND pay.status=? AND pay.confirmed_at>=?", playerID, constUtil.P3PaymentConfirmed, row.RealCutoff)
	if gameSchemaHasTable(tx, &db.LocalPlayPaymentTime{}) {
		query = query.Select("pay.id,pay.player_id,pay.status,pay.cents,pay.confirmed_at AS real_at,COALESCE(play.game_at,pay.confirmed_at) AS game_at,COALESCE(play.real_at,pay.confirmed_at) AS projection_real").Joins("LEFT JOIN local_play_payment_times AS play ON play.order_id=pay.id AND play.player_id=pay.player_id").Where("COALESCE(play.game_at,pay.confirmed_at)>=? AND COALESCE(play.game_at,pay.confirmed_at)<? AND COALESCE(play.game_at,pay.confirmed_at)<=?", row.AfterStartAt, row.AfterEndAt, now.UnixMilli())
	} else {
		query = query.Where("pay.confirmed_at>=? AND pay.confirmed_at<? AND pay.confirmed_at<=?", row.AfterStartAt, row.AfterEndAt, now.UnixMilli())
	}
	const maximumOrders = 10000
	var orders []openingCalendarPaymentCandidate
	if err = query.Order("pay.id").Limit(maximumOrders + 1).Find(&orders).Error; err != nil {
		return nil, true, err
	}
	if len(orders) > maximumOrders {
		return nil, true, constUtil.ErrAdminConfiguration
	}
	values, err := openingCalendarPaymentValues(row, orders, now.UnixMilli())
	return values, true, err
}

// Keep real confirmations and business projections separate while merging IDs.
func openingCalendarPaymentValues(row db.OpeningCalendarPaymentCarry, orders []openingCalendarPaymentCandidate, now int64) (map[string]int32, error) {
	var facts, baseline []OpeningCalendarPaymentFact
	if json.Unmarshal(row.Facts, &facts) != nil || json.Unmarshal(row.Baseline, &baseline) != nil {
		return nil, constUtil.ErrAdminConfiguration
	}
	const maximumOrders = 10000
	if len(facts) > maximumOrders || len(baseline) > maximumOrders || len(orders) > maximumOrders {
		return nil, constUtil.ErrAdminConfiguration
	}
	byID := map[string]OpeningCalendarPaymentFact{}
	for _, fact := range baseline {
		if fact.OrderID == "" || fact.Cents <= 0 || fact.RealAt <= 0 || byID[fact.OrderID].OrderID != "" {
			return nil, constUtil.ErrAdminConfiguration
		}
		byID[fact.OrderID] = fact
	}
	accepted := map[string]OpeningCalendarPaymentFact{}
	for _, fact := range facts {
		original, ok := byID[fact.OrderID]
		if !ok || original.Cents != fact.Cents || original.RealAt != fact.RealAt || accepted[fact.OrderID].OrderID != "" || fact.GameAt < row.BeforeStartAt || fact.GameAt >= row.BeforeEndAt {
			return nil, constUtil.ErrAdminConfiguration
		}
		fact.GameAt = row.AfterStartAt + (fact.GameAt - row.BeforeStartAt)
		accepted[fact.OrderID] = fact
	}
	for _, order := range orders {
		if order.ID == "" || order.PlayerID != row.PlayerID || order.Status != constUtil.P3PaymentConfirmed || order.Cents <= 0 || order.RealAt < row.RealCutoff || order.ProjectionReal != order.RealAt {
			return nil, constUtil.ErrAdminConfiguration
		}
		if original, ok := byID[order.ID]; ok {
			if original.Cents != order.Cents || original.RealAt != order.RealAt {
				return nil, constUtil.ErrAdminConfiguration
			}
			continue
		}
		if prior, ok := accepted[order.ID]; ok {
			if prior.Cents != order.Cents || prior.RealAt != order.RealAt || prior.GameAt != order.GameAt {
				return nil, constUtil.ErrAdminConfiguration
			}
			continue
		}
		if order.GameAt >= row.AfterStartAt && order.GameAt < row.AfterEndAt && order.GameAt <= now {
			accepted[order.ID] = OpeningCalendarPaymentFact{order.ID, order.Cents, order.RealAt, order.GameAt}
		}
	}
	values := map[string]int32{constUtil.P3OpenServerTotalPaid: 0}
	var total int64
	for _, fact := range accepted {
		if total > math.MaxInt64-fact.Cents || fact.Cents/100 > math.MaxInt32 {
			return nil, constUtil.ErrAdminConfiguration
		}
		total += fact.Cents
		// Accepted old cumulative money remains immediately qualified. Its
		// single-payment task still belongs to the mapped calendar day.
		if p3Day(time.UnixMilli(fact.GameAt)) > p3Day(time.UnixMilli(now)) {
			continue
		}
		key := constUtil.P3OpenServerSinglePaid + p3Day(time.UnixMilli(fact.GameAt))
		values[key] = max(values[key], int32(fact.Cents/100))
	}
	if total/100 > math.MaxInt32 {
		return nil, constUtil.ErrAdminConfiguration
	}
	values[constUtil.P3OpenServerTotalPaid] = int32(total / 100)
	return values, nil
}

func openingCalendarMergePaymentCounters(tx *gorm.DB, id int64, rows []db.OpenServerActivityCounter) ([]db.OpenServerActivityCounter, error) {
	values, carried, err := openingCalendarPaymentCounters(tx, id, nil, LocalPlayNow(tx.Statement.Context))
	if err != nil || !carried {
		return rows, err
	}
	merged := make([]db.OpenServerActivityCounter, 0, len(rows)+len(values))
	for _, row := range rows {
		if row.CounterKey != constUtil.P3OpenServerTotalPaid && !strings.HasPrefix(row.CounterKey, constUtil.P3OpenServerSinglePaid) {
			merged = append(merged, row)
		}
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		merged = append(merged, db.OpenServerActivityCounter{PlayerID: id, CounterKey: key, Value: values[key]})
	}
	return merged, nil
}
