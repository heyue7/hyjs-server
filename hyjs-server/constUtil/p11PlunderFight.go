package constUtil

const (
	P11PlunderPermanentVersion   = "local-plunder-permanent-v40"
	P11PlunderPermanentMigration = "feedback40-permanent-default-v1"
	P11PlunderPermanentSource    = "feedback40：仅首次未发布系统默认夺宝政策迁移常驻，保留原匹配、掉率、消耗、扫荡和独立证明"
	P11PlunderFightUnsupported   = "当前队伍暂不支持夺宝战斗"
	P11PlunderFightSource        = "p11-plunder-original-type3"
	P11PlunderFightPolicySource  = "plunder-type3-local-v1：原21055按目标碎片找人、原56011双方原队伍/Inst/合法firstOrder、map_arena_001及固定Max20；原RobTreasureLevel15/TreaSweepCostEnergy2/TreaSweepCoinPerLevel55/TreaSweepExpPerLevel2。原真实饰品碎片与免战截止保持，默认关闭且须预发布窗口。未知原服对手筛选、概率等级与掉片概率、基础奖发放条件和保护解除规则采用具名本地运营：按真实角色ID从小到大扫描最多512、最多3个等级差20内未免战实际持片角色，候选冻结5分钟；概率展示原1-6档与实际百万分率独立配置并冻结，默认中等档3/250000，胜后才随机转移目标碎片一份并按原等级系数发coin/exp，败不发基础奖，自身原免战截止保留。实际Get含基础奖及真正转移碎片；抽牌仅展示真正片或coin0，两张预览固定coin0，不额外入账。无机器人，不动初始排名，不借Arena或好友证书；缺独立原type3证明全事务拒绝，不扣费不发片。"
	P11PlunderFightUnavailable   = "夺宝暂未开放，请稍后再试"
	P11PlunderFightRefresh       = "夺宝目标、实际持片或当前队伍已变化，请刷新"
	P11PlunderFightConflict      = "原夺宝冻结或已提交回执来源不一致"
)
