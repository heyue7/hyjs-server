package model

// AdminActivityDefinitionDTO is a global reusable typed template; dates live in realm occurrences.
type AdminActivityDefinitionDTO struct {
	ID               string                    `json:"id"`               // ID is the global template identity.
	Name             string                    `json:"name"`             // Name is its readable operator label.
	Mode             string                    `json:"mode"`             // Mode follows the supported original consumer catalogue.
	Form             AdminActivityForm         `json:"form"`             // Form contains mode-specific resources and conditions.
	DefaultDays      int64                     `json:"defaultDays"`      // DefaultDays is the initial calendar length.
	ClaimDays        int64                     `json:"claimDays"`        // ClaimDays is the extra claim grace period.
	Enabled          bool                      `json:"enabled"`          // Enabled true allows scheduling, false retains a draft.
	Version          string                    `json:"version"`          // Version protects concurrent template editing.
	Availability     AdminActivityAvailability `json:"availability"`     // Availability exposes incomplete or unavailable original content.
	SourceActivityID string                    `json:"sourceActivityId"` // SourceActivityID identifies an imported legacy configuration.
}

// AdminActivityDefinitionSaveRequest saves global rules without scheduling an activity.
type AdminActivityDefinitionSaveRequest struct {
	// RecruitSettings 随活动原子保存当前同类的全局招募次数和保底；未提交时保持原规则。
	RecruitSettings *AdminRecruitSettingsSaveRequest `json:"recruitSettings,omitempty"`
	Name            string                           `json:"name" binding:"required"` // Name is the readable title.
	Form            AdminActivityForm                `json:"form" binding:"required"` // Form is the existing typed mode editor.
	DefaultDays     int64                            `json:"defaultDays"`             // DefaultDays ranges from 1 through 366.
	ClaimDays       int64                            `json:"claimDays"`               // ClaimDays ranges from 0 through 90.
	Enabled         bool                             `json:"enabled"`                 // Enabled true requires a usable complete template.
	Version         string                           `json:"version"`                 // Version is empty for creation.
}

// AdminActivityOccurrenceDTO is one realm calendar bar.
type AdminActivityOccurrenceDTO struct {
	StartDay     int64 `json:"startDay"`     // StartDay is one-based in this realm's character life, never account registration.
	DurationDays int64 `json:"durationDays"` // DurationDays is the relative activity window; the original opening task structure remains fourteen days.
	ClaimDays    int64 `json:"claimDays"`    // ClaimDays is the relative post-activity claim grace.

	Timing          string            `json:"timing"`          // Timing is calendar for absolute realm dates, character-day for each role's fixed opening days.
	Form            AdminActivityForm `json:"form"`            // Form is this occurrence's exact stored content; newer template edits never replace it.
	ID              string            `json:"id"`              // ID is the stable occurrence identity, empty for unsaved rows.
	TemplateID      string            `json:"templateId"`      // TemplateID references a global template.
	TemplateVersion string            `json:"templateVersion"` // TemplateVersion confirms the exact global rules selected for new publication.
	Name            string            `json:"name"`            // Name is the published title.
	Mode            string            `json:"mode"`            // Mode identifies the original UI and consumer.
	StartAt         int64             `json:"startAt"`         // StartAt is an inclusive millisecond boundary.
	EndAt           int64             `json:"endAt"`           // EndAt is an exclusive millisecond boundary.
	ClaimEndAt      int64             `json:"claimEndAt"`      // ClaimEndAt preserves a separate claim grace window.
	Lane            int64             `json:"lane"`            // Lane is a visual row, 0 through 31.
	Enabled         bool              `json:"enabled"`         // Enabled true publishes the slot.
	Frozen          bool              `json:"frozen"`          // Frozen true protects issued rules and dates.
	Version         string            `json:"version"`         // Version protects a single occurrence identity.
	ConsumerID      string            `json:"consumerId"`      // ConsumerID links the original gameplay consumer for audit.
}

