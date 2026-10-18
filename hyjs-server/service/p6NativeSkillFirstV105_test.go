package service

import (
	"os"
	"testing"
)

// Every case uses its exact original cold source, input SHA and automatic
// PlayerAI driver. Compare every callback layer without using historical records.
func TestP6NativeSkillFirstV105OriginalAutomaticCoreAndDynamic(t *testing.T) {
	const file = "native-next-real-skills-golden-v105.json"
	const sha = "1107882fa87fee797c228a4581d1ea6f1eaf46576b206061662b224062b21802"
	t.Run("core", func(t *testing.T) { p6NativeAutomaticPVPOriginalTickGolden(t, file, sha, 14) })
	t.Run("actor44", func(t *testing.T) { p6NativeActorPropertiesFromFixture(t, "", true, file, sha, 14) })
	t.Run("lifetimes", func(t *testing.T) { p6NativeDetailsFromFixture(t, "", true, file, sha, 14) })
	t.Run("Buffmetadata", func(t *testing.T) { p6NativeBuffSourceAndValuesFromFixture(t, "", true, file, sha, 14) })
	t.Run("manager", func(t *testing.T) {
		p6NativeOriginalManagerFixture(t, "native-next-real-skills-manager-golden-v105.json", "ac956f4cfcea05073258d064b9560668037e2207dd817a48db3c6e6146e06906", 1, 14)
	})
}

func TestP6NativeSkillFirstV105OriginalAutomaticExports(t *testing.T) {
	const file = "native-next-real-skills-hit-golden-v105.json"
	const sha = "10e10fbc3519ef436e0fa5b37e28a70674e0149a039b3f392ee6840c69d387fa"
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

func TestP6NativeSkillFirstV105OriginalAutomaticFinalProtocolBytes(t *testing.T) {
	p6NativeOriginalProtocolFixtureWithTerminalHP(t, "native-next-real-skills-original-records-v105.json", "5c3ec58fed3db6bd13ab28ab1b05daf63e0385ded5c3a2ea86135c96fd342132", false, true, 14)
}

// Keep the former opt-in name for reproducing historical runs. The ordinary
// final-protocol test now always checks the same complete terminal HP fields.
func TestP6NativeSkillFirstV105PendingStrictTerminalHP(t *testing.T) {
	if os.Getenv("HYJS_P6_V105_PENDING_TERMINAL_HP") != "1" {
		t.Skip("historical opt-in alias; the ordinary final-protocol test now includes strict terminal HP")
	}
	p6NativeOriginalProtocolRecordFixture(t, "native-next-real-skills-original-records-v105.json", "5c3ec58fed3db6bd13ab28ab1b05daf63e0385ded5c3a2ea86135c96fd342132", false, 14)
}
