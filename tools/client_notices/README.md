# Original-frame announcement content

The original and GM announcement pages retain a Unity frame and put their content in the channel SDK WebView. Our earlier `ClientFeedback.prepareNotices` only made a matching WebView transparent; `/notices` still supplied a browser-font HTML layout. That did not replace the content typography with the game's font and could not style a white SDK parent container.

Read-only evidence is under `analysis/chat-hero-gacha-20261018/`:

- `announcement-original-frames.json`: `AnnouncePage` (`1fe6ee4addfdbad4ebda538e1bb94349`) and `SystemAnnouncePage` (`f3aa60fabc5d0a44a9f6a44b33d89df7`) are byte-identical in original/GM/Re/current. They contain the original frame, mask, title and close controls, with no complete announcement-text list to connect.
- `re-notice-rea.json` and `re-notice-render-rea.json`: Re's `OfflineNotices`/`OfflineCloud.NoticeTab` display a scrollable Unity view. `ChineseFont` selects a runtime Font named `Words` in Re. The current original APK font-object inventory only exposes built-in Arial and has no caller creating a Words Font. Our original game uses NGUI label/bitmap-font references, so Re's IMGUI Font lookup is not assumed available; its scroll/measurement approach is reference only. Re's embedded notice text and cloud/account system stay separate.

This bridge preserves both original Unity frames, original page opening/tweening, the original authenticated 20203 notice-URL request and original close/next-popup flow. It replaces only the two SDK `OpenWebview` consumers with a Unity view and adds cleanup to the two original close methods. The view creates NGUI UILabels using the original announcement title label's bitmapFont/trueTypeFont/fontStyle references, a left announcement-title list and right content flowing downward. It anchors inside the original background's local bounds, uses native NGUI clipped scroll views and only measures/rebuilds on content/selection/size changes. Long bodies retain all characters across bounded labels instead of one oversized text mesh.

Content still comes from the existing `/notices` backend endpoint. Its escaped `<article><h2>title</h2><p>content</p></article>` values are parsed as plain text, decoded once and drawn with `richText=false`. No HTML/JavaScript executes. Empty, loading and unavailable states remain inside the game frame. No backend, API, Java, wallet, login style, image or prefab edits are required.

## Build and combine

```sh
rtk proxy .analysis-venv/bin/python tools/client_notices/build.py \
  --input-managed <current composed Assembly-CSharp.dll> \
  --input-managed-sha256 <explicit SHA256> \
  --output-directory <fresh directory>
```

Four pinned target bodies must remain unchanged from the qualified input; other independent overlays may already be present. The verifier preserves every other original raw method, the 547 original battle-engine methods, ABI/tokens and assembly identity/references. The added assembly reference has an empty token with PublicKey flag cleared; an old-profile C# consumer actually recompiles against the output. The injector compares each helper method's full parameter names with the injected signature. The SDK's sixth argument is **Int32**, not bool.

Final packaging must include `Assembly-CSharp.dll` and `SoulFlowerNotices.dll`, retaining all other inherited bridges. No `classes2.dex` change is involved.

```sh
rtk proxy .analysis-venv/bin/python tools/client_notices/verify_content.py \
  --bridge <SoulFlowerNotices.dll> --output <content-representatives.json>
```

The content check executes the exact compiled bridge's parser under .NET with no Unity-native or rendering calls. It covers empty/multiple/ordered announcements, escaped and numeric entities, literal markup, malformed responses and a 100,000-character body. Actual Unity/NGUI JIT, frame bounds, font appearance, dragging, content selection and both close/reopen paths need one short combined device check. Missing title-font initialization waits for original NGUI labels; the bridge never creates or substitutes an Android/Arial font. A loaded original bitmap-font label is the bounded fallback if the page label has not initialized yet.

The earlier `notices-after-art-r1` output had an incomplete helper-signature check and must not be integrated. `r2` stopped at the added SDK-ABI guard. The final handoff names its exact qualified output and helper SHA; historical artifacts are diagnostic evidence only.

Prototype r3/r4 used Re's runtime Words Font lookup and are not deliverables: the original-font inventory exposed that assumption before packaging. r5 implemented NGUI rendering; r6 also qualifies the compiled NGUI font-copy call contract and corrects report metadata. The root handoff identifies only the final candidate.
