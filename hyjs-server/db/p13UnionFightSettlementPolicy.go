package db

// P13UnionFightSettlementPolicy 保存具名运营零战斗终局及其真实发奖来源。
type P13UnionFightSettlementPolicy struct {
	Group                  string `gorm:"primaryKey;type:char(1)" json:"group"`
	Enabled                bool   `gorm:"not null;default:false" json:"enabled"`
	Version                string `gorm:"type:varchar(64);not null" json:"version"`
	Source                 string `gorm:"type:text;not null" json:"source"`
	ParticipationRewards   string `gorm:"type:text;not null" json:"participation_rewards"`
	VictoryRewards         string `gorm:"type:text;not null" json:"victory_rewards"`
	TransportMVPRewards    string `gorm:"type:varchar(2048);not null;default:''" json:"transport_mvp_rewards,omitempty"`
	PersonalMissions       string `gorm:"type:varchar(4096);not null;default:''" json:"personal_missions,omitempty"`
	TaskRewards            string `gorm:"type:varchar(2048);not null;default:''" json:"task_rewards,omitempty"`
	FactoryMissionPrestige int32  `gorm:"not null;default:0" json:"factory_mission_prestige,omitempty"`
	WinPrestige            int32  `gorm:"not null" json:"win_prestige"`
	LosePrestige           int32  `gorm:"not null" json:"lose_prestige"`
	PublishedAt            int64  `gorm:"not null;autoCreateTime:false" json:"published_at"`
}

func (P13UnionFightSettlementPolicy) TableName() string { return "p13_union_fight_settlement_policies" }
