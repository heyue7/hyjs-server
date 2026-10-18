package service

import "testing"

// Actual original UF14 BattleManager selects PlayerAI on both camps. This
// exact local guard source is separate from the preserved v85 CopyAI diagnostic.
func TestP6NativeLocalUF14GuardModeOriginalCoreAndDynamic(t *testing.T) {
	const file = "native-local-uf14-guard-mode-golden-v94.json"
	const sha = "8ac6333351b65dda15e3d7c6e03b76a1939173caf8a3103d5f88f2fda7630a9e"
	t.Run("core", func(t *testing.T) { p6NativeAutomaticPVPOriginalTickGolden(t, file, sha, 1) })
	t.Run("actor44", func(t *testing.T) { p6NativeActorPropertiesFromFixture(t, "", true, file, sha, 1) })
	t.Run("lifetimes", func(t *testing.T) { p6NativeDetailsFromFixture(t, "", true, file, sha, 1) })
	t.Run("Buffmetadata", func(t *testing.T) { p6NativeBuffSourceAndValuesFromFixture(t, "", true, file, sha, 1) })
	t.Run("manager", func(t *testing.T) {
		p6NativeOriginalManagerFixture(t, "native-local-uf14-guard-mode-manager-golden-v94.json", "6abc1c117379722ae740e21c944ebc2e2fccb63da3a0493a60da4c92666e7839", 1, 1)
	})
}

func TestP6NativeLocalUF14GuardModeOriginalExports(t *testing.T) {
	const file = "native-local-uf14-guard-mode-hit-golden-v94.json"
	const sha = "3ca1e2d2d9a7708efd0a4219c1593cc5bff532c72319b37aa981b0f7532986dd"
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

func TestP6NativeLocalUF14GuardModeOriginalFinalProtocol(t *testing.T) {
	p6NativeOriginalProtocolRecordFixture(t, "native-local-uf14-guard-mode-original-records-v94.json", "a56339fe25b8ea0324c376009f25fe3b1af8dded51363f59f8781459a364a330", false, 1)
}
