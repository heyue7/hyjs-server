package service

import "testing"

// Actual two-realm paid Prayer/purpose2 sources preserve their original actors,
// integer HP fold and Inst00 fallback. Original typed17 automatic driver owns this scope.
func TestP6NativeQualifiedPrayer17V139OriginalAutomaticCoreAndDynamic(t *testing.T) {
	const file = "native-qualified-prayer-type17-golden-v139.json"
	const sha = "2658ba2869bfde4465f35848922de6b12c9e7f9fc45c6cac29603875fdc36baf"
	t.Run("core", func(t *testing.T) { p6NativeAutomaticPVPOriginalTickGolden(t, file, sha, 3) })
	t.Run("actor44", func(t *testing.T) { p6NativeActorPropertiesFromFixture(t, "", true, file, sha, 3) })
	t.Run("lifetimes", func(t *testing.T) { p6NativeDetailsFromFixture(t, "", true, file, sha, 3) })
	t.Run("Buffmetadata", func(t *testing.T) { p6NativeBuffSourceAndValuesFromFixture(t, "", true, file, sha, 3) })
	t.Run("manager", func(t *testing.T) {
		p6NativeOriginalManagerFixture(t, "native-qualified-prayer-type17-manager-golden-v139.json", "b4c81713cc98e2dfcf8aee93435a4906ff09e973bc45c083201b92c003e9a3b6", 1, 3)
	})
}

func TestP6NativeQualifiedPrayer17V139OriginalAutomaticExports(t *testing.T) {
	const file = "native-qualified-prayer-type17-hit-golden-v139.json"
	const sha = "a9b693b67abd434c3017e91b748d26f1c43f376e4ed6e43e84deeb0996541e22"
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

func TestP6NativeQualifiedPrayer17V139OriginalAutomaticFinalProtocolBytes(t *testing.T) {
	p6NativeOriginalProtocolRecordFixture(t, "native-qualified-prayer-type17-original-records-v139.json", "f2b25c3c35ec900adb2f9c93531c45932bea3cbf00c6a970bbd50715b17011c7", false, 3)
}
