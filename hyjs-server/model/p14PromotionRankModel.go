package model

import "encoding/json"

// P14PromotionRankImportRequest transfers one trusted source's actual score version.
type P14PromotionRankImportRequest struct {
	// ProtocolVersion is the named local federation version1.
	ProtocolVersion int64 `json:"protocolVersion" binding:"required"`
	// DeliveryID binds this exact source version to its single destination.
	DeliveryID string `json:"deliveryId" binding:"required"`
	// SourceServerID identifies the authenticated origin deployment.
	SourceServerID int64 `json:"sourceServerId" binding:"required"`
	// DestinationServerID is the only deployment permitted to acknowledge this delivery.
	DestinationServerID int64 `json:"destinationServerId" binding:"required"`
	// Snapshot contains the canonical qualified score version and frozen policy.
	Snapshot json.RawMessage `json:"snapshot" binding:"required"`
}

// P14PromotionRankImportResponse acknowledges only an exact persisted version.
type P14PromotionRankImportResponse struct {
	// DeliveryID echoes the exact committed delivery identity.
	DeliveryID string `json:"deliveryId"`
	// Confirmed uses false=unconfirmed and true=exact version persisted after commit.
	Confirmed bool `json:"confirmed"`
}
