# 竞技与夺宝源码及既有性能证据

## 范围和证据口径

- 本文读取的源码工作树 HEAD 为 `f906b82a133bea8dc2f6ac9c37238b93fbb0bf79`；对应产品提交 `3fb495f43063933d127577c4a14a772b8c30ceca`。路径及行号相对该工作树。只分析代码和已有记录，本次没有查询数据库、重跑测试或采样、操作手机或部署。
- 以下“锁”指代码发出的 Gorm/MySQL 锁定读。非主键条件的锁实际扫描及锁覆盖范围取决于数据库执行计划、隔离级别和数据分布，本文未作现场 `EXPLAIN` 或抓取实时锁；各项 SQL 写入也可能取得数据库隐式锁。
- `GameSlowRequestThreshold` 为 200 ms；`Dispatch` 在鉴权、业务分发及返回前处理期间计时，HTTP/TCP 入口解析和参数校验发生在前，网络写回发生在 `Dispatch` 之后。慢日志时长不能当作客户端完整交互时间。见 `hyjs-server/constUtil/game.go:9`、`hyjs-server/service/gameService.go:117-165,231-234`、`hyjs-server/handler/gameHandler.go:1342-1382,1498-1525`。

## 原协议和公共入口

1. HTTP `/sao/` 读取原二进制包 → `model.ParseGamePacket` → `ValidateGameRequest` → `GameService.Dispatch` → `EncodeGameResponses` → 写 HTTP；TCP 同样解析、校验后同步调用 `Dispatch`，再写响应，见 `hyjs-server/handler/gameHandler.go:33-42,1342-1382,1498-1525`。竞技目标的正数、非本人、本服、部分正时间戳校验在 `gameHandler.go:1033-1045`；夺宝扫荡仅接受 1 或 5 次、合法碎片、补能标志和正时间戳，列表/单战校验见 `gameHandler.go:1068-1086`。
2. `dispatch` 先做 `AuthorizeAccount`、区服、`BindLocalPlayContext`、角色活动计划及运行时处理，然后调用 `DispatchP11Arena`：`hyjs-server/service/gameService.go:352-400,543-547`。`ErrConfigurationReadRetry` 可由 `Dispatch` 整次重试，普通场景按一次路径描述。
3. `DispatchP11Arena` 把 `CSPvpFindMatch/CSPvpRank` 交 `p11ArenaQuery`，`CSPvpFightResult` 交 `p11ArenaScopedFightAt`；`CSTreasureBattleList/CSFightTreasureStartResult/CSSweepTreasureFight` 经 `p11PlunderAt` 分别进入列表、单场、扫荡，见 `hyjs-server/service/p11ArenaService.go:566-609`、`hyjs-server/service/p11PlunderService.go:93-105`。
4. 引导版匹配/战斗分别复用上述匹配/战斗逻辑再转换协议消息：`hyjs-server/service/p11ArenaGuideService.go:62-99`。`CSPvpFight/CSPvpFightEnd` 及相应旧引导入口走 `p11ArenaChallenge`，当前检查后返回错误；`CSFightTreasureStart/End` 到 `p11PlunderAt` 后返回关闭错误，见 `p11ArenaService.go:525-564,604-606`、`p11PlunderService.go:94-105`。这些分支不应按成功的结果结算链计数。

## 竞技匹配与排名列表

### 同步路径和锁

`CSPvpFindMatch` / `CSPvpRank` → `p11ArenaQuery`：一个事务内首先通过 `p11ArenaConfig` 读取 `p11_arena_policies.id=1 FOR UPDATE`。相同库中访问该行的竞技请求以及日结工作器有相同的锁竞争点；这不证明其他所有游戏请求都等待它。带正 stamp 的匹配已有回执在持该锁后直接反序列化返回，跳过下面的名册重建。见 `hyjs-server/service/p11ArenaService.go:52-60,185-228`、`hyjs-server/service/p11ArenaDailyService.go:460-478`。

非回执路径在上述锁仍持有时执行：

