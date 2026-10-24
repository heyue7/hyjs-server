# 配置范围 UI 审查

## 初始决定（实施前）

- 保持现有蓝色、Naive 组件与表格/列表结构；所有接口、数据、校验、保存、权限及脏草稿机制保持。
- 试炼：删除动态开发来源与算法复述，防御作用保留到字段标题；普通关闭后果贴近开关，生效与旧轮次处理合并到保存前可见短说明。
- 活动/商店：逐个审查独立分支、详情、编辑、说明与二级入口；来源文本只在真正作为运营备注输入时保留，开发来源输出删除。费用、期限、冻结/限购与历史记录后果保留。
- 发现局部render样式失效时使用有效deep样式或约定公共ops-table-actions；去掉删文空壳。
- 检查320/390布局的源码约束；真实视口、交互、保存/失败/关闭由主代理验，当前报告不声称运行通过。

## 逐文件覆盖（45个Vue）

以下是源码静态审查；实际显示、键盘/触屏、保存和失败恢复均待主代理运行核验。无独立详情/二级弹层的文件已明确列出；共享工作区覆盖每个所属入口。

| 文件（admin-web/src/） | 主页面/空态 | 详情/编辑/二级入口 | 处理或保留依据 |
|---|---|---|---|
| components/operations/activity-milestones.vue | 奖励档选择、空态、目标、固定/单项选一 | 逐档编辑与删除、3项选择奖励分支 | 保留按模式的目标/单位/短规则，以及奖励资源加载重试；无可删实现说明。 |
| components/operations/activity-options.vue | 排名、商店时段、餐厅、返钻、连消、探索、转盘、折扣全部独立分支 | 日内时间/当天结束、增删金币/折扣等二级输入 | 固定规则去原版字样；所有零值语义、北京时间、钻石/权重/概率约束、独立角色记录等有价值规则保留。 |
| components/operations/activity-prizes.vue | 九奖位导航、库存、概率和递增保底 | 逐奖位编辑、TurntableProbability入口 | 保留余概率承接、缺货暂停与保底兑现后果，全部影响配置；导航已有窄屏单列及长名称换行。 |
| components/operations/activity-products.vue | 商品列表选择、空态、获得/消耗 | 购买限制/资格/刷新折叠、资格开关、增删商品 | 各分支为真实购买条件；保留每日/累计/库存/折扣/资源以及关闭资格时清空要求的现有行为。无来源介绍。 |
| components/operations/activity-summary.vue | 活动、商品/兑换、里程碑、王者/转盘、固定/额外规则完整摘要与无表单空态 | 作为活动详情/预览/检查配置的共享内容 | 删除开发source的汇总输出及空CSS；编辑中的运营备注字段与API source保留。固定规则去原版字样，长奖项标题允许wrap。价格/资格/库存/概率/单位全部保留。 |
| components/operations/activity-template-workspace.vue | 全部/可用性模板表、筛选空态、读/写/复制/新建 | 基本/内容/设置/检查分区、删除/放弃模板、排期内嵌编辑 | render操作接公共class，保存栏直接显示用于新排期及已发布活动不变；参与方式、关闭新增排期后果、开服14档/249任务与招募分支保留。 |
| components/operations/activity-workspace.vue | 可用/不可用活动列表、筛选、读/写/复制/新建 | 各基本/内容/时间/规则/检查分区、更多启停操作、删除/放弃/离开确认 | render操作容器接公共ops-table-actions；审查动态可用性原因/模式规则，保留冻结与复制新期、历史和未保存草稿后果。 |
| components/operations/guild-prayer-editor.vue | 祈祷奖励、空配置、概率/加成、解析/校验错误 | 增删属性与输入 | 百分比与相对权重规则、原值保留/不支持结构错误是必要保护；已有窄屏两列及380以下单列，无需删改。 |
| components/operations/guild-rewards-workspace.vue | 战后/周/月奖励列表、版本/发布时间、只读 | 新建/编辑奖励抽屉、战后任务/奖励、周/月排名/称号、关闭确认 | 逐分支保留未来期可编辑、结算锁定、发布依据输入、单项保存和放弃草稿后果；费用/奖励/日期/状态均为真实数据。 |
| components/operations/guild-tech-directory-editor.vue | 初始等级、八项属性归属、空配置/错误 | 逐属性所属建筑选择 | 每项只能一座建筑、两座都需有属性及初始0为真实规则；解析失败保留原值，维持。 |
| components/operations/native-activity-workspace.vue | 常驻与精彩活动全列表、过滤/空态、类型/时间/启用/冻结 | 签到双分支、基金、首充/单充/累计/连充/消费、月/终身卡、会员福利、副本福利、橙卡招募、庆典/节日/新版子活动、移除/放弃确认 | 逐类型整理note的开发措辞，所有生效/冻结/金额/订单/已领边界保留到保存前可见段落；未知类型note原样保留。修正native-kind-period deep样式，操作接公共class。 |
| components/operations/number-sequence-editor.vue | 数值序列、解析/范围/合计校验 | 重配确认、增删次数 | 原值保留和替换后全部重填直接关系数据，保留；字段单位/误差与窄屏分行不变。 |
| components/operations/personal-opening-activities.vue | 区服、活动连续列表、无区服/失败/空列表 | 个人活动抽屉、奖励档与关闭/切服/放弃确认 | 创建日起算、之后新实例生效与区服归属均保留；无冗余介绍，窄屏已wrap且长身份允许换行。 |
| components/operations/recruit-settings.vue | 招募规则、加载/失败/未载入、每日次数/保底/幸运值 | 刷新与放弃规则确认、具名规则帮助 | 保存后影响后续抽取/进度保留从长帮助中拆到字段区直接可见；幸运值来源改常驻池，保留跨世界共享、0不限、10倍数/10或20保底、免费占次和不保证红卡。 |
| components/operations/reward-list.vue | 资源、数量、逐行移除及添加上限 | 资源选择器为外部组件；无自身弹层 | 绑定、数量下限、上限、禁用与选择器完整；无开发文案可删，使用公共奖励行样式。 |
| components/operations/reward-summary.vue | 奖励名称/图片/品质/数量、空态、名称失败 | 无自身编辑或弹层 | 实际资源ID为失败兜底识别信息；保留加载失败与数量，不删除ID。已有wrap/长名称换行。 |
| components/operations/shop-workspace.vue | 所有商店列表、资格/限购/成本/数量、营业/无会员礼包/资源失败 | 商品读/写/新建、数量档、限制/状态分区、商店设置模态、删除/放弃/离开确认 | 操作接公共class；会员礼包帮助去shopType等实现术语；生效/订单/已用库存与限购记录移到保存状态槽。11商店条件分支逐项保留，详见下表。 |
| views/achievements/index.vue | 条件目录、全部状态列表、创建/只读/编辑、缺玩法/错误分支 | 目标/奖励/状态分区、条件帮助、删除及离开确认 | 逐ID整理开发notes为运营规则；41个当前目录条件+历史培养条件有明确映射，未知ID保留API notes。进度记录帮助保留缺失历史不补计；生效后果移到全部编辑分区均可见的保存状态槽。 |
| views/activities-unavailable/index.vue | 旧书签加载状态 | 重定向到活动分类 | 只有加载状态与保留query的跳转，无可删开发文案；不改变路由合同。 |
| views/activities/index.vue | 常驻/个人/排期模板三页签 | 各页签beforeLeave与其子工作区 | 入口无重复介绍，保持页签草稿关闭保护；实际分支审查记录见各工作区。 |
| views/activity-schedules/index.vue | 新玩家/定期排期、方案/活动库、日历/清单、未选区服/冲突/首次接管 | 归档恢复、排期抽屉、方案编辑/预览、活动详情/内嵌模板、发布/归档/离开确认 | 空态改真实暂无或筛选为空，删除左侧等位置指示；保留首次发布关闭未排期活动、结束日不含、冻结、恢复停用、宽限期和玩家起算规则。 |
| views/arena-prizes/index.vue | 每日排名、常规金币、额外池、失败/冲突/空奖励 | 额外奖励池折叠、奖品增删 | 规则均涉及概率、时间、冻结、参与资格或历史保留；保留。原布局已有窄屏单列/奖品分行，未发现开发来源输出。 |
| views/daily-sign/index.vue | 月份、历史/未来/默认状态、天数表、失败/资源重试 | 单天奖励模态、月份切换/放弃修改确认 | 签到帮助不再直接显示数据库source，保留月累计/每日一档/历史记录规则；资源错误行加wrap。编辑VIP0含义与生效规则保留。 |
| views/field-resources/index.vue | 野外Boss每日次数、无限/零次、读取失败 | 直接编辑；无二级弹层 | 次数共享、世界Boss独立、零次仍可战斗及新开战生效均影响选择，保留；按钮已有wrap及窄屏适配。 |
| views/fields/combat-editor.vue | 角色身份、等级/阵位及全部属性 | 嵌入成员编辑；无弹层 | 修正dark祖先的scoped选择器；等级提示允许换行，窄屏属性单列，长提示不再挤出边界；数值范围、模型身份和等级可编辑条件保留。 |
| views/fields/drop-editor.vue | 资源选择、数量档、权重、增删 | 嵌入掉落编辑；无弹层 | 保留每种道具独立抽取、数量0不掉落与32项上限；已有wrap/max-width，无新增文案问题。 |
| views/fields/field-map.vue | 图例/缩放、地图空态、候选点选择 | 选中点详情与世界首领说明 | 删原地图投影实现语言，保留非实时与同周期仅一只首领；原点位改点位编号，保留ID、坐标及候选资源。 |
| views/fields/ggo-config-editor.vue | 规则缺失、入口/终端、死亡/撤离、交易/离开、小怪/Boss、芯片、经济/周限购、NPC全分支 | 怪物/Boss/NPC折叠、撤离台词二级折叠 | 删除迁移版本术语、测试初值/样式说明、重复掉落池/职责文字；保留未接入/未生效、当前1信用点每日1次商品事实、全部待定选项/占位符/死亡损失和结算边界。清理弃用样式。 |
| views/fields/index.vue | 资源方案、分布预览、区服开放/分线，全部规则/采集/怪物/普通及世界首领分支 | 删除方案/发布确认、搜打撤编辑、子编辑器 | 删除普通首领与只读地图的功能复述，简化原等级系数与全服时段用词；保留关闭后果、周期冻结、分线关闭限制、缺场景资源与重新发布要求。 |
| views/fields/point-editor.vue | 候选点、坐标、刷新资源与权重 | 每个候选点折叠分支 | 名称/坐标为识别信息；无开发复述。保持已有窄屏分行及选择器图标、刷新资源的增删条件。 |
| views/guild-settings/index.vue | 参数分组、商店入口、公会规则、祈祷/科技、捐献、公会战、等级/科技表 | 来源帮助、场景备注折叠、公会奖励工作区 | 删除公会开发source输出，场景source可编辑记录改称配置备注；指挥名称去原字。保留已开始场次规则、所有费用与发布缺项/只读/非法输入提示。 |
| views/shops/ggo/index.vue | ShopWorkspace入口、所属商店列表/空态 | 共享商品详情/编辑/新建/商店设置/删除确认 | 入口自身无介绍文字；已审查shop-id=ggo的独立分支，保留信用点、刷新固定数量档/成本、每格独立、报价与进度保留。共享改动见shop-workspace行。 |
| views/shops/guild/index.vue | ShopWorkspace入口、所属商店列表/空态 | 共享商品详情/编辑/新建/商店设置/删除确认 | 入口自身无介绍文字；已审查shop-id=guild的独立分支，保留公会贡献成本、最低公会等级、购买资格。共享改动见shop-workspace行。 |
| views/shops/memory/index.vue | ShopWorkspace入口、所属商店列表/空态 | 共享商品详情/编辑/新建/商店设置/删除确认 | 入口自身无介绍文字；已审查shop-id=memory的独立分支，保留固定6位置/1～4碎片、材料重复、刷新权重/数量档、道具刷新优先。共享改动见shop-workspace行。 |
| views/shops/ordinary/index.vue | ShopWorkspace入口、所属商店列表/空态 | 共享商品详情/编辑/新建/商店设置/删除确认 | 入口自身无介绍文字；已审查shop-id=ordinary的独立分支，保留普通消耗/每日/累计/库存/等级VIP、阶梯价格在明确修改前保留。共享改动见shop-workspace行。 |
| views/shops/pet-market/index.vue | ShopWorkspace入口、所属商店列表/空态 | 共享商品详情/编辑/新建/商店设置/删除确认 | 入口自身无介绍文字；已审查shop-id=pet-market的独立分支，保留周库存1～100、固定6位置/碎片材料/独立每格、数量档及刷新。共享改动见shop-workspace行。 |
| views/shops/point/index.vue | ShopWorkspace入口、所属商店列表/空态 | 共享商品详情/编辑/新建/商店设置/删除确认 | 入口自身无介绍文字；已审查shop-id=point的独立分支，保留攻略点成本与通用资格/营业时间。共享改动见shop-workspace行。 |
| views/shops/popularity-rank/index.vue | ShopWorkspace入口、所属商店列表/空态 | 共享商品详情/编辑/新建/商店设置/删除确认 | 入口自身无介绍文字；已审查shop-id=popularity-rank的独立分支，保留历史最高排名门槛、每日/整期各一次、只读配置备注。共享改动见shop-workspace行。 |
| views/shops/popularity/index.vue | ShopWorkspace入口、所属商店列表/空态 | 共享商品详情/编辑/新建/商店设置/删除确认 | 入口自身无介绍文字；已审查shop-id=popularity的独立分支，保留声望成本与商品/奖励页签导航。共享改动见shop-workspace行。 |
| views/shops/score/index.vue | ShopWorkspace入口、所属商店列表/空态 | 共享商品详情/编辑/新建/商店设置/删除确认 | 入口自身无介绍文字；已审查shop-id=score的独立分支，保留积分消耗、通用限购/营业。共享改动见shop-workspace行。 |
| views/shops/union-fight/index.vue | ShopWorkspace入口、所属商店列表/空态 | 共享商品详情/编辑/新建/商店设置/删除确认 | 入口自身无介绍文字；已审查shop-id=union-fight的独立分支，保留报价日期、刷新分钟、贡献职务资格、当前场次后续购买/已用库存。共享改动见shop-workspace行。 |
| views/shops/vip/index.vue | ShopWorkspace入口、所属商店列表/空态 | 共享商品详情/编辑/新建/商店设置/删除确认 | 入口自身无介绍文字；已审查shop-id=vip的独立分支，保留会员等级/价格/奖励、无开放商品提示、真实每礼包已领次数保留。共享改动见shop-workspace行。 |
| views/sprout-settings/index.vue | 标识预览、启停、天数、只读/失败状态 | 直接编辑；无二级弹层 | 保留全区服范围、有效天数和客户端尚未接入的真实限制；无功能复述或开发来源可删，原响应式样式保留。 |
| views/titles/index.vue | 图示/获得条件/状态列表、只读/编辑 | 条件/属性/状态分区、图片预览、编号帮助、离开确认 | 修正render生成title-list-art的deep选择器；编号帮助只留编号，去重复规则；属性/期限生效规则移到保存状态槽。关闭新资格及赛季只读限制保留。 |
| views/trials/index.vue | 状态、规则、曲线、阵容、三难度预览及读取失败空态 | 直接编辑、预览；无独立详情弹层 | 删除source与算法说明及空样式；防御改为物防与法防；关闭普通试炼后果贴近开关；旧首组和生效规则合并到保存前。保留精英边界、实际层数/版本、权限与脏草稿。 |

