# 游戏持久化字段静态盘点

源码基线 `f906b82`；主代理确认的生产程序 `3fb495f`。本文件仅依据源码与主代理提供的只读元数据，未连接数据库、构建或运行测试。

生产元数据采样 `2026-11-02T05:42:38.730300+00:00`，采样文件 [production-storage-metadata.json](production-storage-metadata.json)；这是时点表存在性，不代表当前实时状态。CSV 的生产列类型仅在提供的 722 条 TEXT/BLOB/JSON/BINARY 类记录中存在时填写；普通 VARCHAR 未被该列集覆盖，空白不证明列不存在。

共 **537 个源码表**（537 个显式模型、535 个不同建表SQL表），共 **2241 个候选字段**，生产采样含 535 表、722 条目标类型列。

候选范围刻意包含全部持久化 Go `string`、`*string`、`[]byte`，以及 SQL 的 TEXT、JSON、BLOB、CHAR/VARCHAR、BINARY/VARBINARY；普通业务 ID、文本和摘要也保留，避免遗漏以短字符串存放结构的数据。CSV 不含数值数据列，但主键和归属列列于每行。`gorm:"-"` 非持久字段已排除；只读生成列随SQL保留。

## 分类与边界

- `已确认（调用链人工核对）`：人工核对存储字段与实际读写调用。
- `已确认（静态字段绑定）`：同函数中变量明确声明或构造为该 db 模型，并将该字段作为 JSON/Proto 解码、`p14PromotionRankDecode` 严格 JSON 解码或字符串分隔调用的首个参数；仅证明该读路径，可能还有其他历史格式。
- `待核`：字段类型、名称或无绑定调用线索不足以断言编码；BLOB/`[]byte` 本身不代表 JSON。`unbound_hints` 为同函数相关线索，未绑定时不可单独视为证据。
- `摘要/签名/凭据候选` 为字段名识别，具体是凭据、普通摘要或业务标识需继续核验；文档不记录值。
- `primary_keys` 合并模型标签与建表SQL声明；`ownership_columns` 按模型字段名识别身份、参与者与关联列，并非证明实际授权边界。
- `Go无同名SQL列` / `SQL无Go字段` 表示静态文件匹配差异，可能由迁移、生成列、历史兼容或字段别名产生，不能直接视为线上缺列。

## 计数

| 分类 | 字段数 |
|---|---:|
| 普通文本/标识候选 | 986 |
| 摘要/签名/凭据候选 | 534 |
| 未知或文本候选 | 319 |
| JSON | 186 |
| Proto | 146 |
| 二进制格式未知 | 47 |
| 分隔字符串 | 9 |
| 凭据/二进制 | 6 |
| JSON+gzip兼容 | 5 |
| 二进制摘要 | 2 |
| 二进制/WAV | 1 |

| 核验状态 | 字段数 |
|---|---:|
| 已确认（调用链人工核对） | 40 |
| 已确认（静态字段绑定） | 315 |
| 待核 | 1886 |

| 源码匹配情况 | 字段数 |
|---|---:|
| Go+SQL | 2236 |
| Go无同名SQL列 | 4 |
| SQL无Go字段 | 1 |

## 未找到对应建表 SQL 的模型

- `p13_union_fight_settings`：`P13UnionFightSetting`（hyjs-server/db/p13UnionFightSetting.go:5）。
- `t_example`：`ExampleModel`（hyjs-server/db/exampleModel.go:5）。

`profiles` 的 SQLite 与 MySQL 建表 SQL 分别保留在对应 CSV 的 `sql_locations`；同表去重计数。seed 文件不计入结构。

## 源码与生产元数据的表集合

- 生产采样存在、源码未找到模型或建表 SQL：0 表。
- 源码存在、生产采样未见：2 表：`p13_union_fight_settings`, `t_example`。
- 生产目标类型列中，源码模型和建表SQL均未匹配的字段：0（逐项在 CSV 标记，仅为静态映射差异）。
- 解析警告：0。

## 重点编码示例与名称差异

- `profiles.player` / `profiles.game`：角色存档 JSON；`game` 内的待结算战斗还可能含压缩输入（`hyjs-server/service/profileService.go:287`, `hyjs-server/service/fieldBossService.go:641`）。
- `p6_battle_records.input`：历史原文 JSON 与 gzip JSON 兼容；`operates` 为 Proto，`outcome` 为 JSON（`hyjs-server/service/battleReplayService.go:42`, `:67`, `:202`, `:558`, `:563`）。
- `p7_rush_manual_calculations.series`、`p7_rush_scoped_calculations.series`、`p14_final_result_receipts.input`、`p14_final_continuation_receipts.input`：JSON 原文或 gzip，详见 CSV 对应调用链。
- `ggo_field_points.payload`、`ggo_field_lines.payload`、`ggo_field_receipts.response/chip_delta` 与 `p1_period_instances.frozen` 等已核为 JSON；`p8_gather_attempts.rewards` 为分号/逗号分隔格式。
- `p13_guild_receipts.request/response` 与若干活动请求回执为 Proto；`p10_voice_assets.bytes` 为 WAV 字节，`p14_trusted_peers.signing_key` 为签名密钥，不记录实际值。
- `P12WantedInstance.MonsterHPJSON` 按项目 GORM 的常用缩写命名规则映射 `monster_hp_json`，生产采样包含该列；建表 SQL 列名为 `monster_hpjson`（`hyjs-server/db/p12WantedInstance.go:20`、`hyjs-server/db/sql/189_p12_wanted_instances.sql:18`、`hyjs-server/service/p12WantedService.go:227`）。CSV 分别保留两个物理名称；本盘点未检查历史迁移或实际列值。

## 编码分类涉及的表数

同一表可出现多种格式，不可将表数直接相加。`分隔字符串`包括带分隔符的业务标识，不等同于列表。

| 分类 | 字段数 | 不同表数 |
|---|---:|---:|
| JSON | 186 | 150 |
| JSON+gzip兼容 | 5 | 5 |
| Proto | 146 | 89 |
| 分隔字符串 | 9 | 9 |
| 二进制/WAV | 1 | 1 |

## 生产表中待核的 TEXT/BLOB 字段

从“未知或文本候选”和“二进制格式未知”中筛出源码建表 SQL 为 TEXT/BLOB、生产采样有同名目标类型列且所在表 `DATA_LENGTH` 大于零的字段，共 116 字段、94 表。按所在表采样的数据分配字节数降序；以下前 10 位因第 10 位并列列出 11 个字段。分配量属于整张表，无法代表任何单个字段的实际大小、内容或访问频率。

| 待核字段 | SQL 类型 | 采样表 DATA_LENGTH（字节） | 模型 / SQL 证据 |
|---|---|---:|---|
| `p12_world_boss_battle_receipts.finish_proto` | LONGBLOB | 9977856 | hyjs-server/db/p12WorldBossBattleReceipt.go:16；hyjs-server/db/sql/181_p12_world_boss_battle_receipts.sql:13 |
| `p1_reward_deliveries.content` | TEXT | 5750784 | hyjs-server/db/rewardDelivery.go:9；hyjs-server/db/sql/80_p1_reward_deliveries.sql:3 |
| `p1_reward_deliveries.result` | LONGBLOB | 5750784 | hyjs-server/db/rewardDelivery.go:14；hyjs-server/db/sql/80_p1_reward_deliveries.sql:3 |
| `p1_reward_deliveries.rewards` | TEXT | 5750784 | hyjs-server/db/rewardDelivery.go:10；hyjs-server/db/sql/80_p1_reward_deliveries.sql:3 |
| `p11_arena_prize_awards.boxes_json` | TEXT | 3686400 | hyjs-server/db/p11ArenaPrizeAward.go:23；hyjs-server/db/sql/147_p11_arena_prize_awards.sql:8 |
| `p1_period_instances.rank_rewards` | TEXT | 2605056 | hyjs-server/db/periodInstance.go:22；hyjs-server/db/sql/80_p1_period_instances.sql:3 |
| `p1_period_instances.rewards` | TEXT | 2605056 | hyjs-server/db/periodInstance.go:17；hyjs-server/db/sql/80_p1_period_instances.sql:3 |
| `p8_resource_nodes.snapshot` | LONGTEXT | 1589248 | hyjs-server/db/p8ResourceNode.go:13；hyjs-server/db/sql/87_p8_resource_nodes.sql:5 |
| `p12_wanted_discoveries.reward` | TEXT | 491520 | hyjs-server/db/p12WantedDiscovery.go:12；hyjs-server/db/sql/189_p12_wanted_discoveries.sql:10 |
| `p7_rush_stage_rules.first_reward` | TEXT | 409600 | hyjs-server/db/p7RushStageRule.go:18；hyjs-server/db/sql/86_p7_rush_stage_rules.sql:2 |
| `p7_rush_stage_rules.reward` | TEXT | 409600 | hyjs-server/db/p7RushStageRule.go:19；hyjs-server/db/sql/86_p7_rush_stage_rules.sql:2 |

`p12_wanted_instances.monster_hpjson` 的 SQL 类型为 LONGTEXT，所在表采样 `DATA_LENGTH` 为 63455232 字节；但采样中没有该同名目标类型列，因此未列入以上生产列清单。它与生产样本中的 `monster_hp_json` 是两个不同的列名，不能据表分配量推断其存在或存储内容。

## 全部表覆盖

