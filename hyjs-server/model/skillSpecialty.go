package model

// SkillSpecialtyReceipt 保存专精原请求和原回包，角色存档与扣费在同一事务提交。
type SkillSpecialtyReceipt struct {
	Timestamp int64  `json:"timestamp"`
	Request   []byte `json:"request"`
	Response  []byte `json:"response"`
}
