package model

// AdminGuildSettingDTO 是全局公会运营页的已提交有效配置。
// 字段值来自后台 singleton 覆盖或原静态表，不写回原表。
type AdminGuildSettingDTO struct {
	Version      string                  `json:"version"`      // Version is the last-read optimistic digest.
	UpdatedAt    int64                   `json:"updatedAt"`    // UpdatedAt is the last committed unix millisecond.
	UpdatedBy    string                  `json:"updatedBy"`    // UpdatedBy is the operator username of the last commit.
	Groups       []AdminGuildConfigGroup `json:"groups"`       // Groups are the editable parameter tabs and original labels.
	UnionLevels  AdminGuildTableDTO      `json:"unionLevels"`  // UnionLevels is the submitted full UnionConfig table.
	UnionTech    AdminGuildTableDTO      `json:"unionTech"`    // UnionTech is the submitted full UnionTech table.
	Policies     AdminGuildPolicyState   `json:"policies"`     // Policies are the independent guild and union-fight gates.
	Shops        []AdminGuildShopLink    `json:"shops"`        // Shops link to existing guild shop administration.
	Capabilities []AdminGuildCapability  `json:"capabilities"` // Capabilities state implemented and explicitly missing rules.
}

// AdminGuildConfigGroup 按业务页签组织实际可编辑的公会参数。
type AdminGuildConfigGroup struct {
	Key    string                  `json:"key"`    // Key is the stable business identifier.
	Label  string                  `json:"label"`  // Label is the original table description or display title.
	Fields []AdminGuildConfigField `json:"fields"` // Fields are the editable parameters in this group.
}

// AdminGuildConfigField 表示一个实际被公会消费者读取的原 GameConfigure 参数。
type AdminGuildConfigField struct {
	Key       string `json:"key"`       // Key is the stable business identifier.
	Label     string `json:"label"`     // Label is the original table description or display title.
	ValueType string `json:"valueType"` // ValueType is int, double or string as parsed by the consumer.
	Value     string `json:"value"`     // Value is the current effective string value.
	Editable  bool   `json:"editable"`  // Editable=false marks a read-only or unsupported field.
}

// AdminGuildConfigValue 是保存时提交的单字段覆盖值。
type AdminGuildConfigValue struct {
	Key   string `json:"key"`   // Key is the stable business identifier.
	Value string `json:"value"` // Value is the current effective string value.
}

// AdminGuildTableColumn 描述原 UnionConfig 或 UnionTech 的真实列。
type AdminGuildTableColumn struct {
	Key       string `json:"key"`       // Key is the stable business identifier.
	Label     string `json:"label"`     // Label is the original table description or display title.
	ValueType string `json:"valueType"` // ValueType is int, double or string as parsed by the consumer.
	Editable  bool   `json:"editable"`  // Editable=false marks a read-only or unsupported field.
}

// AdminGuildTableDTO 返回原分级表全列和当前生效行。
type AdminGuildTableDTO struct {
	Columns []AdminGuildTableColumn `json:"columns"` // Columns are the real original table columns in display order.
	Rows    []map[string]string     `json:"rows"`    // Rows are the effective graded table rows.
}

// AdminGuildSettingSaveRequest 提交全局配置覆盖，version 防止覆盖其他管理员已提交内容。
type AdminGuildSettingSaveRequest struct {
	Version     string                  `json:"version" binding:"required"` // Version is the last-read optimistic digest.
	Reason      string                  `json:"reason" binding:"required"`  // Reason is recorded in the admin audit.
	Fields      []AdminGuildConfigValue `json:"fields"`                     // Fields are the editable parameters in this group.
	UnionLevels []map[string]string     `json:"unionLevels"`                // UnionLevels is the submitted full UnionConfig table.
	UnionTech   []map[string]string     `json:"unionTech"`                  // UnionTech is the submitted full UnionTech table.
	Policies    *AdminGuildPolicySave   `json:"policies"`                   // Policies are the independent guild and union-fight gates.
}

