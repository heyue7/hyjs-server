package service

import (
	"google.golang.org/protobuf/proto"

	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"testing"
)

// One exact actual original manual-control source preserves every native value/clock.
// The original manual wait and live Auto control are replayed as real ticks.
// Every callback value/sequence and final protocol field stays strict.
func TestP6NativeKFRealmV175OriginalAutomaticCoreAndDynamic(t *testing.T) {
	const file = "native-local-kf-joint-realm-golden-v175.json"
	const sha = "ac77919ea1b57c59b72fb077c48ce3ea9016d15af5bd4b9b3bb315ab59fea592"
	t.Run("core", func(t *testing.T) { p6NativeAutomaticPVPOriginalTickGolden(t, file, sha, 2) })
	t.Run("actor44", func(t *testing.T) { p6NativeActorPropertiesFromFixture(t, "", true, file, sha, 2) })
	t.Run("lifetimes", func(t *testing.T) { p6NativeDetailsFromFixture(t, "", true, file, sha, 2) })
	t.Run("Buffmetadata", func(t *testing.T) { p6NativeBuffSourceAndValuesFromFixture(t, "", true, file, sha, 2) })
	t.Run("manager", func(t *testing.T) {
		p6NativeOriginalManagerFixture(t, "native-local-kf-joint-realm-manager-golden-v175.json", "3802906f430e1e463994c963f8937471c8e2fe3039c68dc1002e9670373d8f71", 1, 2)
	})
}

func TestP6NativeKFRealmV175OriginalAutomaticExports(t *testing.T) {
	const file = "native-local-kf-joint-realm-hit-golden-v175.json"
	const sha = "46d4e43fd84923e88df8ff9c085ff497be2fe0241780252cebd03335b1dcf94a"
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

func TestP6NativeKFRealmV175OriginalAutomaticFinalProtocolBytes(t *testing.T) {
	p6NativeOriginalProtocolRecordFixture(t, "native-local-kf-joint-realm-original-records-v175.json", "b704c543745bfb80982f79495a79804458c810945a55e82fec98a02a1c5c8843", false, 2)
}

// The exact fixture driver carries the independently named local KF wrapper;
// ordinary17 and legacy numeric-PID probes never use this diagnostic projection.
func p6NativeKFRealmFixtureInput(t *testing.T, s *GameService, input *model.FrozenBattleInput, wireSHA, fixtureSHA string) (*model.FrozenBattleInput, bool) {
	t.Helper()
	switch fixtureSHA {
	case "ac77919ea1b57c59b72fb077c48ce3ea9016d15af5bd4b9b3bb315ab59fea592", "46d4e43fd84923e88df8ff9c085ff497be2fe0241780252cebd03335b1dcf94a", "3802906f430e1e463994c963f8937471c8e2fe3039c68dc1002e9670373d8f71", "b704c543745bfb80982f79495a79804458c810945a55e82fec98a02a1c5c8843":
	default:
		return input, false
	}
	index := -1
	switch wireSHA {
	case "4723f99584e071bba40883f1ef7cecd2ae4cc2c2bdae0e11693d4dc9ee9e3e4f":
		index = 0
	case "3f5c426911f63c051d40620a5788b7ce872ab2aa8a415097cf9e25db0de4eb67":
		index = 1
	default:
		t.Fatal("KF original wire source identity changed")
	}
	raw, err := readTestAnalysis75("../../analysis/p14/final-game-actual-native/actual-Game1-KFReplayData.pb")
	if err != nil || p6Digest(raw) != "18473d1c95d92d51850578f69a8fa6273039a6f77d55a0ebd6f64abf699c502c" {
		t.Fatal("actual KF source SHA", err)
	}
	source := &pb.KFReplayData{}
	if proto.Unmarshal(raw, source) != nil {
		t.Fatal("actual KF typed source")
	}
	viewer := source.PlayerList[index]
	projected, err := s.FreezeP6KFReplayRealmInput(source, int64(viewer.GetServerId()), viewer.GetPlayerId())
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(input.Attacker, projected.Attacker) || !proto.Equal(input.Defender, projected.Defender) || input.Seed != projected.Seed {
		t.Fatal("KF source transport projection mismatch")
	}
	return projected, true
}