## 动态条件与说明的核对依据

- 成就映射绑定只读源码 `hyjs-server/service/adminGameContentService.go:23-67` 的 AdminAchievementKinds 与 `hyjs-server/constUtil/reAchievement.go` 的明确ID；21个基础条件、15个觉醒条件、5个新增成长条件共41个目录ID，另有 `warrior_training` 历史条件。
- 基础条件逐ID保留等级/装备人数/星数区别、四人同时达标、远征整小时及任务时长封顶/不乘人数、仅本人首领击杀、成功帮助去重、精灵生命周期去重/历史缺失不补、相同模型元素分组、单只已解锁天赋最大值、亲密星级/经验区别、招募引导首抽不计。觉醒只 `awake-1-1` 可用，保留4箱各5分+怪物80分；其余14个明确ID保留暂不能启用及100%目标，选择项与保存禁用机制未改。
- 未识别条件ID的 `row.notes` 原样兜底，未对任意动态字符串做关键词删除；API字段与数据未改。新增条件需要后续逐ID审校，当前兜底优先保留真实信息。
- 成就顶部开发范围介绍被删除；其中“缺失历史不补计”保留为具名进度帮助；已查看目标/奖励/领取记录保留移到实际保存栏。
- 常驻活动note逐类型核对 `adminNativeActivityService.go:262-491`；去除SKU/P1/消费者/原表/域名等实现措辞，保留各类金额单位、初次参与/订单冻结、启停和别名限制、父子活动同步及完整后启用。未知类型继续使用原API note。
- 商店只对已核对的vip说明使用具体短文；其余仍取服务端规则，已核 `adminShopsService.go:20-67`，费用/库存/资格/报价冻结/营业时间均保留。
- 签到source是多行数据库来源拼接（`adminMonthSignService.go:47`），不当作签到规则展示；月累计、每日一档及已领记录保留为明确文案。试炼source只移除输出，未移除接口字段。
- ActivitySummary不再汇总输出source开发来源；所有实际运营备注输入、值和保存接口保留。

