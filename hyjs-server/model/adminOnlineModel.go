package model

// AdminOnlineServer 是同一连接快照中的区服统计。
type AdminOnlineServer struct {
	// ServerID 是区服ID。
	ServerID int64 `json:"serverId"`
	// Name 是区服名称。
	Name string `json:"name"`
	// Online 是有效在线角色数。
	Online int64 `json:"online"`
	// Connected 表示 false=当前服务未接入，true=当前服务已接入。
	Connected bool `json:"connected"`
}

// AdminOnlinePlayer 是在线角色的有限展示字段。
type AdminOnlinePlayer struct {
	// ID 是角色ID。
	ID int64 `json:"id"`
	// ServerID 是角色所属区服。
	ServerID int64 `json:"serverId"`
	// Name 是角色昵称。
	Name string `json:"name"`
	// Level 是角色等级。
	Level int64 `json:"level"`
	// VIP 是角色VIP等级。
	VIP int64 `json:"vip"`
}

// AdminOnlinePlayersResponse 返回连接快照统计和筛选后的分页列表。
type AdminOnlinePlayersResponse struct {
	// Servers 包含全部已登记区服。
	Servers []AdminOnlineServer `json:"servers"`
	// Items 是本页在线角色。
	Items []AdminOnlinePlayer `json:"items"`
	// Total 是筛选后的在线角色总数。
	Total int64 `json:"total"`
	// ObservedAt 是快照时间，Unix毫秒。
	ObservedAt int64 `json:"observedAt"`
}
