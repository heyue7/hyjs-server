package service

import "testing"

func TestP6NativeOriginalStandaloneBuffCoverageTickGolden(t *testing.T) {
	p6NativeAutomaticPVPOriginalTickGolden(t, "native-next-real-golden-v59.json", "91b155a2bdb6fae3fbcfa447920f1b9851746aee76115048a55f00680884fc7f", 14)
}
func TestP6NativeOriginalStandaloneBuffCoverageDynamicGoldens(t *testing.T) {
	const (
		file = "native-next-real-golden-v59.json"
		sha  = "91b155a2bdb6fae3fbcfa447920f1b9851746aee76115048a55f00680884fc7f"
	)
	t.Run("44-actor-properties-and-counter-mode", func(t *testing.T) { p6NativeActorPropertiesFromFixture(t, "", true, file, sha) })
	t.Run("skill-CD-Buff-lifetime-shields", func(t *testing.T) { p6NativeDetailsFromFixture(t, "", true, file, sha) })
	t.Run("Buff-source-values-flags", func(t *testing.T) { p6NativeBuffSourceAndValuesFromFixture(t, "", true, file, sha) })
}
