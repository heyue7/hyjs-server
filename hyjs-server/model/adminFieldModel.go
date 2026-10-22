package model

// AdminFieldWeight 表示一个原候选ID的正整数抽取权重。
type AdminFieldWeight struct {
	ID     string `json:"id"`     // ID 为原资源组或原怪物组。
	Weight int64  `json:"weight"` // Weight 为相对权重。
}

// AdminFieldQuantity 表示一次独立掉落的数量与权重。
type AdminFieldQuantity struct {
	Amount int64 `json:"amount"` // Amount 为数量，0允许空掉落。
	Weight int64 `json:"weight"` // Weight 为相对权重。
}

// AdminFieldDrop 表示每种道具独立抽取的数量分布。
type AdminFieldDrop struct {
	ResourceID string               `json:"resourceId"` // ResourceID 为原道具ID。
	Quantities []AdminFieldQuantity `json:"quantities"` // Quantities 为数量分布。
}

// AdminFieldPoint 使用原客户端能识别的候选点，坐标保持原资源映射。
type AdminFieldPoint struct {
	ID      string             `json:"id"`      // ID 为原候选点编号。
	Point   string             `json:"point"`   // Point 为只读坐标。
	Choices []AdminFieldWeight `json:"choices"` // Choices 为刷新组权重。
}

// AdminFieldGatherGroup 配置矿物或钓鱼组的实际采集和掉落。
type AdminFieldGatherGroup struct {
	ID         string           `json:"id"`         // ID 为原采集组编号。
	Level      int64            `json:"level"`      // Level 为生活技能门槛1至6。
	GatherMS   int64            `json:"gatherMs"`   // GatherMS 为采集毫秒。
	CooldownMS int64            `json:"cooldownMs"` // CooldownMS 为刷新毫秒。
	Times      int64            `json:"times"`      // Times 为每轮可采次数。
	Fatigue    int64            `json:"fatigue"`    // Fatigue 为疲劳消耗。
	Drops      []AdminFieldDrop `json:"drops"`      // Drops 为独立掉落。
}

// AdminFieldMonsterGroup 配置原怪物编队的刷新和队长奖励，战斗阵容继续使用原表。
type AdminFieldMonsterGroup struct {
	ID         string           `json:"id"`         // ID 为原怪物编队编号。
	HeroID     string           `json:"heroId"`     // HeroID 为只读队长模型。
	Level      int64            `json:"level"`      // Level 为只读原等级。
	CooldownMS int64            `json:"cooldownMs"` // CooldownMS 为重生毫秒。
	Drops      []AdminFieldDrop `json:"drops"`      // Drops 为击杀掉落。
}

// AdminFieldBossReward 为首领一个原奖励档的固定奖励。
type AdminFieldBossReward struct {
	Type     int64            `json:"type"`     // Type 为0参与、1伤害第11名以后、2伤害第4至10名、3伤害第2至3名、4伤害第1名、5最后一击。
	Level    int64            `json:"level"`    // Level 为原首领等级1至50。
	Weighted bool             `json:"weighted"` // Weighted 为true数量权重、false固定道具。
	Drops    []AdminFieldDrop `json:"drops"`    // Drops 为原档位奖励。
}

