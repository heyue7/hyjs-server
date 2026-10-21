package model

// RecycleSelectionRequest asks for original picker candidates without recycling them.
type RecycleSelectionRequest struct {
	PlayerID   int64  `json:"playerId" binding:"required"`   // PlayerID is the current role.
	SessionHex string `json:"sessionHex" binding:"required"` // SessionHex is the current 16-byte session encoded as hex.
	Kind       string `json:"kind" binding:"required"`       // Kind is hero, equipment or pet.
	Limit      int64  `json:"limit" binding:"required"`      // Limit is the original selection capacity, 1 through 4.
}

// RecycleSelectionResponse contains only currently eligible original instance GUIDs.
type RecycleSelectionResponse struct {
	GUIDs   []string `json:"guids"`   // GUIDs are sorted by original quality and GUID, capped by limit.
	Skipped int64    `json:"skipped"` // Skipped counts original-quality candidates excluded by actual eligibility.
}