## 实际验证与交接

- 对范围内45个Vue完成 `@vue/compiler-sfc` parse、compileScript、compileTemplate，并用TypeScript transpileModule检查语法：45文件、0失败。最后一次实施完成后重跑通过。
- `git diff --check` 通过；未运行全量typecheck/build，未启动任何服务，未操作浏览器/数据库/Docker/手机。
- 文件写入仅限brief授权Vue及本报告；工作树其他修改属于主代理/并行执行者，未覆盖或清理。未提交、推送、发布或清理文件。
- 公共依赖：4个工作区的render操作使用主代理约定 `.ops-table-actions`；主代理负责公共CSS最终集成。
- 主代理重点运行验收：试炼开关后果与长保存规则、320/390曲线横滚和阵容分行；GGO所有折叠与待定/零值；成就条件目录/已禁用探索/未知ID说明；全部固定活动编辑分支及长保存规则；11商店商品和设置弹层、金额/库存/报价不变；称号图片样式、地图成员dark/长提示。
- 实际交互与视口前后对照均未执行，不能据静态检查称已完成运行验收。

## 追加修正：编辑器中的原始来源字段（冻结交接）

主代理QA后追加只读核查发现：删除摘要/帮助中的来源仍不充分，原textarea直接绑定Source，技术来源仍可出现在常规编辑表单。本次按主代理批准的最小范围移除6处展示，保留所有内部合同字段、原值、默认值、版本及提交逻辑。未对用户输入做关键词判断，未将技术来源移入帮助。

