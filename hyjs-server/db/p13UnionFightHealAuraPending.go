package db

// P13UnionFightHealAuraPending freezes one accepted new-rule aura tick/target.
// Pending HP never changes native P6 inputs or terminals; recovery is separate.
type P13UnionFightHealAuraPending struct {
	ID                 string  `gorm:"primaryKey;type:char(64)" json:"id"`
	MatchID            string  `gorm:"type:char(64);not null;index:p13_aura_pending_actor,priority:1;index:p13_aura_pending_effect,priority:1;index:p13_aura_pending_match,priority:1" json:"matchId"`
	EffectGUID         int64   `gorm:"not null;index:p13_aura_pending_effect,priority:2" json:"effectGuid"`
	PlayerID           int64   `gorm:"not null;index:p13_aura_pending_actor,priority:2;index:p13_aura_pending_match,priority:3" json:"playerId"`
	CasterID           int64   `gorm:"not null" json:"casterId"`
	GuildID            string  `gorm:"type:varchar(64);not null" json:"guildId"`
	Camp               int64   `gorm:"not null" json:"camp"`
	LifeOrdinal        int64   `gorm:"not null" json:"lifeOrdinal"`
	PlayerLimit        int64   `gorm:"not null" json:"playerLimit"`
	TickAt             int64   `gorm:"not null;index:p13_aura_pending_effect,priority:3" json:"tickAt"`
	ObservedAt         int64   `gorm:"not null" json:"observedAt"`
	HPPercent          int64   `gorm:"not null" json:"hpPercent"`
	Rule               string  `gorm:"type:varchar(64);not null" json:"rule"`
	OfferHash          string  `gorm:"type:char(64);not null" json:"offerHash"`
	EffectHash         string  `gorm:"type:char(64);not null" json:"effectHash"`
	BattleHash         string  `gorm:"type:char(64);not null" json:"battleHash"`
	CasterX            float32 `gorm:"not null" json:"casterX"`
	CasterY            float32 `gorm:"not null" json:"casterY"`
	CasterZ            float32 `gorm:"not null" json:"casterZ"`
	TargetX            float32 `gorm:"not null" json:"targetX"`
	TargetY            float32 `gorm:"not null" json:"targetY"`
	TargetZ            float32 `gorm:"not null" json:"targetZ"`
	OccupancyKind      string  `gorm:"type:varchar(16);not null;default:''" json:"occupancyKind"`
	OccupancyID        string  `gorm:"type:char(64);not null;default:'';index:p13_aura_pending_actor,priority:4" json:"occupancyId"`
	OccupancyInputHash string  `gorm:"type:char(64);not null;default:''" json:"occupancyInputHash"`
	OccupancyDueAt     int64   `gorm:"not null;default:0" json:"occupancyDueAt"`
	Capture            []byte  `gorm:"type:blob;not null" json:"-"`
	CaptureHash        string  `gorm:"type:char(64);not null" json:"captureHash"`
	ImmutableHash      string  `gorm:"type:char(64);not null" json:"immutableHash"`
	Status             string  `gorm:"type:varchar(16);not null;index:p13_aura_pending_actor,priority:3;index:p13_aura_pending_match,priority:2" json:"status"`
	AppliedAt          int64   `gorm:"not null;default:0" json:"appliedAt"`
	CancelReason       string  `gorm:"type:varchar(32);not null;default:''" json:"cancelReason"`
	CancelLeftAt       int64   `gorm:"not null;default:0" json:"cancelLeftAt"`
	RecoveryID         string  `gorm:"type:char(64);not null;default:''" json:"recoveryId"`
	Final              []byte  `gorm:"type:blob" json:"-"`
	FinalHash          string  `gorm:"type:char(64);not null;default:''" json:"finalHash"`
	SourceHash         string  `gorm:"type:char(64);not null" json:"sourceHash"`
}

func (P13UnionFightHealAuraPending) TableName() string { return "p13_union_fight_heal_aura_pendings" }
