package service

import "testing"

// Four terminal sources came from the actual finite local P7 producer and original
// typed9/10 callback. Player automatic mode is explicit; the enemy uses the
// original CopyEnemyAI. No private-server floors or criterion rules are inferred.
func TestP6NativeRushTypedV136OriginalAutomaticCoreAndDynamic(t *testing.T) {
	const file = "native-local-p7-rush-terminal-golden-v136.json"
	const sha = "df73678f1494a5749a0a801e327ca57643386eef523091ac06b1a01d17523540"
	t.Run("core", func(t *testing.T) { p6NativeAutomaticPVPOriginalTickGolden(t, file, sha, 4) })
	t.Run("actor44", func(t *testing.T) { p6NativeActorPropertiesFromFixture(t, "", true, file, sha, 4) })
	t.Run("lifetimes", func(t *testing.T) { p6NativeDetailsFromFixture(t, "", true, file, sha, 4) })
	t.Run("Buffmetadata", func(t *testing.T) { p6NativeBuffSourceAndValuesFromFixture(t, "", true, file, sha, 4) })
	t.Run("manager", func(t *testing.T) {
		p6NativeOriginalManagerFixture(t, "native-local-p7-rush-terminal-manager-golden-v136.json", "40da32cde6ce622b1daa5ad2c9604de0b7429d5ca3c2425b8565bd1cb8de6743", 1, 4)
	})
}

func TestP6NativeRushTypedV136OriginalAutomaticExports(t *testing.T) {
	const file = "native-local-p7-rush-terminal-hit-golden-v136.json"
	const sha = "44952189b9a2aa58d52766149356739d5cedadae48d12ec9deef5cd6c4cc1579"
	t.Run("HP-AP-combo-control", func(t *testing.T) { p6OriginalPrimaryHPFixture(t, file, sha, 1, 2, 3, 4, 5, 6) })
	t.Run("cast-values-order", func(t *testing.T) { p6OriginalOrderedCastFixture(t, file, sha) })
	t.Run("cast-complete-collection", func(t *testing.T) { p6OriginalCastCollectionFixture(t, file, sha) })
	t.Run("cast-loads-dead", func(t *testing.T) { p6OriginalCastLifecycleFixture(t, file, sha) })
	t.Run("Buff-HP-AP", func(t *testing.T) {
		p6OriginalBuffHPFixture(t, file, sha, 1)
		p6OriginalBuffHPFixture(t, file, sha, 2)
	})
	t.Run("Buff-internal-order", func(t *testing.T) { p6OriginalOrderedBuffFixture(t, file, sha) })
	t.Run("standalone-full-collection", func(t *testing.T) { p6OriginalStandaloneMixedCollectionFixture(t, file, sha) })
}

func TestP6NativeRushTypedV136OriginalAutomaticFinalProtocolBytes(t *testing.T) {
	p6NativeOriginalProtocolRecordFixture(t, "native-local-p7-rush-terminal-original-records-v136.json", "e233684f8f6c53bb083950b320239f7c0901fde8343d8cf954cc4b9b04bb50ef", false, 4)
}

func TestP6NativeRushTypedV136PriorActualWinsFinalHP(t *testing.T) {
	p6NativeOriginalProtocolRecordFixture(t, "native-local-p7-rush-typed-terminal-original-records-v136.json", "17229a397bd4b6f5b24c46d1c7416056fc05f7a8f7216eef943a06b13fc82f51", false, 2)
}
