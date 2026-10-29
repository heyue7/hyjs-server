# 批84夺宝原引擎对照准备

本入口固定十套 NPC 的真实 `BattlePlayer`、各自显示战力、Go 战斗结果与资格信息，另外固定一次 NPC 单战和一次扫荡子战的实际回执输入。原引擎宿主使用原 `SCFightTreasureStartResult` 的初始战斗字段、目标碎片和 56011 对应的 type3 自动战斗入口。已结算单战/扫荡回包原始字节单独完整保留；宿主输入只投影其 fightId、roomId、seed、两侧队伍和 maxRound，不携带预设胜负、奖励或费用。全部输入在测试导出后按 SHA-256 冻结，重复运行须使用新目录。

## 运行前提

- 排到主代理协调的 Go 编译及隔离 MySQL 窗口。数据库连接使用本项目分配，测试账号及测试库由统一负责人创建和释放。
- 本项目候选源码、原 162 表和已验原宿主必须同时固定。脚本校验原宿主 delivery、runtime manifest、6 个 DLL、JIT 和 162 表；只读 `/Users/lirui/LR/hyjs` 下已编译的宿主。显示战力重算绑定本批 Go 测试使用的 arm64 融合乘加与 `RoundToEven`，并要求 Python 支持 `math.fma`；其他架构须先建立对应的精确数值证据。
- `HYJS_P6_GENERIC_EXPORT_DIR` 必须是当前候选目录下 `analysis/` 中的新路径。目录中每个文件通过独占创建，防止覆盖旧证据。

## 命令

以下示例假设候选为 `/Users/lirui/LR/hyjs/build/feedback84`；测试数据库环境变量由窗口负责人设置。先在候选 `hyjs-server/` 执行：

```sh
rtk proxy env GOCACHE=/private/tmp/hyjs-feedback84-go-cache HYJS_P6_GENERIC_EXPORT_DIR=/Users/lirui/LR/hyjs/build/feedback84/analysis/plunder84/export-unique go test ./service -run '^(TestP11Plunder84TenActualRostersAndCertifiedComputation|TestP11Plunder84NPCFightAndSweepUseFrozenSourceAndColdReceipts)$' -v -count=1
```

保留测试日志、候选源码 SHA 和实际测试库标识。两项通过后，在候选根目录执行：

```sh
rtk proxy /Users/lirui/LR/hyjs/.analysis-venv/bin/python tools/plunder84/native_compare.py prepare --exports analysis/plunder84/export-unique --output analysis/plunder84/frozen-unique
rtk proxy /Users/lirui/LR/hyjs/.analysis-venv/bin/python tools/plunder84/native_compare.py run --source analysis/plunder84/frozen-unique --output analysis/plunder84/host-unique --host-root /Users/lirui/LR/hyjs
```

`prepare` 逐案比对 Go 冻结输入与原 protobuf wire，独立按原公式从战斗属性重算显示战力，生成原 type3 callback 和对应目标碎片 sidecar；记录每套英雄、目标战力、实际战力、偏差与资格摘要。十套阵容使用诊断 callback；单战和扫荡子战保留已接受回执中的真实 FightID 与完整原回包字节，并校验它们和冻结队伍、种子一致。全部 Go 源码、依赖描述及协议解码器纳入摘要，冻结后任何变化需要重新导出。`run` 在临时目录复用固定宿主，逐案保存日志和无损压缩原 trace，严格比较赢家、最终轮次、所有角色的顺序/身份/状态/HP 及原 `BattleOperate` protobuf 字节；任一差异标为失败，其他样本继续。报告逐案写入，原始输入和既有黄金不被改写。

脚本比较的是本机原 Engine 与 Go 的上述终局和原协议字节，不覆盖全量逐事件比对，也没有采集原机界面。原机点击、实际播放及 MySQL 原协议并发/冷恢复由联合验收负责；输出中的 `phoneCompared` 与 `businessAuthority` 均保持 `false`。导出、冻结或原引擎运行中出现错误时保留输出及首个错误，由负责人判断是否需要调整代码并重新固定输入。

## 原客户端静态链

以下方法来自原 `Assembly-CSharp.dll`，SHA-256 `ee14d5df4077573abf94be309d6ebdbe7f4ba84c31b09743ed9a9008b65d200f`，MVID `22d2d1ef-5802-443d-9c1e-315513506718`。方法 token 仅属于该程序集：

- `UI.PlunderPvpPanel.OnClickAutoPlunder`（`0x0600514c`）和 `AutoPlunder`（`0x0600514d`）通过 `UITreasureFrag.GetFragNumber` 选取本地未持有的片。`PlunderResultPage.DoOpen`（`0x0600515c`）以首片发送 `56009`，自动模式每次请求 1 场。
- `PlunderResultPage.AutoSweepCallback`（`0x0600515e`）先应用 `get` 和 `cost`，以首条 reward 的 `isGetFrag` 推进目标，再跳过本地已有片。`<SendAutoOnce>c__IteratorF6.MoveNext`（`0x060069d2`）等待后以当前目标、次数 1 及原补药勾选值发送下一个 `56009`。`UIPlayerData.AddSimpleItem`（`0x06003afe`）将 `frag_trea` 更新到 `UITreasureFrag.AddFrag`。相关 REA evidence：`ev_ac7e89dd0101ce1f33c7538efe25fa50577e6becdac5d2adf2459c5256b388a6`、`ev_e12d0bd2cfa3a86267b529bce06c1d9959c2dd4dcd7901a8227930e592812ad3`。
- 普通单战的 `PlunderSelectPage.DoReopen`（`0x06005169`）清除旧候选并重新 `GetRivals`；`OnCoverPageRemove`（`0x0600516f`）发现已有目标片时关闭选择页。`HTTPManager.Tick` 先派发响应首帧给 callback，再将额外 `20102 SCPlayer` 帧交 `UIPlayerData.RefreshData`；静态证据 `ev_222c3ce8bb79a816dd3fd91f953e82d65d06fa24d2437692c6f85230cf68d64d`。
- 待原机确认的显示风险：单战现有响应在战斗前附最终 `SCPlayer`，战斗结果 `Battle.BalanceBase.Refresh` 随后虚调用 `CopyBalance.AddBalanceReward`，其 type3 分支再次应用 `get`，其中 `coin` 分支调用 `AddCoin`。这可能让客户端暂时重复显示本次金币，数据库结算仍由原一次事务及回执约束。`BattleData.SetData` 的该重载与 `FightCallback` 未观察到应用 `cost`，因此单纯移除额外 `SCPlayer` 会影响能量显示。相关 REA evidence：`ev_bccec7699ba7945121873b650843b8a161608129f11fe3c05bdad32a6e04dad5`、`ev_578075cb0d997d96ff0dfff096b4a211bbae3f3f438317624533feb86189b21c`。客户端处理需通过原机、真实协议与资源同步共同验收。

## 已结算回包的独立计算输入

固定原宿主 `OriginalHeadlessPlaybackCaller.RunPlunderAutomatic` 拒绝预设 `isWin/isGetFrag/get/cost/reward/rewardFake1/rewardFake2`，以免把已知结果喂给独立计算。`prepare` 先核 `.actual-response.pb` 的完整摘要、规范字节及冻结身份/队伍，再构造仅含原始初始战斗字段的 `.plunder-start.bin`。两份字节分别固定摘要；完整业务回包不被改写，投影标签写入manifest。这只调整验证适配器，不改服务、原宿主、计算胜负或数值容差。
