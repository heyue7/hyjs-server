package constUtil

const (
	P8GatherStartOperation = "gather-start"
	P8GatherClaimOperation = "gather-claim"
	P8ResourceAvailable    = int32(1)
	P8ResourceMissing      = int32(2003)
	P8FieldIdle            = int32(0)
	P8FieldFishing         = int32(3)
	P8FieldMining          = int32(4)
	P8EventMineGather      = "field_mine_gather"
	P8EventFishGather      = "field_fish_gather"
	P8GatherScene          = "field_001"
	P8GatherMaxDurationMS  = int64(600000)
	P8GatherPolicySource   = "APK FieldResource/FieldResourcePos/Field；有限单人服：随机无放回选点，按原资源权重，5次耗尽后原位重生；开始冻结奖励，领取才扣疲劳，移动/离场/开战取消不扣费；采集不额外造技能经验"
)