// AdminGuildShopLink 指向已有商店页，商品仍由原商店后台保存。
type AdminGuildShopLink struct {
	Label string `json:"label"` // Label is the original table description or display title.
	Path  string `json:"path"`  // Path is an existing admin shop route.
	Note  string `json:"note"`  // Note explains which store still owns product data.
}

// AdminGuildCapability 明确当前页面的实现状态或需要其他消费者配合的缺口。
type AdminGuildCapability struct {
	Key         string `json:"key"`         // Key is the stable business identifier.
	Label       string `json:"label"`       // Label is the original table description or display title.
	Implemented bool   `json:"implemented"` // Implemented=false marks a deliberate unimplemented capability.
	Editable    bool   `json:"editable"`    // Editable=false marks a read-only or unsupported field.
	Detail      string `json:"detail"`      // Detail explains the capability boundary or integration requirement.
}

// AdminGuildPolicyState 汇总现有独立门禁和已发布规则。
type AdminGuildPolicyState struct {
	Guild         AdminGuildGatePolicy            `json:"guild"`         // Guild is the three independent guild gates.
	Prayer        AdminGuildPrayerPolicy          `json:"prayer"`        // Prayer is the published random prayer pool.
	Tech          AdminGuildTechPolicy            `json:"tech"`          // Tech is the published building technology directory.
	DonateBox     AdminGuildDonateBoxPolicy       `json:"donateBox"`     // DonateBox is the daily donation box gate.
	DonationBadge AdminGuildDonationBadgePolicy   `json:"donationBadge"` // DonationBadge is the coin-donation badge source gate.
	UnionFight    AdminGuildUnionFightPolicyState `json:"unionFight"`    // UnionFight groups existing union-fight policies.
}

// AdminGuildPolicySave 只提交需要更新的策略分区，nil 表示保持当前值。
type AdminGuildPolicySave struct {
	Guild         *AdminGuildGatePolicy           `json:"guild"`         // Guild is the three independent guild gates.
	Prayer        *AdminGuildPrayerPolicy         `json:"prayer"`        // Prayer is the published random prayer pool.
	Tech          *AdminGuildTechPolicy           `json:"tech"`          // Tech is the published building technology directory.
	DonateBox     *AdminGuildDonateBoxPolicy      `json:"donateBox"`     // DonateBox is the daily donation box gate.
	DonationBadge *AdminGuildDonationBadgePolicy  `json:"donationBadge"` // DonationBadge is the coin-donation badge source gate.
	UnionFight    *AdminGuildUnionFightPolicySave `json:"unionFight"`    // UnionFight groups existing union-fight policies.
}

// AdminGuildGatePolicy 是公会影响所有入口的三个独立门禁。
type AdminGuildGatePolicy struct {
	ManagementEnabled bool   `json:"managementEnabled"` // ManagementEnabled=true opens guild management.
	EconomyEnabled    bool   `json:"economyEnabled"`    // EconomyEnabled=true opens guild economy operations.
	ChatEnabled       bool   `json:"chatEnabled"`       // ChatEnabled=true opens the guild channel with management.
	Source            string `json:"source"`            // Source is the immutable original or named local source text.
}

// AdminGuildPrayerPolicy 是提前发布的祈祷随机池。
type AdminGuildPrayerPolicy struct {
	Enabled     bool   `json:"enabled"`     // Enabled=true opens the independent policy.
	Version     string `json:"version"`     // Version is the last-read optimistic digest.
	Source      string `json:"source"`      // Source is the immutable original or named local source text.
	PoolJSON    string `json:"poolJson"`    // PoolJSON is the explicit weighted prayer pool JSON.
	PoolHash    string `json:"poolHash"`    // PoolHash is the SHA-256 digest of PoolJSON.
	PublishedAt int64  `json:"publishedAt"` // PublishedAt is the explicit publication instant in unix milliseconds.
}

