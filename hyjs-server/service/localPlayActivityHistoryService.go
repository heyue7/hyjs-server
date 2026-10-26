package service

import (
	"gorm.io/gorm"
	"hyjs-server/db"
)

func localPlayActivityHistory(tx *gorm.DB, key string) (*db.LocalPlayActivityCycle, error) {
	cycleID := localPlayPrivateCycleID(key)
	if cycleID == "" {
		return nil, nil
	}
	allowed, err := localPlayActivityAccess(tx, key, false)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, errLocalPlayActivitiesInvalid
	}
	var cycle db.LocalPlayActivityCycle
	if err := tx.First(&cycle, "id=?", cycleID).Error; err != nil {
		return nil, err
	}
	if cycle.RealCreatedAt <= 0 || cycle.EventAfterID < 0 {
		return nil, errLocalPlayActivitiesInvalid
	}
	return &cycle, nil
}

// A virtual date can occur repeatedly. The owner's committed event watermark
// distinguishes new business from facts committed before the private refresh.
func localPlayActivityFactScope(tx *gorm.DB, key string) (*gorm.DB, error) {
	cycle, err := localPlayActivityHistory(tx, key)
	if err != nil {
		return nil, err
	}
	if cycle == nil {
		return tx, nil
	}
	return tx.Where("id > ?", cycle.EventAfterID), nil
}

// The original helper projects ConfirmedAt into the game calendar. Retain its
// immutable real confirmation boundary as a separate check for private periods.
func localPlayActivityPaymentOrders(tx *gorm.DB, id int64, key string, start, end int64) ([]db.PaymentOrder, error) {
	cycle, err := localPlayActivityHistory(tx, key)
	if err != nil {
		return nil, err
	}
	orders, err := localPlayPaymentOrders(tx, id, start, end)
	if err != nil || len(orders) == 0 {
		return orders, err
	}
	realStart, err := rechargeActivityRealStart75(tx, id, key)
	if err != nil {
		return nil, err
	}
	if cycle != nil {
		realStart = max(realStart, cycle.RealCreatedAt)
	}
	if realStart <= 0 {
		return orders, nil
	}
	ids := make([]string, 0, len(orders))
	for _, order := range orders {
		ids = append(ids, order.ID)
	}
	var eligible []string
	if err := tx.Model(&db.PaymentOrder{}).Where("player_id=? AND id IN ? AND confirmed_at>=?", id, ids, realStart).Pluck("id", &eligible).Error; err != nil {
		return nil, err
	}
	keep := map[string]bool{}
	for _, id := range eligible {
		keep[id] = true
	}
	filtered := make([]db.PaymentOrder, 0, len(orders))
	for _, order := range orders {
		if keep[order.ID] {
			filtered = append(filtered, order)
		}
	}
	return filtered, nil
}
