package model

// P13GuildMapRequest identifies the authenticated role loading its current war map.
type P13GuildMapRequest struct {
	// PlayerID is the current realm role.
	PlayerID int64 `json:"playerId" binding:"required"`
	// SessionHex is the current original protocol session encoded as 32 hex characters.
	SessionHex string `json:"sessionHex" binding:"required"`
}

// P13GuildMapResponse selects the resource adapter for an already accepted field entry.
type P13GuildMapResponse struct {
	// MapProjection is empty for the legacy map or re-flat-v1 for the frozen Re-compatible plane.
	MapProjection string `json:"mapProjection"`
}
