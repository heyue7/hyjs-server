package model

// GameAccountRegisterRequest creates a new account in the existing original profile system.
type GameAccountRegisterRequest struct {
	Nickname        string `json:"nickname" binding:"required"`        // Nickname is the global account display name, separate from each realm character.
	Username        string `json:"username" binding:"required"`        // Username is 3-32 ASCII letters, digits, underscores or hyphens.
	Password        string `json:"password" binding:"required"`        // Password is at least 6 Unicode characters and at most 72 UTF-8 bytes and is never returned.
	ConfirmPassword string `json:"confirmPassword" binding:"required"` // ConfirmPassword must match Password exactly.
}

// GameAccountRegisterResponse leaves original login and starter-character creation unchanged.
type GameAccountRegisterResponse struct {
	Registered bool   `json:"registered"` // Registered is true only for a newly inserted original account.
	PlayerID   int64  `json:"playerId"`   // PlayerID is the existing profile identity; the starter character is not yet created.
	Username   string `json:"username"`   // Username is the new login identity.
}

// AdminGamePlayerCreateRequest creates a normal account and its original starter character.
type AdminGamePlayerCreateRequest struct {
	AccountID int64  `json:"accountId"`                 // AccountID selects an existing global account; username/password are then not required.
	ServerID  int64  `json:"serverId"`                  // ServerID selects a bound running realm; zero preserves legacy single-runtime callers.
	Username  string `json:"username"`                  // Username is the new unique login identity.
	Password  string `json:"password"`                  // Password is required and stored only as the original scrypt digest.
	Name      string `json:"name" binding:"required"`   // Name is the initial character display name.
	HeroID    string `json:"heroId"`                    // HeroID is an allowed original starter; empty defaults to hero_043.
	Reason    string `json:"reason" binding:"required"` // Reason is the finite operator explanation recorded in audit.
}
