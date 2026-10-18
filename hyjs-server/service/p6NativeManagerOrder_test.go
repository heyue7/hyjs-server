package service

import "testing"

func TestP6NativeOriginalManagerOrderCoverageTickGolden(t *testing.T) {
	p6NativeAutomaticPVPOriginalTickGolden(t, "native-next-real-golden-v56.json", "76b02ca8ac602664ddaf2ddb7d9d7e766e64aeeb7d5420969ecd0bf898eb586f", 14)
}

func TestP6NativeOriginalManagerOrderCoverageDynamicGoldens(t *testing.T) {
	const (
		file = "native-next-real-golden-v56.json"
		sha  = "76b02ca8ac602664ddaf2ddb7d9d7e766e64aeeb7d5420969ecd0bf898eb586f"
	)
	t.Run("44-actor-properties-and-counter-mode", func(t *testing.T) { p6NativeActorPropertiesFromFixture(t, "", true, file, sha) })
	t.Run("skill-CD-Buff-lifetime-shields", func(t *testing.T) { p6NativeDetailsFromFixture(t, "", true, file, sha) })
	t.Run("Buff-source-values-flags", func(t *testing.T) { p6NativeBuffSourceAndValuesFromFixture(t, "", true, file, sha) })
}
