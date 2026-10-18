package service

import "testing"

// Every case uses its exact original cold source, input SHA and automatic
// PlayerAI driver. Compare every callback layer without using historical records.
func TestP6NativeLinearFriendV113OriginalAutomaticCoreAndDynamic(t *testing.T) {
	const file = "native-linear-physical-friend11-golden-v113.json"
	const sha = "3eeebaa4e361bb96f6e07797adc6a3f71a7f1c9ba5291e0e9dc62c2b49bacd5a"
	t.Run("core", func(t *testing.T) { p6NativeAutomaticPVPOriginalTickGolden(t, file, sha, 2) })
	t.Run("actor44", func(t *testing.T) { p6NativeActorPropertiesFromFixture(t, "", true, file, sha, 2) })
	t.Run("lifetimes", func(t *testing.T) { p6NativeDetailsFromFixture(t, "", true, file, sha, 2) })
	t.Run("Buffmetadata", func(t *testing.T) { p6NativeBuffSourceAndValuesFromFixture(t, "", true, file, sha, 2) })
	t.Run("manager", func(t *testing.T) {
		p6NativeOriginalManagerFixture(t, "native-linear-physical-friend11-manager-golden-v113.json", "591b8cce87d085e1a7d1a2e086faa3b11c24c3575cda2f7849e073f83cc9ed35", 1, 2)
	})
}

func TestP6NativeLinearFriendV113OriginalAutomaticExports(t *testing.T) {
	const file = "native-linear-physical-friend11-hit-golden-v113.json"
	const sha = "89d9e3d9a1efe822266ef75cdec94ab2966fbc50ea02690537c9514589dc5971"
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

func TestP6NativeLinearFriendV113OriginalAutomaticFinalProtocolBytes(t *testing.T) {
	p6NativeOriginalProtocolRecordFixture(t, "native-linear-physical-friend11-original-records-v113.json", "413f7d593e2385d098937698a7c561a695b4494c80d2785d7e5378848b7f36fc", false, 2)
}
