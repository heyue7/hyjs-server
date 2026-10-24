# 每日签到后台入口调整

## 已实现

- 全局活动设置移除顶部独立“每日签到”，在真实常驻活动列表新增 month-sign / daily-sign 行。
- 列表仅在 MonthSignRule 完整31日时出现；启用状态来自普通签到或真实有效豪华签到，无测试行和奖励默认值。
- 专用抽屉提供“每日签到”“豪华签到”两个菜单；前者复用31日原页面/API，后者新增启停、每日奖励、原当天确认支付门槛只读显示。
- 沿用NaiveUI及后台现有配色字体、资源选择/奖励编辑组件；窄屏沿用全宽抽屉并令资源选择占整行。
- 父抽屉关闭、内菜单切换、外标签切换、路由离开、刷新保留未保存/保存中保护；确认放弃真实恢复快照。普通签到独立路由保护保留。
- GET/PUT `/admin/api/luxury-sign-rules` 仅固定 `luxury_sign` + `daily` 原表，按权限、行锁、摘要版本、资源支持范围及审计保存。缺行读取disabled/rewards[]；不创建新表、不自动开启或预设奖励。
- 有效及未来窗口保留；过期窗口显式保存启用时改为安全常驻上限，显示到期/待开放并记录专门审计。daily多行视为配置错误，保原数据。
- 原 `luxurySignReward`、当天6元资格、`p3Claim(luxury-sign, day)` 完全复用；未修改支付门槛、玩家财产、当日领取历史及原APK。

## 本代理文件

- admin-web/src/views/activities/index.vue
- admin-web/src/views/daily-sign/index.vue
- admin-web/src/components/operations/native-activity-workspace.vue
- admin-web/src/components/operations/luxury-sign-settings.vue（新增）
- admin-web/src/service/luxury-sign.ts（新增）
- hyjs-server/model/adminLuxurySignModel.go（新增）
- hyjs-server/service/adminLuxurySignService.go（新增）
- hyjs-server/service/adminLuxurySign_test.go（新增）
- hyjs-server/handler/adminLuxurySignHandler.go（新增）
- hyjs-server/handler/adminMonthSignHandler.go（仅路由注册）
- hyjs-server/constUtil/adminLuxurySign.go（新增）
- hyjs-server/service/adminNativeActivityService.go（仅列表新增签到行；保留schema64所有机械替换）

## 验证

已执行gofmt及受影响已跟踪文件git diff --check，通过。已核源码消费者、资源限制、活动表结构与原资格路径。

已新增TestAdminLuxurySignConfigurationBoundary：覆盖缺行、空奖拒绝、正常保存/原消费者读取、旧版本拒绝、未来窗口保留、过期重开及多源拒绝。该测试默认显式SQLite临时目录；存在HYJS_P4_MYSQL_DATABASE时使用分配隔离MySQL，Root运行前须核环境。

Root报告前端typecheck通过；Go定向测试由Root正在执行（结果待Root记录）。未运行服务、触DB、浏览器、手机、commit、push或发布。

## Root验收

1. 排队运行前端typecheck与新定向测试/相关原每日签到测试，合批Go编译。
2. 真实API核列表、disabled空奖励、viewer写拒绝、资源校验、409、有效/未来窗口保留、过期重开、领取历史不变。
3. 同状态前后桌面与390px浏览器实际渲染、资源选择、保存/刷新/取消、内外切换/路由/遮罩/Escape保护；构建不作为UI通过证据。
4. 原客户端两个菜单及当天确认支付/领奖幂等按Root隔离QA进行。

## 独立源码复核追加

- 修复每日签到无改动/只读奖励弹窗切菜单时teleport弹窗残留；获准离开后统一关闭内层弹窗。
- 放弃草稿保留原409冲突锁，刷新失败时保留旧版本锁定。
- 固定豪华签到初次缺行创建竞争：仅将MySQL1062/1213及SQLite明确database is locked驱动错误映射409；其他错误保留原失败。
- 父子ref按当前签到活动限定，内菜单离开检查当前子页，整个抽屉/外菜单/路由检查两个已挂载子页；保存/读取期间父入口阻断关闭。

无本代理自建进程/DB/容器/缓存；交付源码及本记录保留。