1. `p11ArenaReserveVirtualSlots` 查询名册总数、最大名次、虚拟 ID 冲突及待迁移占位；对符合条件的每个旧成员按其历史依次检查最多 4 类记录并可能更新该成员。`hyjs-server/service/p11ArenaNPCService.go:24-99`。
2. `PrepareP11ArenaDailyMutation` 在日结表存在时锁 `p11_arena_daily_policies.id=1 FOR UPDATE`，查询到期待结的窗口，单次最多 32 个并在该事务内关闭；即使关闭开关为 false，取策略锁及检查到期窗口在开关判断前。`hyjs-server/service/p11ArenaDailyService.go:167-188`、`hyjs-server/constUtil/p11ArenaDaily.go:15`。
3. `p11SyncArenaMembers` 查询全部 `player IS NOT NULL AND game IS NOT NULL` 的 `profiles`，按 ID 排序，没有分页、指定列或 server_id 条件。它对每条 profile 执行 `p11ArenaProjection`；可用且达等级要求者再按 player_id 查一条竞技成员，并对成员执行 `Save`；然后读取全部竞技成员、逐条停用失效成员，最后 `Save(cfg)`。这里描述查询形状和程序处理数量，未推断数据库物理扫描计划。`hyjs-server/service/p11ArenaService.go:95-158`。
4. `p11ArenaProjection` 对 `player` 和 `game` 做 JSON 解码，计算祈祷属性、`battlePlayer`、`formationFightPower` 和展示资料；`battlePlayer` 与 `formationFightPower` 分别调用 `battleFormationActors`，故每次投影有两次阵容 actor 构建。`p11ArenaService.go:62-91`、`hyjs-server/service/profileService.go:287-308`、`hyjs-server/service/copyService.go:409-414`、`hyjs-server/service/formationService.go:863-879`。
5. 本人 profile 随后又按 ID 读取并投影；`p11ArenaCandidates` 再按 position 加载全部竞技成员，在内存映射名次，匹配真实候选时逐个查询黑名单；没有真实占位的名次按需生成 NPC 战斗阵容/战力。匹配按更高名次、最佳目标及附近名次补齐并排序；排名列表按 rank 从前往后取上限。`p11ArenaService.go:229-267`、`hyjs-server/service/p11ArenaNPCService.go:132-155,174-263`。
6. 候选逐个解码名册中 info 和 battle；匹配分支每个候选再解码双方 battle、设先手、重新编码、生成并插入 match。请求中还序列化一组九张战斗配置表计算 hash，正 stamp 的匹配保存回执。排名和匹配输出均等事务完成后返回。`p11ArenaService.go:160-181,260-316`。

## 竞技单场和每日结算

### 新战斗

`CSPvpFightResult` → `p11ArenaScopedFightAt`。事务先按角色 ID 升序对双方真实 `profiles.id` 加 UPDATE 锁并逐个完整解码；虚拟对手只锁本人。随后按 `(player_id,stamp)` UPDATE 查询战斗结算回执。未命中回执的新战斗依次锁 `p11_arena_policies.id=1`、`p11_arena_fight_policies.id=1`，并可能锁 `p11_arena_daily_policies.id=1`；再按 player_id UPDATE 锁双方活跃 `p11_arena_members`，虚拟对手不锁真实对方成员。匹配行以 viewer、opponent、时间窗口排序 `FOR UPDATE`，P6 记录按新 record ID `FOR UPDATE`。源码入口及顺序见 `hyjs-server/service/p11ArenaFightService.go:38-119,123-181,204-220`、`hyjs-server/service/p10SocialService.go:109-127`。`lockProfilePeriods` 在当前代码中是空操作，不能根据函数名推断本路径先锁 P1 全表：`hyjs-server/service/eventService.go:80-82`。

双方 profile 在上一步 `p10LockProfiles` 后又分别读取；本人做 `decodeProfileForCalendarUpdate`、对手做 `decodeProfile`。`p11RefreshArenaParticipant` 对本人和真实对手分别再次 `p11ArenaProjection`，按快照变化决定是否更新成员；随后解码冻结匹配双方、冻结/保存 P6、校验配置，调用一条选中的竞技结算实现。结果、名次交换、能量、奖金、回执、日挑战事实在同一事务内保存。事务提交后另调用 `resourceProfile`，重新开始事务并对本人 profile 加 UPDATE 锁，完成读取/必要恢复后才返回 `SCPvpFightResult + SCPlayer`。见 `p11ArenaFightService.go:123-181,204-315,319-331`、`hyjs-server/service/p11ArenaSweep84Service.go:16-37`、`hyjs-server/service/resourceRecoveryService.go:106-179`。

### 重复 P6 读取与战斗计算

