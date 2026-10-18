package service

import "testing"

// Every case uses an actual allowed current CreatePlayer main source, input SHA and automatic
// PlayerAI driver. Compare every callback layer without using historical records.
func TestP6NativeMainChoiceV125OriginalAutomaticCoreAndDynamic(t *testing.T) {
	const file = "native-legal-main-choices-friend11-golden-v125.json"
	const sha = "591cb82907815247df773adac5f42f1e660b75d9de9fdef0b897bddf9b2a3840"
	t.Run("core", func(t *testing.T) { p6NativeAutomaticPVPOriginalTickGolden(t, file, sha, 8) })
	t.Run("actor44", func(t *testing.T) { p6NativeActorPropertiesFromFixture(t, "", true, file, sha, 8) })
	t.Run("lifetimes", func(t *testing.T) { p6NativeDetailsFromFixture(t, "", true, file, sha, 8) })
	t.Run("Buffmetadata", func(t *testing.T) { p6NativeBuffSourceAndValuesFromFixture(t, "", true, file, sha, 8) })
	t.Run("manager", func(t *testing.T) {
		p6NativeOriginalManagerFixture(t, "native-legal-main-choices-friend11-manager-golden-v125.json", "d5f6739d91a879116c8cb2e93c23d62b57d8632df4dafe3bca10fd1d5b43f6f3", 1, 8)
	})
}

func TestP6NativeMainChoiceV125OriginalAutomaticExports(t *testing.T) {
	const file = "native-legal-main-choices-friend11-hit-golden-v125.json"
	const sha = "6fdc7cc31bf5113e999167c8013cd21f87417fcdd37f5dc87e9c0fdeaae0d2e7"
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

func TestP6NativeMainChoiceV125OriginalAutomaticFinalProtocolBytes(t *testing.T) {
	p6NativeOriginalProtocolFixtureWithTerminalHP(t, "native-legal-main-choices-friend11-original-records-v125.json", "7c00dfdc14b6cbbf68c762a6343891b468c5f9af72f47c42edb249359671a0ed", false, true, 8)
}
