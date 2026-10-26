package constUtil

import "time"

const P13UFPreviewLogoConflict = "公会战预览旗帜原表编号无效"

const (
	P13UnionFightClosed             = "公会战报名本批尚未开放"
	P13UnionFightUnknown            = "公会战匹配、战场与每场道具实际投入仍待完整原规则核实"
	P13UnionFightSource             = "TableText.union_fight_desc及原UnionFightTab/WarOnInfo IL：一三五宣战日、2级、角色1/2、0点前5分钟自动、截止当日23:59:59、先25首发。时区沿用本服Asia/Shanghai。"
	P13UnionFightStageDeclare int32 = 1
	P13UnionFightStageMatch   int32 = 2
	P13UnionFightTickInterval       = 30 * time.Second
)

const P13UnionFightMatchSource = "原uFGroupConfig15/30/30/25及A/B/C/D原UI映射；本地兼容：最大余数整数配额、按真实冻结总战力邻近配对、首次有效匹配分组后同周固定；原胜率权重/首次晚报名/轮空规则待审计，绝不写固定胜负。"