正常新战斗且奖品回执表尚无该场记录时，源码直接可数出同一个 P6 记录的 5 次 `decodeP6Record`：用于计算的初读（`p11ArenaFightService.go:217-220`）；`p11StoreArenaFightP6`（`:334-335`）；`ApplyP11ArenaFightPrize` 调 `p11ReadArenaFightReceipt`（`p11ArenaPrizeService.go:264-271`、`p11ArenaFightService.go:364-374`）；`applyP11ArenaRankReward` 调排名事实（`p11ArenaRankRewardService.go:36-42,95-96`）；奖品写入后的 `ValidateP11ArenaFightPrizeReceipt` 再调排名事实（`p11ArenaPrizeService.go:420-431,552`）。日结、活动钩子和其他条件分支可能增加读取，以上为可见基础路径，并非全链路 SQL 数量实测。

新战斗按 scope 三选一调用 `ComputeP6ArenaSettlement`、`ComputeP6ActualArenaSettlement` 或 `ComputeP11GenericMechanismSettlement`；进入所选成功路径后有一次 `ComputeOriginalP6ProtocolFinalHP` 完整战斗计算。通用分支先 `ValidateP6GenericMechanisms`，再进入 `ComputeP6GenericMechanismBattle` 时再次调用同一验证；各次验证都 `FreezeBattleInput`、序列化/校验配置并初始化一个资格校验引擎，然后才进行一次完整战斗计算。初次捕获另有一次 `FreezeBattleInput`。计算在 Go 服务端调用链内同步完成，不能据此称为每场多次完整战斗或外部原客户端进程调用。证据：`p11ArenaFightService.go:224-233`、`hyjs-server/service/p6ArenaSettlementService.go:26-50`、`hyjs-server/service/p6ActualArenaSettlementService.go:29-54`、`hyjs-server/service/p11GenericMechanismService.go:50-77`、`hyjs-server/service/p6GenericMechanismService.go:20-43,69-90,222-238`、`hyjs-server/service/battleReplayService.go:232-296,404-420`、`hyjs-server/service/combatRoundService.go:317-358,388-393`。

当前 `decodeP6Record` 仍先处理 gzip（若输入为 gzip）、校验摘要，再将完整输入 JSON 反序列化为 `FrozenBattleInput`，并另反序列化 envelope 取得原始 tables 字节；当前规范字节可直接算配置摘要，历史非规范字节才回退完整 tables 重编码。见 `hyjs-server/service/battleReplayService.go:67-85,202-229`。

### 回执和日结

同 stamp 已接受竞技战斗先在事务内锁双方 profile 和 `(player_id,stamp)` 回执，复制回执后提交；`p11ReadArenaFightReceipt` 在这笔事务外重新读取 P6 并验证，奖品回执存在时其排名事实还再次读取 P6，然后 `resourceProfile` 再起一次 profile 事务；该回执分支不重新计算战斗。`hyjs-server/service/p11ArenaFightService.go:49-75,315-331,364-443`、`p11ArenaPrizeService.go:436-455,552-563`、`p11ArenaRankRewardService.go:36-42`。

有到期窗口时，日结按挑战分组统计、读取前 1000 名次；合资格挑战验证按 256 条批量读取精简 settlement/P6 元数据，每个合资格成员查最近一条 challenge，并对不同 challenge 的最新证明通过 `p11ReadArenaFightReceipt` 验证，事务内局部 map 复用已检查事实。完成后逐名创建奖励递送及 award。上述关闭工作可发生在匹配/排行的 `p11SyncArenaMembers` 内，也可发生在新战斗计算前后两次 `PrepareP11ArenaDailyMutation` 调用中；是否有到期窗口取决于运行数据。`hyjs-server/service/p11ArenaDailyService.go:167-185,273-335,337-455`、`p11ArenaFightService.go:117-121,240-245`。

## 夺宝列表、单场、扫荡

### 对手列表

`CSTreasureBattleList` → `p11PlunderListAt` 在一个事务内先锁本人 `profiles.id FOR UPDATE`，先 `decodeProfile`，再经 `p11ArenaProjection` 对本人再次完整解码及生成阵容。`p11PlunderFightOffer` 先普通读取 `p11_plunder_fight_policies.id=1` 的 version 并统计 offer：已有版本时对 policy 行取 SHARE，否则 UPDATE 并可首次冻结；正式读状态与预读冲突返回重试。`hyjs-server/service/p11PlunderFightService.go:120-170,207-245`。

对手查询以 `id > cursor`、排除本人、同 server 且 player/game 非空为条件，按 ID 正序每批最多 64 个完整 profile，最多处理 `ScanLimit` 条（策略合法上限 4096），找到 `MatchCount` 即止（策略合法上限 3）。每个候选先完整 decode 并按等级、碎片过滤；持有碎片的候选逐个查免战和黑名单；最终合格真实对手再经 `p11ArenaProjection` 再完整 decode 一次，生成 info/双方战斗快照并插入 match。`p11PlunderFightService.go:105-106,247-334`。

