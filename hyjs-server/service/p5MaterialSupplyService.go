package service

import (
	"encoding/json"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
)

// seedP5SpecialtySupply 将原表缺少投放源的四种书接入真实主线金币兑换。
// 独立月实例保留既有商店与旧消费快照；运营可提前关闭或修改尚未冻结的窗口。
func (s *GameService) seedP5SpecialtySupply(tx *gorm.DB, now time.Time) error {
	local := now.In(time.FixedZone("UTC+8", 8*3600))
	start := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, local.Location())
	end := start.AddDate(0, 1, 0)
	id := constUtil.P5SpecialtySupplyPrefix + start.Format("200601")
	var exists int64
	if err := tx.Model(&db.P4CommerceRule{}).Where("id=?", id).Count(&exists).Error; err != nil {
		return err
	}
	if exists > 0 {
		return nil
	}
	rules := p4CommerceRules{}
	for index, spec := range []struct {
		id, cost     string
		level, limit int32
	}{
		{"item_441", "coin,50000", 75, 20},
		{"item_445", "coin,100000", 85, 20},
		{"item_449", "coin,200000", 90, 10},
		{"item_453", "coin,400000", 95, 10},
	} {
		product := p4BaseProduct(spec.id, spec.id+",10", spec.cost, int32(index+1), spec.level)
		product.Name = s.table("Item", "itemId", spec.id)["name"]
		product.DailyLimit = spec.limit
		rules.Products = append(rules.Products, product)
	}
	raw, err := json.Marshal(rules)
	if err != nil {
		return err
	}
	cfg := db.P4CommerceRule{ID: id, ShopType: 0, Kind: "exchange", Enabled: false, StartAt: start.UnixMilli(), EndAt: end.UnixMilli(), ClaimEndAt: end.UnixMilli(), Rules: raw, Source: "P5具名本地策略specialty-mainline-coin-exchange-v1：原164表未留下item_441/445/449/453任何产出配置，原服投放未知；真实主线金币兑换每份10本，分别50000/100000/200000/400000金币，玩家75/85/90/95级、每日20/20/10/10份；按已恢复主线金币和养成费用的递增梯度制定本地价格，非原价格复原；独立北京时间月窗口和交易快照/历史回执，不修改ItemShopPrice或既有商品"}
	if _, err := s.validateP4Commerce(cfg); err != nil {
		return err
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&cfg).Error
}
