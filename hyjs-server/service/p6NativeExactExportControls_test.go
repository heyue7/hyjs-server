package service

import "testing"

func TestP6NativeOriginalExactExportCoverageTickGolden(t *testing.T) {
	p6NativeAutomaticPVPOriginalTickGolden(t, "native-next-real-golden-v58.json", "8430083aa0bb0ac2aba8608d6e9a631f1515098bf88553cc83a6366aa2ec96c7", 14)
}
func TestP6NativeOriginalExactExportCoverageDynamicGoldens(t *testing.T) {
	const (
		file = "native-next-real-golden-v58.json"
		sha  = "8430083aa0bb0ac2aba8608d6e9a631f1515098bf88553cc83a6366aa2ec96c7"
	)
	t.Run("44-actor-properties-and-counter-mode", func(t *testing.T) { p6NativeActorPropertiesFromFixture(t, "", true, file, sha) })
	t.Run("skill-CD-Buff-lifetime-shields", func(t *testing.T) { p6NativeDetailsFromFixture(t, "", true, file, sha) })
	t.Run("Buff-source-values-flags", func(t *testing.T) { p6NativeBuffSourceAndValuesFromFixture(t, "", true, file, sha) })
}