// AdminFieldConfiguration 定义一张野外地图的资源组合。
type AdminFieldConfiguration struct {
	Scene          string                   `json:"scene"`          // Scene 为原地图身份。
	SceneResource  string                   `json:"sceneResource"`  // SceneResource 为原场景资源，只读。
	BattleResource string                   `json:"battleResource"` // BattleResource 为原战斗地图资源，只读。
	MinLevel       int64                    `json:"minLevel"`       // MinLevel 为进入等级。
	AllowPK        bool                     `json:"allowPk"`        // AllowPK 为false禁止PK、true允许原PK政策。
	MineCount      int64                    `json:"mineCount"`      // MineCount 为同时刷新的矿点数量。
	FishCount      int64                    `json:"fishCount"`      // FishCount 为同时刷新的钓鱼点数量。
	MinePoints     []AdminFieldPoint        `json:"minePoints"`     // MinePoints 为矿物候选点。
	FishPoints     []AdminFieldPoint        `json:"fishPoints"`     // FishPoints 为钓鱼候选点。
	MonsterPoints  []AdminFieldPoint        `json:"monsterPoints"`  // MonsterPoints 为怪物候选点。
	GatherGroups   []AdminFieldGatherGroup  `json:"gatherGroups"`   // GatherGroups 为采集组。
	MonsterGroups  []AdminFieldMonsterGroup `json:"monsterGroups"`  // MonsterGroups 为怪物组。
	BossEnabled    bool                     `json:"bossEnabled"`    // BossEnabled 为false不显示首领、true沿用原首领周期。
	BossRewards    []AdminFieldBossReward   `json:"bossRewards"`    // BossRewards 为原首领奖励档。
}

// AdminFieldTemplateDTO 为一个完整全局方案。
type AdminFieldTemplateDTO struct {
	ID                string                  `json:"id"`                // ID 为方案编号。
	Name              string                  `json:"name"`              // Name 为运营名称。
	Available         bool                    `json:"available"`         // Available 为客户端资源齐备。
	UnavailableReason string                  `json:"unavailableReason"` // UnavailableReason 为暂未开放原因。
	Version           string                  `json:"version"`           // Version 为并发版本。
	Configuration     AdminFieldConfiguration `json:"configuration"`     // Configuration 为完整资源组合。
}

// AdminFieldListResponse 为全局方案目录。
type AdminFieldListResponse struct {
	Items        []AdminFieldTemplateDTO           `json:"items"`
	Resources    []AdminFieldResource              `json:"resources"`    // Resources 为实际消费者支持的原道具目录。
	Presentation map[string]AdminFieldPresentation `json:"presentation"` // Presentation 为地图、资源组及怪物的名称和原图，不参与配置保存。
}

// AdminFieldPresentation 将稳定资源值转换为运营可读的名称和真实原图。
type AdminFieldPresentation struct {
	Name    string         `json:"name"`          // Name 为原文或按用途命名的中文标签。
	IconURL string         `json:"iconUrl"`       // IconURL 为原美术缩略图，无原图时为空。
	Detail  string         `json:"detail"`        // Detail 为地图或编队的简短用途说明。
	Map     *AdminFieldMap `json:"map,omitempty"` // Map 为原小地图参数与首领出生点，只读且不参与配置保存。
}

// AdminFieldMapPoint 表示原表中的一个首领候选出生点。
type AdminFieldMapPoint struct {
	ID   string  `json:"id"`   // ID 为原点位编号。
	Name string  `json:"name"` // Name 为原表候选首领名称。
	X    float64 `json:"x"`    // X 为原场景横坐标。
	Z    float64 `json:"z"`    // Z 为原场景纵坐标。
}

// AdminFieldMap 为前端展示原大地图分布所需的只读参数。
type AdminFieldMap struct {
	Width           int64                `json:"width"`                     // Width 为原图宽度，当前为512。
	Height          int64                `json:"height"`                    // Height 为原图高度，当前为512。
	Scale           float64              `json:"scale"`                     // Scale 为原小地图缩放。
	OffsetX         float64              `json:"offsetX"`                   // OffsetX 为原小地图水平偏移。
	OffsetY         float64              `json:"offsetY"`                   // OffsetY 为原小地图纵向偏移。
	BossPoints      []AdminFieldMapPoint `json:"bossPoints,omitempty"`      // BossPoints 为原野外普通首领出生点。
	WorldBossPoints []AdminFieldMapPoint `json:"worldBossPoints,omitempty"` // WorldBossPoints 为原世界首领出生点。
}

