package model

// AdminGuildRewardRank 定义一个名次的明确资源。
type AdminGuildRewardRank struct {
	// Rank 是获奖名次。
	Rank int32 `json:"rank"`
	// Rewards 是该名次的奖励。
	Rewards []AdminResourceAmount `json:"rewards"`
}

// AdminGuildRewardPolicy 是独立保存的公会奖励配置。
type AdminGuildRewardPolicy struct {
	// Kind 为 settlement、weekly 或 monthly。
	Kind string `json:"kind"`
	// Group 为 A、B、C、D 组。
	Group string `json:"group"`
	// WeekStart 为周一日期。
	WeekStart string `json:"weekStart"`
	// Month 为奖励月份。
	Month string `json:"month"`
	// Enabled 控制本期奖励投放。
	Enabled bool `json:"enabled"`
	// Version 是运营版本名称。
	Version string `json:"version"`
	// Source 是奖励发布依据。
	Source string `json:"source"`
	// ParticipationRewards 是参与奖励。
	ParticipationRewards []AdminResourceAmount `json:"participationRewards"`
	// VictoryRewards 是胜利奖励。
	VictoryRewards []AdminResourceAmount `json:"victoryRewards"`
	// TransportMVPRewards 是运矿优胜奖励。
	TransportMVPRewards []AdminResourceAmount `json:"transportMvpRewards"`
	// TaskRewards 是任务奖励。
	TaskRewards []AdminResourceAmount `json:"taskRewards"`
	// PersonalMissionsJSON 是个人任务目录。
	PersonalMissionsJSON string `json:"personalMissionsJson"`
	// FactoryMissionPrestige 是工厂任务声望。
	FactoryMissionPrestige int32 `json:"factoryMissionPrestige"`
	// WinPrestige 是胜方声望。
	WinPrestige int32 `json:"winPrestige"`
	// LosePrestige 是败方声望。
	LosePrestige int32 `json:"losePrestige"`
	// Ranks 是名次奖励目录。
	Ranks []AdminGuildRewardRank `json:"ranks"`
	// MemberTitle 控制成员称号。
	MemberTitle bool `json:"memberTitle"`
	// ChairmanTitle 控制会长称号。
	ChairmanTitle bool `json:"chairmanTitle"`
}

// AdminGuildRewardRow 包含配置版本与可编辑状态。
type AdminGuildRewardRow struct {
	AdminGuildRewardPolicy
	// RowVersion 是整行乐观锁版本。
	RowVersion string `json:"rowVersion"`
	// PublishedAt 是服务端发布时间。
	PublishedAt int64 `json:"publishedAt"`
	// Frozen 表示已有奖励场次采用该期配置。
	Frozen bool `json:"frozen"`
	// Editable 表示仍可编辑。
	Editable bool `json:"editable"`
}

// AdminGuildRewardsResponse 返回当前奖励政策。
type AdminGuildRewardsResponse struct {
	// Items 是三类奖励政策列表。
	Items []AdminGuildRewardRow `json:"items"`
}

// AdminGuildRewardSaveRequest 提交一项奖励配置。
type AdminGuildRewardSaveRequest struct {
	AdminGuildRewardPolicy
	// RowVersion 是读取时整行版本，新建为空。
	RowVersion string `json:"rowVersion"`
	// Reason 是本次修改原因。
	Reason string `json:"reason" binding:"required"`
}