| 表 | 模型 | 候选数 | 主键 | 身份关联列线索 | 建表 SQL | 生产采样 |
|---|---|---:|---|---|---|---|
| `account_wallet_balances` | `AccountWalletBalance` | 0 | `account_id` | `account_id` | hyjs-server/db/sql/217_account_wallet_balances.sql:1 | 存在 |
| `account_wallet_entries` | `AccountWalletEntry` | 7 | `account_id;key` | `account_id;player_id;server_id` | hyjs-server/db/sql/218_account_wallet_entries.sql:1 | 存在 |
| `account_wallet_migrations` | `AccountWalletMigration` | 0 | `player_id;server_id` | `account_id;player_id;server_id` | hyjs-server/db/sql/219_account_wallet_migrations.sql:1 | 存在 |
| `activity_calendar_period_archives` | `ActivityCalendarPeriodArchive` | 4 | `instance_id` | `源码未识别，需核业务` | hyjs-server/db/sql/activity_calendar_period_archives.sql:1 | 存在 |
| `activity_login_states` | `ActivityLoginState` | 3 | `activity_id;player_id` | `player_id` | hyjs-server/db/sql/11_activity_login_states.sql:2 | 存在 |
| `activity_recharge_carries` | `ActivityRechargeCarry` | 6 | `instance_id;player_id` | `player_id` | hyjs-server/db/sql/activity_recharge_carries.sql:1 | 存在 |
| `activity_rewards` | `ActivityReward` | 3 | `activity_id;reward_type;threshold` | `源码未识别，需核业务` | hyjs-server/db/sql/09_activity_rewards.sql:2 | 存在 |
| `activity_rules` | `ActivityRules` | 2 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/08_activity_rules.sql:2 | 存在 |
| `activity_states` | `ActivityState` | 2 | `activity_id;player_id` | `player_id` | hyjs-server/db/sql/10_activity_states.sql:2 | 存在 |
| `admin_activity_occurrences` | `AdminActivityOccurrence` | 9 | `id` | `server_id` | hyjs-server/db/sql/201_admin_activity_occurrences.sql:1 | 存在 |
| `admin_activity_patterns` | `AdminActivityPattern` | 3 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/202_admin_activity_patterns.sql:1 | 存在 |
| `admin_activity_presentations` | `AdminActivityPresentation` | 5 | `activity_id` | `源码未识别，需核业务` | hyjs-server/db/sql/202_admin_activity_presentations.sql:1 | 存在 |
| `admin_activity_schedule_revisions` | `AdminActivityScheduleRevision` | 0 | `server_id` | `server_id` | hyjs-server/db/sql/203_admin_activity_schedule_revisions.sql:1 | 存在 |
| `admin_activity_template_owners` | `AdminActivityTemplateOwner` | 2 | `player_id;server_id;template_id` | `player_id;server_id` | hyjs-server/db/sql/221_admin_activity_template_owners.sql:1 | 存在 |
| `admin_activity_templates` | `AdminActivityDefinition` | 6 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/200_admin_activity_templates.sql:1 | 存在 |
| `admin_announcements` | `AdminAnnouncement` | 2 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/200_admin_announcements.sql:1 | 存在 |
| `admin_audits` | `AdminAudit` | 4 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/199_admin_audits.sql:1 | 存在 |
| `admin_broadcast_batches` | `AdminBroadcastBatch` | 10 | `id` | `actor_id;server_id` | hyjs-server/db/sql/232_admin_broadcast_batches.sql:1 | 存在 |
| `admin_broadcast_recipients` | `AdminBroadcastRecipient` | 2 | `batch_id;player_id` | `player_id` | hyjs-server/db/sql/233_admin_broadcast_recipients.sql:1 | 存在 |
| `admin_cdk_codes` | `AdminCDKCode` | 3 | `hash` | `server_id` | hyjs-server/db/sql/226_admin_cdk_codes.sql:1 | 存在 |
| `admin_character_activity_instances` | `AdminCharacterActivityInstance` | 6 | `occurrence_id;player_id` | `player_id;server_id` | hyjs-server/db/sql/220_admin_character_activity_instances.sql:1 | 存在 |
| `admin_field_battle_snapshots` | `AdminFieldBattleSnapshot` | 3 | `fight_id` | `player_id` | hyjs-server/db/sql/212_admin_field_battle_snapshots.sql:1 | 存在 |
| `admin_field_channel_activations` | `AdminFieldChannelActivation` | 1 | `server_id` | `server_id` | hyjs-server/db/sql/214_admin_field_channel_activations.sql:1 | 存在 |
| `admin_field_channels` | `AdminFieldChannel` | 1 | `scene;server_id` | `server_id` | hyjs-server/db/sql/213_admin_field_channels.sql:1 | 存在 |
| `admin_field_deployments` | `AdminFieldDeployment` | 3 | `scene;server_id` | `server_id` | hyjs-server/db/sql/211_admin_field_deployments.sql:1 | 存在 |
| `admin_field_templates` | `AdminFieldTemplate` | 4 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/210_admin_field_templates.sql:1 | 存在 |
| `admin_game_servers` | `AdminGameServer` | 1 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/198_admin_game_servers.sql:1 | 存在 |
| `admin_grant_receipts` | `AdminGrantReceipt` | 2 | `request_id` | `actor_id` | hyjs-server/db/sql/201_admin_grant_receipts.sql:1 | 存在 |
| `admin_guild_settings` | `AdminGuildSetting` | 3 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/239_admin_guild_settings.sql:1 | 存在 |
| `admin_login_throttles` | `AdminLoginThrottle` | 1 | `username` | `源码未识别，需核业务` | hyjs-server/db/sql/198_admin_login_throttles.sql:1 | 存在 |
| `admin_mail_batches` | `AdminMailBatch` | 9 | `id` | `actor_id;server_id` | hyjs-server/db/sql/234_admin_mail_batches.sql:1 | 存在 |
| `admin_mail_recipients` | `AdminMailRecipient` | 2 | `batch_id;player_id` | `player_id` | hyjs-server/db/sql/235_admin_mail_recipients.sql:1 | 存在 |
| `admin_native_activity_policies` | `AdminNativeActivityPolicy` | 3 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/238_admin_native_activity_policies.sql:2 | 存在 |
| `admin_opening_counters` | `AdminOpeningCounter` | 2 | `counter_key;occurrence_id;player_id` | `player_id` | hyjs-server/db/sql/223_admin_opening_counters.sql:1 | 存在 |
| `admin_opening_receipts` | `AdminOpeningReceipt` | 3 | `player_id;timestamp` | `player_id` | hyjs-server/db/sql/225_admin_opening_receipts.sql:1 | 存在 |
| `admin_opening_states` | `AdminOpeningState` | 1 | `occurrence_id;player_id;task_id` | `player_id` | hyjs-server/db/sql/222_admin_opening_states.sql:1 | 存在 |
| `admin_opening_stocks` | `AdminOpeningStock` | 1 | `occurrence_id;task_id` | `源码未识别，需核业务` | hyjs-server/db/sql/224_admin_opening_stocks.sql:1 | 存在 |
| `admin_security_events` | `AdminSecurityEvent` | 8 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/236_admin_security_events.sql:1 | 存在 |
| `admin_sessions` | `AdminSession` | 2 | `token_hash` | `user_id` | hyjs-server/db/sql/197_admin_sessions.sql:1 | 存在 |
| `admin_shop_availability` | `AdminShopAvailability` | 1 | `shop_key` | `源码未识别，需核业务` | hyjs-server/db/sql/229_admin_shop_availability.sql:1 | 存在 |
| `admin_sprout_settings` | `AdminSproutSetting` | 1 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/admin_sprout_settings.sql:1 | 存在 |
| `admin_title_overrides` | `AdminTitleOverride` | 3 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/207_admin_title_overrides.sql:2 | 存在 |
| `admin_users` | `AdminUser` | 3 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/196_admin_users.sql:1 | 存在 |
| `client_update_actions` | `ClientUpdateAction` | 3 | `request_id` | `actor_id` | hyjs-server/db/sql/241_client_update_actions.sql:1 | 存在 |
| `client_update_policy` | `ClientUpdatePolicy` | 6 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/241_client_update_policy.sql:1 | 存在 |
| `client_update_receipts` | `ClientUpdateReceipt` | 2 | `batch_id;kind;startup_id` | `player_id` | hyjs-server/db/sql/241_client_update_receipts.sql:1 | 存在 |
| `client_update_releases` | `ClientUpdateRelease` | 6 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/241_client_update_releases.sql:1 | 存在 |
| `client_update_sessions` | `ClientUpdateSession` | 3 | `player_id` | `player_id` | hyjs-server/db/sql/241_client_update_sessions.sql:1 | 存在 |
| `cooking_craft_states` | `CookingCraftState` | 0 | `player_id` | `player_id` | hyjs-server/db/sql/48_cooking_craft_states.sql:1 | 存在 |
| `cooking_effects` | `CookingEffect` | 2 | `player_id` | `player_id` | hyjs-server/db/sql/51_cooking_effects.sql:1 | 存在 |
| `cooking_mutation_receipts` | `CookingMutationReceipt` | 2 | `opcode;player_id` | `player_id` | hyjs-server/db/sql/49_cooking_mutation_receipts.sql:1 | 存在 |
| `cooking_rules` | `CookingRule` | 2 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/50_cooking_rules.sql:2 | 存在 |
| `copy_box_claims` | `CopyBoxClaim` | 3 | `box_id;player_id` | `player_id` | hyjs-server/db/sql/110_copy_box_claims.sql:1 | 存在 |
| `copy_box_rules` | `CopyBoxRule` | 1 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/110_copy_box_rules.sql:1 | 存在 |
| `copy_drop_rules` | `CopyDropRule` | 3 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/97_copy_drop_rules.sql:1 | 存在 |
| `copy_drop_snapshots` | `CopyDropSnapshot` | 4 | `fight_id;player_id` | `player_id` | hyjs-server/db/sql/97_copy_drop_snapshots.sql:1 | 存在 |
| `copy_reset_daily` | `CopyResetDaily` | 3 | `day;player_id` | `player_id` | hyjs-server/db/sql/109_copy_reset_daily.sql:1 | 存在 |
| `copy_reset_rules` | `CopyResetRule` | 2 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/109_copy_reset_rules.sql:1 | 存在 |
| `copy_special_rules` | `CopySpecialRule` | 2 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/108_copy_special_rules.sql:2 | 存在 |
| `copy_special_states` | `CopySpecialState` | 3 | `player_id;reward_id;task_id` | `player_id` | hyjs-server/db/sql/108_copy_special_states.sql:1 | 存在 |
| `copy_sweep_witnesses` | `CopySweepWitness` | 4 | `player_id` | `player_id` | hyjs-server/db/sql/copy_sweep_witnesses.sql:1 | 存在 |
| `daily_copy_fights` | `DailyCopyFight` | 8 | `fight_id` | `player_id` | hyjs-server/db/sql/41_daily_copy_fights.sql:1 | 存在 |
| `daily_copy_policies` | `DailyCopyPolicy` | 3 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/39_daily_copy_policies.sql:1 | 存在 |
| `daily_copy_start_receipts` | `DailyCopyStartReceipt` | 2 | `player_id;stamp` | `player_id` | hyjs-server/db/sql/43_daily_copy_start_receipts.sql:1 | 存在 |
| `daily_copy_states` | `DailyCopyState` | 2 | `day;player_id;type` | `player_id` | hyjs-server/db/sql/40_daily_copy_states.sql:1 | 存在 |
| `daily_task_box_rules` | `DailyTaskBoxRule` | 3 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/33_daily_task_box_rules.sql:1 | 存在 |
| `daily_task_rules` | `DailyTaskRule` | 3 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/32_daily_task_rules.sql:1 | 存在 |
| `daily_task_states` | `DailyTaskState` | 2 | `day;player_id` | `player_id` | hyjs-server/db/sql/34_daily_task_states.sql:1 | 存在 |
| `equipment_mutation_receipts` | `EquipmentMutationReceipt` | 2 | `opcode;player_id` | `player_id` | hyjs-server/db/sql/20_equipment_mutation_receipts.sql:2 | 存在 |
| `equipment_recycle_rules` | `EquipmentRecycleRule` | 2 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/19_equipment_recycle_rules.sql:1 | 存在 |
| `field_boss_contributions73` | `FieldBossContribution73` | 5 | `id` | `player_id;server_id` | hyjs-server/db/sql/field_boss_contributions73.sql:1 | 存在 |
| `field_resource_policies` | `FieldResourcePolicy` | 1 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/field_resource_policies.sql:1 | 存在 |
| `first_double_policies` | `FirstDoublePolicy` | 2 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/195_first_double_policies.sql:1 | 存在 |
| `game_account_roles` | `GameAccountRole` | 1 | `account_id;server_id` | `account_id;player_id;server_id` | hyjs-server/db/sql/216_game_account_roles.sql:1 | 存在 |
| `game_account_sessions` | `GameAccountSession` | 1 | `token_hash` | `account_id` | hyjs-server/db/sql/game_account_sessions.sql:1 | 存在 |
| `game_accounts` | `GameAccount` | 4 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/215_game_accounts.sql:2 | 存在 |
| `game_feedback` | `GameFeedback` | 5 | `id` | `owner_id;player_id` | hyjs-server/db/sql/208_game_feedback.sql:2 | 存在 |
| `game_feedback_messages` | `GameFeedbackMessage` | 5 | `id` | `actor_id` | hyjs-server/db/sql/209_game_feedback_messages.sql:2 | 存在 |
| `game_login_throttles` | `GameLoginThrottle` | 1 | `account_key` | `源码未识别，需核业务` | hyjs-server/db/sql/237_game_login_throttles.sql:2 | 存在 |
| `gameplay_pool_rewards` | `GameplayPoolReward` | 2 | `id;pool_id` | `源码未识别，需核业务` | hyjs-server/db/sql/03_gameplay_pools.sql:23 | 存在 |
| `gameplay_pools` | `GameplayPool` | 7 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/03_gameplay_pools.sql:2 | 存在 |
| `gem_craft_states` | `GemCraftState` | 0 | `player_id` | `player_id` | hyjs-server/db/sql/35_gem_craft_states.sql:1 | 存在 |
| `gem_mutation_receipts` | `GemMutationReceipt` | 2 | `opcode;player_id` | `player_id` | hyjs-server/db/sql/36_gem_mutation_receipts.sql:1 | 存在 |
| `gem_rules` | `GemRule` | 1 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/37_gem_rules.sql:1 | 存在 |
| `ggo_account_weekly` | `GGOAccountWeekly` | 2 | `account_id;product_id;week_start` | `account_id` | hyjs-server/db/sql/ggo_account_weekly.sql:1 | 存在 |
| `ggo_field_lines` | `GGOFieldLine` | 2 | `line;server_id` | `server_id` | hyjs-server/db/sql/ggo_field_lines.sql:1 | 存在 |
| `ggo_field_points` | `GGOFieldPoint` | 3 | `line;point_id;server_id` | `server_id` | hyjs-server/db/sql/ggo_field_points.sql:1 | 存在 |
| `ggo_field_receipts` | `GGOFieldReceipt` | 7 | `player_id;request_id` | `player_id` | hyjs-server/db/sql/ggo_field_receipts.sql:1 | 存在 |
| `ggo_shop_receipts` | `GGOShopReceipt` | 4 | `player_id;request_id` | `player_id` | hyjs-server/db/sql/242_ggo_shop_receipts.sql:1 | 存在 |
| `growth_recruit_policies` | `GrowthRecruitPolicy` | 2 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/191_growth_recruit_policies.sql:1 | 存在 |
| `king_recruit_activity_snapshots` | `KingRecruitActivitySnapshot` | 2 | `pool_id` | `源码未识别，需核业务` | hyjs-server/db/sql/230_king_recruit_activity_snapshots.sql:1 | 存在 |
| `king_recruit_selections` | `KingRecruitSelection` | 1 | `player_id` | `player_id` | hyjs-server/db/sql/195_king_recruit_selections.sql:2 | 存在 |
| `local_play_activity_bindings` | `LocalPlayActivityBinding` | 2 | `activity_id;player_id` | `player_id` | hyjs-server/db/sql/193_local_play_activity_bindings.sql:1 | 存在 |
| `local_play_activity_cycles` | `LocalPlayActivityCycle` | 4 | `id` | `player_id` | hyjs-server/db/sql/193_local_play_activity_cycles.sql:1 | 存在 |
| `local_play_activity_operations` | `LocalPlayActivityOperation` | 4 | `player_id;request_id` | `player_id` | hyjs-server/db/sql/193_local_play_activity_operations.sql:1 | 存在 |
| `local_play_clock_operations` | `LocalPlayClockOperation` | 3 | `player_id;request_id` | `player_id` | hyjs-server/db/sql/194_local_play_clock_operations.sql:1 | 存在 |
| `local_play_clock_states` | `LocalPlayClockState` | 0 | `player_id` | `player_id` | hyjs-server/db/sql/194_local_play_clock_states.sql:1 | 存在 |
| `local_play_open_server_counters` | `LocalPlayOpenServerCounter` | 2 | `counter_key;cycle_id` | `player_id` | hyjs-server/db/sql/193_local_play_open_server_counters.sql:1 | 存在 |
| `local_play_open_server_receipts` | `LocalPlayOpenServerReceipt` | 3 | `player_id;timestamp` | `player_id` | hyjs-server/db/sql/193_local_play_open_server_receipts.sql:1 | 存在 |
| `local_play_open_server_states` | `LocalPlayOpenServerState` | 1 | `cycle_id;task_id` | `player_id` | hyjs-server/db/sql/193_local_play_open_server_states.sql:1 | 存在 |
| `local_play_payment_times` | `LocalPlayPaymentTime` | 1 | `order_id` | `player_id` | hyjs-server/db/sql/194_local_play_payment_times.sql:1 | 存在 |
| `local_play_policies` | `LocalPlayPolicy` | 2 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/194_local_play_policies.sql:1 | 存在 |
| `local_wallet_balances` | `LocalWalletBalance` | 0 | `player_id` | `player_id` | hyjs-server/db/sql/192_local_wallet_balances.sql:1 | 存在 |
| `local_wallet_entries` | `LocalWalletEntry` | 6 | `key;player_id` | `player_id` | hyjs-server/db/sql/192_local_wallet_entries.sql:1 | 存在 |
| `local_wallet_policies` | `LocalWalletPolicy` | 2 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/192_local_wallet_policies.sql:1 | 存在 |
| `local_wallet_reasons` | `LocalWalletReason` | 2 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/203_local_wallet_reasons.sql:1 | 存在 |
| `month_sign_calendars` | `MonthSignCalendar` | 2 | `month_key` | `源码未识别，需核业务` | hyjs-server/db/sql/month_sign_calendars.sql:1 | 存在 |
| `month_sign_rules` | `MonthSignRule` | 2 | `day` | `源码未识别，需核业务` | hyjs-server/db/sql/21_month_sign_rules.sql:1 | 存在 |
| `month_sign_states` | `MonthSignState` | 3 | `month;player_id` | `player_id` | hyjs-server/db/sql/22_month_sign_states.sql:1 | 存在 |
| `open_server_activity_counters` | `OpenServerActivityCounter` | 1 | `counter_key;player_id` | `player_id` | hyjs-server/db/sql/27_open_server_activity_counters.sql:1 | 存在 |
| `open_server_activity_receipts` | `OpenServerActivityReceipt` | 2 | `player_id;timestamp` | `player_id` | hyjs-server/db/sql/28_open_server_activity_receipts.sql:1 | 存在 |
| `open_server_activity_rules` | `OpenServerActivityRule` | 2 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/24_open_server_activity_rules.sql:1 | 存在 |
| `open_server_activity_states` | `OpenServerActivityState` | 0 | `player_id;task_id` | `player_id` | hyjs-server/db/sql/26_open_server_activity_states.sql:1 | 存在 |
| `open_server_activity_tasks` | `OpenServerActivityTask` | 2 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/25_open_server_activity_tasks.sql:1 | 存在 |
| `opening_calendar_carries` | `OpeningCalendarCarry` | 5 | `player_id;scope` | `player_id` | hyjs-server/db/sql/opening_calendar_carries.sql:1 | 存在 |
| `opening_calendar_payment_carries` | `OpeningCalendarPaymentCarry` | 6 | `player_id;scope_id` | `player_id` | hyjs-server/db/sql/opening_calendar_payment_carries.sql:1 | 存在 |
| `p10_channel_chats` | `P10ChannelChat` | 3 | `id` | `sender_id` | hyjs-server/db/sql/89_p10_channel_chats.sql:1 | 存在 |
| `p10_channel_policies` | `P10ChannelPolicy` | 1 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/89_p10_channel_policies.sql:1 | 存在 |
| `p10_expedition_help_actions` | `P10ExpeditionHelpAction` | 8 | `event_serial;mission_serial;owner_id` | `owner_id` | hyjs-server/db/sql/104_p10_expedition_help_actions.sql:1 | 存在 |
| `p10_expedition_help_dailies` | `P10ExpeditionHelpDaily` | 5 | `day;player_id` | `player_id` | hyjs-server/db/sql/104_p10_expedition_help_dailies.sql:1 | 存在 |
| `p10_expedition_help_policies` | `P10ExpeditionHelpPolicy` | 1 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/104_p10_expedition_help_policies.sql:1 | 存在 |
| `p10_friend_energy_daily` | `P10FriendEnergyDaily` | 1 | `day;player_id` | `player_id` | hyjs-server/db/sql/89_p10_friend_energy_daily.sql:1 | 存在 |
| `p10_friend_energy_gifts` | `P10FriendEnergyGift` | 1 | `day;receiver_id;sender_id` | `receiver_id;sender_id` | hyjs-server/db/sql/89_p10_friend_energy_gifts.sql:1 | 存在 |
| `p10_friend_energy_policies` | `P10FriendEnergyPolicy` | 1 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/89_p10_friend_energy_policies.sql:1 | 存在 |
| `p10_lucky_dailies` | `P10LuckyDaily` | 3 | `day;player_id` | `player_id` | hyjs-server/db/sql/89_p10_lucky_dailies.sql:1 | 存在 |
| `p10_lucky_objects` | `P10LuckyObject` | 7 | `guid` | `owner_id` | hyjs-server/db/sql/89_p10_lucky_objects.sql:1 | 存在 |
| `p10_lucky_policies` | `P10LuckyPolicy` | 3 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/89_p10_lucky_policies.sql:1 | 存在 |
| `p10_scene_policies` | `P10ScenePolicy` | 1 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/89_p10_scene_policies.sql:1 | 存在 |
| `p10_scene_presence` | `P10ScenePresence` | 3 | `player_id` | `player_id` | hyjs-server/db/sql/89_p10_scene_presence.sql:1 | 存在 |
| `p10_social_applies` | `P10SocialApply` | 1 | `receiver_id;sender_id` | `receiver_id;sender_id` | hyjs-server/db/sql/89_p10_social_applies.sql:1 | 存在 |
| `p10_social_blacks` | `P10SocialBlack` | 0 | `owner_id;target_id` | `owner_id` | hyjs-server/db/sql/89_p10_social_blacks.sql:1 | 存在 |
| `p10_social_chat_states` | `P10SocialChatState` | 0 | `player_id` | `player_id` | hyjs-server/db/sql/89_p10_social_chat_states.sql:1 | 存在 |
| `p10_social_chats` | `P10SocialChat` | 3 | `id` | `receiver_id;sender_id` | hyjs-server/db/sql/89_p10_social_chats.sql:1 | 存在 |
| `p10_social_policies` | `P10SocialPolicy` | 1 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/89_p10_social_policies.sql:1 | 存在 |
| `p10_social_receipts` | `P10SocialReceipt` | 3 | `domain;player_id;stamp` | `player_id` | hyjs-server/db/sql/89_p10_social_receipts.sql:1 | 存在 |
| `p10_social_relations` | `P10SocialRelation` | 0 | `high_id;low_id` | `源码未识别，需核业务` | hyjs-server/db/sql/89_p10_social_relations.sql:1 | 存在 |
| `p10_voice_assets` | `P10VoiceAsset` | 10 | `id` | `owner_id;target_id` | hyjs-server/db/sql/121_p10_voice_assets.sql:1 | 存在 |
| `p10_voice_messages` | `P10VoiceMessage` | 5 | `id` | `owner_id;target_id` | hyjs-server/db/sql/121_p10_voice_messages.sql:1 | 存在 |
| `p10_voice_policies` | `P10VoicePolicy` | 1 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/121_p10_voice_policies.sql:1 | 存在 |
| `p10_worship_actions` | `P10WorshipAction` | 4 | `day;player_id;target_id` | `player_id;target_id` | hyjs-server/db/sql/102_p10_worship_actions.sql:1 | 存在 |
| `p10_worship_dailies` | `P10WorshipDaily` | 4 | `day;player_id` | `player_id` | hyjs-server/db/sql/102_p10_worship_dailies.sql:1 | 存在 |
| `p10_worship_policies` | `P10WorshipPolicy` | 1 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/102_p10_worship_policies.sql:1 | 存在 |
| `p11_arena_daily_awards` | `P11ArenaDailyAward` | 6 | `day;player_id` | `player_id` | hyjs-server/db/sql/149_p11_arena_daily_awards.sql:1 | 存在 |
| `p11_arena_daily_challenges` | `P11ArenaDailyChallenge` | 6 | `id` | `opponent_id;player_id` | hyjs-server/db/sql/149_p11_arena_daily_challenges.sql:1 | 存在 |
| `p11_arena_daily_offers` | `P11ArenaDailyOffer` | 3 | `day` | `源码未识别，需核业务` | hyjs-server/db/sql/149_p11_arena_daily_offers.sql:1 | 存在 |
| `p11_arena_daily_policies` | `P11ArenaDailyPolicy` | 2 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/149_p11_arena_daily_policies.sql:1 | 存在 |
| `p11_arena_daily_snapshots` | `P11ArenaDailySnapshot` | 6 | `day` | `源码未识别，需核业务` | hyjs-server/db/sql/149_p11_arena_daily_snapshots.sql:1 | 存在 |
| `p11_arena_fight_policies` | `P11ArenaFightPolicy` | 1 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/91_p11_arena_fight_policies.sql:1 | 存在 |
| `p11_arena_fight_settlements` | `P11ArenaFightSettlement` | 11 | `player_id;stamp` | `opponent_id;player_id;winner_id` | hyjs-server/db/sql/92_p11_arena_fight_settlements.sql:1 | 存在 |
| `p11_arena_matches` | `P11ArenaMatch` | 7 | `id` | `opponent_id` | hyjs-server/db/sql/90_p11_arena_matches.sql:1 | 存在 |
| `p11_arena_members` | `P11ArenaMember` | 3 | `player_id` | `player_id` | hyjs-server/db/sql/90_p11_arena_members.sql:1 | 存在 |
| `p11_arena_policies` | `P11ArenaPolicy` | 1 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/90_p11_arena_policies.sql:1 | 存在 |
| `p11_arena_prize_awards` | `P11ArenaPrizeAward` | 16 | `fight_id` | `player_id;winner_id` | hyjs-server/db/sql/147_p11_arena_prize_awards.sql:1 | 存在 |
| `p11_arena_prize_offers` | `P11ArenaPrizeOffer` | 3 | `month` | `源码未识别，需核业务` | hyjs-server/db/sql/147_p11_arena_prize_offers.sql:1 | 存在 |
| `p11_arena_prize_policies` | `P11ArenaPrizePolicy` | 3 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/147_p11_arena_prize_policies.sql:1 | 存在 |
| `p11_arena_receipts` | `P11ArenaReceipt` | 4 | `operation;player_id;stamp` | `player_id` | hyjs-server/db/sql/90_p11_arena_receipts.sql:1 | 存在 |
| `p11_friend_duels` | `P11FriendDuel` | 6 | `id` | `opponent_id` | hyjs-server/db/sql/90_p11_friend_duels.sql:1 | 存在 |
| `p11_friend_fight_settlements` | `P11FriendFightSettlement` | 8 | `player_id;stamp` | `opponent_id;player_id` | hyjs-server/db/sql/90_p11_friend_fight_settlements.sql:1 | 存在 |
| `p11_grab_daily` | `P11GrabDaily` | 3 | `day;player_id` | `player_id` | hyjs-server/db/sql/90_p11_grab_daily.sql:1 | 存在 |
| `p11_grab_fight_offers` | `P11GrabFightOffer` | 3 | `version` | `源码未识别，需核业务` | hyjs-server/db/sql/168_p11_grab_fight_offers.sql:1 | 存在 |
| `p11_grab_fight_policies` | `P11GrabFightPolicy` | 2 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/168_p11_grab_fight_policies.sql:1 | 存在 |
| `p11_grab_fight_settlements` | `P11GrabFightSettlement` | 15 | `player_id;stamp` | `defender_id;player_id` | hyjs-server/db/sql/168_p11_grab_fight_settlements.sql:1 | 存在 |
| `p11_grab_policies` | `P11GrabPolicy` | 3 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/90_p11_grab_policies.sql:1 | 存在 |
| `p11_grab_random_awards` | `P11GrabRandomAward` | 6 | `month;ordinal;player_id` | `player_id` | hyjs-server/db/sql/153_p11_grab_random_awards.sql:1 | 存在 |
| `p11_grab_random_offers` | `P11GrabRandomOffer` | 3 | `month` | `源码未识别，需核业务` | hyjs-server/db/sql/153_p11_grab_random_offers.sql:1 | 存在 |
| `p11_grab_random_policies` | `P11GrabRandomPolicy` | 3 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/153_p11_grab_random_policies.sql:1 | 存在 |
| `p11_grab_reward_claims` | `P11GrabRewardClaim` | 4 | `day;hours;player_id` | `player_id` | hyjs-server/db/sql/90_p11_grab_reward_claims.sql:1 | 存在 |
| `p11_grab_sessions` | `P11GrabSession` | 2 | `id` | `player_id` | hyjs-server/db/sql/90_p11_grab_sessions.sql:1 | 存在 |
| `p11_grab_slots` | `P11GrabSlot` | 1 | `position` | `owner_id` | hyjs-server/db/sql/90_p11_grab_slots.sql:1 | 存在 |
| `p11_limit_daily` | `P11LimitDaily` | 2 | `day;period_id;player_id` | `player_id` | hyjs-server/db/sql/90_p11_limit_daily.sql:1 | 存在 |
| `p11_limit_daily_awards` | `P11LimitDailyAward` | 6 | `offer_id;player_id` | `player_id` | hyjs-server/db/sql/164_p11_limit_daily_awards.sql:1 | 存在 |
| `p11_limit_daily_challenges` | `P11LimitDailyChallenge` | 7 | `id` | `opponent_id;player_id` | hyjs-server/db/sql/164_p11_limit_daily_challenges.sql:1 | 存在 |
| `p11_limit_daily_offers` | `P11LimitDailyOffer` | 6 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/164_p11_limit_daily_offers.sql:1 | 存在 |
| `p11_limit_daily_policies` | `P11LimitDailyPolicy` | 2 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/164_p11_limit_daily_policies.sql:1 | 存在 |
| `p11_limit_daily_snapshots` | `P11LimitDailySnapshot` | 8 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/164_p11_limit_daily_snapshots.sql:1 | 存在 |
| `p11_limit_fight_matches` | `P11LimitFightMatch` | 10 | `id` | `opponent_id;player_id` | hyjs-server/db/sql/163_p11_limit_fight_matches.sql:1 | 存在 |
| `p11_limit_fight_offers` | `P11LimitFightOffer` | 3 | `version` | `源码未识别，需核业务` | hyjs-server/db/sql/163_p11_limit_fight_offers.sql:1 | 存在 |
| `p11_limit_fight_policies` | `P11LimitFightPolicy` | 2 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/163_p11_limit_fight_policies.sql:1 | 存在 |
| `p11_limit_fight_settlements` | `P11LimitFightSettlement` | 12 | `player_id;stamp` | `opponent_id;player_id` | hyjs-server/db/sql/163_p11_limit_fight_settlements.sql:1 | 存在 |
| `p11_limit_members` | `P11LimitMember` | 1 | `period_id;player_id` | `player_id` | hyjs-server/db/sql/90_p11_limit_members.sql:1 | 存在 |
| `p11_limit_periods` | `P11LimitPeriod` | 4 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/90_p11_limit_periods.sql:1 | 存在 |
| `p11_limit_policies` | `P11LimitPolicy` | 2 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/90_p11_limit_policies.sql:1 | 存在 |
| `p11_limit_reward_claims` | `P11LimitRewardClaim` | 4 | `period_id;player_id;reward_key` | `player_id` | hyjs-server/db/sql/90_p11_limit_reward_claims.sql:1 | 存在 |
| `p11_plunder_fight_matches` | `P11PlunderFightMatch` | 10 | `id` | `opponent_id;player_id` | hyjs-server/db/sql/156_p11_plunder_fight_matches.sql:1 | 存在 |
| `p11_plunder_fight_offers` | `P11PlunderFightOffer` | 3 | `version` | `源码未识别，需核业务` | hyjs-server/db/sql/156_p11_plunder_fight_offers.sql:1 | 存在 |
| `p11_plunder_fight_policies` | `P11PlunderFightPolicy` | 2 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/156_p11_plunder_fight_policies.sql:1 | 存在 |
| `p11_plunder_fight_settlements` | `P11PlunderFightSettlement` | 12 | `player_id;stamp` | `opponent_id;player_id` | hyjs-server/db/sql/156_p11_plunder_fight_settlements.sql:1 | 存在 |
| `p11_plunder_policies` | `P11PlunderPolicy` | 3 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/90_p11_plunder_policies.sql:1 | 存在 |
| `p11_plunder_states` | `P11PlunderState` | 1 | `player_id` | `player_id` | hyjs-server/db/sql/90_p11_plunder_states.sql:1 | 存在 |
| `p11_plunder_sweep_receipts` | `P11PlunderSweepReceipt` | 7 | `player_id;stamp` | `player_id` | hyjs-server/db/sql/156_p11_plunder_sweep_receipts.sql:1 | 存在 |
| `p12_field_pvp_policies` | `P12FieldPVPPolicy` | 2 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/190_p12_field_pvp_policies.sql:1 | 存在 |
| `p12_field_pvp_receipts` | `P12FieldPVPReceipt` | 13 | `fight_id` | `opponent_id;player_id;winner_id` | hyjs-server/db/sql/190_p12_field_pvp_receipts.sql:1 | 存在 |
| `p12_field_pvp_states` | `P12FieldPVPState` | 3 | `player_id` | `player_id` | hyjs-server/db/sql/190_p12_field_pvp_states.sql:1 | 存在 |
| `p12_wanted_discoveries` | `P12WantedDiscovery` | 6 | `player_id;source_key` | `player_id` | hyjs-server/db/sql/189_p12_wanted_discoveries.sql:2 | 存在 |
| `p12_wanted_fights` | `P12WantedFight` | 17 | `id` | `player_id` | hyjs-server/db/sql/189_p12_wanted_fights.sql:2 | 存在 |
| `p12_wanted_instances` | `P12WantedInstance` | 8 | `id` | `finder_id;killer_id;server_id` | hyjs-server/db/sql/189_p12_wanted_instances.sql:2 | 存在 |
| `p12_wanted_policies` | `P12WantedPolicy` | 3 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/189_p12_wanted_policies.sql:2 | 存在 |
| `p12_wanted_reward_claims` | `P12WantedRewardClaim` | 5 | `day;player_id;reward_id` | `player_id` | hyjs-server/db/sql/189_p12_wanted_reward_claims.sql:2 | 存在 |
| `p12_world_boss_awards` | `P12WorldBossAward` | 3 | `instance_id;kind;player_id` | `player_id` | hyjs-server/db/sql/186_p12_world_boss_awards.sql:1 | 存在 |
| `p12_world_boss_battle_instances` | `P12WorldBossBattleInstance` | 2 | `id` | `killer_id` | hyjs-server/db/sql/181_p12_world_boss_battle_instances.sql:1 | 存在 |
| `p12_world_boss_battle_participations` | `P12WorldBossBattleParticipation` | 0 | `player_id` | `player_id` | hyjs-server/db/sql/181_p12_world_boss_battle_participations.sql:1 | 存在 |
| `p12_world_boss_battle_policies` | `P12WorldBossBattlePolicy` | 3 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/181_p12_world_boss_battle_policies.sql:1 | 存在 |
| `p12_world_boss_battle_receipts` | `P12WorldBossBattleReceipt` | 12 | `fight_id` | `player_id` | hyjs-server/db/sql/181_p12_world_boss_battle_receipts.sql:1 | 存在 |
| `p12_world_boss_boosts` | `P12WorldBossBoost` | 3 | `instance_id;player_id;property_id` | `player_id` | hyjs-server/db/sql/186_p12_world_boss_boosts.sql:1 | 存在 |
| `p12_world_boss_cycles` | `P12WorldBossCycle` | 5 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/91_p12_world_boss_cycles.sql:1 | 存在 |
| `p12_world_boss_policies` | `P12WorldBossPolicy` | 3 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/91_p12_world_boss_policies.sql:1 | 存在 |
| `p13_guild_applies` | `P13GuildApply` | 1 | `guild_id;player_id` | `guild_id;player_id` | hyjs-server/db/sql/93_p13_guild_applies.sql:2 | 存在 |
| `p13_guild_donate_box_claims` | `P13GuildDonateBoxClaim` | 7 | `day;ordinal;player_id` | `guild_id;player_id` | hyjs-server/db/sql/183_p13_guild_donate_box_claims.sql:1 | 存在 |
| `p13_guild_donate_box_days` | `P13GuildDonateBoxDay` | 6 | `day;guild_id` | `guild_id` | hyjs-server/db/sql/183_p13_guild_donate_box_days.sql:1 | 存在 |
| `p13_guild_donate_box_policies` | `P13GuildDonateBoxPolicy` | 3 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/183_p13_guild_donate_box_policies.sql:1 | 存在 |
| `p13_guild_donate_box_sources` | `P13GuildDonateBoxSource` | 4 | `player_id;stamp` | `guild_id;player_id` | hyjs-server/db/sql/183_p13_guild_donate_box_sources.sql:1 | 存在 |
| `p13_guild_donation_badge_policies` | `P13GuildDonationBadgePolicy` | 2 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/185_p13_guild_donation_badge_policies.sql:1 | 存在 |
| `p13_guild_donation_badge_sources` | `P13GuildDonationBadgeSource` | 6 | `player_id;stamp` | `guild_id;player_id` | hyjs-server/db/sql/185_p13_guild_donation_badge_sources.sql:1 | 存在 |
| `p13_guild_gift_archives` | `P13GuildGiftArchive` | 5 | `guild_id` | `guild_id` | hyjs-server/db/sql/p13_guild_gift_archives.sql:2 | 存在 |
| `p13_guild_gift_awards` | `P13GuildGiftAward` | 7 | `id` | `guild_id;player_id` | hyjs-server/db/sql/p13_guild_gift_awards.sql:2 | 存在 |
| `p13_guild_gift_credits` | `P13GuildGiftCredit` | 8 | `id` | `guild_id;player_id` | hyjs-server/db/sql/p13_guild_gift_credits.sql:2 | 存在 |
| `p13_guild_gift_cutoffs` | `P13GuildGiftCutoff` | 10 | `id` | `guild_id` | hyjs-server/db/sql/p13_guild_gift_cutoffs.sql:2 | 存在 |
| `p13_guild_gift_heads` | `P13GuildGiftHead` | 7 | `guild_id` | `guild_id` | hyjs-server/db/sql/p13_guild_gift_heads.sql:2 | 存在 |
| `p13_guild_gift_policies` | `P13GuildGiftPolicy` | 2 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/p13_guild_gift_policies.sql:2 | 存在 |
| `p13_guild_gift_rules` | `P13GuildGiftRule` | 9 | `month` | `源码未识别，需核业务` | hyjs-server/db/sql/p13_guild_gift_rules.sql:2 | 存在 |
| `p13_guild_members` | `P13GuildMember` | 4 | `player_id` | `guild_id;player_id` | hyjs-server/db/sql/93_p13_guild_members.sql:2 | 存在 |
| `p13_guild_policies` | `P13GuildPolicy` | 1 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/93_p13_guild_policies.sql:2 | 存在 |
| `p13_guild_prayer_policies` | `P13GuildPrayerPolicy` | 4 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/145_p13_guild_prayer_policies.sql:1 | 存在 |
| `p13_guild_prayer_rolls` | `P13GuildPrayerRoll` | 9 | `player_id;stamp` | `guild_id;player_id` | hyjs-server/db/sql/145_p13_guild_prayer_rolls.sql:1 | 存在 |
| `p13_guild_receipts` | `P13GuildReceipt` | 3 | `operation;player_id;stamp` | `player_id` | hyjs-server/db/sql/93_p13_guild_receipts.sql:2 | 存在 |
| `p13_guild_tech_offers` | `P13GuildTechOffer` | 9 | `guild_id` | `guild_id` | hyjs-server/db/sql/146_p13_guild_tech_offers.sql:1 | 存在 |
| `p13_guild_tech_policies` | `P13GuildTechPolicy` | 4 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/146_p13_guild_tech_policies.sql:1 | 存在 |
| `p13_guild_tech_upgrades` | `P13GuildTechUpgrade` | 7 | `guild_id;level;tech_type` | `guild_id;player_id` | hyjs-server/db/sql/146_p13_guild_tech_upgrades.sql:1 | 存在 |
| `p13_guilds` | `P13Guild` | 4 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/93_p13_guilds.sql:4 | 存在 |
| `p13_team_invites` | `P13TeamInvite` | 1 | `target_id;team_id` | `inviter_id;target_id;team_id` | hyjs-server/db/sql/94_p13_team_invites.sql:2 | 存在 |
| `p13_team_members` | `P13TeamMember` | 1 | `player_id` | `player_id;team_id` | hyjs-server/db/sql/94_p13_team_members.sql:2 | 存在 |
| `p13_team_policies` | `P13TeamPolicy` | 1 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/94_p13_team_policies.sql:2 | 存在 |
| `p13_team_receipts` | `P13TeamReceipt` | 3 | `operation;player_id;stamp` | `player_id` | hyjs-server/db/sql/94_p13_team_receipts.sql:2 | 存在 |
| `p13_teams` | `P13Team` | 1 | `id` | `leader_id` | hyjs-server/db/sql/94_p13_teams.sql:2 | 存在 |
| `p13_union_fight_battle_audit_attempts` | `P13UnionFightBattleAuditAttempt` | 9 | `guid;match_id` | `player_id;target_id` | hyjs-server/db/sql/117_p13_union_fight_battle_audit_attempts.sql:1 | 存在 |
| `p13_union_fight_battle_audit_offers` | `P13UnionFightBattleAuditOffer` | 3 | `match_id` | `源码未识别，需核业务` | hyjs-server/db/sql/117_p13_union_fight_battle_audit_offers.sql:1 | 存在 |
| `p13_union_fight_battle_audit_sources` | `P13UnionFightBattleAuditSource` | 2 | `day` | `源码未识别，需核业务` | hyjs-server/db/sql/117_p13_union_fight_battle_audit_sources.sql:1 | 存在 |
| `p13_union_fight_byes` | `P13UnionFightBye` | 8 | `id` | `guild_id` | hyjs-server/db/sql/p13_union_fight_byes.sql:1 | 存在 |
| `p13_union_fight_camp_resources` | `P13UnionFightCampResource` | 3 | `guild_id;match_id` | `guild_id` | hyjs-server/db/sql/96_p13_union_fight_camp_resources.sql:1 | 存在 |
| `p13_union_fight_car_attack_offers` | `P13UnionFightCarAttackOffer` | 4 | `factory_guid;match_id` | `源码未识别，需核业务` | hyjs-server/db/sql/110_p13_union_fight_car_attack_offers.sql:1 | 存在 |
| `p13_union_fight_car_attack_sources` | `P13UnionFightCarAttackSource` | 2 | `day;factory_id` | `源码未识别，需核业务` | hyjs-server/db/sql/110_p13_union_fight_car_attack_sources.sql:1 | 存在 |
| `p13_union_fight_car_attack_states` | `P13UnionFightCarAttackState` | 3 | `factory_guid;match_id` | `源码未识别，需核业务` | hyjs-server/db/sql/110_p13_union_fight_car_attack_states.sql:1 | 存在 |
| `p13_union_fight_car_generations` | `P13UnionFightCarGeneration` | 7 | `car_guid;match_id` | `源码未识别，需核业务` | hyjs-server/db/sql/115_p13_union_fight_car_generations.sql:1 | 存在 |
| `p13_union_fight_car_impacts` | `P13UnionFightCarImpact` | 12 | `id` | `player_id` | hyjs-server/db/sql/p13UnionFightCarImpact.sql:1 | 存在 |
| `p13_union_fight_car_item_offers` | `P13UnionFightCarItemOffer` | 6 | `item_id;match_id` | `源码未识别，需核业务` | hyjs-server/db/sql/108_p13_union_fight_car_item_offers.sql:1 | 存在 |
| `p13_union_fight_car_item_sources` | `P13UnionFightCarItemSource` | 5 | `day;item_id` | `源码未识别，需核业务` | hyjs-server/db/sql/108_p13_union_fight_car_item_sources.sql:1 | 存在 |
| `p13_union_fight_car_motion_offers` | `P13UnionFightCarMotionOffer` | 4 | `factory_guid;match_id` | `源码未识别，需核业务` | hyjs-server/db/sql/109_p13_union_fight_car_motion_offers.sql:1 | 存在 |
| `p13_union_fight_car_motion_sources` | `P13UnionFightCarMotionSource` | 2 | `day;factory_id` | `源码未识别，需核业务` | hyjs-server/db/sql/109_p13_union_fight_car_motion_sources.sql:1 | 存在 |
| `p13_union_fight_car_motion_states` | `P13UnionFightCarMotionState` | 3 | `factory_guid;match_id` | `源码未识别，需核业务` | hyjs-server/db/sql/109_p13_union_fight_car_motion_states.sql:1 | 存在 |
| `p13_union_fight_chat_policies` | `P13UnionFightChatPolicy` | 1 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/103_p13_union_fight_chat_policies.sql:1 | 存在 |
| `p13_union_fight_chats` | `P13UnionFightChat` | 6 | `id` | `guild_id;sender_id` | hyjs-server/db/sql/103_p13_union_fight_chats.sql:1 | 存在 |
| `p13_union_fight_entries` | `P13UnionFightEntry` | 7 | `day;guild_id` | `guild_id` | hyjs-server/db/sql/95_p13_union_fight_entries.sql:2 | 存在 |
| `p13_union_fight_factory_cycle_offers` | `P13UnionFightFactoryCycleOffer` | 4 | `factory_guid;match_id` | `源码未识别，需核业务` | hyjs-server/db/sql/115_p13_union_fight_factory_cycle_offers.sql:1 | 存在 |
| `p13_union_fight_factory_cycle_sources` | `P13UnionFightFactoryCycleSource` | 3 | `day;factory_id` | `源码未识别，需核业务` | hyjs-server/db/sql/115_p13_union_fight_factory_cycle_sources.sql:1 | 存在 |
| `p13_union_fight_factory_cycle_states` | `P13UnionFightFactoryCycleState` | 3 | `factory_guid;match_id` | `源码未识别，需核业务` | hyjs-server/db/sql/115_p13_union_fight_factory_cycle_states.sql:1 | 存在 |
| `p13_union_fight_factory_sources` | `P13UnionFightFactorySource` | 2 | `day;factory_id` | `源码未识别，需核业务` | hyjs-server/db/sql/99_p13_union_fight_factory_sources.sql:1 | 存在 |
| `p13_union_fight_factory_states` | `P13UnionFightFactoryState` | 8 | `guid;match_id` | `源码未识别，需核业务` | hyjs-server/db/sql/99_p13_union_fight_factory_states.sql:1 | 存在 |
| `p13_union_fight_field_players` | `P13UnionFightFieldPlayer` | 9 | `match_id;player_id` | `guild_id;player_id` | hyjs-server/db/sql/95_p13_union_fight_field_players.sql:1 | 存在 |
| `p13_union_fight_field_policies` | `P13UnionFightFieldPolicy` | 4 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/95_p13_union_fight_field_policies.sql:1 | 存在 |
| `p13_union_fight_fields` | `P13UnionFightField` | 4 | `match_id` | `源码未识别，需核业务` | hyjs-server/db/sql/95_p13_union_fight_fields.sql:1 | 存在 |
| `p13_union_fight_fighters` | `P13UnionFightFighter` | 6 | `day;guild_id;player_id` | `guild_id;player_id` | hyjs-server/db/sql/95_p13_union_fight_fighters.sql:2 | 存在 |
| `p13_union_fight_focus` | `P13UnionFightFocus` | 5 | `guild_id;match_id` | `guild_id;leader_id` | hyjs-server/db/sql/95_p13_union_fight_focus.sql:1 | 存在 |
| `p13_union_fight_gate_impacts` | `P13UnionFightGateImpact` | 10 | `id` | `player_id` | hyjs-server/db/sql/p13UnionFightGateImpact.sql:1 | 存在 |
| `p13_union_fight_gate_item_offers` | `P13UnionFightGateItemOffer` | 6 | `item_id;match_id` | `源码未识别，需核业务` | hyjs-server/db/sql/107_p13_union_fight_gate_item_offers.sql:1 | 存在 |
| `p13_union_fight_gate_item_sources` | `P13UnionFightGateItemSource` | 5 | `day;item_id` | `源码未识别，需核业务` | hyjs-server/db/sql/107_p13_union_fight_gate_item_sources.sql:1 | 存在 |
| `p13_union_fight_gate_states` | `P13UnionFightGateState` | 6 | `guid;match_id` | `源码未识别，需核业务` | hyjs-server/db/sql/107_p13_union_fight_gate_states.sql:1 | 存在 |
| `p13_union_fight_groups` | `P13UnionFightGroup` | 4 | `guild_id;week_start` | `guild_id` | hyjs-server/db/sql/95_p13_union_fight_groups.sql:2 | 存在 |
| `p13_union_fight_guard_audit_attempts` | `P13UnionFightGuardAuditAttempt` | 9 | `guid;match_id` | `player_id` | hyjs-server/db/sql/118_p13_union_fight_guard_audit_attempts.sql:1 | 存在 |
| `p13_union_fight_guard_audit_offers` | `P13UnionFightGuardAuditOffer` | 9 | `guard_guid;match_id` | `源码未识别，需核业务` | hyjs-server/db/sql/118_p13_union_fight_guard_audit_offers.sql:1 | 存在 |
| `p13_union_fight_guard_audit_sources` | `P13UnionFightGuardAuditSource` | 5 | `day;guard_id` | `源码未识别，需核业务` | hyjs-server/db/sql/118_p13_union_fight_guard_audit_sources.sql:1 | 存在 |
| `p13_union_fight_guard_battle_attempts` | `P13UnionFightGuardBattleAttempt` | 21 | `id` | `player_id` | hyjs-server/db/sql/p13_union_fight_guard_battle_attempts.sql:1 | 存在 |
| `p13_union_fight_guard_battle_policies` | `P13UnionFightGuardBattlePolicy` | 2 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/p13_union_fight_guard_battle_policies.sql:1 | 存在 |
| `p13_union_fight_guard_states` | `P13UnionFightGuardState` | 13 | `guard_guid;match_id` | `源码未识别，需核业务` | hyjs-server/db/sql/p13_union_fight_guard_states.sql:1 | 存在 |
| `p13_union_fight_guild_mission_facts` | `P13UnionFightGuildMissionFact` | 4 | `guild_id;match_id` | `guild_id;player_id` | hyjs-server/db/sql/142_p13_union_fight_guild_mission_facts.sql:1 | 存在 |
| `p13_union_fight_heal_aura_effects` | `P13UnionFightHealAuraEffect` | 3 | `guid;match_id` | `源码未识别，需核业务` | hyjs-server/db/sql/112_p13_union_fight_heal_aura_effects.sql:1 | 存在 |
| `p13_union_fight_heal_aura_offers` | `P13UnionFightHealAuraOffer` | 5 | `item_id;match_id` | `源码未识别，需核业务` | hyjs-server/db/sql/112_p13_union_fight_heal_aura_offers.sql:1 | 存在 |
| `p13_union_fight_heal_aura_pendings` | `P13UnionFightHealAuraPending` | 19 | `id` | `guild_id;player_id` | hyjs-server/db/sql/112_p13_union_fight_heal_aura_pendings.sql:1 | 存在 |
| `p13_union_fight_heal_aura_sources` | `P13UnionFightHealAuraSource` | 4 | `day;item_id` | `源码未识别，需核业务` | hyjs-server/db/sql/112_p13_union_fight_heal_aura_sources.sql:1 | 存在 |
| `p13_union_fight_item_offers` | `P13UnionFightItemOffer` | 4 | `item_id;match_id` | `源码未识别，需核业务` | hyjs-server/db/sql/106_p13_union_fight_item_offers.sql:1 | 存在 |
| `p13_union_fight_item_policies` | `P13UnionFightItemPolicy` | 1 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/106_p13_union_fight_item_policies.sql:1 | 存在 |
| `p13_union_fight_item_sources` | `P13UnionFightItemSource` | 3 | `day;item_id` | `源码未识别，需核业务` | hyjs-server/db/sql/106_p13_union_fight_item_sources.sql:1 | 存在 |
| `p13_union_fight_item_states` | `P13UnionFightItemState` | 2 | `match_id;player_id` | `player_id` | hyjs-server/db/sql/106_p13_union_fight_item_states.sql:1 | 存在 |
| `p13_union_fight_item_target_cooldowns` | `P13UnionFightItemTargetCooldown` | 5 | `effect_family;match_id;target_guid` | `源码未识别，需核业务` | hyjs-server/db/sql/p13UnionFightItemTargetCooldown.sql:1 | 存在 |
| `p13_union_fight_item_team_uses` | `P13UnionFightItemTeamUse` | 5 | `camp;item_id;match_id` | `源码未识别，需核业务` | hyjs-server/db/sql/p13UnionFightItemTeamUse.sql:1 | 存在 |
| `p13_union_fight_loadouts` | `P13UnionFightLoadout` | 2 | `match_id;player_id` | `player_id` | hyjs-server/db/sql/95_p13_union_fight_loadouts.sql:1 | 存在 |
| `p13_union_fight_matches` | `P13UnionFightMatch` | 13 | `id` | `left_guild_id;right_guild_id` | hyjs-server/db/sql/95_p13_union_fight_matches.sql:2 | 存在 |
| `p13_union_fight_monthly_cutoffs` | `P13UnionFightMonthlyCutoff` | 6 | `month` | `源码未识别，需核业务` | hyjs-server/db/sql/143_p13_union_fight_monthly_cutoffs.sql:1 | 存在 |
| `p13_union_fight_monthly_offers` | `P13UnionFightMonthlyOffer` | 3 | `month` | `源码未识别，需核业务` | hyjs-server/db/sql/143_p13_union_fight_monthly_offers.sql:1 | 存在 |
| `p13_union_fight_monthly_policies` | `P13UnionFightMonthlyPolicy` | 4 | `month` | `源码未识别，需核业务` | hyjs-server/db/sql/143_p13_union_fight_monthly_policies.sql:1 | 存在 |
| `p13_union_fight_monthly_results` | `P13UnionFightMonthlyResult` | 8 | `month` | `源码未识别，需核业务` | hyjs-server/db/sql/143_p13_union_fight_monthly_results.sql:1 | 存在 |
| `p13_union_fight_monthly_titles` | `P13UnionFightMonthlyTitle` | 6 | `month;player_id;title_id` | `guild_id;player_id` | hyjs-server/db/sql/143_p13_union_fight_monthly_titles.sql:1 | 存在 |
| `p13_union_fight_player_battle_attempts` | `P13UnionFightPlayerBattleAttempt` | 18 | `id` | `dead_player_id;player_id;reward_player_id;target_id` | hyjs-server/db/sql/172_p13_union_fight_player_battle_attempts.sql:1 | 存在 |
| `p13_union_fight_player_battle_offers` | `P13UnionFightPlayerBattleOffer` | 3 | `match_id` | `源码未识别，需核业务` | hyjs-server/db/sql/172_p13_union_fight_player_battle_offers.sql:1 | 存在 |
| `p13_union_fight_player_battle_policies` | `P13UnionFightPlayerBattlePolicy` | 3 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/172_p13_union_fight_player_battle_policies.sql:1 | 存在 |
| `p13_union_fight_player_recoveries` | `P13UnionFightPlayerRecovery` | 12 | `id` | `player_id` | hyjs-server/db/sql/p13UnionFightPlayerRecovery.sql:1 | 存在 |
| `p13_union_fight_policies` | `P13UnionFightPolicy` | 1 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/95_p13_union_fight_policies.sql:2 | 存在 |
| `p13_union_fight_resource_nodes` | `P13UnionFightResourceNode` | 6 | `guid;match_id` | `源码未识别，需核业务` | hyjs-server/db/sql/96_p13_union_fight_resource_nodes.sql:1 | 存在 |
| `p13_union_fight_resource_sources` | `P13UnionFightResourceSource` | 4 | `day;source_key` | `源码未识别，需核业务` | hyjs-server/db/sql/96_p13_union_fight_resource_sources.sql:1 | 存在 |
| `p13_union_fight_resource_tasks` | `P13UnionFightResourceTask` | 5 | `id` | `player_id` | hyjs-server/db/sql/96_p13_union_fight_resource_tasks.sql:1 | 存在 |
| `p13_union_fight_revive_point_offers` | `P13UnionFightRevivePointOffer` | 3 | `match_id;point_guid` | `源码未识别，需核业务` | hyjs-server/db/sql/119_p13_union_fight_revive_point_offers.sql:1 | 存在 |
| `p13_union_fight_revive_point_sources` | `P13UnionFightRevivePointSource` | 2 | `day;point_id` | `源码未识别，需核业务` | hyjs-server/db/sql/119_p13_union_fight_revive_point_sources.sql:1 | 存在 |
| `p13_union_fight_revive_point_states` | `P13UnionFightRevivePointState` | 5 | `match_id;point_guid` | `源码未识别，需核业务` | hyjs-server/db/sql/119_p13_union_fight_revive_point_states.sql:1 | 存在 |
| `p13_union_fight_revive_receipts` | `P13UnionFightReviveReceipt` | 18 | `dead_ordinal;match_id;player_id` | `player_id` | hyjs-server/db/sql/p13UnionFightReviveReceipt.sql:1 | 存在 |
| `p13_union_fight_schedules` | `P13UnionFightSchedule` | 4 | `day` | `源码未识别，需核业务` | hyjs-server/db/sql/95_p13_union_fight_schedules.sql:2 | 存在 |
| `p13_union_fight_settings` | `P13UnionFightSetting` | 1 | `guild_id` | `guild_id` | 未找到建表SQL | 未见 |
| `p13_union_fight_settlement_intents` | `P13UnionFightSettlementIntent` | 2 | `match_id;player_id;stamp;target_id` | `player_id;target_id` | hyjs-server/db/sql/139_p13_union_fight_settlement_intents.sql:1 | 存在 |
| `p13_union_fight_settlement_offers` | `P13UnionFightSettlementOffer` | 6 | `match_id` | `源码未识别，需核业务` | hyjs-server/db/sql/139_p13_union_fight_settlement_offers.sql:1 | 存在 |
| `p13_union_fight_settlement_policies` | `P13UnionFightSettlementPolicy` | 10 | `group` | `源码未识别，需核业务` | hyjs-server/db/sql/139_p13_union_fight_settlement_policies.sql:1 | 存在 |
| `p13_union_fight_settlement_prestiges` | `P13UnionFightSettlementPrestige` | 2 | `guild_id;week_start` | `guild_id` | hyjs-server/db/sql/139_p13_union_fight_settlement_prestiges.sql:1 | 存在 |
| `p13_union_fight_settlement_results` | `P13UnionFightSettlementResult` | 12 | `match_id` | `loser_guild_id;winner_guild_id` | hyjs-server/db/sql/139_p13_union_fight_settlement_results.sql:1 | 存在 |
| `p13_union_fight_shop_offers` | `P13UnionFightShopOffer` | 6 | `match_id;shop_item_id` | `源码未识别，需核业务` | hyjs-server/db/sql/98_p13_union_fight_shop_offers.sql:1 | 存在 |
| `p13_union_fight_shop_purchases` | `P13UnionFightShopPurchase` | 13 | `id` | `player_id` | hyjs-server/db/sql/p13UnionFightShopPurchase.sql:1 | 存在 |
| `p13_union_fight_shop_sources` | `P13UnionFightShopSource` | 5 | `day;shop_item_id` | `源码未识别，需核业务` | hyjs-server/db/sql/98_p13_union_fight_shop_sources.sql:1 | 存在 |
| `p13_union_fight_shop_stocks` | `P13UnionFightShopStock` | 2 | `match_id;player_id;shop_item_id` | `player_id` | hyjs-server/db/sql/98_p13_union_fight_shop_stocks.sql:1 | 存在 |
| `p13_union_fight_shop_visits` | `P13UnionFightShopVisit` | 2 | `match_id;player_id` | `player_id` | hyjs-server/db/sql/98_p13_union_fight_shop_visits.sql:1 | 存在 |
| `p13_union_fight_speed_item_effects` | `P13UnionFightSpeedItemEffect` | 3 | `guid;match_id` | `target_id` | hyjs-server/db/sql/111_p13_union_fight_speed_item_effects.sql:1 | 存在 |
| `p13_union_fight_speed_item_offers` | `P13UnionFightSpeedItemOffer` | 4 | `item_id;match_id` | `源码未识别，需核业务` | hyjs-server/db/sql/111_p13_union_fight_speed_item_offers.sql:1 | 存在 |
| `p13_union_fight_speed_item_sources` | `P13UnionFightSpeedItemSource` | 3 | `day;item_id` | `源码未识别，需核业务` | hyjs-server/db/sql/111_p13_union_fight_speed_item_sources.sql:1 | 存在 |
| `p13_union_fight_tower_hits` | `P13UnionFightTowerHit` | 27 | `match_id;sequence;tower_guid` | `dead_player_id` | hyjs-server/db/sql/p13UnionFightTowerHit.sql:1 | 存在 |
| `p13_union_fight_tower_sources` | `P13UnionFightTowerSource` | 4 | `day;tower_id` | `源码未识别，需核业务` | hyjs-server/db/sql/p13UnionFightTowerSource.sql:1 | 存在 |
| `p13_union_fight_tower_states` | `P13UnionFightTowerState` | 17 | `match_id;tower_guid` | `源码未识别，需核业务` | hyjs-server/db/sql/p13UnionFightTowerState.sql:1 | 存在 |
| `p13_union_fight_transfer_tasks` | `P13UnionFightTransferTask` | 8 | `id` | `player_id` | hyjs-server/db/sql/97_p13_union_fight_transfer_tasks.sql:1 | 存在 |
| `p13_union_fight_transport_scopes` | `P13UnionFightTransportScope` | 2 | `match_id` | `源码未识别，需核业务` | hyjs-server/db/sql/140_p13_union_fight_transport_scopes.sql:1 | 存在 |
| `p13_union_fight_transport_stats` | `P13UnionFightTransportStat` | 2 | `match_id;player_id` | `guild_id;player_id` | hyjs-server/db/sql/140_p13_union_fight_transport_stats.sql:1 | 存在 |
| `p13_union_fight_war_drum_effects` | `P13UnionFightWarDrumEffect` | 4 | `guid;match_id` | `guild_id` | hyjs-server/db/sql/113_p13_union_fight_war_drum_effects.sql:1 | 存在 |
| `p13_union_fight_war_drum_offers` | `P13UnionFightWarDrumOffer` | 5 | `item_id;match_id` | `源码未识别，需核业务` | hyjs-server/db/sql/113_p13_union_fight_war_drum_offers.sql:1 | 存在 |
| `p13_union_fight_war_drum_sources` | `P13UnionFightWarDrumSource` | 4 | `day;item_id` | `源码未识别，需核业务` | hyjs-server/db/sql/113_p13_union_fight_war_drum_sources.sql:1 | 存在 |
| `p13_union_fight_weekly_offers` | `P13UnionFightWeeklyOffer` | 6 | `group;week_start` | `源码未识别，需核业务` | hyjs-server/db/sql/141_p13_union_fight_weekly_offers.sql:1 | 存在 |
| `p13_union_fight_weekly_policies` | `P13UnionFightWeeklyPolicy` | 5 | `group;week_start` | `源码未识别，需核业务` | hyjs-server/db/sql/141_p13_union_fight_weekly_policies.sql:1 | 存在 |
| `p13_union_fight_weekly_results` | `P13UnionFightWeeklyResult` | 8 | `group;week_start` | `源码未识别，需核业务` | hyjs-server/db/sql/141_p13_union_fight_weekly_results.sql:1 | 存在 |
| `p14_audition_boundaries` | `P14AuditionBoundary` | 5 | `season_index;server_id` | `server_id` | hyjs-server/db/sql/100_p14_audition_boundaries.sql:1 | 存在 |
| `p14_audition_match_policies` | `P14AuditionMatchPolicy` | 2 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/101_p14_audition_match_policies.sql:1 | 存在 |
| `p14_audition_matches` | `P14AuditionMatch` | 7 | `id` | `attacker_id;defender_id;server_id;winner_id` | hyjs-server/db/sql/101_p14_audition_matches.sql:1 | 存在 |
| `p14_audition_qualifications` | `P14AuditionQualification` | 5 | `season_index;server_id` | `server_id` | hyjs-server/db/sql/101_p14_audition_qualifications.sql:1 | 存在 |
| `p14_audition_rosters` | `P14AuditionRoster` | 7 | `player_id;season_index;server_id` | `player_id;server_id` | hyjs-server/db/sql/100_p14_audition_rosters.sql:1 | 存在 |
| `p14_audition_rounds` | `P14AuditionRound` | 4 | `round_index;season_index;server_id` | `server_id` | hyjs-server/db/sql/101_p14_audition_rounds.sql:1 | 存在 |
| `p14_deliveries` | `P14Delivery` | 2 | `id` | `destination_server_id` | hyjs-server/db/sql/96_p14_deliveries.sql:2 | 存在 |
| `p14_final_continuation_deliveries` | `P14FinalContinuationDelivery` | 5 | `id` | `destination_server_id` | hyjs-server/db/sql/169_p14_final_continuation_deliveries.sql:1 | 存在 |
| `p14_final_continuation_inputs` | `P14FinalContinuationInput` | 8 | `game_index;series_id` | `源码未识别，需核业务` | hyjs-server/db/sql/169_p14_final_continuation_inputs.sql:1 | 存在 |
| `p14_final_continuation_offers` | `P14FinalContinuationOffer` | 5 | `series_id` | `源码未识别，需核业务` | hyjs-server/db/sql/169_p14_final_continuation_offers.sql:1 | 存在 |
| `p14_final_continuation_policies` | `P14FinalContinuationPolicy` | 6 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/169_p14_final_continuation_policies.sql:1 | 存在 |
| `p14_final_continuation_receipts` | `P14FinalContinuationReceipt` | 11 | `game_index;series_id` | `source_server_id;winner_player_id;winner_server_id` | hyjs-server/db/sql/169_p14_final_continuation_receipts.sql:1 | 存在 |
| `p14_final_continuation_results` | `P14FinalContinuationResult` | 10 | `game_index;series_id` | `winner_player_id;winner_server_id` | hyjs-server/db/sql/169_p14_final_continuation_results.sql:1 | 存在 |
| `p14_final_draw_policies` | `P14FinalDrawPolicy` | 2 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/155_p14_final_draw_policies.sql:2 | 存在 |
| `p14_final_draws` | `P14FinalDraw` | 2 | `season_index` | `源码未识别，需核业务` | hyjs-server/db/sql/155_p14_final_draws.sql:2 | 存在 |
| `p14_final_formation_offers` | `P14FinalFormationOffer` | 4 | `round_stage;season_index` | `源码未识别，需核业务` | hyjs-server/db/sql/154_p14_final_formation_offers.sql:2 | 存在 |
| `p14_final_formation_policies` | `P14FinalFormationPolicy` | 2 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/154_p14_final_formation_policies.sql:2 | 存在 |
| `p14_final_formations` | `P14FinalFormation` | 10 | `player_id;round_stage;season_index;server_id` | `player_id;server_id` | hyjs-server/db/sql/154_p14_final_formations.sql:2 | 存在 |
| `p14_final_game_policies` | `P14FinalGamePolicy` | 2 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/157_p14_final_game_policies.sql:2 | 存在 |
| `p14_final_games` | `P14FinalGame` | 6 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/157_p14_final_games.sql:1 | 存在 |
| `p14_final_qualification_policies` | `P14FinalQualificationPolicy` | 2 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/152_p14_final_qualification_policies.sql:2 | 存在 |
| `p14_final_qualifications` | `P14FinalQualification` | 2 | `season_index` | `源码未识别，需核业务` | hyjs-server/db/sql/152_p14_final_qualifications.sql:2 | 存在 |
| `p14_final_result_deliveries` | `P14FinalResultDelivery` | 5 | `id` | `destination_server_id` | hyjs-server/db/sql/166_p14_final_result_deliveries.sql:1 | 存在 |
| `p14_final_result_offers` | `P14FinalResultOffer` | 5 | `series_id` | `源码未识别，需核业务` | hyjs-server/db/sql/166_p14_final_result_offers.sql:1 | 存在 |
| `p14_final_result_policies` | `P14FinalResultPolicy` | 3 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/166_p14_final_result_policies.sql:1 | 存在 |
| `p14_final_result_receipts` | `P14FinalResultReceipt` | 11 | `game_index;series_id` | `source_server_id;winner_player_id;winner_server_id` | hyjs-server/db/sql/166_p14_final_result_receipts.sql:1 | 存在 |
| `p14_final_seal_deliveries` | `P14FinalSealDelivery` | 3 | `id` | `destination_server_id` | hyjs-server/db/sql/157_p14_final_seal_deliveries.sql:1 | 存在 |
| `p14_final_seals` | `P14FinalSeal` | 2 | `round_stage;season_index;server_id` | `server_id` | hyjs-server/db/sql/157_p14_final_seals.sql:1 | 存在 |
| `p14_final_series` | `P14FinalSeries` | 6 | `id` | `winner_player_id;winner_server_id` | hyjs-server/db/sql/161_p14_final_series.sql:1 | 存在 |
| `p14_final_series_policies` | `P14FinalSeriesPolicy` | 2 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/161_p14_final_series_policies.sql:1 | 存在 |
| `p14_final_series_slots` | `P14FinalSeriesSlot` | 7 | `game_index;series_id` | `源码未识别，需核业务` | hyjs-server/db/sql/161_p14_final_series_slots.sql:1 | 存在 |
| `p14_final_settlement_offers` | `P14FinalSettlementOffer` | 5 | `series_id` | `源码未识别，需核业务` | hyjs-server/db/sql/165_p14_final_settlement_offers.sql:1 | 存在 |
| `p14_final_settlement_policies` | `P14FinalSettlementPolicy` | 6 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/165_p14_final_settlement_policies.sql:1 | 存在 |
| `p14_final_settlements` | `P14FinalSettlement` | 10 | `game_index;series_id` | `winner_player_id;winner_server_id` | hyjs-server/db/sql/165_p14_final_settlements.sql:1 | 存在 |
| `p14_foreign_qualifications` | `P14ForeignQualification` | 5 | `season_index;server_id` | `server_id` | hyjs-server/db/sql/122_p14_foreign_qualifications.sql:1 | 存在 |
| `p14_guess_offers` | `P14GuessOffer` | 7 | `guess_type;season_index` | `源码未识别，需核业务` | hyjs-server/db/sql/158_p14_guess_offers.sql:1 | 存在 |
| `p14_guess_policies` | `P14GuessPolicy` | 2 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/158_p14_guess_policies.sql:1 | 存在 |
| `p14_guesses` | `P14Guess` | 2 | `guess_type;owner_player_id;owner_server_id;season_index` | `owner_player_id;owner_server_id;target_player_id;target_server_id` | hyjs-server/db/sql/158_p14_guesses.sql:1 | 存在 |
| `p14_import_receipts` | `P14ImportReceipt` | 2 | `id` | `source_server_id` | hyjs-server/db/sql/96_p14_import_receipts.sql:2 | 存在 |
| `p14_members` | `P14Member` | 6 | `player_id;season_index;server_id` | `player_id;server_id` | hyjs-server/db/sql/96_p14_members.sql:2 | 存在 |
| `p14_pair_guess_awards` | `P14PairGuessAward` | 9 | `owner_player_id;owner_server_id;pair_hash;season_index` | `owner_player_id;owner_server_id;winner_player_id;winner_server_id` | hyjs-server/db/sql/170_p14_pair_guess_awards.sql:1 | 存在 |
| `p14_pair_guess_offers` | `P14PairGuessOffer` | 8 | `pair_hash;season_index` | `源码未识别，需核业务` | hyjs-server/db/sql/160_p14_pair_guess_offers.sql:1 | 存在 |
| `p14_pair_guess_policies` | `P14PairGuessPolicy` | 2 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/160_p14_pair_guess_policies.sql:1 | 存在 |
| `p14_pair_guesses` | `P14PairGuess` | 3 | `owner_player_id;owner_server_id;pair_hash;season_index` | `owner_player_id;owner_server_id;target_player_id;target_server_id` | hyjs-server/db/sql/160_p14_pair_guesses.sql:1 | 存在 |
| `p14_per_game_deliveries` | `P14PerGameDelivery` | 3 | `id` | `destination_server_id` | hyjs-server/db/sql/162_p14_per_game_deliveries.sql:1 | 存在 |
| `p14_per_game_formations` | `P14PerGameFormation` | 9 | `game_index;player_id;series_id;server_id` | `player_id;server_id` | hyjs-server/db/sql/162_p14_per_game_formations.sql:1 | 存在 |
| `p14_per_game_offers` | `P14PerGameOffer` | 4 | `game_index;series_id` | `源码未识别，需核业务` | hyjs-server/db/sql/162_p14_per_game_offers.sql:1 | 存在 |
| `p14_per_game_policies` | `P14PerGamePolicy` | 2 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/162_p14_per_game_policies.sql:1 | 存在 |
| `p14_per_game_seals` | `P14PerGameSeal` | 3 | `game_index;series_id;server_id` | `server_id` | hyjs-server/db/sql/162_p14_per_game_seals.sql:1 | 存在 |
| `p14_promotion_fight_offers` | `P14PromotionFightOffer` | 5 | `season_index;server_id` | `server_id` | hyjs-server/db/sql/148_p14_promotion_fight_offers.sql:2 | 存在 |
| `p14_promotion_fight_policies` | `P14PromotionFightPolicy` | 4 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/148_p14_promotion_fight_policies.sql:2 | 存在 |
| `p14_promotion_fights` | `P14PromotionFight` | 13 | `id` | `opponent_id;opponent_server_id;player_id;server_id;winner_id` | hyjs-server/db/sql/148_p14_promotion_fights.sql:2 | 存在 |
| `p14_promotion_formations` | `P14PromotionFormation` | 9 | `player_id;season_index;server_id` | `player_id;server_id` | hyjs-server/db/sql/144_p14_promotion_formations.sql:1 | 存在 |
| `p14_promotion_grants` | `P14PromotionGrant` | 5 | `grant_at;player_id;season_index;server_id` | `player_id;server_id` | hyjs-server/db/sql/123_p14_promotion_grants.sql:1 | 存在 |
| `p14_promotion_policies` | `P14PromotionPolicy` | 3 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/123_p14_promotion_policies.sql:1 | 存在 |
| `p14_promotion_rank_deliveries` | `P14PromotionRankDelivery` | 3 | `id` | `destination_server_id;source_server_id` | hyjs-server/db/sql/151_p14_promotion_rank_deliveries.sql:2 | 存在 |
| `p14_promotion_rank_offers` | `P14PromotionRankOffer` | 7 | `season_index;server_id` | `server_id` | hyjs-server/db/sql/151_p14_promotion_rank_offers.sql:2 | 存在 |
| `p14_promotion_rank_policies` | `P14PromotionRankPolicy` | 2 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/151_p14_promotion_rank_policies.sql:2 | 存在 |
| `p14_promotion_rank_versions` | `P14PromotionRankVersion` | 3 | `revision;season_index;server_id` | `server_id` | hyjs-server/db/sql/151_p14_promotion_rank_versions.sql:2 | 存在 |
| `p14_promotion_states` | `P14PromotionState` | 5 | `player_id;season_index;server_id` | `player_id;server_id` | hyjs-server/db/sql/123_p14_promotion_states.sql:1 | 存在 |
| `p14_qualification_deliveries` | `P14QualificationDelivery` | 3 | `id` | `destination_server_id` | hyjs-server/db/sql/122_p14_qualification_deliveries.sql:1 | 存在 |
| `p14_qualification_federation_policies` | `P14QualificationFederationPolicy` | 1 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/122_p14_qualification_federation_policies.sql:1 | 存在 |
| `p14_receipts` | `P14Receipt` | 3 | `operation;player_id;stamp` | `player_id` | hyjs-server/db/sql/96_p14_receipts.sql:2 | 存在 |
| `p14_season_policies` | `P14SeasonPolicy` | 1 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/96_p14_season_policies.sql:2 | 存在 |
| `p14_time_slots` | `P14TimeSlot` | 2 | `season_index;start_at` | `源码未识别，需核业务` | hyjs-server/db/sql/96_p14_time_slots.sql:1 | 存在 |
| `p14_trusted_peers` | `P14TrustedPeer` | 2 | `server_id` | `server_id` | hyjs-server/db/sql/96_p14_trusted_peers.sql:2 | 存在 |
| `p15_seasonal_bundles` | `P15SeasonalBundle` | 6 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/102_p15_seasonal_bundles.sql:2 | 存在 |
| `p1_event_facts` | `EventFact` | 3 | `id` | `player_id` | hyjs-server/db/sql/80_p1_event_facts.sql:2 | 存在 |
| `p1_gather_catalog_guards` | `P1GatherCatalogGuard` | 1 | `kind` | `源码未识别，需核业务` | hyjs-server/db/sql/p1_gather_catalog_guards.sql:2 | 存在 |
| `p1_login_directory_members` | `P1LoginDirectoryMember` | 1 | `index_id` | `源码未识别，需核业务` | hyjs-server/db/sql/109_p1_login_directory_members.sql:2 | 存在 |
| `p1_mail_messages` | `MailMessage` | 5 | `id` | `player_id;target_id` | hyjs-server/db/sql/80_p1_mail_messages.sql:2 | 存在 |
| `p1_period_instances` | `PeriodInstance` | 10 | `id` | `owner_player_id;realm_id` | hyjs-server/db/sql/80_p1_period_instances.sql:2 | 存在 |
| `p1_period_progress` | `PeriodProgress` | 3 | `instance_id;player_id` | `player_id` | hyjs-server/db/sql/80_p1_period_progress.sql:2 | 存在 |
| `p1_period_settlements` | `PeriodSettlement` | 2 | `instance_id` | `源码未识别，需核业务` | hyjs-server/db/sql/80_p1_period_settlements.sql:2 | 存在 |
| `p1_reward_deliveries` | `RewardDelivery` | 5 | `id` | `player_id` | hyjs-server/db/sql/80_p1_reward_deliveries.sql:2 | 存在 |
| `p2_king_rules` | `KingRecruitRules` | 2 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/81_p2_king_rules.sql:2 | 存在 |
| `p2_king_states` | `KingRecruitState` | 1 | `player_id` | `player_id` | hyjs-server/db/sql/81_p2_king_states.sql:2 | 存在 |
| `p2_red_box_rewards` | `RedP2BoxReward` | 3 | `box_id;resource_id` | `源码未识别，需核业务` | hyjs-server/db/sql/81_p2_red_box_rewards.sql:2 | 存在 |
| `p2_red_exchange_states` | `RedP2ExchangeState` | 1 | `day;player_id;product_id` | `player_id` | hyjs-server/db/sql/81_p2_red_exchange_states.sql:2 | 存在 |
| `p2_red_exchanges` | `RedP2Exchange` | 3 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/81_p2_red_exchanges.sql:2 | 存在 |
| `p2_red_receipts` | `RedP2Receipt` | 3 | `operation;player_id;timestamp` | `player_id` | hyjs-server/db/sql/81_p2_red_receipts.sql:2 | 存在 |
| `p3_membership_claims` | `MembershipClaim` | 3 | `kind;period;player_id` | `player_id` | hyjs-server/db/sql/82_p3_membership_claims.sql:2 | 存在 |
| `p3_payment_activity_rules` | `PaymentActivityRule` | 5 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/82_p3_payment_activity_rules.sql:2 | 存在 |
| `p3_payment_activity_states` | `PaymentActivityState` | 2 | `activity_id;player_id` | `player_id` | hyjs-server/db/sql/82_p3_payment_activity_states.sql:2 | 存在 |
| `p3_payment_confirmations` | `PaymentConfirmation` | 5 | `event_id` | `源码未识别，需核业务` | hyjs-server/db/sql/82_p3_payment_confirmations.sql:2 | 存在 |
| `p3_payment_gift_codes` | `PaymentGiftCode` | 3 | `hash` | `源码未识别，需核业务` | hyjs-server/db/sql/82_p3_payment_gift_codes.sql:2 | 存在 |
| `p3_payment_luxury_offers` | `PaymentLuxuryOffer` | 1 | `offer_id;player_id` | `player_id` | hyjs-server/db/sql/82_p3_payment_luxury_offers.sql:2 | 存在 |
| `p3_payment_orders` | `PaymentOrder` | 6 | `id` | `player_id` | hyjs-server/db/sql/82_p3_payment_orders.sql:2 | 存在 |
| `p3_payment_products` | `PaymentProduct` | 9 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/82_p3_payment_products.sql:2 | 存在 |
| `p3_vip_shop_products` | `VIPShopProduct` | 4 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/82_p3_vip_shop_products.sql:2 | 存在 |
| `p4_activity_registry` | `P4ActivityRegistry` | 6 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/83_p4_activity_registry.sql:2 | 存在 |
| `p4_commerce_rules` | `P4CommerceRule` | 5 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/83_p4_commerce_rules.sql:2 | 存在 |
| `p4_commerce_states` | `P4CommerceState` | 2 | `player_id;rule_id` | `player_id` | hyjs-server/db/sql/83_p4_commerce_states.sql:2 | 存在 |
| `p4_commerce_stocks` | `P4CommerceStock` | 2 | `product_id;rule_id` | `源码未识别，需核业务` | hyjs-server/db/sql/83_p4_commerce_stocks.sql:2 | 存在 |
| `p4_copy_benefit_grants` | `P4CopyBenefitGrant` | 4 | `player_id;source_key` | `player_id` | hyjs-server/db/sql/83_p4_copy_benefit_grants.sql:2 | 存在 |
| `p4_copy_benefit_rules` | `P4CopyBenefitRule` | 3 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/83_p4_copy_benefit_rules.sql:2 | 存在 |
| `p4_player_stats` | `P4PlayerStats` | 1 | `player_id` | `player_id` | hyjs-server/db/sql/83_p4_player_stats.sql:2 | 存在 |
| `p4_popularity_rank_claims` | `P4PopularityRankClaim` | 5 | `player_id;product_id;rule_id` | `player_id` | hyjs-server/db/sql/167_p4_popularity_rank_claims.sql:1 | 存在 |
| `p4_popularity_rank_offer_history` | `P4PopularityRankOfferHistory` | 3 | `offer_hash` | `源码未识别，需核业务` | hyjs-server/db/sql/231_p4_popularity_rank_offer_history.sql:1 | 存在 |
| `p4_receipts` | `P4Receipt` | 3 | `domain;player_id;timestamp` | `player_id` | hyjs-server/db/sql/83_p4_receipts.sql:2 | 存在 |
| `p4_recruit_rotations` | `P4RecruitRotation` | 4 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/83_p4_recruit_rotations.sql:2 | 存在 |
| `p4_resource_exchange_directories` | `P4ResourceExchangeDirectory` | 4 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/106_p4_resource_exchange_directories.sql:2 | 存在 |
| `p4_welfare_rules` | `P4WelfareRule` | 5 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/83_p4_welfare_rules.sql:2 | 存在 |
| `p4_welfare_states` | `P4WelfareState` | 3 | `player_id;rule_id` | `player_id` | hyjs-server/db/sql/83_p4_welfare_states.sql:2 | 存在 |
| `p5_achievement_claims` | `P5AchievementClaim` | 2 | `id;player_id` | `player_id` | hyjs-server/db/sql/84_p5_achievement_claims.sql:2 | 存在 |
| `p5_achievement_rules` | `P5Achievement` | 5 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/84_p5_achievement_rules.sql:2 | 存在 |
| `p5_collections` | `P5Collection` | 1 | `hero_id;player_id` | `player_id` | hyjs-server/db/sql/84_p5_collections.sql:2 | 存在 |
| `p5_gold_egg_rules` | `P5GoldEggRule` | 5 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/107_p5_gold_egg_rules.sql:2 | 存在 |
| `p5_mutation_receipts` | `P5MutationReceipt` | 3 | `operation;player_id;timestamp` | `player_id` | hyjs-server/db/sql/84_p5_mutation_receipts.sql:2 | 存在 |
| `p6_battle_records` | `P6BattleRecord` | 14 | `id` | `player_id` | hyjs-server/db/sql/85_p6_battle_records.sql:2 | 存在 |
| `p6_combat_rules` | `P6CombatRule` | 3 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/85_p6_combat_rules.sql:1 | 存在 |
| `p7_awake_explorations` | `P7AwakeExploration` | 6 | `run_id` | `player_id` | hyjs-server/db/sql/188_p7_awake_explorations.sql:1 | 存在 |
| `p7_awake_fights` | `P7AwakeFight` | 15 | `id` | `player_id` | hyjs-server/db/sql/188_p7_awake_fights.sql:1 | 存在 |
| `p7_awake_policies` | `P7AwakePolicy` | 1 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/86_p7_awake_policies.sql:1 | 存在 |
| `p7_awake_runs` | `P7AwakeRun` | 6 | `id` | `player_id` | hyjs-server/db/sql/86_p7_awake_runs.sql:1 | 存在 |
| `p7_awake_stage_rules` | `P7AwakeStageRule` | 4 | `copy_id` | `源码未识别，需核业务` | hyjs-server/db/sql/86_p7_awake_stage_rules.sql:1 | 存在 |
| `p7_awake_states` | `P7AwakeState` | 1 | `player_id` | `player_id` | hyjs-server/db/sql/86_p7_awake_states.sql:1 | 存在 |
| `p7_expedition_catalogue_policies` | `P7ExpeditionCataloguePolicy` | 1 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/110_p7_expedition_catalogue_policies.sql:1 | 存在 |
| `p7_expedition_event_policies` | `P7ExpeditionEventPolicy` | 2 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/105_p7_expedition_event_policies.sql:1 | 存在 |
| `p7_expedition_events` | `P7ExpeditionEvent` | 6 | `serial` | `player_id` | hyjs-server/db/sql/105_p7_expedition_events.sql:1 | 存在 |
| `p7_expedition_heroes` | `P7ExpeditionHero` | 3 | `guid;player_id` | `player_id` | hyjs-server/db/sql/86_p7_expedition_heroes.sql:2 | 存在 |
| `p7_expedition_missions` | `P7ExpeditionMission` | 7 | `serial` | `player_id` | hyjs-server/db/sql/86_p7_expedition_missions.sql:2 | 存在 |
| `p7_expedition_policies` | `P7ExpeditionPolicy` | 2 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/86_p7_expedition_policies.sql:2 | 存在 |
| `p7_expedition_states` | `P7ExpeditionState` | 2 | `player_id` | `player_id` | hyjs-server/db/sql/86_p7_expedition_states.sql:2 | 存在 |
| `p7_expedition_story_policies` | `P7ExpeditionStoryPolicy` | 1 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/114_p7_expedition_story_policies.sql:1 | 存在 |
| `p7_maze_trader_offers` | `P7MazeTraderOffer` | 5 | `player_id;run_id` | `player_id` | hyjs-server/db/sql/174_p7_maze_trader_offers.sql:1 | 存在 |
| `p7_maze_trader_policies` | `P7MazeTraderPolicy` | 3 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/174_p7_maze_trader_policies.sql:1 | 存在 |
| `p7_re_rush_policies` | `P7ReRushPolicy` | 2 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/194_p7_re_rush_policies.sql:1 | 存在 |
| `p7_re_rush_power_proofs` | `P7ReRushPowerProof` | 2 | `floor;player_id;run_id` | `player_id` | hyjs-server/db/sql/p7_re_rush_power_proofs.sql:1 | 存在 |
| `p7_re_rush_runs` | `P7ReRushRun` | 0 | `player_id;run_id` | `player_id` | hyjs-server/db/sql/194_p7_re_rush_runs.sql:1 | 存在 |
| `p7_re_rush_sections` | `P7ReRushSection` | 3 | `player_id;run_id;section` | `player_id` | hyjs-server/db/sql/194_p7_re_rush_sections.sql:1 | 存在 |
| `p7_re_rush_sweeps` | `P7ReRushSweep` | 2 | `floor;player_id;run_id` | `player_id` | hyjs-server/db/sql/194_p7_re_rush_sweeps.sql:1 | 存在 |
| `p7_rush_addition_offers` | `P7RushAdditionOffer` | 8 | `player_id;run_id` | `player_id` | hyjs-server/db/sql/182_p7_rush_addition_offers.sql:1 | 存在 |
| `p7_rush_addition_policies` | `P7RushAdditionPolicy` | 5 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/182_p7_rush_addition_policies.sql:1 | 存在 |
| `p7_rush_continuation_policies` | `P7RushContinuationPolicy` | 2 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/184_p7_rush_continuation_policies.sql:1 | 存在 |
| `p7_rush_fights` | `P7RushFight` | 11 | `id` | `player_id` | hyjs-server/db/sql/86_p7_rush_fights.sql:2 | 存在 |
| `p7_rush_history_certificates` | `P7RushHistoryCertificate` | 5 | `fight_id;player_id;server_id` | `player_id;server_id` | hyjs-server/db/sql/p7_rush_history_certificates.sql:1 | 存在 |
| `p7_rush_manual_calculations` | `P7RushManualCalculation` | 13 | `fight_id` | `player_id` | hyjs-server/db/sql/159_p7_rush_manual_calculations.sql:1 | 存在 |
| `p7_rush_manual_policies` | `P7RushManualPolicy` | 1 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/159_p7_rush_manual_policies.sql:1 | 存在 |
| `p7_rush_policies` | `P7RushPolicy` | 1 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/86_p7_rush_policies.sql:2 | 存在 |
| `p7_rush_progress` | `P7RushProgress` | 2 | `kind;player_id;task_id` | `player_id` | hyjs-server/db/sql/86_p7_rush_progress.sql:2 | 存在 |
| `p7_rush_receipts` | `P7RushReceipt` | 3 | `domain;player_id;timestamp` | `player_id` | hyjs-server/db/sql/86_p7_rush_receipts.sql:2 | 存在 |
| `p7_rush_scoped_calculations` | `P7RushScopedCalculation` | 8 | `fight_id` | `player_id` | hyjs-server/db/sql/150_p7_rush_scoped_calculations.sql:1 | 存在 |
| `p7_rush_scoped_policies` | `P7RushScopedPolicy` | 1 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/150_p7_rush_scoped_policies.sql:1 | 存在 |
| `p7_rush_stage_rules` | `P7RushStageRule` | 8 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/86_p7_rush_stage_rules.sql:2 | 存在 |
| `p7_rush_states` | `P7RushState` | 2 | `player_id` | `player_id` | hyjs-server/db/sql/86_p7_rush_states.sql:2 | 存在 |
| `p7_section_reward_offers` | `P7SectionRewardOffer` | 6 | `player_id;run_id` | `player_id` | hyjs-server/db/sql/178_p7_section_reward_offers.sql:1 | 存在 |
| `p7_section_reward_policies` | `P7SectionRewardPolicy` | 5 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/178_p7_section_reward_policies.sql:1 | 存在 |
| `p8_gather_attempts` | `P8GatherAttempt` | 5 | `player_id` | `player_id` | hyjs-server/db/sql/87_p8_gather_attempts.sql:1 | 存在 |
| `p8_gather_receipts` | `P8GatherReceipt` | 4 | `operation;player_id;timestamp` | `player_id` | hyjs-server/db/sql/87_p8_gather_receipts.sql:1 | 存在 |
| `p8_life_skills` | `P8LifeSkill` | 3 | `player_id;skill_id` | `player_id` | hyjs-server/db/sql/87_p8_life_skills.sql:1 | 存在 |
| `p8_life_states` | `P8LifeState` | 4 | `player_id` | `player_id` | hyjs-server/db/sql/87_p8_life_states.sql:1 | 存在 |
| `p8_resource_nodes` | `P8ResourceNode` | 4 | `player_id;pos_id;scene` | `player_id` | hyjs-server/db/sql/87_p8_resource_nodes.sql:1 | 存在 |
| `p8_resource_reservations` | `P8ResourceReservation` | 3 | `line;pos_id;scene;server_id` | `player_id;server_id` | hyjs-server/db/sql/87_p8_resource_reservations.sql:1 | 存在 |
| `p9_pet_copy_grants` | `P9PetCopyGrant` | 4 | `player_id;source_key` | `player_id` | hyjs-server/db/sql/88_p9_pet_copy_grants.sql:1 | 存在 |
| `p9_pet_deferred_consumptions` | `P9PetDeferredConsumption` | 3 | `guid;player_id;source_key` | `player_id` | hyjs-server/db/sql/98_p9_pet_deferred_consumptions.sql:1 | 存在 |
| `p9_pet_hudong_rules` | `P9PetHudongRule` | 3 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/104_p9_pet_hudong_rules.sql:1 | 存在 |
| `p9_pet_hudong_states` | `P9PetHudongState` | 4 | `guid` | `player_id` | hyjs-server/db/sql/104_p9_pet_hudong_states.sql:1 | 存在 |
| `p9_pet_instances` | `P9PetInstance` | 4 | `guid` | `player_id` | hyjs-server/db/sql/88_p9_pet_instances.sql:1 | 存在 |
| `p9_pet_intimacy_rules` | `P9PetIntimacyRule` | 2 | `quality` | `源码未识别，需核业务` | hyjs-server/db/sql/98_p9_pet_intimacy_rules.sql:1 | 存在 |
| `p9_pet_investments` | `P9PetInvestment` | 4 | `guid` | `player_id` | hyjs-server/db/sql/88_p9_pet_investments.sql:1 | 存在 |
| `p9_pet_market_rules` | `P9PetMarketRule` | 3 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/101_p9_pet_market_rules.sql:1 | 存在 |
| `p9_pet_market_states` | `P9PetMarketState` | 6 | `player_id` | `player_id` | hyjs-server/db/sql/101_p9_pet_market_states.sql:1 | 存在 |
| `p9_pet_receipts` | `P9PetReceipt` | 3 | `operation;player_id;timestamp` | `player_id` | hyjs-server/db/sql/88_p9_pet_receipts.sql:1 | 存在 |
| `p9_pet_recruit_progress` | `P9PetRecruitProgress` | 2 | `player_id` | `player_id` | hyjs-server/db/sql/100_p9_pet_recruit_progress.sql:1 | 存在 |
| `p9_pet_recruit_rules` | `P9PetRecruitRule` | 3 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/100_p9_pet_recruit_rules.sql:1 | 存在 |
| `p9_pet_retired` | `P9PetRetired` | 5 | `guid` | `player_id` | hyjs-server/db/sql/88_p9_pet_retired.sql:1 | 存在 |
| `p9_pet_states` | `P9PetState` | 2 | `player_id` | `player_id` | hyjs-server/db/sql/88_p9_pet_states.sql:1 | 存在 |
| `p9_pet_talent_allocations` | `P9PetTalentAllocation` | 4 | `guid` | `player_id` | hyjs-server/db/sql/98_p9_pet_talent_allocations.sql:1 | 存在 |
| `player_opening_calendars` | `PlayerOpeningCalendar` | 1 | `player_id` | `player_id` | hyjs-server/db/sql/206_player_opening_calendars.sql:1 | 存在 |
| `profiles` | `Profile` | 7 | `id` | `account_id;server_id;uid` | hyjs-server/db/sql/01_profiles.mysql.sql:1<br>hyjs-server/db/sql/01_profiles.sql:1 | 存在 |
| `recharge_directory_members` | `RechargeDirectoryMember` | 2 | `index_id` | `源码未识别，需核业务` | hyjs-server/db/sql/recharge_directory_members.sql:2 | 存在 |
| `recruit_runtime_rules` | `RecruitRuntimeRule` | 0 | `kind` | `源码未识别，需核业务` | hyjs-server/db/sql/240_recruit_runtime_rules.sql:2 | 存在 |
| `role_wallet_balances` | `RoleWalletBalance` | 0 | `player_id;server_id` | `account_id;player_id;server_id` | hyjs-server/db/sql/role_wallet_balances.sql:1 | 存在 |
| `role_wallet_entries` | `RoleWalletEntry` | 7 | `key;player_id;server_id` | `account_id;player_id;server_id` | hyjs-server/db/sql/role_wallet_entries.sql:1 | 存在 |
| `role_wallet_migrations` | `RoleWalletMigration` | 0 | `account_id` | `account_id;player_id;server_id` | hyjs-server/db/sql/role_wallet_migrations.sql:1 | 存在 |
| `server_game_clock_operations` | `ServerGameClockOperation` | 2 | `request_id;server_id` | `actor_id;server_id` | hyjs-server/db/sql/204a_server_game_clock_operations.sql:1 | 存在 |
| `server_game_clock_recoveries` | `ServerGameClockRecovery` | 0 | `player_id;server_id` | `player_id;server_id` | hyjs-server/db/sql/204b_server_game_clock_recoveries.sql:1 | 存在 |
| `server_game_clocks` | `ServerGameClock` | 0 | `server_id` | `server_id` | hyjs-server/db/sql/204_server_game_clocks.sql:1 | 存在 |
| `shared_field_boss55` | `SharedFieldBoss55` | 7 | `line;pos_id;scene;server_id` | `killer_id;server_id` | hyjs-server/db/sql/shared_field_boss55.sql:1 | 存在 |
| `shop_products` | `ShopProduct` | 4 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/12_shop_products.sql:2 | 存在 |
| `shop_receipts` | `ShopReceipt` | 2 | `player_id` | `player_id` | hyjs-server/db/sql/16_shop_receipts.sql:2 | 存在 |
| `shop_states` | `ShopState` | 2 | `day;player_id;product_id` | `player_id` | hyjs-server/db/sql/13_shop_states.sql:2 | 存在 |
| `skill_specialty_rules` | `SkillSpecialtyRule` | 4 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/42_skill_specialty_rules.sql:1 | 存在 |
| `story_quest_receipts` | `StoryQuestReceipt` | 6 | `player_id;stamp` | `player_id` | hyjs-server/db/sql/187_story_quest_receipts.sql:1 | 存在 |
| `t_example` | `ExampleModel` | 2 | `id` | `源码未识别，需核业务` | 未找到建表SQL | 未见 |
| `terminal_account_states` | `TerminalAccountState` | 0 | `account_id` | `account_id` | hyjs-server/db/sql/240_terminal_account_states.sql:1 | 存在 |
| `terminal_batches` | `TerminalBatch` | 6 | `id` | `server_id` | hyjs-server/db/sql/240_terminal_batches.sql:1 | 存在 |
| `terminal_bursts` | `TerminalBurst` | 2 | `account_id;request_id` | `account_id` | hyjs-server/db/sql/240_terminal_bursts.sql:1 | 存在 |
| `terminal_incomes` | `TerminalIncome` | 3 | `account_id;id` | `account_id` | hyjs-server/db/sql/240_terminal_incomes.sql:1 | 存在 |
| `terminal_recipients` | `TerminalRecipient` | 3 | `account_id;batch_id` | `account_id;player_id` | hyjs-server/db/sql/240_terminal_recipients.sql:1 | 存在 |
| `terminal_role_bursts` | `TerminalRoleBurst` | 2 | `player_id;request_id;server_id` | `account_id;player_id;server_id` | hyjs-server/db/sql/terminal_role_bursts.sql:1 | 存在 |
| `terminal_role_incomes` | `TerminalRoleIncome` | 3 | `id;player_id;server_id` | `account_id;player_id;server_id` | hyjs-server/db/sql/terminal_role_incomes.sql:1 | 存在 |
| `terminal_role_states` | `TerminalRoleState` | 0 | `player_id;server_id` | `account_id;player_id;server_id` | hyjs-server/db/sql/terminal_role_states.sql:1 | 存在 |
| `terminal_seed_migrations` | `TerminalSeedMigration` | 1 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/240_terminal_seed_migrations.sql:1 | 存在 |
| `terminal_settings` | `TerminalSetting` | 1 | `server_id` | `server_id` | hyjs-server/db/sql/240_terminal_settings.sql:1 | 存在 |
| `treasure_box_rewards` | `TreasureBoxReward` | 3 | `id;item_id` | `源码未识别，需核业务` | hyjs-server/db/sql/18_treasure_box_rewards.sql:1 | 存在 |
| `treasure_box_rules` | `TreasureBoxRule` | 3 | `item_id` | `源码未识别，需核业务` | hyjs-server/db/sql/17_treasure_box_rules.sql:1 | 存在 |
| `turntable_draw_receipts` | `TurntableDrawReceipt` | 2 | `player_id;stamp` | `player_id` | hyjs-server/db/sql/197_turntable_draw_receipts.sql:1 | 存在 |
| `turntable_globals` | `TurntableGlobal` | 3 | `pool_id` | `源码未识别，需核业务` | hyjs-server/db/sql/04_turntable_globals.sql:2 | 存在 |
| `turntable_personals` | `TurntablePersonal` | 2 | `player_id;pool_id` | `player_id` | hyjs-server/db/sql/199_turntable_personals.sql:1 | 存在 |
| `turntable_records` | `TurntableRecord` | 3 | `id` | `player_id` | hyjs-server/db/sql/07_turntable_records.sql:2 | 存在 |
| `turntable_rules` | `TurntableRules` | 5 | `pool_id` | `源码未识别，需核业务` | hyjs-server/db/sql/06_turntable_rules.sql:2 | 存在 |
| `turntable_states` | `TurntableState` | 5 | `player_id;pool_id` | `player_id` | hyjs-server/db/sql/05_turntable_states.sql:1 | 存在 |
| `warrior_awake_rules` | `WarriorAwakeRule` | 2 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/23_warrior_awake_rules.sql:3 | 存在 |
| `warrior_growth_receipts` | `WarriorGrowthReceipt` | 4 | `guid;kind;player_id` | `player_id` | hyjs-server/db/sql/15_warrior_growth_receipts.sql:1 | 存在 |
| `warrior_growth_rules` | `WarriorGrowthRule` | 3 | `id` | `源码未识别，需核业务` | hyjs-server/db/sql/14_warrior_growth_rules.sql:2 | 存在 |

逐字段类型、证据行、未绑定线索和待核状态见 [schema-candidates.csv](schema-candidates.csv)。
