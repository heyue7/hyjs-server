# Activity rendering boundaries

This small overlay preserves the original activity ID decoder, 34 category
prefix mappings, prefab paths, server requests and reward logic. It borrows the
useful boundary-handling approach identified in Re without importing that
package's offline directory, operator settings, page-ID remapping or services.

Seven original methods change:

- `UI.UIActivity.MsgCallback`: validate successful packet/body/list, remove null
  or empty IDs and duplicate prefix categories in server order, then execute the
  complete original decoder. Unknown nonempty prefixes remain subject to the
  original mapping; valid IDs and the first category occurrence are unchanged.
- `UI.UIActivity.Init`: remove undefined/duplicate tab enums and show an original
  font empty state for no renderable tabs instead of dereferencing tabs[0].
- `UI.UIActivityPanel.Show`: retain the original InitActivity invocation, template
  Init/Hide/Show, activity ID dictionary and tween path, with explicit unknown or
  unpublished-type guards and an initialization exception boundary. The cached
  template is rebuilt when the same category receives a different published ID.
- The four original panel refresh/close/cover lifecycle methods return when no
  template exists, protecting the empty state.

The built-in sign-in and kitchen tabs retain their original behavior. The new
bridge does not grant items, construct activities, change completion or make
additional direct requests. It displays “暂无开放活动” using the original NGUI
font and logs the failing original template/exception for subsequent diagnosis.
A failed initialization clears only its owned template/new module children.

Use an explicit input SHA when composing:

```sh
rtk proxy .analysis-venv/bin/python tools/client_activity/build.py \
  --input-managed analysis/playability-fixes-20261018/display/overlay-after-playability-r3-release/Assembly-CSharp.dll \
  --input-managed-sha256 8daa5e1d20b7091ec586bdb81ee9b5337d53337d9e815588568b07ae01543cbc \
  --output-directory analysis/activity/combined-r1
```

Output `Assembly-CSharp.dll` and `SoulFlowerActivity.dll` must be packaged
together. Existing display/playability/formation bridges remain required. All
non-activity raw methods remain exact relative to the explicitly bound input,
including 547 original engine methods. Original directory method suffixes,
branches, exception regions and metadata identities are verified. The bridge
uses only the original mscorlib/UnityEngine/Assembly-CSharp runtime.

The user-reported crash lacks a matching native/client stack. These guards cover
specific confirmed null/index/duplicate/initialization risks, not every possible
native crash or later template Update/Awake callback. Backend publication must
still exclude unfinished/broken operational configurations. Static checks and
compilation are not a phone pass; packaging and device checks belong to root.

## Guarded empty-state rendering and phone diagnosis

The first phone batch showed the original activity background with no visible
tabs. The server returned five valid published entries for the same phone
session; this observation does not establish an empty activity directory.

The guard's failure path had a confirmed rendering omission: `InitTabs` creates
navigation alpha tweeners, while only the successful template path invoked the
original `PlayTweener`. The bridge now invokes that same navigation animation
on guarded failure and forces the new empty label's alpha to 1. These changes
fix the fallback visibility boundary; the reason a healthy template might have
failed still requires the actual phone callback or exception.

The final bridge removes temporary direct JNI logging. The original Unity
warning path remains available for template failures.

The decisive original-device exception was `TypeLoadException: Could not load
type 'UI.UIActivity'` while invoking the patched `UIActivity.MsgCallback`.
Direct helper calls succeeded. The caller's imported parameter TypeRefs had
incorrect ModuleRef scopes pointing at `SoulFlowerActivity.dll`, although the
page, packet, enum and panel types belong to the main module.

`Injector.cs` now constructs explicit MemberRef signatures using the existing
main TypeDefs and original parameter signatures. Only the helper declaring type
uses external AssemblyRef `SoulFlowerActivity`. Existing composed output is
repaired by `tools/client_release/repair_bindings.py`, without changing method
bodies; fresh builds use the corrected injector directly. Source qualification
is recorded in `activity/overlay-import-scope-r3/`. Device validation must match
the final combined APK.

Exact prefab evidence in `original-owner-prefab.json` confirms that
`UIActivityPanel` and `UIActivity` are enabled components of the same original
GameObject. The original Show method uses the same `GetComponent<UIActivity>`
owner lookup. `original-prefix-enum-map.json` confirms that the 34 original
category prefixes map to 34 distinct enums; the guard does not introduce a
second category mapping.
