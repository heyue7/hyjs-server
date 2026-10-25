package db

// P13UnionFightPlayerBattleAttempt journals durable capture, certified calculation and timed application.
type P13UnionFightPlayerBattleAttempt struct {
	ID                  string `gorm:"primaryKey;type:char(64);index:p13_uf_player_battle_compute,priority:6" json:"id"`
	MatchID             string `gorm:"type:char(64);not null;index:p13_uf_player_battle_match" json:"match_id"`
	PlayerID            int64  `gorm:"not null;uniqueIndex:p13_uf_player_battle_stamp,priority:1;index;index:p13_uf_player_battle_client,priority:1" json:"player_id"`
	TargetID            int64  `gorm:"not null;index:p13_uf_player_battle_target" json:"target_id"`
	Stamp               int64  `gorm:"not null;uniqueIndex:p13_uf_player_battle_stamp,priority:2" json:"stamp"`
	ClientStamp         int64  `gorm:"not null;index:p13_uf_player_battle_client,priority:2" json:"client_stamp"`
	CalculationStage    string `gorm:"type:varchar(16);not null;default:'';index:p13_uf_player_battle_compute,priority:1" json:"calculation_stage,omitempty"`
	NextCalculationAt   int64  `gorm:"not null;default:0;index:p13_uf_player_battle_compute,priority:4" json:"next_calculation_at,omitempty"`
	CalculationFailures int64  `gorm:"not null;default:0" json:"calculation_failures,omitempty"`
	// Quarantine is an independent scheduler flag, excluded from authenticated
	// source bytes so damaged payloads are never re-signed to defer their work.
	CalculationQuarantined bool   `gorm:"not null;default:false;index:p13_uf_player_battle_compute,priority:3" json:"-"`
	Status                 string `gorm:"type:varchar(16);not null;index:p13_uf_player_battle_due,priority:1;index:p13_uf_player_battle_compute,priority:2" json:"status"`
	FightID                string `gorm:"type:varchar(128);not null;uniqueIndex:p13_uf_player_battle_fight_id" json:"fight_id"`
	RequestHash            string `gorm:"type:char(64);not null" json:"request_hash"`
	OfferHash              string `gorm:"type:char(64);not null" json:"offer_hash"`
	P6RecordID             string `gorm:"type:char(64);not null;uniqueIndex:p13_uf_player_battle_p6" json:"p6_record_id"`
	InputHash              string `gorm:"type:char(64);not null" json:"input_hash"`
	StartPB                []byte `gorm:"type:longblob;not null" json:"start_pb"`
	StartHash              string `gorm:"type:char(64);not null" json:"start_hash"`
	AppliedProperties      []byte `gorm:"type:longblob;not null" json:"applied_properties"`
	Before                 []byte `gorm:"type:longblob;not null" json:"before"`
	After                  []byte `gorm:"type:longblob" json:"after"`
	Terminal               []byte `gorm:"type:longblob;not null" json:"terminal"`
	TerminalHash           string `gorm:"type:char(64);not null" json:"terminal_hash"`
	ScopeProof             string `gorm:"type:char(64);not null" json:"scope_proof"`
	DeadPlayerID           int64  `gorm:"not null;default:0" json:"dead_player_id"`
	RewardPlayerID         int64  `gorm:"not null;default:0" json:"reward_player_id"`
	ReviveSeconds          int64  `gorm:"not null;default:0" json:"revive_seconds"`
	CreatedAt              int64  `gorm:"not null;autoCreateTime:false;index:p13_uf_player_battle_compute,priority:5" json:"created_at"`
	DueAt                  int64  `gorm:"not null;index:p13_uf_player_battle_due,priority:2" json:"due_at"`
	SettledAt              int64  `gorm:"not null;default:0" json:"settled_at"`
	SourceHash             string `gorm:"type:char(64);not null" json:"source_hash"`
}

func (P13UnionFightPlayerBattleAttempt) TableName() string {
	return "p13_union_fight_player_battle_attempts"
}
