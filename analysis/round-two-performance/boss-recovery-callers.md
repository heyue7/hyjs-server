# 首领开战超时与现有恢复调用证据

工作树源码 `beb8706`；以下为静态调用链，未连接数据库、运行客户端或验证线上行为。

## 没有 FightID 时的查询

- `FieldWire.FightActive(Session session, int type, Action<FieldWireResult> callback)`（`tools/desert_extraction/FieldWire.cs:172`）：首领传 `type=12`，POST `/ggo-field/boss/active`；请求体仅 `playerId`、`sessionHex`（`:46`），无 FightID 参数。回调 `FieldWireResult` 含 `uncertain`、`code`、`message`、`data`（`:11`）。HTTP 请求 15 秒；超时、会话不匹配、解析失败及系统码 `7002` 会标为 `uncertain`（`:189-224`）。
- 对应 `GGOFieldBossActiveHandler`（`hyjs-server/handler/ggoBossHandler.go:24`）和 `GameService.GGOFieldBossActive(ctx, auth, session) (*model.GGOFieldBossActiveResponse,error)`（`hyjs-server/service/ggoBossBattleService.go:308`）。`data.active=true` 时给 `fightId`、`eventId`、`inputHash`、`startedAtMs`、`endsAtMs`（`hyjs-server/model/ggoBossModel.go:31`）；服务读取当前未结算且含 BossStart 的角色战斗。`active=false` 只说明查询当刻未读到这样的当前战斗；已结算记录不在此查询结果中，无法据此判断仍在途的原开战后续是否提交。
- `ColdBattleRecovery.Discover()` 为私有方法，已有 6 和 12 两次 `FightActive` 串行查询与 `ParseActive` 绑定校验；若同时存在两种已接纳战斗则拒绝（`tools/desert_extraction/ColdBattleRecovery.cs:88-118`）。其 `Update()` 只在 `known=false` 时发现，场景已完成一次发现且 `known=true` 时不会自动重新发现（`:191-203`）。`BattleRecovery.QueryActive` 也是私有方法，现有触发条件要求已有 `liveFightId` 或结算日志（`tools/desert_extraction/BattleRecovery.cs:292-301`, `:316-347`）。

## 原开战、原结算与交还原 handler

- `FieldWire.ResumeFight(Session session, int type, string fight, string inputHash, Action<FieldWireResult> callback)`（`FieldWire.cs:177`）：POST `/ggo-field/battle/resume`，附 `battleType`、`expectedFightId`、`expectedInputHash`；服务 handler 要求 24 位小写十六进制 FightID 和 64 位小写十六进制摘要（`hyjs-server/handler/ggoFieldBattleResume86Handler.go:28`）。`GGOFieldBattleResumeResponse` 的 `status` 为 `active` / `settled`，分别提供 base64 `nativeStart` / `nativeEnd`，并返回 battleType、fightId、inputHash、eventId；settled 还返回 `endRequestHash`（`hyjs-server/model/ggoFieldBattleResume86.go:14`）。
- `GameService.GGOFieldBattleResume(ctx, req, session)` 在认证及当前角色档案锁内先查已结算收据，再查已接纳战斗；校验摘要、原战斗身份及冻结输入，读取原始 Start/End，不重新开战或发奖（`hyjs-server/service/ggoFieldBattleResume86Service.go:76`, `:124`, `:168-227`）。
- 现有 `ColdBattleRecovery.Resume()`（`ColdBattleRecovery.cs:131-189`）核对 type/FightID/hash/eventId：active 的 `nativeStart` 经 `PBConvert.toObject(30084, bytes)` 恢复 `SCSidePveBossFightStart`，再调用 `BattleRecovery.AdmitResume(12,fight,hash)` 和 `FieldRuntime.ResumeEncounter(12,native)`。后者为 public，反射调用当前房间原 `RoomNetField.SCSidePveBossFightStart`；`InteractionReady` 为真且 `awaitingEncounter=false` 才返回 true（`FieldRuntime.cs:709-731`）。
- 同一 Cold 恢复的 settled 分支校验 `nativeEnd`/`endRequestHash`，刷新场景并提示已结算，不调用原战斗结束 handler（`ColdBattleRecovery.cs:144-156`）。已有 `FieldWire.FightReceipt(session,12,fightId,callback)` 访问 `/ggo-field/boss/receipt`，需 FightID；`BattleRecovery.QueryReceipt()` 在有原结束日志时校验回执并在当前战斗仍匹配且原界面 owner 可用时反射调用 `FieldPVEBossFightResultHandler`（`FieldWire.cs:183`, `BattleRecovery.cs:354-407`）。

## 15 秒与旧回包的现状

- `StartEncounter(12, uuid)` 在 `ConfirmPosition` 之前设定 `awaitingEncounter=true` 和 15 秒截止（`FieldRuntime.cs:589-624`）。到期只设 `encounterTimedOut`、隐藏 loading、提示，`awaitingEncounter` 仍为 true（`:347-351`），所以现有 `ResumeEncounter` 此时直接返回 false。首领位置确认回调检查 sceneSerial、连接 epoch、房间、目标及 session（`:622-628`），但超时后发送分支只检查 `awaitingEncounter`，没有检查 `encounterTimedOut`（`:638-655`）。
- HTTP 恢复的现有回调过滤：`ColdBattleRecovery.Still(token)` 绑定 generation、当前 RoomField、`field_003`、sceneSerial、playerId/session/origin 及 `PositionSync.ConnectionEpoch`；退出场景或重连会更新 generation（`ColdBattleRecovery.cs:29-76`）。`FieldWire.Post/Request` 另外核对当前会话（`FieldWire.cs:189-224`）。`FieldRuntime.ReleaseScene` 更新 sceneSerial 并清本地等待（`FieldRuntime.cs:370-400`）。
- 原生 LateStart 与上述 HTTP 回调不同：`FieldHooks.FightStartResponse(int type, object value)` 返回 void，只检查当前静态 `field_003`，再调用 `EndEncounter` 和 `BattleRecovery.StartResponse`（`FieldRuntime.cs:89-95`）。后者检查场景、回复类型/code、FightID 格式、当前会话及既有 pending，但未接收入站回包所属原 room/sceneSerial/epoch/session（`BattleRecovery.cs:222-242`）。`InjectorField.cs:151-156` 将该 void hook 插入原 `SCSidePveBossFightStart` 开头后继续执行原 handler；现有 hook 本身不拦截旧 Start，也未提供旧请求作用域过滤结果。
