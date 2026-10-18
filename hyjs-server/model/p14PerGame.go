package model

import "encoding/json"

// P14PerGameImportRequest carries only the source server's signed internal snapshot.
// The original81030 player request has no added fields or client-chosen game index.
type P14PerGameImportRequest struct {
	// ProtocolVersion is1 for this separate per-game formation/seal contract.
	ProtocolVersion int64 `json:"protocolVersion" binding:"required"`
	// DeliveryID is the exact64hex digest of the signed snapshot and joint realms.
	DeliveryID string `json:"deliveryId" binding:"required"`
	// SourceServerID is the trusted source realm1..32.
	SourceServerID int64 `json:"sourceServerId" binding:"required"`
	// DestinationServerID is the other trusted receiving realm1..32.
	DestinationServerID int64 `json:"destinationServerId" binding:"required"`
	// Snapshot is a non-null JSON object with internal kind1 formation or2 origin seal.
	Snapshot json.RawMessage `json:"snapshot" binding:"required"`
}

// P14PerGameImportResponse acknowledges only a durably imported exact delivery.
type P14PerGameImportResponse struct {
	// DeliveryID is the same exact64hex signed delivery digest.
	DeliveryID string `json:"deliveryId"`
	// Confirmed is false before persistence, true after durable import or exact retry.
	Confirmed bool `json:"confirmed"`
}
