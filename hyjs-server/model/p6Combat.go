package model

import pb "hyjs-server/model/protocol"

// FrozenBattleInput 保存原协议双方实例和整场运行参数；不引用可变角色存档。
// 属性输入为客户端同版本的战斗快照，面板已展开项由AppliedProperties记录。
type FrozenBattleInput struct {
	Attacker      *pb.BattlePlayer         `json:"attacker"`
	Defender      *pb.BattlePlayer         `json:"defender,omitempty"`
	Monster       *pb.BattleMonster        `json:"monster,omitempty"`
	Seed          int64                    `json:"seed"`
	MaxRound      int32                    `json:"maxRound"`
	BattleType    int32                    `json:"battleType"`
	WinConditions []*pb.BattleWinCondition `json:"winConditions,omitempty"`
	// 原KF全局双方/所有组与元数据保持原协议；首组投影按原当地观察者映射。
	KFReplay *pb.KFReplayData `json:"kfReplay,omitempty"`
	// KFReplayViewer records the original numeric viewer or explicit local joint-identity compatibility.
	KFReplayViewer *FrozenP6KFViewer `json:"kfReplayViewer,omitempty"`
	// 表行按原TSV顺序冻结，保留相同时间点出手的稳定次序。
	DefenderFirst bool `json:"defenderFirst"`
	// 原BattleData方向只控制同逻辑帧双方更新顺序；标准原SC回放默认false。
	IsAttToDef bool `json:"isAttToDef,omitempty"`
	// 空值保持原自动AI；具名本地规则只恢复有真实无目标证据的PVE回合。
	PVECompatibilityRule string `json:"pveCompatibilityRule,omitempty"`
	// Empty selects the original BattleManager driver by battleType. The named
	// diagnostic policy replays preserved UF14 Monster probes that explicitly
	// used CopyEnemyAI; it cannot certify the original UF14 PlayerAI wrapper.
	DiagnosticAIDriverPolicy string `json:"diagnosticAIDriverPolicy,omitempty"`
	// The explicitly named original manual-to-auto diagnostic accumulates real
	// 0.1 ticks before the original player Auto control; nil preserves old input.
	DiagnosticManualAutoStartRoundTime *float64 `json:"diagnosticManualAutoStartRoundTime,omitempty"`
	// 原伤害倍率按来源阵营作用于负 HP、先于吸盾；nil 保持旧冻结输入的隐含 1。
	// 显式 0 保留原零倍率，不能用默认值覆盖。
	AttackerDamageFactor *float64                       `json:"attackerDamageFactor,omitempty"`
	DefenderDamageFactor *float64                       `json:"defenderDamageFactor,omitempty"`
	Tables               map[string][]map[string]string `json:"tables"`
	ConfigurationHash    string                         `json:"configurationHash"`
	EngineVersion        string                         `json:"engineVersion"`
	ClientVersion        string                         `json:"clientVersion"`
	AppliedProperties    map[string][]string            `json:"appliedProperties,omitempty"`
}

// CombatEvent 记录原逻辑事件及消费随机数的位置，供伤害/目标/Buff时序逐项对照。
type CombatEvent struct {
	Sequence int64   `json:"sequence"`
	Round    int32   `json:"round"`
	Time     float64 `json:"time"`
	Camp     int32   `json:"camp"`
	Kind     string  `json:"kind"`
	Actor    string  `json:"actor,omitempty"`
	Target   string  `json:"target,omitempty"`
	Skill    string  `json:"skill,omitempty"`
	Cast     string  `json:"cast,omitempty"`
	Buff     string  `json:"buff,omitempty"`
	Value    float64 `json:"value,omitempty"`
	TargetHP float64 `json:"targetHp,omitempty"`
	Miss     bool    `json:"miss,omitempty"`
	Critical bool    `json:"critical,omitempty"`
	// OriginalValueOrder is the 1-based position inside its original cast export; zero means ungrouped.
	OriginalValueOrder int64 `json:"originalValueOrder,omitempty"`
	// OriginalBuffExport identifies a calculated Buff instance for ordered numerical lists; callback identity remains separate.
	OriginalBuffExport          int64                     `json:"originalBuffExport,omitempty"`
	OriginalStandaloneLoadGroup int64                     `json:"originalStandaloneLoadGroup,omitempty"`
	OriginalBuffParentSnapshot  *CombatBuffExportSnapshot `json:"originalBuffParentSnapshot,omitempty"`
	// OriginalBuffInstance preserves aliasing of the actual calculated Buff object.
	// It is local to this calculation, independent of the original UIDUtil number.
	OriginalBuffInstance int64 `json:"originalBuffInstance,omitempty"`
	// OriginalBuffParentInstance links a delayed export to its owner Buff.
	OriginalBuffParentInstance int64                     `json:"originalBuffParentInstance,omitempty"`
	OriginalBuffSnapshot       *CombatBuffExportSnapshot `json:"originalBuffSnapshot,omitempty"`
	OriginalExportSource       string                    `json:"originalExportSource,omitempty"`
	OriginalExportSkill        string                    `json:"originalExportSkill,omitempty"`
	OriginalExportCast         string                    `json:"originalExportCast,omitempty"`
	OriginalBuffTalent         string                    `json:"originalBuffTalent,omitempty"`
	OriginalBuffSkill          string                    `json:"originalBuffSkill,omitempty"`
	OriginalBuffCast           string                    `json:"originalBuffCast,omitempty"`
	OriginalBuffOwner          string                    `json:"originalBuffOwner,omitempty"`
	OriginalValueClass         int32                     `json:"originalValueClass,omitempty"`
	OriginalValueSource        string                    `json:"originalValueSource,omitempty"`
	OriginalValueTarget        string                    `json:"originalValueTarget,omitempty"`
	OriginalExtraValue         float64                   `json:"originalExtraValue,omitempty"`
	OriginalHPValue            *float64                  `json:"originalHpValue,omitempty"`
	OriginalHPType             *int32                    `json:"originalHpType,omitempty"`
	RandomDraws                int64                     `json:"randomDraws"`
}

