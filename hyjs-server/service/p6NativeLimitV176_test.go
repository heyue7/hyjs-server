package service

import (
	"testing"
)

// One exact actual original manual-control source preserves every native value/clock.
// The original manual wait and live Auto control are replayed as real ticks.
// Every callback value/sequence and final protocol field stays strict.
func TestP6NativeLimitV176OriginalAutomaticCoreAndDynamic(t *testing.T) {
	const file = "native-original-limit20-golden-v176.json"
	const sha = "1b6ec6f0ea0cddd5004294c003c93fbf77854d482f99629e3ae7eff4611fd88b"
	t.Run("core", func(t *testing.T) { p6NativeAutomaticPVPOriginalTickGolden(t, file, sha, 1) })
	t.Run("actor44", func(t *testing.T) { p6NativeActorPropertiesFromFixture(t, "", true, file, sha, 1) })
	t.Run("lifetimes", func(t *testing.T) { p6NativeDetailsFromFixture(t, "", true, file, sha, 1) })
	t.Run("Buffmetadata", func(t *testing.T) { p6NativeBuffSourceAndValuesFromFixture(t, "", true, file, sha, 1) })
	t.Run("manager", func(t *testing.T) {
		p6NativeOriginalManagerFixture(t, "native-original-limit20-manager-golden-v176.json", "15749756fc36d6c258fae45975008c791b1e1f8f4f94862385fc6bec0de1e8fc", 1, 1)
	})
}

func TestP6NativeLimitV176OriginalAutomaticExports(t *testing.T) {
	const file = "native-original-limit20-hit-golden-v176.json"
	const sha = "3ee03f7ae6bdcb46e3ba679ed113aee594b17a747e8624931ad39e9c27ea728d"
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

func TestP6NativeLimitV176OriginalAutomaticFinalProtocolBytes(t *testing.T) {
	p6NativeOriginalProtocolRecordFixture(t, "native-original-limit20-original-records-v176.json", "037edbc7abb4f1e18e278988b2ce66d59a1390e49be413dbdf21519add810047", false, 1)
}
