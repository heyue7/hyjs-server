# Local Awake501 resource readiness

## Current policy — 2026-10-17

The user requested removal of resource-readiness rejection at both awakening
entries. `RCUINormalNPC.OpenPage` and `AwakeDiffItem.OnChallengeClick` now follow
their normal eligibility flow without querying the aggregate full getter or
LocalAssetReady. Preserve global flags, saved state, gameplay requirements and
the actual loader/battle/result code. Address concrete missing files or logic
only when encountered. See `remove_gate.py` and
`analysis/p7/awake-gate-removed-20261017/`. Removal is not a gameplay pass.

The scoped inventory, bridge admission and diagnostic instructions below are
historical. Do not reinstate them or schedule a separate gate investigation.

## Entry diagnostics on qualified28ea

The observed NPC310 toast with no verification log remains a runtime diagnosis,
not a proven actor or predicate bug. The actual28ea hook and bridge, original
NPC310 row and Init stores all match the intended type/name/option/level checks.
The scoped bridge now emits `Local501 catalogue-entry` before the predicate,
then public NPC fields and a distinct verification/quality/party/loader/cell
rejection reason. Repeated identical messages are deduplicated; the diagnostic
budget is64 messages per process. No credentials or player identity are logged.
All admission predicates and the resource inventory remain unchanged.

For the next combined unload build, rebuild only the existing DLL:

```sh
rtk proxy .analysis-venv/bin/python tools/local_asset_ready/build_bridge.py \
  --base-apk build/hyjs-local.apk \
  --base-proof analysis/growth-first-20261016/first-double/title-apk-r1.json \
  --output-directory analysis/p7/awake-assets-ready-r3-20261017/bridge-diagnostics-next
```

This mode binds28ea and its qualified proof, validates all2680 existing resource
entries, and emits only `LocalAssetReady.dll`. It does not reinject either hook
or rewrite Assembly-CSharp.dll. Merge its one replacement entry into the next
explicit unload overlay; do not install a separate diagnostic APK. The offline
component harness has25 checks, including pre-predicate logging, unchanged
negative admission and the bounded log budget. Actual Unity/device behavior
is still pending. See the independent r3 diagnostic record for the two-click
maximum runtime plan and inherited547-method protection.

This is a bounded local resource policy for the existing packaged chapter5/copy501 assets. It does not restore the original full downloader. It does not change saved `full`, `downover`, `GameConfig.isFull`, `get_IsAssetFull`, the original loader, battle engine, protocol payloads, inventory or results.

The original phone attempt reached NPC310 but failed before catalogue/network access. The original aggregate full flag covers content that this small APK does not own: 743 bundle graph members are absent. The verified chapter5/copy501 closure itself is present: 755 dependency bundles, root manifest/small manifest and 1923 built-in Unity data files (2680 entries). There is consequently no justified missing-file download/addition for this particular closure. The adapter accepts a verified scoped closure at the two original gates while leaving the global full gate unchanged everywhere else.

## Scope

- NPC page `ChallengeActivityCopyPage`, option `type=0`, page level70 (original NPC310 row).
- Challenge entry copy `501`, chapter `5` only.
- Original render quality `2` (high); quality settings are never changed automatically.
- Current formation visual families `hero_001`, `hero_003`, optional `pet_002`; other actors remain closed. This resource scope is independent of battle certification and does not grant combat authority.
- Original local `mon_030` uses original `monster_033`; its original scene, ordinary party models, weapons, skills, particles and audio are included. Built-in UI/atlas data must be byte-identical.
- If the original full getter already returns true, its original behavior is retained.

The two hooks are exactly `Room.RCUINormalNPC.OpenPage` and `AwakeDiffItem.OnChallengeClick`. Each retains the original getter and branch, adding `ldarg.0; call <adapter>` immediately between them. Their original instructions, branch destinations and exception regions are checked after removing just the added instructions. All other method bodies are compared, including all547 original engine methods with exact instruction bytes and exception regions.

## Runtime behavior

The first eligible click starts a background hash check of the installed APK and existing persistent overrides. Until it finishes, the original resource toast remains. Retry the NPC after the check; the log reports `Local501 resource verification state=2` on a successful subsequent call. The bridge then checks original loader initialization and loaded manifest cells (size/MD5). An override being added/changed invalidates readiness and requires another hash check. Corrupt bytes fail closed; restarting after a genuine cache repair starts a fresh check. No arbitrary cached file is deleted or rewritten.

The verifier reads `Application.dataPath` and the exact original `GetFullPath` override convention `persistentDataPath/<lower-case bundle path>`. No endpoint, credential or player state is written.

## Build only the small overlay

Use the project Python environment; no Unity editor, Go, SQL, phone or APK composition is involved:

```sh
rtk proxy .analysis-venv/bin/python tools/local_asset_ready/build.py \
  --base-apk build/hyjs-local.apk \
  --base-sha256 f5006a543e4001d01b32fa6dd4b7475859266ef278d6d37b99ffd830bf5a9a72 \
  --base-proof analysis/growth-first-20261016/wallet/client-build-wallet-style-ime-r3.json \
  --output-directory analysis/p7/awake-assets-ready-r2-20261016/overlay-next
```

Output directory must be new. The builder validates the exact current qualified APK, its wallet/IME entries, and the original closure proof. It compiles `LocalAssetReady.dll` against the actual original Mono references and patches the current qualified Assembly-CSharp.dll in a temporary directory. It never starts from the original unpatched game DLL. Temporary compiler/project artifacts are cleaned automatically. Component checks are separate: `rtk proxy .analysis-venv/bin/python tools/local_asset_ready/test_bridge.py` (19 checks, .NET stubs; no Unity loader evidence).

## Root integration proposal

The root packager should accept the exact overlay manifest and replace/add only:

1. `assets/bin/Data/Managed/Assembly-CSharp.dll` with the checked overlay DLL.
2. `assets/bin/Data/Managed/LocalAssetReady.dll` with the compiled bridge, which embeds `local501.inventory`.

All other APK entries, including wallet r7 bridge/UI/assets, modern IME classes.dex/classes3.dex, AndroidManifest.xml and native libraries, must retain their previous entry SHA. No new Java/Android asset/permission is needed. Compose on f500, retain f500 rollback, zipalign/sign/verify with existing protected contracts, and record the new APK SHA and both qualified proofs. Do not invoke the old original-APK patch flags or reinject the wallet bridge. Existing wallet proof has an old Assembly SHA: the new composition record must explicitly bind this additional allowed two-entry overlay rather than claim whole Assembly byte equality.

## Pending acceptance

This source/module delivery does **not** count as NPC/exploration completion. Root must later verify high-quality normal-party original NPC310 → original catalogue501 → four real boxes + original monster/type16 → real score/settle → actual awakening material use → cold relogin. Preserve the prior gate failure. Also sample an out-of-scope copy/quality/actor to confirm it remains closed when original full is false. Backend chain proofs are reused; no new server-side exactness is claimed.
