# 试炼首领按需查询与挑战超时

用户反馈 1.1.10 中首领加载后仍提示网络超时，挑战按钮显示慢；要求按需获取并正常挑战。沿已上线 `bc2999e` 产品源码、文档头 `aa1ea33` 新建隔离分支 `codex/trial-boss-demand-20261104`，保留原目录混合改动。

已核事实：固定 APK 的按需 helper 和页面钩子存在，普通试炼已移除全首领预取；主 HTTP 设置 15 秒超时，目录使用独立 WWW。手机现场为精英页上的网络超时弹窗，不能只据页面认定请求来源。线上部分 Start 11.2～28.3 秒、随后 context canceled；这些记录涉及多个玩家，不将其全部归属于手机。

实现范围：

- 可见窗口只取摘要，未选择项不读取敌人大字段、不跑前置及战斗机制资格；只有当前选中项读完整规则并严格校验，其他候选明确 pending。翻页不重复校验旧窗口选择。
- 实际挑战沿用同一事务已绑定的玩家历史投影和服务器证书身份；修正 Re 前置重新创建无 serverID 投影而重复验证整段历史的分支。保留新事务重读、来源完整性、资格、计次、原子提交与结算。
- 先复用服务端能力，暂不改客户端、超时或整场计算模型。只读 Start 快照本机冻结 123～169 ms、admission 89～110 ms、preview 409～630 ms，这个局部样本没有复现线上 15 秒，不据此宣称已确认全部慢因。

验收：真实 InnoDB 定向覆盖窗口/选择/严格前置/来源篡改与请求内复用；用现场角色的私有只读来源构建隔离副本，完整保留真实前置、阵容、首领与结算，测进入页面、选中按钮和 Start。手机必须实际进入并结算，失败按第一断点修复。禁止生产购买、补偿、伪造通关或跳过前置来充当验收。主代理负责投影、隔离数据、构建/设备与集成；子代理仅负责分页两文件及明确的快照夹具文件，资源统一窗口。

## 已完成与验证

产品提交 `4ccd95f1c70007eb6e95811db175431e2f9389f1` 已推送并上线。两处产品修改分别落在 `p7ElitePageService.go` 和 `p7RushService.go`；未调整资格、奖励、次数、客户端超时或战斗算法。

- 真实 InnoDB 的五项定向验证全部通过，无跳过：摘要窗口、未选候选 pending、明确选择严格校验、窗口尾部与原资格一致、同事务历史复用、新请求拒绝损坏来源。摘要见 `analysis/trial-boss-demand-20261104/mysql/summary.json`，原日志 `regression.log`；共享核心编译、gofmt、diff 检查通过。
- 原角色 62 的只读来源在独立账号、隔离库中复现，保留真实阵容、试炼进度、前置证明和首领规则。首屏实测 1.324 秒，选中 11 号查询 0.297 秒、重复 0.294 秒。这里记录本机隔离样本，不据此给出线上整体分位数或提升倍数。夹具日志见 `analysis/trial82/phone-trial-boss-source-r5/fixture.log`。
- 手机使用现有 1.1.10 同源客户端，仅隔离包名及本机地址发生变化；547 个原战斗方法、helper 与 Dex 核对一致。2026-11-04 08:24:00.694 UTC 点击尤金挑战，实际进入战斗、获胜、显示精炼石 6，并返回首领列表。该次服务端 Start 2.277 秒、End 1.693 秒；这些是服务端请求耗时，不包含原生场景资源加载。截图和完整时序见 `analysis/trial-boss-demand-20261104/phone/native-boss11-evidence.md`。
- 实机前后数据库回读确认新局 `p7-29a1a614dca0d6a1fbc82d10f7f56c9a` 已 settled，manualStatus=1、authoritative=1、winner=0，activeFight 清空；已用次数 1→2，精炼石 736→742，金币 33837244、攻略点 63048 不变。最终结果与 Start 预览不同，记录保留 `terminalMatchesPreview=false`；本次未改既有结算机制，不将预览或该字段当成独立重算证明。证据为 `phone/battle-reward-before.json`、`phone/battle-started.json`、`phone/battle-after-native.json`。

隔离夹具准备中修正了角色换号后对象复用、成长表加载、原生监督 stdin 环境，以及归一化精灵实例缺失；这些失败与修正单独保留在 `analysis/trial-boss-demand-20261104/execution-notes.json` 和各轮 fixture/cleanup 记录。补齐原精灵数据后执行真实客户端链路，未用假通关、降资格或跳过前置来通过验收。精灵表白名单补充仅影响后续夹具复现，产品仍固定发布 `4ccd95f`。

## 发布与资源

2026-11-04 08:30:28 UTC 应用启动新版本，08:30:56 UTC 核验完成。沿既有 `deploy/deploy.py` 从干净 detached worktree 发布，执行 CORS 定向检查、全量 vet、Linux/amd64 构建、管理端类型检查和构建。线上 host、container、实际运行进程的程序 SHA-256 均为 `6655235404d36307a07f6c4ad3da901a6ab922cdb0ba86a9172fd2df9c927130`。

HTTP 健康、管理端入口与 JS 均 200，elite-page CORS 204，私有 operations 对外 404，游戏 TCP 可连接；应用无重启、OOM 或启动 fatal。MySQL 容器身份、启动时间和限额、Compose/nginx 摘要及试炼商店 rules/frozen 摘要与发布前一致。没有生产资产验证、数据补写或通知。部署证据在 `analysis/trial-boss-demand-20261104/deployment/{deployed,summary,post-deploy}.json` 和 `deployment.log`。

设备已交回并卸载本次隔离 `.multi`，移除 18871/18872 两个自有转发，恢复进入本轮时的屏幕偏好；正式 `.local` 保留 1.1.10、相同 APK 摘要与安装时间，用户无需换包。临时服务停止、各轮测试库及账号释放、独立登录凭据删除；共享 MySQL/Redis 保留。已清理可再生测试/打包制品、签名目录的本次符号链接、无用截图与含临时账号截图、临时发布 worktree 的依赖和缓存；原交付 APK、原签名凭据、必要私有来源与实际战斗证据保留。线上旧应用镜像与 staging 按全部容器引用检查后清理，当前镜像保留。清理结果见 `phone/cleanup.json`、`cleanup-local.json`、`deployment/cleanup.json` 及 `analysis/trial82/phone-trial-boss-source-r5/cleanup.json`。

本次请求已完成。客户端无需新增版本；线上时延仍受实际角色历史、服务器负载与网络影响，本机样本不作为生产时延承诺。
