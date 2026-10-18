package model

// AdminGameServerDTO describes an operator-managed realm without leaking endpoints.
type AdminGameServerDTO struct {
	ID           int64  `json:"id"`           // ID is the immutable realm number.
	Name         string `json:"name"`         // Name is the original login list display label.
	Enabled      bool   `json:"enabled"`      // Enabled is true when bound runtime accepts login.
	OpenAt       int64  `json:"openAt"`       // OpenAt is the UTC millisecond opening instant.
	RuntimeBound bool   `json:"runtimeBound"` // RuntimeBound is true only for a configured running deployment.
	Version      string `json:"version"`      // Version prevents stale overwrites.
}

// AdminGameServersResponse returns configured and planned realms.
type AdminGameServersResponse struct {
	Items []AdminGameServerDTO `json:"items"`
}

// AdminGameServerCreateRequest creates a planned realm; it cannot provision a runtime.
type AdminGameServerCreateRequest struct {
	Name   string `json:"name" binding:"required"`   // Name is a finite display name.
	OpenAt int64  `json:"openAt" binding:"required"` // OpenAt is the planned opening instant.
}

// AdminGameServerUpdateRequest updates metadata with optimistic concurrency.
type AdminGameServerUpdateRequest struct {
	Version string `json:"version" binding:"required"` // Version is the last read digest.
	Name    string `json:"name" binding:"required"`    // Name is the login display label.
	Enabled bool   `json:"enabled"`                    // Enabled=true permits login only with a bound runtime.
	OpenAt  int64  `json:"openAt" binding:"required"`  // OpenAt is an operator opening date, independent from existing character day-one anchors.
}
