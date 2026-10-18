package service

import "testing"

// Same actual paid Daily50 source under the explicitly named local no-target
// controller recovery. This never certifies strict original automatic parity.
func TestP6NativeDaily50CompatibilityV108CoreAndDynamic(t *testing.T) {
	const file = "native-local-daily50-compat-golden-v108.json"
	const sha = "45db90ac490109975123ac7e14dccc749205aef5925f295efd3847bd7661769d"
	t.Run("core", func(t *testing.T) { p6NativeAutomaticPVPOriginalTickGolden(t, file, sha, 1) })
	t.Run("actor44", func(t *testing.T) { p6NativeActorPropertiesFromFixture(t, "", true, file, sha, 1) })
	t.Run("lifetimes", func(t *testing.T) { p6NativeDetailsFromFixture(t, "", true, file, sha, 1) })
	t.Run("Buffmetadata", func(t *testing.T) { p6NativeBuffSourceAndValuesFromFixture(t, "", true, file, sha, 1) })
	t.Run("manager", func(t *testing.T) {
		p6NativeOriginalManagerFixture(t, "native-local-daily50-compat-manager-golden-v108.json", "e2369e881cb4bf1b2b59a8ecbf1876502cddae45ffce352741d852f13179d575", 1, 1)
	})
}

func TestP6NativeDaily50CompatibilityV108Exports(t *testing.T) {
	const file = "native-local-daily50-compat-hit-golden-v108.json"
	const sha = "fa5a135e6bbad41390ca8254215196c35a5538b5dd2e9557a3b2ca4621f907c6"
	t.Run("HP-AP-combo-control", func(t *testing.T) { p6OriginalPrimaryHPFixture(t, file, sha, 1, 2, 3, 4, 5, 6) })
	t.Run("cast-values-order", func(t *testing.T) { p6OriginalOrderedCastFixture(t, file, sha) })
	t.Run("cast-complete-collection", func(t *testing.T) { p6OriginalCastCollectionFixture(t, file, sha) })
	t.Run("cast-loads-dead", func(t *testing.T) { p6OriginalCastLifecycleFixture(t, file, sha) })
	t.Run("Buff-HP-AP", func(t *testing.T) {
		p6OriginalBuffHPFixture(t, file, sha, 1)
		p6OriginalBuffHPFixture(t, file, sha, 1)
	})
	t.Run("Buff-internal-order", func(t *testing.T) { p6OriginalOrderedBuffFixture(t, file, sha) })
	t.Run("standalone-full-collection", func(t *testing.T) { p6OriginalStandaloneMixedCollectionFixture(t, file, sha) })
}

func TestP6NativeDaily50CompatibilityV108FinalProtocolBytes(t *testing.T) {
	p6NativeOriginalProtocolRecordFixture(t, "native-local-daily50-compat-original-records-v108.json", "094b6de34f83d92dfc7b7782893c9344839cbe13ea7f0e127555a07f4c5cfa1d", false, 1)
}