| 字段路径及所属分支 | 本次展示处理 | 创建/更新payload保持依据 |
|---|---|---|
| activity-workspace.vue / form.source / 时间分区 | 移除原“运营备注”Source输入 | 新建继续newActivityForm既有“用户运营配置”默认；更新仍克隆完整Form；save仍展开form.value提交，Source键和值未改；后端原长度限制保留。 |
| activity-template-workspace.vue / form.source / 基础分区 | 移除原“运营备注”Source输入 | 新建继续newActivityForm默认；更新仍克隆row.form，save仍form:{...form.value,...原有时间归零}，原Source与版本不变。 |
| achievements/index.vue / draft.source / 状态分区 | 移除Source输入，“状态与备注”改“状态” | 新建fresh()原“用户运营配置”不变；更新仍JSON克隆行；save仍展开draft.value，后端Source非空合同保持。修改原因/审计机制未变。 |
| shop-workspace.vue / draft.source / VIP或公会战商品状态分区 | 移除Source输入，“状态与备注”改“状态” | newShopProduct原“用户运营配置”不变；更新仍合并并克隆原商品；save仍展开draft.value及原版本，VIP/公会战Source必填合同保持。 |
| shop-workspace.vue / settingsDraft.source / 商店设置模态 | 移除Source输入，含原不可编辑的声望排名来源 | configure仍克隆原settings，source仍按原逻辑使用已有值或“用户运营配置”；新期/更新均展开settingsDraft及原版本提交。前后端必填校验、声望排名强制来源合同不变。 |
| guild-settings/index.vue / form.policies.unionFight.field.source / 场景分区 | 移除整个来源输入折叠，不留空壳 | form仍保留完整policies；save仍clone(form.value.policies)提交，原source及非空/1024长度合同不变。原变更原因字段继续保留。 |
| guild-rewards-workspace.vue / draft.source / 战后、周、月奖励发布依据 | **保留，不修改** | 主代理明确决定保留真正的业务发布依据：新建默认空且后端强制非空；已有依据也应继续可编辑。未取得当前数据库内容，不据技术常量推断已有依据属于开发来源。save仍展开draft并携带原修改原因。 |

