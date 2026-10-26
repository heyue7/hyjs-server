package model

// SproutStatusRequest identifies the authenticated role and visible scene peers.
type SproutStatusRequest struct {
	PlayerID   int64   `json:"playerId" binding:"required"`
	SessionHex string  `json:"sessionHex" binding:"required"`
	PlayerIDs  []int64 `json:"playerIds" binding:"required"`
}

// SproutStatusPlayer contains only the authorized visible eligibility.
type SproutStatusPlayer struct {
	PlayerID  int64 `json:"playerId"`
	Active    bool  `json:"active"`
	ExpiresAt int64 `json:"expiresAt"`
}

// SproutStatusResponse carries real server time and expiry in milliseconds.
type SproutStatusResponse struct {
	ServerTime int64                `json:"serverTime"`
	Icon       string               `json:"icon"`
	Players    []SproutStatusPlayer `json:"players"`
}
