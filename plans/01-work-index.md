# 任务索引与功能归属

当前排期以[单人养成优先计划](06-growth-first-roadmap.md)为准：P2/P3/P4/P5和必要P1/P6/P7/P8/P9/P15优先服务抽卡、礼包、培养与资源循环；P14跨服及其它多人附加范围后置。下方编号保留功能所有权，不作为顺序开发要求。

本索引用于定位文件；状态和证据只在各期文件维护。范围依据 [原聊天盘点](00-baseline.md)，已有实现的任务采用差量补齐与验收。

## 执行批次索引

| 任务 | 期文件 | 功能范围 |
| --- | --- | --- |
| P0-A | [首个可持续游玩的 Demo](P0-demo.md) | **版本与体验入口**：记录运行的服务端/客户端版本、数据来源、设备与连接方式；提供可复现启动和重登步骤。无线访问优先核对现有配置与协议地址，确需改包再单独实施。 |
| P0-B | [首个可持续游玩的 Demo](P0-demo.md) | **Demo账号与进度路线**：保留原账号作为历史回归对象；独立测试账号采用显式测试初始化，有限资源、原等级门槛、正常消耗。分别准备新手循环和高等级功能验收场景。 |
| P0-C | [首个可持续游玩的 Demo](P0-demo.md) | **基础循环实机贯通**：按当前已实机通过关卡范围，验证重复战斗、奖励、招募、上阵、穿戴与单次/五次强化；补齐体验阻断问题。 |
| P0-D | [首个可持续游玩的 Demo](P0-demo.md) | **每日资源与已有玩法验收**：验证真实日常事件、任务奖/宝箱、资源恢复、签到、累计登录；独立账号验证已实现资源副本、扫荡和高等级养成入口。 |
| P0-E | [首个可持续游玩的 Demo](P0-demo.md) | **Demo发布收口**：处理本期入口红点、提示、引导断点、失败退出与重连；记录可玩入口、实际开放范围、阻断清单、版本和证据。 |
| P1-A | [奖励、事件与周期公共能力](P1-rewards-events.md) | **资源与奖励发放边界**：梳理现有金币/钻石/体力/耐力、代币、道具与实例发放；明确定义每类资源来源、溢出、失败回滚和回执范围。 |
| P1-B | [奖励、事件与周期公共能力](P1-rewards-events.md) | **邮件与领奖中心**：实现列表、未读、通知、附件、领取、删除、过期和离线保存；业务发奖使用可追溯来源键。 |
| P1-C | [奖励、事件与周期公共能力](P1-rewards-events.md) | **实际事件与多活动实例**：记录登录日、招募、成功养成、购买、真实消费、挑战等事件；定义累计/当天/单次口径、实例窗口和冻结的领奖规则。 |
| P1-D | [奖励、事件与周期公共能力](P1-rewards-events.md) | **周期、排行与恢复**：统一周期ID、北京时间日切、领奖截止、重启恢复、排行并列规则和截止快照；玩法分别定义自己的指标。 |
| P1-E | [奖励、事件与周期公共能力](P1-rewards-events.md) | **运维最小能力**：提供可查的版本、运行状态、迁移/备份/恢复步骤、活动实例与奖励记录查询；具备有限的测试场景准备能力。 |
| P2-A | [红卡与红装备优先体验](P2-red-heroes-equipment.md) | **红卡/红装身份与获取规则**：从原表、色阶、资质、客户端界面/IL核实红卡、红武及其他红装备ID，区分王者招募、转盘、选择箱、兑换与神装礼包；登记奖池/概率/缺失规则。 |
| P2-B | [红卡与红装备优先体验](P2-red-heroes-equipment.md) | **红卡获取**：王者招募单抽/十连、预览、免费冷却/日限制、轮换池、幸运值/幸运奖励；红卡整卡/碎片选择箱、碎片合成/出售及王者之魂兑换。 |
| P2-C | [红卡与红装备优先体验](P2-red-heroes-equipment.md) | **红装备获取**：核实并恢复红武转盘入口、奖池、周期、价格、库存、记录及稀有兑换；审计其他红装备真实来源，接入明确可用渠道。 |
| P2-D | [红卡与红装备优先体验](P2-red-heroes-equipment.md) | **使用与培养**：红卡上阵/换位/经验成长/现有天赋与高等级培养入口；红装穿戴/卸下/强化/精炼/升星/锻造及可用回收；战力、属性与现有单人战斗。 |
| P2-E | [红卡与红装备优先体验](P2-red-heroes-equipment.md) | **早期体验版本与支付衔接**：使用隔离Demo账号核对红卡/红装获得→培养→战斗→重登；记录有限初始资源与后续来源；给P3明确商品、支付到账奖励和会员资格接点。 |
| P3-A | [订单、会员与支付活动](P3-membership-payments.md) | **商品与订单事实**：商品档位、订单生成、支付确认来源、幂等到账、赠钻、失败/超时/退款相关资格规则审计；测试与真实支付标识。 |
| P3-B | [订单、会员与支付活动](P3-membership-payments.md) | **VIP与会员卡**：VIP成长/特权、每日福利/每周礼包/会员商店、容量/购买次数；月卡剩余日与终身卡永久资格、每日领取。 |
| P3-C | [订单、会员与支付活动](P3-membership-payments.md) | **支付条件活动**：首充、单充、累计充值、连充、充值双倍、豪华签到；活动窗口/单笔/累计口径与最终奖。 |
| P3-D | [订单、会员与支付活动](P3-membership-payments.md) | **限时特惠与神装礼包**：触发、倒计时、原价/折扣、购买与发奖；章节触发的神装礼包使用原表；礼包码/激活码次数与领奖。 红卡/红装付费商品与P2实际获取链路衔接；不以付费界面展示替代到账及奖励。 |
| P3-E | [订单、会员与支付活动](P3-membership-payments.md) | **消费活动与配置衔接**：回填消费返利、VIP签到补差、基金购买、会员商店及14天支付条件；审计周期/资格变更后的历史奖励。 与P2红卡/红装来源联合实机验收，涉及P4未来活动实例的子项保留联动任务。 |
| P4-A | [招募、商店与非支付活动](P4-recruit-shops-activities.md) | **招募与抽取活动**：复用P2王者招募、红卡来源、红武转盘与预览；补普通/强者卡池配置、整体轮换及非红装转盘运营；藏宝箱/招财猫冷却与探索。 |
| P4-B | [招募、商店与非支付活动](P4-recruit-shops-activities.md) | **商店与稀有兑换**：普通商店完整价格/限购规则、回忆圣坛刷新、声望/积分/试炼商店通用交易能力；复用P2王者之魂/红装兑换，补神装精华等其他兑换；半价、折扣、限购。 |
| P4-C | [招募、商店与非支付活动](P4-recruit-shops-activities.md) | **签到与体力福利**：独立登录签到、限时签到、月光餐厅定时领体力；回归月签到、累计登录、基金和全民福利；审计奖励档/领奖宽限。 |
| P4-D | [招募、商店与非支付活动](P4-recruit-shops-activities.md) | **活动条件与排名**：开服战力竞赛；招募送礼/橙卡概率提升/收集角色、消费累计/返利、主线/攻略/关卡福利、通用兑换/抢购、神秘小屋折扣。 |
| P4-E | [招募、商店与非支付活动](P4-recruit-shops-activities.md) | **活动类型登记与联动**：登记基线中的36种活动类型，对庆典/节日/新版/开服狂欢及AincradStars等逐项记录协议、规则、参数和归属；竞技/夺宝/悬赏送礼预留真实事件联动。 |
| P5-A | [角色、阵容、物品与回收闭环](P5-growth-inventory.md) | **物品与碎片**：复用P2红卡整卡/碎片选择箱与合成，补其他固定/随机/选择礼包、普通角色碎片查询/获得/合成/出售、完整背包容量与批量出售；与现有饰品箱衔接。 |
| P5-B | [角色、阵容、物品与回收闭环](P5-growth-inventory.md) | **队伍关系与编排**：缘分/羁绊、助阵位、伙伴祝福、技能发动顺序；完成等级开放、上/下阵、保存及属性作用域。 |
| P5-C | [角色、阵容、物品与回收闭环](P5-growth-inventory.md) | **主角成长**：世界之种/潜能解放、品质路线、更换主角；先核实原配置和存档归属，再接消耗、奖励和身份切换。 |
| P5-D | [角色、阵容、物品与回收闭环](P5-growth-inventory.md) | **回收与便捷培养**：角色/装备/饰品出售、分解、重置、重生、培养投入返还；单件和全队一键强化。 |
| P5-E | [角色、阵容、物品与回收闭环](P5-growth-inventory.md) | **收集、称号与已有养成收尾**：图鉴、成就累计及领奖、称号获得/佩戴/更换/有效期；训练、突破、觉醒、融合、专精、装备/饰品进阶、宝石/料理逐项实机。 |
| P6-A | [完整服务端战斗与录像公共能力](P6-combat.md) | **规则与对照样例**：审计原IL/表格的回合次序、技能、随机调用、状态与结束条件；保存有限可复现样例及预期，不批量装入截图历史。 |
| P6-B | [完整服务端战斗与录像公共能力](P6-combat.md) | **整场基础运行**：服务端冻结阵容/配置版本/seed；实现目标选择、移动/站位作用、AI、回合、普通/终极技能、怒气、冷却、连击和胜败。 |
| P6-C | [完整服务端战斗与录像公共能力](P6-combat.md) | **状态与组合效果**：Buff/减益、盾、特殊态、队友光环、动态天赋、觉醒、装备/饰品触发技能与成长叠加。 |
| P6-D | [完整服务端战斗与录像公共能力](P6-combat.md) | **战斗实例与协议接入**：战斗归属、超时、取消、断线、结果回执、操作记录重演/核验；适配客户端播放快照与先算后播协议。 |
| P6-E | [完整服务端战斗与录像公共能力](P6-combat.md) | **录像及既有PVE回归**：保存双方快照、配置/客户端版本、seed、操作/播放数据与结果；查询/回放；逐步接入既有副本和野外。 |
| P7-A | [可反复玩的单人挑战](P7-solo-challenges.md) | **已有关卡公共规则**：补普通/特殊/装备掉落、星级条件、挑战次数与钻石/VIP重置；手机验证资源副本与扫荡。 |
| P7-B | [可反复玩的单人挑战](P7-solo-challenges.md) | **试炼与迷宫商人**：爬层、星数、加成选择、阶段奖励、重置、扫荡、一键三星及失败商人；交付首组楼层。 |
| P7-C | [可反复玩的单人挑战](P7-solo-challenges.md) | **精英首领**：开放条件、次数购买、战斗、失败、奖励和重置；选首个可用首领。 |
| P7-D | [可反复玩的单人挑战](P7-solo-challenges.md) | **远征/佣兵大厅**：任务刷新、伙伴派遣/占用、耗时、撤回、完成、好运/厄运和奖励；好友帮助接点预留给P10。 |
| P7-E | [可反复玩的单人挑战](P7-solo-challenges.md) | **觉醒之地**：地图探索、战斗、宝箱、道具、复活、完成度、结算和录像；交付首个探索实例。 |
| P8-A | [第一张单人野外与生活技能](P8-field-life-skills.md) | **普通怪与探索恢复**：验证出生点/权重、战斗、伤害、血量/怒气、超时、奖励次数、重生、返城和重登；配置与场景资源检查。 |
| P8-B | [第一张单人野外与生活技能](P8-field-life-skills.md) | **采矿**：资源点、资格、耗时/消耗、采集中状态、完成领取、归属与重生；串通矿石、宝石制作/镶嵌。 |
| P8-C | [第一张单人野外与生活技能](P8-field-life-skills.md) | **钓鱼**：鱼点、采集过程、食材、成长和重生；串通料理制作。 |
| P8-D | [第一张单人野外与生活技能](P8-field-life-skills.md) | **生活技能与生存**：高级技能书获得/使用、配方解锁/等级、料理快捷栏、回血设置、使用限制、倒下/复活费用与状态。 |
| P8-E | [第一张单人野外与生活技能](P8-field-life-skills.md) | **地图功能配置与验收**：将场景、怪物、采集点和奖励来源分别登记；验证原表映射、客户端资源和任务入口；标记NPC巡逻路径用途。 |
| P9-A | [守护精灵全链路](P9-pets.md) | **获得与实例**：首次获得、扭蛋/招募、免费次数、收费抽取、实例/碎片、列表和预览。 |
| P9-B | [守护精灵全链路](P9-pets.md) | **携带与守护**：上阵、替换、跟随/守护效果及技能作用域，真实属性进入战斗。 |
| P9-C | [守护精灵全链路](P9-pets.md) | **成长与亲密度**：升级、喂食、突破、天赋；互动、食物、攻略增长和效果。 |
| P9-D | [守护精灵全链路](P9-pets.md) | **精灵装备与商店**：装备查询、单件/全部强化、进阶、觉醒；专属商店、货币、刷新与限购。 |
| P9-E | [守护精灵全链路](P9-pets.md) | **回收与重登**：出售、碎片出售、分解、重置及返还，携带实例保护与离线恢复。 |
| P10-A | [社交与多人场景](P10-social-scenes.md) | **好友与黑名单**：搜索/推荐/申请/同意/拒绝/删除、资料、黑名单及权限，状态保存。 |
| P10-B | [社交与多人场景](P10-social-scenes.md) | **耐力、人气与远征帮助**：赠送/领取/批量、每日限制/时间、人气点赞/名单/排行/奖励；完成P7远征好友帮助。 |
| P10-C | [社交与多人场景](P10-social-scenes.md) | **聊天与语音**：世界/场景/私聊频道、历史与离线、消息边界和权限；语音资源传输与播放。公会频道权限由P13绑定。 |
| P10-D | [社交与多人场景](P10-social-scenes.md) | **同屏、安全区与分线**：场景玩家列表、移动广播、线路列表/换线、安全区、实例隔离、断线/重登和共享对象版本。 |
| P10-E | [社交与多人场景](P10-social-scenes.md) | **福袋与烟花**：使用/投放、附近玩家资格、抢取/过期、次数、奖励、播放与持久化。 |
| P11-A | [竞技、夺宝与奖杯争夺](P11-competition.md) | **天梯竞技与好友切磋**：真实对手/匹配、阵容信息、次数/购买、挑战、排名变化、奖励、录像；好友切磋。 |
| P11-B | [竞技、夺宝与奖杯争夺](P11-competition.md) | **限时竞技**：活动周期、匹配、次数、积分、每日排名及阶段奖励、过期和历史。 |
| P11-C | [竞技、夺宝与奖杯争夺](P11-competition.md) | **夺宝与声望兑换**：碎片目标、对手刷新、抢夺资格、免战、扫荡及合成；完成声望商店实际货币来源/兑换。 |
| P11-D | [竞技、夺宝与奖杯争夺](P11-competition.md) | **奖杯争夺**：低中高级场、挑战/占领/放弃、冷却、次数购买；占领随机奖励、累计时长、日切和排行。 |
| P11-E | [竞技、夺宝与奖杯争夺](P11-competition.md) | **经济与活动联动**：竞技/夺宝送礼、14天相关条件、领奖中心、录像历史和对应商店验收；回填P4联动项。 |
| P12-A | [共享首领、悬赏与野外 PK](P12-bosses-field-pvp.md) | **首领实例与参与记录**：实例ID/配置/血量/位置/版本/出生/结束；参与、伤害、次数、增益、领奖资格/回执。 |
| P12-B | [共享首领、悬赏与野外 PK](P12-bosses-field-pvp.md) | **野外与世界首领**：野外出生/重生、共享血量、攻击/击杀奖励；世界首领活动时间、增益购买、实际战斗、伤害排行与领奖窗口。 |
| P12-C | [共享首领、悬赏与野外 PK](P12-bosses-field-pvp.md) | **悬赏任务与首领**：发现/分享/参与、攻击/击杀、次数、伤害/表现排行、奖励；积分来源与积分商店。 |
| P12-D | [共享首领、悬赏与野外 PK](P12-bosses-field-pvp.md) | **野外 PK**：在线/场景/线路/安全区/距离资格、双方快照、真实胜败、金币等资源转移、日限额和录像。 |
| P12-E | [共享首领、悬赏与野外 PK](P12-bosses-field-pvp.md) | **红名、监狱与联动**：击杀累计、红名、入狱、在线/离线服刑、赎出和权限；悬赏送礼、14天任务及P4/P8回填。 |
| P13-A | [公会、组队与公会战](P13-guild-team.md) | **公会管理**：创建/搜索/申请/审批、成员与职位、转让、退出/解散、公告、旗帜、频道权限。 |
| P13-B | [公会、组队与公会战](P13-guild-team.md) | **公会经济与福利**：捐献、金币/徽章/活跃度/等级、仓库/捐献箱；雕像/守护树/教堂、升级/祈祷及加成；商店/体力/月活/职位奖励。 |
| P13-C | [公会、组队与公会战](P13-guild-team.md) | **组队与副本接入**：创建/查询/加入/退出/邀请、队长转让/踢人/解散、准备/开始、伙伴队伍、断线重连、共享战斗/奖励。 |
| P13-D | [公会、组队与公会战](P13-guild-team.md) | **公会战报名与匹配**：宣战/自动宣战、成员报名、首发/替补调整、阵容/道具、冻结名单、匹配与分组。 |
| P13-E | [公会、组队与公会战](P13-guild-team.md) | **公会战地图、指挥与结算**：采集/上交、强化建筑、工厂/战车/据点/传送/复活；标记/任务/焦点/自动战斗；胜败/周排行/奖励/录像。 |
| P14-A | [跨服、赛季与竞猜](P14-cross-server-seasons.md) | **跨服身份与通信**：区服与玩家联合身份、阵容/角色快照上传、路由与匹配、跨服配置版本和断线恢复。 |
| P14-B | [跨服、赛季与竞猜](P14-cross-server-seasons.md) | **巅峰赛完整赛程**：报名、资格、海选、分组/晋级、淘汰、决赛、赛季历史与奖励。 |
| P14-C | [跨服、赛季与竞猜](P14-cross-server-seasons.md) | **竞猜**：单场/冠军竞猜、投入、截止、记录、取消比赛规则审计和奖励。 |
| P14-D | [跨服、赛季与竞猜](P14-cross-server-seasons.md) | **录像与跨服归奖**：比赛回放、历史赛季/排行、奖励路由到原服、离线领取及补偿。 |
| P15-A | [逐步开放内容与运行维护](P15-content-liveops.md) | **当前章与后续主线**：先恢复此前暂缓的第一章第3～5关逐关手机验收，再按章检查任务/对白/场景/过场、敌人、等级前置、掉落与星箱。 |
| P15-B | [逐步开放内容与运行维护](P15-content-liveops.md) | **野外地图与玩法内容**：逐张配置场景/怪物/首领/采集点/安全区/线路；增加楼层、探索地图、远征任务、公会战地图。 |
| P15-C | [逐步开放内容与运行维护](P15-content-liveops.md) | **养成、招募与经济内容**：更多角色/精灵/装备/饰品、技能/配方/材料、奖池轮换、商店和礼包档。 |
| P15-D | [逐步开放内容与运行维护](P15-content-liveops.md) | **活动版本投放**：节日/庆典/新版/开服狂欢实例、签到历/比赛赛季/活动档及内容公告；逐项消化P4规则待审计项。 |
| P15-E | [逐步开放内容与运行维护](P15-content-liveops.md) | **运行维护与完整配套**：管理后台、配置校验/发布、备份恢复、迁移、监控与维护；改名、功能导航/红点、公告/客服、引导及升级解锁提示。 |

