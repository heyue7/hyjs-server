package constUtil

import "time"

// P13UnionFightClockContextKey 仅供服务器内部注入可控测试时钟；原请求没有设置此ctx值的接口。
type P13UnionFightClockContextKey struct{}

const (
	P13UnionFightGuardHPSourceMonster = "published-monster-v1"
	P13UnionFightMapProjectionReFlat  = "re-flat-v1"
	P13UnionFightFlatYTolerance       = .05
	P13UnionFightFocusOperation       = "CSUFLeaderFocus"
	P13UnionFightFocusSource          = "原UIUnionFightMapPage authority2，原UFLeaderFocusLastTime/CDTime，SCUFLeaderFocus.lastTime为绝对Java毫秒；坐标为原大地图UI ScreenToWorldPoint原样传回，scaleBig5.2；本场本会通知，未赋战斗/传送效果。敌城点击的原UI闭门区域几何后续恢复。"
	P13UnionFightFieldTarget          = "fight_union"
	P13UnionFightFieldUnverified      = "公会战原战斗变换、机关效果与权威胜负尚未完整恢复"
	P13UnionFightFieldTick            = time.Second
	P13UnionFightFieldLifecycleBatch  = 128
	// Snapshot PlayerLimit counts both camps; existing admission permits100 each.
	P13UnionFightFieldMaxPlayers = 200
	P13UnionFightFieldSource     = "原SceneType9/ServerType10、战斗14/回放15、全部UF地图与状态枚举；具名本地策略：显式准备窗口、按运营选原复活点入场，守卫/城门初始HP运营配置，初始门塔等级1及工厂按IsField置0/2，工厂生产未驱动；玩家取冻结Pose面板且未应用未知groupFactor；无权威胜负或默认奖励。"
)

// P13UnionFightOriginalCacheLimit 限定原表只读解析缓存容量，原blob仍逐次校验SHA。
const P13UnionFightOriginalCacheLimit = 2
