package constUtil

const (
	InstanceLockPrefix      = "p5.lock:"
	InstanceLockRevisionKey = "p5.lockRevision"
	InstanceLockInvalid     = "角色或装备已不存在，请刷新后重试"
	InstanceLockConflict    = "锁定状态已变化，请刷新后重试"
	InstanceLocked          = "该角色或装备已锁定，请先解锁"
)

const (
	TalentMaterialExpedition = "材料角色正在远征，请先完成远征并领取奖励"
	TalentMaterialMain       = "主角不能作为天赋材料"
	TalentMaterialFormation  = "材料角色已上阵或穿戴，请先从队伍中移除"
	TalentMaterialHelper     = "材料角色正在助阵，请先取消助阵"
	TalentMaterialLocked     = "材料角色已锁定，请先解锁"
	TalentMaterialDispatch   = "材料角色已派遣，请先结束派遣"
	TalentMaterialProgressed = "材料角色已培养或有待确认训练，不能作为天赋材料"
	TalentMaterialMissing    = "材料角色状态已变化，请刷新后重试"
)
