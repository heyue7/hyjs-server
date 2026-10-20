package constUtil

import "errors"

// 安全事件使用固定短码和独立表，避免把原始请求、凭据或错误详情写入日志。
const (
	AdminSecurityEventOutcomeSuccess   = "success"
	AdminSecurityEventOutcomeFailure   = "failure"
	AdminSecurityEventOutcomeDenied    = "denied"
	AdminSecurityEventOutcomeThrottled = "throttled"

	AdminSecurityEventActorMax     = 64
	AdminSecurityEventSourceIPMax  = 64
	AdminSecurityEventActionMax    = 64
	AdminSecurityEventTargetMax    = 191
	AdminSecurityEventOutcomeMax   = 16
	AdminSecurityEventReasonMax    = 64
	AdminSecurityEventRequestIDMax = 64

	AdminSecurityEventMinuteMillis      int64 = 60 * 1000
	AdminSecurityEventRetentionMillis   int64 = 30 * 24 * 60 * 60 * 1000
	AdminSecurityEventCleanupIntervalMS int64 = 5 * 60 * 1000
	AdminSecurityEventCleanupBatch            = 1000
)

// ErrAdminSecurityEventInput 表示事件字段无法安全落库，调用方应记录报警而不是写入原始值。
var ErrAdminSecurityEventInput = errors.New("安全事件字段无效")
