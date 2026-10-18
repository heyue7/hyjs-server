package constUtil

const (
	SkillSpecialtyConfigError  = "技能专精配置或存档无效"
	SkillSpecialtyGateError    = "技能专精需要品质20或23且角色75级"
	SkillSpecialtyLockedError  = "该专精尚未解锁"
	SkillSpecialtyLimitError   = "技能专精已达到上限"
	SkillSpecialtyModeError    = "请按当前专精阶段选择升级或强化"
	SkillSpecialtyReplayError  = "专精请求已过期或时间戳冲突"
	SkillSpecialtyItemError    = "专精材料不足或库存无效"
	SkillSpecialtyRuleID       = "default"
	SkillSpecialtyUnlockPolicy = "table_pair_strength"
	SkillSpecialtyRuleSource   = "本地明确规则：首次达到原75级和品质门槛，类型1/2/3免费从等级1强化1开始，4/5锁定；canOpen解释为所在类型与指定类型同时达到强化门槛后开启目标。原表费用、属性和上限沿用，初始值及解锁解释缺少原服务端实现证据。"
)
