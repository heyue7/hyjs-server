package model

// TestPaymentConfirmRequest 指定本地隔离订单的可信测试结果。
type TestPaymentConfirmRequest struct {
	// OrderID 是原协议生成的订单编号。
	OrderID string `json:"orderId" binding:"required,max=96"`
	// EventID 是测试确认来源键，重试必须复用同一值。
	EventID string `json:"eventId" binding:"required,max=128"`
	// AmountCents 是冻结商品的人民币分金额。
	AmountCents int64 `json:"amountCents" binding:"required,gt=0"`
	// Outcome 只接受 confirmed=确认、failed=失败。
	Outcome string `json:"outcome" binding:"required,oneof=confirmed failed"`
}

// TestPaymentConfirmResponse 返回本地测试订单结果，不含账号或会话。
type TestPaymentConfirmResponse struct {
	// OrderID 是已处理订单编号。
	OrderID string `json:"orderId"`
	// Status 为 pending=待确认、confirmed=已确认、failed=失败。
	Status string `json:"status"`
	// TestOnly 为true，表示没有发生真实收费。
	TestOnly bool `json:"testOnly"`
}

// LocalOperationsRequest 限定本机只读运行查询的范围。
type LocalOperationsRequest struct {
	// PlayerID 是需检查的角色编号。
	PlayerID int64 `json:"playerId" binding:"required,gt=0"`
	// InstanceID 是可选活动实例编号。
	InstanceID string `json:"instanceId" binding:"max=64"`
}
