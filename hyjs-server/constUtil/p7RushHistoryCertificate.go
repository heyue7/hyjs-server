package constUtil

import "errors"

const (
	P7RushHistoryCertificateFormat int64 = 2
	P7RushHistoryCertificateDomain       = "hyjs:p7-rush-history-certificate:v2"
	// Increment this contract whenever historical admission, driver proof or accepted-End semantics change.
	P7RushHistoryValidationContract = "settled-manual:full-source-scope-driver-terminal-end-hp-pb:v2"
	P7RushHistoryWarmLimit          = 128
)

var (
	ErrP7RushHistoryCertificateUnavailable = errors.New("试炼历史校验证书未启用或未初始化")
	ErrP7RushHistoryWarmLimit              = errors.New("试炼历史预热超过单次128场上限，请使用有界分批维护")
)

// P7EliteQueryProjectionKey stays inside one locked read transaction.
type P7EliteQueryProjectionKey struct{}
