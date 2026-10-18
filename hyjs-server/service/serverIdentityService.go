package service

import "hyjs-server/constUtil"

// OriginalServerID 读取可信启动编号；旧隔离测试未注入时保留原本地区服。
// 对外请求中的zoneId/serverId仅用于校验与路由，不能覆盖此身份。
func (s *GameService) OriginalServerID() int64 {
	if s.ServerID > 0 {
		return s.ServerID
	}
	return constUtil.GameDefaultServerID
}