// CombatOutcome 包含整场结果、原操作记录和完整审计事件；Authoritative仅允许已对照能力范围。
type CombatOutcome struct {
	Winner         int32             `json:"winner"` // 0=进攻胜，1=防守胜；2仅保留旧版本未校准终局。
	FinalRound     int32             `json:"finalRound"`
	Operates       *pb.BattleOperate `json:"operates"`
	Attacker       *pb.BattlePlayer  `json:"attacker"`
	Defender       *pb.BattlePlayer  `json:"defender,omitempty"`
	Monster        *pb.BattleMonster `json:"monster,omitempty"`
	Events         []CombatEvent     `json:"events"`
	RandomDraws    int64             `json:"randomDraws"`
	Authoritative  bool              `json:"authoritative"`
	CapabilityHash string            `json:"capabilityHash"`
	// 兼容来源与控制动作只用于审计；不改变原protobuf操作字段。
	PVECompatibilityRule string `json:"pveCompatibilityRule,omitempty"`
	// Empty selects the original BattleManager driver by battleType. The named
	// diagnostic policy replays preserved UF14 Monster probes that explicitly
	// used CopyEnemyAI; it cannot certify the original UF14 PlayerAI wrapper.
	DiagnosticAIDriverPolicy string `json:"diagnosticAIDriverPolicy,omitempty"`
}

// CombatCapability 明示可计算机制和对应原客户端对照状态，未对照范围禁止先算后播决定奖励。
type CombatCapability struct {
	Name           string `json:"name"`
	Implemented    bool   `json:"implemented"`
	NativeCompared bool   `json:"nativeCompared"`
	Evidence       string `json:"evidence"`
}

// CombatBuffExportSnapshot retains original callback Buff fields without live pointers.
type CombatBuffExportSnapshot struct {
	ID            string  `json:"id"`
	Factor        string  `json:"factor"`
	Args1         float64 `json:"args1"`
	Args2         float64 `json:"args2"`
	Args3         float64 `json:"args3"`
	BuffClass     int32   `json:"buffClass"`
	BuffFlag      int32   `json:"buffFlag"`
	Group         string  `json:"group"`
	IsDebuff      bool    `json:"isDebuff"`
	CanBeCleared  bool    `json:"canBeCleared"`
	OnceData      bool    `json:"onceData"`
	ShouldCover   bool    `json:"shouldCover"`
	Param         string  `json:"param"`
	TotalCount    float64 `json:"totalCount"`
	TotalTime     float64 `json:"totalTime"`
	TotalRound    int32   `json:"totalRound"`
	AttCount      int32   `json:"attCount"`
	DefCount      int32   `json:"defCount"`
	Val           float64 `json:"val"`
	Val2          float64 `json:"val2"`
	LoadBuffRound int32   `json:"loadBuffRound"`
	LoadBuffTime  float64 `json:"loadBuffTime"`
	Timer         int32   `json:"timer"`
	Target        string  `json:"target"`
	Source        string  `json:"source"`
	Talent        string  `json:"talent"`
	Skill         string  `json:"skill"`
	Cast          string  `json:"cast"`
}

// FrozenP6KFViewer keeps viewer context outside the unchanged original KF PB.
// Empty compatibility uses original PID-only matching. The named local rule
// selects one current realm+PID and does not certify any battle result.
type FrozenP6KFViewer struct {
	PlayerID          int64  `json:"playerId"`
	ServerID          int64  `json:"serverId,omitempty"`
	CompatibilityRule string `json:"compatibilityRule,omitempty"`
}
