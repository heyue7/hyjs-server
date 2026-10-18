# Three original character artwork fallback

For the locally obtainable original characters `hero_069` (凪), `hero_070` (罗亚), and `hero_073` (克洛维), their original `heroheader/hero_icon_XXX` and `hero2d/hero_2d_XXX` files are missing in all inspected APKs. Each has its own complete original `heropainter/hero_card_XXX` painting. This overlay references those original paintings and changes per-widget UVs; it does not generate artwork, replace PNGs, modify the reward pool, or alter models.

The six exact missing bundle names are aliased at the original asset-manager load, check, lookup and unload boundaries. Lookup also maps the corresponding asset name. All other paths and asset names pass through unchanged. This preserves original dependency/refcount behavior and shares the original texture without changing its pixels.

`TextureKit` and the battle hero head widget receive a face crop only for those three original painting textures. `AloneTextureKit` records whether its original request was a missing recruit card; its callback applies an upper-body crop for that case. Original detail-painting requests keep their original full UVs. A small component restores original UVs when pooled widgets are cleared or reused by other characters.

Only the three exact IDs bypass `IsShowInCard` in `AllHerosPage.SetHeroList`. All other atlas filtering stays intact. The original tables, 3D loading, animation, weapons, battle logic, resource manifests and APK assets stay unchanged. Backend collection-count policy is separate.

## Build and compose

```sh
rtk proxy .analysis-venv/bin/python tools/client_hero_art/build.py \
  --input-managed <input Assembly-CSharp.dll> \
  --input-managed-sha256 <explicit input SHA256> \
  --output-directory <fresh output directory>
```

The default bound input is `5b6ba3af357c36f7e81d7ad82f4ba31157eda6d84dc6ec8f18a1314a3604c8f4`. A composed input is allowed only with its explicit SHA and every original target-body pin unchanged. The verifier permits 11 target methods, checks every other original raw body, original ABI/token inventories and all 547 original engine methods. Prefix targets retain original control flow and exception boundaries; the remaining targets change one field read or texture setter call each.

Final packaging must include both output `Assembly-CSharp.dll` and `SoulFlowerHeroArt.dll`. The helper references host types through the original module signature. No Java or asset payload changes are required. The integration owner handles union verification, packaging, signing, runtime checks and delivery.

## Validation boundary

```sh
rtk proxy .analysis-venv/bin/python tools/client_hero_art/verify_representatives.py \
  --output <representatives.json>
```

The bounded harness executes the actual helper source with Unity UI stubs: aliases, unrelated paths, the three atlas exceptions, UV bounds, widget reuse, clearing, normal cards and full detail paintings. It does not instantiate Unity, load AssetBundles on a phone or claim actual rendering success. Face and card boxes were chosen after decoding and visually reading the original 1024×1024 paintings. A short existing-character/profile/preview check remains required after integration.

The reported 3D model blank has no proven root cause from static evidence; this overlay intentionally leaves its loading path unchanged. Existing model bundles, meshes and dependency closures were verified present. Observe the actual bundle/pool/callback/visibility state before choosing a model fix. The old `UI.ActorItem` uses the separate legacy `ResourceManager`/`Resources.Load` path; this patch does not claim to repair that obsolete formation UI path.
