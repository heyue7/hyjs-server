package model

// P13GuildWarStateRequest identifies a current original-protocol account session.
type P13GuildWarStateRequest struct {
	// PlayerID is the authenticated role in this realm.
	PlayerID int64 `json:"playerId" binding:"required"`
	// SessionHex is exactly the 16-byte session encoded as 32 hexadecimal characters.
	SessionHex string `json:"sessionHex" binding:"required"`
}

// P13GuildWarStateResponse exposes only the live new-admission master switch.
type P13GuildWarStateResponse struct {
	// WarEnabled=false hides new entry; accepted declarations and rounds continue.
	WarEnabled bool `json:"warEnabled"`
}