### 来源证据与边界

- 本次未请求实时API、未访问数据库；后端实际构造可证明此类默认值会进入Source，不能证明某条当前数据库记录仍是默认值。
- 成就：adminGameContentService.go:70直接投影row.Source；Re默认来自constUtil/reAchievement.go:3，含OfflineEvents315/AchievementPlans306。
- 活动：adminActivityModesService.go:356复制数据库Source；adminActivityScheduleService.go:60开服模板明确含APK来源；普通月份源见p4CommerceService.go:134。
- 商店：adminShopsService.go:134/146/181/195投影Source；VIP6源见constUtil/p3VIP6Growth.go:5；声望排名Source严格常量合同见p4PopularityRankService.go:41和adminShopsService.go:674；不能清空或把它当可任意改写的文案。
- 公会场景：adminGuildService.go:590投影、894校验、908保存；默认来源含SceneType/ServerType/Pose等，见constUtil/p13UnionFightField.go:14。
- 公会奖励：adminGuildRewardsHandler.go:38要求Source非空且不超过2048；guild-rewards.ts:38新建默认空。因此保留业务输入，不改变其编辑能力。
- 所有Source仍留在draft/API中，内部来源、审计与供应识别合同保持；本次调整原因继续走原有reason/变更原因审计字段。

### 本次检查与冻结

