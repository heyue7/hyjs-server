# P9 equipment five-cost UI overlay

Only changes `UI.PetEquipStrengthTab.GetFiveCost` in the qualified installed 692767bc APK's patched Assembly-CSharp.dll. The input DLL must be `0be3d908fdec86aecc588c024900f7068e3bf0ef578b1cf8fb497532ed4b67b6`. It preserves inherited awake gate removal, wallet and all other managed methods. It never repatches the original ee14 DLL, packages an APK, operates a device, or modifies shared build tools.

Run from project root with existing dependencies:

```sh
rtk proxy env PYTHONDONTWRITEBYTECODE=1 .analysis-venv/bin/python tools/p9_pet_equip_five/build.py --base-apk build/hyjs-local.apk --base-proof analysis/p7/awake-gate-removed-20261017/apk-build-r1.json --output-directory analysis/growth-first-20261016/function-pet-equip-five-r1-20261017/ui-overlay-new
```

The output directory must be fresh. The tool verifies the APK/proof and all inherited qualified entries before using existing `.tools/native-batch/dotnet/dotnet` (.NET 8) and the existing dnlib DLL. Temporary compilation files are automatically removed. The output is a single replacement Assembly-CSharp.dll and a manifest; it requires no new runtime assembly.

The inserted method prefix computes `actualSteps=clamp(min(petLevel, rawMax, advanceCap)-current,0,5)` with the original first advance needStrengthLevel or current advance maxStrengthLevel. Missing advance configuration returns the maximum int quotation; ordinary validated rows use the original loop, fee lookup and coin parsing unchanged. Only the loop's fixed five constant becomes the computed local. `isCanStrength` still rejects current+5 beyond raw max; Num5, the success callback, the one-step cost label and every other method remain byte-identical at the method-body level. The client adaptation implements local rule SLR025, not a claim about unknown original server semantics.

Verification rejects any changed non-target raw method header/IL/EH or method ABI/flags/token, including all 547 Engine bodies. It also compares the complete original target loop suffix, allowing only its bound change. An interpreter executes the emitted target IL over all 76 original equipment rows × 13 advance states × 2 pet limits × 0–5 steps, plus normal, over-cap, missing-row and total-maximum quotations. This is offline emitted-IL correctness, not actual Unity/phone execution. The unchanged native consumer gate keeps maximum-crossing five-clicks rejected even where the quote function alone returns a smaller amount.

The first local attempt exposed a verifier object-equality bug for dnfile Flags; comparing the actual numeric metadata fields fixed it. The same produced DLL passed the guards. The small failure note is retained in ui-overlay-r1. Current delivery and exact hashes are in ui-overlay-r2/manifest.json. Original UI click and balance-display acceptance remains for the next coordinated client batch with the separate 501 fix.
