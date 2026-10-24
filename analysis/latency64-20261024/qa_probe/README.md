# 批64野外临时只读 QA probe

基于批63 `analysis/feedback63-20261024/qa_probe/NativeBatchRunner.cs` 复制，保留原 caller 透传及原合法命令白名单。初始由执行代理准备源码；Root随后按固定候选编译并完成原机验收，最终结果见下方收尾。

## 构建输入

Root执行：`rtk proxy .analysis-venv/bin/python analysis/latency64-20261024/qa_probe/build.py`。

默认Main为`build/field64-overlay/Assembly-CSharp.dll`，SHA256 `57f12e1c1aaa0ee46bc00d12be6b06bba5fda16533b47cc50a02c19c5e5e4f22`；引用APK为`/Users/lirui/LR/hyjs/build/hyjs-local.apk`，SHA256 `4a85ed009e86c649b5954e8d1f02165e9d1951ec89256196f3dccd9076e6183d`。若Root已组合后续Main，明确传`--main 路径 --expected-main-sha256 实际SHA`；更换引用APK同样传`--apk`及`--expected-apk-sha256`，builder在编译前核对。仅提取mscorlib/System/System.Core/UnityEngine/Assembly-CSharp-firstpass作为引用；不改Main。源码SHA决定独立`HYJS64QA_<hash>.dll`身份，避免复用旧Mono assembly。

## 装载与归还

沿用批63合法原桥装载流程。实际`Application.persistentDataPath`下，原功能caller先保存为`native-runner-original64.dll`，probe覆盖`native-runner-current.dll`。原backup不得删除。所有桥入口先Attach并将原参数透传同名方法；TryStart保持原bool。缺原caller或方法会记录，不制造业务响应；拒绝把probe或带私有Load入口的wrapper作为透传原caller，避免递归。复制文件本身不会执行，下次原合法LogRoomPosition等桥入口才会挂载。

本轮路径：`hyjs64-qa/command.txt`、`hyjs64-qa/observations.jsonl`。host为`HYJS64ReadOnlyProbe`，组件`HYJS64Probe`。不要同时保留活动批63 probe；Root持手机独占锁后完成切换及清理。结束恢复原`native-runner-current.dll`，仅清理本次临时probe/命令/链式副本，保留原backup及必要证据。

## 采样与边界

每0.1真实秒，最长180秒；observations每次UTF-8写入前检查，整个文件硬上限8MiB。达到时停止采样，命令仍可消费；文件已满时结束标记和命令日志可能无法写入。保全证据并移走本轮旧文件后，`sample`可重新开始。普通怪最多64只，Boss最多8只。

仅采room、fieldId、self transform/navigation已有字段、netMask/loading已有静态/实例字段、monsters及Boss。删除副本/jobs/boxes和采集资源/摄像机等大采样。观测不调用PageManager.Instance、LoadingPanel.Instance、导航agent或uiObj的惰性getter。

每普通怪含uuid、monId、server state/currentPoint、owner transform/running、AI状态、moving、patrolAnchor，以及只读FieldMotion的`PatrolAttempts/PatrolStarts/PatrolArrivals/PatrolFailures/PatrolStatus/LastSource/LastTarget`。组件通过已有GetComponents查找，不创建、不Bind、不推进。字段缺失输出null。

Boss读取原`StaticData.fieldBoss`字典、`RCUIFieldBoss.boss`及owner/AI已有字段，输出uuid/serverUuid/uiBossUuid、ownerTransform、AI类型/curState和selfDistance。原正式63元数据已核：StaticData存在fieldBoss、RCUIFieldBoss存在boss/timer/curTime/dis；不调用OnClick/SendPveBoss或修改状态。

## 原命令白名单

原Command方法除63→64组件身份替换外逐字保留。支持原sign/ui topbar只读树、inventory限定只读观察、open credit/activity/trial/map/copy及原限定copy页选项、field 0/1原地图点击、move/stopto有限坐标原玩家入口、goto原AutoGoto（初始化/空闲/完整NavMesh路径门禁）、sample、stop。每条需显式command文件，最大128字符；无自动连点、warp、直接发包、战斗开始/结算、资格或奖励修改。StopTo及其他移动命令会执行原移动和原协议，由Root明确发起。

## 静态检查结果

- Python AST解析通过，尚未运行builder或C#编译。
- 对照63源验证原透传区和整个Command方法在身份替换后完全一致。
- 验证0.1秒、180秒和预写8MiB检查存在；资源/副本采样函数与调用已移除。
- `SF`只取字段；Boss采样与FieldMotion诊断只读，无动作方法。实际装载/采样JSON/全51点巡游与Boss碰撞时延待Root构建及原机验收。

首次静态禁词断言命中已遗留的CameraTool.Instance注释，已去除无关注释后通过；无产品失败或编译失败被隐藏。

## 追加只读导航采点命令

`navsample X Y Z` 仅限当前RoomField且已有self；三坐标必须有限且abs<=1000。只读调用原NavMesh.SamplePosition(target,100,AllAreas)，输出requested/projected/xzOffset/selfPosition及self→投影目标CalculatePath的calculated/status/corners数；不调用MoveTo、StopTo、AutoGoto、warp或协议。Root读取投影地面Y后自行决定是否显式发原goto，goto完整路径与空闲门禁完全保留。闭合QA命令失败日志新增异常message，截长256字符。

Boss采样后续已修正为优先manager.GetCharactors已有RCUIFieldBoss/RCAIFieldBoss对象，并读ui.boss；缓存仅补充，按UUID去重且仍最多8只。selfInvincibleTimer直接读ai.allStates[0].invincibleTimer背字段，原getter IL已核实。


## Root最终验收与归还

0.1秒采样/8MiB上限，雪原r2共28/28实际位移，草原23点覆盖；Boss单次接触892.89ms进入RoomBattle，采样误差保守上界1011.42ms，首次冷资源另5826.11ms已按用户明确要求暂缓。`navsample`追加只读cornerPoints，原MoveTo分段沿已观察完整导航路径行走，未改坐标/战斗结果/资格。正式1.0.64已安装并核SHA；临时probe及链式副本已移除，原caller和8份偏好/显示/转发精确恢复并返回桌面，未登录生产角色。所有probe编译DLL已清理，仅留源码、manifest及压缩证据。
