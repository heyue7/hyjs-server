package service

import (
	"testing"
)

// Independent original SCUFFightRecordReplay.SetData, both original PlayerAI and full source.
// Each complete core/export/terminal comparison retains exact native clocks.
// Fixed hashes bind actual collector output and unmodified original values.
func TestP6NativeUFPlayerV192OriginalAutomaticCoreAndDynamic(t *testing.T) {
	const file = "native-original-uf14-player-factor1-golden-v192.json"
	const sha = "a23143a203971311f5012c32b85490c7c89e5e3b2a536401812f93d3ace79984"
	t.Run("core", func(t *testing.T) { p6NativeAutomaticPVPOriginalTickGolden(t, file, sha, 7) })
	t.Run("actor44", func(t *testing.T) { p6NativeActorPropertiesFromFixture(t, "", true, file, sha, 7) })
	t.Run("lifetimes", func(t *testing.T) { p6NativeDetailsFromFixture(t, "", true, file, sha, 7) })
	t.Run("Buffmetadata", func(t *testing.T) { p6NativeBuffSourceAndValuesFromFixture(t, "", true, file, sha, 7) })
	t.Run("manager", func(t *testing.T) {
		p6NativeOriginalManagerFixture(t, "native-original-uf14-player-factor1-manager-golden-v192.json", "da089de05c03b759a3235344e91ae3dc46f3bb11d8c920fc116c45cbf736f19a", 1, 7)
	})
}

func TestP6NativeUFPlayerV192OriginalAutomaticExports(t *testing.T) {
	const file = "native-original-uf14-player-factor1-hit-golden-v192.json"
	const sha = "a7096469214d686dc0c67620c0efa6f627fc0e6693c7b8aa42ebf5d787aae597"
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

func TestP6NativeUFPlayerV192OriginalAutomaticFinalProtocolBytes(t *testing.T) {
	p6NativeOriginalProtocolRecordFixture(t, "native-original-uf14-player-factor1-original-records-v192.json", "1b346084e9d0dddcac8f99b414fbdc77a8524397948c8556fd9f2c5daecd5ac1", false, 7)
}
