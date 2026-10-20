package model

// SecurityEventInput 是内部安全事件采集契约，不依赖HTTP或gin，供登录与游戏分发复用。
type SecurityEventInput struct {
	Actor      string // Actor 是已清洗的账号或后台用户名。
	SourceIP   string // SourceIP 必须可解析为IP，不接受原始请求头。
	Action     string // Action 是固定短码，例如 auth.login。
	Target     string // Target 是有限对象标识，不保存请求正文。
	Outcome    string // Outcome 取空、success、failure、denied、throttled。
	ReasonCode string // ReasonCode 是短码，不写密码、验证码、token或错误详情。
	RequestID  string // RequestID 是可选关联标识，不保存原始请求。
}

// AdminSecurityEventsRequest 是安全事件页的固定筛选参数。
type AdminSecurityEventsRequest struct {
	Page     int64  // Page 从1开始。
	PageSize int64  // PageSize 受现有后台分页上限约束。
	Action   string // Action 为空表示不筛选。
	Outcome  string // Outcome 取空、success、failure、denied、throttled。
	Actor    string // Actor 为空表示不筛选。
}

// AdminSecurityEventDTO 只暴露安全事件页需要的展示字段，不返回内部EventKey。
type AdminSecurityEventDTO struct {
	ID          int64  `json:"id"`          // ID 是事件行编号。
	FirstAt     int64  `json:"firstAt"`     // FirstAt 是同一分钟聚合的首次毫秒时间。
	LastAt      int64  `json:"lastAt"`      // LastAt 是同组最近一次毫秒时间。
	Actor       string `json:"actor"`       // Actor 是事件账号或后台用户名。
	SourceIP    string `json:"sourceIp"`    // SourceIP 是服务端解析后的规范IP。
	Action      string `json:"action"`      // Action 是固定事件短码。
	Target      string `json:"target"`      // Target 是有限对象标识。
	Outcome     string `json:"outcome"`     // Outcome 取空、success、failure、denied、throttled。
	ReasonCode  string `json:"reasonCode"`  // ReasonCode 是原因短码。
	RequestID   string `json:"requestId"`   // RequestID 是可选关联标识。
	Occurrences int64  `json:"occurrences"` // Occurrences 是同一分钟分组的累计次数。
}

// AdminSecurityEventsResponse 返回一页安全事件及筛选后的总数。
type AdminSecurityEventsResponse struct {
	Items []AdminSecurityEventDTO `json:"items"` // Items 是本页事件。
	Total int64                   `json:"total"` // Total 是筛选后的总行数。
}
