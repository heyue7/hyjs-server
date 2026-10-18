package service

import "testing"

// Eight independent original Arena callbacks retain complete actual mixed/mirror parties.
// The original runtime actually begins all four ultimate IDs, including long
// battles and timeout. Every callback value/sequence and terminal field stays strict.
func TestP6NativeFullPartyArenaV153OriginalAutomaticCoreAndDynamic(t *testing.T) {
	const file = "native-full-party-arena2-golden-v153.json"
	const sha = "16958c167b09311cbef5eced825a26b9cc12fdc84ef03efe2c9318aa139ea102"
	t.Run("core", func(t *testing.T) { p6NativeAutomaticPVPOriginalTickGolden(t, file, sha, 8) })
	t.Run("actor44", func(t *testing.T) { p6NativeActorPropertiesFromFixture(t, "", true, file, sha, 8) })
	t.Run("lifetimes", func(t *testing.T) { p6NativeDetailsFromFixture(t, "", true, file, sha, 8) })
	t.Run("Buffmetadata", func(t *testing.T) { p6NativeBuffSourceAndValuesFromFixture(t, "", true, file, sha, 8) })
	t.Run("manager", func(t *testing.T) {
		p6NativeOriginalManagerFixture(t, "native-full-party-arena2-manager-golden-v153.json", "c075cd6b0349be5c0aa7c2f5ece711ce6d10352dc0543069847e3fd77bf16c6b", 1, 8)
	})
}

func TestP6NativeFullPartyArenaV153OriginalAutomaticExports(t *testing.T) {
	const file = "native-full-party-arena2-hit-golden-v153.json"
	const sha = "6b37bfef2052c883a21513f30ea0f69150a9c24813ec0822a48ab212d94477d8"
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

func TestP6NativeFullPartyArenaV153OriginalAutomaticFinalProtocolBytes(t *testing.T) {
	p6NativeOriginalProtocolRecordFixture(t, "native-full-party-arena2-original-records-v153.json", "e5559cd57536d9248bb25477db977fd16bc5fb31d2b290fa8d5aa24c05c513b5", false, 8)
}
