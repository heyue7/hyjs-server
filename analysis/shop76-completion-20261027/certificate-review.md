# Certificate review — 2026-10-27

Reviewed candidate: `ceeb3de` in activity trial worktree. Scope: read-only audit; no certificate, hash, policy, source, mobile, production or DB changes.

## Finding

`TestP6Arena047CertificatePinsActualProofAndPolicy` correctly rejects stale runtime pins. Four runtime files differ, so the first failure at combatRoundService.go hides three additional differences. The same four mismatches occur in all four examined p6 certificates: arena047 v122, starter047 v118, linear-physical v117, owned-partner v126. The arena certificate originalNativeEvidence has 10 entries; all 10 files still exist in the original analysis source and match recorded SHA256. The five remaining runtime pins and supporting policy pin match. Historical original evidence remains intact; current implementation certification needs renewal.

| File | pinned SHA256 | candidate SHA256 |
|---|---|---|
| combatRoundService.go | 1314c87a6e866acc7ffcc5963458dad8141fb6948350963ef489765793adf4dd | 7462d10bab49e0830717602a6c82d6363662a1261225bf4a59812096b9fb4585 |
| combatRoundBuffService.go | e24c072222f8834238323ed691465e52ae6acdc788b2a1b2e8119994bb0559ad | 58d1508eea810e5375bdef687cf9a107405cd7d46cf8710acefdb0209eb1d136 |
| p6OriginalPercentService.go | 465901677e2ed9ff14934c69a37a054a686d3bbfa7380ca8e4fc04dbbdeddfb4 | b18296e92caa744bce4e20f9f0ed2aabfc480ca4d026646ebe9dbb979f87a1c2 |
| combatRoundSpecialService.go | ca2a3dceef099da5f510e045c933a2e1e9f84ad516c7d5b0ad9d825a8c572e50 | dca1dc174a20d1d68b5e4798fe6e44bed9ba9ab3f8af124ca19e1fbeb9acd885 |

## Behavior assessment

Commit b83199f adds lifestealHp to original fractional accumulator and applies damage*fraction directly, preserving native Effect/UnEffect residue and avoiding percent roundtrip. This is a real generic combat numeric behavior change, covered by combatLifestealPrecision_test.go and immutable p7 original314 fixture. It cannot substitute for arena evidence.

The arena047 admitted family is independently closed by p6ArenaSettlementService.go plus p6StarterSettlementService.go: one hero047 per camp, canonical three skills, no talent/awake/weapon, full HP/AP0, frozen original tables, and only buff380 damageAdd. This family has no lifesteal producer. The lifesteal-specific changed branches are unreachable for admitted arena inputs. Existing damageAdd/damageReduce handling remains unchanged.

Since initial Git source, combatRoundService also gained optional round live controls and live-recorded mode. Arena entry invokes ComputeOriginalP6ProtocolFinalHP with nil record; optional controls remain empty and liveRecorded false. applyRoundLiveAutoControl returns immediately. New recorded playback branching therefore retains the old arena behavior. However combatRoundService and combatRoundSpecialService already differ from pinned certificate bytes in the initial Git release 9d90bc2. Git history alone cannot reconstruct every historical pre-release delta; do not attribute all four hash mismatches solely to lifesteal.

## Current verification

Executed on candidate, using read-only historical analysis fixture root:

`rtk proxy env HYJS_TEST_FIXTURE_ROOT=/Users/lirui/LR/hyjs/analysis go test ./service -run ^TestP6NativeArenaV122\|^TestP6Arena047RequiresIndependentOriginalTypedProof$ -count=1`

PASS, service elapsed 7.135s. Includes all three arena native golden test groups: core/actor44/lifetimes/Buff metadata/manager; exact ordered HP/AP/combo/control and cast/Buff collections; final original protocol bytes with terminal HP. It also covers admitted structural inputs and rejection of certificate transfer to Friend11. These tests compare current Go behavior against pinned original typed mode2 captures; no original host or mobile recapture was performed. They support behavioral equivalence for the preserved three actual native source cases (timeout and both death directions), plus existing scope checks. They do not automatically certify every other family.

## Minimal safe renewal

