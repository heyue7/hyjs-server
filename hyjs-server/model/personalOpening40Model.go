package model

// AdminPersonalOpening40DTO describes one realm's persistent personal growth rule.
type AdminPersonalOpening40DTO struct {
	ID           string            `json:"id"`           // ID is the stable bootstrap occurrence.
	Name         string            `json:"name"`         // Name follows the original client title.
	Mode         string            `json:"mode"`         // Mode is one of the nine supported period consumers.
	StartDay     int64             `json:"startDay"`     // StartDay is one-based from character creation.
	DurationDays int64             `json:"durationDays"` // DurationDays is the individual activity length.
	ClaimDays    int64             `json:"claimDays"`    // ClaimDays is the claim grace after completion closes.
	Enabled      bool              `json:"enabled"`      // Enabled false pauses access without clearing history.
	Version      string            `json:"version"`      // Version protects concurrent edits.
	Form         AdminActivityForm `json:"form"`         // Form contains future characters' frozen thresholds and resources.
}

// AdminPersonalOpening40SaveRequest edits future personal instances only.
type AdminPersonalOpening40SaveRequest struct {
	Version      string            `json:"version" binding:"required"` // Version must match the current rule.
	StartDay     int64             `json:"startDay"`                   // StartDay ranges from 1 to 366.
	DurationDays int64             `json:"durationDays"`               // DurationDays ranges from 1 to 366.
	ClaimDays    int64             `json:"claimDays"`                  // ClaimDays ranges from 0 to 90.
	Enabled      bool              `json:"enabled"`                    // Enabled true permits the rule's owned instances.
	Form         AdminActivityForm `json:"form" binding:"required"`    // Form uses the existing typed milestone editor.
}

// AdminPersonalOpening40ListResponse returns the selected realm's personal rules.
type AdminPersonalOpening40ListResponse struct {
	Items []AdminPersonalOpening40DTO `json:"items"` // Items contains only the persistent personal opening catalogue.
}
