package model

// AdminAchievementDTO projects the actual claim rule and its original UI type.
type AdminAchievementDTO struct {
	ID      string                `json:"id"`      // Stable rule ID, server generated for new entries.
	Name    string                `json:"name"`    // Operator display name; original client uses the original type text.
	Kind    string                `json:"kind"`    // One of the 36 original condition IDs from the kinds catalogue; legacy rules remain visible.
	Type    int32                 `json:"type"`    // Original NGUI achievement type.
	Target  int32                 `json:"target"`  // Required progress.
	Rewards []AdminResourceAmount `json:"rewards"` // Explicit configured payouts, never auto populated.
	Source  string                `json:"source"`  // Configuration provenance.
	Enabled bool                  `json:"enabled"` // false stops creating new player snapshots, preserving existing ones.
	Version string                `json:"version"` // Rule version.
	// Supported indicates an authoritative progress consumer is available.
	Supported bool `json:"supported"`
	// Notes explains the original trigger and any explicit local aggregation boundary.
	Notes string `json:"notes"`
}

// AdminAchievementKind declares only progress consumers supported by the original UI.
type AdminAchievementKind struct {
	ID    string `json:"id"`    // Progress identifier.
	Label string `json:"label"` // Original UI category.
	Type  int32  `json:"type"`  // Original UI type.
	// Supported is false when original conditions exist but the necessary actual scenario is unavailable.
	Supported bool `json:"supported"`
	// Notes records original trigger text and actual progress source.
	Notes string `json:"notes"`
	// FixedTarget is nonzero only for original conditions with an explicit fixed target, such as 100% exploration.
	FixedTarget int32 `json:"fixedTarget,omitempty"`
}

// AdminAchievementsResponse returns empty original content honestly until configured.
type AdminAchievementsResponse struct {
	Items []AdminAchievementDTO  `json:"items"` // Rule list.
	Kinds []AdminAchievementKind `json:"kinds"` // Supported progress kinds.
	Notes string                 `json:"notes"` // Original source limitations.
}

// AdminTitleEffect is an original additive stat value, not an item reward.
type AdminTitleEffect struct {
	Key   string `json:"key"`   // hp, att, pdef, mdef, cri, hit, dodge, antiCri, criFactor, damageAdd, damageReduce.
	Value int64  `json:"value"` // Additive stat value.
}

// AdminTitleDTO retains original title identity and resources.
type AdminTitleDTO struct {
	ID          string             `json:"id"`          // Original title ID.
	Name        string             `json:"name"`        // Original localized name.
	Type        int64              `json:"type"`        // Original qualification category, immutable.
	Param       string             `json:"param"`       // Original qualification parameter.
	Effects     []AdminTitleEffect `json:"effects"`     // Future grant effect values.
	EffectTime  int64              `json:"effectTime"`  // Hours; 0 means permanent.
	Show        bool               `json:"show"`        // Original list presentation flag.
	Enabled     bool               `json:"enabled"`     // Future qualification enabled.
	Version     string             `json:"version"`     // Original plus override version.
	Notes       string             `json:"notes"`       // Protected qualification/history notes.
	IconURL     string             `json:"iconUrl"`     // Original title-ID sprite exported from the client atlas.
	Requirement string             `json:"requirement"` // Human-readable current qualification, including any target override.
}

// AdminTitlesResponse lists the original resource-backed title catalogue.
type AdminTitlesResponse struct {
	Items    []AdminTitleDTO        `json:"items"`    // Original titles with optional future-grant overrides.
	Chapters []AdminCatalogueOption `json:"chapters"` // Original main-story chapters for the chapter qualification selector.
}
