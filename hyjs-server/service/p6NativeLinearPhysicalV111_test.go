package service

import "testing"

// Every case uses its exact original cold source, input SHA and automatic
// PlayerAI driver. Compare every callback layer without using historical records.
func TestP6NativeLinearPhysicalV111OriginalAutomaticCoreAndDynamic(t *testing.T) {
	const file = "native-linear-physical-candidate-golden-v111.json"
	const sha = "6e4669ffcc053adfb06cdef2a989dac1895b2d6ebb8920f6e364deb5374eeb79"
	t.Run("core", func(t *testing.T) { p6NativeAutomaticPVPOriginalTickGolden(t, file, sha, 8) })
	t.Run("actor44", func(t *testing.T) { p6NativeActorPropertiesFromFixture(t, "", true, file, sha, 8) })
	t.Run("lifetimes", func(t *testing.T) { p6NativeDetailsFromFixture(t, "", true, file, sha, 8) })
	t.Run("Buffmetadata", func(t *testing.T) { p6NativeBuffSourceAndValuesFromFixture(t, "", true, file, sha, 8) })
	t.Run("manager", func(t *testing.T) {
		p6NativeOriginalManagerFixture(t, "native-linear-physical-candidate-manager-golden-v111.json", "1feb5b7595da4ff8c757d214bb70c9a6fac76e22347e5ab9d20530d80db96825", 1, 8)
	})
}

func TestP6NativeLinearPhysicalV111OriginalAutomaticExports(t *testing.T) {
	const file = "native-linear-physical-candidate-hit-golden-v111.json"
	const sha = "5dd66566e8da17c253e33e27b47ca4c20e6d0aff53480ffef51882893b785825"
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

func TestP6NativeLinearPhysicalV111OriginalAutomaticFinalProtocolBytes(t *testing.T) {
	p6NativeOriginalProtocolRecordFixture(t, "native-linear-physical-candidate-original-records-v111.json", "dcefaed808217c6cbb859f79bf2de27475ce5099dcd6caf119d3a174518a3c7c", false, 8)
}
