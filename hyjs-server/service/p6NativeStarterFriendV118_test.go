package service

import "testing"

// Every case uses its exact original cold source, input SHA and automatic
// PlayerAI driver. Compare every callback layer without using historical records.
func TestP6NativeStarterFriendV118OriginalAutomaticCoreAndDynamic(t *testing.T) {
	const file = "native-legal-starter-friend11-golden-v118.json"
	const sha = "16948272a37b0e9a85fb5edf40b88c354e33a95a962bb9a59c041e9a94626e1e"
	t.Run("core", func(t *testing.T) { p6NativeAutomaticPVPOriginalTickGolden(t, file, sha, 8) })
	t.Run("actor44", func(t *testing.T) { p6NativeActorPropertiesFromFixture(t, "", true, file, sha, 8) })
	t.Run("lifetimes", func(t *testing.T) { p6NativeDetailsFromFixture(t, "", true, file, sha, 8) })
	t.Run("Buffmetadata", func(t *testing.T) { p6NativeBuffSourceAndValuesFromFixture(t, "", true, file, sha, 8) })
	t.Run("manager", func(t *testing.T) {
		p6NativeOriginalManagerFixture(t, "native-legal-starter-friend11-manager-golden-v118.json", "a9541521b902d7141b016c0612ea6dbf3afa314410cf8eb798e13b032632aa03", 1, 8)
	})
}

func TestP6NativeStarterFriendV118OriginalAutomaticExports(t *testing.T) {
	const file = "native-legal-starter-friend11-hit-golden-v118.json"
	const sha = "2dd56aeceb57d2929ae5b2d8bf21d37b5fe0c8cdd1e8e937fd79a7f15526f30a"
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

func TestP6NativeStarterFriendV118OriginalAutomaticFinalProtocolBytes(t *testing.T) {
	p6NativeOriginalProtocolFixtureWithTerminalHP(t, "native-legal-starter-friend11-original-records-v118.json", "0d707d437df627ef380b8bf63ed7f515a6f6ec976cd8806d15238a07c61d29a8", false, true, 8)
}
