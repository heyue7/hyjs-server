package db

// P13UnionFightFieldPlayer 保存每场冻结角色HP/AP、挂机状态和位置；道具投入与战斗结算尚未接入。
type P13UnionFightFieldPlayer struct {
	FrameState    int64   `gorm:"not null;default:0;index:p13_field_natural_revive,priority:1" json:"frameState,omitempty"`
	MapProjection string  `gorm:"type:varchar(32);not null;default:''" json:"mapProjection,omitempty"`
	MatchID       string  `gorm:"primaryKey;type:char(64);index:p13_field_natural_revive,priority:3;index:p13_field_lifecycle,priority:3" json:"matchId"`
	PlayerID      int64   `gorm:"primaryKey;autoIncrement:false;index;index:p13_field_natural_revive,priority:4;index:p13_field_lifecycle,priority:2" json:"playerId"`
	GuildID       string  `gorm:"type:varchar(64);not null" json:"guildId"`
	Camp          int64   `gorm:"not null" json:"camp"`
	Frame         []byte  `gorm:"type:longblob;not null" json:"-"`
	FrameHash     string  `gorm:"type:char(64);not null" json:"frameHash"`
	Battle        []byte  `gorm:"type:longblob;not null" json:"-"`
	BattleHash    string  `gorm:"type:char(64);not null" json:"battleHash"`
	X             float32 `gorm:"not null" json:"x"`
	Y             float32 `gorm:"not null" json:"y"`
	Z             float32 `gorm:"not null" json:"z"`
	SessionHash   string  `gorm:"type:char(64);not null;default:''" json:"-"`
	JoinedAt      int64   `gorm:"not null" json:"joinedAt"`
	UpdatedAt     int64   `gorm:"not null;autoUpdateTime:false" json:"updatedAt"`
	LeftAt        int64   `gorm:"not null;default:0;index:p13_field_natural_revive,priority:2;index:p13_field_lifecycle,priority:1" json:"leftAt"`
	Hooked        bool    `gorm:"not null;default:false" json:"hooked"`
	MovementDebt  float64 `gorm:"not null;default:0" json:"movementDebt"`
	Source        string  `gorm:"type:text;not null" json:"source"`
}

func (P13UnionFightFieldPlayer) TableName() string { return "p13_union_fight_field_players" }
