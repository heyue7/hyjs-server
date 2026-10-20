package model

// AdminAudience 是后台勾选的操作对象，预览时冻结为具体角色。
type AdminAudience struct {
	// Mode 是选择方式：all=全部玩家（跨分页，预览时冻结本服全部有效角色），players=仅勾选玩家。
	Mode string `json:"mode" binding:"required"`
	// PlayerIDs 是勾选的角色ID；Mode=players 时必填且去重，Mode=all 时必须为空。
	PlayerIDs []int64 `json:"playerIds"`
}
