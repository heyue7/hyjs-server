package service

import "testing"

// Eleven original Plunder3 Max20 callbacks retain all original single and full-party sources.
// The original runtime actually begins all four ultimate IDs, including long
// battles and timeout. Every callback value/sequence and terminal field stays strict.
func TestP6NativePlunderV162OriginalAutomaticCoreAndDynamic(t *testing.T) {
	const file = "native-original-plunder3-golden-v162.json"
	const sha = "0c9b0441a246093d0f046179b3d1d67201eb6b128715a35e2632c0e80f95d66e"
	t.Run("core", func(t *testing.T) { p6NativeAutomaticPVPOriginalTickGolden(t, file, sha, 11) })
	t.Run("actor44", func(t *testing.T) { p6NativeActorPropertiesFromFixture(t, "", true, file, sha, 11) })
	t.Run("lifetimes", func(t *testing.T) { p6NativeDetailsFromFixture(t, "", true, file, sha, 11) })
	t.Run("Buffmetadata", func(t *testing.T) { p6NativeBuffSourceAndValuesFromFixture(t, "", true, file, sha, 11) })
	t.Run("manager", func(t *testing.T) {
		p6NativeOriginalManagerFixture(t, "native-original-plunder3-manager-golden-v162.json", "99571f841d7b6f6209468d09af75d1ead15d99338b1eaccf2d1eee822a690b20", 1, 11)
	})
}

func TestP6NativePlunderV162OriginalAutomaticExports(t *testing.T) {
	const file = "native-original-plunder3-hit-golden-v162.json"
	const sha = "0dbf163e74a127c834aad91d6977738d4fb244db6dd8671b27f654e329480ddb"
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

func TestP6NativePlunderV162OriginalAutomaticFinalProtocolBytes(t *testing.T) {
	p6NativeOriginalProtocolRecordFixture(t, "native-original-plunder3-original-records-v162.json", "61c172dc55c22c239d37c26a0cdb945ac9eb27b1b8240816ea54348b1e074497", false, 11)
}
