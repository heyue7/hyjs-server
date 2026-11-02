# 存储结构与竞技、秘境、野外首领性能调研

本轮按用户要求只调研。技术判断、数据归属和后续方案由主代理确定；执行代理仅按指定范围提取字段、调用链和既有证据。没有修改产品源码、安装客户端、运行游戏测试或发布，也没有接入对象存储。

研究基线为源码 `f906b82`，其中产品提交 `3fb495f43063933d127577c4a14a772b8c30ceca`。本轮重新只读核验生产容器仍运行该版本，应用重启计数 0，MySQL 历史重启计数仍为 5。时间证据统一保存为 UTC，避免将客户端时区、运营日历和服务端真实时间混用。

## 1. 结论与优先顺序

1. 内网对象存储适合承载大体积、不可变、低频读取的负载，以及按版本保存的战斗规则包。角色当前资源、位置、库存和进行中玩法状态继续由 MySQL 事务管理。
2. `player` 字段本身很小；大角色存档主要是 `game`。当前最大的数据库负载来自战斗输入、结算终末和重复冻结的规则表。把整个 `player/game` 搬到对象存储，仍须整体编解码，并增加网络访问及跨存储提交问题，不能据此承诺交互提速。
3. 下一轮先处理竞技/夺宝的全局策略写锁、全名册重建和重复输入校验，再处理首领刷新锁与客户端等待恢复。随后拆分角色高频状态，并在轻量业务证明就绪后迁出冷负载。
4. 上一轮已经减少的个人活动、矿点和移动互斥不等于所有路径已完成。此次继续确认竞技和普通首领存在尚未优化到位的公共锁及重复计算。

## 2. 全量盘点范围与编码清单

完整盘点文件：

- [全字段候选 CSV](/Users/lirui/.codex/worktrees/hyjs-player-concurrency-20261102/hyjs/analysis/storage-gameplay-research/schema-candidates.csv)：2,241 行，逐项包含表/字段、Go 类型、SQL 类型、主键、归属线索、生产存在性和编码证据。
- [聚合字段 CSV](/Users/lirui/.codex/worktrees/hyjs-player-concurrency-20261102/hyjs/analysis/storage-gameplay-research/aggregate-fields.csv)：从完整候选中筛出已有编码证据的 JSON、压缩 JSON、Proto、分隔字符串和 WAV，共 347 字段、220 张表。
- [逐表盘点及核验边界](/Users/lirui/.codex/worktrees/hyjs-player-concurrency-20261102/hyjs/analysis/storage-gameplay-research/schema-inventory.md)：覆盖 537 个源码模型表、535 个不同建表 SQL 表；生产采样为 535 张表，全部是 InnoDB。另两个源码表为 `p13_union_fight_settings` 和示例 `t_example`，生产采样未见。
- [角色 game 内部组件](/Users/lirui/.codex/worktrees/hyjs-player-concurrency-20261102/hyjs/analysis/storage-gameplay-research/profile-components.csv)：38 个持久化顶层组件，按资产、运行状态、阵容、进度和回执分组；临时字段不计入。

为避免遗漏短字符串内的结构，候选包含所有持久化 string、字节字段，以及 SQL 文本/二进制类型。候选数包含普通名称、ID、摘要和凭据，不能等同于 JSON 数量。355 个候选具有人工调用链或静态字段绑定证据；其余 1,886 个仍保留待核状态，其中大部分属于普通文本/标识或摘要候选。所有 722 个生产目标类型列均已对应到源码候选；没有把未知 BLOB 自动标为 JSON。

| 已有绑定证据的格式 | 字段数 | 不同表数 | 典型位置 |
| --- | ---: | ---: | --- |
| JSON | 186 | 150 | profiles、活动冻结规则、共享点位、结算/排名快照 |
| JSON/gzip 兼容 | 5 | 5 | P6 input、试炼 series、跨服最终结果/续局输入 |
| Protobuf | 146 | 89 | 原开始包、响应回执、阵容、社交/场景帧 |
| 分隔字符串 | 9 | 9 | 奖励或部分标识列表；部分普通奖励字符串尚未绑定到解码器 |
| WAV | 1 | 1 | p10_voice_assets.bytes |

