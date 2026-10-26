package model

import (
	"bytes"
	"encoding/json"
	"fmt"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	pb "hyjs-server/model/protocol"
)

// GamePacket 保存游戏原始包头以及按 opcode 解码的消息。
type GamePacket struct {
	Opcode    int32
	PlayerId  int64
	ZoneId    int32
	Session   []byte
	Timestamp int64
	Body      proto.Message
}

// GameState 与已经产生的 SQLite 存档兼容，协议对象使用原始字段名。
type GameState struct {
	// BagCapacityScale pairs the one-time capacity expansion with all three saved bag limits.
	BagCapacityScale int32 `json:"bag_capacity_scale,omitempty"`
	// WarriorBagCapacityScale reads the unreleased hero-only fivefold candidate without multiplying it again.
	WarriorBagCapacityScale int32 `json:"warrior_bag_capacity_scale,omitempty"`
	// Guild prayer attributes are recaptured from the current transaction and never persisted.
	GuildPrayerProjection *P5GuildPrayerProjection `json:"-"`
	// Authenticated request projection; absolute field cooldowns keep wall time.
	FieldCalendarDay       string                            `json:"-"`
	CopySweepReceipt       *CopySweepReceipt                 `json:"copy_sweep_receipt,omitempty"`
	SkillSpecialtyReceipts map[string]*SkillSpecialtyReceipt `json:"skill_specialty_receipts,omitempty"`
	TreasureBoxReceipt     *TreasureBoxReceipt               `json:"treasure_box_receipt,omitempty"`
	TreasureCombineReceipt *TreasureCombineReceipt           `json:"treasure_combine_receipt,omitempty"`
	TreasureRefineReceipts map[string]*TreasureRefineReceipt `json:"treasure_refine_receipts,omitempty"`
	Treasures              []*pb.Treasure                    `json:"treasures,omitempty"`
	Gems                   []*pb.SimpleItem                  `json:"gems,omitempty"`
	Cooking                []*pb.SimpleItem                  `json:"cooking,omitempty"`
	Recruit                map[int32]*RecruitState           `json:"recruit,omitempty"`
	// KingRecruitSharedLuck 保存永久共享余额；非nil同时标记遗留迁移完成。
	KingRecruitSharedLuck *int32 `json:"king_recruit_shared_luck,omitempty"`
	// KingRecruitLuck 仅保留旧分世界余额的迁移来源。
	KingRecruitLuck         map[int32]int32                    `json:"king_recruit_luck,omitempty"`
	EquipmentRefineReceipts map[string]*EquipmentRefineReceipt `json:"equipment_refine_receipts,omitempty"`
	EquipmentStarReceipts   map[string]*EquipmentStarReceipt   `json:"equipment_star_receipts,omitempty"`
	Field                   *FieldState                        `json:"field,omitempty"`

	TacticalUnlockReceipts   map[string]*TacticalUnlockReceipt  `json:"tactical_unlock_receipts,omitempty"`
	Equipment                []*pb.Equipment                    `json:"equipment,omitempty"`
	TacticalFormations       []*pb.TaticalFormation             `json:"tactical_formations,omitempty"`
	TacticalSkillId          string                             `json:"tactical_skill_id,omitempty"`
	TacticalUpgradeReceipts  map[string]*TacticalUpgradeReceipt `json:"tactical_upgrade_receipts,omitempty"`
	FormationExchangeReceipt *FormationExchangeReceipt          `json:"formation_exchange_receipt,omitempty"`
	CopyFirstPassedTasks     []string                           `json:"copy_first_passed_tasks,omitempty"`
	CopyChallengeDay         string                             `json:"copy_challenge_day,omitempty"`
	GuideRecruit             *pb.SCWarriorRecruitGuide          `json:"guide_recruit,omitempty"`
	Warriors                 []*pb.Warrior                      `json:"warriors"`
	Formation                []*pb.FormationItem                `json:"formation"`
	Items                    []*pb.SimpleItem                   `json:"items"`
	Quests                   []string                           `json:"quests"`
	KV                       []*pb.KeyValueEntry                `json:"kv"`
	Copy                     *pb.SCCopyPassedInfo               `json:"copy"`
	Position                 *pb.Position                       `json:"position"`
	// 转场到原CurrentPosition握手完成前，旧房间排队移动不能覆盖新入口。
	PositionLoading    bool     `json:"position_loading,omitempty"`
	Run                *CopyRun `json:"run,omitempty"`
	FullClientVerified bool     `json:"full_client_verified,omitempty"`
}

// TacticalUnlockReceipt 保存一次阵型解锁响应，网络重试仍收到原始扣费结果。
type TacticalUnlockReceipt struct {
	Timestamp int64                        `json:"timestamp"`
	Response  *pb.SCTaticalFormationUnLock `json:"response"`
}

// TacticalUpgradeReceipt 保存一次阵型升级响应，网络重试不重复扣除材料。
type TacticalUpgradeReceipt struct {
	Timestamp int64                         `json:"timestamp"`
	Response  *pb.SCTaticalFormationLevelUp `json:"response"`
}

// FormationExchangeReceipt 绑定原始交换请求和响应，避免重试把位置换回。
type FormationExchangeReceipt struct {
	Timestamp int64                   `json:"timestamp"`
	FromPos   int32                   `json:"from_pos"`
	ToPos     int32                   `json:"to_pos"`
	Response  *pb.SCFormationExChange `json:"response"`
}

