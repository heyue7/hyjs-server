# client_arena_state

Bounded original-client overlay for two arena UI state defects:

1. Occupying an empty point in one grab field, switching to another field and
   coming back loses the occupation display until the next server refresh.
2. Opening the fragment selection page briefly shows the four prefab target
   slots, then hides them all when the server returns an empty rival list.

The server contract is unchanged: `p11PlunderListAt` still publishes only real,
eligible fragment holders, and no robot, target, reward or qualification is
fabricated on either side.

## Original faults and evidence

| Symptom | Original method | IL evidence |
| --- | --- | --- |
| Stale grab display after field switch | `UI.GrabPVPItem::<OnGrab>m__118` | Calls `GrabPVPItem.SetItem(result.grabData)`, `GrabPVPPanel.UpdateTimes`, `GrabPVPPanel.UpdateEnergy`; never updates `GrabPVPPanel.grabList` or `isSelfGrabed`. |
| Same | `UI.GrabPVPPanel::OnTabClick` | Renders each of the six items from the cached `grabList` only; no request while switching the three fields. |
| Same | `UI.GrabPVPPanel::ReShowTab` / `ShowTab` / `OnRemoveCoverPage` | All three call `Refresh()`, which clears `grabList` and re-sends `CSGetGrabData`; that is why switching to 夺宝 and back repaired the display. |
| Four prefab targets then empty | `UI.PlunderSelectPage::DoOpen` → `DelayGetRivals` → `GetRivals` | `DoOpen` never hides `targetItems`; `DelayGetRivals` waits `delayTime` (0.5s) before `GetRivals`. |
| Same | `UI.PlunderSelectPage::GetRivalCallback` | Clears every item, `SetActive(true)` + `SetData` for `infoList.Count` entries, then hides the remaining slots. Empty `infoList` hides all four. |

The give-up path also ends in `GrabPVPItem.SetItem`:
`<CancleGrab>c__AnonStorey190::<>m__119` sets `isSelfGrabed = false` and calls
`SetItem(result.grabData)`. One `SetItem` hook therefore covers occupation and
give-up.

## Change

| Hook | Pinned method | Inserted before | Inserted IL | Purpose |
| --- | --- | --- | --- | --- |
| 1 | `UI.GrabPVPItem::SetItem(ProtoBuf.Message.GrabPositionModel)`, body `d5f97529…` | final `ret` (`IL_00ef` in the analyzed DLL) | `ldarg.0; ldfld grabPanel; ldarg.1; call ArenaStateSync.SyncGrabCache` | Update the matching `grabList` position with the same server frame and recompute `isSelfGrabed`; does not replace an element when it is the same reference, so the original `OnTabClick` enumeration never sees a `List` version change. |
| 2 | `UI.PlunderSelectPage::DoOpen()`, body `6b30f53d…` | first instruction (`IL_000c`) | `ldarg.0; call ArenaStateSync.ClearPlunderTargets` | Clear and hide all prefab target slots before the original 0.5s request; the original request, delay and callbacks remain. |
| 3 | `UI.PlunderSelectPage::GetRivalCallback(NetWork.Packets.Packet, bool)`, body `ba16b12f…` | final `ret` (`IL_00d0`) | `ldarg.0; ldloc.1; call ArenaStateSync.ShowEmptyRivals` | Notice only on the success fall-through; the failure branch jumps straight to the original `ret`, so failures are never reported as empty. |

The last hook is inserted before the `ret` object, not at the shared success
fall-through offset: the existing `brfalse` failure branch keeps targeting the
original `ret` and skips the notice. A closed/inactive page is also skipped by
`ShowEmptyRivals`.

`HYJS.ArenaState.ArenaStateSync` (new `HYJS.ArenaState.dll`) contains:

- `SyncGrabCache(GrabPVPPanel, GrabPositionModel)` — reads the original private
  `grabList` through a cached `FieldInfo` (the field is `private` and cannot be
  referenced directly from another assembly), updates the matching position,
  and recomputes the public `isSelfGrabed` flag from the cache. An empty cache
  (original `Refresh` in flight) keeps the current flag instead of guessing.
- `ClearPlunderTargets(PlunderSelectPage)` — `Clear()` + `SetActive(false)` for
  each prefab slot.