同表可同时存在多种编码。这里的“聚合字段”描述 MySQL 行里的序列化负载，不表示生产另有一个 NoSQL 数据库。

盘点还发现 SQL 与模型的名称差异：`P12WantedInstance.MonsterHPJSON` 使用 `monster_hp_json`，结构 SQL 写作 `monster_hpjson`，生产目标列采样只有前者。该差异须在迁移时核对；本轮没有证明它造成当前卡顿。其他未绑定大字段已在盘点末尾列出，保留普通文本、Proto 和 JSON 的待核边界。

### 数据库以外的存储

- SQLite：用于迁移源及显式 `--db` 隔离兼容入口。生产本轮核验仍使用 MySQL；本轮没有启动 SQLite 或本机数据库。
- 原游戏 TSV：启动时读取为 `GameService.Tables` 内存结构；运行时多次冻结、复制和编码这些表。它们属于规则配置，当前已经在进程内，无需每场从对象存储重新下载。见 [LoadTables](/Users/lirui/.codex/worktrees/hyjs-player-concurrency-20261102/hyjs/hyjs-server/service/copyService.go:29)。
- Redis：存在基础封装，未找到游戏代码对该封装的初始化/使用调用。当前游戏启动不依赖 Redis。
- OSS/COS：存在 `fileStorage` 封装，未找到游戏读写对 `Init/GetStorage` 的调用。它还没有参与角色或回放保存。
- 诊断/工具文件：试炼拒绝 End 保存为 .pb，现有上限为 4 个、每个 64 KiB、20 分钟；离线导出、迁移和 Trial87 诊断工具还写 JSON/PB。它们与资产/领奖账本生命周期分开。见 [拒绝证据保存](/Users/lirui/.codex/worktrees/hyjs-player-concurrency-20261102/hyjs/hyjs-server/service/p7RejectedEvidenceService.go:15)。
- 连接、原表索引和配置等内存结构是运行态缓存，单独评估失效和作用域；它们不计为落库字段，也不迁成对象文件。

## 3. 生产数据量：先优化谁

[生产存储元数据](/Users/lirui/.codex/worktrees/hyjs-player-concurrency-20261102/hyjs/analysis/storage-gameplay-research/production-storage-metadata.json) 采样时：73 个 profile，`player` 总计 34,096 字节，平均 467 字节；`game` 总计 5,496,034 字节，平均约 73.5 KiB，最大约 181.6 KiB。这是持久化字节数，不能代表展开压缩人物/战斗输入后的内存或 CPU 成本。

| 表 | DATA_LENGTH | 主要负载 | 本轮判断 |
| --- | ---: | --- | --- |
| p6_battle_records | 2,666.6 MiB | input、operates、outcome | 冻结表去重、轻量证明与回放负载分离优先 |
| p12_wanted_fights | 2,360.5 MiB | 原输入 JSON、终末 JSON、开始/结束 PB | 须梳理输入/终末重复与业务验证依赖 |
| p11_arena_fight_settlements | 318.5 MiB | response、terminal | 先减少重复校验，区分结算事实和大终末 |
| p7_rush_manual_calculations | 245.5 MiB | series、preview、terminal | 活跃续局就近；结束后按引用和证明分层 |
| p11_plunder_fight_settlements | 208.5 MiB | response、terminal | 与 P6 同场输入/结果的引用和复用 |
| p12_wanted_instances | 60.5 MiB | 定义及怪物 HP JSON | 活跃 HP 留事务内，静态定义可按版本复用 |
| profiles | 约 9.0 MiB | player/game 及 InnoDB 存储开销 | 容量小；高频全读、解码和写回值得拆分 |

上述为 InnoDB 统计中的已分配数据空间，含页和历史布局影响；不等于字段有效字节、可立即回收空间或对象存储节省量。535 张表的 DATA_LENGTH+INDEX_LENGTH 合计约 5.90 GiB。

