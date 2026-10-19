# Formation model load completion refresh

The original UI starts loading models only after `_Init` waits 0.5 seconds. The
initial formation-list callback selects the tab and performs the 82000 read.
When that read returns before the model coroutine completes, `SetModelPos` sees
an empty model list. Every loaded model is initially inactive. The coroutine's
last `UIToggle.Set(true)` then returns immediately for an already-selected tab,
so it never repeats the placement/activation callback. A low-latency local server
makes this race visible.

This overlay preserves the original coroutine and adds one direct invocation of
its original `OnTabClick(currTab)` immediately after the final toggle selection.
It repeats the existing authoritative 82000 read after loading, then uses the
original `SetModelPos`, activation, sizing and drag code. The original toggle's
selection behavior stays in place. No resource, renderer, camera, combat engine,
formation data, level gate or mutation request is changed.

```sh
rtk proxy .analysis-venv/bin/python tools/client_combat_visuals/build.py \
  --input-managed <current composed Assembly-CSharp.dll> \
  --input-managed-sha256 <explicit SHA256> \
  --output-directory analysis/<batch>/formation-model-overlay
```

The tool accepts preceding overlays only while its original target pin remains
unchanged. Only `<CreateModels>c__Iterator14F::MoveNext` changes; every other raw
method and all 547 original Engine methods are checked. There is no new helper
assembly or AssemblyRef. Root composes and packages the single final APK.

Evidence is under `analysis/play-feedback-20261019/combat/`: original model,
toggle, tab-callback CIL and the emitted manifest. Static control-flow proof does
not assert Unity/phone rendering success. Open the actual 调整阵型 popup after
integration and confirm models appear on first opening and survive drag/save/
reopening. The separate `BattleFormationPopupPage` is outside this patch.
