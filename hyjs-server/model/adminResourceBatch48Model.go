package model

// AdminInventoryBatchResource identifies a native inventory addition.
type AdminInventoryBatchResource struct {
	Category   string `json:"category"`   // Category is the native inventory category.
	ResourceID string `json:"resourceId"` // ResourceID is the original catalogue ID.
	Amount     int64  `json:"amount"`     // Amount is the positive addition, at most 100 instances.
}

// AdminBatchGrantRequest delivers up to 20 resources to one character.
type AdminBatchGrantRequest struct {
	PlayerID  int64                 `json:"playerId" binding:"required"`  // PlayerID is the recipient.
	Resources []AdminResourceAmount `json:"resources" binding:"required"` // Resources contains unique IDs and quantities.
	Reason    string                `json:"reason" binding:"required"`    // Reason is the operator's explanation.
	RequestID string                `json:"requestId" binding:"required"` // RequestID is a stable UUID for retries.
}

// AdminInventoryBatchRequest atomically adds selected resources to one character.
type AdminInventoryBatchRequest struct {
	Version   string                        `json:"version" binding:"required"`   // Version is the previously read inventory version.
	Resources []AdminInventoryBatchResource `json:"resources" binding:"required"` // Resources contains unique native additions.
	Reason    string                        `json:"reason" binding:"required"`    // Reason is retained in the audit.
	RequestID string                        `json:"requestId" binding:"required"` // RequestID identifies the complete frozen batch.
}