- 实际改变5个Vue文件、移除6个来源控件；第7个业务发布依据按主代理决定保留。没有服务端、共享组件、接口或默认来源改动。
- 5个受影响Vue的SFC解析、compileScript和compileTemplate通过；git diff --check通过。
- 未运行全量typecheck/build、未启动服务、未提交/推送；由主代理更新固定候选并重验受影响编辑器。
- 本代理已停止修改源码，交接冻结。

## 第7项复核修订：已确认的内部结算发布标记

- 新证据：主代理隔离SQLite的实际 `/admin/api/guild-rewards` 返回4条settlement记录，Source均精确等于后端 `P13UnionFightSettlementSource` 完整固定英文串。该证据补齐此前“尚未确认当前数据”的缺口，因此修订之前保留所有发布依据输入的决定。
- 本次只修改 `guild-rewards-workspace.vue`：仅对已有settlement且Source完整精确相等的记录隐藏发布依据textarea，并在保存按钮前直接显示中文结算条件。新建永远保留原必填输入；其他用户说明、未知Source及其他类型保持可编辑。没有前缀、关键词或模糊过滤。
- `draft.source`、克隆、dirty序列化、original快照、saveGuildReward展开payload、rowVersion及reason全部原样保留；显示条件独立computed，不参与业务状态修改。精确技术标记仅在脚本中用于识别，没有放进帮助、可访问文本或界面。
- 中文规则依据：`p13UnionFightSettlementService.go:200-317` 仅结束后处理，要求冻结双方总战力合法且不同，无战斗/审计/击杀死亡/生命变化，双方有实际入场者，较低冻结报名总战力方获胜；`:330` 固定比赛当日北京时间23:30；`:430-443` 到时通过EnqueueReward投递。`:235-239/259-260/277/311-313` 和相关test的missing-side/withdrawn/equal-power覆盖未确定的分支；中文明确同战力、撤回或一方缺席暂不结算，没有宣称完整战斗胜负已支持。
- 最终检查仅对该文件做SFC parse、compileScript、compileTemplate，全部通过。未运行浏览器、构建、服务、数据库或提交；已停止修改，交主代理更新冻结并继续实测。

