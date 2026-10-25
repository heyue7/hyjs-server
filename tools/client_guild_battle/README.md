# 公会战平面场景兼容层

`GuildBattleScene.cs` 将已恢复的Re1.1.11 Assets310场景/移动/冲车/小地图适配为独立`SoulFlowerGuildBattle.dll`；原参考DLL SHA为`b26c6b453e5a534bcdfa7da5d302f9266c9ade88e813129d6ecd321ebe59179b`，证据见共享`analysis/guild-full-qa-20261025/re-reference/`。原编译器生成iterator已改为C#3 yield。没有Re账号、云业务、自动采集、奖励或客户端胜负上报。

## 加载与合同

`RoomUnionFight._WaitForLoadComplete`只在原公会战SceneType9转入helper。Helper从`ServerConfig.GetServerUrl()`提取authority，以WWW POST `/guild-war/map`发送当前`playerId/sessionHex`，Content-Type为application/json，等待最多3秒。只有已认证响应`code0/data.mapProjection=re-flat-v1`创建自有场景与可恢复的DAO坐标投影。

空projection通过精确原嵌套`<_WaitForLoadComplete>c__Iterator98`、无参构造、`<>f__this`及`$PC=0`恢复原加载协程，不递归调用被hook的方法。未知/解析/网络失败使用原`BoxManager.CreateNetError`，保留原反馈/重试流程，不调用假OnLoadComplete。请求与后续初始化绑定room实例、当前RoomManager.currentRoom、id/session和generation；旧响应丢弃。服务器负责从入场actor及冻结场次读取投影，客户端不能选择合同。

其余hook只有SceneType9、自有root且room/id/session仍匹配时生效；其他地图完整回落原方法。原语使用灰石、木色和蓝/红阵营，保留原表节点名、原角色和NGUI交互，不新增弹窗工程说明。已有冲车model Bundle优先；仅缺失battle_obj_801/802时生成原语外观。粒子沿原注册协程，整批等待预算15秒；地图元信息WWW完成或取消后Dispose；不更改共享池。退出时恢复仅本helper投影的DAO字符串、小地图字段/纹理，销毁自有root/材质/纹理。

## 构建

仅接受固定正式69输入：`build/guild-input-69.apk`，SHA256 `d60dd8b6a9de9b11d8a673d240f19f3cc16d33956cdede386794cc26e2f71f8d`。

```sh
rtk proxy /Users/lirui/LR/hyjs/.analysis-venv/bin/python tools/client_guild_battle/build.py --base-apk build/guild-input-69.apk --output tools/client_guild_battle/overlay
```

输出目录必须不存在。脚本仅复用项目本地dotnet/Roslyn、dnlib和包内6个编译引用，不下载依赖。临时引用/注入器bin/obj随TemporaryDirectory清除。输出`Assembly-CSharp.dll`、`SoulFlowerGuildBattle.dll`及`manifest.json`；manifest记录payloadFiles/replacementEntries/baseSHA/sourcePins、原ABI和全部16个精确method/RID/signature/prefix/body SHA。

验证主程序集TypeDef/Field/MethodDef数量、原token/签名/flags保持；全部非目标raw method bodies逐字节保持，547个Battle.Engine方法逐字节保持；目标原方法保留完整原CIL/分支/异常区，前缀只有helper处理成功才返回，其余回落原入口。原Iterator98构造/字段ABI与无OfflineBootstrap依赖也检查。继承helpers未替换、未删除公共API。此为静态证据，尚无原机操作结论。

## 交由主代理包装和验收

当前`overlay/manifest.json`为managedOnly，能够作为现有包装器的独立输入。主代理选择新输出路径及正式版本参数后运行：

```sh
rtk proxy /Users/lirui/LR/hyjs/.analysis-venv/bin/python tools/client_release/package_native.py --base-apk build/guild-input-69.apk --overlay tools/client_guild_battle/overlay --output build/<new-client>.apk --record analysis/guild-full-qa-20261025/client-package.json
```

本任务没有运行包装或安装。若服务端/其他overlay需合批，先按manifest的精确基线与payload组合，不能把此Assembly-CSharp直接覆盖后继候选的其他新hook。

原机待验：认证成功/超时/错误/切号/离房取消；旧空projection回原加载；进入平面战场，建筑/守卫/采集/NPC与原小地图对位；摇杆/自动靠近/冲车/门HP与基地资格；退出重进、PK场景转入返回及后台恢复；普通地图导航/小地图/LOD不回归。恢复时序、战斗结果和资源账本由服务端权威生产，此helper只负责场景表现与已有交互。

## r6敌守卫点击

保留r5，r6在同一正式69基线上增加3个前缀：RCUIUnionGuard.OnClick、RoomNetUnionFight.SCUFPlayerFightBegin/End。友方沿完整原0.8靠近/Say；敌方沿原AutoGoto(.8)靠近后复核本场/会话/generation、双方idle及XZ距离，只调用原SendCSUFPlayerAskFight(long)一次，对应80069/targetGuid字段1。浮点端点检查允许0.0001米舍入误差，服务端距离/敌我/门禁仍独立权威校验。

Pending区分接近、等待Begin、已Begin；3秒只限制允许再次手动点击，无自动重发、无本地成败。真实自己Begin的targetGuid匹配才标记已接纳，真实自己End或原idle更新释放。观察前缀内部捕获异常并保证原消费继续；守卫Begin/End/Dead和PVE回放消费者保持。发送异常清理本次pending并继续原异常/提示路径。摇杆取消/友方或其他目标/离房/切号/root释放使旧回调generation失效，不改其他寻路回调。

本批最终包装使用`--overlay tools/client_guild_battle/overlay-r6`；r5原manifest与二进制保持不变；frozen-r5/四个源文件已逐项核对原sourcePins完全相等，frozen-r5/manifest.json将pins指向这些精确冻结源，payload仍引用原r5二进制。单独包装r5使用--overlay tools/client_guild_battle/frozen-r5。原机点击、包收发、距离与失败重试尚待主代理验证。

## 现有packager合同补齐

managedOnly仍须填写inputDexSHA256；本层不替换DEX，outputDexSHA256等于基线classes2.dex SHA。现有packager的allowedChangedMethods要求字符串集合，因此保存14个去重方法名，16个重载的RID/签名/原body/前缀详情放allowedChangedMethodDetails。DLL payload与原16hook/547校验保持。

只读前检（不取打包锁、不签名、不生成APK）：

```sh
rtk proxy /Users/lirui/LR/hyjs/.analysis-venv/bin/python tools/client_guild_battle/build.py --base-apk build/guild-input-69.apk --output tools/client_guild_battle/overlay-r6 --preflight --preflight-report analysis/guild-full-qa-20261025/client-r6-packager-preflight.json
```

## 正式70独立r7

新增明确固定正式70输入build/guild-input-70.apk，SHA97a9ebcc82e1127a8c877ea2385da1a715a1b17d1f9a0eeed352debc53c70061；脚本仅接受既有69与这份70两种精确SHA，不接受任意APK。r7在70主程序集上重新匹配16个原钩子，保留其余方法及所有继承managed helpers字节；姓名/材料helper和资源不进入本层payload。frozen-r6保存r6当前源码的精确旧SHA和可包装manifest，原r6二进制/manifest保持。

```sh
rtk proxy /Users/lirui/LR/hyjs/.analysis-venv/bin/python tools/client_guild_battle/build.py --base-apk build/guild-input-70.apk --output tools/client_guild_battle/overlay-r7
```
