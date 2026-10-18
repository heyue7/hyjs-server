# Original promotion self identity

`UI.ServerBattlePromotion.UpdateList` previously matched self labels by numeric `pid` alone. The controlled opt-in patch first requires the original `RankRowInfo.values[1]` realm to equal `StaticData.playerData.ZoneId`, then preserves the original `pid` comparison. All rows, original protobuf identities, score and rank values, and formation links remain unchanged. Missing three-value rows cannot become self.

Build from the original APK with `tools/build_client.py --repair-promotion-rank-self-zone` plus the existing local build flags. The injector accepts only the original single self-comparison shape. The verifier checks every other original method and all 547 engine instruction regions and exception regions. The original local signed APK remains the immediate rollback; native UI verification is recorded separately.

`build.py --input <Assembly-CSharp.dll> --output-directory <empty-directory>` verifies the isolated patch without packaging/installing. Temporary compiler projects are always removed; the caller cleans packaging intermediates and redundant assemblies once the signed deliverable is verified.
