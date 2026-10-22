package model

// AdminWalletAdjustRequest 调整选中角色所属全局账号的超频点数。
type AdminWalletAdjustRequest struct {
	// RequestID 是本次操作的36位UUID，重试时保持不变。
	RequestID string `json:"requestId" binding:"required"`
	// Action 取值 credit（发放）或 debit（扣除）。
	Action string `json:"action" binding:"required"`
	// Points 是正整数超频点数，最多1000000000000点。
	Points int64 `json:"points" binding:"required"`
	// Reason 是去除首尾空白后1至200字的操作原因。
	Reason string `json:"reason" binding:"required"`
	// Revision 是操作者读取的余额版本，必传且大于等于0。
	Revision *int64 `json:"revision" binding:"required"`
}

// AdminWalletAdjustResponse 返回账号当前余额及本次调整回执。
type AdminWalletAdjustResponse struct {
	// BalanceCents 是账号当前超频点数余额的百分之一单位。
	BalanceCents int64 `json:"balanceCents"`
	// Revision 是账号当前余额版本。
	Revision int64 `json:"revision"`
	// AmountCents 是本次变动的百分之一单位，扣除时为负数。
	AmountCents int64 `json:"amountCents"`
	// AlreadyApplied 为true表示此前已成功处理，本次仅返回回执。
	AlreadyApplied bool `json:"alreadyApplied"`
}