## 容易遗漏或混用的归属

| 条目 | 主要任务 | 依赖/边界 |
| --- | --- | --- |
| 普通/强者招募及本地转盘 | P0-C、P4-A | 已有基础；先验收，再补轮换/完整规则；未知原服卡池不宣称已恢复 |
| 训练、突破、觉醒、融合、锻造、专精、宝石、料理 | P0-D、P5-E | 已有实现与高等级手机验收分开记；采矿/钓鱼来源归P8 |
| 单次/五次强化与一键强化 | P0-C、P5-D | 前两类已有；单件/全队一键强化另实施 |
| 回忆圣坛、声望、积分、试炼商店 | P4-B | 竞技货币P11、悬赏积分P12、试炼代币P7；完整闭环各期回填 |
| 会员、精灵、公会商店 | P3-B、P9-D、P13-B | 复用交易能力，资格与货币由所属系统负责 |
| 饰品箱、红卡整卡/碎片选择箱、稀有兑换 | P2-B/C、P5-A、P4-B | 选择使用与兑换购买分开验收 |
| 月光餐厅 | P4-C | 定时领取体力，与P8料理制作/使用分别实现 |
| 14天开服任务与开服战力竞赛 | P4-D | 两套规则；任务条件由所属玩法成功事件接入；第15天规则未由APK证实 |
| 月签到、独立登录签到、限时签到、累计登录 | P0-D、P4-C | 各自窗口/计数/奖历；豪华签到支付资格P3-C |
| 招募/收集/概率提升、消费/返利、攻略/关卡福利 | P4-D | 每个活动实例的事件口径和本地参数明确 |
| 竞技/夺宝、悬赏攻击/击杀送礼 | P4-E、P11-E、P12-E | 玩法未恢复前保持未达成 |
| 首充/单充/累计/连充/双倍、VIP福利 | P3-B/C/E | 使用真实确认的支付事实；Demo测试支付明确隔离 |
| 神秘小屋、藏宝箱/招财猫、红武转盘 | P4-D/A、P2-C | 折扣、探索与转盘分别建规则；红武入口归属继续审计 |
| 红卡与红装备早期体验 | P2-A～E | 红卡ID/卡池/选择箱/兑换、红装真实渠道与培养提前完成；动态机制P6回填 |
| 氪金获取红卡/红装 | P3-A/D/E | 支付事实、到账、商品/礼包与P2获取链路联合验收 |
| 限时神装礼包 | P3-D | 原表20资质套装；章节触发开放随P15验证 |
| 开服狂欢、庆典、节日、新版、AincradStars等旧类型 | P4-E、P15-D | 先登记36种活动类型；只有枚举的项保持待审计 |
| 守护精灵/宠物/Pet | P9 | 合并为同一系统；包含装备、亲密度、商店和回收 |
| NPC巡逻路径/TablePatrol | P8-E | 无独立巡逻副本证据；后续若证实新玩法，增加单独任务 |
| 奖杯争夺/挑战场及占领奖励 | P11-D | 挑战、占领、随机奖、累计时长、冷却和日切一并验收 |
| 福袋、烟花、语音 | P10-C/E | 真正的多客户端、附近资格和存储/播放验证 |
| 录像、排名、奖励与周期 | P6-E、P1-D | 公共能力；各玩法分别完成调用、周期与真实指标 |
| 安全区、线路、同屏、共享怪 | P10-D | P8先交付单人地图；多玩家对象并发由本任务验收 |
| 野外首领、世界首领、悬赏、PK、红名/监狱 | P12 | 依赖P6完整战斗、P10多人状态；共同回填首张地图整体能力 |
| 公会建筑、仓库、职位福利、公会战指挥 | P13-B/E | 公会经济、地图、报名、指挥和结算分别按批执行 |
| 公会/公会战聊天、远征好友帮助 | P13-A/E、P10-B | 回填P10-C及P7-D |
| 客户端改名、导航、红点、公告、客服、引导 | P0-E、P15-E | 每期处理自己入口必需配套；P15全量补齐与维护 |
| 管理、无线连接、维护 | P0-A、P1-E、P15-E | 无线Demo先验收；最小运维早做；完整后台后做 |
| 第一章第3～5关、后续章节、更多野外地图 | P15-A/B | 保留此前主线暂缓边界；功能完成后逐批开放内容 |

