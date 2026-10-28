# Green and blue formation candidates

Official79 APK SHA256: `60075046b9dfde0a00083b07a3cca878d071b604fcb31bad63d8c21be821eff8`.

The original `UI.UIInstPlayerData.GetAvailableHerosForFormation` method filters quality<=12. This hides the six owned green combat heroes from `UI.HeroReplacePage.<UpdatePanel>c__IteratorB4.MoveNext`; blue13 was already admitted. The patch changes one `ldc.i4.s` operand from12 to11, retaining every method header, branch, exception region, token and all547 Engine method bodies. Experience materials are independently excluded by heroClass99. Existing owner, helper and same-hero-ID exclusions remain.

`build_formation.py` verifies the complete original managed hash and target IL hash, emits a one-byte scoped overlay and checks all other raw method bodies. `CandidateProbe.cs` executes the actual original/patched selection method with controlled inventory objects populated from the packaged Hero table. It covers all six green and twelve blue combat cards, both material qualities, a synthetic white boundary, mounted/helper/duplicate cases and an empty bag. This probe executes the managed method without a scene; phone UI evidence is recorded separately.

The server legacy eligibility threshold is aligned to quality>11. Its original-wire native formation route already permits legitimate green cards and preserves material, owner, duplicate, helper and dispatch protection. The isolated MySQL regression tests both routes, all18 cards, cold restore and battle-player projection; it does not alter production players.

Build the overlay in a new directory, run both probes, then use the established `tools/client_release/package_native.py` against official79 with build2026102803/version1.0.80. No Hero/resource/skill/attribute rows or DEX changes are required. Formal signing, compressed inherited entries and APK structure checks remain mandatory.
