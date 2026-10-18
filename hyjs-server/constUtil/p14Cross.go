package constUtil

import "time"

const (
	P14Closed                  = "跨服报名本批尚未开放"
	P14ResultsUnverified       = "跨服匹配/权威赛果/奖励/竞猜与历史仍待完成，当前不提交胜负或扣竞猜资产"
	P14Source                  = "原TableGameConfigure.ServerBattleLevelLimit70；TableText.ServerBattle_Audition_rule；原CSKFEnterGame/EnterFrightGame/Login空请求与公开资料联合serverId。内部HMAC和持久outbox为具名本地跨服兼容协议。"
	P14ProtocolVersion   int32 = 1
	P14RegisterStage     int32 = 2
	P14DeliveryInterval        = 5 * time.Second
	P14DeliveryPath            = "/internal/p14/snapshot"
)