## 功能覆盖复核方式

对照基线每个功能组，确认能在上表或批次索引找到主要任务。新增发现功能先登记其证据和所属任务，再调整受影响依赖；未知规则和缺实机验收分别记状态。

本索引保留业务组和实施批次；本轮已将原479请求、164表、134基线条目及36活动逐行登记至[全量范围审计](03-full-scope-audit.md)。归属登记不表示当前业务已通过：原UI/IL入口、枚举/字段、全部状态链、证据和实现差量仍由P15-E.1及所属任务逐项核实，CSKVInfo用途具名待查。


## 全量范围审计补充细项

以下任务补足逐ID/最高阶/自然来源/全部状态链/原机对照。状态和完整验收标准仅在所属期文件维护；原阶段记录不覆盖这些细项。

| 任务 | 所属期文件 | 补全范围 |
| --- | --- | --- |
| P0-A.1 | [P0-demo.md](P0-demo.md) | 原登录/建角/区服/HTTP/TCP/心跳与无线地址变化、会话轮换、前后台和重连的完整入口矩阵。 |
| P0-E.1 | [P0-demo.md](P0-demo.md) | 原Guide/GuideStep、任务与解锁提示的完整逐步骤索引；当前Demo必需步骤先闭合，其余按玩法及P15-E.2归属。 |
| P0-E.2 | [P0-demo.md](P0-demo.md) | 用户确认当前主城“潜能药剂”购买弹窗无法关闭/挡住操作。恢复可用关闭入口、原关闭回调和触摸层释放，定位弹窗所属玩法与触发来源。 |
| P1-A.1 | [P1-rewards-events.md](P1-rewards-events.md) | 全资源/币种/堆叠与GUID实例的单位、容量、来源和消费边界；重放与合法重复操作、失败回滚及并发。 |
| P1-D.1 | [P1-rewards-events.md](P1-rewards-events.md) | 各周期/真实排行并列和截止快照的复用矩阵，含活动关停后已冻结实例及离线待领。 |
| P1-E.1 | [P1-rewards-events.md](P1-rewards-events.md) | 隔离环境数据库引擎冷重启、完整备份恢复、升级迁移与失败回退演练；恢复已结束/未结束玩法及历史回执。 |
| P10-A.1 | [P10-social-scenes.md](P10-social-scenes.md) | 搜索/推荐/申请列表、同意/拒绝/撤回是否支持、删除/再加、黑名单加入/移除及容量/资格。 |
| P10-A.2 | [P10-social-scenes.md](P10-social-scenes.md) | 好友完整资料、实际装备/伙伴/助阵/宠物/称号/战力和在线状态、不同服目标范围。 |
| P10-B.1 | [P10-social-scenes.md](P10-social-scenes.md) | 单赠/单领/批量耐力、领取剩余数/按钮、每日重置、删除/拉黑后未领取处理。 |
| P10-B.2 | [P10-social-scenes.md](P10-social-scenes.md) | CSGood、CSFamous人气查询/点赞、剩余次数/名单、真实玩家排行、对应奖励来源与领取。 |
| P10-B.3 | [P10-social-scenes.md](P10-social-scenes.md) | 远征坏事件列表、可帮助资格/次数、求助与接受帮助、帮助结果/奖励及失效。 |
| P10-C.1 | [P10-social-scenes.md](P10-social-scenes.md) | 原世界/场景/私聊文字、Cache0/频道历史/离线、会话与双向黑名单、长度/频率。 |
| P10-C.2 | [P10-social-scenes.md](P10-social-scenes.md) | CSAudioChatCheck与原录音/上传/文件名/发送/下载播放、语音历史、频道权限与失败恢复。 |
| P10-C.3 | [P10-social-scenes.md](P10-social-scenes.md) | 公会/公会战频道与缓存、语音同权限、角色退会/换职/结束比赛后的权限撤销。 |
| P10-D.1 | [P10-social-scenes.md](P10-social-scenes.md) | 玩家列表/移动/外观宠物刷新、安全区/线路查询与换线、离线消失/背景状态、实例隔离。 |
| P10-D.2 | [P10-social-scenes.md](P10-social-scenes.md) | 共享普通怪/矿/鱼与场景对象归属、刷新/耗尽/重生、互动动作/对象消失同步。 |
| P10-E.1 | [P10-social-scenes.md](P10-social-scenes.md) | 原烟花350/福袋349自然获取渠道、活动条件/次数/原资格、完整57奖池及奖励用途。 |
| P10-E.2 | [P10-social-scenes.md](P10-social-scenes.md) | 原动画、位置/附近范围/同线资格、所有权/显示/抢取/次数、过期清除、日切及重登。 |
| P11-A.3 | [P11-competition.md](P11-competition.md) | 原天梯匹配窗口/刷新、对手完整资料、真实挑战、剩余次数或能量、购买补充、扫荡/连战、合法退出/恢复。 |
| P11-A.4 | [P11-competition.md](P11-competition.md) | 胜利名次交换、最高/当前名次、首达/挑战箱奖励、原全排名日奖励/领奖期、竞技称号。 |
| P11-A.5 | [P11-competition.md](P11-competition.md) | 好友切磋真实双方、原battleType11、发起/录像/结果展示、关系失效/合法退出/掉线恢复。 |
| P11-B.1 | [P11-competition.md](P11-competition.md) | 限时竞技活动查询/周期/资格、匹配/真实战斗、次数/买次、积分增减与退出恢复。 |
| P11-B.2 | [P11-competition.md](P11-competition.md) | 每日排行奖励积分、累计阶段奖励、限时签到资格/奖励、榜单/历史/过期与称号联动。 |
| P11-C.1 | [P11-competition.md](P11-competition.md) | 全饰品碎片目标/拥有/缺片、真实对手刷新与资料、抢夺资格/胜败/掉落、批量扫荡。 |
| P11-C.2 | [P11-competition.md](P11-competition.md) | 免战物品获取/使用/剩余时间、挑战后保护状态、碎片合成→饰品装备/成长与声望兑换。 |
| P11-D.1 | [P11-competition.md](P11-competition.md) | 低/中/高级场全部位置查询、挑战/占领/替换、放弃/冷却、挑战次数/买次、战斗与记录恢复。 |
| P11-D.2 | [P11-competition.md](P11-competition.md) | 随占领时长的随机奖励、累计分钟档领奖、被夺/放弃后的时长、日切/排行/称号及过期处理。 |
| P11-E.1 | [P11-competition.md](P11-competition.md) | 竞技/夺宝送礼、14天条件、声望/相关商店、领奖中心、战斗错误反馈与全录像入口。 |
| P12-A.1 | [P12-bosses-field-pvp.md](P12-bosses-field-pvp.md) | 首领实例/版本/出生位置/共享血量/当前阶段查询、参与资格、加入/退出/断线恢复。 |
| P12-B.1 | [P12-bosses-field-pvp.md](P12-bosses-field-pvp.md) | 野外首领查询/开始/结束/重生、共享伤害、参与/攻击/击杀奖励、死亡/复活继续。 |
| P12-B.2 | [P12-bosses-field-pvp.md](P12-bosses-field-pvp.md) | 世界首领时间/状态/增益购买/资格/次数、战斗、伤害榜、奖励查询与窗口。 |
| P12-C.1 | [P12-bosses-field-pvp.md](P12-bosses-field-pvp.md) | 悬赏自然发现、列表/详情、分享对象/资格、参与/次数/补充购买是否支持、攻击/击杀/超时。 |
| P12-C.2 | [P12-bosses-field-pvp.md](P12-bosses-field-pvp.md) | 悬赏伤害/表现排行、奖励预览/领取期、失败/过期、积分真实来源与商店兑换。 |
| P12-D.1 | [P12-bosses-field-pvp.md](P12-bosses-field-pvp.md) | 野外PK资格/倒计时/对战距离、双方队伍冻结、真实胜败/退出/死亡、金币转移日限額与录像。 |
| P12-E.1 | [P12-bosses-field-pvp.md](P12-bosses-field-pvp.md) | 红名/击杀累计、入狱/服刑、在线/离线计时、查询/赎出/费用、监狱内操作限制和复活。 |
| P12-E.2 | [P12-bosses-field-pvp.md](P12-bosses-field-pvp.md) | 全首领/悬赏/PK事件对应14天任务、活动送礼、称号和领奖中心；首张图多人终验。 |
| P13-A.1 | [P13-guild-team.md](P13-guild-team.md) | 公会查询/搜索/创建/申请/撤回/列表/同意拒绝、职位任免/踢人/转会长/退出/解散、公告/宣言/旗帜。 |
| P13-A.2 | [P13-guild-team.md](P13-guild-team.md) | 公会成员资料/在线/贡献、普通公会与公会战频道历史/推送/语音授权撤销。 |
| P13-B.1 | [P13-guild-team.md](P13-guild-team.md) | 捐献种类/次数、公会金币/徽章/活跃/等级成长、仓库/捐献宝箱查询/领取与升级。 |
| P13-B.2 | [P13-guild-team.md](P13-guild-team.md) | 英雄雕像/守护树/光明教堂全部查询、解锁/建设、各等级升级、祈祷和成员属性。 |
| P13-B.3 | [P13-guild-team.md](P13-guild-team.md) | 公会体力、商店目录/资格/货币/限购刷新、月活礼包/职位差异福利。 |
| P13-C.1 | [P13-guild-team.md](P13-guild-team.md) | 全原组队创建/查询/加入/邀请/创建并邀、退出/队长转让/踢人/解散及tick/check。 |
| P13-C.2 | [P13-guild-team.md](P13-guild-team.md) | 准备/取消准备/开始是否原支持、每人伙伴队伍、共同副本/战斗/消耗/奖励、合法取消/退出。 |
| P13-C.3 | [P13-guild-team.md](P13-guild-team.md) | 队长离线/客户端后台、队员中途掉线/重连、战中重登/服务重启和结束清理。 |
| P13-D.1 | [P13-guild-team.md](P13-guild-team.md) | 当前阶段/赛程/报名、手动与自动宣战、成员报名/撤回是否支持、首发/替补、阵容与道具配置。 |
| P13-D.2 | [P13-guild-team.md](P13-guild-team.md) | 匹配/分组/预览/入场资格/轮空或取消比赛、完整一场开始结束恢复。 |
| P13-E.1 | [P13-guild-team.md](P13-guild-team.md) | 公会战所有原地图建筑/对象：基地、门/守卫、塔、工厂/战车、据点、资源点、传送点、复活点。 |
| P13-E.2 | [P13-guild-team.md](P13-guild-team.md) | 元素刷新/采集/上交、建筑强化查询/成本、开工厂成本/开启/战车、商店买用道具、据点归属/进攻、传送开始/结束与复活。 |
| P13-E.3 | [P13-guild-team.md](P13-guild-team.md) | 指挥标记/请求攻击、成员/公会任务、焦点/查询、自动战斗设置与挂机。 |
| P13-E.4 | [P13-guild-team.md](P13-guild-team.md) | 胜负/战果/周排行、参与与特殊排行奖励、离线归奖、全部战报/录像/历史。 |
| P14-A.1 | [P14-cross-server-seasons.md](P14-cross-server-seasons.md) | 原跨服开放服/时段查询、进入/登录、联合身份、原服与比赛服会话、阵容上传/玩家资料和路由。 |
| P14-A.2 | [P14-cross-server-seasons.md](P14-cross-server-seasons.md) | 网络断开/超时/消息乱序/重复、比赛服或原服进程重启、重新登录与待处理请求恢复。 |
| P14-B.1 | [P14-cross-server-seasons.md](P14-cross-server-seasons.md) | 资格/报名与合法撤回/阵容更新截止、海选/分组赛/晋级淘汰/决赛、对阵/时间/实时状态。 |
| P14-B.2 | [P14-cross-server-seasons.md](P14-cross-server-seasons.md) | 赛季ID/起止/归档、所有阶段/排名奖励与称号、上届结果/当前历史查询。 |
| P14-C.1 | [P14-cross-server-seasons.md](P14-cross-server-seasons.md) | 单场/冠军竞猜资格/对局与选手、下注资源/档位/次数、截止锁定、竞猜列表与历史。 |
| P14-C.2 | [P14-cross-server-seasons.md](P14-cross-server-seasons.md) | 胜/负/平局/取消或改赛、选手退赛时的竞猜结算、奖励/退款/过期领取与原服归奖。 |
| P14-D.1 | [P14-cross-server-seasons.md](P14-cross-server-seasons.md) | 录像列表/选手资料/回放、战报与原赛季排行关联、版本与权限。 |
| P14-D.2 | [P14-cross-server-seasons.md](P14-cross-server-seasons.md) | 比赛奖励/竞猜奖励路由原服、原服离线/网络不可达/重启、领奖中心领取及补偿。 |
| P14-D.3 | [P14-cross-server-seasons.md](P14-cross-server-seasons.md) | 全原跨服赛季类型/阶段/奖励目录及各入口终验清单。 |
| P15-C.1 | [P15-content-liveops.md](P15-content-liveops.md) | 全部16红卡逐卡投放和完整最终验收，包含当前池外OS桐人/OS亚丝娜/尤娜；原红装备全部ID逐件核销。 |
| P15-C.2 | [P15-content-liveops.md](P15-content-liveops.md) | 原全部角色/主角形态/敌人与助战、78精灵（19红）、装备/饰品/宝石/料理、技能/天赋及各原最高可达阶段的逐ID最终目录。 |
| P15-C.3 | [P15-content-liveops.md](P15-content-liveops.md) | 每种材料/币种/配方/SKU/兑换/随机或选择礼包的生产到用途跨期矩阵。 |
| P15-D.1 | [P15-content-liveops.md](P15-content-liveops.md) | 36原活动类型及所有原可达节日/庆典/新版/开服狂欢/KaiGong、赛季、领奖档和运营窗口。 |
| P15-E.1 | [P15-content-liveops.md](P15-content-liveops.md) | 479客户端请求、164配置表、134基线条目的原入口/调用/字段枚举/状态链/台账终审。 |
| P15-E.2 | [P15-content-liveops.md](P15-content-liveops.md) | 全原客户端功能导航、红点、公告/客服、改名/次数、升级解锁、完整引导/任务/剧情断点与旧渠道/遥测/调试入口归属。 |
| P15-E.3 | [P15-content-liveops.md](P15-content-liveops.md) | 原各角色/敌人/精灵/物品模型、图标、特效、语音、UI文案与剧情/地图资产在实际客户端的逐ID可用性。 |
| P15-E.4 | [P15-content-liveops.md](P15-content-liveops.md) | 完整配置/运营管理权限、版本发布/回退、存档迁移、备份恢复、日志/故障定位与持续运行。 |
| P2-A.1 | [P2-red-heroes-equipment.md](P2-red-heroes-equipment.md) | 锁定原Hero中heroClass=0/quality=23的全部16伙伴及原红装备目录，逐ID记录原显示/模型/阵营/技能/天赋与获取渠道。 |
| P2-D.1 | [P2-red-heroes-equipment.md](P2-red-heroes-equipment.md) | 每张红卡与每件红装的完整获取→培养至原可达最高阶段→天赋/觉醒/装备触发→实际战斗→回收或保护→重登。 |
| P3-A.1 | [P3-membership-payments.md](P3-membership-payments.md) | 原Payment全部商品档位、订单状态、确认来源、赠钻规则与历史商品/活动快照。 |
| P3-B.1 | [P3-membership-payments.md](P3-membership-payments.md) | VipPerks全部等级和特权列、月卡/终身卡所有资格/天数/每日周福利、容量与购买次数联动。 |
| P3-C.1 | [P3-membership-payments.md](P3-membership-payments.md) | 首充/单充/累计/连充/双倍/豪华签到/消费相关活动的所有原资格和最终档窗口。 |
| P3-D.1 | [P3-membership-payments.md](P3-membership-payments.md) | 章节/时间触发礼包、限时特惠、原激活码/礼包码及原渠道权益的完整来源和领取权限。 |
| P4-A.1 | [P4-recruit-shops-activities.md](P4-recruit-shops-activities.md) | 原全部普通/强者/王者及各随机箱的候选ID、价格/免费/保底/幸运、轮换/预览与原概率或明确兼容参数。 |
| P4-B.1 | [P4-recruit-shops-activities.md](P4-recruit-shops-activities.md) | 原各商店每商品/价格档/限购/刷新/折扣与全部稀有币种真实来源和用途。 |
| P4-E.1 | [P4-recruit-shops-activities.md](P4-recruit-shops-activities.md) | 36活动类型逐类型原请求/实例/资格/真实事件/奖励/排行/周期/库存与原客户端完整入口。 |
| P5-E.3 | [P5-growth-inventory.md](P5-growth-inventory.md) | 原TalentAdvenced的76角色、1003配置行逐角色恢复矩阵，包含全部主干/分支、零阶基线、parent/parent2、talentId、等级条件、互斥/前置和真实消耗… |
| P5-E.4 | [P5-growth-inventory.md](P5-growth-inventory.md) | 全部高级养成最高阶与完整生命周期，按下方系统清单分别登记原表最大值、原UI可达上限、当前服务端允许上限。 |
| P5-A.3 | [P5-growth-inventory.md](P5-growth-inventory.md) | 每种培养材料和币种的实际获取→库存/实例→消耗用途矩阵；成本清单从天赋、训练、突破、觉醒、融合、专精、装备/饰品、世界之种、宝石/料理原表逐项抽取。 |
| P5-A.4 | [P5-growth-inventory.md](P5-growth-inventory.md) | 原物品箱与生活库存连接：item076/123/137随机宝石箱、item342～345六色选择箱等所有ItemUse中MineItem/FishItem结果；分类核对已知固定/随… |
| P5-D.3 | [P5-growth-inventory.md](P5-growth-inventory.md) | 全量角色/装备/饰品回收原语义与每种培养投入返还；逐项对照recycle_desc_text、原表/IL、实际账本，区分出售/分解/重置/重生。 |
| P5-E.5 | [P5-growth-inventory.md](P5-growth-inventory.md) | 按P5-E.3角色与原Equip/Treasure/MineSkill/FishSkill/WorldSeed目录建立全量最终覆盖台账，红卡/红装逐ID优先，剩余内容逐ID移交P1… |
| P6-F | [P6-combat.md](P6-combat.md) | 原APK全量内容与战斗逐ID矩阵 |
| P6-G | [P6-combat.md](P6-combat.md) | 全部剩余效果与工厂关闭 |
| P6-H | [P6-combat.md](P6-combat.md) | 主干/分支/高级天赋与最高阶组合 |
| P6-I | [P6-combat.md](P6-combat.md) | 全范围原机逐击黄金与原播放器 |
| P6-J | [P6-combat.md](P6-combat.md) | 多组敌人、特殊结束与全协议终验 |
| P6-G.01 | [P6-combat.md](P6-combat.md) | `sourceNhitToperdef` |
| P6-G.02 | [P6-combat.md](P6-combat.md) | `blockDam` |
| P6-G.03 | [P6-combat.md](P6-combat.md) | `angerTocri` |
| P6-G.04 | [P6-combat.md](P6-combat.md) | `WeaponPerAtt` |
| P6-G.05 | [P6-combat.md](P6-combat.md) | `comboNumTocri` |
| P6-G.06 | [P6-combat.md](P6-combat.md) | `angerToperatt` |
| P6-G.07 | [P6-combat.md](P6-combat.md) | `dieAfterLastAttack` |
| P6-G.08 | [P6-combat.md](P6-combat.md) | `MangerToNperhp` |
| P6-G.09 | [P6-combat.md](P6-combat.md) | `MperhpreduceToNanger` |
| P6-G.10 | [P6-combat.md](P6-combat.md) | `treatHurtAndAddperAtt` |
| P6-G.11 | [P6-combat.md](P6-combat.md) | `damShareByMaxHpTeammate` |
| P6-G.12 | [P6-combat.md](P6-combat.md) | `reboundPerMaxhp` |
| P6-G.13 | [P6-combat.md](P6-combat.md) | `additionAttBySelfRound` |
| P6-G.14 | [P6-combat.md](P6-combat.md) | `criTocriFactor` |
| P6-G.15 | [P6-combat.md](P6-combat.md) | `permaxHpBySelfRound` |
| P6-G.16 | [P6-combat.md](P6-combat.md) | `sameDamForRandom` |
| P6-G.17 | [P6-combat.md](P6-combat.md) | `angerChange` |
| P6-G.18 | [P6-combat.md](P6-combat.md) | `angerChangeAndAddHp` |
| P6-G.19 | [P6-combat.md](P6-combat.md) | `angerAndhp` |
| P6-G.20 | [P6-combat.md](P6-combat.md) | `sourcepermdefTomdef` |
| P6-G.21 | [P6-combat.md](P6-combat.md) | `changeSkillBuff` |
| P6-G.22 | [P6-combat.md](P6-combat.md) | `damageAddAndReduce` |
| P6-G.23 | [P6-combat.md](P6-combat.md) | `iceYNT` |
| P6-G.24 | [P6-combat.md](P6-combat.md) | `clearAllDebuff` |
| P6-G.25 | [P6-combat.md](P6-combat.md) | `clearSomeBuffs` |
| P6-G.26 | [P6-combat.md](P6-combat.md) | `damageReduceChange` |
| P6-G.27 | [P6-combat.md](P6-combat.md) | `cripple` |
| P6-G.28 | [P6-combat.md](P6-combat.md) | 来源盾工厂错配 |
| P7-A.1 | [P7-solo-challenges.md](P7-solo-challenges.md) | 普通/特殊/装备随机掉落、首胜/重复通关、完整星条件与星箱、全部原资源副本类别/难度/开放日。 |
| P7-A.2 | [P7-solo-challenges.md](P7-solo-challenges.md) | 挑战剩余次数/重置价格查询、钻石/VIP重置、扫荡资格/数量、料理加成、合法退出与断点恢复。 |
| P7-B.1 | [P7-solo-challenges.md](P7-solo-challenges.md) | 原全部楼层/任务/敌人/先后顺序、挑战星档、爬层继续、失败后状态与合法退出恢复。 |
| P7-B.2 | [P7-solo-challenges.md](P7-solo-challenges.md) | 加成选项查询、选择/累计/生效、阶段加成与奖励领取、一键三星、扫荡到顶/预览、免费/付费重置。 |
| P7-B.3 | [P7-solo-challenges.md](P7-solo-challenges.md) | 迷宫商人出现资格/有效期、商品查询、库存/价格/限购、购买、离开/重进及重置后失效。 |
| P7-B.4 | [P7-solo-challenges.md](P7-solo-challenges.md) | 试炼星数/积分与试炼商店、AincradStars活动、累计任务/阶段星奖励联动。 |
| P7-C.1 | [P7-solo-challenges.md](P7-solo-challenges.md) | 全34精英首领开放前置、原敌方队伍/场景/规则、次数查询/购买、首胜与重复胜利奖励、失败/退出/恢复/录像。 |
| P7-D.1 | [P7-solo-challenges.md](P7-solo-challenges.md) | 远征全部任务阶档/等级成长、4/8/12h等原任务时长与奖励、免费/付费刷新、任务详情与闲置伙伴筛选。 |
| P7-D.2 | [P7-solo-challenges.md](P7-solo-challenges.md) | 派遣占用、各事件好运/厄运、事件选择/解决、真实好友求助/帮助、终奖、失败/到期及合法撤回规则。 |
| P7-D.3 | [P7-solo-challenges.md](P7-solo-challenges.md) | 远征真实时钟完成、离线/重启/跨日恢复，伙伴上下阵/出售/分解/重置与任务占用互斥。 |
| P7-E.1 | [P7-solo-challenges.md](P7-solo-challenges.md) | 原五章节各开放日/难度、原地图节点/探索移动、怪/箱/道具、探索完成度与评分。 |
| P7-E.2 | [P7-solo-challenges.md](P7-solo-challenges.md) | 觉醒死亡/复活次数与道具成本、30min到期、合法提前结算/退出、重复进入/结算、记录列表与回放。 |
| P8-A.1 | [P8-field-life-skills.md](P8-field-life-skills.md) | 原普通怪出生/选点/权重、战斗伤害/怒气/生命、奖励次数、超时/重生、退出返城与重登。 |
| P8-B.1 | [P8-field-life-skills.md](P8-field-life-skills.md) | 矿点完整查询/刷新、资格/技能等级、采集耗时/疲劳/费用、原合法取消、完成领取、耗尽/重生。 |
| P8-C.1 | [P8-field-life-skills.md](P8-field-life-skills.md) | 全鱼点/鱼种查询、钓鱼资格/过程/取消/失败、次数/重生、技能经验与等级。 |
| P8-D.1 | [P8-field-life-skills.md](P8-field-life-skills.md) | 采矿/钓鱼高级技能书自然来源、使用/重复学习、配方/书等级门槛、原全部制作与升级入口。 |
| P8-D.2 | [P8-field-life-skills.md](P8-field-life-skills.md) | 血量查询/同步、快捷料理配置/切换、自动回血阈值、战中/死亡/冷却/库存不足限制。 |
| P8-D.3 | [P8-field-life-skills.md](P8-field-life-skills.md) | 原地/安全区复活方式、免费等待/付费/道具资格、返安全区/复活位置与血量、死亡后禁止操作。 |
| P8-E.1 | [P8-field-life-skills.md](P8-field-life-skills.md) | 原场景/NPC巡逻路径/怪/矿/鱼/首领/安全区/线路配置逐张登记，角色进入/移动/切图/恢复。 |
| P8-E.2 | [P8-field-life-skills.md](P8-field-life-skills.md) | 单人兼容节点迁移到多人共享资源时的归属/消耗/重生规则及场景退出恢复。 |
| P9-A.1 | [P9-pets.md](P9-pets.md) | 普通精灵单/多抽奖池、免费冷却/次数、保底计数与轮换；原参数与未知运营数据分开。 |
| P9-A.2 | [P9-pets.md](P9-pets.md) | 原Pet全78ID、各整卡/碎片/扭蛋箱、合成与售片的真实获取来源；每种培养材料/币种的生产和使用矩阵。 |
| P9-B.1 | [P9-pets.md](P9-pets.md) | 全部Pet默认属性/天赋、PetTalentSkill逐ID二/四突破映射与原触发/作用域。 |
| P9-C.1 | [P9-pets.md](P9-pets.md) | 每只精灵适用最高等级、全部突破层与原完整生命周期。 |
| P9-C.2 | [P9-pets.md](P9-pets.md) | 亲密全部0～7层、原品质附加属性槽、随机属性池/分配/重复/权重和升级效果，攻略延后点数消化。 |
| P9-C.3 | [P9-pets.md](P9-pets.md) | 原三类互动、critTrend/critedValue、hdTimeList、更新窗口、CD、次数与暴击/经验分配。 |
| P9-D.1 | [P9-pets.md](P9-pets.md) | 全部适用精灵装备四槽逐equipId的强化、进阶、觉醒到原最高阶；单/五次/一键与材料来源。 |
| P9-D.2 | [P9-pets.md](P9-pets.md) | 原精灵商店商品目录、精灵币来源/用途、原价格、刷新/周期/限购与轮换。 |
| P9-E.1 | [P9-pets.md](P9-pets.md) | 精灵最高阶后重置/分解/出售的全量原返还、完整投入/捐献快照与亲密来源区分。 |
| P9-F.1 | [P9-pets.md](P9-pets.md) | 全78ID内容目录与最终P15-C归属台账（见下表），包括全19红精灵。 |

