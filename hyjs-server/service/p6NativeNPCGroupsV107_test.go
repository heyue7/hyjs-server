package service

import "testing"

// Every case uses its exact original cold source, input SHA and automatic
// PlayerAI driver. Compare every callback layer without using historical records.
func TestP6NativeNPCGroupsV107OriginalAutomaticCoreAndDynamic(t *testing.T) {
	const file = "native-original-copy-enemy-mechanism-golden-v107.json"
	const sha = "077855e3eef992baba97dfa4ae238bc75731e5d6ea016d31cd63514e114f81ee"
	t.Run("core", func(t *testing.T) { p6NativeAutomaticPVPOriginalTickGolden(t, file, sha, 14) })
	t.Run("actor44", func(t *testing.T) { p6NativeActorPropertiesFromFixture(t, "", true, file, sha, 14) })
	t.Run("lifetimes", func(t *testing.T) { p6NativeDetailsFromFixture(t, "", true, file, sha, 14) })
	t.Run("Buffmetadata", func(t *testing.T) { p6NativeBuffSourceAndValuesFromFixture(t, "", true, file, sha, 14) })
	t.Run("manager", func(t *testing.T) {
		p6NativeOriginalManagerFixture(t, "native-original-copy-enemy-mechanism-manager-golden-v107.json", "7fb2206b6beef721da2980633cfaf7965213aab4f5e828c41e27df3c5dfbc4c0", 1, 14)
	})
}

func TestP6NativeNPCGroupsV107OriginalAutomaticExports(t *testing.T) {
	const file = "native-original-copy-enemy-mechanism-hit-golden-v107.json"
	const sha = "fa69cc5ec589da675679fc444ee1d8c1d1d52d9e8cb1be1797f2f55206679f84"
	t.Run("HP-AP-combo-control", func(t *testing.T) { p6OriginalPrimaryHPFixture(t, file, sha, 1, 2, 3, 4, 5, 6) })
	t.Run("cast-values-order", func(t *testing.T) { p6OriginalOrderedCastFixture(t, file, sha) })
	t.Run("cast-complete-collection", func(t *testing.T) { p6OriginalCastCollectionFixture(t, file, sha) })
	t.Run("cast-loads-dead", func(t *testing.T) { p6OriginalCastLifecycleFixture(t, file, sha) })
	t.Run("Buff-HP-AP", func(t *testing.T) {
		p6OriginalBuffHPFixture(t, file, sha, 1)
		p6OriginalBuffHPFixture(t, file, sha, 14)
	})
	t.Run("Buff-internal-order", func(t *testing.T) { p6OriginalOrderedBuffFixture(t, file, sha) })
	t.Run("standalone-full-collection", func(t *testing.T) { p6OriginalStandaloneMixedCollectionFixture(t, file, sha) })
}

func TestP6NativeNPCGroupsV107OriginalAutomaticFinalProtocolBytes(t *testing.T) {
	p6NativeOriginalProtocolRecordFixture(t, "native-original-copy-enemy-mechanism-original-records-v107.json", "fa172ccc81f1c718f1fe2f09b70be043792b3e002cbfdbdda70fbe1e8d57a9f8", false, 14)
}
