package constUtil

const (
	P11PlunderSweepSource       = "original56009-local-real-holder-type3-batch-v1"
	P11PlunderSweepUnavailable  = "夺宝扫荡当前未开放或自动补药尚未恢复"
	P11PlunderSweepConflict     = "夺宝扫荡实际结算来源不完整"
	P11PlunderSweepSourceNote   = "具名本地原56009兼容策略v1：复用预发布defaultoff夺宝政策及真实持片对手，不使用机器人；每次原type3独立结算、消耗实际2耐力，五次以取得目标片/不足耐力/真实对手持片耗尽为停止条件。原服多次匹配/停止/概率未知，沿已冻结本地PPM；自动补药关闭。内部负序号只标识批次子回执，原HTTP仍正时间戳。"
	P11PlunderSweep84Source     = "original56009-local-type3-auto-energy-v2"
	P11PlunderSweep84SourceNote = "批84原56009：真实type3结算，允许选中自动补耐力时按原item_001每瓶10点实际扣药，取得目标、耐力与药不足或对手耗尽停止；批次冻结请求、药剂配方和实际数量。"
	P11PlunderSweep84Medicine   = "item_001"
	P11PlunderSweep84Energy     = int32(10)
	P11PlunderSweepEnergyShort  = "夺宝耐力不足，请使用耐力药剂"
)
