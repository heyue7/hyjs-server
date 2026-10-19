package model

// AdminActivityDefinitionDeleteRequest archives a reusable activity using its latest version.
type AdminActivityDefinitionDeleteRequest struct {
	Version string `json:"version" binding:"required"` // Version is the 64-character version returned by the template list.
}

// AdminActivityDefinitionDeleteResponse confirms archival while keeping issued snapshots and claims.
type AdminActivityDefinitionDeleteResponse struct {
	OK       bool `json:"ok"`       // OK true indicates the requested archive was committed.
	Archived bool `json:"archived"` // Archived true hides the template from future scheduling.
}
