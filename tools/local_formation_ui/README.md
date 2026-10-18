# Isolated original formation UI bootstrap (r3)

The small module adds an explicit “下阵” control to an already initialized original ArrayPage. Automatic polling attaches UI only. The user must click the control to send original21049; no background business request is added. No Go, SQL, device, APK packaging or installation is performed by this builder.

## Retained failures and r3 boundary

r1's OnHeroClick hook compiled, but observed original model taps never reached it. r2's final DoOpen hook also preserved all original IL and compiled, but real5e8a phone acceptance displayed only prefab name/Lv.0/empty slots and no new button. That failure remains in `analysis/growth-first-20261016/wallet-growth-joint-phone-r1/summary.json`; no21049, currency use or combat occurred. The phone returned Town/noRun/Appclosed with exact4/7 player/game, wallet and protected1 hashes. Absent app logcat lines do not establish a loader root cause.

r3 removes the added DoOpen call completely. Its output main DLL is the exact qualified90450ff8 base: every ArrayPage method is restored. The already running LocalWalletSessionBridge's original .5s Update tick can call an optional private reflection bootstrap after its player/session/blocked-session checks and before its unchanged-session early return. The proposal additionally requires an existing bound local player>1/session pair, attempts once per page entry, catches any module/invoke exception, and reports only exception type/fixed descriptions through Android `HYJSFormation`, deduplicated to16. Missing or invalid optional modules therefore leave original ArrayPage JIT, native wallet binding and payment refresh intact. This isolates failure; it does not establish its runtime cause or prove successful attachment.

Owned wallet proposal: `LocalWalletSessionBridge.proposal.cs`, `wallet-bootstrap.patch` and `wallet-bootstrap-proposal.json`. Root alone applies/builds that shared wallet module. The proposal changes no public ABI and no Java wallet classification code. Formation's public reflection entry is `HYJS.LocalFormation.LocalFormationUI, LocalFormationUI::Ensure(UI.ArrayPage)->bool`; the attached `HYJS.LocalFormation.FormationControls` is a top-level MonoBehaviour.

## Visible control and authority

Original prefab `assets/bin/Data/f543d31e1e9b8fa49a1ac4e0077bf451` contains BtnProperty/UIToggle735, with Transform306→UISprite623 and Transform308→UILabel780. Their background atlas/sprite and font supply the theme. A fresh110×46 BoxCollider/UIEventListener, caption “下阵” and local position(40,230,0) are attached to ArrayPage above the model. No UIToggle, TextKit or original event bindings are copied. Final visible placement/hit area is still subject to the combined phone acceptance.

Each click resolves the selected real heroGuid and current inventory/owner/camp/session. Main heroes receive a no-mutation hint. Mounted non-main heroes must first remove equipment, treasure and pet. Only a naked non-main hero sends original21049 CSFormationOff. After acknowledgment original20111 CSFormationList must validate in full before cache mutation. Failure before acknowledgment preserves the original cache. A committed mutation with failed fetch retries only20111. Stale sessions ignore callbacks. The adapter closes ArrayPage while old cache is valid, releases absent naked visual camps through original UnloadHero, then uses original camp constructor, InitBattleCamp, UpdateFormationData, ResetAllFormationAttribute and navigation refresh before reopening on the main hero. Ownership/inventory remain intact. The compatible server omits empty no-hero protocol FormationItem rows while retaining internal slots.

## Qualified overlay build

Input is current5e8a APK/main974c and `wallet-activity-catalogue-r1/joint-apk-r1.json`; restore input is exact90450 main from `first-double/title-overlay-r2/Assembly-CSharp.dll`. The old direct injector is retired and refuses execution.

```sh
rtk proxy .analysis-venv/bin/python tools/local_formation_ui/build.py \
  --base-apk build/hyjs-local.apk \
  --base-proof analysis/growth-first-20261016/wallet-activity-catalogue-r1/joint-apk-r1.json \
  --output-directory analysis/formation-bootstrap-new-overlay \
  --enable-local-formation-ui
```

A new directory and explicit opt-in are required. Outputs are restored Assembly-CSharp.dll, newly compiled LocalFormationUI.dll and `local-formation-bootstrap-overlay-v1` manifest. Relative to current5e8a, only UI.ArrayPage::DoOpen differs, by removing the exact six added IL bytes; all26945 other raw method headers/IL/EH and547 Engine methods remain identical. The restored whole DLL SHA is pinned. Manifest declares originalArrayPageAllMethodsRestored/bootstrapUsesReflection and source pins. Root combines these with its separately compiled wallet proposal, retaining current Java20acd/5012e6 entries and other qualified APK contents.

r3 compilation and byte preservation passed. No new APK/phone success is claimed. Next combined native acceptance must verify original Array initialization, actual button visibility/main hint/naked non-main click,21049→20111/cold recovery, then legitimate two-actor Trial/Awake501 paths. Battle input, resources, engine/EH, golden results and qualification guards remain unchanged.
