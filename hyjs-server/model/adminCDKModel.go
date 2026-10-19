package model

// AdminCDKDTO exposes policy and counts without the redeemable plaintext code.
type AdminCDKDTO struct {
	ID         string                `json:"id"`         // ID is the original SHA256 key.
	Name       string                `json:"name"`       // Name is the operator's label.
	CodeHint   string                `json:"codeHint"`   // CodeHint is only the final four characters.
	Enabled    bool                  `json:"enabled"`    // Enabled permits redemption when true.
	Unlimited  bool                  `json:"unlimited"`  // Unlimited removes the date window when true.
	StartAt    int64                 `json:"startAt"`    // StartAt is the inclusive game-time millisecond boundary.
	EndAt      int64                 `json:"endAt"`      // EndAt is the exclusive game-time boundary.
	TotalLimit int64                 `json:"totalLimit"` // TotalLimit is the shared redemption quota; 0=unlimited, positive=maximum claims.
	Used       int64                 `json:"used"`       // Used counts committed original claims.
	Rewards    []AdminResourceAmount `json:"rewards"`    // Rewards are fixed original resources.
	Version    string                `json:"version"`    // Version protects concurrent operator edits.
	CreatedAt  int64                 `json:"createdAt"`  // CreatedAt is the real creation time.
}

// AdminCDKListResponse is a finite page of realm-owned policies.
type AdminCDKListResponse struct {
	Items []AdminCDKDTO `json:"items"` // Items contains this page.
	Total int64         `json:"total"` // Total is the matching count.
}

// AdminCDKCreateRequest explicitly generates codes with operator-selected rewards.
type AdminCDKCreateRequest struct {
	Name       string                `json:"name" binding:"required"`    // Name identifies the allocation.
	Rewards    []AdminResourceAmount `json:"rewards" binding:"required"` // Rewards must be grantable original resources.
	Count      int64                 `json:"count" binding:"required"`   // Count is 1..500.
	Prefix     string                `json:"prefix"`                     // Prefix is optional uppercase ASCII, at most 12 characters.
	Code       string                `json:"code"`                       // Code optionally supplies exactly one manual code.
	TotalLimit int64                 `json:"totalLimit"`                 // TotalLimit: 0=unlimited, 1..1000000=shared claim quota.
	Unlimited  bool                  `json:"unlimited"`                  // Unlimited=true omits both date boundaries.
	StartAt    int64                 `json:"startAt"`                    // StartAt is required when limited.
	EndAt      int64                 `json:"endAt"`                      // EndAt must follow StartAt when limited.
	Enabled    bool                  `json:"enabled"`                    // Enabled=true opens redemption.
	Reason     string                `json:"reason" binding:"required"`  // Reason records the operator's purpose.
}

// AdminCDKUpdateRequest keeps used rewards immutable while allowing policy edits.
type AdminCDKUpdateRequest struct {
	Name       string                `json:"name" binding:"required"`    // Name is the display label.
	Rewards    []AdminResourceAmount `json:"rewards" binding:"required"` // Rewards cannot change after a successful claim.
	TotalLimit int64                 `json:"totalLimit"`                 // TotalLimit: 0=unlimited; a positive quota cannot be below usage.
	Unlimited  bool                  `json:"unlimited"`                  // Unlimited=true removes the date restriction.
	StartAt    int64                 `json:"startAt"`                    // StartAt is inclusive when limited.
	EndAt      int64                 `json:"endAt"`                      // EndAt is exclusive when limited.
	Enabled    bool                  `json:"enabled"`                    // Enabled permits or stops future claims.
	Version    string                `json:"version" binding:"required"` // Version is the loaded snapshot token.
	Reason     string                `json:"reason" binding:"required"`  // Reason is included in audit history.
}

// AdminCDKCreateResponse contains the only plaintext delivery of newly generated codes.
type AdminCDKCreateResponse struct {
	Items []AdminCDKDTO `json:"items"` // Items are persisted policies.
	Codes []string      `json:"codes"` // Codes must be copied before the operator closes the result.
}

// AdminCDKClaimDTO identifies an actual original redemption without credentials.
type AdminCDKClaimDTO struct {
	PlayerID   int64  `json:"playerId"`   // PlayerID is the local role ID.
	PlayerName string `json:"playerName"` // PlayerName is the role's current name.
	ClaimedAt  int64  `json:"claimedAt"`  // ClaimedAt is the original committed claim time.
}

// AdminCDKClaimsResponse is a page from the original membership claim ledger.
type AdminCDKClaimsResponse struct {
	Items []AdminCDKClaimDTO `json:"items"` // Items contains committed redemptions.
	Total int64              `json:"total"` // Total is the redeemed count.
}
