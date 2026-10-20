# client_stamina_sweep

Bounded overlay for the original stamina potion purchase page and automatic copy-sweep stop.

- `Stamina.cs` — new `SoulFlowerStaminaSweep.dll`: reuses original BuyItemPage labels/font/buttons for `item_002`, and tracks sweep stop/generation state.
- `Injector.cs` — dnlib binder for seven pinned original methods. It adds no original fields or methods.
- `build.py` — compile helper + injector, inject, and verify raw-body/ABI/token/engine invariants. Writes `Assembly-CSharp.dll`, `SoulFlowerStaminaSweep.dll`, `manifest.json`.
- `scan.py`, `inspect_prefab.py`, `decode_buyitempage.py`, `list_prefab_buttons.py` — read-only evidence helpers.

Usage:

```bash
.analysis-venv/bin/python tools/client_stamina_sweep/build.py \
  --output-directory analysis/playability31-20261020/client/overlay-stamina-sweep-r1
```

This does not package, install, launch, modify resources, or touch `build/hyjs-local.apk`. Device/visual checks stay with Root.