- `ShowEmptyRivals(PlunderSelectPage, SCTreasureBattleList)` — if the page is
  still active and `infoList.Count == 0`, shows the original
  `BoxManager.CreatePopupTextBox` with
  `当前没有可夺取该碎片的对手，请换个碎片或稍后刷新。`.

No server message, APK resource, Java/Dex entry, target id, chance, cost, timer
or reward is changed. The helper never adds a rival and never alters the
original protocol.

## Files

- `ArenaState.cs` — helper assembly source (`HYJS.ArenaState`).
- `Injector.cs` — dnlib binder. It validates the three method identities, the
  `SetItem` listener tail, the `DoOpen` entry, the `GetRivalCallback` success
  fall-through and the `SCTreasureBattleList` local, then appends only the nine
  instructions above.
- `build.py` — copies the main DLL from the pinned APK, compiles the helper with
  the old Unity runtime, injects, verifies all invariants and writes
  `Assembly-CSharp.dll`, `HYJS.ArenaState.dll` and `manifest.json`.

## Usage

All three path arguments are required. `--tool-root` may point at a read-only
project tree that owns `.tools/native-batch` (dotnet, csc, dnlib); it defaults to
this project.

```bash
.analysis-venv/bin/python tools/client_arena_state/build.py \
  --base-apk <base.apk> \
  --expected-sha256 <base-apk-sha256> \
  --output-directory <fresh dir inside the project> \
  [--tool-root <project with .tools/native-batch>]
```

Build/packaging stays with Root. The build refuses a base APK whose three pinned
original method bodies differ (a transport-only QA APK with the same managed
bodies is accepted), refuses an existing output directory, and re-checks the base
APK after injection.

For packaging and device acceptance, place the tool inside the project that runs
`package_native.py` (or keep the overlay and source pins under that project root),
then:

```bash
.analysis-venv/bin/python tools/client_release/package_native.py \
  --base-apk <same base.apk> \
  --overlay <output-directory> \
  --output <new apk> \
  --record <new record>
```

`package_native.py` re-checks `baseAPKSHA256`, `inputManagedSHA256`,
`inputDexSHA256`, `allowedChangedMethods` (exactly the three above), the 547
engine methods, every inherited Java/Dex/native entry and the source pins.

## Static verification in `build.py`

- Base APK SHA equals `--expected-sha256`; the analyzed original managed DLL
  hash (`ee14d5df…`) is recorded, and each of the three target bodies must match
  its own raw pin.
- Each of the three target bodies is uniquely pinned by raw SHA-256 and
  signature (`2001011281d8`, `200001`, `20020112a83c02`).
- Each target body differs only by the exact inserted block at the pinned
  anchor; opcodes, operands, branch destinations, switch targets and exception
  handlers of all original instructions are preserved.
- Every other original method body is byte-identical, including all 547
  `Battle.Engine` methods.
- Original type/field/method/parameter ABI and tokens, module MVID, assembly
  identity, CLI flags and strong-name state are unchanged.
- Metadata additions are only `TypeRef +1` (`HYJS.ArenaState.ArenaStateSync`),
  `MemberRef +3` (the three helper calls) and `AssemblyRef +1` (unsigned).
- Helper assembly: `v2.0.50727`, one type, four static methods (three injected
  entry points plus the private cache getter), references exactly
  `Assembly-CSharp`, `mscorlib`, `UnityEngine`, and the notice text/field name
  are present.
- All Java/Dex/class/jar/native-library entries are recorded and inherited
  byte-exact; only the two managed DLLs are payload entries.

## Not verified here

No C# compilation, APK packaging, install, launch, phone action, database,
network request or server change was performed while authoring this overlay.
The build must be run and statically verified by Root; device checks:

1. Occupy an empty point in one grab field, switch fields and return: name and
   timer remain without a manual refresh.
2. Give up the point, switch fields and return: the slot is empty and the
   original abandon/grab prompts match the server.
3. Open the fragment page: no prefab portraits during the 0.5s delay; real
   rivals appear as before.
4. With no eligible rival: empty page plus the notice; a failed request keeps
   the original error and shows no empty notice.
5. Close the page before the reply: no late notice appears elsewhere.

A stale cache can still show other players' later changes only through the
original 60s `UpdatePanelEveryMinute` refresh or a tab re-entry; this patch adds
no periodic refresh and no extra network traffic beyond the original requests.
`PlunderSelectPage.DoReopen` is not hooked: it already clears the slots and
requests directly without the 0.5s delay, and it was outside the confirmed fix.
