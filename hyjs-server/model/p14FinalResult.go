package model

import "encoding/json"

// P14FinalResultImportRequest carries an exact signed original-origin snapshot.
type P14FinalResultImportRequest struct {
	// ProtocolVersion is 1 for this independent canonical-result envelope.
	ProtocolVersion int64 `json:"protocolVersion" binding:"required"`
	// DeliveryID is the lowercase SHA256 identity of this destination delivery.
	DeliveryID string `json:"deliveryId" binding:"required"`
	// SourceServerID is the canonical-left originating realm, in 1..32.
	SourceServerID int64 `json:"sourceServerId" binding:"required"`
	// DestinationServerID is the distinct receiving realm, in 1..32.
	DestinationServerID int64 `json:"destinationServerId" binding:"required"`
	// Snapshot is a non-null compressed-data object with a bounded decoded size.
	Snapshot json.RawMessage `json:"snapshot" binding:"required"`
}

// P14FinalResultImportResponse acknowledges only a durable imported result.
type P14FinalResultImportResponse struct {
	// DeliveryID identifies the exact durable destination receipt.
	DeliveryID string `json:"deliveryId"`
	// Confirmed is false=not acknowledged, true=persisted exactly once.
	Confirmed bool `json:"confirmed"`
}
