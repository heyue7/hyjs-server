package db

// P11ArenaRankAllocator owns the next rank for one arena policy.
type P11ArenaRankAllocator struct {
	PolicyID int64 `gorm:"column:policy_id;primaryKey;autoIncrement:false"`
	NextRank int64 `gorm:"column:next_rank;not null"`
}

func (P11ArenaRankAllocator) TableName() string { return "p11_arena_rank_allocators" }
