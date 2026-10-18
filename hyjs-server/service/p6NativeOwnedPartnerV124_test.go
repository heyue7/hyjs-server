package service

import "testing"

// Every case uses actual owned047 and claimed069/070 source, input SHA and automatic
// PlayerAI driver. Compare every callback layer without using historical records.
func TestP6NativeOwnedPartnerV124OriginalAutomaticCoreAndDynamic(t *testing.T) {
	const file = "native-owned-partner-friend11-golden-v124.json"
	const sha = "83cc1ec6857472c356949f7ffa37f156ad4306341efcdf729ab975526f299dd1"
	t.Run("core", func(t *testing.T) { p6NativeAutomaticPVPOriginalTickGolden(t, file, sha, 4) })
	t.Run("actor44", func(t *testing.T) { p6NativeActorPropertiesFromFixture(t, "", true, file, sha, 4) })
	t.Run("lifetimes", func(t *testing.T) { p6NativeDetailsFromFixture(t, "", true, file, sha, 4) })
	t.Run("Buffmetadata", func(t *testing.T) { p6NativeBuffSourceAndValuesFromFixture(t, "", true, file, sha, 4) })
	t.Run("manager", func(t *testing.T) {
		p6NativeOriginalManagerFixture(t, "native-owned-partner-friend11-manager-golden-v124.json", "d3f5b761db2a104a539f3f71cd1545a522bfab7818ca3da34e126290f0405e87", 1, 4)
	})
}

func TestP6NativeOwnedPartnerV124OriginalAutomaticExports(t *testing.T) {
	const file = "native-owned-partner-friend11-hit-golden-v124.json"
	const sha = "38942ffb7ad31d201b440694bf3bf0732e2ea478c945e57e929b813006d211ee"
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

func TestP6NativeOwnedPartnerV124OriginalAutomaticFinalProtocolBytes(t *testing.T) {
	p6NativeOriginalProtocolFixtureWithTerminalHP(t, "native-owned-partner-friend11-original-records-v124.json", "82219d4c239f0a146574381d16fbd20ca32698d38cbc1a6d785565d7ae04a154", false, true, 4)
}
