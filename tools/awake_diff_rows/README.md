# Awake difficulty rows

This opt-in overlay inherits the installed P9 APK `a137ae5b…` and managed DLL `f4eb8b19…`. It emits one replacement `Assembly-CSharp.dll`, with no APK build, installation, new runtime module, resource check, scene-table change, game state or battle change.

```sh
rtk proxy env PYTHONDONTWRITEBYTECODE=1 .analysis-venv/bin/python tools/awake_diff_rows/build.py --base-apk build/hyjs-local.apk --base-proof analysis/growth-first-20261016/function-pet-equip-five-r1-20261017/apk-candidate-build-r1.json --output-directory analysis/p7/awake-diff-rows-r1-20261017/overlay-new
```

Use a fresh output directory. The tool pins the APK, proof, inherited qualified entries and original two raw method bodies. It uses existing .NET 8/dnlib, removes its temporary compiler directory and writes with PreserveAll. All method tokens, signatures and flags remain; every other raw method header/IL/EH must match, including 547 original Engine methods and inherited P9 five-step cost behavior.

Only two prefixes are added:

- `AwakeDiffItem.SetDiffItem`: null task clears its own private `taskInfo` and `page`, hides its root and returns. A nonnull task activates the root before the original binding code.
- `ChallengeActivityCopyPage.SetSecondPage`: calls the null-reset path for every existing difficulty slot before the unchanged original chapter/task loop runs. Only actual current tasks are then bound and shown. Empty chapters expose no stale exploration action.

The verifier retains both original method instruction suffixes, including branch targets and unchanged original locals. An interpreter executes the emitted prefixes and original loops, modeling only original collection/label/Unity calls. It checks transitions 3→0→1→2 closed→3→0 tasks, plus an empty chapter dictionary. It checks current sorted IDs, private-reference reset, root visibility, original open/closed controls, fight labels and background/open-time callbacks. These vectors are offline control-flow evidence; Unity/phone behavior awaits the parent-coordinated APK batch. No P9 matrix is repeated.

Existing pending resume (`BoxCallback` / page `ChallengeCallback`) and the async new-entry callback remain byte-identical. The fix prevents the proven old-row interaction; it adds no speculative async fields or ABI.

An explicitly authorized candidate-only package can be generated with:

```sh
rtk proxy env PYTHONDONTWRITEBYTECODE=1 .analysis-venv/bin/python tools/awake_diff_rows/package.py
```

This uses the project client-build lock and the existing signing key/SDK. Its fixed output is `build/hyjs-awake-diff-rows-candidate-20261017.apk`; current/rollback aliases remain unchanged. The base is checked against both the real P9 candidate qualification and its separate promotion record. It preserves all original entry compression types, including the 2680 stored resources, and checks every non-signature non-target payload SHA after signing. No original proof status is rewritten. Build attempt records remain in the delivery directory, and owned unsigned/aligned/temporary artifacts are removed.
