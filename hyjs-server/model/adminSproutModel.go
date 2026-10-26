package model

// AdminSproutSettingsResponse 返回按角色创建天数判定的小芽配置。
type AdminSproutSettingsResponse struct {
	NewCharacterEnabled *bool  `json:"newCharacterEnabled"` // null沿用已有新角色规则。
	ReturnEnabled       *bool  `json:"returnEnabled"`       // true开启回归资格。
	ReturnOfflineDays   *int64 `json:"returnOfflineDays"`   // 连续离线门槛天数。
	ReturnDays          *int64 `json:"returnDays"`          // 回归资格持续天数。
	Enabled             bool   `json:"enabled"`             // true启用资格规则；false停用。
	Days                *int64 `json:"days"`                // 创建后的前N天，null表示尚未配置。
	Icon                string `json:"icon"`                // 已有小芽标识资源。
	Version             string `json:"version"`             // 乐观并发版本。
}

// AdminSproutSettingsSaveRequest 保存全局小芽配置。
type AdminSproutSettingsSaveRequest struct {
	NewCharacterEnabled *bool  `json:"newCharacterEnabled"`        // null沿用已有新角色规则。
	ReturnEnabled       *bool  `json:"returnEnabled"`              // true开启回归资格。
	ReturnOfflineDays   *int64 `json:"returnOfflineDays"`          // 连续离线门槛天数。
	ReturnDays          *int64 `json:"returnDays"`                 // 回归资格持续天数。
	Enabled             bool   `json:"enabled"`                    // true启用；启用时必须填写days。
	Days                *int64 `json:"days"`                       // 1至3650，停用时可以留空。
	Version             string `json:"version" binding:"required"` // 原读取版本。
	Reason              string `json:"reason" binding:"required"`  // 修改原因。
}
