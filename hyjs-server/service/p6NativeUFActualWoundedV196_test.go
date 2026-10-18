package service

import (
	"testing"
)

// Independent original SCUFFightRecordReplay.SetData with genuine current-war survivor HP/AP.
// Source192 fullHP capability is deliberately not reused for partial settlement.
// Each complete core/export/terminal comparison retains exact native clocks.
// Fixed hashes bind actual collector output and unmodified original values.
func TestP6NativeUFActualWoundedV196OriginalAutomaticCoreAndDynamic(t *testing.T) {
	const file = "native-actual-war-wounded14-golden-v196.json"
	const sha = "e53d0a4bbc6a2ce2b43c7bc69400ad34048c4b0ec19915708de3f53688bf7bc2"
	t.Run("core", func(t *testing.T) { p6NativeAutomaticPVPOriginalTickGolden(t, file, sha, 2) })
	t.Run("actor44", func(t *testing.T) { p6NativeActorPropertiesFromFixture(t, "", true, file, sha, 2) })
	t.Run("lifetimes", func(t *testing.T) { p6NativeDetailsFromFixture(t, "", true, file, sha, 2) })
	t.Run("Buffmetadata", func(t *testing.T) { p6NativeBuffSourceAndValuesFromFixture(t, "", true, file, sha, 2) })
	t.Run("manager", func(t *testing.T) {
		p6NativeOriginalManagerFixture(t, "native-actual-war-wounded14-manager-golden-v196.json", "01e02081325717dadc4aa40a5fc61d9a8d9f9943f17cbb841cd53a5b664c9c38", 1, 2)
	})
}

func TestP6NativeUFActualWoundedV196OriginalAutomaticExports(t *testing.T) {
	const file = "native-actual-war-wounded14-hit-golden-v196.json"
	const sha = "9b80e809f35083a880329cdf2f864640e1ec4e488dee5c3d78ea13c20bfdcadc"
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

func TestP6NativeUFActualWoundedV196OriginalAutomaticFinalProtocolBytes(t *testing.T) {
	p6NativeOriginalProtocolRecordFixture(t, "native-actual-war-wounded14-original-records-v196.json", "4182e8dad6003b0da9f87c2d436dd81a5917247550200f72afd82c0f38f9925f", false, 2)
}