补充的有界采样：[数量与负载长度](/Users/lirui/.codex/worktrees/hyjs-player-concurrency-20261102/hyjs/analysis/storage-gameplay-research/production-payload-size-samples.json)。当时 P6 精确计数 9,021，Wanted fight 358；按主键前 8 条 P6，input 平均约 215 KiB、全部为 gzip，outcome 平均约 134 KiB。这是小样本，不能外推全表平均或新增速率。Wanted 的 4 条长度采样达到 1 秒上限后已停止；先前逐 JSON 顶层键统计达到 3 秒上限后也已停止，均未提高上限重跑。

原 TSV 按当前 `LoadTables` 规则做本地体量估算：普通冻结 11 表约 4.37 MB JSON，包含 WantedEnemy 的 12 表约 5.06 MB；这是 Python 估算、未套运行时 normalizer，不是生产配置摘要。源码确实每次冻结复制这些表，见 [冻结表范围](/Users/lirui/.codex/worktrees/hyjs-player-concurrency-20261102/hyjs/hyjs-server/service/battleReplayService.go:268)。表文件 SHA 与估算方法保存在 [估算记录](/Users/lirui/.codex/worktrees/hyjs-player-concurrency-20261102/hyjs/analysis/storage-gameplay-research/frozen-table-size-estimate.json)。

## 4. 对象存储如何使用

用户提供的约束是内网访问、仅服务器使用。厂商、地域、存储级别、兼容接口和额度尚待补充，本轮未读取密钥、访问 bucket 或测试吞吐；以下方案不预设某一家云的时延及费用。

| 数据类别 | 建议位置/组织 | 对象存储的作用 |
| --- | --- | --- |
| 账号认证、账户余额、账本、领奖/扣费状态 | MySQL 关系字段与既有唯一键/事务 | 维持原权威位置 |
| 角色金币/钻石/体力等小标量 | 按实际角色归属的资源列；与资产变更同事务 | 维持事务一致性，无需对象 GET/PUT |
| 位置、loading、地图/线路、活动 fight/run ID | 角色高频运行行，只读写所需列 | 先减少 game 整体编解码 |
| 人物、装备、材料与阵容 | 按读取/写入边界拆；需要 GUID 点查/独立更新的项目用关系行；同主体小状态合并 | 当前活跃资产不依赖外置大对象 |
| 原规则和冻结规则版本 | 不可变版本包，按内容摘要寻址；当前版本常驻有界内存/本地 | 保存历史版本，启动/冷缺失时读取，跨战斗复用 |
| 进行中战斗独有输入 | 本场攻击/防守快照、seed、版本引用和状态靠近 MySQL；锁外准备 | 规则大包通过版本引用复用，避免每场整包复制 |
| 已结束的大回放、原终末/审计快照 | MySQL 保留身份、归属、时间、摘要、紧凑结果和业务证明；大负载按引用迁出 | 降低数据库冷负载占用与读写放大 |
| 小回执、聊天帧、普通奖励字符串、运营配置 | 原 MySQL 表/列；重复规则按版本解析和复用 | 小数据逐条对象化收益不足，先保留简单路径 |
| 大语音/WAV | 若实际使用量增长，元数据留库，内容可外置 | 容量优化候选，当前优先级低于战斗负载 |

