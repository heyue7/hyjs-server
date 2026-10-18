package model

import "encoding/json"

// P14QualificationImportRequest 原服签名的有限本地autoPass证书，携实际截止名册与资格来源，未携账号秘密。
type P14QualificationImportRequest struct {
	// ProtocolVersion 为内部具名证书协议版本1。
	ProtocolVersion int64 `json:"protocolVersion"`
	// DeliveryID 为同来源/赛季/资格SHA/目的服的确定性SHA256。
	DeliveryID string `json:"deliveryId"`
	// SourceServerID 为持有互信密钥的原服编号。
	SourceServerID int64 `json:"sourceServerId"`
	// DestinationServerID 为本证书唯一目的部署编号。
	DestinationServerID int64 `json:"destinationServerId"`
	// SeasonIndex 为实际已发布赛季。
	SeasonIndex int32 `json:"seasonIndex"`
	// Source 声明具名auto证书范围，不代表原私服服务间协议。
	Source string `json:"source"`
	// Boundary 为完整真实原服截止事实及其SHA。
	Boundary json.RawMessage `json:"boundary"`
	// Roster 为按playerID排序的全部原服截止阵容及联合身份SHA。
	Roster json.RawMessage `json:"roster"`
	// Qualification 为实际零场自动晋级证书及来源SHA。
	Qualification json.RawMessage `json:"qualification"`
}

// P14QualificationImportResponse 确认本目的服持久了精确签名证书。
type P14QualificationImportResponse struct {
	// DeliveryID 回显已持久精确证书身份。
	DeliveryID string `json:"deliveryId"`
	// Confirmed 为true时精确证书已提交。
	Confirmed bool `json:"confirmed"`
}