// AdminActivityScheduleResponse returns the whole realm calendar so saving never drops hidden dates.
type AdminActivityScheduleResponse struct {
	ServerID   int64                        `json:"serverId"`   // ServerID is the selected managed realm.
	Managed    bool                         `json:"managed"`    // Managed true means this realm has adopted its calendar; an empty calendar closes the supported legacy entrances.
	GameTimeMs int64                        `json:"gameTimeMs"` // GameTimeMs is the shared game calendar now, including approved day offsets.
	Version    string                       `json:"version"`    // Version protects atomic calendar replacement.
	Items      []AdminActivityOccurrenceDTO `json:"items"`      // Items includes all current and future nonarchived slots.
	Warnings   []string                     `json:"warnings"`   // Warnings are visible nonblocking compatibility notes.
}

// AdminActivityScheduleSaveRequest atomically saves one realm calendar.
type AdminActivityScheduleSaveRequest struct {
	Version string                       `json:"version" binding:"required"` // Version must match the latest whole-calendar read.
	Items   []AdminActivityOccurrenceDTO `json:"items"`                      // Items is the desired complete calendar, maximum 1000.
}

// AdminActivitySchedulePreviewRequest checks a proposed realm calendar without writing.
type AdminActivitySchedulePreviewRequest struct {
	Version string                       `json:"version" binding:"required"` // Version protects preview relevance.
	Items   []AdminActivityOccurrenceDTO `json:"items"`                      // Items is the desired complete calendar.
}

// AdminActivityScheduleConflict is a concrete actionable overlap or frozen-state error.
type AdminActivityScheduleConflict struct {
	Index      int64  `json:"index"`      // Index identifies the proposed calendar item.
	OtherIndex int64  `json:"otherIndex"` // OtherIndex is the overlapping item, or -1.
	Message    string `json:"message"`    // Message describes the original consumer limitation.
}

// AdminActivitySchedulePreviewResponse is a reviewable no-write proposal result.
type AdminActivitySchedulePreviewResponse struct {
	Valid     bool                            `json:"valid"`     // Valid true allows atomic publish.
	Conflicts []AdminActivityScheduleConflict `json:"conflicts"` // Conflicts must be resolved before save.
	Warnings  []string                        `json:"warnings"`  // Warnings do not block independently supported parallel modes.
}

// AdminActivityPatternEntry uses day offsets from the date where a pattern is dropped.
type AdminActivityPatternEntry struct {
	TemplateID   string `json:"templateId"`   // TemplateID references a reusable global template.
	StartDay     int64  `json:"startDay"`     // StartDay is zero-based, 0 through 365.
	DurationDays int64  `json:"durationDays"` // DurationDays is 1 through 366.
	ClaimDays    int64  `json:"claimDays"`    // ClaimDays is 0 through 90.
	Lane         int64  `json:"lane"`         // Lane is a visual row, 0 through 31.
}

// AdminActivityPatternDTO is a reusable operator-maintained schedule scheme.
type AdminActivityPatternDTO struct {
	ID        string                      `json:"id"`        // ID is the pattern identity.
	Name      string                      `json:"name"`      // Name describes the rotation.
	CycleDays int64                       `json:"cycleDays"` // CycleDays determines repeated fill spacing.
	Entries   []AdminActivityPatternEntry `json:"entries"`   // Entries may run in parallel.
	Version   string                      `json:"version"`   // Version protects concurrent scheme edits.
}

// AdminActivityPatternSaveRequest creates or edits a reusable relative-day scheme.
type AdminActivityPatternSaveRequest struct {
	Name      string                      `json:"name" binding:"required"` // Name is the readable scheme label.
	CycleDays int64                       `json:"cycleDays"`               // CycleDays ranges from 1 through 366.
	Entries   []AdminActivityPatternEntry `json:"entries"`                 // Entries is a typed sequence, maximum 100.
	Version   string                      `json:"version"`                 // Version is empty for creation.
}

// AdminActivityCostResourcesResponse contains only resources accepted by the original commerce consumer.
type AdminActivityCostResourcesResponse struct {
	Total  int64                `json:"total"`  // Total counts all legal costs matching the requested filters.
	Facets AdminCatalogueFacets `json:"facets"` // Facets contains labels present in the legal-cost catalogue.
	Items  []AdminItemDTO       `json:"items"`  // Items carries readable names and original thumbnails for valid positive-cost resources.
}
