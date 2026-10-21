package db

// ClientUpdateAction 保存客户端更新的独立运行事实。
type ClientUpdateAction struct {
	RequestID    string `gorm:"column:request_id;primaryKey;size:80" json:"requestId"`
	PayloadHash  string `gorm:"column:payload_hash;size:64" json:"payloadHash"`
	ActorID      int64  `gorm:"column:actor_id;not null" json:"actorID"`
	ResponseJSON string `gorm:"column:response_json;type:longtext" json:"responseJSON"`
	CreatedAt    int64  `gorm:"column:created_at;autoCreateTime:false" json:"createdAt"`
}

func (ClientUpdateAction) TableName() string { return "client_update_actions" }
