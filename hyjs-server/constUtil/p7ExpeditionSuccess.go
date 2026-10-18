package constUtil

const (
	P7ExpeditionSuccessRule            = "original-client-expedition-success-v1"
	P7ExpeditionSuccessSource          = "原UI.SelectPlayerPanel.setPrecent：sum((level+talent*5)/100+匹配任务专长0.5-difficulty/100)/maxHero，非空队伍钳10%至100%；Accept冻结原档位、伙伴贡献和一次随机结果；旧任务nil沿旧约；原服务端随机算法未恢复，服务端采用crypto/rand均匀24bit抽样。既有小时风险为独立具名事件，其未解除失败后果保持。"
	P7ExpeditionSuccessDrawRange int64 = 1 << 24
)