## 活动模式动态帮助复验修正

- 主代理本机GET `/admin/api/activity-modes` 实测多个模式复用固定notes，copy-win/login-total等错误出现充值目标单位。本次只改展示：`activity-operations.ts` 新增纯函数formatActivityModeNotes，activity-workspace与activity-template-workspace两个具名模式帮助共同使用。API Mode对象、notes字段、默认Form、payload与保存逻辑均未修改。
- 精确基础串：`只修改今后未冻结的新一期；空奖品草稿保持关闭。`。仅对已核实的10个默认模式 exchange、discount-shop、half-price、limited-shop、mystic-shop、login-rewards、limited-sign、stamina-meal、diamond-rebate、treasure-box，完整相等时显示“未配置奖励的草稿保持关闭。”；保存范围已在现有提交区域说明，不重复。
- 精确周期串为上述基础串加 ` 每实例支持最多31档独立目标；累计充值目标单位为分。`。仅对adminActivityDefinitions绑定period域的14个模式 recharge-total、single-spend、work、diamond-spent、recruit-count、copy-win、arena-wins、plunder-count、wanted-attempts、wanted-kills、hero-collection、trial-stars、power-race、login-total，完整相等时显示最多31档独立目标和未配置奖励草稿关闭。仅recharge-total附“充值目标以人民币分填写”。
- 模式合同依据：`adminActivityModesService.go:45` recharge-total绑定payment_amount；`paymentService.go:421` 真实确认支付记录cents。single-spend虽当前名称为单充送礼，实际`:46`绑定P1EventDiamondSpent；`eventService.go:277`按实际钻石减少量记事件。`activity-operations.ts`原milestoneTargets也明确single-spend为累计消费钻石/钻。本次保留原字段标签与所有其他模式单位，不按模式名称猜单位，不扩大为产品改名。
- trial-stars/power-race等已有专属notes不等于这两类固定串，原样保留；未知模式、运营自定义notes、仅附加一段内容或空格的notes均原样保留。未使用trim、前缀或关键词过滤。
- 检查：两个调用Vue完成SFC解析/compileScript/compileTemplate；活动service通过TypeScript transpileModule局部语法检查；本机内存执行纯helper验证14周期模式、10基础模式、自定义/未知/输入不变的边界均通过。未做全量typecheck、构建、服务或浏览器操作，未提交。实现已停止，交主代理冻结和实测帮助。
