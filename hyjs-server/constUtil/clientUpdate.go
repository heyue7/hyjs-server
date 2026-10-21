package constUtil

import "errors"

const (
	ClientUpdateReconnectMS     int64 = 5 * 60 * 1000
	ClientUpdateSeenIntervalMS  int64 = 10 * 1000
	ClientUpdateDownloadText          = "请前往QQ群下载最新版"
	ClientUpdateModeNotice            = "notice"
	ClientUpdateModeNextLogin         = "next_login"
	ClientUpdateModeMaintenance       = "maintenance"
)

var ErrClientUpdateRequired = errors.New(ClientUpdateDownloadText)
var ErrClientUpdateMaintenance = errors.New("服务器维护中，请前往QQ群下载最新版")
