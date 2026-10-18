package service

import (
	"testing"
)

// One exact actual original manual-control source preserves every native value/clock.
// The original manual wait and live Auto control are replayed as real ticks.
// Every callback value/sequence and final protocol field stays strict.
func TestP6NativeLimitV180OriginalAutomaticCoreAndDynamic(t *testing.T) {
	const file = "native-original-limit20-strong-golden-v180.json"
	const sha = "2e554171101cf94bbc243bdb2323d0b598cf3f9754316b9ed496c973ccf79b23"
	t.Run("core", func(t *testing.T) { p6NativeAutomaticPVPOriginalTickGolden(t, file, sha, 2) })
	t.Run("actor44", func(t *testing.T) { p6NativeActorPropertiesFromFixture(t, "", true, file, sha, 2) })
	t.Run("lifetimes", func(t *testing.T) { p6NativeDetailsFromFixture(t, "", true, file, sha, 2) })
	t.Run("Buffmetadata", func(t *testing.T) { p6NativeBuffSourceAndValuesFromFixture(t, "", true, file, sha, 2) })
	t.Run("manager", func(t *testing.T) {
		p6NativeOriginalManagerFixture(t, "native-original-limit20-strong-manager-golden-v180.json", "06cae43c5acd400287d8ffb526b60ee3f1e9f4c7e9ee2b2e0c51e6210bd41b5a", 1, 2)
	})
}

func TestP6NativeLimitV180OriginalAutomaticExports(t *testing.T) {
	const file = "native-original-limit20-strong-hit-golden-v180.json"
	const sha = "2e5f72f988842726075aaa74bc2fedce797452ec1a73d2e491dd0e199c24c9b2"
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

func TestP6NativeLimitV180OriginalAutomaticFinalProtocolBytes(t *testing.T) {
	p6NativeOriginalProtocolRecordFixture(t, "native-original-limit20-strong-original-records-v180.json", "81b95216b3c7151554db5d8ae6ef056280f37c903058c5279aee26a6b610c48d", false, 2)
}