当未找到真实对手、没有不可验证候选且碎片符合本地 NPC 条件时，还用 `LIMIT 1` 检查游标之后是否有剩余 profile；确认扫描结束后生成 10 套 NPC 模板，各自做两次战力计算及一次 battlePlayer，按与本人的战力差排序取 3 个并逐个插入 match。见 `p11PlunderFightService.go:335-359`、`hyjs-server/service/p11PlunderNPC84Service.go:33-46,58-110,113-177`、`hyjs-server/constUtil/p11PlunderNPC84.go:5`。只有本事务结束才返回列表；未核实际 profile 数量和该条件的索引执行计划。

### 真实单场与历史回执

`CSFightTreasureStartResult` → `p11PlunderFightAt` → `p11PlunderFightTx`：外层一个事务；`p10LockProfiles` 按 ID 升序 UPDATE 锁并完整 decode 本人与真实对手，NPC 只锁本人。已有 `(player_id,stamp)` settlement 回执时，调用 `p11ReadPlunderReceipt` 验证；该函数在外层事务尚未结束时读取 match、offer、P6 并 decode 完整 P6，仍持有前述 profile 锁。`hyjs-server/service/p11PlunderFightService.go:365-415,665-672,701-735`、`hyjs-server/service/p10SocialService.go:109-127`。

新单场按规则读取/锁 offer，重新读取双方 profile 并各做日历 decode，再对双方分别执行 `p11ArenaProjection`（真实对手路径）；计算配置 hash。`p11Plunder84ChooseMatch` 以 owner、opponent、fragment、时间窗口查询 `FOR UPDATE`，排序后最多取 32 行，再批量查已使用的 match_id 并逐条比较冻结阵容；扫荡子场另传具体 match ID。实际锁扫描范围需以数据库计划核实。`p11PlunderFightService.go:438-524`、`hyjs-server/service/p11PlunderNPC84Service.go:204-255`、`hyjs-server/constUtil/p11PlunderNPC84.go:8`。

每个新单场捕获一份 P6、decode 后调用一次 `ComputeP6PlunderSettlement`：原证据 scope 符合时调用一次 `ComputeOriginalP6ProtocolFinalHP`，否则进入上文所述通用 scope，成功时同样执行一次完整战斗；真实碎片转移时保存双方，否则按分支保存本人。settlement、P4/P1 事实及可能的开服进度均在事务中；提交后 `resourceProfile` 又对本人进行一次独立事务读取/必要恢复，然后返回战斗响应和当前 player。`p11PlunderFightService.go:524-672`、`hyjs-server/service/p6PlunderSettlementService.go:35-61`、`p11PlunderFightService.go:365-386`。

### 扫荡

`CSSweepTreasureFight` 有旧回执时先在事务外按 `(player_id,stamp)` 验证；无旧回执先同步调用并提交上述列表事务，取得最多 3 个对手，然后进入第二个扫荡事务。该事务将本人及所有真实候选 profile 按 ID 排序逐一 UPDATE 加锁，重新查 sweep 回执、读取本人及 offer。若并发请求此时已留下回执，`p11ReadPlunderSweepReceipt` 会在持有这些 profile 锁的事务内执行。`hyjs-server/service/p11PlunderSweepService.go:64-180`。

请求为 1 或 5，循环到请求次数，或因已获得目标、真实持有者耗尽、能量不足而提前停止。每次循环重读并日历 decode 本人，依次重读/解码真实候选并查保护/黑名单；`p11PlunderSweepMatch` 再读本人和真实对手并各做投影、再算一次九表配置 hash、创建 match；分配子 stamp 最多做 8 次碰撞 count；`p11PlunderFightTx` 再对参与者调用 `p10LockProfiles`、重读/解码并投影、再次计算配置 hash，然后在同一事务内完成一场 P6 计算及结算。按这些直接调用，**每完成一个真实对手子场**，循环中至少有本人 5 次、选中对手 5 次完整 profile 解码；统计不含列表事务、循环前/后本人解码、提交后的 `resourceProfile` 和条件钩子。若选中 NPC，则没有虚拟对手 profile 行，另有 NPC 投影。见 `p11PlunderSweepService.go:185-282,292-324,329-417`、`p11PlunderFightService.go:392-403,438-524`、`p11ArenaService.go:62-91`。

