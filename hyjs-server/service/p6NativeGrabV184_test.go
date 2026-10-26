package service

import (
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"path/filepath"
	"testing"
)

// Independent original SCGrab.SetData, both original PlayerAI and full source.
// Each complete core/export/terminal comparison retains exact native clocks.
// SHA markers must be replaced from actual collector output before validation.
func TestP6NativeGrabV184OriginalAutomaticCoreAndDynamic(t *testing.T) {
	const file = "native-original-grab19-golden-v184.json"
	const sha = "4b4345119ce7c3114998110c95f0ffe18054e2d0d0f53599e038b25c0f5feb60"
	t.Run("core", func(t *testing.T) { p6NativeAutomaticPVPOriginalTickGolden(t, file, sha, 7) })
	t.Run("actor44", func(t *testing.T) { p6NativeActorPropertiesFromFixture(t, "", true, file, sha, 7) })
	t.Run("lifetimes", func(t *testing.T) { p6NativeDetailsFromFixture(t, "", true, file, sha, 7) })
	t.Run("Buffmetadata", func(t *testing.T) { p6NativeBuffSourceAndValuesFromFixture(t, "", true, file, sha, 7) })
	t.Run("manager", func(t *testing.T) {
		p6NativeOriginalManagerFixture(t, "native-original-grab19-manager-golden-v184.json", "719dba0154243fa519e98299585a496f0c14408a7e4e20e507f15cfc0d7e62fd", 1, 7)
	})
}

func TestP6NativeGrabV184OriginalAutomaticExports(t *testing.T) {
	const file = "native-original-grab19-hit-golden-v184.json"
	const sha = "05f3a698663f1b611ac1193fc99b8c14175cea06d16b17ba697ad0c881a4ed48"
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

func TestP6NativeGrabV184OriginalAutomaticFinalProtocolBytes(t *testing.T) {
	p6NativeOriginalProtocolRecordFixture(t, "native-original-grab19-original-records-v184.json", "d55d03562704f3e7e6108f91982ac11f98790937ab21282b89b4428d7a567f70", false, 7)
}

// The binder verifies exact typed SCGrab input and source bytes in tests only.
// Public calculation scope never uses an input, seed or GUID whitelist.
func p6NativeGrabFixtureInput(t *testing.T, s *GameService, input *model.FrozenBattleInput, wireSHA, fixtureSHA string) (*model.FrozenBattleInput, bool) {
	t.Helper()
	switch fixtureSHA {
	case "4b4345119ce7c3114998110c95f0ffe18054e2d0d0f53599e038b25c0f5feb60", "05f3a698663f1b611ac1193fc99b8c14175cea06d16b17ba697ad0c881a4ed48", "719dba0154243fa519e98299585a496f0c14408a7e4e20e507f15cfc0d7e62fd", "d55d03562704f3e7e6108f91982ac11f98790937ab21282b89b4428d7a567f70":
	default:
		return input, false
	}
	const source = "../../analysis/p6/original-grab19-candidate-inputs-v183/manifest.json"
	raw, err := readTestAnalysis75(source)
	if err != nil || p6Digest(raw) != "c5cd717a47aaa6ed874a148bb710ca351d98b89ca72b7c1b70dbb4d22197757c" {
		t.Fatal("original Grab19 source manifest SHA", err)
	}
	var manifest struct {
		Cases []struct {
			Wire     string `json:"wire"`
			WireSHA  string `json:"wireSHA256"`
			Typed    string `json:"typedCallback"`
			TypedSHA string `json:"typedCallbackSHA256"`
		} `json:"cases"`
	}
	if json.Unmarshal(raw, &manifest) != nil || len(manifest.Cases) != 7 {
		t.Fatal("original Grab19 source denominator")
	}
	for _, row := range manifest.Cases {
		if row.WireSHA != wireSHA {
			continue
		}
		encoded, err := readTestAnalysis75(filepath.Join(filepath.Dir(source), row.Typed))
		if err != nil || p6Digest(encoded) != row.TypedSHA {
			t.Fatal("original Grab19 typed SHA", err)
		}
		typed := &pb.SCGrab{}
		if proto.Unmarshal(encoded, typed) != nil {
			t.Fatal("original SCGrab typed source")
		}
		frozen, err := s.FreezeP6GrabInput(typed)
		if err != nil {
			t.Fatal(err)
		}
		if !proto.Equal(input.Attacker, frozen.Attacker) || !proto.Equal(input.Defender, frozen.Defender) || input.Seed != frozen.Seed || input.MaxRound != frozen.MaxRound || input.BattleType != 19 || input.IsAttToDef || input.DefenderFirst != frozen.DefenderFirst {
			t.Fatal("SCGrab typed projection drift")
		}
		return frozen, true
	}
	t.Fatal("original Grab19 wire SHA not in pinned source")
	return nil, false
}