统一终验：P15-C.1全16红卡、P15-C.2全原内容、P15-C.3资源来源/用途、P15-D.1全活动、P15-E.1逐请求/表查漏；P6-F～J负责全部战斗与原机，P5/P9负责全部成长和生命周期。

## 下一轮优先问题

| 任务 | 所属期文件 | 内容 |
| --- | --- | --- |
| P0-E.2 | [P0](P0-demo.md) | [潜能药剂购买弹窗无法关闭/阻断主城操作](04-next-round-bugs.md)，用户已确认并留当前截图；下一轮优先修，关闭不误购买/不扣费，恢复原场景输入与原业务。 |


2026-10-14全品质补项：P4-A.2负责全部74伙伴获取，P5-E.6负责每卡各自最高阶与完整生命周期，P15-C.4负责逐卡投放/终验。清单[all-partners.tsv](coverage/all-partners.tsv)覆盖绿6/蓝10/紫13/橙29/红16；[hero-classification.tsv](coverage/hero-classification.tsv)覆盖全部167原Hero行，主角形态/材料/敌人分别有归属。当前执行状态与剩余范围见[审计缺项实施对照](05-full-scope-execution.md)。

2026-10-15 P13-E.4增量归属：真实零战斗计时/低冻结总战力终局、原结果/战报和23:30参与/胜利P1归奖服务器链已过，默认off本地奖档与完整MVP/周榜/战斗分支分列；原机和发布由root。P13-B.3普通公会商店原入口为shopType8，双成本固定[荣誉311,可消费贡献]，由P4通用交易代理独立实施并联动本轮真实战奖。

