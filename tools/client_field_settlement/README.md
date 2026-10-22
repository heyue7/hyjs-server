# client_field_settlement

Bounded recovery for the original **type12 field-boss** settlement wait. This
directory does not change the server, the battle algorithm, the original
request format, the original success callback or any reward rule.

The separate type18 world-boss bridge (`tools/p12_worldboss`) is not touched or
reused by this overlay.

## Original fault

`Battle.FieldPVEBossBattleOver.AskFieldPVEFightResult` builds one
`CSSidePveBossFightEnd`, freezes `fightId`, `pveBossUUID`, `result` and
`ParseOptRecord()`, registers the opcode `30086` handler, sends opcode `30085`
once and returns:

```text
IL_0104 ldc.i4 30086
IL_0109 ldarg.0; ldftn FieldPVEBossFightResultHandler; newobj DelegateFunc..ctor
IL_0115 call NetAPI::RegistSocketHandler
IL_011a ldc.i4 30085
IL_011f ldloc.0
IL_0120 call NetAPI::SendSocket
IL_0125 ret
```

Only `FieldPVEBossFightResultHandler` unregisters `30086` and calls `End`, which
invokes `onResult`. An `ErrorMessage` or a dropped socket never reaches that
handler, so the result screen waits forever. The observed server failure was the
MySQL container hitting its 768 MiB cgroup limit and being OOM-killed during
`CSSidePveBossFightEnd`, which surfaced as `invalid connection` on the client.

## Change

Two hooks only, both in `Battle.FieldPVEBossBattleOver`:

1. `AskFieldPVEFightResult`, after the original `SendSocket(30085, request)` and
   before `ret`:

   ```text
   ldarg.0
   ldloc.0
   call LocalFieldSettlement::Observe(object, object)
   ```

   `Observe` serializes and reads back the **same** request into a frozen
   `CSSidePveBossFightEnd`, then starts a 10-second wait. It does not call the
   original method again, does not rebuild the request and does not touch
   `StaticData.curBattleData`.

2. `FieldPVEBossFightResultHandler`, at the first instruction:

   ```text
   ldarg.0
   ldarg.1
   call LocalFieldSettlement::Completed(object, object)
   ```

   `Completed` accepts `SCSidePveBossFightEnd` only for the same live owner,
   battle, player and session; the original response has no fightId field, stops the wait and closes this adapter's dialog. The original
   handler then stores `StaticData.pveBossEnd`, `isLastBattleWin` and
   `lastBattleEndMsg`, unregisters `30086`, and calls the original `End`.

### Bounded retry

- Waits 10 seconds for the original `30086` callback.
- Re-sends the frozen request through the original `NetAPI::SendSocket(30085, ...)`
  at most twice (10 seconds each window). The original `30086` handler is **not**
  registered again; `NetWork.SocketController.ReConnect` reuses the same
  `EventManger`, so the original listener survives a reconnect.
- After the third window, opens the original `BoxManager` confirm/cancel box:
  content `首领结算暂未完成，服务器尚未返回结果。可重试同一结算请求，或取消返回登录。`,
  confirm re-sends the same request, cancel calls the original
  `ManagerController.Instance.BackToLogin()`.
- A response that arrives while the box is open closes the box and proceeds
  through the original handler. A late response cannot open a second box.
- Account/session change, a different `curBattleData`, a different `fightId` or
  a playback battle cancels the job and destroys only this adapter's box.

The adapter never writes `isLastBattleWin`, `lastBattleEndMsg` or battle damage,
never calls `End`, and never fabricates a win, a reward or a socket response.

## Files

- `LocalFieldSettlement.cs` — new `LocalFieldSettlement.dll`; only static
  `Observe(object, object)` and `Completed(object, object)` are injected.
- `Injector.cs` — dnlib binder. It validates the original local variable,
  send opcode, handler shape and absence of previous hooks, then inserts the two
  hook sequences. It adds no original type, field or method.
- `build.py` — copies the main DLL from an explicitly SHA-256 pinned APK,
  compiles the helper and injector with the pinned local toolchain, injects,
  verifies all other bodies byte-for-byte, and writes the managed overlay plus
  `manifest.json`.
- `README.md` — this note.

## Build contract

```bash
python3 tools/client_field_settlement/build.py \
  --base-apk <explicit base.apk> \
  --base-sha256 <64 lowercase hex> \
  --output-directory <fresh dir inside the project> \
  [--tool-root <read-only dotnet/csc/dnlib root, default .tools/native-batch>]
```

`--expected-sha256` is accepted as an alias for `--base-sha256`. The output
directory must be fresh and inside this project so `package_native` can use it
as a managed overlay. The command reads the pinned toolchain from
`--tool-root` and never writes to it.

Packaging is a separate, explicit step:

```bash
python3 tools/client_release/package_native.py \
  --base-apk <same base.apk> --overlay <output-directory> \
  --output <new.apk> --record <record.json>
```

`build.py` does not package, install, launch the game, edit resources or access
a device.

## Verification

Static, per build:

- every original metadata table, field/type ABI, assembly identity, CLI flag and
  existing assembly reference is unchanged; only one `TypeRef`, two `MemberRef`
  and one `AssemblyRef` are appended;
- every method body except the two named methods is byte-identical, including
  all `Battle.Engine` bodies; the engine count is read from the pinned base
  instead of hardcoded;
- the two target bodies differ only by the three inserted instructions; the
  existing `try/finally` around the formation loop in
  `AskFieldPVEFightResult` keeps the same boundaries;
- the helper assembly targets the old Unity runtime, exposes only the static
  `Observe`/`Completed` hooks, with private retry/lifecycle methods and nested coroutine state, and
  contains no call named `End`, `Invoke`, `set_isLastBattleWin` or
  `set_lastBattleEndMsg`;
- all Java/Dex/native entries are inherited unchanged and recorded by SHA-256.

Not done here (main agent owns these): running `build.py`, packaging, phone
reproduction, forced server failure, reconnect recovery and final acceptance.

## Known limits

- Only type12 settlement is covered; type18 remains on its own bridge.
- A server that keeps failing cannot be recovered by retrying. The box then
  offers return to login; the server keeps its real pending/settled state.
- The retry assumes the socket layer eventually reconnects and delivers the
  original `30086` callback. `SocketController.ReConnect` keeping its
  `EventManger` is static evidence, not a device result.
- `BoxManager.CreateMessageBoxWithCofirmAndCancelButton` is public original code
  and loads `UI/MessageBox/MessageBoxWithConfirmAndCancelButton`, but it has no
  other call site in `Assembly-CSharp`. Its runtime appearance and input
  blocking need the device check.
- Leaving the battle scene while waiting cancels the retry by design; it does
  not attempt to restore the battle from another scene.
