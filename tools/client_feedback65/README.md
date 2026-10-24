# Feedback 65 managed overlay

Only accepts formal64 APK SHA256 `f3ce73cc1f2e41bb45873c64f01e9a6ec81d84d1e41f18482c27d62c42ace3f9`.

Restores the original hidden `PlayerInfoPage/playback` debug form via `PlaybackKit.Start`; preserves its diagnostic click bridge. Resets registered reward clones, selection dictionary and selected ID before the original `UI.SelectReward.DoOpen`, preserving its callback. Removes the three obsolete floor6/elite02–34 guards while retaining each original method suffix and all other guards. Recompiles the existing bulk purchase helper from its separately reviewed source.

Run from the project root, in the reserved build slot:

```sh
rtk proxy .analysis-venv/bin/python tools/client_feedback65/build.py --base-apk build/hyjs-feedback64-formal-r2.apk --output-directory build/feedback65-overlay
```

The builder compiles two helpers and the injector, verifies every non-target raw body and all 547 original engine methods, then emits a package_native-compatible manifest. It performs no packaging, installation, device, database or network test. Its temporary compiler workspace is removed automatically. Failed output is retained for diagnosis; use a fresh output directory for retries.

Required device verification: character information opens normally with the complete ServerId/FightId/OK block hidden; reopening a reward chooser requires a new choice and invokes the current callback once; original visual assets and quantity controls remain coherent; trial and elite entry now reaches the server's existing qualification checks. Unchanged map/NPC unavailable guards remain active.

## Wanted settlement error recovery

Six additional prefixes capture the original start request identity and a protobuf-serialized copy of its real reply before the battle engine can mutate monster/player references. Each settlement callback is bound to its owner, fight ID, player reference and session. Stale callbacks return without touching result globals. A failed settlement exits directly to the original Wanted tab, with the original message-box style. It never calls `End(false)`, fabricates a result, edits attack counts or issues another Start request. The matching original normal/deadly attack button resumes this snapshot before the local cost gate; a different task or attack type stays blocked while recovery is pending. A second click during transition is suppressed. Each replay deserializes a fresh graph. Only explicit user clicks resume; there is no automatic retry loop. A genuine successful result clears the cache and runs the unchanged original success callback.

Verify failed settlement returns to Wanted without a defeat/reward screen; the matching attack resumes the same fight with original HP/seed and unchanged attack count even at zero remaining; wrong task/type, duplicate click, account/session switch and late replies cannot start or settle the wrong fight. The cache is session-local and does not survive process termination. Recovery after restart remains governed by the server's existing pending-start recovery. The six original method bodies, local signatures and branch graphs remain intact after their prefixes; all 11 target inputs are pinned.

### Repeated failure return correction

The original `MercenaryTaskPage.DoOpen` (`0x06004fd6`) parses `options["tab"]` as an integer; Wanted is tab `1`. The old helper's extra `OpenPage(..., "tab=WantedMission")` poisoned the existing page options and threw after the first visible return. On the next failure, `BattleToRoom` reached synchronous `ChangeUIBack` → `DoReopen` → `DoOpen`, then threw before its subsequent `HideLoading`. The correction uses the original public `PageManager.currentPage` (`0x040050a8`) and inherited public `Page.options` (`0x040041df`) to set `tab=1` before `BattleToRoom`, and removes the extra page open. The original return/reopen and original message box remain. No wait, coroutine, room switch, result, cost or recovery-cache behavior is added.

The rejected-settlement exit also clears the original public `BattleManager.isBattleUI` presentation flag, matching the original result-panel click before `BattleToRoom`; it does not mark any server battle as settled.
