package service

import "testing"

// Both sources came from the actual finite local P7 producer and original
// typed9/10 callback. Player automatic mode is explicit; the enemy uses the
// original CopyEnemyAI. No private-server floors or criterion rules are inferred.
func TestP6NativeRushTypedV106OriginalAutomaticCoreAndDynamic(t *testing.T) {
	const file = "native-local-p7-rush-typed-golden-v106.json"
	const sha = "05c182964266c09e844e7b47d7774b58976214caa735ad6eb90a95182f39df58"
	t.Run("core", func(t *testing.T) { p6NativeAutomaticPVPOriginalTickGolden(t, file, sha, 2) })
	t.Run("actor44", func(t *testing.T) { p6NativeActorPropertiesFromFixture(t, "", true, file, sha, 2) })
	t.Run("lifetimes", func(t *testing.T) { p6NativeDetailsFromFixture(t, "", true, file, sha, 2) })
	t.Run("Buffmetadata", func(t *testing.T) { p6NativeBuffSourceAndValuesFromFixture(t, "", true, file, sha, 2) })
	t.Run("manager", func(t *testing.T) {
		p6NativeOriginalManagerFixture(t, "native-local-p7-rush-typed-manager-golden-v106.json", "ccca7f19f5f325f9a04deae44da67cc4ad438761d2260067cc43aceda99073ae", 1, 2)
	})
}

func TestP6NativeRushTypedV106OriginalAutomaticExports(t *testing.T) {
	const file = "native-local-p7-rush-typed-hit-golden-v106.json"
	const sha = "254d7d06b78371aafc24a55b4cc2e22f7484374e555514bf85cde19a9258d3c4"
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

func TestP6NativeRushTypedV106OriginalAutomaticFinalProtocolBytes(t *testing.T) {
	p6NativeOriginalProtocolRecordFixture(t, "native-local-p7-rush-typed-original-records-v106.json", "17229a397bd4b6f5b24c46d1c7416056fc05f7a8f7216eef943a06b13fc82f51", false, 2)
}
