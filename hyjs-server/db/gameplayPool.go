package db

import (
	"context"
	"gorm.io/gorm"
)

// GameplayPool 保存运营可编辑的本地服抽取规则；权重与奖励保存在子表。
type GameplayPool struct {
	ID                  string `gorm:"column:id;primaryKey;size:64" json:"id"`
	Name                string `gorm:"size:128;not null" json:"name"`
	Source              string `gorm:"size:255;not null" json:"source"`
	Enabled             bool   `gorm:"not null" json:"enabled"`
	MinLevel            int32  `gorm:"not null" json:"minLevel"`
	SingleCost          string `gorm:"size:128;not null" json:"singleCost"`
	TenCost             string `gorm:"size:128;not null" json:"tenCost"`
	SingleTicket        string `gorm:"size:128;not null" json:"singleTicket"`
	TenTicket           string `gorm:"size:128;not null" json:"tenTicket"`
	FreeSeconds         int64  `gorm:"not null" json:"freeSeconds"`
	FreePerDay          int32  `gorm:"not null" json:"freePerDay"`
	GuaranteeEvery      int32  `gorm:"not null" json:"guaranteeEvery"`
	CoinPerDraw         int32  `gorm:"not null" json:"coinPerDraw"`
	StartAt             int64  `gorm:"not null" json:"startAt"`
	EndAt               int64  `gorm:"not null" json:"endAt"`
	RewardEndAt         int64  `gorm:"not null" json:"rewardEndAt"`
	InitialDiamondPool  int64  `gorm:"not null" json:"initialDiamondPool"`
	JackpotPerMillion   int32  `gorm:"not null" json:"jackpotPerMillion"`
	PoolContributionBps int32  `gorm:"not null" json:"poolContributionBps"`
}

func (GameplayPool) TableName() string { return "gameplay_pools" }

// GameplayPoolReward 的 weight 为相对权重，guaranteed 决定保底时可选范围。
// stock=0 表示不限量；大转盘的共享已用库存另行持久化。
type GameplayPoolReward struct {
	PoolID     string `gorm:"column:pool_id;primaryKey;size:64" json:"poolId"`
	ID         int32  `gorm:"column:id;primaryKey;autoIncrement:false" json:"id"`
	ResourceID string `gorm:"column:resource_id;size:64;not null" json:"resourceId"`
	Amount     int32  `gorm:"not null" json:"amount"`
	Weight     int64  `gorm:"not null" json:"weight"`
	Guaranteed bool   `gorm:"not null" json:"guaranteed"`
	Stock      int32  `gorm:"not null" json:"stock"`
}

func (GameplayPoolReward) TableName() string { return "gameplay_pool_rewards" }

// InitGameplayPoolSchema 同时支持正式 MySQL 和隔离协议测试 SQLite。
func InitGameplayPoolSchema(ctx context.Context, tx *gorm.DB) error {
	return tx.WithContext(ctx).AutoMigrate(&GameplayPool{}, &GameplayPoolReward{})
}
