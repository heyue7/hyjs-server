package model

// LocalWalletView is the authenticated local wallet state used by the 蓝绿钱包 overlay.
type LocalWalletView struct {
	Name           string                    `json:"name"`                     // 固定显示名：蓝绿钱包。
	BalanceCents   int64                     `json:"balanceCents"`             // 当前余额，单位人民币分，非负整数。
	Revision       int64                     `json:"revision"`                 // 同角色成功入账/支付的单调递增序号。
	Currency       string                    `json:"currency"`                 // 枚举：CNY。
	Enabled        bool                      `json:"enabled"`                  // 当前持久政策是否开放。
	TestOnly       bool                      `json:"testOnly"`                 // 固定true，本地试玩余额。
	Version        string                    `json:"version"`                  // 政策版本local-demo-wallet-v1。
	AlreadyApplied bool                      `json:"alreadyApplied,omitempty"` // true表示该入账请求已执行，本次未重复加款。
	CreditLabel    string                    `json:"creditLabel,omitempty"`    // CreditLabel为本次入账冻结的故事文案。
	Summary        LocalWalletSummary        `json:"summary"`                  // Summary是实际账务时间范围汇总。
	Total          int64                     `json:"total"`                    // Total是当前流水筛选总数。
	Page           int64                     `json:"page"`                     // Page是一开始的页码。
	PageSize       int64                     `json:"pageSize"`                 // PageSize是本页最多条数。
	Entries        []LocalWalletEntryView    `json:"entries"`                  // 最近20条真实余额流水，最新在前。
	PendingOrders  []LocalWalletPendingOrder `json:"pendingOrders"`            // 最近20条本角色原未过期待付订单。
}

// LocalWalletEntryView projects one immutable wallet ledger entry.
type LocalWalletEntryView struct {
	ID           string `json:"id"`                // 同角色唯一流水标识。
	Kind         string `json:"kind"`              // 枚举：credit增加余额；payment原充值消费。
	AmountCents  int64  `json:"amountCents"`       // 本次变动分数，credit正数、payment负数。
	BalanceCents int64  `json:"balanceCents"`      // 此次成功变动后的余额分数。
	CreatedAt    int64  `json:"createdAt"`         // Unix毫秒时间戳。
	ReasonID     string `json:"reasonId"`          // ReasonID为入账理由，老记录为空。
	OrderID      string `json:"orderId,omitempty"` // OrderID为真实游戏充值订单。
	Label        string `json:"label"`             // 流水显示文字。
}

// LocalWalletPendingOrder identifies an original unpaid frozen payment order.
type LocalWalletPendingOrder struct {
	OrderID     string `json:"orderId"`     // 原订单ID，支付重试复用此ID。
	ProductName string `json:"productName"` // 原订单冻结商品名称。
	AmountCents int64  `json:"amountCents"` // 原订单冻结价格，单位人民币分。
	ExpiresAt   int64  `json:"expiresAt"`   // 原订单到期Unix毫秒时间戳。
}

// LocalWalletSummary 按真实入账时间累计收支；游戏调日不影响账务。
type LocalWalletSummary struct {
	StartAt          int64 `json:"startAt"`          // StartAt是包含的起始毫秒，0表示全部。
	EndAt            int64 `json:"endAt"`            // EndAt是不包含的截止毫秒，0表示不限。
	IncomeCents      int64 `json:"incomeCents"`      // IncomeCents是该时段收入分。
	ExpenseCents     int64 `json:"expenseCents"`     // ExpenseCents是该时段消费分。
	NetCents         int64 `json:"netCents"`         // NetCents是净变化分。
	TransactionCount int64 `json:"transactionCount"` // TransactionCount是收支总笔数。
}

// LocalWalletLedgerRequest 读取角色完整分页流水。
type LocalWalletLedgerRequest struct {
	PlayerID   int64  `json:"playerId" binding:"required"`   // PlayerID是当前角色。
	SessionHex string `json:"sessionHex" binding:"required"` // SessionHex是当前会话。
	StartAt    int64  `json:"startAt"`                       // StartAt是起始真实时间毫秒，0不限。
	EndAt      int64  `json:"endAt"`                         // EndAt是截止真实时间毫秒，0不限。
	Kind       string `json:"kind"`                          // Kind可为空、credit或payment。
	Page       int64  `json:"page"`                          // Page是从1开始页码。
	PageSize   int64  `json:"pageSize"`                      // PageSize是1至100条。
}

// AdminWalletReasonRequest 编辑未来入账理由，不改已有流水。
type AdminWalletReasonRequest struct {
	Label    string `json:"label" binding:"required"`   // Label可含{amount}金额占位。
	MinCents int64  `json:"minCents"`                   // MinCents为包含的最小正整数分。
	MaxCents int64  `json:"maxCents"`                   // MaxCents为包含的最大分。
	Weight   int64  `json:"weight"`                     // Weight为1至1000000相对权重。
	Enabled  bool   `json:"enabled"`                    // Enabled控制未来是否参与抽取。
	Version  string `json:"version" binding:"required"` // Version是读取时摘要。
}
