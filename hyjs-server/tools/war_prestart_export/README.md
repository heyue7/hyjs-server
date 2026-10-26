# Isolated war prestart source export and publication

This tool only loads original tables and calls the existing `P13UnionFightMirrorGuardTemplate` for all ten IDs. It does not initialize a database, create a match, freeze an offer, or grant authority. Its output contains deterministic complete protobuf bytes, SHA256, readable complete monsters, table pins and the current separately reviewed proof strings. Existing outputs are rejected.

Run from `/Users/lirui/.codex/worktrees/guild-full-qa/hyjs` after the parent reserves its Go window:

```sh
rtk proxy go -C hyjs-server run ./tools/war_prestart_export --tables /Users/lirui/LR/hyjs/analysis/tables --out /Users/lirui/LR/hyjs/analysis/guild-full-qa-20261025/war-prestart-source-r1.json
```

Publisher: `analysis/guild-full-qa-20261025/war_prestart_publish.py`. It imports the existing phase2 helper only for online dryrun/apply. `--check-only` performs no runtime import or DB operation. The complete source export is required, never replaced with a hand-built four-stat monster. Model columns are read from current source; missing columns/rules are explicit blockers.

```sh
rtk proxy env PYTHONDONTWRITEBYTECODE=1 python3 analysis/guild-full-qa-20261025/war_prestart_publish.py --source-export analysis/guild-full-qa-20261025/war-prestart-source-r1.json --check-only --plan analysis/guild-full-qa-20261025/war-prestart-offline-r1.json
rtk proxy env PYTHONDONTWRITEBYTECODE=1 python3 analysis/guild-full-qa-20261025/war_prestart_publish.py --source-export analysis/guild-full-qa-20261025/war-prestart-source-r1.json --expected-binary-sha EXACT_RUNNING_SHA --plan analysis/guild-full-qa-20261025/war-prestart-dryrun-r1.json
```

The parent inspects every target column, current row/hash, schema prerequisite, resource bounds and conflict. Copy the embedded `reviewTemplate` to a **new** review file, approve only after inspection (`approved:true`), hash the exact review bytes, then create a fresh reviewed dryrun using `--review FILE --review-sha SHA`. Do not edit a saved plan. The plan material binds sources and export; `--plan-sha` additionally binds the complete observed DB snapshot. Existing primary native certificate review files and their evidence hashes must match nonempty Mirror/PVP proof constants. Publication review grants no new battle authority.

```sh
rtk proxy env PYTHONDONTWRITEBYTECODE=1 python3 analysis/guild-full-qa-20261025/war_prestart_publish.py --source-export SOURCE_JSON --expected-binary-sha EXACT_RUNNING_SHA --review REVIEW_JSON --review-sha REVIEW_SHA --plan NEW_REVIEWED_PLAN_JSON
rtk proxy env PYTHONDONTWRITEBYTECODE=1 python3 analysis/guild-full-qa-20261025/war_prestart_publish.py --source-export SOURCE_JSON --expected-binary-sha EXACT_RUNNING_SHA --review REVIEW_JSON --review-sha REVIEW_SHA --plan REVIEWED_PLAN_JSON --plan-sha EXACT_PLAN_SHA --apply --record NEW_APPLY_RECORD_JSON
```

Only the parent runs online dryrun and apply. The publisher checks allocated private phase2 ownership, live wrapper/app PID and executable mapped inode+SHA, DB user/UUID, source pins, native reviews, all current target hashes, absence of target-day Match and exact existing signup deadlines. All mutations share one guarded transaction and one real DB `NOW(3)` timestamp. Missing/changed prerequisites cause zero target writes; database errors disconnect/rollback. Publication updates only named Source/Policy and the authorized pre-matcher legacy Schedule, never Entry, Match, Offer, actor, inventory, battle result, qualification, or reward delivery. Existing custom/nondefault sources are refused. Identical rows retain their previous real timestamp. A failed post-commit verification is recorded and must be inspected before retry.

Fixed contract: day27, legacy custom=false, registration26 00:00 through27 00:00, starts19:00/20:00, duration30min/preparation5min. A pristine legacy Schedule (empty registration/version/enrollment/publication metadata) may be upgraded under exact deadline/no-Match locks; an existing custom schedule is refused. SeedSalt is explicitly0, while actual per-battle seed remains the existing frozen hash derivation. Resource positions are named local mirror coordinates inside original XZ bounds `[-20,93]×[-33,32]`, tolerance3. Weekly publishes current-week original80 rows; monthly only future2026-11 original20 rows, no historical October freeze. Full reward strings are new local historical compatibility; tasks are explicitly approved local50/50 score5/5 and factory prestige5. Shop only016..020 OwnToken, no diamond SKU or inventory injection.013 requires the new pending EffectRule; until its source/model/schema is integrated the dryrun blocks.

Offline checks (already run; no Go/DB):

```sh
rtk proxy env PYTHONDONTWRITEBYTECODE=1 python3 analysis/guild-full-qa-20261025/war_prestart_publish_test.py
```

The source-only Go exporter was formatted but not compiled or run by its implementing agent. Parent Go compile/export, online SQL syntax/lock behavior, actual publication, complete Build/Freeze and real protocol field flow remain unverified.

Parent integration: publisher now includes73 targets, enabling existing same-camp war text and18-command channels with original named20s cooldown. The sole nonzero disabled Field default accepted is the explicitly prepared300000ms admin-tested preparation. No arbitrary preexisting custom values are accepted. Earlier72-row offline output is preserved; create a new plan/review for the integrated73-row target set.
