# Feedback66 per-round LiveAuto handoff

Production Go source frozen. Driver `original-pve-per-round-live-auto-v1`, independent proof `3180805adfd09473d3e151dd5dabbd90d79037f480deb0bae9ac11e104e9a218`.

## Cause and change
The original client may wait for manual timeout in later player rounds. Previous live-auto fallback only handled first-round delay; original playback also schedules one command per update and cannot reconstruct simultaneous live-AI commands. The new independent fallback derives exactly one finite first-success clock per consecutive player round and runs original AI queue construction. It receives no supplied skills. Full generated BattleOperate must be proto.Equal to the entire submitted record; terminal outcome/HP also remain strict.

No public FrozenBattleInput/model/hash/proof changed. Old wrappers and fallback priority remain. Private controls, driver, controls hash, frozen-input hash and full-record hash bind a separate receipt envelope. New receipt validation is separate; old accepted receipt paths remain.

## Owned sources
- hyjs-server/constUtil/p7RoundLiveAuto.go
- hyjs-server/service/p6RoundLiveAutoService.go
- hyjs-server/service/p7RoundLiveAutoRushService.go
- hyjs-server/service/p7RoundLiveAutoRush_test.go
- hyjs-server/service/testdata/p7_round_live_auto_phone66.json
- hyjs-server/service/testdata/p7_round_live_auto_phone66_floor8.json
- tools/local_original_engine_host/round_live_auto66.py
- tools/local_original_engine_host/RoundLiveAuto66Host.cs
- tools/local_original_engine_host/compare_round_live_auto66.py

Four shared service files have only 39 insertions/1 deletion: combatRoundService, p6AIDriverService, p6FinalHPService, p7GenericMechanismRushService.

## Evidence
Original547 source, DLL and tables unchanged. Native outer host uses original SetAuto / EnginePlayerAI.Run at independently observed per-round clocks; no record passed to AI. Two floor7 original-phone cases exactly match native and Go full protocol bytes; 29 press events, 28 skill_begin events including targets, all four actors state/currentHP/maxHP match exactly. Native targets were singleton and fully checked. Report: round-live-auto-native-r2/go-native-strict-comparison.json. R1 retains Go final outputs; R2 retains event clock/round native outputs. No third host case needed.

Passed representative selector: `^(TestP7GenericMechanismRushFullGrowthAndStrictEnd|TestP7GenericMechanismRushMultipleCommandsAndFormation|TestP7RoundLiveAuto66PhoneAndColdReceipt)$` (15.218s). New tests cover exact phone records, cold receipts, source/seed/owner separation, driver/proof downgrade and actor/NaN/clock/skill/HP/winner tampering. Typed10 is an explicit diagnostic projection, not an Elite phone anchor.

Root owns API/MySQL integration, idempotency/double-spend checks and original-phone same-End retry against upgraded QA. Floor8 transport-drop has not yet demonstrated a committed response loss. No DB, phone or production operations performed here.

Floor8 selector `^TestP7RoundLiveAuto66Floor8RetrySource$` PASS (1.35s case / 3.077s package): 45 skills, round17, HP14547.97000426885/0, complete protocol equality and cold receipt. Production and test files are now frozen; Root may take ownership.
