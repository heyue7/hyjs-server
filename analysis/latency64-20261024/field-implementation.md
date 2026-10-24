# Field64 implementation and static proof

Implemented the root-approved correction in tools/client_field63 only. No backend, package, phone, DB or browser changes were made by this task.

- Patrol preserves server anchor/UUID/monId and original correction behavior. Samples use owner Y, original ten-metre search, bounded source offset (XZ 0.5m/Y 1m), target/corner elevation difference <=2m, anchor XZ R2 limit, complete paths and original StopTo. It keeps 8 candidate attempts per second, speed3.2, original animation and all state/pause gates. No warp/transform write/straight-line fallback was added.
- Fixed-size local diagnostic fields expose saturating attempt/start/arrival/failure counters, latest reason and latest source/target; there is no per-frame logging, network field or persistence. Boss gets only contact checking and never Bind/Advance.
- New BossCollisionTick(object) handles RCUIFieldBoss at 0.1 unscaled seconds, retaining original distance3m, AutoPathFinding gate, original OnClick and one-second retry cadence. OnClick CIL is untouched, preserving state, invincibility, current HP and SendPveBoss behavior.
- Builder pins published63 APK/Main/helper, validates and retains all three existing hook bodies and object ABIs, and injects only Boss Tick. This preserves published shop/dungeon/other fixes. The original helper filename and assembly identity stay HYJS.Field63.dll for existing callers.

Single managed build completed successfully with zero C# helper/injector warnings/errors. Static proof: exactly one changed Main method (Room.RCUIFieldBoss.Tick), all 547 original engine methods exact, 26945 unrelated raw bodies exact, metadata ABI/tokens preserved, pure-object hook ABI and correct Room type scopes verified. No APK was built. Build manifest and SHA pins are below; documentation source pins were refreshed after README-only edits without rebuilding binary.

{
  "baseAPKSHA256": "4a85ed009e86c649b5954e8d1f02165e9d1951ec89256196f3dccd9076e6183d",
  "inputManagedSHA256": "6ee235813aa4c40b3d0e447857fec5248f4439c47828cb5e76553293a464bf57",
  "outputManagedSHA256": "57f12e1c1aaa0ee46bc00d12be6b06bba5fda16533b47cc50a02c19c5e5e4f22",
  "replacementEntries": {
    "assets/bin/Data/Managed/Assembly-CSharp.dll": "57f12e1c1aaa0ee46bc00d12be6b06bba5fda16533b47cc50a02c19c5e5e4f22",
    "assets/bin/Data/Managed/HYJS.Field63.dll": "7b34e256ad4965657776adc1729d27b231e607b29b05b7e6cbc5ff282788833a"
  },
  "retained63HookCount": 3,
  "newBossHookCount": 1,
  "allowedChangedMethods": [
    "Room.RCUIFieldBoss::Tick"
  ],
  "originalEngineMethodsExact": 547,
  "unrelatedRawMethodBodiesExact": 26945,
  "originalMetadataABIAndTokensPreserved": true
}

Output: build/field64-overlay/manifest.json. Device movement for every instance, map geometry, gate behavior and latency remain root QA responsibilities. Static evidence supports the height/selection defect hypothesis; actual per-instance rejection causes remain unverified until runtime diagnostics are inspected.

No own background process remains. The build's temporary directory was automatically removed; the sole current managed overlay (~4.5MiB) is retained for integration. No product data, shared resources or other agents' files were removed.