2026-10-15 P13-E.4追加：真实矿运个人累计与原人物槽2/最佳奖2、提前具名local唯一正数最多奖、23:30 P1归奖服务器通过（b7f8e3b4），并列/零值不冒原规则。P13-B.3/P4原shop8双成本真实消费已由成长代理联动，原机/发布仍root；其他MVP/任务/周榜未关闭。

2026-10-15 P13个人自然任务17稳定待联合：manifest analysis/p13/stable-guild-natural-mission-source-20261015/manifest.json（72261602），接零战斗23→运矿MVP21；原80072/80074空bodyTCP已Dispatch，实际3秒采2矿任务5分→实际运2矿任务7分→原push累计12/两项2/2→原slot3/P1一次归奖，真实HTTP/TCP/InnoDB/race2项66.575s通过，旧Resource/Zero/MVP回归15.407s、allpkg编译/vet/build过。具名local目录/数值提前发布冻结且默认空off，原Textkey有效/不造图标；未计算战斗/未知分数继续阻止终局，原机/root联合待验。公会任务仍无发布目录，下一真实周榜/周奖；整期未完成。

2026-10-15 P13真实周榜/预发布周奖20稳定待联合：analysis/p13/stable-guild-weekly-source-20261015/manifest.json（8cb74274），依赖个人任务17。原HTTP61017 type1/sub1..4读取真实分组/威望，values[威望,serverID]；实际双轮终局100/50→Sunday冻结8/4本地奖→P1离线/cold/8claim/native+MySQL/race250.881s过，实际拒战未决不发周奖。所有eligible报名须有两轮；真实下次报名缺日历第三Go亦过（3条14.895s）。Root精确be287 handler短原HTTP4有效/非法enum+stamp/cold/InnoDB3另37.591s过，handler2.626s；无新main worker。三SQL141表/default无policy行off，原Sunday外精确时刻/同分/参与资格/量为具名local边界，当前私服参数未知；月榜/月末成员会长快照与月称号资格尚无，P6authorityfalse。原机/联合发布由root验收；下一原400矿开厂→公会任务/真实威望在独立WIP overlay通过Go3.086s，尚待native/MySQL。