// AdminGuildTechPolicy 是建筑科技目录发布状态。
type AdminGuildTechPolicy struct {
	Enabled       bool   `json:"enabled"`       // Enabled=true opens the independent policy.
	Version       string `json:"version"`       // Version is the last-read optimistic digest.
	Source        string `json:"source"`        // Source is the immutable original or named local source text.
	DirectoryJSON string `json:"directoryJson"` // DirectoryJSON maps the eight original technology kinds.
	DirectoryHash string `json:"directoryHash"` // DirectoryHash is the SHA-256 digest of DirectoryJSON.
	PublishedAt   int64  `json:"publishedAt"`   // PublishedAt is the explicit publication instant in unix milliseconds.
}

// AdminGuildDonateBoxTier 是原四档捐献宝箱的目标和资源清单。
type AdminGuildDonateBoxTier struct {
	Threshold int32  `json:"threshold"` // Threshold 是公会当日累计捐献积分目标。
	Reward    string `json:"reward"`    // Reward 是实际发放的资源清单。
}

// AdminGuildDonateBoxPolicy 是捐献宝箱日规则，开始累计后保留当日快照。
type AdminGuildDonateBoxPolicy struct {
	Tiers       []AdminGuildDonateBoxTier `json:"tiers"`       // Tiers 固定四档，按积分递增。
	Enabled     bool                      `json:"enabled"`     // Enabled=true opens the independent policy.
	Version     string                    `json:"version"`     // Version is the last-read optimistic digest.
	Source      string                    `json:"source"`      // Source is the immutable original or named local source text.
	PublishedAt int64                     `json:"publishedAt"` // PublishedAt is the explicit publication instant in unix milliseconds.
}

// AdminGuildDonationBadgePolicy 是金币捐献换徽章的独立来源。
type AdminGuildDonationBadgePolicy struct {
	Enabled     bool   `json:"enabled"`     // Enabled=true opens the independent policy.
	Version     string `json:"version"`     // Version is the last-read optimistic digest.
	Source      string `json:"source"`      // Source is the immutable original or named local source text.
	CoinCost    int32  `json:"coinCost"`    // CoinCost is the fixed original coin amount.
	BadgeAmount int32  `json:"badgeAmount"` // BadgeAmount is the fixed delivered badge amount.
	DailyLimit  int64  `json:"dailyLimit"`  // DailyLimit is the fixed daily source count.
	PublishedAt int64  `json:"publishedAt"` // PublishedAt is the explicit publication instant in unix milliseconds.
}

// AdminGuildUnionFightPolicyState 汇总公会战现有独立策略。
type AdminGuildUnionFightPolicyState struct {
	Enrollment   AdminGuildUnionFightEnrollmentPolicy     `json:"enrollment"`
	Chat         AdminGuildUnionFightChatPolicy           `json:"chat"`         // Chat controls in-field text and original commands.
	Item         AdminGuildUnionFightItemPolicy           `json:"item"`         // Item controls new union-fight item consumption.
	Field        AdminGuildUnionFightFieldPolicy          `json:"field"`        // Field is the named local field policy.
	PlayerBattle []AdminGuildUnionFightPlayerBattlePolicy `json:"playerBattle"` // PlayerBattle lists existing ordinary-player policies.
}

// AdminGuildUnionFightPolicySave 只提交需要更新的公会战策略。
type AdminGuildUnionFightPolicySave struct {
	Enrollment   *AdminGuildUnionFightEnrollmentPolicy    `json:"enrollment"`
	Chat         *AdminGuildUnionFightChatPolicy          `json:"chat"`         // Chat controls in-field text and original commands.
	Item         *AdminGuildUnionFightItemPolicy          `json:"item"`         // Item controls new union-fight item consumption.
	Field        *AdminGuildUnionFightFieldPolicy         `json:"field"`        // Field is the named local field policy.
	PlayerBattle []AdminGuildUnionFightPlayerBattlePolicy `json:"playerBattle"` // PlayerBattle lists existing ordinary-player policies.
}

