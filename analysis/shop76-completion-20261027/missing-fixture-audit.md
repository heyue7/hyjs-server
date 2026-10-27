# Missing fixture audit 2026-10-27

Read-only inspection only. No tests, fixture copies, goldens, DB, phone or production mutations.

## Reuse

Set `HYJS_TEST_FIXTURE_ROOT=/Users/lirui/.codex/worktrees/guild-full-qa/hyjs/analysis` for the existing test process. `readTestAnalysis75` first reads the current worktree, then uses this absolute analysis root only for missing paths under `../../analysis/`. No helper change or fixture copy is needed.

Each test directly requires seven files: bundle + core/hits/manager/records + source and capture manifests. Hashes below were checked against fixed test-source constants and bundle bindings. Referenced artifacts inside source/capture manifests are outside these four tests' direct file-read boundary.

## guild-guard14-phone-golden-20261026-r1

| Absolute path | Bytes | SHA256 | Match |
| --- | ---: | --- | --- |
| `/Users/lirui/.codex/worktrees/guild-full-qa/hyjs/analysis/p6/guild-guard14-phone-golden-20261026-r1/bundle.json` | 2328 | `f28f6896c44c9693273160f7d18090883ac5781c1d601e13d0e73d031bcdd9fe` | yes |
| `/Users/lirui/.codex/worktrees/guild-full-qa/hyjs/analysis/p6/guild-guard14-phone-golden-20261026-r1/core.json` | 5182391 | `062bdc3dfe35eee4fc32e0d1e6e84164bb58193b94f499131b754efe20c42798` | yes |
| `/Users/lirui/.codex/worktrees/guild-full-qa/hyjs/analysis/p6/guild-guard14-phone-golden-20261026-r1/hits.json` | 950570 | `3825b609189ce715742b66027d82bd49cbf6668d24ce6ca10fd4f29c6556ff21` | yes |
| `/Users/lirui/.codex/worktrees/guild-full-qa/hyjs/analysis/p6/guild-guard14-phone-golden-20261026-r1/manager.json` | 209470 | `0ed278290278509ee528308afebc46df2d978fd4abcb5fd9ba77da2a14d2b73e` | yes |
| `/Users/lirui/.codex/worktrees/guild-full-qa/hyjs/analysis/p6/guild-guard14-phone-golden-20261026-r1/records.json` | 11900 | `ace1e4d2e34845d045b18a09a62d385f1b49eccee513c70a75717c6ad07c7171` | yes |
| `/Users/lirui/.codex/worktrees/guild-full-qa/hyjs/analysis/p6/guild-guard-prospective-inputs-20261026-r1/manifest.json` | 46968 | `a4fd9c297a43aca5793c4be171b45abfa23dd3e0252d6830957f5935d7f26644` | yes |
| `/Users/lirui/.codex/worktrees/guild-full-qa/hyjs/analysis/p6/guild-guard-prospective-phone-20261026-r1/manifest.json` | 65761 | `324fb1bb0c36c4b1ed6d1ba19a28e45b8deec687130c77e2f321bb5a8cace14f` | yes |

## guild-guard-party-new2-phone-golden-20261026-r2

| Absolute path | Bytes | SHA256 | Match |
| --- | ---: | --- | --- |
| `/Users/lirui/.codex/worktrees/guild-full-qa/hyjs/analysis/p6/guild-guard-party-new2-phone-golden-20261026-r2/bundle.json` | 6372 | `e38e0fd103304ded6dfb472b2bc04b51340dbd5e5d16485445e83c9c324ea14b` | yes |
| `/Users/lirui/.codex/worktrees/guild-full-qa/hyjs/analysis/p6/guild-guard-party-new2-phone-golden-20261026-r2/core.json` | 4502853 | `1b6002fb76102e8ef9dd8c51206ea9249035ca1e9d6ad891ee473bb1648e4be3` | yes |
| `/Users/lirui/.codex/worktrees/guild-full-qa/hyjs/analysis/p6/guild-guard-party-new2-phone-golden-20261026-r2/hits.json` | 355173 | `c84625ca563e91fc72c085abf817b9e170fd1177a4de65b6bffa718b0091e13b` | yes |
| `/Users/lirui/.codex/worktrees/guild-full-qa/hyjs/analysis/p6/guild-guard-party-new2-phone-golden-20261026-r2/manager.json` | 117155 | `11b941ba845168bd0dc518d6c9ff652bee655de3a55fbbde0af5730da3f1619c` | yes |
| `/Users/lirui/.codex/worktrees/guild-full-qa/hyjs/analysis/p6/guild-guard-party-new2-phone-golden-20261026-r2/records.json` | 2849 | `87a85b91ae05a81ca21250c4f9c86db0310d06d3913d7c2df4d97d309daafce7` | yes |
| `/Users/lirui/.codex/worktrees/guild-full-qa/hyjs/analysis/p6/guild-guard-party16-new2-inputs-20261026-r1/manifest.json` | 31282 | `9c0f4ad70100c01db04e54ace672eaf1ce0f64dd6ee9fde3a9e50cc2a3e0925e` | yes |
| `/Users/lirui/.codex/worktrees/guild-full-qa/hyjs/analysis/p6/guild-guard-party16-new2-phone-20261026-r1/manifest.json` | 31023 | `fa71f55f8872feb5364f36b6bf85bff604ffb73404d98cefc91c0315781d9a79` | yes |

