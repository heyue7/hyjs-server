# Copy chest instance sync: partial candidate for 1.1.2

This independent managed overlay is pinned to APK SHA-256 `408e9d03291d6f3505fb91590134b25a8b6aa68f3ebde9c2f4ff985299baccb3` and main assembly SHA-256 `3d8f50e543ad082ea03bff294f17421a2eed4316d5cda664b03cc0f46cc23e8a`. It has not been packaged, installed, device-tested, or deployed.

The original open and exit callbacks still run. After a successful exit, the helper waits for the native HTTP queue and network UI to be idle, then serially reads the native warrior and equipment lists. It merges only captured chest GUIDs that are still present in those lists, retaining every existing instance and its formation references. The original channel shows its ordinary wait mask and original timeout dialogue. Three failed sends, callbacks, or timeouts stop a drain; a later successful chest exit can start a fresh bounded drain. A send rejected synchronously clears only its own mask after the channel, account, generation and existing dialog checks; an intervening normal request is never cancelled. Multiple chests are batched per exited room and late callbacks are invalidated.

**Stackable items and fragments are deliberately excluded.** Their IDs are captured while the room is active but are never applied to inventory. The HTTP-send observer and local object fingerprints cannot detect every independent inventory write or cover the interval between a side-channel server commit and its local callback. The normal full login loads item and fragment lists again. A complete automatic correction requires a server-authoritative ordered revision or equivalent complete causal contract and must also protect against stale responses. Immediate per-box increments with a local dedupe key have snapshot and crash counterexamples covered by `ModelTests.cs`. Currency remains under native SCPlayer handling.

The retained generation identifies account, player object, player ID, and session; independent exact attempts reject delayed callbacks. Overflow rotates a distinct generation epoch. The native send observer records only; it does not alter the original send or its return value. A pending item is discarded on a new account or session. The worker does not persist its pending set, and process termination falls back to the original full login.

The list confirms existence only at the server snapshot. The observer protects original same-client HTTP requests and changed local objects; it cannot establish the order of arbitrary external GM/admin/other side-channel mutations after that snapshot. This is a partial client candidate, not proof of complete distributed inventory convergence. Production or device acceptance must cover real animations, scene transitions, net-mask timeout/retry, another request issued around the snapshot, session changes, stale replies, live formation references, and both player and server inventory. No compensation is sent by this module.

The original socket reconnect does not load inventory. A forced full BackToLogin is not used: the actual 1.1.2 BackToLogin calls `LoginCache.pause`, which clears the account and role wallet sessions; the original login chain may save or clear persistent credential settings and may select/persist a fallback realm. Thus its safe automatic reuse without altering persisted login data is unproven.

Build only the managed overlay and model/ABI checks (no APK packaging):

```sh
rtk proxy /Users/lirui/LR/hyjs/.analysis-venv/bin/python tools/client_copybox_sync/build.py \
  --base-apk /Users/lirui/LR/hyjs/deliveries/client-update-1.1.2/黑衣剑士灵魂之花v1.1.2-修复候选.apk \
  --output-directory tools/client_copybox_sync/.build/<new-owned-directory>
```