// AdminGuildUnionFightEnrollmentPolicy 控制报名和匹配入口。
type AdminGuildUnionFightEnrollmentPolicy struct {
	EnrollmentEnabled bool   `json:"enrollmentEnabled"` // EnrollmentEnabled=true opens union-fight enrollment.
	MatchEnabled      bool   `json:"matchEnabled"`      // MatchEnabled=true opens union-fight matching.
	Source            string `json:"source"`            // Source is the immutable original or named local source text.
}

// AdminGuildUnionFightChatPolicy 控制场内文字和原十八条指挥。
type AdminGuildUnionFightChatPolicy struct {
	TextEnabled        bool   `json:"textEnabled"`        // TextEnabled=true opens original type6 text.
	TalkEnabled        bool   `json:"talkEnabled"`        // TalkEnabled=true opens the original 18 commands.
	TalkCooldownMillis int64  `json:"talkCooldownMillis"` // TalkCooldownMillis is the explicit command cooldown.
	Source             string `json:"source"`             // Source is the immutable original or named local source text.
}

// AdminGuildUnionFightItemPolicy 控制新道具消费入口。
type AdminGuildUnionFightItemPolicy struct {
	Enabled bool   `json:"enabled"` // Enabled=true opens the independent policy.
	Source  string `json:"source"`  // Source is the immutable original or named local source text.
}

// AdminGuildUnionFightFieldPolicy 是已冻结到新场次的具名场景参数。
type AdminGuildUnionFightFieldPolicy struct {
	Enabled            bool    `json:"enabled"`            // Enabled=true opens the independent policy.
	PreparationMillis  int64   `json:"preparationMillis"`  // PreparationMillis is the field preparation window.
	LeftRevivePointID  int64   `json:"leftRevivePointId"`  // LeftRevivePointID is the original left fixed spawn.
	RightRevivePointID int64   `json:"rightRevivePointId"` // RightRevivePointID is the original right fixed spawn.
	InitialGuardHP     float64 `json:"initialGuardHp"`     // InitialGuardHP is the non-authoritative initial guard HP.
	InitialGateHP      float64 `json:"initialGateHp"`      // InitialGateHP is the non-authoritative initial gate HP.
	MoveTolerance      float64 `json:"moveTolerance"`      // MoveTolerance is the accepted position tolerance.
	Source             string  `json:"source"`             // Source is the immutable original or named local source text.
}

// AdminGuildUnionFightPlayerBattlePolicy 是普通玩家参战的前置发布策略。
type AdminGuildUnionFightPlayerBattlePolicy struct {
	ID          int64  `json:"id"`          // ID is the stable guild or policy identifier.
	Enabled     bool   `json:"enabled"`     // Enabled=true opens the independent policy.
	Version     string `json:"version"`     // Version is the last-read optimistic digest.
	Source      string `json:"source"`      // Source is the immutable original or named local source text.
	PublishedAt int64  `json:"publishedAt"` // PublishedAt is the explicit publication instant in unix milliseconds.
	StartsAt    int64  `json:"startsAt"`    // StartsAt is the first allowed match start instant.
	EndsAt      int64  `json:"endsAt"`      // EndsAt is the exclusive policy end instant.
	ReviveRule  string `json:"reviveRule"`  // ReviveRule is the frozen original revive rule id.
}

// AdminGuildSummary 是区服公会列表行。
type AdminGuildSummary struct {
	ID           string `json:"id"`           // ID is the stable guild or policy identifier.
	Name         string `json:"name"`         // Name is the guild display name.
	Level        int32  `json:"level"`        // Level is the guild level.
	MemberCount  int64  `json:"memberCount"`  // MemberCount is the current real member count.
	ChairmanName string `json:"chairmanName"` // ChairmanName is the unique current chairman display name.
	Coin         int64  `json:"coin"`         // Coin is the guild shared coin balance.
	Badge        int32  `json:"badge"`        // Badge is the guild shared badge balance.
	Exp          int32  `json:"exp"`          // Exp is the guild shared experience.
	DonateScore  int32  `json:"donateScore"`  // DonateScore is the guild donation score.
	ActiveScore  int32  `json:"activeScore"`  // ActiveScore is the guild active score.
	CreatedAt    int64  `json:"createdAt"`    // CreatedAt is the original guild creation instant.
	Version      string `json:"version"`      // Version is the last-read optimistic digest.
}

