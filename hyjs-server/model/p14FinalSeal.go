package model

import "encoding/json"

// P14FinalSealImportRequest carries an authenticated source realm's frozen lock boundary.
type P14FinalSealImportRequest struct {
	// ProtocolVersion 1=first-final source seal.
	ProtocolVersion int32 `json:"protocolVersion" binding:"required"`
	// DeliveryID is the exact source payload and destination SHA256.
	DeliveryID string `json:"deliveryId" binding:"required"`
	// SourceServerID is the trusted originating realm, 1..32.
	SourceServerID int64 `json:"sourceServerId" binding:"required"`
	// DestinationServerID is the distinct receiving realm, 1..32.
	DestinationServerID int64 `json:"destinationServerId" binding:"required"`
	// Snapshot is the complete non-null seal object authenticated as raw bytes.
	Snapshot json.RawMessage `json:"snapshot" binding:"required"`
}

// P14FinalSealImportResponse confirms durable receipt without exposing profile data.
type P14FinalSealImportResponse struct {
	// DeliveryID is the acknowledged immutable delivery SHA256.
	DeliveryID string `json:"deliveryId"`
	// Confirmed false=not acknowledged, true=durably saved or already identical.
	Confirmed bool `json:"confirmed"`
}
