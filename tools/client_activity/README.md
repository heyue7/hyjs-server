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
