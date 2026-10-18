package constUtil

// 豪华签到只读取后台明确配置的活动和日奖，不再自动补设奖励。
const P3LuxurySignActivityID = "luxury_sign"
const P3LuxurySignRewardType = "daily"

type P3PaymentContextKey string

const P3TestPaymentContextKey P3PaymentContextKey = "p3_test_payment_adapter"
const P3PaymentPending = "pending"
const P3PaymentConfirmed = "confirmed"
const P3PaymentFailed = "failed"
const P3PaymentSource = "local_demo_test"
const P3PaymentOrderLifetimeMS int64 = 30 * 60 * 1000
const P3MonthCardDurationMS int64 = 30 * 24 * 60 * 60 * 1000
const P3PaymentError = "支付请求或可信测试订单状态无效"
const P3PaymentUnavailable = "支付资格未达成、已领取或窗口已结束"
const P3PaymentConfigError = "支付或会员规则配置无效"
const P3PaymentRealDisabled = "真实收费未启用；客户端成功通知不能确认支付"
const P3OpenServerTotalPaid = "p3_paid_total"
const P3OpenServerSinglePaid = "p3_paid_single:"
const P3VIP9GrowthProduct = "p3vip9_growth_local"
const P3VIP9GrowthSource = "本地会员成长礼包；默认关闭；1000钻、每日一份为本地规则，原购买价格未恢复；原item_043→item_065料理技能书，VIP9及真实确认测试订单资格"
const P3VIP7GrowthProduct = "p3vip7_growth_local"
const P3VIP8GrowthProduct = "p3vip8_growth_local"
const P3VIP7GrowthSource = "本地会员成长礼包；默认关闭；800钻、每日一份为本地规则，原购买价格未恢复；原item_041资源礼包，VIP7及真实确认测试订单资格"
const P3VIP8GrowthSource = "本地会员成长礼包；默认关闭；900钻、每日一份为本地规则，原购买价格未恢复；原item_042→item_063宝石技能书，VIP8及真实确认测试订单资格"
const P3VIP10GrowthProduct = "p3vip10_growth_local"
const P3VIP10GrowthSource = "本地会员成长礼包；默认关闭；1100钻、每日一份为本地规则，原购买价格未恢复；原item_044→item_064宝石技能书，VIP10及真实确认测试订单资格"
