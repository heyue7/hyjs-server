# Fixed legacy manifest cache compatibility

Scope: only `AssetLoad.AssetBundleManager.GetFullPath(string,out bool,bool)` return values. Original body and branches remain. All three original returns pass their selected path and original normalized name through `ManifestPath.Resolve`.

Original current55 `GetFullPath` selects persistentDataPath/name first. APK path expression is exactly `Application.dataPath + "!assets/android/" + name` (original IL0043–0053). `LoadAndroidManifest.MoveNext` selects android through GetBundleSimple/GetFullPath and independently prefers cached smallassetmanifest/assetmanifest. Evidence: `analysis/dungeon56/nightforest-manifest-entry.il`, `nightforest-loader.il`.

Only name `android`, selected path exactly persistentDataPath + `/android`, file SHA256 `28e264a89fb6b1a5fc27fa5a6fbdfb55cf510e4ba6e6431d1a023631d620f28d` (base55 APK android manifest,321821bytes) is redirected to original APK expression. Unknown cache content, missing/oversize/inaccessible/changing files and every other resource preserve original selected path. No cache deletion/write, download status/full flags or asset tables change. Reads bounded to 2MiB; SHA result cached by path/length/UTC write ticks, checked only for the android manifest name.

Parent supplies patched APK android manifest and fixed nightforest root. This hook alone does not provide those resources. Cache root or asset-table compatibility outside this precise manifest source pin remains unchanged.

Build with repository Python dependencies, fixed base55, and optional final upstream Main:

```sh
rtk proxy /Users/lirui/LR/hyjs/.analysis-venv/bin/python tools/client_dungeon_manifest/build.py --base-apk build/hyjs-feedback55.apk --tool-root /Users/lirui/LR/hyjs --input-managed <upstream-final-main.dll> --output-directory <fresh-project-output>
```

Output contains Main, HYJS.DungeonManifest.dll, manifest.json and bounded build logs; no APK package/install. C#3 helper references original base55 mscorlib/UnityEngine. Static verification compares against supplied upstream Main: 547 engine methods and every non-target raw body unchanged, original metadata token/ABI retained, each original return transformed into ldarg1/call/ret, branch targets follow original return, final serialized TypeRefs have no helper ModuleRef scopes. Duplicate injection is rejected. Parent owns final serial integration and device checks with old cache and absent cache.

## out bool review

The original flag is not a persistent-only indication. GetFullPath IL0036–003b assigns the out flag `(arg3 == false)` after the persistent miss, then IL003c–003e enters the APK `!assets/android/` branch only when that flag is true. Thus both persistent and APK paths have out=true. The streaming branch has out=false. The helper's selected persistent→APK redirect keeps the original out=true, exactly matching original APK semantics; no ref-bool change is needed.

GetBundleSimple passes arg3=false at IL0010 and directly calls AssetBundle.LoadFromFile at IL0017 without consuming the returned out local. LoadAssetBundleSingle0.MoveNext passes false at IL0099, writes its isFile field via GetFullPath at IL009a, and never reads that field again; it chooses sync/async by scene-loading and size, not this flag. smallassetmanifest/assetmanifest calls use arg3=true and branch on the flag for File versus WWW, but this helper does not redirect those names. Evidence is pinned in analysis/dungeon56/nightforest-loader.il and nightforest-single-loader.il.
