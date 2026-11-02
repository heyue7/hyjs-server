package model

// CopyRunStatusRequest identifies the logged-in character and its current session.
type CopyRunStatusRequest struct {
	// PlayerID is the currently logged-in character.
	PlayerID int64 `json:"playerId" binding:"required"`
	// SessionHex is the 16-byte original session in lowercase hexadecimal.
	SessionHex string `json:"sessionHex" binding:"required"`
}

// CopyRunLeaveRequest confirms the exact Run previously returned by status.
type CopyRunLeaveRequest struct {
	// PlayerID is the currently logged-in character.
	PlayerID int64 `json:"playerId" binding:"required"`
	// SessionHex is the 16-byte original session in lowercase hexadecimal.
	SessionHex string `json:"sessionHex" binding:"required"`
	// RunToken is the lowercase SHA-256 of the observed Run.
	RunToken string `json:"runToken" binding:"required"`
}

// CopyRunRecoveryResponse exposes only the current Run identity and its token.
type CopyRunRecoveryResponse struct {
	// Active reports whether an incomplete Run exists.
	Active bool `json:"active"`
	// ChapterID is the active Run's chapter, or empty when inactive.
	ChapterID string `json:"chapterId"`
	// MissionID is the active Run's mission, or empty when inactive.
	MissionID string `json:"missionId"`
	// RunToken binds a later confirmation to the complete observed Run.
	RunToken string `json:"runToken"`
}
