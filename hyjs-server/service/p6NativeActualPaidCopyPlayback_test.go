package service

import "testing"

// Exact actually accepted player record drives the original PlaybackAI;
// independently compare all native values and rerecorded actual protocol bytes.
func TestP6NativeActualPaidCopyOriginalPlaybackCoreAndDynamic(t *testing.T) {
	const file = "native-actual-paid-copy-playback-golden-v89.json"
	const sha = "43d435d9075c7138dd790402985d8c1ef40240391326ecb6965050a616c69dad"
	t.Run("core", func(t *testing.T) { p6NativeAutomaticPVPOriginalTickGolden(t, file, sha, 2) })
	t.Run("actor44", func(t *testing.T) { p6NativeActorPropertiesFromFixture(t, "", true, file, sha, 2) })
	t.Run("lifetimes", func(t *testing.T) { p6NativeDetailsFromFixture(t, "", true, file, sha, 2) })
	t.Run("Buffmetadata", func(t *testing.T) { p6NativeBuffSourceAndValuesFromFixture(t, "", true, file, sha, 2) })
	t.Run("manager", func(t *testing.T) {
		p6NativeOriginalManagerFixture(t, "native-actual-paid-copy-playback-manager-golden-v89.json", "f5a939d36047b744384018c947917e7c25ceeab39643a64136d1317448a4e283", 1, 2)
	})
}

func TestP6NativeActualPaidCopyOriginalPlaybackExports(t *testing.T) {
	const file = "native-actual-paid-copy-playback-hit-golden-v89.json"
	const sha = "b6bae7a884da9a4a9771c5717013b5bdaf9b5f5320bec24f6317d64b4899ce25"
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

func TestP6NativeActualPaidCopyOriginalPlaybackFinalProtocolBytes(t *testing.T) {
	p6NativeOriginalProtocolRecordFixture(t, "native-actual-paid-copy-playback-original-records-v89.json", "4a3801741fdc6cf0c250477e82eb3714932b2dc22ae80e485817dc5dcc2fc156", false)
}
