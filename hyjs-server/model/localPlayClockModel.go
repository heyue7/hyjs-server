package model

// LocalPlayTimeView shows the player's play calendar and unchanged actual time.
type LocalPlayTimeView struct {
	// GameTimeMS is the current personal play-calendar Unix time in milliseconds.
	GameTimeMS int64 `json:"gameTimeMs"`
	// RealTimeMS is the actual server Unix time in milliseconds.
	RealTimeMS int64 `json:"realTimeMs"`
	// OffsetDays is the signed calendar-day difference from actual time.
	OffsetDays int64 `json:"offsetDays"`
	// Revision counts successful distinct calendar operations.
	Revision int64 `json:"revision"`
	// Timezone is the display calendar zone, Asia/Shanghai.
	Timezone string `json:"timezone"`
	// TestOnly is true for these local single-player tools.
	TestOnly bool `json:"testOnly"`
}

// LocalPlayTimeStateRequest reads the authenticated personal play calendar.
type LocalPlayTimeStateRequest struct {
	// PlayerID is the original logged-in character ID.
	PlayerID int64 `json:"playerId" binding:"required"`
	// SessionHex is the current original 16-byte session encoded as hex.
	SessionHex string `json:"sessionHex" binding:"required"`
}

// LocalPlayTimeChangeRequest changes the current player's play calendar once.
type LocalPlayTimeChangeRequest struct {
	// PlayerID is the original logged-in character ID.
	PlayerID int64 `json:"playerId" binding:"required"`
	// SessionHex is the current original 16-byte session encoded as hex.
	SessionHex string `json:"sessionHex" binding:"required"`
	// Action is previousDay, nextDay or reset.
	Action string `json:"action" binding:"required"`
	// RequestID is reused when retrying an uncertain result.
	RequestID string `json:"requestId" binding:"required"`
}
