package service

import "testing"

// Original complete actual-party mirrors retain all actors, panels and skills.
// The original runtime actually begins all four ultimate IDs, including long
// battles and timeout. Every callback value/sequence and terminal field stays strict.
func TestP6NativeActualDemoUltimateV150OriginalAutomaticCoreAndDynamic(t *testing.T) {
	const file = "native-actual-demo-ultimate-golden-v150.json"
	const sha = "074d2762cc0f1b6feb0ae5fe094db10c0a76ea479f12ff1596869ffb72100c14"
	t.Run("core", func(t *testing.T) { p6NativeAutomaticPVPOriginalTickGolden(t, file, sha, 4) })
	t.Run("actor44", func(t *testing.T) { p6NativeActorPropertiesFromFixture(t, "", true, file, sha, 4) })
	t.Run("lifetimes", func(t *testing.T) { p6NativeDetailsFromFixture(t, "", true, file, sha, 4) })
	t.Run("Buffmetadata", func(t *testing.T) { p6NativeBuffSourceAndValuesFromFixture(t, "", true, file, sha, 4) })
	t.Run("manager", func(t *testing.T) {
		p6NativeOriginalManagerFixture(t, "native-actual-demo-ultimate-manager-golden-v150.json", "1e312c550d40654f8b033f0451197f9397120b48b762fd81869421ff87e86c7e", 1, 4)
	})
}

func TestP6NativeActualDemoUltimateV150OriginalAutomaticExports(t *testing.T) {
	const file = "native-actual-demo-ultimate-hit-golden-v150.json"
	const sha = "eba2f9dbec8f16871e83036c7c847515574d02ef3714a2a87e791676acf993ac"
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

func TestP6NativeActualDemoUltimateV150OriginalAutomaticFinalProtocolBytes(t *testing.T) {
	p6NativeOriginalProtocolRecordFixture(t, "native-actual-demo-ultimate-original-records-v150.json", "4413b7e211e25fde59411efeefbc75ebcc20c75f4c0f0c996bfeb9e8ac5a2b18", false, 4)
}
