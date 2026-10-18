package service

import "testing"

// This readonly current Demo4/3 source retains every actual hero, skill, item and GUID.
// Only independent local seed/initiative are selected; no business result is produced.
func TestP6NativeActualDemoFriendV138OriginalAutomaticCoreAndDynamic(t *testing.T) {
	const file = "native-actual-demo4-vs-demo3-friend11-golden-v138.json"
	const sha = "ab2447863985359038e37b94f543dd5ca1f0b4d8c11b4c7f43473975b7dd70dd"
	t.Run("core", func(t *testing.T) { p6NativeAutomaticPVPOriginalTickGolden(t, file, sha, 1) })
	t.Run("actor44", func(t *testing.T) { p6NativeActorPropertiesFromFixture(t, "", true, file, sha, 1) })
	t.Run("lifetimes", func(t *testing.T) { p6NativeDetailsFromFixture(t, "", true, file, sha, 1) })
	t.Run("Buffmetadata", func(t *testing.T) { p6NativeBuffSourceAndValuesFromFixture(t, "", true, file, sha, 1) })
	t.Run("manager", func(t *testing.T) {
		p6NativeOriginalManagerFixture(t, "native-actual-demo4-vs-demo3-friend11-manager-golden-v138.json", "7189df406f62e30750658b2c1707124ae940daa3c75a33e745494bffab8134ec", 1, 1)
	})
}

func TestP6NativeActualDemoFriendV138OriginalAutomaticExports(t *testing.T) {
	const file = "native-actual-demo4-vs-demo3-friend11-hit-golden-v138.json"
	const sha = "bff0c72a42650d22d63c5ee143c122371bcb10e439ae0cbda9b2d9e92fd3196a"
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

func TestP6NativeActualDemoFriendV138OriginalAutomaticFinalProtocolBytes(t *testing.T) {
	p6NativeOriginalProtocolRecordFixture(t, "native-actual-demo4-vs-demo3-friend11-original-records-v138.json", "e512041dd494ff6b57e1aacb53e2fbfe28c41eef04548d1e0324e934d4acf4e4", false, 1)
}
