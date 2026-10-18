package model

// LocalPlayActivitiesStateRequest reads the current private activity directory.
type LocalPlayActivitiesStateRequest struct {
	// PlayerID is the original logged-in character.
	PlayerID int64 `json:"playerId" binding:"required"`
	// SessionHex is the current 16-byte game session encoded as hex.
	SessionHex string `json:"sessionHex" binding:"required"`
}

// LocalPlayActivitiesApplyRequest opens or refreshes an activity once.
type LocalPlayActivitiesApplyRequest struct {
	// PlayerID is the original logged-in character.
	PlayerID int64 `json:"playerId" binding:"required"`
	// SessionHex is the current 16-byte game session encoded as hex.
	SessionHex string `json:"sessionHex" binding:"required"`
	// Action is open or refresh.
	Action string `json:"action" binding:"required"`
	// ActivityID is all or an ID from the authenticated activity directory.
	ActivityID string `json:"activityId" binding:"required"`
	// RequestID binds one operation and is reused after an uncertain response.
	RequestID string `json:"requestId" binding:"required"`
}