1. Freeze final source candidate. Preserve old certificates, original golden captures and failed full-suite output.
2. Reuse this arena comparison only if its relevant runtime and policies still match this candidate. Run existing StarterFriendV118 and OwnedPartnerV124 original callback/export/final-byte groups, linear-physical original certificate source tests, their structural ownership/rejection tests, and lifesteal complete original End regressions. Use exact test names from source; no broad new phone journey or DB is necessary for this numeric closure step.
3. For each family, validate all original assets, scope policy, normalized constant pins and current runtime source closure. Record the unreachable/retained branch argument and fresh comparison command/results. If any native comparison fails, repair behavior or keep certification blocked; do not edit golden outputs or numeric tolerances.
4. Create explicit successor/revalidation certificates with references to immutable predecessor evidence, candidate SHA/source hashes, fresh strict validation results and unchanged scope. Update the corresponding four constUtil proof/file pointers only after review. Preserve old certificates. Re-run all four pin tests, strict native groups and full regression candidate gate. Updating hashes without these validation records would discard the evidence contract.
5. Actual changes outside the old scope, caller/assembly/table changes, or failed strict comparisons require new same-mode original-engine evidence and the applicable native anchor. Existing current tests give no reason to recapture arena047 on the phone solely for unreachable lifesteal arithmetic.

Proposed writable files for the owner after completing validation: four new analysis/p6 certificate/revalidation documents and four existing constUtil p6{ArenaSettlement,StarterSettlement,ScopedSettlement,OwnedPartnerSettlement}.go pointer files, plus batch validation record. Keep native golden PB/JSON, scope predicates, certificate tests and tolerances unchanged. Check certificate consumers and prior-proof compatibility before switching pointers; do not claim arena-only revalidation renews partner/linear/friend.

## Limits and resources

No DB, mobile, original host, deployment, heavy suite, credentials or business data touched. No temporary resources created. Only this report was written. Shared full-suite run remains owned by main agent.

## Historical receipt compatibility — rotation blocker

The four const pointers remain unchanged. p11FriendFightService.go:247 validates stored row.ScopeProof through p6Known* before checking unchanged terminal/record hashes; p11ArenaFightService.go:404 does the same and feeds verified historical rank/prize facts. Current starter, arena and partner helpers recognize only current proof. Linear PreviousProof applies only to the old v109 engine, so replacing it cannot retain the current v117 parent proof. Direct four-pointer rotation would invalidate previously accepted immutable settlements.

Safe minimal extension requires explicit parent-proof constants plus exact historical acceptance in p6ArenaSettlementService.go, p6StarterSettlementService.go, p6OwnedPartnerSettlementService.go and p6ScopedSettlementService.go, retaining every mode/client/config/engine restriction and original v109 branches. Persisted terminal/operation/response/record hash checks stay intact; do not recompute old receipts. Add targeted old-proof cold-receipt coverage. This exceeds the current approved file ownership. The generator now refuses --apply without an explicit history compatibility review acknowledgement, and still requires all 18 strict native/lifesteal passes. No production logic or test semantics changed.

## Approved renewal implementation

Main agent approved the minimal receipt compatibility extension. Four const files now retain precise ParentProof values and point to four `*-capability-certificate-revalidated-20261027.json` successors. The four p6Known* helpers accept current or exact parent proof within their existing identity boundary; linear v109 PreviousProof/OriginalProof branches remain unchanged. No calculation, terminal verifier, prize or ledger behavior changed. Successors preserve original scope/mechanism/authority/native evidence/historical validation and add an explicit revalidation block, parent path/digest, frozen strict native event evidence digest, current runtime/supporting/policy digests and normalized constants digest. Old certificates/native goldens remain untouched.

Frozen `certificate-revalidation-evidence.json` contains 18 actual PASS events: ArenaV122, StarterFriendV118, LinearPhysicalV111, LinearFriendV113, OwnedPartnerV124 each in three exact comparison groups, plus three lifesteal regressions. These events precede proof-helper rotation; arithmetic runtime sources are unchanged. New helper/receipt compatibility tests and successor pin tests remain pending resource release; no PASS is claimed for them yet.

New test `p6CertificateHistory20261027_test.go` covers each family current/parent acceptance, random proof/nil/wrong mode-engine-client-config rejection, linear Friend11 parent support, and an isolated real Friend receipt explicitly carrying parent identity. It reopens the database and retries twice with current policy closed, checking byte-identical response, unchanged profiles/assets and exactly one receipt/record. The fixture models accepted historical identity by rebuilding its own terminal/outcome integrity hashes; it never edits production data.

## Successor and historical compatibility verification — PASS

Authorized short SQLite run completed 2026-10-27 21:59 Asia/Shanghai: four exact certificate pin tests plus two new compatibility tests all PASS; package 6.206s, total wall 35.110s including compilation. `HYJS_P11_MYSQL_DATABASE` was absent; p11ArenaSetup used its own temporary SQLite fixture and built-in teardown. No MySQL/Redis/external service was started or accessed. Raw Go events: `certificate-short-tests.jsonl`; command/timing/exit status: `certificate-short-tests-result.json`. The historical Friend receipt successfully reopened and returned its original response twice with current policy closed, assets unchanged and no extra settlement/record. No failure repair was required.
