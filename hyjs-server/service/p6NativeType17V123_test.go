package service

import "testing"

// Every case uses the actual two-realm qualified-source typed17 draft, input SHA and automatic
// PlayerAI driver. Compare every callback layer without using historical records.
func TestP6NativeType17V123OriginalAutomaticCoreAndDynamic(t *testing.T) {
	const file = "native-local-p14-type17-qualified-golden-v123.json"
	const sha = "271ee2155df8253104fa5321c94d0a31bc8601de10ef1d05ce92114fecb881aa"
	t.Run("core", func(t *testing.T) { p6NativeAutomaticPVPOriginalTickGolden(t, file, sha, 1) })
	t.Run("actor44", func(t *testing.T) { p6NativeActorPropertiesFromFixture(t, "", true, file, sha, 1) })
	t.Run("lifetimes", func(t *testing.T) { p6NativeDetailsFromFixture(t, "", true, file, sha, 1) })
	t.Run("Buffmetadata", func(t *testing.T) { p6NativeBuffSourceAndValuesFromFixture(t, "", true, file, sha, 1) })
	t.Run("manager", func(t *testing.T) {
		p6NativeOriginalManagerFixture(t, "native-local-p14-type17-qualified-manager-golden-v123.json", "46b479f9f07f8278de5578659f279396c040dcbfc05f58dc642010945436c46c", 1, 1)
	})
}

func TestP6NativeType17V123OriginalAutomaticExports(t *testing.T) {
	const file = "native-local-p14-type17-qualified-hit-golden-v123.json"
	const sha = "adbd511d9897e9080b1c12785dc7a9d72ec9555246a08f46a5f200c553d06078"
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

func TestP6NativeType17V123OriginalAutomaticFinalProtocolBytes(t *testing.T) {
	p6NativeOriginalProtocolFixtureWithTerminalHP(t, "native-local-p14-type17-qualified-original-records-v123.json", "dbb535809cd536d2c28858c6a29dff1017a92f68201404163c37e091ab5115f5", false, true, 1)
}
