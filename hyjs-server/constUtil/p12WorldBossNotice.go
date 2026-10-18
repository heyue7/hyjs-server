package constUtil

const (
	// 原RoomNetBase.SCPublicMessage的type=1进入CreateMarqueeBox；scrollType=2另开全屏提示，故保持0。
	P12WorldBossKillNoticeType       int32 = 1
	P12WorldBossKillNoticeScrollType int32 = 0
	// Re仅随world/get读取服务端body，未附服务端模板；下文为本服文案。
	P12WorldBossKillNoticeFormat = "恭喜%s完成最后一击，击败了世界首领！"
)