## guild-guard-phone-golden-20261026-r2

| Absolute path | Bytes | SHA256 | Match |
| --- | ---: | --- | --- |
| `/Users/lirui/.codex/worktrees/guild-full-qa/hyjs/analysis/p6/guild-guard-phone-golden-20261026-r2/bundle.json` | 2762 | `46e642359287d6d11aaf8071d9017a0c00e4eec874e9511320a04bb5c7dd1783` | yes |
| `/Users/lirui/.codex/worktrees/guild-full-qa/hyjs/analysis/p6/guild-guard-phone-golden-20261026-r2/core.json` | 3477804 | `2b709e1805773a70d6e9ed67a1b9b3ea25c18c7037c3c9daf254f5d2e274b689` | yes |
| `/Users/lirui/.codex/worktrees/guild-full-qa/hyjs/analysis/p6/guild-guard-phone-golden-20261026-r2/hits.json` | 636549 | `ccc4f8ebab9ecf89d8f5bf6e88cd52e7315581e4430ed8790005a7874b148488` | yes |
| `/Users/lirui/.codex/worktrees/guild-full-qa/hyjs/analysis/p6/guild-guard-phone-golden-20261026-r2/manager.json` | 141014 | `8165a91ac4ea05d02cfca36ef4b7b7eaa92eeea6680958dfba4cc3807274aa11` | yes |
| `/Users/lirui/.codex/worktrees/guild-full-qa/hyjs/analysis/p6/guild-guard-phone-golden-20261026-r2/records.json` | 8268 | `0389d74c58c3500148eabcae5f4dace9c2a0d5efc8efe4b319158d7294d5efb3` | yes |
| `/Users/lirui/.codex/worktrees/guild-full-qa/hyjs/analysis/p6/guild-guard-inputs-20261025-native/manifest.json` | 24004 | `462e6144ab013f7cc228cc3ebcdb01bd1b76ecbc6b2bfadc8c0640579fe73bb0` | yes |
| `/Users/lirui/.codex/worktrees/guild-full-qa/hyjs/analysis/p6/guild-guard-inputs-20261026-phone/manifest.json` | 23799 | `fcef9fc77ee651cffd22ed7bdc1821473575ebf90eec5934450126c6f1df83e5` | yes |

## guild-party-pvp-phone-golden-20261026-r1

| Absolute path | Bytes | SHA256 | Match |
| --- | ---: | --- | --- |
| `/Users/lirui/.codex/worktrees/guild-full-qa/hyjs/analysis/p6/guild-party-pvp-phone-golden-20261026-r1/bundle.json` | 5660 | `8e88c32f1eda4014cd066f498a2db488cb815dbf9ef8cc0875484011b5380d87` | yes |
| `/Users/lirui/.codex/worktrees/guild-full-qa/hyjs/analysis/p6/guild-party-pvp-phone-golden-20261026-r1/core.json` | 6931271 | `1585ee3c7dac650a7438f62e1c44964011c288beee73189244b0ab25b70d2847` | yes |
| `/Users/lirui/.codex/worktrees/guild-full-qa/hyjs/analysis/p6/guild-party-pvp-phone-golden-20261026-r1/hits.json` | 399926 | `df4f82ec860016329809958f58e4c5dbb020c9f2d1927f478c1f44e6438ce094` | yes |
| `/Users/lirui/.codex/worktrees/guild-full-qa/hyjs/analysis/p6/guild-party-pvp-phone-golden-20261026-r1/manager.json` | 195618 | `ccee232ac7dd584a6f6aec594aff049209d50f54e5d6bebfe07a925fcbcaedb9` | yes |
| `/Users/lirui/.codex/worktrees/guild-full-qa/hyjs/analysis/p6/guild-party-pvp-phone-golden-20261026-r1/records.json` | 2074 | `6d2b9a97f1acfff3e3f4601fc81a18c629ac10665cf3fd642eaec62b008018ea` | yes |
| `/Users/lirui/.codex/worktrees/guild-full-qa/hyjs/analysis/p6/guild-party-pvp-input-20261026-r1/manifest.json` | 15306 | `5f973f3880d05a4b6e732c80b3e66d334f793fba1a09639f7e4c947525bd0eb6` | yes |
| `/Users/lirui/.codex/worktrees/guild-full-qa/hyjs/analysis/p6/guild-party-pvp-phone-20261026-r1/manifest.json` | 14462 | `0cef1440f9afefd50a96bffac7f30ae28389bc84671cdfda016a09746450f103` | yes |

Total direct fixture footprint: 23394612 bytes across 28 files. All 28 SHA256 values match.

Root and relevant worktree directories were inspected with bounded directory enumeration; all four exact fixture directories were located together in `guild-full-qa`. Root analysis contains no matching target fixture directories.
