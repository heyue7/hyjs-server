package constUtil

const (
	// AdminFieldCombatMaxLevel 为野外小怪/普通首领成员可配置等级上限。
	AdminFieldCombatMaxLevel int64 = 1000
	// AdminFieldCombatMaxBase 为生命/攻击/防御等基础数值上限，和现有战斗协议双精度输入保持安全余量。
	AdminFieldCombatMaxBase float64 = 1e12
	// AdminFieldCombatMaxRate 为命中/闪避/暴击/抗暴/倍率/增伤/减伤的数值上限。
	AdminFieldCombatMaxRate float64 = 10000
	// AdminFieldBossMaxRebornHours 为普通首领原小时刷新间隔上限，7天。
	AdminFieldBossMaxRebornHours int64 = 168
	// AdminFieldWorldBossMaxLevel 为世界首领下一周期等级上限，沿用原等级系数表1至50。
	AdminFieldWorldBossMaxLevel int64 = 50
)
