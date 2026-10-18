# Native display and saved 60/120 FPS

This display-only overlay uses the qualified b982 APK for the original Unity 5.3,
mscorlib and Android manifest. It applies the useful Re 1.0.5.1 display recipe
without importing its offline account, network, economy or window policy.

The game assembly changes five methods:

- `UIAutoSize.Awake`: remove only the old 1280-wide `Screen.SetResolution` call
  using equal-width stack pops; preserve size classification, NGUI FitBoth,
  design dimensions, anchors and existing safe-region handling.
- `FPSFixer.Start`: call the small `SoulFlowerDisplay` bridge to apply the saved
  60/120 FPS cap, full textures/LOD, and foreground/resume policy.
- `FPSFixer.Update`: return immediately, stopping the old main-thread sleep and
  adaptive cap. Its private old Refresh has no remaining direct caller.
- `RenderManager.get_MaxFPS`: read the saved 60/120 selection independently of
  game graphics quality.
- `UI.QualityBar.Start`: attach a small original-style frame selector to the
  existing player-info graphics-quality area.

`SoulFlowerDisplay.dll` is compiled only against the original APK's mscorlib,
UnityEngine and Assembly-CSharp. Its metadata runtime remains v2.0.50727. It
stores `soulflower_target_fps` in device PlayerPrefs, defaults to 60, accepts only
60/120, saves on selection, and updates all current FPSFixer instances. Newly
created FPSFixers and focus/resume events reapply the saved cap. The game's saved
low/middle/high quality remains independent; no game quality preset is forced.
Texture and maximum-LOD limits remain zero, matching the bundled asset defaults.
MSAA/FXAA and simulation timing are unchanged.

The new row uses the existing quality bar's atlas/background and label font.
The pinned prefab `9c299c378f30e464dab999f1b7db6f2b` places the quality row at
y=-154 and music at y=-214 in the common Center coordinates. Their click targets
are 40/44 high, so the quality row moves 8 units up and music 8 down before the
new 24-high selector is placed at y=-186. This avoids overlapping touch targets
and retains all original quality buttons,
checkmarks and callbacks. No clone carries inherited button events or localized
quality labels. Actual layout and touch targets require the combined phone check.

```sh
rtk proxy .analysis-venv/bin/python tools/client_display/build.py \
  --output-directory analysis/display/overlay-r1
```

The optional composition input must have an explicitly supplied exact SHA:

```sh
rtk proxy .analysis-venv/bin/python tools/client_display/build.py \
  --input-managed analysis/playability-fixes-20261018/client/overlay-r3/Assembly-CSharp.dll \
  --input-managed-sha256 2f594ee939830b12ec9d9f4f1d24566571384e06ed865e0b069429ad834a0c55 \
  --output-directory analysis/display/combined-r1
```

Use fresh output directories. The verifier binds the input, guards all five
original target bodies, preserves method tokens/ABI/flags/metadata, and requires
every non-display raw method header/IL/EH to remain exact relative to that input,
including all 547 original Battle.Engine methods. `verify(before, after,
expected_input_sha=...)` and `verify_bridge(raw)` are available for integration.

Outputs are `Assembly-CSharp.dll`, `SoulFlowerDisplay.dll`, logs and manifest.
The bridge must be included alongside the patched game DLL. For playability r3,
compose playability first and display second, then preserve its separate
LocalPlayability.dll and LocalFormationUI.dll. The display tool does not modify
these adapters. Packaging/signing, installation and promotion belong to the
release coordinator.

The base Android manifest stays exact (SDK28, no explicit aspect cap). Do not
copy Re's target24/aspect2.1 manifest. Native resolution means the login/register
window must use current Screen or Android view dimensions, not a fixed 1280-wide
rendering assumption; the managed overlay cannot correct absolute native-dialog
coordinates. Retain the current modern IME and Android insets.

60/120 are requested caps. Achieved presentation rate depends on hardware,
display refresh, scene complexity, heat and power policy. No actual frame-rate,
full-screen-layout or anti-aliasing benefit is claimed from static checks.
