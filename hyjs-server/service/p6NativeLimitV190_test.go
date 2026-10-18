package service

import (
	"testing"
)

// Independent original typed20 full-party projection retains every actor and skill.
// These engine diagnostics do not assert actual Limit matching or qualification.
// Every callback value/sequence and final protocol field stays strict.
func TestP6NativeLimitV190OriginalAutomaticCoreAndDynamic(t *testing.T) {
	const file = "native-original-full-party-limit20-mirror-golden-v190.json"
	const sha = "b88f64120798a0873125cb8629e94a9b2636afc350a0b12fb3f77d73dde62ff5"
	t.Run("core", func(t *testing.T) { p6NativeAutomaticPVPOriginalTickGolden(t, file, sha, 2) })
	t.Run("actor44", func(t *testing.T) { p6NativeActorPropertiesFromFixture(t, "", true, file, sha, 2) })
	t.Run("lifetimes", func(t *testing.T) { p6NativeDetailsFromFixture(t, "", true, file, sha, 2) })
	t.Run("Buffmetadata", func(t *testing.T) { p6NativeBuffSourceAndValuesFromFixture(t, "", true, file, sha, 2) })
	t.Run("manager", func(t *testing.T) {
		p6NativeOriginalManagerFixture(t, "native-original-full-party-limit20-mirror-manager-golden-v190.json", "aeb0c2e4f02299713ca002b3b78b6f83c1133371515cab6910f46775b5ff3ce9", 1, 2)
	})
}

func TestP6NativeLimitV190OriginalAutomaticExports(t *testing.T) {
	const file = "native-original-full-party-limit20-mirror-hit-golden-v190.json"
	const sha = "55c8904b1708dfe336eee24d70421d7d86cdcb88e2a2caf65f891b9e25b4e085"
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

func TestP6NativeLimitV190OriginalAutomaticFinalProtocolBytes(t *testing.T) {
	p6NativeOriginalProtocolRecordFixture(t, "native-original-full-party-limit20-mirror-original-records-v190.json", "33cb6fc2b787d2f7c9ed4d79af26287dd98bb64acacd70a0c609118981e4170b", false, 2)
}
