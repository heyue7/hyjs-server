package model

// TerminalWeeklyRule configures a real Beijing weekday delivery (Monday=1, Sunday=7).
type TerminalWeeklyRule struct {
	ID      string `json:"id"`      // ID remains stable across edits.
	Weekday int64  `json:"weekday"` // Weekday is 1 through 7.
	Points  int64  `json:"points"`  // Points is the whole overclock point award.
	Enabled bool   `json:"enabled"` // Enabled permits future deliveries.
}

// TerminalSettings is the per-realm settings request and response.
type TerminalSettings struct {
	Version         string               `json:"version"`         // Version is the optimistic concurrency digest.
	SignupPoints    int64                `json:"signupPoints"`    // SignupPoints applies only to a new registration.
	BurstEnabled    bool                 `json:"burstEnabled"`    // BurstEnabled controls the daily action.
	BurstCostPoints int64                `json:"burstCostPoints"` // BurstCostPoints is charged before awarding.
	BurstMinPoints  int64                `json:"burstMinPoints"`  // BurstMinPoints is the inclusive random minimum.
	BurstMaxPoints  int64                `json:"burstMaxPoints"`  // BurstMaxPoints is the inclusive random maximum.
	Weekly          []TerminalWeeklyRule `json:"weekly"`          // Weekly starts empty.
}

// TerminalDistributeRequest freezes one idempotent operational distribution.
type TerminalDistributeRequest struct {
	RequestID string `json:"requestId" binding:"required"` // RequestID is a UUID reused for retries.
	Points    int64  `json:"points"`                       // Points is the whole award per account.
	Reason    string `json:"reason" binding:"required"`    // Reason is the audit explanation.
	Version   string `json:"version" binding:"required"`   // Version is the current settings digest.
}

// TerminalDistributeResponse reports durable batch progress.
type TerminalDistributeResponse struct {
	BatchID        string `json:"batchId"`        // BatchID identifies the frozen distribution.
	Recipients     int64  `json:"recipients"`     // Recipients counts unique eligible accounts.
	Credited       int64  `json:"credited"`       // Credited counts committed accounts.
	AlreadyApplied bool   `json:"alreadyApplied"` // AlreadyApplied indicates a reused request.
}

// TerminalDistributionItem projects one distribution without recipient private data.
type TerminalDistributionItem struct {
	BatchID    string `json:"batchId"`    // BatchID identifies the batch.
	Points     int64  `json:"points"`     // Points is the frozen award.
	Recipients int64  `json:"recipients"` // Recipients is the frozen count.
	Credited   int64  `json:"credited"`   // Credited is committed progress.
	CreatedAt  int64  `json:"createdAt"`  // CreatedAt is real Unix milliseconds.
	Kind       string `json:"kind"`       // Kind is admin_manual or admin_weekly.
	Status     string `json:"status"`     // Status is pending, blocked (retryable operational failure), or complete.
}

// TerminalDistributionsResponse is the paginated durable batch history.
type TerminalDistributionsResponse struct {
	Items []TerminalDistributionItem `json:"items"` // Items are newest first.
	Total int64                      `json:"total"` // Total counts selected-realm batches.
}

// TerminalIncomeEntry is an immutable income card.
type TerminalIncomeEntry struct {
	ID          string `json:"id"`          // ID is unique within the account.
	Label       string `json:"label"`       // Label was frozen at credit time.
	AmountCents int64  `json:"amountCents"` // AmountCents uses 100 units per point.
	CreatedAt   int64  `json:"createdAt"`   // CreatedAt uses real Unix milliseconds.
	Revision    int64  `json:"revision"`    // Revision increases with committed wallet mutations.
	Source      string `json:"source"`      // Source is admin_manual, admin_weekly, signup or burst.
}

// TerminalState is the authenticated daily action and unread income projection.
type TerminalState struct {
	BurstEnabled         bool                  `json:"burstEnabled"`         // BurstEnabled reflects the selected realm.
	BurstCostPoints      int64                 `json:"burstCostPoints"`      // BurstCostPoints is the current charge.
	BurstMinPoints       int64                 `json:"burstMinPoints"`       // BurstMinPoints is the current minimum.
	BurstMaxPoints       int64                 `json:"burstMaxPoints"`       // BurstMaxPoints is the current maximum.
	CanBurst             bool                  `json:"canBurst"`             // CanBurst checks qualification and available balance.
	BurstDay             string                `json:"burstDay"`             // BurstDay is the real Beijing YYYY-MM-DD.
	UnreadCount          int64                 `json:"unreadCount"`          // UnreadCount includes every unconfirmed new income.
	LatestIncomeRevision int64                 `json:"latestIncomeRevision"` // LatestIncomeRevision is safe to acknowledge after display.
	IncomeEntries        []TerminalIncomeEntry `json:"incomeEntries"`        // IncomeEntries contains at most 20 newest cards.
}

// LocalWalletBurstRequest executes the once-daily account action.
type LocalWalletBurstRequest struct {
	PlayerID   int64  `json:"playerId" binding:"required"`   // PlayerID identifies the authenticated role.
	SessionHex string `json:"sessionHex" binding:"required"` // SessionHex is the original session.
	RequestID  string `json:"requestId" binding:"required"`  // RequestID is a retry-stable UUID.
}

// LocalWalletSeenRequest confirms only income already returned to this account.
type LocalWalletSeenRequest struct {
	PlayerID     int64  `json:"playerId" binding:"required"`   // PlayerID identifies the authenticated role.
	SessionHex   string `json:"sessionHex" binding:"required"` // SessionHex is the original session.
	SeenRevision int64  `json:"seenRevision"`                  // SeenRevision cannot exceed the delivered high watermark.
}
