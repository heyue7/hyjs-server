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

// AdminFieldCombatMember 是一款原战斗成员的受限数值配置。
// ID、HeroID、Location 和 GroupIndex 绑定原表行，只读；其余字段是该行实际参与战斗的数值。
type AdminFieldCombatMember struct {
	ID           string  `json:"id"`           // ID 为原行主键：FieldMonster 使用 MonsterId，FieldBoss 使用 Id，WorldBoss 使用 Id。
	HeroID       string  `json:"heroId"`       // HeroID 为只读原英雄/怪物模型。
	Location     int64   `json:"location"`     // Location 为只读原阵位。
	GroupIndex   int64   `json:"groupIndex"`   // GroupIndex 为只读原波次/组号；WorldBoss 固定为1。
	Level        int64   `json:"level"`        // Level 为成员等级1至1000；世界首领成员等级由 WorldBossLevel 与等级系数决定，此值仅回显。
	Hp           float64 `json:"hp"`           // Hp 为生命，须大于0。
	Attack       float64 `json:"attack"`       // Attack 为攻击，须大于0。
	PhyDefend    float64 `json:"phyDefend"`    // PhyDefend 为物理防御，须大于等于0。
	MagicDefend  float64 `json:"magicDefend"`  // MagicDefend 为魔法防御，须大于等于0。
	Hit          float64 `json:"hit"`          // Hit 为命中，须大于等于0。
	Dodge        float64 `json:"dodge"`        // Dodge 为闪避，须大于等于0。
	Cri          float64 `json:"cri"`          // Cri 为暴击，须大于等于0。
	AntiCri      float64 `json:"antiCri"`      // AntiCri 为抗暴，须大于等于0。
	CriFactor    float64 `json:"criFactor"`    // CriFactor 为暴击系数，须大于等于0。
	DamageAdd    float64 `json:"damageAdd"`    // DamageAdd 为伤害加成，须大于等于0。
	DamageReduce float64 `json:"damageReduce"` // DamageReduce 为伤害减免，须大于等于0。
}

// AdminFieldMonsterGroup 配置原怪物编队的刷新、实际成员数值和队长奖励。
type AdminFieldMonsterGroup struct {
	ID         string                   `json:"id"`                // ID 为原怪物编队编号。
	HeroID     string                   `json:"heroId"`            // HeroID 为只读队长模型。
	Level      int64                    `json:"level"`             // Level 为只读原队长等级快照，战斗等级以 Members 为准。
	CooldownMS int64                    `json:"cooldownMs"`        // CooldownMS 为重生毫秒。
	Members    []AdminFieldCombatMember `json:"members,omitempty"` // Members 为原编队完整成员；旧方案缺省时继承原表。
	Drops      []AdminFieldDrop         `json:"drops"`             // Drops 为击杀掉落。
}

// AdminFieldBossGroup 配置本地图普通首领一组的刷新、成员数值和原奖励。
type AdminFieldBossGroup struct {
	ID          string                   `json:"id"`                // ID 为原 FieldBoss.BossId。
	GroupIndex  int64                    `json:"groupIndex"`        // GroupIndex 为只读原组号。
	HeroID      string                   `json:"heroId"`            // HeroID 为只读原队长模型。
	RebornHours int64                    `json:"rebornHours"`       // RebornHours 为原 RebornTime 小时，取值1至168。
	Members     []AdminFieldCombatMember `json:"members,omitempty"` // Members 为原组完整成员；旧方案缺省时继承原表。
	Drops       []AdminFieldDrop         `json:"drops"`             // Drops 为每次胜利的原 rewardGroupId 奖励。
	KillDrops   []AdminFieldDrop         `json:"killDrops"`         // KillDrops 为最后一击击杀的原 KillReward 奖励。
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
	GGO            *AdminFieldGGOConfiguration `json:"ggo,omitempty"`  // GGO 为沙漠搜打撤配置，尚未放置场景。
	Scene          string                      `json:"scene"`          // Scene 为原地图身份。
	SceneResource  string                      `json:"sceneResource"`  // SceneResource 为原场景资源，只读。
	BattleResource string                      `json:"battleResource"` // BattleResource 为原战斗地图资源，只读。
	MinLevel       int64                       `json:"minLevel"`       // MinLevel 为进入等级。
	AllowPK        bool                        `json:"allowPk"`        // AllowPK 为false禁止PK、true允许原PK政策。
	MineCount      int64                       `json:"mineCount"`      // MineCount 为同时刷新的矿点数量。
	FishCount      int64                       `json:"fishCount"`      // FishCount 为同时刷新的钓鱼点数量。
	MinePoints     []AdminFieldPoint           `json:"minePoints"`     // MinePoints 为矿物候选点。
	FishPoints     []AdminFieldPoint           `json:"fishPoints"`     // FishPoints 为钓鱼候选点。
	MonsterPoints  []AdminFieldPoint           `json:"monsterPoints"`  // MonsterPoints 为怪物候选点。
	GatherGroups   []AdminFieldGatherGroup     `json:"gatherGroups"`   // GatherGroups 为采集组。
	MonsterGroups  []AdminFieldMonsterGroup    `json:"monsterGroups"`  // MonsterGroups 为怪物组。
	// OrdinaryBossEnabled 控制本地图普通首领；nil表示旧方案未配置，运行时继承原行为。
	OrdinaryBossEnabled *bool `json:"ordinaryBossEnabled,omitempty"`
	// OrdinaryBossGroups 为本地图 FieldBossPos 候选对应的全部普通首领组；缺省时继承原表。
	OrdinaryBossGroups []AdminFieldBossGroup `json:"ordinaryBossGroups,omitempty"`
	// BossEnabled 为世界首领开关，保留旧字段名以兼容既有方案。
	BossEnabled bool `json:"bossEnabled"`
	// BossRewards 为世界首领原奖励档，保留旧字段名以兼容既有方案。
	BossRewards []AdminFieldBossReward `json:"bossRewards"`
	// WorldBossMembers 覆盖世界首领四成员基础数值；缺省时继承原表。
	WorldBossMembers []AdminFieldCombatMember `json:"worldBossMembers,omitempty"`
	// WorldBossLevel 为世界首领新建周期等级1至50；nil表示不修改当前配置。
	WorldBossLevel *int64 `json:"worldBossLevel,omitempty"`
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
	Resources    []AdminFieldResource              `json:"resources"`           // Resources 为实际消费者支持的原道具目录。
	Presentation map[string]AdminFieldPresentation `json:"presentation"`        // Presentation 为地图、资源组及怪物的名称和原图，不参与配置保存。
	WorldBoss    *AdminFieldWorldBossStatus        `json:"worldBoss,omitempty"` // WorldBoss 为当前全服世界首领只读配置，不创建周期。
}

// AdminFieldWorldBossStatus 为现有全服策略中世界首领的只读当前事实。
type AdminFieldWorldBossStatus struct {
	Enabled    bool   `json:"enabled"`    // Enabled 为现有世界首领策略开关。
	Level      int64  `json:"level"`      // Level 为策略给下一周期使用的原等级1至50。
	PositionID int64  `json:"positionId"` // PositionID 为策略当前选择的原出生编号。
	Scene      string `json:"scene"`      // Scene 为当前出生编号对应的原野外地图。
	SceneName  string `json:"sceneName"`  // SceneName 为当前出生地图的运营名称。
	BossID     string `json:"bossId"`     // BossID 为当前世界首领原BossId，用于区分普通首领同名展示。
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
