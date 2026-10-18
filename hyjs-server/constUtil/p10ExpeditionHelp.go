package constUtil

const (
	P10ExpeditionHelpOperation    = "exp-friend-help"
	P10ExpeditionHelpSource       = "原HelpFriendsPage/HelpGrid/VipPerks.numPerk5及ExpeditionHelperReward；P7实际风险事件与当前好友资格"
	P10ExpeditionHelpEvent        = "expedition_friend_help"
	P10ExpeditionHelpDiamondEvent = "expedition_help_diamond_received"
	P10ExpeditionHelpVIPCatalog   = "10,15,15,15,20,20,25,30,30,30,30,30,30"
)

// P10ExpeditionHelpClockContextKey 只由内部测试请求上下文提供时钟，客户端不能指定时间。
type P10ExpeditionHelpClockContextKey struct{}
