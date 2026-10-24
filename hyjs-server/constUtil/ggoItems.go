package constUtil

const (
	// AdminFieldGGOSchemaVersion 为当前沙漠后台配置结构版本。
	AdminFieldGGOSchemaVersion   int64 = 2
	GGOShopKey                         = "ggo"
	GGOShopName                        = "信用商店"
	GGOCurrencyID                      = "item_9710"
	GGOOverclockID                     = "item_9711"
	GGOOverclockUnit                   = int64(100)
	GGOIncomeSource                    = "ggo-item"
	GGOIncomeLabel                     = "超频点数道具入账"
	GGOSeedRuleID                      = "ggo:default"
	GGOSeedProductID                   = "ggo_overclock_1"
	GGOShopStatePrefix                 = "ggo:"
	GGOShopRefreshReceiptProduct       = "@ggo:refresh:v1" // Refresh receipt sentinel shares the purchase UUID namespace.
)
