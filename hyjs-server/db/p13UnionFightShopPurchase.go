package db

// P13UnionFightShopPurchase retains every actual purchase, including stamp0.
// Quote and compact before/after frames remain business evidence after expiry.
type P13UnionFightShopPurchase struct {
	ID              string  `gorm:"primaryKey;type:char(64)" json:"id"`
	MatchID         string  `gorm:"type:char(64);not null;index:p13_shop_purchase_actor,priority:1" json:"match_id"`
	PlayerID        int64   `gorm:"not null;index:p13_shop_purchase_actor,priority:2" json:"player_id"`
	Stamp           int64   `gorm:"not null" json:"stamp"`
	ShopItemID      string  `gorm:"type:varchar(64);not null" json:"shop_item_id"`
	ItemID          string  `gorm:"type:varchar(64);not null" json:"item_id"`
	Count           int64   `gorm:"not null" json:"count"`
	Quantity        int64   `gorm:"not null" json:"quantity"`
	Price           int64   `gorm:"not null" json:"price"`
	TotalPrice      int64   `gorm:"not null" json:"total_price"`
	CostCurrency    string  `gorm:"type:varchar(64);not null" json:"cost_currency"`
	InventoryBefore int64   `gorm:"not null" json:"inventory_before"`
	InventoryAfter  int64   `gorm:"not null" json:"inventory_after"`
	TokenBefore     float64 `gorm:"not null" json:"token_before"`
	TokenAfter      float64 `gorm:"not null" json:"token_after"`
	StockBefore     int64   `gorm:"not null" json:"stock_before"`
	StockAfter      int64   `gorm:"not null" json:"stock_after"`
	QuoteFrame      []byte  `gorm:"type:blob;not null" json:"-"`
	QuoteSource     string  `gorm:"type:text;not null" json:"quote_source"`
	QuoteHash       string  `gorm:"type:char(64);not null" json:"quote_hash"`
	Before, After   []byte  `gorm:"type:blob;not null" json:"-"`
	BeforeHash      string  `gorm:"type:char(64);not null" json:"before_hash"`
	AfterHash       string  `gorm:"type:char(64);not null" json:"after_hash"`
	LogicalAt       int64   `gorm:"not null;index:p13_shop_purchase_actor,priority:3" json:"logical_at"`
	SourceHash      string  `gorm:"type:char(64);not null" json:"source_hash"`
}

func (P13UnionFightShopPurchase) TableName() string { return "p13_union_fight_shop_purchases" }