// AdminGuildListResponse 返回已分页的当前区服公会。
type AdminGuildListResponse struct {
	Items    []AdminGuildSummary `json:"items"`    // Items are the paged guild rows.
	Total    int64               `json:"total"`    // Total is the matching guild count for the selected realm.
	Page     int64               `json:"page"`     // Page is the requested one-based page.
	PageSize int64               `json:"pageSize"` // PageSize is the applied page size.
}

// AdminGuildMemberDTO 是公会成员的管理投影。
type AdminGuildMemberDTO struct {
	PlayerID              int64  `json:"playerId"`              // PlayerID is the immutable character id.
	PlayerName            string `json:"playerName"`            // PlayerName is the current character name.
	Level                 int32  `json:"level"`                 // Level is the guild level.
	FightPower            int32  `json:"fightPower"`            // FightPower is the current formation fight power.
	Role                  int32  `json:"role"`                  // Role is 0=member, 1=vice chairman, 2=chairman.
	Contribution          int32  `json:"contribution"`          // Contribution is the lifetime guild contribution.
	SpendableContribution int32  `json:"spendableContribution"` // SpendableContribution is the player balance available to spend.
	TodayCoin             int64  `json:"todayCoin"`             // TodayCoin is the current day coin donation total.
	TodayBadge            int32  `json:"todayBadge"`            // TodayBadge is the current day badge donation total.
	JoinedAt              int64  `json:"joinedAt"`              // JoinedAt is the current membership join instant.
	QuitUntil             int64  `json:"quitUntil"`             // QuitUntil is the original rejoin cooldown instant.
	LastLoginAt           int64  `json:"lastLoginAt"`           // LastLoginAt is the last authenticated guild login instant.
	DonateDay             string `json:"donateDay"`             // DonateDay is the day of the stored daily donation totals.
	Version               string `json:"version"`               // Version is the last-read optimistic digest.
}

// AdminGuildDetail 是当前区服单个公会的完整管理信息。
type AdminGuildDetail struct {
	ID              string                `json:"id"`              // ID is the stable guild or policy identifier.
	Name            string                `json:"name"`            // Name is the guild display name.
	Level           int32                 `json:"level"`           // Level is the guild level.
	Notice          string                `json:"notice"`          // Notice is the guild notice text.
	Declaration     string                `json:"declaration"`     // Declaration is the guild declaration text.
	Flag            int32                 `json:"flag"`            // Flag is the original UnionLogo identifier.
	Coin            int64                 `json:"coin"`            // Coin is the guild shared coin balance.
	Badge           int32                 `json:"badge"`           // Badge is the guild shared badge balance.
	Exp             int32                 `json:"exp"`             // Exp is the guild shared experience.
	DonateScore     int32                 `json:"donateScore"`     // DonateScore is the guild donation score.
	ActiveScore     int32                 `json:"activeScore"`     // ActiveScore is the guild active score.
	HeroStatueOpen  int32                 `json:"heroStatueOpen"`  // HeroStatueOpen is 0=not built, 1=built.
	GuardTreeOpen   int32                 `json:"guardTreeOpen"`   // GuardTreeOpen is 0=not built, 1=built.
	UnionFightApply bool                  `json:"unionFightApply"` // UnionFightApply is the persisted auto-enroll choice.
	CreatedAt       int64                 `json:"createdAt"`       // CreatedAt is the original guild creation instant.
	Version         string                `json:"version"`         // Version is the last-read optimistic digest.
	Members         []AdminGuildMemberDTO `json:"members"`         // Members are the current real members.
}

