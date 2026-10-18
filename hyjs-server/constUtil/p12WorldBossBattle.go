package constUtil

const (
	P12WorldBossBattlePolicy  = "local-worldboss-sum-terminal-hp-on-finish-v1"
	P12WorldBossBattleClosed  = "本地世界首领战斗策略尚未开放"
	P12WorldBossBattleInvalid = "世界首领战斗回执无效或不属于当前角色"
	P12WorldBossEconomyPolicy = "local-worldboss-cycle-boost-and-six-tier-mail-v1"
	P12WorldBossEconomySource = "具名本地经济规则：原四种150钻增益每角色每周期各买一次，hit/cri+80或原tal_3163/tal_3165对本周期之后开战的全队生效；原始队伍数值不持久修改。周期击杀或60分钟结束后，已完成参与者获原type0，伤害排名1/type4、2..3/type3、4..10/type2、11+/type1，最后一击另获type5，零伤害仅参与奖；每角色/周期/奖档唯一。原概率串按每个物品实际权重总和独立抽取，不补1000；结果冻结后经原领奖中心领取，截止结束后7天。新经济/奖励开关默认关闭，不声明原服归属、概率实现或原机完整验收。"
	P12WorldBossBattleSource  = "用户授权本地规则：原type18、40回合、双方自动、倍率1；四部位初始HP减最终HP非负之和一次向下取整，bloodNumber不乘；总共享HP为四部位初始HP之和；当前真实野外位置须在原出生点10米内，角色存活且无其他战斗。开战预计算并扣原免费次数，正常结束或跳过的认证finish才扣共享HP并累计排行；pending无定时自动结算，活动期内重登可重放；未提交战斗跨期后由恢复/普通动作/结束请求原子放弃且不扣HP、不退次数，主动放弃亦如此；finish幂等且伤害封顶当前共享HP；原int32伤害展示上限2147483647，账本保持int64。原bosshpForwardLab/NowLab自行乘100，回包两比例为共享剩余HP before/total、after/total。该规则不声明原服公式或原机严格数值认证；经济与发奖另受独立具名默认关闭开关控制。"
)
