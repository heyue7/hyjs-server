package service

import "testing"

func TestP6NativeOriginalCallbackClockCoverageTickGolden(t *testing.T) {
	p6NativeAutomaticPVPOriginalTickGolden(t, "native-next-real-golden-v60.json", "aba778143e9b90a553bfa2c7c6d27b69ac8889a35ee213ff6510c432e1e2ae8a", 14)
}
func TestP6NativeOriginalCallbackClockCoverageDynamicGoldens(t *testing.T) {
	const (
		file = "native-next-real-golden-v60.json"
		sha  = "aba778143e9b90a553bfa2c7c6d27b69ac8889a35ee213ff6510c432e1e2ae8a"
	)
	t.Run("44-actor-properties-and-counter-mode", func(t *testing.T) { p6NativeActorPropertiesFromFixture(t, "", true, file, sha) })
	t.Run("skill-CD-Buff-lifetime-shields", func(t *testing.T) { p6NativeDetailsFromFixture(t, "", true, file, sha) })
	t.Run("Buff-source-values-flags", func(t *testing.T) { p6NativeBuffSourceAndValuesFromFixture(t, "", true, file, sha) })
}

func TestP6NativeOriginalExpandedManagerIdentitiesGolden(t *testing.T) {
	p6NativeOriginalManagerFixture(t, "native-manager-flags-golden-v60.json", "d8b9d03443d7ef59b296ff9225cbbf8edf7f10093d043ca58c2c3a73fb91d5f4", 8)
}
