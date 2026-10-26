package constUtil

const (
	// FieldBossBattleType 为原 BattleData.SetData(SCSidePveBossFightStart) 的野外首领战斗类型。
	FieldBossBattleType       int32 = 12
	FieldBossKillNoticeFormat       = "恭喜%s完成最后一击，击败了%s！"
	// FieldBossSource 说明本次个人实例只复用原普通首领表、协议与一张地图内个人进度。
	FieldBossSource = "original-fieldboss-personal-instance-v1：只使用原TableFieldBoss/TableFieldBossPos与30082/30083/30085/30087；同一FieldFight占用与现有野外互斥/断线门卫一致。刷新按原RebornTime小时，奖励按原rewardGroupId/KillReward逐资源原权重，未改数值/次数/价格。共享全服首领与多人归属仍后置。"
)
