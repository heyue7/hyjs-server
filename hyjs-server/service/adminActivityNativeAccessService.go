package service

import (
	"errors"
	"strings"

	"gorm.io/gorm"
	"hyjs-server/db"
)

// adminNativePaymentAccess 让显式保存的支付配置在已接管日历的区服仍能使用原消费者。
// 未被运营保存的旧占位配置继续受日历门禁约束，已发布排期不走此兼容路径。
func adminNativePaymentAccess(tx *gorm.DB, key string) (bool, bool, error) {
	matched := false
	for _, prefix := range []string{"firstRecharge_", "rechargeSingle_", "rechargeTotal_", "continuedRecharge_", "costTotal_"} {
		matched = matched || strings.HasPrefix(key, prefix)
	}
	if !matched || !gameSchemaHasTable(tx, &db.AdminActivityPresentation{}) {
		return false, false, nil
	}
	var meta db.AdminActivityPresentation
	err := tx.Where("activity_id=? AND archived=?", "payment:"+key, false).Take(&meta).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	var row db.PaymentActivityRule
	if err := tx.First(&row, "id=?", key).Error; err != nil {
		return false, false, err
	}
	kind, ok := adminNativePaymentKind(row.Kind)
	if !ok || meta.Mode != "native-"+kind {
		return false, false, nil
	}
	return true, row.Enabled, nil
}
