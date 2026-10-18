package service

import (
	"testing"
)

// Independent original typed20 full-party projection retains every actor and skill.
// These engine diagnostics do not assert actual Limit matching or qualification.
// Every callback value/sequence and final protocol field stays strict.
func TestP6NativeLimitV188OriginalAutomaticCoreAndDynamic(t *testing.T) {
	const file = "native-original-full-party-limit20-golden-v188.json"
	const sha = "5d8026eaccd6eeaa81694b3a92c9fbb504aaa1e7ca08f96920f9126e7b8a919d"
	t.Run("core", func(t *testing.T) { p6NativeAutomaticPVPOriginalTickGolden(t, file, sha, 2) })
	t.Run("actor44", func(t *testing.T) { p6NativeActorPropertiesFromFixture(t, "", true, file, sha, 2) })
	t.Run("lifetimes", func(t *testing.T) { p6NativeDetailsFromFixture(t, "", true, file, sha, 2) })
	t.Run("Buffmetadata", func(t *testing.T) { p6NativeBuffSourceAndValuesFromFixture(t, "", true, file, sha, 2) })
	t.Run("manager", func(t *testing.T) {
		p6NativeOriginalManagerFixture(t, "native-original-full-party-limit20-manager-golden-v188.json", "676768f16d6a0be31e1aac6edd3b7b6b64eab803f0b1522d70be2d66db056e2d", 1, 2)
	})
}

func TestP6NativeLimitV188OriginalAutomaticExports(t *testing.T) {
	const file = "native-original-full-party-limit20-hit-golden-v188.json"
	const sha = "2e08ad35095a49eb4dd69dc8f324aab9d7d6793eef814c2d457aea66e643f021"
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

func TestP6NativeLimitV188OriginalAutomaticFinalProtocolBytes(t *testing.T) {
	p6NativeOriginalProtocolRecordFixture(t, "native-original-full-party-limit20-original-records-v188.json", "844516367d45fa688ae110ec319992f55298b9e29be43989ced3afc94662629e", false, 2)
}
