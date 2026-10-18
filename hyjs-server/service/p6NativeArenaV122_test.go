package service

import "testing"

// Every case uses its exact actual query-frozen typed arena source, input SHA and automatic
// PlayerAI driver. Compare every callback layer without using historical records.
func TestP6NativeArenaV122OriginalAutomaticCoreAndDynamic(t *testing.T) {
	const file = "native-actual-arena047-typed-golden-v122.json"
	const sha = "53b5af8e2b4ab0bd8b708f4cc84b33d06e66434c600a3803a19215898eaee6d4"
	t.Run("core", func(t *testing.T) { p6NativeAutomaticPVPOriginalTickGolden(t, file, sha, 3) })
	t.Run("actor44", func(t *testing.T) { p6NativeActorPropertiesFromFixture(t, "", true, file, sha, 3) })
	t.Run("lifetimes", func(t *testing.T) { p6NativeDetailsFromFixture(t, "", true, file, sha, 3) })
	t.Run("Buffmetadata", func(t *testing.T) { p6NativeBuffSourceAndValuesFromFixture(t, "", true, file, sha, 3) })
	t.Run("manager", func(t *testing.T) {
		p6NativeOriginalManagerFixture(t, "native-actual-arena047-typed-manager-golden-v122.json", "8640f10864118535c5d455889ae17703867ba9f7a106b75df6e31b96b02d16ae", 1, 3)
	})
}

func TestP6NativeArenaV122OriginalAutomaticExports(t *testing.T) {
	const file = "native-actual-arena047-typed-hit-golden-v122.json"
	const sha = "c4a4652dc0bfa55175f51dd37929aad811476372e7964c4c2546a92fdb80ecf5"
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

func TestP6NativeArenaV122OriginalAutomaticFinalProtocolBytes(t *testing.T) {
	p6NativeOriginalProtocolFixtureWithTerminalHP(t, "native-actual-arena047-typed-original-records-v122.json", "09916ab69abbe6d23a4cc815193da57fa086478ffd33186e02f74134dd9fed76", false, true, 3)
}
