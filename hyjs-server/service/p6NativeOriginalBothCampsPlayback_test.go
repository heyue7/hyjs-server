package service

import "testing"

// Actual original full both-camp record drives camp0 PlaybackAI with automatic NPC;
// diagnostic0/0 factors remain separate from natural paid1/1 proofs.
func TestP6NativeOriginalBothCampsRecordedPlaybackCoreAndDynamic(t *testing.T) {
	const file = "native-original-both-camps-recorded-replay-golden-v91.json"
	const sha = "5f078d28fc65c4e3568ec73f0b3d03f8d0e6aac9a0c47a6c1390766104812b36"
	t.Run("core", func(t *testing.T) { p6NativeAutomaticPVPOriginalTickGolden(t, file, sha, 2) })
	t.Run("actor44", func(t *testing.T) { p6NativeActorPropertiesFromFixture(t, "", true, file, sha, 2) })
	t.Run("lifetimes", func(t *testing.T) { p6NativeDetailsFromFixture(t, "", true, file, sha, 2) })
	t.Run("Buffmetadata", func(t *testing.T) { p6NativeBuffSourceAndValuesFromFixture(t, "", true, file, sha, 2) })
	t.Run("manager", func(t *testing.T) {
		p6NativeOriginalManagerFixture(t, "native-original-both-camps-recorded-replay-manager-golden-v91.json", "cdfd4e342d9283a69a3afc1900234cce456db0cef054e19409cab5f4026746bb", 1, 2)
	})
}

func TestP6NativeOriginalBothCampsRecordedPlaybackExports(t *testing.T) {
	const file = "native-original-both-camps-recorded-replay-hit-golden-v91.json"
	const sha = "90bd9b92f75c0a6371d2255577f0fce431211d7d0bb3e4608a51a50e9f4e3510"
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

func TestP6NativeOriginalBothCampsRecordedPlaybackFinalProtocolBytes(t *testing.T) {
	p6NativeOriginalProtocolRecordFixture(t, "native-original-both-camps-recorded-replay-original-records-v91.json", "6afcc5606a28053482bb1090425ed044556d6f746e9413bf86b445bd4bdd5a86", true)
}
