package model

// AdminArenaCoinRange is the inclusive per-card regular victory coin interval.
type AdminArenaCoinRange struct {
	Min int64 `json:"min"` // Inclusive minimum,1..2147483647.
	Max int64 `json:"max"` // Inclusive maximum,at least min.
}

// AdminArenaPrizePoolItem is one typed original operator-pool entry.
type AdminArenaPrizePoolItem struct {
	ResourceID string `json:"resourceId"` // Original allowed resource ID.
	Amount     int64  `json:"amount"`     // Quantity,1..2147483647.
	Weight     int64  `json:"weight"`     // Relative weight,1..1000000000.
}

// AdminArenaPrizePoolSettings edits the existing separately enabled operator pool.
type AdminArenaPrizePoolSettings struct {
	Enabled    bool                      `json:"enabled"`    // Enables only this optional pool.
	StartAt    int64                     `json:"startAt"`    // Unix milliseconds; zero for disabled draft.
	EndAt      int64                     `json:"endAt"`      // Exclusive Unix milliseconds in the same month.
	MinLevel   int64                     `json:"minLevel"`   // Minimum player level,10..2147483647.
	DailyWins  int64                     `json:"dailyWins"`  // Maximum custom awards per day,1..100.
	WindowWins int64                     `json:"windowWins"` // Maximum monthly custom awards,1..3100.
	Items      []AdminArenaPrizePoolItem `json:"items"`      // One to32 typed reward entries.
}

// AdminArenaPrizeResponse is the versioned concrete arena reward editor snapshot.
type AdminArenaPrizeResponse struct {
	Version            string                      `json:"version"`            // Last-read optimistic concurrency digest.
	RegularCoin        AdminArenaCoinRange         `json:"regularCoin"`        // Persistent regular coin range.
	OperatorPool       AdminArenaPrizePoolSettings `json:"operatorPool"`       // Optional existing operator pool.
	OperatorPoolFrozen bool                        `json:"operatorPoolFrozen"` // Current month has an immutable source offer.
	Notes              string                      `json:"notes"`              // Chinese guidance for future versus accepted rewards.
	AllowedResourceIDs []string                    `json:"allowedResourceIds"` // IDs searchable through the existing catalog API.
}

// AdminArenaPrizeSaveRequest saves the regular range and optionally the custom pool.
type AdminArenaPrizeSaveRequest struct {
	Version      string                       `json:"version" binding:"required"` // Required current snapshot digest.
	RegularCoin  AdminArenaCoinRange          `json:"regularCoin"`                // Required inclusive range.
	OperatorPool *AdminArenaPrizePoolSettings `json:"operatorPool,omitempty"`     // Omit to preserve the optional pool.
}