每个已接受子场同步调用一次完整战斗；成功响应聚合所有已完成子场，写 sweep receipt 并提交，再读当前 player 后才返回。回放逐个读取子 settlement，依次读取该子场 match、offer、P6 并校验，最多 5 子场；无新对手选择和战斗计算。`p11PlunderSweepService.go:263-324,420-482`、`p11PlunderFightService.go:701-735`。

其他入口中，`CSRefreshTreasFrag` 锁本人 profile、解码/遍历库存并调用 `saveProfile`；`CSUseAvoidPlunder` 锁本人 profile，按个人/操作/stamp 校验回执，读取普通策略后保存个人免战状态、profile、活动事实与回执。见 `hyjs-server/service/p11PlunderService.go:107-218`。

## 其他条件锁

事务绑定全服时钟时，`ServerGameClockGuardTransaction` 读取对应 `server_game_clocks.server_id FOR SHARE`；未绑定时直接返回：`hyjs-server/service/serverGameClockService.go:115-132`。活动事件命中时，`p1EventPeriods` 先筛选候选，再按 `PeriodInstance.id` 对已冻结行取 SHARE、未冻结行取 UPDATE，个人进度按 `(instance_id,player_id)` UPDATE；相关调用取决于 P1/P4 是否就绪及实际活动配置，不能把它当作本次所有请求都命中的锁。`hyjs-server/service/periodOwnershipService.go:70-122`、`hyjs-server/service/eventService.go:132-140,189-212,437-445`、`hyjs-server/service/p4PlunderActivityService.go:51-110`。

## 既有时延和 CPU 记录

### 产品 3fb495f 的生产有界慢日志

`analysis/storage-gameplay-research/production-operation-sample.json:2-32,52-56,67-85` 记录 2026-11-02 05:41:09 UTC 的只读采样，游戏容器 revision 为 `3fb495f`；日志读取 175,794 字节，观察到的日志时间为 03:05:38.869–05:40:55.242 UTC。以下为该窗口 **慢日志条数和最大值**，不是总请求数或分位数：

| 原请求 | 条数 | 最大 Dispatch 时间 |
| --- | ---: | ---: |
| `CSSweepTreasureFight` | 15 | 16,552 ms |
| `CSPvpFightResult` | 21 | 8,135 ms |
| `CSPvpFindMatch` | 26 | 2,410 ms |
| `CSFightTreasureStartResult` | 2 | 5,113 ms |
| `CSTreasureBattleList` | 6 | 733 ms |
| `CSPvpRank` | 1 | 1,403 ms |

该样本未记录每次扫荡实际子场数、角色规模、SQL 执行计划或逐阶段耗时，不能从最大值单独确认具体瓶颈或锁等待比例。

### 第二批 3de72b3 的历史记录

`analysis/player-concurrency-20261102/production-final-observation.json` 记录 2026-11-01 20:11:40–20:23:58 UTC 的有界慢日志：`CSSweepTreasureFight` 2 条 3,684 / 3,832 ms，`CSPvpFightResult` 2 条 3,527 / 3,505 ms；匹配 3 条最大 675 ms、夺宝列表 1 条 322 ms。该时间窗口的第二批容器版本见 `production-second-release-early.json:11`，为 `3de72b3`。

`analysis/player-concurrency-20261102/p11-sweep-cpu-cum.txt:1-24` 和 `p11-arena-cpu-cum.txt:1-25` 是同批次上下文中的本地测试 CPU profile；profile 文件本身不含提交号。扫荡文件总采样 8.78 CPU 秒、`p11PlunderSweepAt` 累计 3.61 CPU 秒；竞技文件总采样 7.61 CPU 秒、`p11ArenaScopedFightAt` 累计 2.98 CPU 秒。二者都展示了 receipt/P6 decode 在累计栈中出现；累计数有父子重叠，不能相加，也不是一次线上请求耗时。测试包括既有夹具及多步用例。

最终版 `3fb495f` 已加入上文规范 tables 字节摘要快路径。`analysis/player-concurrency-20261102/qualification.json` 的本机压缩 P6 解码微基准记录中位时间降低 30.7%，仅限同一原表夹具的解码操作；旧 CPU profile 不可当作最终版采样，也不能用该微基准比例推算线上整体时延。

## 未由本次证实

- 当前请求的实际 profile/名册/活动行数、各 SQL 的索引与物理锁范围、锁等待者和持有者、对手命中数量、扫荡实际子场数及逐阶段耗时。
- 现有生产慢日志与某一次具体数据库等待或某一段计算之间的因果关系；实际客户端等待、服务资源变化和持续负载下的分布。
