package service

import "testing"

// Original-rule probes from exact real APK skill/talent rows and explicit currentHP0.
// They are separate from formal-account sources and from the original PVP sample denominator.
func TestP6NativeInitialZeroHPOriginalAutomaticCoreAndDynamic(t *testing.T) {
	const file = "native-initial-zero-hp-golden-v94.json"
	const sha = "0094b4189f4e58f6687d621f6bf3e95c3a5607f05318fad0033dd73c23011f78"
	t.Run("core", func(t *testing.T) { p6NativeAutomaticPVPOriginalTickGolden(t, file, sha, 2) })
	t.Run("actor44", func(t *testing.T) { p6NativeActorPropertiesFromFixture(t, "", true, file, sha, 2) })
	t.Run("lifetimes", func(t *testing.T) { p6NativeDetailsFromFixture(t, "", true, file, sha, 2) })
	t.Run("Buffmetadata", func(t *testing.T) { p6NativeBuffSourceAndValuesFromFixture(t, "", true, file, sha, 2) })
	t.Run("manager", func(t *testing.T) {
		p6NativeOriginalManagerFixture(t, "native-initial-zero-hp-manager-golden-v94.json", "75c96a7018bf1475daec93c4b8a66631b977c524531b9974d16b6a650c805a0b", 1, 2)
	})
}

func TestP6NativeInitialZeroHPOriginalAutomaticExports(t *testing.T) {
	const file = "native-initial-zero-hp-hit-golden-v94.json"
	const sha = "57ca224c07fe7c78b37cf7b9f6d28712fce46ad458a9cd7570c4bfdb00bb5541"
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

func TestP6NativeInitialZeroHPOriginalFinalProtocol(t *testing.T) {
	p6NativeOriginalProtocolRecordFixture(t, "native-initial-zero-hp-original-records-v94.json", "4e83a01adf5a536f3e58959361dfa235bf4154b4405f4b7cdee829f9daa2ce4d", false)
}
