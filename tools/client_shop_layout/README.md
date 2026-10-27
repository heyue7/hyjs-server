# Native shop quota position overlay

Scope: only `HYJS.Feedback66.CommerceRuntime.Quota`, used by original reputation/point card hooks. No UI skill, new artwork, purchase protocol or purchase-handler changes.

## Evidence

`build/shop-review-20261027/reputation.png` shows the first right card quota overlapping Buy; other visible cards remain left aligned. The original APK has one shared item prototype per shop, with direct sibling Name/IconRoot/Condition/BtnBuy under the card; both Condition labels have row-local x=-146, Left pivot, and BtnBuy x=100,y=-44. See `prefab-evidence.json` (APK SHA included). The exact dynamic cause of the lone misalignment has not been observed; static prefab evidence does not prove which NGUI lifecycle update caused it.

The inherited patch uses `iconRoot` world position to calculate the new label position and writes it before widget geometry properties. This overlay uses original Condition x=-146 and applies the final localPosition after widget geometry properties. It retains original name-label font/style, daily quota text, font size/width rules, rank exclusion, visibility and Buy row y. It permits only ShopPopularityItem and ShopPointItem and skips unexpected widget parent structures. All geometry uses the same row coordinate system; arbitrary row ancestor scale/rotation is handled by Unity local transforms.

## Rebuild and stack

Use the project analysis Python (dnfile) and existing native-batch runtime:

```sh
rtk proxy /Users/lirui/LR/hyjs/.analysis-venv/bin/python tools/client_shop_layout/build.py --base-apk build/vip7-preview-device-test-20261027.apk --runtime-root /Users/lirui/LR/hyjs/.tools/native-batch --output-directory tools/client_shop_layout/new-overlay
```

Replace ONLY `assets/bin/Data/Managed/HYJS.Commerce66.dll` with `final-overlay/HYJS.Commerce66.dll`, stacking after current trial/VIP/item overlays. Keep the current Assembly-CSharp and all other entries unchanged. Input must already contain the Feedback66 hook/helper; manifest pins exact tested APK, Main and inherited helper. Review inherited helper SHA before stacking onto another candidate.

Validation: actual Unity/Main-reference C# compilation passed; helper assembly name and existing two method signatures/tokens preserved; Get body byte exact; only Quota body changes. Main remains untouched. No APK packaging, mobile operation, remote/DB access or commit. Runtime visual/interaction acceptance remains for the parent agent: same reputation first right card, point shop, scrolling/reopen, rank cards and Buy response.