热数据留库、不可变大包按版本复用和冷负载外置要配合实施。单个对象的原子更新无法与 MySQL 中扣费、资产、回执自动组成一个事务；S3 官方也区分单 key 原子性与跨 key 更新边界。这里只引用其语义作为设计参照，用户所购服务仍须按具体厂商核实。[S3 数据一致性说明](https://docs.aws.amazon.com/AmazonS3/latest/userguide/Welcome.html#ConsistencyModel)

### 最小接入方式

1. 先发布不可变规则版本包。上传并校验完成后才允许新战斗引用；每个版本一次准备，游戏请求不争用全服上传锁、计数器或相同可变对象 key。当前规则已在内存时直接复用。
2. 旧记录迁移沿已有有界维护流程执行：读取明确记录及版本，上传内容摘要 key，验证字节/摘要，再以原行版本条件登记 ready 引用。切换前保留原负载；中断可重试。短 MySQL 事务中只切元数据，不持角色/排名/Boss 锁等待上传。
3. 游戏结算与日常列表只取紧凑事实和必要状态；只有回放/人工核查需要完整负载时，由服务器内网读取，再沿原授权接口返回。私有地址或签名地址不直接交给手机。
4. 已结束不等于已无业务引用。P6 仍供扫荡资格、称号、日奖、续局等验证的记录，先建立并核对轻量业务证明及其消费者，再迁出/回收大负载。回放 24 小时与业务证明的保留期分开，不能直接给整个 bucket 配 1 天删除。
5. 从一类负载做小范围迁移和恢复验证，复用现有维护 worker；无需先增加独立缓存服务、通用多态负载巨表或消息队列。对缺失对象、超时及重复上传沿同一版本/摘要恢复，原数据未安全切换前保留。

现有存储封装接入前还需补齐服务端内部读取方式、可配置 endpoint、流式/有界读取、调用超时与 ctx、并发初始化。当前 COS bucketClient 是可变 map，OSS 客户端懒初始化亦无并发保护；两者下载接口都把完整内容读入内存。直接将现有接口放进角色事务会扩大等待。见 [COS 封装](/Users/lirui/.codex/worktrees/hyjs-player-concurrency-20261102/hyjs/hyjs-server/fileStorage/cosStorage.go:35)、[OSS 封装](/Users/lirui/.codex/worktrees/hyjs-player-concurrency-20261102/hyjs/hyjs-server/fileStorage/ossStorage.go:112)。

具体云内网可达范围须核同地域/网络配置；例如 OSS 官方将 public/internal endpoint 分开列出。[OSS endpoint 文档](https://www.alibabacloud.com/help/en/oss/user-guide/regions-and-endpoints) 内网免费流量也不能自动推导请求、存储和取回全部免费，待用户服务规格确认后再核。

## 5. 竞技与夺宝：源码中已经确认的问题

详细证据：[竞技与夺宝调用链](/Users/lirui/.codex/worktrees/hyjs-player-concurrency-20261102/hyjs/analysis/storage-gameplay-research/arena-plunder-evidence.md)。

### 竞技

- 匹配/排行首先对 `p11_arena_policies.id=1 FOR UPDATE`；同一策略行还同时含初次排名分配 `NextRank`。普通查询随后加载所有非空 profiles，没有分页或区服过滤，逐人完整解码，构建阵容/战力，查询并 Save 名册，最后又保存策略。见 [策略写锁](/Users/lirui/.codex/worktrees/hyjs-player-concurrency-20261102/hyjs/hyjs-server/service/p11ArenaService.go:52)、[全名册同步](/Users/lirui/.codex/worktrees/hyjs-player-concurrency-20261102/hyjs/hyjs-server/service/p11ArenaService.go:95)。
- 同一请求会读取全部竞技成员再内存选候选，逐个检查相关条件；NPC 补位也构建阵容。首次初始化、目录稳定读取、名次分配和当前候选选择混在一次请求里。
- 新竞技战斗持双方角色、竞技策略/战斗策略/可能的日结策略以及双方成员相关锁，同步计算后提交。基础新战斗路径对同一 P6 可数到 5 次完整 decode；旧回执也有重复验证。3fb 的 tables 摘要快路径减少了重新编码，但这些解压和整体反序列化仍然存在。
- 通用结算先后两次调用规则资格验证，每次会复制/序列化规则并建立资格引擎；随后才有一次完整战斗计算。不能将两次资格验证说成两次完整模拟。
- 到期日结可以在普通匹配请求里同步关闭最多 32 个窗口并处理奖励。日常路径承担的工作随历史/截止状态波动。

主代理方案：名册按本人真正发生的阵容/等级/属性版本变化增量更新，查询只取有索引的名次区间和所需投影；稳定策略共享读取，初次名次分配放在独立短路径；实际交换名次只保护参与成员及必要的排名唯一性。请求内复用已校验的不可变 P6 输入/规则及结果依据，保留各业务事实的状态、归属和幂等校验。重计算逐步移出公共写锁，采用短受理→锁外计算→短提交，提交复核同一冻结输入和版本。日结归还既有维护流程，边界请求仅处理必要截止，不承担历史全量补结。

### 夺宝与扫荡

- 列表每批扫描最多 64 个完整角色，策略扫描上限 4,096；候选解码、资产过滤、免战/黑名单和最终阵容投影重复进行。无真人时还生成多套 NPC 评估。
- 扫荡先执行并提交一次列表事务，再开启扫荡事务，对本人及所有真人候选加锁。每个实际真人子战，直接调用链中本人和选中对手分别至少 5 次完整 profile 解码；还重复生成匹配、计算配置摘要、读取 P6。请求为 1 或 5 场，响应等待所有已接受子战、回执和当前 player 读取完成。
- 旧回执逐子场读 match/offer/P6 校验，不重算战斗，但仍可能放大 CPU 与读取。

主代理方案：将可检索的等级、可抢碎片资格、保护截止和战斗快照版本做窄投影；匹配阶段只选择少量候选，再批量读取其快照。一个请求内复用本人、候选和规则输入，NPC 不重复生成相同模板。去掉未参与对手的锁，按实际双方有序保护资源。保留当前扫荡停止条件、扣费/转移和整批回执语义；如进一步将子战拆成独立提交，需先核对原失败回滚合同，本轮方案不默认改变它。

## 6. 秘境与首领：分清实际入口和等待阶段

详细原表/IL/桥接证据：[客户端加载与等待](/Users/lirui/.codex/worktrees/hyjs-player-concurrency-20261102/hyjs/analysis/storage-gameplay-research/client-loading-evidence.md)。同样的雪/沙地图资源可被不同玩法复用，故本次分别追了以下入口。

### 主城“秘境”与觉醒地图

- 主城 `npc_011/town_mijing` 对应 `ActivityCopyPage`，走 `CSDailyCopyState 56013`、`CSDailyCopyFightStart 56015`、`CSDailyCopyFightEnd 56017`。服务端连纯状态查询也先锁本人 profile、完整解码并准备公会属性；它按类型读个人次数并组装章节列表。开始时构建队伍、怪物、掉落和 Proto 冻结包，个人次数与体力同事务提交。这里没有发现活动副本独占全服策略行的同类代码，不能套用竞技结论。见 [DailyCopy 分发](/Users/lirui/.codex/worktrees/hyjs-player-concurrency-20261102/hyjs/hyjs-server/service/dailyCopyService.go:275)。
- 觉醒之地是另一入口，走 81005/81007，碰怪开始/结束走 81009/81011。探索入场会为内容怪物逐个冻结和校验资格，碰怪开始又冻结、证明、捕获 P6；这些工作在本人 profile 事务中。见 [逐怪物准备](/Users/lirui/.codex/worktrees/hyjs-player-concurrency-20261102/hyjs/hyjs-server/service/p7AwakeService.go:950)、[开始战斗](/Users/lirui/.codex/worktrees/hyjs-player-concurrency-20261102/hyjs/hyjs-server/service/p7AwakeService.go:1256)。
- 原觉醒客户端等待协程先等 0.5 秒，再等待 `receivePveMsg`；所查协程没有自身超时。缺开始回调、资源未完成和战后 End 未完成会呈现不同的“卡住”，需分别确认。

主代理方案：状态目录读取采用小列和批量个人状态，查询与确需结算恢复的写路径分开；不可变章节/怪物配置按版本准备一次。角色运行状态与库存解耦后，目录与加载握手不再解码全部背包。觉醒逐怪物准备复用规则版本和相同角色来源验证，开始/结束也复用已冻结输入。客户端对网络失败、非法回包和资源加载失败分别退出对应等待，并处理同一实例的续接；下一轮仅用一个目录/一个代表首领分段实测，避免先做全地图资源预热。

### 雪原普通首领

- `refreshSharedFieldBosses55` 对本线配置中的所有 Boss 位置逐个调用 `sharedFieldBoss55Locked`，即使只投影也采用 UPDATE 锁。场景登录、位置握手、Boss 检查/开始/结束等调用它，锁随外层角色事务保持。开始会继续冻结、校验和压缩大输入；结束还进行原战斗计算。见 [刷新全部位置](/Users/lirui/.codex/worktrees/hyjs-player-concurrency-20261102/hyjs/hyjs-server/service/sharedFieldBoss55Service.go:189)、[锁定读](/Users/lirui/.codex/worktrees/hyjs-player-concurrency-20261102/hyjs/hyjs-server/service/sharedFieldBoss55Service.go:125)。
- 现有表主键已是区服/场景/线路/位置。主要问题落在读取方式与持锁时间，优先复用既有主键，无需再增加一张同义 Boss 表。
- P10 常规移动已共享读取，但成员进入/退出/换线仍使用进程级 `p10SceneMutation` 排他门；其中还做完整角色读取和投影，场景之间仍有可继续缩小的互斥。见 [场景同步](/Users/lirui/.codex/worktrees/hyjs-player-concurrency-20261102/hyjs/hyjs-server/service/p10SceneService.go:586)。

主代理方案：普通 Boss 投影使用非锁定快照，只在具体位置的首次创建、到期换代或提交伤害时锁该主键；目标代次/UUID在短事务内复核，保留唯一击杀、实际伤害和奖励一致性。计算与规则准备不持有无关 Boss 锁。场景成员变化收至区服/场景/线路，跨房间迁移按固定顺序处理源/目标；走路不触碰线路容量分配资源。

### 沙漠首领与客户端持续等待

- 沙漠共享首领已按线路存放；开始选择本线具体事件并拿 UPDATE 锁，之后仍在同一事务里构建队伍、冻结/验证规则、编码/压缩输入。结束在本人事务中验证和计算，随后锁具体首领行应用实际伤害。这与普通雪原刷新全部 Boss 的路径不同。
- 沙漠专用桥先显示原预加载，再做位置确认，位置确认成功才发原 30083 开始请求。整体 15 秒超时仅设置 `encounterTimedOut` 并隐藏加载，没有清除 `awaitingEncounter`；新入战被该标志挡住，直到开始回包或场景清理。见 [超时状态](/Users/lirui/.codex/worktrees/hyjs-player-concurrency-20261102/hyjs/tools/desert_extraction/FieldRuntime.cs:347)、[入战门槛](/Users/lirui/.codex/worktrees/hyjs-player-concurrency-20261102/hyjs/tools/desert_extraction/FieldRuntime.cs:605)。这是可发生的源码状态，尚未证明本次现场由它触发。
- 雪原沿原野外入口，沙漠有 field_003 专用位置/恢复桥；它们不能只因外观类似而套同一补丁。已有结算恢复处理也分为 legacy 和 GGO 两路。

主代理方案：超时后复用已有 active/receipt/恢复入口确认服务端是否接纳，决定续接同一冻结战斗或释放本次等待；晚到回包按实例、会话及场景隔离。单纯清标志重发新战斗会破坏已经接纳的状态，须保留原扣费/结果语义。位置确认、开始响应、Unity 资源和战后结算分别计时；提前显示蒙层不计为完整战斗加载提速。

## 7. 当前线上实测证据及缺口

[有界慢日志采样](/Users/lirui/.codex/worktrees/hyjs-player-concurrency-20261102/hyjs/analysis/storage-gameplay-research/production-operation-sample.json) 对应当前 3fb 产品。采集读取 175,794 字节，观察窗口 UTC 03:05–05:40；记录阈值为 Dispatch ≥200 ms，HTTP/TCP解析和网络写回不在该时长内。

| 原操作 | 慢日志条数 | 最大 Dispatch 时间 |
| --- | ---: | ---: |
| 夺宝扫荡 CSSweepTreasureFight | 15 | 16,552 ms |
| 竞技结算 CSPvpFightResult | 21 | 8,135 ms |
| 竞技匹配 CSPvpFindMatch | 26 | 2,410 ms |
| 夺宝单场 CSFightTreasureStartResult | 2 | 5,113 ms |
| 夺宝列表 CSTreasureBattleList | 6 | 733 ms |
| 野外首领开始 CSSidePveBossFightStart | 11 | 2,705 ms |
| 野外首领结束 CSSidePveBossFightEnd | 15 | 5,405 ms |

这些不是全部请求数、P95或同负载对比；日志也不能仅凭 opcode 区分每条 Boss 请求的实际地图。采样时应用 CPU 56.97%、MySQL 14.92%，均属自然负载瞬时值。它们证明当前仍存在长响应，无法单独分解锁等待/计算/读写比例。旧第二批 CPU profile 只作为重复 JSON/回执处理的历史线索，不当作当前版本 profile。

[秘境相关有界日志](/Users/lirui/.codex/worktrees/hyjs-player-concurrency-20261102/hyjs/analysis/storage-gameplay-research/production-mijing-operation-sample.json) 未命中 DailyCopy/Awake 请求，只有 CurrentPosition 10 条、最大 634 ms；缺少命中不能证明秘境正常。当前故障仍缺准确入口、发生时刻、安装桥版本和客户端等待状态。

下轮最小测量保留同一请求标识，分为：点击/碰撞→实际发包→服务端锁等待与计算/SQL→收到响应→第一帧原加载动画→战斗 HUD，结束另计提交到结果页。沙漠继续使用用户已接受的碰撞确认至第一帧原加载动画 ≤1 秒、采集有效触发至动作/倒计时 ≤1 秒；完整资源加载另记。其他新目标在实施前记录为优化目标，不擅自改成既有硬性承诺。

## 8. 下一轮交付单元

| 优先级 | 单元 | 最小验收 |
| --- | --- | --- |
| P0 | Boss 入战/结算等待恢复与分段定位 | 代表地图一次正常开始/结束及一次超时/晚回包；可恢复、不重复接纳或领奖；区分网络和资源阶段 |
| P0 | 竞技匹配与公共策略锁 | A 长战斗时 B 匹配及无关个人动作可完成；普通匹配不重建全服名册；实际名次竞争正确 |
| P0 | 竞技/夺宝重复输入与规则处理 | 相同阵容/seed/规则结果逐项不变；请求内同一输入只验证/解码一次；扫荡 1/5 场保持成本、停止及回执语义 |
| P0 | 雪原普通 Boss 投影与场景成员作用域 | 同线另一 Boss、另一线路及其他角色移动不等待该场计算；同代 Boss 唯一击杀/奖励保持 |
| P1 | 秘境目录、觉醒入场和高频角色运行状态 | 普通状态/位置请求不展开整个库存；只写必要列；断线/转场/活动回执不丢 |
| P1 | 不可变规则版本去重 | 相同规则不逐战存整包；进程间/重启/旧记录仍可验证，战斗对象互不污染 |
| P2 | 冷战斗负载与对象存储试点 | 服务端内网 GET/PUT 延迟及吞吐实测；中断迁移可恢复；摘要和引用完整；资产事务不依赖对象上传 |

实际顺序以首个代表故障复现的阶段为准，优先移除已证重复和公共等待，不新增通用状态机、全服锁或大规模资源预热。表数量减少或 JSON 全面消失都不作为目标；以无关角色可并行、同输入交互更快和持久化语义保持验收。

本轮仅新增研究文档和约 1.4 MB 证据/清单；没有创建数据库、Redis key、容器、构建或手机会话，无运行测试资源需要释放。全部只读采样范围与中止记录见 [研究记录](/Users/lirui/.codex/worktrees/hyjs-player-concurrency-20261102/hyjs/analysis/storage-gameplay-research/research-record.json)。
