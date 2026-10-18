# HYJS 运营台前端

实际复用 Soybean Admin v2.0.2（MIT），上游完整提交 `5e40b850180804aa51573590eb2e058d1fd8eb06`。来源见 `UPSTREAM.json`，许可证保留在 `LICENSE`，原 README 保留。

复用范围：Vue3 / Pinia / Elegant Router、`packages/materials` 的 AdminLayout、NaiveUI、UnoCSS、侧栏/面包屑/用户菜单、主题与亮暗切换、响应式布局。业务页面和鉴权适配接 HYJS Go 服务。默认沿用上游紫色主题，运营页面减少无用模板入口，真实数据与操作优先。

## 构建

```sh
rtk proxy env COREPACK_ENABLE_PROJECT_SPEC=0 pnpm install --frozen-lockfile --ignore-scripts
rtk proxy env COREPACK_ENABLE_PROJECT_SPEC=0 pnpm run typecheck
rtk proxy env COREPACK_ENABLE_PROJECT_SPEC=0 pnpm run build
```

Node >=20.19，pnpm >=10.5，当前验证 Node25.5 / pnpm10.27。产物 `dist/`；Vite base 固定 `/admin/`。Go HTTP 服务应将 `/admin/api` 路由先匹配，`/admin/assets` 提供 dist/assets 的真实静态文件，其余 `/admin/` 前端路由返回 dist/index.html。不要把 node_modules 放进服务候选。

也可运行 `rtk proxy python3 scripts/verify_hyjs.py` 一次串行检查类型、构建并输出 validation/ 固定源码与产物摘要。

所有业务请求同源 `/admin/api`，JSON `{code:0,msg,data}`；会话凭据由 Go 的 HttpOnly Cookie 保管，页面只保存无授权能力的 `cookie-session` UI标记。`data.csrfToken` 仅存内存，并随写入请求使用 `X-CSRF-Token`；重新加载通过 /auth/me 恢复。独立后台账号权限 viewer/operator/admin 由后端最终执行。

## 实际页面

- `/admin/login`：Go会话登录、空字段校验、中文输入法确认保护、登录错误。
- `/admin/home`：真实角色/道具/活动/公告数量、服务状态、最近操作。
- `/admin/items`：原资源中文名称/ID搜索；类别、真实品质与适用类型组合筛选；原图缩略图与 NaiveUI 点击预览；角色搜索和幂等领奖中心发放。筛选选项直接取 Go `facets`，图片直接用原提取资源，缺图显示占位。
- `/admin/activities`：由 Go `/activity-modes` 列出实际支持模式与原界面模板，创建/复制、读取、编辑、启停、归档，保持历史参与与冻结规则。按模式渲染商品兑换、资格、每日奖励/单项选一、累计目标、时段体力、返钻、藏宝箱和九奖位转盘表单，无原始JSON编辑。商品模式显示原生等级/VIP资格；其他模式不添加无消费者支持的资格。日期、数量、比例、奖励均为业务控件，奖励使用中文名/原图选择器。空奖励仍为空，创建默认保存关闭草稿。
- `/admin/players`：真实游戏玩家列表/详情，与后台账号分开；角色资源/持有统计、背包与实例、原因必填的改名/踢下线及钱包账单。修改绑定已选角色ID与版本，过时版本由后端拒绝；背包资源ID每批最多100个解析真实名称/图标。
- `/admin/wallets`：蓝绿钱包列表、按角色查看账期、入账/充值过滤、收入/支出/净额/交易笔数、不可改写的历史账单；28项运营入账理由的文案、金额区间、相对权重与开关编辑。理由文案支持 `{amount}`。后台不新增充值入口，游戏内加款沿现有入口。
- `/admin/announcements`：纯文本公告、时间窗口、草稿/发布、纯文本预览；无 v-html。
- `/admin/users`：仅管理员可见，真实账号列表、新建、权限调整、修改自身密码或重置其他账号密码。密码为空开始，12–72字节校验，自己改密需当前密码，成功后清除表单并重新登录。
- `/admin/audit`：后端事务真实审计分页及详情；无前端伪造记录。

## 端点合同

`POST /auth/login {username,password}`、`GET /auth/me` 返回 `{user:{username,role},csrfToken}`；`POST /auth/logout`。