// AdminGuildUpdateRequest 编辑公会现有共享数值、文本和旗帜。
type AdminGuildUpdateRequest struct {
	Version        string `json:"version" binding:"required"` // Version is the last-read optimistic digest.
	Reason         string `json:"reason" binding:"required"`  // Reason is recorded in the admin audit.
	Name           string `json:"name" binding:"required"`    // Name is the guild display name.
	Level          int32  `json:"level"`                      // Level is the guild level.
	Notice         string `json:"notice"`                     // Notice is the guild notice text.
	Declaration    string `json:"declaration"`                // Declaration is the guild declaration text.
	Flag           int32  `json:"flag"`                       // Flag is the original UnionLogo identifier.
	Coin           int64  `json:"coin"`                       // Coin is the guild shared coin balance.
	Badge          int32  `json:"badge"`                      // Badge is the guild shared badge balance.
	Exp            int32  `json:"exp"`                        // Exp is the guild shared experience.
	DonateScore    int32  `json:"donateScore"`                // DonateScore is the guild donation score.
	ActiveScore    int32  `json:"activeScore"`                // ActiveScore is the guild active score.
	HeroStatueOpen int32  `json:"heroStatueOpen"`             // HeroStatueOpen is 0=not built, 1=built.
	GuardTreeOpen  int32  `json:"guardTreeOpen"`              // GuardTreeOpen is 0=not built, 1=built.
}

// AdminGuildMemberUpdateRequest 编辑成员实际贡献、当日捐献和职位。
type AdminGuildMemberUpdateRequest struct {
	Version                       string `json:"version" binding:"required"`              // Version is the last-read optimistic digest.
	Reason                        string `json:"reason" binding:"required"`               // Reason is recorded in the admin audit.
	Contribution                  int32  `json:"contribution"`                            // Contribution is the lifetime guild contribution.
	SpendableContribution         *int32 `json:"spendableContribution,omitempty"`         // SpendableContribution optionally replaces the available player balance.
	ExpectedSpendableContribution *int32 `json:"expectedSpendableContribution,omitempty"` // ExpectedSpendableContribution is the observed available balance for compare and swap.
	TodayCoin                     int64  `json:"todayCoin"`                               // TodayCoin is the current day coin donation total.
	TodayBadge                    int32  `json:"todayBadge"`                              // TodayBadge is the current day badge donation total.
	Role                          int32  `json:"role"`                                    // Role is 0=member, 1=vice chairman, 2=chairman.
}

// AdminGuildMemberAddRequest 将同区已有角色加入指定公会。
type AdminGuildMemberAddRequest struct {
	Version  string `json:"version" binding:"required"` // Version is the last-read optimistic digest.
	Reason   string `json:"reason" binding:"required"`  // Reason is recorded in the admin audit.
	PlayerID int64  `json:"playerId"`                   // PlayerID is the immutable character id.
}

// AdminGuildMemberRemoveRequest 将成员按原离会流程移出公会。
type AdminGuildMemberRemoveRequest struct {
	Version string `json:"version" binding:"required"` // Version is the last-read optimistic digest.
	Reason  string `json:"reason" binding:"required"`  // Reason is recorded in the admin audit.
}

// AdminGuildDissolveRequest 要求版本、精确公会名和原因后解散公会。
type AdminGuildDissolveRequest struct {
	Version     string `json:"version" binding:"required"`     // Version is the last-read optimistic digest.
	ConfirmName string `json:"confirmName" binding:"required"` // ConfirmName must equal the current exact guild name.
	Reason      string `json:"reason" binding:"required"`      // Reason is recorded in the admin audit.
}

// AdminGuildDissolveResponse 标记公会已按原解散流程移除。
type AdminGuildDissolveResponse struct {
	ID        string `json:"id"`        // ID is the stable guild or policy identifier.
	Dissolved bool   `json:"dissolved"` // Dissolved=true confirms the original dissolution completed.
}
