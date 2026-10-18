package model

// ServerGameClockView describes the calendar shared by every player in the server.
type ServerGameClockView struct {
	Enabled    bool   `json:"enabled"`    // Enabled is true after the server calendar schema is initialized.
	Scope      string `json:"scope"`      // Scope is always server.
	ServerID   int64  `json:"serverId"`   // ServerID identifies the configured game server.
	GameTimeMS int64  `json:"gameTimeMs"` // GameTimeMS is the shared game calendar time.
	RealTimeMS int64  `json:"realTimeMs"` // RealTimeMS remains the wall clock used for accounting and sessions.
	OffsetDays int64  `json:"offsetDays"` // OffsetDays is the signed whole-day calendar adjustment.
	Revision   int64  `json:"revision"`   // Revision increases once for every committed new adjustment.
	Timezone   string `json:"timezone"`   // Timezone is Asia/Shanghai.
	TestOnly   bool   `json:"testOnly"`   // TestOnly marks the local game operation capability.
}

// ServerGameClockChangeRequest adjusts the whole server calendar exactly once.
type ServerGameClockChangeRequest struct {
	DeltaDays int64  `json:"deltaDays" binding:"required"` // DeltaDays is -1 or 1.
	Revision  int64  `json:"revision"`                     // Revision is the previously read server calendar revision.
	RequestID string `json:"requestId" binding:"required"` // RequestID is the idempotency UUID.
}
