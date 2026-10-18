package model

// P14SnapshotImportRequest 是原服内部签名快照，玩家客户端不能设置这些可信身份字段。
type P14SnapshotImportRequest struct {
	// ProtocolVersion 协议版本：1=原报名快照，2=真实晋级资格阵容，3=真实前32和原决赛轮次绑定的阵容。
	ProtocolVersion int32 `json:"protocolVersion"`
	// DeliveryID 由赛季、联合身份、修订号和目的服计算的唯一SHA256单号。
	DeliveryID string `json:"deliveryId"`
	// SourceServerID 可信启动配置中的原服号。
	SourceServerID int64 `json:"sourceServerId"`
	// DestinationServerID 互信目的服号。
	DestinationServerID int64 `json:"destinationServerId"`
	// SeasonIndex 原赛季届数。
	SeasonIndex int32 `json:"seasonIndex"`
	// PlayerID 来源服已认证本地角色号。
	PlayerID int64 `json:"playerId"`
	// Revision 原服持久化递增快照修订号。
	Revision int64 `json:"revision"`
	// CapturedAt 来源服采集Unix毫秒时间。
	CapturedAt int64 `json:"capturedAt"`
	// FightPower 本次真实冻结阵容战力。
	FightPower int32 `json:"fightPower"`
	// Formation 原FormationSimpleInfo二进制。
	Formation []byte `json:"formation"`
	// Battle 原BattlePlayer二进制。
	Battle []byte `json:"battle"`
	// SnapshotHash 阵容和战斗快照联合SHA256。
	SnapshotHash string `json:"snapshotHash"`
	// ConfigurationHash 来源配置内容SHA256。
	ConfigurationHash string `json:"configurationHash"`
	// ClientVersion 原客户端版本SHA256。
	ClientVersion string `json:"clientVersion"`
	// EngineVersion 来源引擎版本。
	EngineVersion string `json:"engineVersion"`
	// RoundStage 用途3的原实际开战阶段；8=32进16第一轮，其它轮次尚未开放。
	RoundStage int32 `json:"roundStage,omitempty"`
	// FinalQualificationHash 用途3对应的完整跨服前32共同来源SHA256。
	FinalQualificationHash string `json:"finalQualificationHash,omitempty"`
	// FinalOfferHash 用途3对应的预发布政策、原轮次和五分钟前锁定来源SHA256。
	FinalOfferHash string `json:"finalOfferHash,omitempty"`
}

// P14SnapshotImportResponse 仅返回已确认单号，不外发阵容、密钥或账号信息。
type P14SnapshotImportResponse struct {
	// DeliveryID 由赛季、联合身份、修订号和目的服计算的唯一SHA256单号。
	DeliveryID string `json:"deliveryId"`
	// Confirmed true=载荷已持久化导入或此前已确认。
	Confirmed bool `json:"confirmed"` // true=载荷已持久化导入或此前已确认。
}