2026-10-15 P13原野外工厂公会任务21稳定待联合：analysis/p13/stable-guild-factory-mission-source-20261015/manifest.json（2d7c436a），依赖weekly20。原desc_battel_mission_09/balance0任务，额外威望quantity/本地任务ID1003具名提前发布且default0；原8次3秒矿50→逐次上交400→原开厂扣400/8同stamp仅一次→任务1/1+真实ledger5→热关改999/cold冻结5→原120秒出车→合法0kill真实终局50/30/周榜50/30。InnoDB/race+原TCP/HTTP两项96.925s过（race34.89），allpkgcompile/Go5.475、Zero/Transport/Natural/Weekly/GuildMission21.900/vet/build通过，17runtime/test+4evidence。新142事实表+139可选default0字段，无shared接点，不从旧OpenedAt补造任务；result保存extra、终局不重复加ledger，周奖核base+实际task。隔离30级/原始币钻/明确local矿50和HP为有限准备，未声称自然成长；SQLschema/user/grants0，手机/发布/root联合待验，UF14 authorityfalse保留。月榜截止身份由Growth独立开发；非零战斗任务仍待真实结构证书。

2026-10-15 P14原stage7晋级阵容更新12稳定待联合：analysis/p14/stable-promotion-formation-source-20261015/manifest.json（5ac3f03f）。原晋级页81030此前被stage2/海选Boundary永久拒绝，现真实本/外服资格后独立144表保存实际Battle/Formation修订，原签名协议purpose2+独立promotion单号，原报名/名单/资格SHA保持。原24907type2/replay0查询独立current阵容或真实晋级冻结阵容，未晋级不借用。两真服同localIDs/原布阵修改→8upload一次→目的offline/sourcecold→实际HMAC投递→原TCP双服查询74.992s过（business72.037），actualMySQLrace2例8.82/8.10复用同runtime，allpkg新Go1.973/allP1413.365/vet/build过。测试clock/tsv5行头/4秒小于worker5秒窗口错误已逐项修正，失败不作通过；最终20秒窗口明确。只有level70有限准备，未注入资格/阵容/比赛/奖励，wallet/profile不因上传改变、match0/pointsstate0，schema/user/grants0/ctxclean，无sharedmain/handler钩子；默认现promotionpolicyoff/arbitraryUF14authorityfalse，Root联合/手机待。下一独立审服务离线跨过stage4时真实≤10auto资格恢复，不混本稳定源。

