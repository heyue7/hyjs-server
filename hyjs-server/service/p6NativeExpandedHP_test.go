package service

import "testing"

func TestP6NativeOriginalExpandedHPCoverageTickGolden(t *testing.T) {
	p6NativeAutomaticPVPOriginalTickGolden(t, "native-next-real-golden-v57.json", "c0d2304f65d0891fd27cf267d42468207d9d43367f4b9dbb3bff78cfe8b9c181", 14)
}

func TestP6NativeOriginalExpandedHPCoverageDynamicGoldens(t *testing.T) {
	const (
		file = "native-next-real-golden-v57.json"
		sha  = "c0d2304f65d0891fd27cf267d42468207d9d43367f4b9dbb3bff78cfe8b9c181"
	)
	t.Run("44-actor-properties-and-counter-mode", func(t *testing.T) { p6NativeActorPropertiesFromFixture(t, "", true, file, sha) })
	t.Run("skill-CD-Buff-lifetime-shields", func(t *testing.T) { p6NativeDetailsFromFixture(t, "", true, file, sha) })
	t.Run("Buff-source-values-flags", func(t *testing.T) { p6NativeBuffSourceAndValuesFromFixture(t, "", true, file, sha) })
}