`GET /overview` 返回 `{server:{version,status},counts:{players,items,activities,announcements},recentAudit:[]}`。

`GET /items?q=&category=&quality=&applicableType=&ids=&page=&pageSize=` 返回 `{items:[{id,name,category,quality,description,icon,iconUrl,previewUrl,applicableTypes}],total,categories,facets:{categories,qualities,applicableTypes}}`；facets每项 `{value,label}`，品质value为整数。`ids`为最多100个原ID的逗号列表，用于批量查名/查图。图片路径 `/admin/media/items/<safe>.png`。

`GET /players?q=` 是发放选择器的精简角色索引，返回 `{items:[{id,name,level}]}`。

`GET /game-players?q=&page=&pageSize=` 返回 `{items,total}`；`GET /game-players/:id` 返回 `{id,name,level,vip,chapter,mission,scene,createdAt,version,balances,counts,inventory:[{resourceId,amount,guid?,level?}]}`。`PUT /game-players/:id/name {version,name,reason}`、`POST /game-players/:id/kick {version,reason}` 返回更新详情。玩家修改表单保留失败上下文，不提供清档/任意存档JSON入口。

`/admin/server-clock` 为独立服务器时间页面。`GET /server-clock` 返回 `{enabled,scope:"server",serverId,gameTimeMs,realTimeMs,offsetDays,revision,timezone,testOnly}`；管理员可 `POST` 同路径 `{deltaDays:-1|1,revision,requestId:UUID}`，viewer/operator只读。日期调整全服生效，真实账务与登录安全仍使用实际时间；结果不确定时保留原UUID重试，版本冲突后先刷新。

`POST /grants {playerId,resourceId,amount,reason,requestId}` 返回持久化的领奖附件信息。

`GET /activity-modes` 返回 `{items:[{mode,label,templates:[{id,name,prefab,preview}],conditions,maxMilestones,supportsProducts,supportsMilestones,supportsPrizes,notes,defaultForm,fixedValues}]}`。原模板无预览图时仅显示真实模板名称。

`GET /activity-instances` 返回 `{items:[{id,name,mode,templateId,enabled,version,frozen,editable,form}]}`；`POST /activity-instances` 与 `PUT /activity-instances/:id` 接业务form字段直接展开，加 `{version?,enabled,makeCurrent}`；创建ID由服务端生成。`PUT /activity-instances/:id/state {version,enabled,makeCurrent}`；`DELETE /activity-instances/:id {version}` 停用归档并保留历史。原始 `/activities` 旧合同仅供兼容，当前页面采用typed层。

支持模式由服务端目录决定：兑换、折扣/半价/限时/神秘商品、登录奖历、限时签到、月光餐厅、消费返利、藏宝箱、转盘、累计充值/消费/招募/主线胜利/登录。里程碑上限与VIP选项均从目录读取；累计类当前原消费者每实例一档，充值目标单位以目录说明为准。

`GET /wallets?q=&page=&pageSize=` 返回 `{items:[{playerId,name,balanceCents,revision}],total}`；`GET /wallets/:id/ledger?startAt=&endAt=&kind=&page=&pageSize=` 返回 `{balanceCents,summary:{startAt,endAt,incomeCents,expenseCents,netCents,transactionCount},items:[{id,kind,amountCents,balanceCents,createdAt,label,reasonId,orderId}],total}`。金额均整数分，界面展示元；时间毫秒。

`GET /wallet-reasons` 返回 `{items:[{id,label,minCents,maxCents,weight,enabled,version}]}`；`PUT /wallet-reasons/:id` 接上述编辑字段与version。历史账单理由不跟随配置改写。

`GET /announcements` 返回 `{items:[]}`；`POST /announcements` / `PUT /announcements/:id` 接 `{title,content,enabled,startAt,endAt,version}`，时间毫秒，版本摘要字符串。

`GET /users` 返回 `{items:[{id,username,role}]}`；`POST /users {username,password,role}`；`PUT /users/:id/role {role}`；`PUT /users/:id/password {currentPassword,newPassword}`。

`GET /audit?page=&pageSize=` 返回 `{items:[{id,time,actor,action,target,detail}],total}`。

## 验证边界