2026-10-15 P13换轮旧kick竞态修复3稳定待联合：Growth真实14会月链定位worker无锁快照旧actor后，按player直接leave会误退新轮。analysis/p13/stable-field-kick-scope-source-20261015/manifest.json（7c40e514），onlyFieldService+新test+validation。定时leave携expectedMatch，在原guild/profile行锁内读取当前actor复核；旧scope/无actor完全noop且不发SCTransferScene/SCUFKick，原主动退场保持。allpkgcompile/真实两轮Go3.016、原25人完整Field+新5.934/vet过；真实35actor10会两轮登记/匹配→捕获旧row→原round2enter→8旧kick全部noop/新profile/actor字节不变→合法当前leave一次，MySQL/race29.56s（suite35.044）过。schema/user/grants0、Growth已接入待重跑14会原HTTP/TCP月链，此域race不冒称月原协议通过。

2026-10-15 P14整海选停服auto恢复13稳定待联合：analysis/p14/stable-auto-recovery-source-20261015/manifest.json（9467ac58），依赖晋级阵容12。旧worker仅stage4，新defaultoff/具名恢复policy须早于冻结海选start发布；仅真实1..10名单、完全无round/match、phase5..7按当前观察QualifiedAt恢复0场资格，不造历史胜负；0/11人/晚发布/真实pending继续未决。资格追加2omitempty冻结恢复来源字段保旧0值JSONhash，HMAC证书共同验证源与时刻；仅此恢复源可stage7读原0win0loss终局，继续准入/81030/双cold/热关履约。全原HTTP/TCP+双真服实际停止跨完整stage4+MySQL/race两项110.390s（native72.954，race3×8.12/6.67/15.90）PASS，allpkgGo2.880/allP1420.192/vet/build过，schema/user/grants0/6ctxclean，无sharedhook，Root联合/手机待。额外actual两服合法047原Roster source asset供P6typed17独立原机：analysis/p14/native-type17-qualified-source-4ab24572/manifest.json（eae084ee）；原FirstOrder0/0/Room与Winner省略，不改属性，typed draft只local只读未执行比赛，engine出处v106实记，17authority仍false。下一按Root授权P13贡献→祈祷→原四百分比属性融合/24h到期/离会解绑。

