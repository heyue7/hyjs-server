package service

import "testing"

// This readonly current Demo4/3 source retains every actual hero, skill, item and GUID.
// Only independent local seed/initiative are selected; no business result is produced.
func TestP6NativeActualDemoBranchesV142OriginalAutomaticCoreAndDynamic(t *testing.T) {
	const file = "native-actual-demo-branches-golden-v142.json"
	const sha = "39646957343d78f90b5dff85fc9fc001002e44510075124df876dfa9cb93a8be"
	t.Run("core", func(t *testing.T) { p6NativeAutomaticPVPOriginalTickGolden(t, file, sha, 8) })
	t.Run("actor44", func(t *testing.T) { p6NativeActorPropertiesFromFixture(t, "", true, file, sha, 8) })
	t.Run("lifetimes", func(t *testing.T) { p6NativeDetailsFromFixture(t, "", true, file, sha, 8) })
	t.Run("Buffmetadata", func(t *testing.T) { p6NativeBuffSourceAndValuesFromFixture(t, "", true, file, sha, 8) })
	t.Run("manager", func(t *testing.T) {
		p6NativeOriginalManagerFixture(t, "native-actual-demo-branches-manager-golden-v142.json", "8245983b09babd572777a0dcde0bd572da1accd130dca5b8222abf93a2df6b1d", 1, 8)
	})
}

func TestP6NativeActualDemoBranchesV142OriginalAutomaticExports(t *testing.T) {
	const file = "native-actual-demo-branches-hit-golden-v142.json"
	const sha = "7607348b0d46ea2da040a1f6dc9a8f194ba245f833be96aab03546f9a313df51"
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

func TestP6NativeActualDemoBranchesV142OriginalAutomaticFinalProtocolBytes(t *testing.T) {
	p6NativeOriginalProtocolRecordFixture(t, "native-actual-demo-branches-original-records-v142.json", "cbcab0733e24f12404264312d628f48f4b619b6b7eb495618279b1e148c781f4", false, 8)
}
