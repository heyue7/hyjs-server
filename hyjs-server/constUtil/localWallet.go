package constUtil

type LocalWalletContextKey string

const LocalWalletAPIContextKey LocalWalletContextKey = "local_demo_wallet_api"
const LocalWalletPaymentContextKey LocalWalletContextKey = "local_demo_wallet_payment"
const LocalWalletSessionContextKey LocalWalletContextKey = "local_demo_wallet_session"
const LocalWalletVersion = "local-demo-wallet-v1"
const LocalWalletName = "蓝绿终端"
const LocalWalletDisabledMessage = "本地余额暂未开放"
const LocalWalletInsufficientMessage = "超频点数不足"
const LocalWalletConflictMessage = "余额请求标识已用于其他金额"
const LocalWalletInvalidMessage = "本地余额请求或账本无效"
const LocalWalletSource = "local-demo-wallet-v1：仅本地demo角色的试玩余额，以人民币分整数保存；用户显式增加余额生成幂等入账，原充值按已冻结订单金额同事务扣账及确认，无真实收费、无外部支付。默认关闭；订单余额不足保持pending，加钱后原订单可重试。"

// LocalWalletMaxCreditCents 试玩单次最大入账一万亿元；整数分保持JS精确范围。
const LocalWalletMaxCreditCents int64 = 100000000000000
