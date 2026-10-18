package service

import "testing"

// Exact original paid source and effective Clear factors1/1; automatic and
// original actual-record playback remain separate original driver proofs.
func TestP6NativeActualPaidCopyOriginalAutomaticCoreAndDynamic(t *testing.T) {
	const file = "native-actual-paid-copy-auto-golden-v88.json"
	const sha = "8f5447747fb0262419943ca69c7a88c53f2c3779d99bdf37ef82de54d4297029"
	t.Run("core", func(t *testing.T) { p6NativeAutomaticPVPOriginalTickGolden(t, file, sha, 2) })
	t.Run("actor44", func(t *testing.T) { p6NativeActorPropertiesFromFixture(t, "", true, file, sha, 2) })
	t.Run("lifetimes", func(t *testing.T) { p6NativeDetailsFromFixture(t, "", true, file, sha, 2) })
	t.Run("Buffmetadata", func(t *testing.T) { p6NativeBuffSourceAndValuesFromFixture(t, "", true, file, sha, 2) })
	t.Run("manager", func(t *testing.T) {
		p6NativeOriginalManagerFixture(t, "native-actual-paid-copy-auto-manager-golden-v88.json", "bc2452abfd89f511b29fce2c251934d17dbd7e0d3a125d3b3c6d2ccff129c36f", 1, 2)
	})
}

func TestP6NativeActualPaidCopyOriginalAutomaticExports(t *testing.T) {
	const file = "native-actual-paid-copy-auto-hit-golden-v88.json"
	const sha = "7f39ad9e70faac2d52fb5136f814ff95ae61aa01da76d73bcfcdbf0ce2b40e06"
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

func TestP6NativeActualPaidCopyOriginalAutomaticFinalProtocolBytes(t *testing.T) {
	p6NativeOriginalProtocolRecordFixture(t, "native-actual-paid-copy-auto-original-records-v88.json", "3971a0cc16f64391a8f21111777bf322473226994d007621f50e71d2c98ce288", false)
}