// CopyRun 保存一次副本的完成节点和当前战斗，保证重试不会重复结算。
type CopyRun struct {
	// Party retains accepted HP and anger for this exploration only.
	Party    *pb.BattlePlayer `json:"party,omitempty"`
	Gain     *pb.ResultObject `json:"gain,omitempty"`
	Chapter  string           `json:"chapter"`
	Mission  string           `json:"mission"`
	Passed   []string         `json:"passed"`
	Complete bool             `json:"complete"`
	Fight    *CopyFight       `json:"fight,omitempty"`
}

// CopyFight 保存原始战斗配置和结算响应。
type CopyFight struct {
	Task    string               `json:"task"`
	Start   *pb.SCCopyFightStart `json:"start"`
	Settled bool                 `json:"settled"`
	Result  *pb.SCCopyFightEnd   `json:"result,omitempty"`
}

// GameMessage 根据生成的协议类型创建消息，拒绝不存在的消息名。
func GameMessage(name string, fields any) (proto.Message, error) {
	t, err := protoregistry.GlobalTypes.FindMessageByName(protoreflect.FullName("hyjs." + name))
	if err != nil {
		return nil, err
	}
	msg := t.New().Interface()
	if fields != nil {
		raw, err := json.Marshal(fields)
		if err != nil {
			return nil, err
		}
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if err = decoder.Decode(msg); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
	}
	return msg, nil
}

// RecruitState 持久化免费次数、保底及最后一次原始请求回执。
type RecruitState struct {
	Day      string `json:"day"`
	FreeUsed int32  `json:"free_used"`
	NextFree int64  `json:"next_free"`
	// DrawDay/Draws 独立记录强者招募按本地日累计的抽数，不影响免费冷却和保底进度。
	DrawDay        string               `json:"draw_day,omitempty"`
	Draws          int32                `json:"draws,omitempty"`
	SinceGuarantee int32                `json:"since_guarantee"`
	Timestamp      int64                `json:"timestamp"`
	Request        *pb.CSWarriorRecruit `json:"request,omitempty"`
	Response       *pb.SCWarriorRecruit `json:"response,omitempty"`
}
type EquipmentRefineReceipt struct {
	Timestamp int64             `json:"timestamp"`
	Request   *pb.CSEquipRefine `json:"request"`
	Response  *pb.SCEquipRefine `json:"response"`
}
type EquipmentStarReceipt struct {
	Timestamp int64           `json:"timestamp"`
	Response  *pb.SCEquipStar `json:"response"`
}
type FieldState struct {
	// Marks the day namespace after one conservative legacy projection.
	LifeCalendar75 bool                          `json:"life_calendar75,omitempty"`
	Scene          string                        `json:"scene"`
	Monsters       []*pb.SideMonster             `json:"monsters"`
	Bosses         []*pb.SideBoss                `json:"bosses,omitempty"`
	BossHP         map[string]map[string]float64 `json:"boss_hp,omitempty"`
	RespawnAt      map[string]int64              `json:"respawn_at"`
	RewardDay      string                        `json:"reward_day"`
	RewardKills    int32                         `json:"reward_kills"`
	// 普通野外首领奖励跨地图共用，世界首领计数由独立域保存。
	BossRewardDay    string                     `json:"boss_reward_day,omitempty"`
	BossRewardClaims int64                      `json:"boss_reward_claims,omitempty"`
	Side             *pb.PlayerSide             `json:"side"`
	Fight            *FieldFight                `json:"fight,omitempty"`
	Areas            map[string]*FieldAreaState `json:"areas,omitempty"`
}

// FieldAreaState preserves local monsters and their respawn deadlines when
// changing maps. HP, fatigue and daily rewards remain on the shared FieldState.
type FieldAreaState struct {
	Monsters  []*pb.SideMonster             `json:"monsters"`
	Bosses    []*pb.SideBoss                `json:"bosses,omitempty"`
	BossHP    map[string]map[string]float64 `json:"boss_hp,omitempty"`
	RespawnAt map[string]int64              `json:"respawn_at"`
	Fight     *FieldFight                   `json:"fight,omitempty"`
}
type FieldFight struct {
	BossDeathRules73 *AdminFieldBossGroup `json:"boss_death_rules73,omitempty"`
	BossSprouts73    int64                `json:"boss_sprouts73,omitempty"`

	StartedAt   int64                   `json:"started_at"`
	MonsterUUID string                  `json:"monster_uuid"`
	Start       *pb.SCSidePveFightStart `json:"start"`
	Result      *pb.SCSidePveFightEnd   `json:"result,omitempty"`
	Settled     bool                    `json:"settled"`
	// 普通野外首领复用同一 FieldFight 占用；小怪与首领互斥且共享断线恢复门卫。
	BossUUID       string                      `json:"boss_uuid,omitempty"`
	BossLeaderGUID string                      `json:"boss_leader_guid,omitempty"`
	BossStart      *pb.SCSidePveBossFightStart `json:"boss_start,omitempty"`
	BossResult     *pb.SCSidePveBossFightEnd   `json:"boss_result,omitempty"`
	BossInput      *FrozenBattleInput          `json:"boss_input,omitempty"`
	// Keep the pending canonical input compressed; movement never expands it.
	BossInputStored []byte `json:"boss_input_stored,omitempty"`
	BossInputHash   string `json:"boss_input_hash,omitempty"`
	BossReward      string `json:"boss_reward,omitempty"`
	BossKill        string `json:"boss_kill,omitempty"`
	BossCooldownMS  int64  `json:"boss_cooldown_ms,omitempty"`
	// BossRewardDailyLimit 按开战冻结，nil兼容既有战斗的不限次数行为。
	BossRewardDailyLimit *int64 `json:"boss_reward_daily_limit,omitempty"`
}