// AdminFieldResource 限定掉落编辑器可选的真实道具和消费者。
type AdminFieldResource struct {
	AdminItemDTO
	MaxAmount    int64    `json:"maxAmount"`    // MaxAmount 为实际实例或堆叠发奖上限。
	AllowedKinds []string `json:"allowedKinds"` // AllowedKinds 为gather采集、monster小怪、boss首领。
}

// AdminFieldSaveRequest 创建或更新方案，已发布的区服快照保持不变。
type AdminFieldSaveRequest struct {
	ID            string                  `json:"id"`                      // ID 为空创建新方案。
	Name          string                  `json:"name" binding:"required"` // Name 为名称。
	Version       string                  `json:"version"`                 // Version 为更新时必需的版本。
	Configuration AdminFieldConfiguration `json:"configuration"`           // Configuration 为资源组合。
}

// AdminFieldDeleteRequest 删除未使用方案。
type AdminFieldDeleteRequest struct {
	ID      string `json:"id" binding:"required"`
	Version string `json:"version" binding:"required"`
}

// AdminFieldDeploymentDTO 为指定区服的单张地图部署。
type AdminFieldDeploymentDTO struct {
	Scene           string `json:"scene"`
	Name            string `json:"name"`
	TemplateID      string `json:"templateId"`
	Enabled         bool   `json:"enabled"`
	Available       bool   `json:"available"`
	Version         string `json:"version"`
	TemplateChanged bool   `json:"templateChanged"`
}

// AdminFieldDeploymentsResponse 为区服地图部署列表。
type AdminFieldDeploymentsResponse struct {
	ServerID int64                     `json:"serverId"`
	Items    []AdminFieldDeploymentDTO `json:"items"`
}

// AdminFieldDeployRequest 发布全局方案快照到一个区服的原地图。
type AdminFieldDeployRequest struct {
	ServerID        int64  `json:"serverId" binding:"required"`
	Scene           string `json:"scene" binding:"required"`
	TemplateID      string `json:"templateId" binding:"required"`
	TemplateVersion string `json:"templateVersion" binding:"required"`
	Version         string `json:"version"`
	Enabled         bool   `json:"enabled"`
}

// AdminFieldChannelLine reports actual live sessions on an original client line.
type AdminFieldChannelLine struct {
	Line   int64 `json:"line"`   // Line is the client line number from one.
	Online int64 `json:"online"` // Online counts authorized TCP sessions only.
}

// AdminFieldChannelDTO exposes one realm map's line settings.
type AdminFieldChannelDTO struct {
	Scene      string                  `json:"scene"`      // Scene is the stable original map ID.
	Name       string                  `json:"name"`       // Name is the localized map name.
	LineCount  int64                   `json:"lineCount"`  // LineCount is 1 to 32.
	MaxPlayers int64                   `json:"maxPlayers"` // MaxPlayers is capacity per line, 1 to 1000.
	Version    string                  `json:"version"`    // Version protects concurrent edits.
	Lines      []AdminFieldChannelLine `json:"lines"`      // Lines contains current online counts.
}

// AdminFieldChannelsResponse contains only the requested realm's line states.
type AdminFieldChannelsResponse struct {
	Enabled  bool                   `json:"enabled"`  // Enabled reports the real original presence policy; false cannot provide client lines.
	ServerID int64                  `json:"serverId"` // ServerID is the requested realm.
	Items    []AdminFieldChannelDTO `json:"items"`    // Items are the original maps.
}

// AdminFieldChannelSaveRequest updates original line capacities for one realm map.
type AdminFieldChannelSaveRequest struct {
	ServerID   int64  `json:"serverId" binding:"required"`   // ServerID is an existing realm.
	Scene      string `json:"scene" binding:"required"`      // Scene is an original map ID.
	LineCount  int64  `json:"lineCount" binding:"required"`  // LineCount is 1 to 32.
	MaxPlayers int64  `json:"maxPlayers" binding:"required"` // MaxPlayers is 1 to 1000.
	Version    string `json:"version"`                       // Version is required once settings exist.
}