2026-10-15 P13祈祷producer18/P5原属性链稳定待联合：analysis/p13/stable-guild-prayer-source-20261015/manifest.json（e06ed1de），8runtime/test+10证据。原100个人贡献/日3/24h/单效果覆盖、原57131成功空SC后57123读usedCount；local私服池未知须提前发布具名权重/幅度、默认off。SQL145两表/seed冻结真实crypto draw与扣费/原属性PB，member累计贡献不扣，只扣Player可用贡献；纯属性API无锁/写/迁移，当前任期/expiry实时验证、optional缺表零值。Root122共用21fresh hydration+Growth3在原base*(1+sum)×.01层实际融合四原百分比，未增P6范围。canonicalDDL/4InnoDBrace8.77/5.10/4.44/4.76及原HTTP/TCP23.642s全过：15真捐→300→3祷各100/8once，a面板330→331/334/334→cold334→真实转会长/退出330；b五真捐一祷330→331→24h到期330；两真CopyEnter各扣10/原CopyStart后续覆盖/冷/退出/到期完整字节同，无winner。6Go3.799/vet/build过。首次43.193s旧时钟早于实际入会的fixture失败单列，修runner后通过、生产码未动；正式币钻/手机未触碰。schema/user/grants/PID0，预编译83MB和WIP已退休、11.5MB唯一小源复用。Root联合全Go/原机与发布待验，未知原私服池/全P6门禁保持；下一按Root授权恢复原公会科技升级，8type×10level真实原效果/cost，未知建筑分组具名local目录默认off，复用现P5 hydration。

2026-10-15 P13科技producer22/P5属性主链稳定待联合：analysis/p13/stable-guild-tech-source-20261015/manifest.json（b0ff9344），11runtime/test+11原IL/原80行/真实源证据。原8type×10level成本及效果完整锁定d8599001；未知私服建筑分组/初始级采用提前发布具名local目录initial0/defaultoff，只有真实原upgrade创建效果；首次offer冻结目录/80行，热关冷启不重算旧档。纯SELECT属性API无锁/写/迁移，当前会员/建筑/真实等级逐次读取；复用Prayer21fresh hooks，四per×.01同层、常数在base*(1+sum)后Gem/Pet前，旧已冻结battle不变。14真创建/加入/210次100kcoin捐→21m，30真实徽章捐→30，原建筑扣10m20，原科技0→1零cost→1→2扣10m10，8同stamp一次；原HTTP/TCP完整104.969s（业务102.413s）通过，公开FP330→331→332/cold332、两真CopyEnter各扣10/完整Start保持；无Prayer历史的远程Kick/转会长Quit/Dissolve会长及离线会员四份rawFP任何查询前均已330。canonicalDDL+3actualMySQL/race50.61/34.17/28.45s、allpkg6.642/vet/build过。首次前缀193.824s原race均过、native默认off准备遗漏仅修runner reset后native-only通过，生产码未动。schema/user/grants/PID0/锁释放；Rootshared3/Growthshared2精确源码已验，待Root联合/原机/发布，不扩P6 authority。下一两服真实P13贡献祈祷→原stage7 purpose2捕获合法047整数HP4641/3094强弱来源，供P6typed17原机终局证据，不造winner。

2026-10-15 P10公开资料真实本服编号修复8稳定待联合：analysis/p10/stable-profile-local-realm-source-20261015/manifest.json（cbfa1e4f）。原CSFindPlayerInfo的handler/service硬1令服2合法server2拒绝且可能把显式服1误归本服；服务现0或OriginalServerID，Rootsharedb68 handler现0或已认证packetZone，不扩大外服资料。4目标Go/allpkgcompile service29.523/vet/build，真实两进程同localID1原HTTP/TCP服1[0,1]/服2[0,2]同本服PB、-1/对端/3拒、完整PlayerGame不变、服2cold27.811s（业务24.093）通过；非法TCP沿既有关闭连接，首次runner误期HTTP式错误帧已留记录并修runner，生产码没变。schema/user/grants/PID0，Rootshared1待标stable/下次联合；与PVP/Team其它硬1分别交付。

2026-10-15 P14实际祈祷→原stage7 purpose2的typed17源26稳定：analysis/p14/stable-paid-prayer-type17-source-20261015/manifest.json（7fed1636），资产analysis/p14/native-type17-prayer-source-df73ce2e/manifest.json（d30bfd48）。四原角色/真实双服各小名册0场资格，两服选手各真5捐100k→100贡献→提前具名local perhp50池真实原100祷，P5 HP3094→4641整数且Perhp0；5原81030独立purpose2与foreign签名原PBexact。A原转会长/Quit后回3094。生成强/弱、强/强、弱/强三typed只读draft，原Inst0/0、Room/Winner省略、真实随机seed/max40，engine117如实留；冻结海选来源不变、match/pointstate/grant0。最终native124.803s（业务78.284）全过，两ctx各0.009s清退出/schema/user/grants/PID0；所有前置fixture/HTTP预算失败单列，不混成功。旧126MBprebuilt/11.5MB复用源和5个无完整报告partial目录退休，固定证据保留。已交combat/Root原机采集补17真实attacker赢/timeout数学范围；此批无P6计算或authority，不称跨服晋级胜负/积分/归奖完成。下一真实stage7匹配消费须新当前引擎Capture及独立17结构证书；等待时恢复真实服2原组队硬1独立主链。

2026-10-15 P13真实本服2组队7稳定待联合：analysis/p13/stable-team-local-realm-source-20261015/manifest.json（2a659dcb），Root批准shared2 proposal37ee984f。旧队员ServerId输出/CSCheckTeam/两invite数组与handler硬1，现仅可信OriginalServerID/已认证packetZone，保原不接受0和defaultoff，不扩跨服组队。原两服8真level1/8实际TCP在线/sameLocalIDs，服1原创建队伍全部存档不动；服2原检查/创建双邀请push/接受/再邀请/4成员全server2/转队长/踢人/退出/冷/queryTick/解散完整native61.072s（业务55.654）PASS，0/foreign1/3三入口拒无半状态，无等级/币钻/资源注入。原Team所有目标+全包编译service4.321/handler0.705/vet/build过；首次旧handlerfixture缺ZoneId，3包显式ZoneId1后过，生产码三case未再变。schema/user/grants/PID0，旧WIP及54MBbinary退休，唯一11.5MB小源可复用。Root联合/手机待，通知后暂停所有SQL/初始化/清库/容器操作；DB/Redis/危险入口只读盘点见analysis/p13/database-migration-handoff-20261015.md，下一P14消费新Capture/真实点数待独立17结构证书且DB测试恢复。

2026-10-15 P14当前下一项：新晋级单场15owned纯源（8db65d94，stable=false）已接真实资格/current purpose2/newCapture/point1+具名local积分事务；v130空proof拒绝，未跑SQL或称完成。等待Root129三实际原机strict后fixed17证书，再在唯一SQL窗口一次实际Noon/cold/幂等链。受限测试infra16（106498af）已稳定，临时账户fixture/app同scope实际证据由ArenaPrize30.136s提供，详P14续接与数据库handoff。Root135 fullGo进行中；P14整期保留决赛/竞猜/归奖/录像等缺项。

2026-10-15 P14下一项更新：真实晋级单场20已稳定26fa7994（native85.974s，真实资格/currentpurpose2→独立17证书→1point/local分→8once/noon/cold全过，管理资源0），不再空proof等待；Root当前135已发布而本20待下一联合/原机。下一为真实积分联邦同步→原type2晋级榜/截止32，保原values三项与同ID原UI缺口，详P14最新续接。

2026-10-15 P14：真实accepted→签名两服原type2榜23（800facdb）已原HTTP127.195s，发现早期晋级阶段错位，独立原6/旧7只读修复17（57b13928）纯15.477s+3例1.980s过待新原6native；Root147联合/手机待，前32必须全部peer最终版后续实现。

2026-10-15 P14：wire12（9ec7d603）+Final32_19/SQL152（c058a9ee）真实131.363s合链过，原6/7/8字段0及未知下一轮pending已纠正，11ctx/manager清0；Root147基线保留，新片待合/手机；下一Finals真实阵容锁定与对阵，KFReplay证书未恢复不造五局winner。
