package constUtil

import "errors"

const (
	P7RushHistoryCertificateFormat int64 = 1
	P7RushHistoryCertificateDomain       = "hyjs:p7-rush-history-certificate:v1"
	P7RushHistoryWarmLimit               = 128
)

var (
	ErrP7RushHistoryCertificateUnavailable = errors.New("试炼历史校验证书未启用或未初始化")
	ErrP7RushHistoryWarmLimit              = errors.New("试炼历史预热超过单次128场上限，请使用有界分批维护")
)
