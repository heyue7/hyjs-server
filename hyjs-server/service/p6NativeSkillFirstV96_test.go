package service

import "testing"

// Every case uses its exact original cold source, input SHA and automatic
// PlayerAI driver. Compare every callback layer without using historical records.
func TestP6NativeSkillFirstV96OriginalAutomaticCoreAndDynamic(t *testing.T) {
	const file = "native-next-real-skills-golden-v96.json"
	const sha = "536a2450041e687573b299b5a893e9239279ff583a03c4ba3e58d778409b5e5d"
	t.Run("core", func(t *testing.T) { p6NativeAutomaticPVPOriginalTickGolden(t, file, sha, 14) })
	t.Run("actor44", func(t *testing.T) { p6NativeActorPropertiesFromFixture(t, "", true, file, sha, 14) })
	t.Run("lifetimes", func(t *testing.T) { p6NativeDetailsFromFixture(t, "", true, file, sha, 14) })
	t.Run("Buffmetadata", func(t *testing.T) { p6NativeBuffSourceAndValuesFromFixture(t, "", true, file, sha, 14) })
	t.Run("manager", func(t *testing.T) {
		p6NativeOriginalManagerFixture(t, "native-next-real-skills-manager-golden-v96.json", "ebe651d6cb8b5d6bfd7e4e6653a88c4c12b2617dee2800cc24fd6a9987a7d248", 1, 14)
	})
}

func TestP6NativeSkillFirstV96OriginalAutomaticExports(t *testing.T) {
	const file = "native-next-real-skills-hit-golden-v96.json"
	const sha = "bb25274c7797fb6d288cc17332503694482803feace5238bd6deef6b02347cd7"
	t.Run("HP-AP-combo-control", func(t *testing.T) { p6OriginalPrimaryHPFixture(t, file, sha, 1, 2, 3, 4, 5, 6) })
	t.Run("cast-values-order", func(t *testing.T) { p6OriginalOrderedCastFixture(t, file, sha) })
	t.Run("cast-complete-collection", func(t *testing.T) { p6OriginalCastCollectionFixture(t, file, sha) })
	t.Run("cast-loads-dead", func(t *testing.T) { p6OriginalCastLifecycleFixture(t, file, sha) })
	t.Run("Buff-HP-AP", func(t *testing.T) {
		p6OriginalBuffHPFixture(t, file, sha, 1)
		p6OriginalBuffHPFixture(t, file, sha, 14)
	})
	t.Run("Buff-internal-order", func(t *testing.T) { p6OriginalOrderedBuffFixture(t, file, sha) })
	t.Run("standalone-full-collection", func(t *testing.T) { p6OriginalStandaloneMixedCollectionFixture(t, file, sha) })
}

func TestP6NativeSkillFirstV96OriginalAutomaticFinalProtocolBytes(t *testing.T) {
	p6NativeOriginalProtocolRecordFixture(t, "native-next-real-skills-original-records-v96.json", "e88b994928110b91ff62cd5a4a53ecfcc8572abbd7d2589028646dc5c842b356", false, 14)
}
