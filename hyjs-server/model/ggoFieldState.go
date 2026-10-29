package model

// GGOFieldState holds only the current carried chips and small recovery baselines.
// Durable action and trade evidence belongs in its own receipts, not this state.
type GGOFieldState struct {
	Chips             [6]int64              `json:"chips"`
	EconomyVersion    string                `json:"economy_version"`
	ExchangeValues    [6]int64              `json:"exchange_values"`
	Revision          int64                 `json:"revision"`
	MaterialRemainder map[string]int64      `json:"material_remainder,omitempty"`
	MonsterRemainder  map[string]int64      `json:"monster_remainder,omitempty"`
	ProtectionUntil   int64                 `json:"protection_until,omitempty"`
	PositionRevision  int64                 `json:"position_revision,omitempty"`
	PositionConfirmed int64                 `json:"position_confirmed,omitempty"`
	PositionSession   string                `json:"position_session,omitempty"`
	LeaveIntent       *GGOFieldLeaveIntent  `json:"leave_intent,omitempty"`
	Extraction        *GGOFieldExtraction   `json:"extraction,omitempty"`
	PVPReturn         *GGOFieldPVPReturn    `json:"pvp_return,omitempty"`
	MonsterFight      *GGOFieldMonsterFight `json:"monster_fight,omitempty"`
	Trade             *GGOFieldTradeLink    `json:"trade,omitempty"`
}

// GGOFieldLeaveIntent records a confirmed target; actual settlement and the
// transfer are committed atomically only after the native target is accepted.
type GGOFieldLeaveIntent struct {
	RequestID   string `json:"request_id"`
	Mode        string `json:"mode"`
	TargetScene string `json:"target_scene,omitempty"`
	TargetLine  int32  `json:"target_line,omitempty"`
	ExpiresAt   int64  `json:"expires_at"`
}

// GGOFieldExtraction holds one frozen countdown, not a second asset ledger.
type GGOFieldExtraction struct {
	RequestID  string                 `json:"request_id"`
	PointID    string                 `json:"point_id"`
	Line       int64                  `json:"line"`
	AcceptedAt int64                  `json:"accepted_at"`
	ReadyAt    int64                  `json:"ready_at"`
	Origin     *AdminFieldGGOPosition `json:"origin"`
	SafeReturn *AdminFieldGGOPosition `json:"safe_return"`
}

// GGOFieldPVPReturn waits for the original battle presentation lease before
// performing the already-settled loser's frozen safe-area return.
type GGOFieldPVPReturn struct {
	FightID   string                 `json:"fight_id"`
	Position  *AdminFieldGGOPosition `json:"position"`
	HealRatio float64                `json:"heal_ratio"`
}

// GGOFieldMonsterFight freezes the public reservation and economic draw while
// the original engine's compressed input is held by FieldFight.BossInputStored.
type GGOFieldMonsterFight struct {
	FightID            string                 `json:"fight_id"`
	MonsterUUID        string                 `json:"monster_uuid"`
	DeploymentRevision string                 `json:"deployment_revision,omitempty"`
	PointID            string                 `json:"point_id"`
	Generation         int64                  `json:"generation"`
	ReservedUntilMS    int64                  `json:"reserved_until_ms"`
	Line               int64                  `json:"line"`
	Reward             string                 `json:"reward"`
	RewardNumerator    int64                  `json:"reward_numerator"`
	RewardDenominator  int64                  `json:"reward_denominator"`
	CooldownMS         int64                  `json:"cooldown_ms"`
	Chips              [6]int64               `json:"chips"`
	EconomyVersion     string                 `json:"economy_version"`
	ExchangeValues     [6]int64               `json:"exchange_values"`
	ProtectedChips     int64                  `json:"protected_chips"`
	DeathLossPercent   *int64                 `json:"death_loss_percent,omitempty"`
	SafeReturn         *AdminFieldGGOPosition `json:"safe_return"`
	HealRatio          float64                `json:"heal_ratio"`
}
