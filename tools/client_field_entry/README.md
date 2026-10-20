# Packaged-field world map lock

Run with `.analysis-venv/bin/python tools/client_field_entry/build.py --output-directory <fresh analysis directory>` through `rtk proxy`.

Pinned input: stamina/sweep r4 request-token main DLL `301dc5eb`. Only six `UI.WorldMapPage` bodies change: four eligibility render methods and two transfer guards. Field001/002 original eligibility methods, map tiles, NGUI fonts, lock objects, type icons and popup remain. Field003–006 show the original lock layer and `暂未开放`; their clicks use original `mapCloseTip`. No helper, resource, combat, reward or endpoint changes are added.

The output includes the inherited `SoulFlowerStaminaSweep.dll`; packaging must replace both manifest entries. All other APK entries must inherit the base APK. Build verifies original method/field/type tokens and ABI, all 26,940 non-target raw bodies, 547 Engine methods, exact inserted guards and original branch destinations. Static verification does not establish device visuals or interaction; Root owns packaging and device acceptance.
