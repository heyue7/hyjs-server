package model

// AdminFieldGGORuntime 保存沙漠 V4 的版本化数值。坐标仍由已核验的场景点位提供。
type AdminFieldGGORuntime struct {
	Enabled        bool                     `json:"enabled"`
	MinClientBuild int64                    `json:"minClientBuild"`
	EconomyVersion string                   `json:"economyVersion"`
	ProtectedChips int64                    `json:"protectedChips"`
	Resource       AdminFieldGGOResource    `json:"resource"`
	MonsterTiers   []AdminFieldGGOMobTier   `json:"monsterTiers"`
	Boss           AdminFieldGGOBoss        `json:"boss"`
	Shop           AdminFieldGGOShop        `json:"shop"`
	Points         []AdminFieldGGOPoint     `json:"points"`
	SafeArea       *AdminFieldGGOSafeArea   `json:"safeArea"`
	SafeReturn     *AdminFieldGGOPosition   `json:"safeReturn"`
	EntryPoints    []*AdminFieldGGOPosition `json:"entryPoints"`
}

type AdminFieldGGOResource struct {
	MineCount           int64                       `json:"mineCount"`
	FishCount           int64                       `json:"fishCount"`
	Capacity            int64                       `json:"capacity"`
	CycleSeconds        int64                       `json:"cycleSeconds"`
	MineCooldownSeconds int64                       `json:"mineCooldownSeconds"`
	FishCooldownSeconds int64                       `json:"fishCooldownSeconds"`
	MineGatherMillis    int64                       `json:"mineGatherMillis"`
	FishGatherMillis    int64                       `json:"fishGatherMillis"`
	Fatigue             int64                       `json:"fatigue"`
	MaterialNumerator   int64                       `json:"materialNumerator"`
	MaterialDenominator int64                       `json:"materialDenominator"`
	Tiers               []AdminFieldGGOResourceTier `json:"tiers"`
}

type AdminFieldGGOResourceTier struct {
	Level          int64            `json:"level"`
	Weight         int64            `json:"weight"`
	ChipTriggerBps int64            `json:"chipTriggerBps"`
	MineDrops      []AdminFieldDrop `json:"mineDrops"`
	FishDrops      []AdminFieldDrop `json:"fishDrops"`
}

type AdminFieldGGOMobTier struct {
	HeroID            string `json:"heroId"`
	Level             int64  `json:"level"`
	Count             int64  `json:"count"`
	HP                int64  `json:"hp"`
	Attack            int64  `json:"attack"`
	PhyDefend         int64  `json:"phyDefend"`
	MagicDefend       int64  `json:"magicDefend"`
	RespawnSeconds    int64  `json:"respawnSeconds"`
	ChipTriggerBps    int64  `json:"chipTriggerBps"`
	RewardNumerator   int64  `json:"rewardNumerator"`
	RewardDenominator int64  `json:"rewardDenominator"`
}

type AdminFieldGGOBoss struct {
	HeroID                    string                         `json:"heroId"`
	Level                     int64                          `json:"level"`
	HP                        int64                          `json:"hp"`
	Attack                    int64                          `json:"attack"`
	PhyDefend                 int64                          `json:"phyDefend"`
	MagicDefend               int64                          `json:"magicDefend"`
	ParticipationThresholdBps int64                          `json:"participationThresholdBps"`
	DailyLimit                int64                          `json:"dailyLimit"`
	ActiveSeconds             int64                          `json:"activeSeconds"`
	PartyRounds               int64                          `json:"partyRounds"`
	WarningSeconds            int64                          `json:"warningSeconds"`
	EveningStartHour          int64                          `json:"eveningStartHour"`
	DayIntervalMinutes        int64                          `json:"dayIntervalMinutes"`
	EveningIntervalMinutes    int64                          `json:"eveningIntervalMinutes"`
	ParticipationItemID       string                         `json:"participationItemId"`
	ParticipationAmount       int64                          `json:"participationAmount"`
	ThresholdAmount           int64                          `json:"thresholdAmount"`
	RankRewards               []AdminFieldGGORankReward      `json:"rankRewards"`
	TailRewards               []AdminFieldGGOWeight          `json:"tailRewards"`
	ResourceDrops             []AdminFieldDrop               `json:"resourceDrops"`
	RankResourceDrops         []AdminFieldBossRank73         `json:"rankResourceDrops"`
	TailResourceDrops         []AdminFieldDrop               `json:"tailResourceDrops"`
	SproutMax                 int64                          `json:"sproutMax"`
	SproutBonusBps            int64                          `json:"sproutBonusBps"`
	OverclockRewards          []AdminFieldGGOOverclockReward `json:"overclockRewards"`
}

// AdminFieldGGOOverclockReward selects an overclock amount for one sprout count.
type AdminFieldGGOOverclockReward struct {
	Sprouts    int64                `json:"sprouts"`
	Quantities []AdminFieldQuantity `json:"quantities"`
}

type AdminFieldGGORankReward struct {
	From   int64  `json:"from"`
	To     int64  `json:"to"`
	ItemID string `json:"itemId"`
	Amount int64  `json:"amount"`
}

type AdminFieldGGOWeight struct {
	ItemID string `json:"itemId"`
	Weight int64  `json:"weight"`
}

type AdminFieldGGOShop struct {
	PriceNumerator   int64 `json:"priceNumerator"`
	PriceDenominator int64 `json:"priceDenominator"`
	RoundTo          int64 `json:"roundTo"`
	RefreshCost      int64 `json:"refreshCost"`
}

// AdminFieldGGOPoint binds one stable V4 identity to verified world positions.
type AdminFieldGGOPoint struct {
	ID            string                 `json:"id"`
	Kind          string                 `json:"kind"`
	Level         int64                  `json:"level"`
	Position      *AdminFieldGGOPosition `json:"position"`
	StandPosition *AdminFieldGGOPosition `json:"standPosition"`
	Radius        float64                `json:"radius"`
}

type AdminFieldGGOSafeArea struct {
	MinX float64 `json:"minX"`
	MaxX float64 `json:"maxX"`
	MinZ float64 `json:"minZ"`
	MaxZ float64 `json:"maxZ"`
}