构建和 TypeScript 验证单独记录于 validation/；浏览器实际渲染、Go端点联验、保存重读及角色权限由主代理合批集成验收。前端构建通过不等同这些已通过。资源图标仅在后端给出明确安全URL时加载，缺图显示占位。导航及布局图标使用 Iconify offline 组件本地打包，无 Iconify CDN 请求。图标来源与许可证见 UPSTREAM_ICONS.json。


## 2026-10-18 活动与游戏用户工作区调整

- 结构：原活动列表+980px长抽屉 → 保留筛选/分页的活动列表+整页分区编辑（基本信息/奖励内容/活动时间/规则/检查），返回与保存固定可达；窄屏分区导航横排。商品、奖励档和转盘奖位采用单项选择编辑，切换不丢数组数据；只读查看用实际资源、数量、消耗和资格摘要代替禁用输入框。
- 可运营入口 `/admin/activities` 与 `/admin/activities-unavailable` 分开，读取同一 `availability:{usable,reason,missing}`。正常关闭、冻结与不可运营分别展示；未配齐且editable的草稿允许补齐，unsupported/旧个人期只读，不启用或复制。补齐成功后按服务端可用性返回相应菜单。
- 游戏用户新增沿 `POST /game-players {username,password,name,heroId,reason}`，operator/admin可写；9个原初始伙伴白名单，原新手资源，密码仅提交创建接口、不在响应/日志/浏览器存储中保留。角色名称与账号区分，重复账号交后端409。
- 道具 `quality` 按标准阶0..6筛选，`qualityColor` 与中文 `qualityLabel` 来自后端，列表/筛选呈现颜色标签，不展示原始资质数字。
- 本轮初始只读浏览器已实际观察旧列表混排、冻结转盘长表单；routine导航low_confidence后在同一session按规则fallback，结束已关闭。新结构的保存/返回/窄屏实际验收由root统一执行，静态构建不替代该证据。


## 2026-10-18 商店与内容管理

- 结构：新增商店管理父菜单，普通、VIP、记忆圣坛、声望、声望排名、攻略点、积分、公会、精灵、公会战各有独立路由。商品列表保留搜索与状态筛选，编辑按「商品与消耗 / 资格与限购 / 发布与来源」分区；窄屏分区横排。商品、成就、称号共用返回/保存固定可达的工作区，切换页面前提示未保存修改。
- 商店 `GET /shops` 返回真实目录和可编辑 `fields/settingsFields/canCreate`；`GET /shops/:id` 返回商品、全店version、settings。商品 `POST /shops/:id/products`、`PUT/DELETE /shops/:id/products/:productId` 均使用全店version；设置 `PUT /shops/:id/settings`。已冻结运营期商品只读，普通商店原固定商品位仍可单独编辑；通过settings的 `newPeriod:true` 复制关闭新期，再编辑商品。商店的消费者支持哪些字段，前端才展示哪些字段，不向精灵等商店加入无消费者的资格。
- 公会战资源通过商店详情专属 `resources:Item[]` 搜索/展示，不混入普通道具发放目录；其他奖励沿现有 `/items`。资源选择和摘要复用原图、中文名与品质标签。
- `/admin/achievements`：真实列表、名称/类型搜索与状态筛选，分区配置支持的计数类型、目标、奖励和来源；`GET/POST /achievements` 与 `PUT/DELETE /achievements/:id`。新建ID由服务端生成，历史已领取和个人快照按原规则保存。
- `/admin/titles`：`GET /titles`、`PUT /titles/:id`；原称号标识/名称/类型只读，资格参数、原属性整数、期限（小时，0永久）、展示与新资格开关分区编辑。无原客户端资源的新称号不提供伪创建入口，特殊资格仍由其玩法判断，已持有属性不随运营修改追溯改写。
- `/admin/feedback`：`GET /feedback?state&q&page&pageSize` 列表、`GET /feedback/:id` 对话详情；`POST /feedback/:id/replies {version,content,requestId}` 回复，失败保留原UUID；`PUT /feedback/:id/state {version,state}` 更新待处理/处理中/已回复/已关闭。viewer只读，operator/admin回复及处理；正文仅按纯文本展示。窄屏列表和对话顺序堆叠，输入在网络失败时保留。
- 此处记录接口与源码实现；真实Go发布、保存重读、角色权限与浏览器窄屏验收由root统一执行，不能以构建替代。
