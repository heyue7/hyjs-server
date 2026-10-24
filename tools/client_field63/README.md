# Batch63 field visual adapter

Uses official62 APK SHA `fe427f13e8683b434bf7d2756b2dbd4a6f42746d0d6c66c86264182252704580`; supports `--input-managed` for the parent's disjoint Main overlay chain. Target CIL pins remain official62; every unrelated raw method body and all 547 battle engine methods must remain exact.

- Original `RCUIServerMonster.Tick` polls at one second. The replacement polls against `Time.unscaledTime` every 0.1 real second and calls the original `OnClick` gates. While waiting for state changes it retries at the original one-second cadence.
- `RoomNetField.SCSideMonster` applies only received objects. Same UUID/anchor updates keep existing patrol paths. Battle/death states still use original `SwitchState`; dead objects are removed using the original manager.
- The server still owns UUID, state, spawn anchor, qualification, frozen battle and rewards. Visual patrol uses original `TableFieldMonster.WanderR/WanderSpeed`; local visual policy approved for batch63 rests one real second and tries at most eight NavMesh samples. Full paths and corners must remain inside the server anchor radius. Pause, background, zero time scale, fights, death and leaving field halt movement.
- No network mask suppression was added: static original/62 socket push handlers contain no CreateNetMask call. HTTPManager.Send creates the network mask, so correlate `[PACKET] Tomsg :<opcode>` with the push on device before changing mask semantics.

Build: `rtk proxy .analysis-venv/bin/python tools/client_field63/build.py --base-apk /absolute/official62.apk --output-directory build/field63-overlay`.

Managed compilation/static ABI proofs do not establish on-device latency, NavMesh behavior or complete field UI acceptance. The parent owns the combined candidate and device checks.

Review correction: initial/reconnect AI ticks bind from the original UI monster UUID and freshest StaticData.fieldMon frame, so no respawn push is needed to initialize patrol. RCMoveNavigation lazily creates its agent; patrol samples all areas without requiring a pre-existing/enabled NavMeshAgent. Original StopTo queues the complete navigation path while running, and SamplePosition only applies while paused. A changed authoritative anchor keeps that original correction behavior. Use the latest r4 overlay; the earlier final helper was held for review and is superseded.
