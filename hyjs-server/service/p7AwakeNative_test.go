package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

type p7AwakeNativeBinding struct {
	Path string `json:"path"`
	SHA  string `json:"sha256"`
}

type p7AwakeNativeBundle struct {
	Schema     string                          `json:"schema"`
	Source     string                          `json:"sourceManifest"`
	SourceSHA  string                          `json:"sourceManifestSHA256"`
	Capture    string                          `json:"captureManifest"`
	CaptureSHA string                          `json:"captureManifestSHA256"`
	Count      int                             `json:"caseCount"`
	Scope      string                          `json:"nativeScope"`
	Authority  bool                            `json:"businessAuthority"`
	Business   bool                            `json:"actualBusinessStart"`
	Kind       string                          `json:"sourceKind"`
	Fixtures   map[string]p7AwakeNativeBinding `json:"fixtures"`
}

type p7AwakeNativeCase struct {
	Name     string `json:"case"`
	Wire     string `json:"wire"`
	WireSHA  string `json:"wireSHA256"`
	Typed    string `json:"typedCallback"`
	TypedSHA string `json:"typedCallbackSHA256"`
	FightID  string `json:"fightId"`
	Seed     int64  `json:"seed"`
	Type     int32  `json:"battleType"`
}

func p7AwakeNativePath(t *testing.T, relative string) string {
	t.Helper()
	clean := filepath.Clean(relative)
	if filepath.IsAbs(clean) || (!strings.HasPrefix(filepath.ToSlash(clean), "analysis/p6/") && !strings.HasPrefix(filepath.ToSlash(clean), "analysis/p7/awake-playable-20261016/")) {
		t.Fatal("P7 Awake diagnostic evidence must remain under analysis/p6", relative)
	}
	return filepath.Join("../..", clean)
}

func p7AwakeNativeRead(t *testing.T, name, expected string) []byte {
	t.Helper()
	raw, err := readTestAnalysis75(p7AwakeNativePath(t, name))
	if err != nil || len(expected) != 64 || p6Digest(raw) != expected {
		t.Fatal("P7 Awake diagnostic immutable source SHA", name, err)
	}
	return raw
}

func p7AwakeNativeCases(t *testing.T) []p7AwakeNativeCase {
	t.Helper()
	raw := p7AwakeNativeRead(t, "analysis/p7/awake-playable-20261016/native-inputs/manifest.json", "89f13163e4edb4461c13a3a95dcf3347a67e1e9a5091b9fd303cc7bd04635104")
	var source struct {
		Cases     []p7AwakeNativeCase `json:"cases"`
		Compared  bool                `json:"nativeCompared"`
		Authority bool                `json:"authoritative"`
		Pins      map[string]string   `json:"sourcePins"`
	}
	if json.Unmarshal(raw, &source) != nil || len(source.Cases) != 2 || source.Compared || source.Authority || len(source.Pins) != 3 {
		t.Fatal("independent typed16 source identity")
	}
	for path, expected := range source.Pins {
		data, err := readTestAnalysis75(filepath.Join("../..", path))
		if err != nil || len(expected) != 64 || p6Digest(data) != expected {
			t.Fatal("Awake original DLL/actual actor/NPC source SHA", path, err)
		}
	}
	return source.Cases
}

func p7AwakeNativeTypedInput(t *testing.T, row p7AwakeNativeCase) *pb.SCFightPlayback {
	t.Helper()
	base := "analysis/p7/awake-playable-20261016/native-inputs/"
	var wire pb.SCFightPlayback
	var start pb.SCAwakeCopyFightBegin
	if proto.Unmarshal(p7AwakeNativeRead(t, base+row.Wire, row.WireSHA), &wire) != nil || proto.Unmarshal(p7AwakeNativeRead(t, base+row.Typed, row.TypedSHA), &start) != nil {
		t.Fatal("Awake original protobuf inputs")
	}
	if row.Type != 16 || row.Name != row.FightID || wire.GetFightid() != row.FightID || start.GetFightId() != row.FightID || start.RandomSeed == nil || wire.GetRandSeed() != row.Seed || start.GetRandomSeed() != row.Seed || wire.GetBattleType() != 16 || wire.GetMaxRound() != 40 || start.GetMaxRoundNum() != 40 || start.GetRoomId() != "map_forest_003" || wire.Defender1 != nil || wire.OptRecord != nil || len(wire.WinConditions) != 0 || wire.Attacker == nil || wire.Defender2 == nil || len(wire.Attacker.Formation) != 2 || len(wire.Defender2.Formation) != 2 || wire.AttackerFactor == nil || wire.GetAttackerFactor() != 1 || wire.DefenderFactor == nil || wire.GetDefenderFactor() != 1 || !proto.Equal(wire.Attacker, start.BattlePlayer) || !proto.Equal(wire.Defender2, start.BattleMonser) {
		t.Fatal("strict complete typed16 automatic projection")
	}
	for _, fighter := range [][]*pb.BattleHero{wire.Attacker.Formation, wire.Defender2.Formation} {
		for _, actor := range fighter {
			if actor.Inst == nil || actor.Prop == nil {
				t.Fatal("complete original actors required")
			}
		}
	}
	actorSource := &pb.SCRushFightStart{}
	raw, err := readTestAnalysis75("../../analysis/p7/normal-party-phone-20261016/wire/start-CSRushFightStart-1792135594857-SCRushFightStart.pb")
	if err != nil || proto.Unmarshal(raw, actorSource) != nil || !proto.Equal(actorSource.BattlePlayer, wire.Attacker) {
		t.Fatal("actual normal account4 actor differs")
	}
	npcSource := &pb.SCFightPlayback{}
	raw, err = readTestAnalysis75("../../analysis/p6/native-actual-paid-copy-auto-input-v87/000.pb")
	if err != nil || proto.Unmarshal(raw, npcSource) != nil || !proto.Equal(npcSource.Defender2, wire.Defender2) {
		t.Fatal("actual original NPC source differs")
	}
	return &wire
}

func p7AwakeNativeLoadBundle(t *testing.T) *p7AwakeNativeBundle {
	t.Helper()
	name := os.Getenv("HYJS_P7_AWAKE_NATIVE_BUNDLE")
	if name == "" {
		return nil
	}
	raw, err := readTestAnalysis75(name)
	expected := os.Getenv("HYJS_P7_AWAKE_NATIVE_BUNDLE_SHA256")
	if err != nil || len(expected) != 64 || p6Digest(raw) != expected {
		t.Fatal("explicit P7 Awake native bundle SHA", err)
	}
	var bundle p7AwakeNativeBundle
	if json.Unmarshal(raw, &bundle) != nil || bundle.Schema != "p7-awake-native-matrix-v1" || bundle.Source != "analysis/p7/awake-playable-20261016/native-inputs/manifest.json" || bundle.SourceSHA != "89f13163e4edb4461c13a3a95dcf3347a67e1e9a5091b9fd303cc7bd04635104" || bundle.Count != 2 || bundle.Scope != "original-awake16-automatic-ai" || bundle.Authority || bundle.Business || bundle.Kind != "" || len(bundle.Fixtures) != 4 {
		t.Fatal("bounded P7 Awake original typed16 diagnostic bundle identity/denominator/authority")
	}
	p7AwakeNativeRead(t, bundle.Source, bundle.SourceSHA)
	captureRaw := p7AwakeNativeRead(t, bundle.Capture, bundle.CaptureSHA)
	var capture struct {
		Source    string              `json:"sourceManifest"`
		SourceSHA string              `json:"sourceManifestSHA256"`
		Selector  string              `json:"selector"`
		Cases     []p7AwakeNativeCase `json:"cases"`
	}
	if json.Unmarshal(captureRaw, &capture) != nil || capture.SourceSHA != bundle.SourceSHA || capture.Selector != "highest" || len(capture.Cases) != 2 {
		t.Fatal("P7 Awake original capture source/selector/denominator")
	}
	actualSource, err := filepath.Abs(capture.Source)
	expectedSource, pathErr := filepath.Abs(p7AwakeNativePath(t, bundle.Source))
	if err != nil || pathErr != nil || actualSource != expectedSource {
		t.Fatal("P7 Awake capture must bind the exact diagnostic source manifest")
	}
	seen := map[string]bool{}
	for _, row := range capture.Cases {
		for _, original := range p7AwakeNativeCases(t) {
			if row.WireSHA == original.WireSHA && row.TypedSHA == original.TypedSHA && row.FightID == original.FightID && row.Seed == original.Seed && !seen[row.WireSHA] {
				seen[row.WireSHA] = true
			}
		}
	}
	if len(seen) != 2 {
		t.Fatal("P7 Awake capture cannot borrow another typed16 source")
	}
	for _, kind := range []string{"core", "hits", "manager", "records"} {
		fixture := bundle.Fixtures[kind]
		if fixture.Path == "" || len(fixture.SHA) != 64 {
			t.Fatal("all P7 Awake native comparison layers required", kind)
		}
		p7AwakeNativePath(t, filepath.Join("analysis/p6", fixture.Path))
	}
	return &bundle
}

// The count hook admits this exact independent bundle; earlier P7/other fixture
// denominators remain unchanged. Missing or malformed opt-in evidence cannot pass.
func p7AwakeNativeHitCaseCount(name string) int {
	raw, err := readTestAnalysis75(os.Getenv("HYJS_P7_AWAKE_NATIVE_BUNDLE"))
	if err != nil || p6Digest(raw) != os.Getenv("HYJS_P7_AWAKE_NATIVE_BUNDLE_SHA256") {
		return 0
	}
	var bundle p7AwakeNativeBundle
	if json.Unmarshal(raw, &bundle) != nil || bundle.Schema != "p7-awake-native-matrix-v1" || bundle.Count != 2 || bundle.Authority || bundle.Business || bundle.Kind != "" || bundle.Scope != "original-awake16-automatic-ai" || bundle.Source != "analysis/p7/awake-playable-20261016/native-inputs/manifest.json" || bundle.SourceSHA != "89f13163e4edb4461c13a3a95dcf3347a67e1e9a5091b9fd303cc7bd04635104" || len(bundle.Fixtures) != 4 {
		return 0
	}
	hits := bundle.Fixtures["hits"]
	if hits.Path != name || filepath.IsAbs(name) || strings.HasPrefix(filepath.ToSlash(filepath.Clean(name)), "../") {
		return 0
	}
	for _, binding := range []p7AwakeNativeBinding{{bundle.Source, bundle.SourceSHA}, {bundle.Capture, bundle.CaptureSHA}, {filepath.ToSlash(filepath.Join("analysis/p6", name)), hits.SHA}} {
		clean := filepath.ToSlash(filepath.Clean(binding.Path))
		// 唯一新增P7路径是已钉source manifest；capture/fixtures仍限定原analysis/p6。
		if filepath.IsAbs(binding.Path) || (!strings.HasPrefix(clean, "analysis/p6/") && clean != "analysis/p7/awake-playable-20261016/native-inputs/manifest.json") {
			return 0
		}
		data, err := readTestAnalysis75(filepath.Join("../..", binding.Path))
		if err != nil || len(binding.SHA) != 64 || p6Digest(data) != binding.SHA {
			return 0
		}
	}
	return 2
}

func p7AwakeNativeBoundInput(t *testing.T, input *model.FrozenBattleInput, wireSHA, fixtureSHA string) bool {
	t.Helper()
	bundle := p7AwakeNativeLoadBundle(t)
	if bundle == nil {
		return false
	}
	bound := false
	for _, fixture := range bundle.Fixtures {
		if fixture.SHA == fixtureSHA {
			p7AwakeNativeRead(t, filepath.ToSlash(filepath.Join("analysis/p6", fixture.Path)), fixture.SHA)
			bound = true
		}
	}
	if !bound {
		return false
	}
	for _, row := range p7AwakeNativeCases(t) {
		if row.WireSHA != wireSHA {
			continue
		}
		wire := p7AwakeNativeTypedInput(t, row)
		if input == nil || input.BattleType != 16 || input.Monster == nil || input.Defender != nil || !proto.Equal(input.Attacker, wire.Attacker) || !proto.Equal(input.Monster, wire.Defender2) || input.Seed != wire.GetRandSeed() || input.MaxRound != 40 || input.DefenderFirst || input.IsAttToDef || input.KFReplay != nil || input.KFReplayViewer != nil || len(input.WinConditions) != 0 || input.PVECompatibilityRule != "" || input.DiagnosticAIDriverPolicy != "" || input.DiagnosticManualAutoStartRoundTime != nil || input.AttackerDamageFactor == nil || *input.AttackerDamageFactor != 1 || input.DefenderDamageFactor == nil || *input.DefenderDamageFactor != 1 {
			t.Fatal("P7 Awake original typed16 complete frozen projection differs")
		}
		return true
	}
	t.Fatal("P7 Awake fixture attempted another input family")
	return false
}

func TestP7AwakeNativeSourceBinding(t *testing.T) {
	// Verify actual account4 and original NPC provenance before admitting this finite diagnostic source.

	rows := p7AwakeNativeCases(t)
	for index, row := range rows {
		if row.Seed != []int64{7, 314159}[index] {
			t.Fatal("prepublished finite original seeds changed")
		}
		p7AwakeNativeTypedInput(t, row)
	}
}

func TestP7AwakeNativeAllLayers(t *testing.T) {
	bundle := p7AwakeNativeLoadBundle(t)
	if bundle == nil {
		t.Skip("PENDING: independent two-case full-four-actor typed16 native capture and SHA-bound diagnostic bundle")
	}
	TestP7AwakeNativeSourceBinding(t)
	for kind, fixture := range bundle.Fixtures {
		fixtureRaw := p7AwakeNativeRead(t, filepath.ToSlash(filepath.Join("analysis/p6", fixture.Path)), fixture.SHA)
		var tree any
		if json.Unmarshal(fixtureRaw, &tree) != nil {
			t.Fatal("P7 Awake derived fixture JSON", kind)
		}
		allowed := map[string]bool{}
		for _, row := range p7AwakeNativeCases(t) {
			allowed[row.WireSHA] = true
		}
		seenWires := map[string]bool{}
		var walk func(any)
		walk = func(node any) {
			switch value := node.(type) {
			case map[string]any:
				if hash, ok := value["wireSHA256"].(string); ok {
					if !allowed[hash] {
						t.Fatal("P7 Awake comparison layer attempted another wire family", kind)
					}
					seenWires[hash] = true
				}
				for _, child := range value {
					walk(child)
				}
			case []any:
				for _, child := range value {
					walk(child)
				}
			}
		}
		walk(tree)
		if len(seenWires) != 2 {
			t.Fatal("P7 Awake every comparison layer must retain both diagnostic sources", kind)
		}
		if kind == "records" {
			var records struct {
				Cases []struct {
					WireSHA string                     `json:"wireSHA256"`
					Actors  []model.CombatFinalActorHP `json:"originalFinalActorHP"`
				} `json:"cases"`
			}
			raw := p7AwakeNativeRead(t, filepath.ToSlash(filepath.Join("analysis/p6", fixture.Path)), fixture.SHA)
			if json.Unmarshal(raw, &records) != nil || len(records.Cases) != 2 {
				t.Fatal("P7 Awake native complete terminal actor cases required")
			}
			seen := map[string]bool{}
			for _, record := range records.Cases {
				for _, row := range p7AwakeNativeCases(t) {
					if record.WireSHA != row.WireSHA {
						continue
					}
					wire := p7AwakeNativeTypedInput(t, row)
					if seen[record.WireSHA] || len(record.Actors) != 4 {
						t.Fatal("P7 Awake native four-actor terminal denominator")
					}
					seen[record.WireSHA] = true
					for index, actor := range append(append([]*pb.BattleHero{}, wire.Attacker.Formation...), wire.Defender2.Formation...) {
						final := record.Actors[index]
						camp := int32(1)
						if index < 2 {
							camp = 0
						}
						if final.Camp != camp || final.HeroID != actor.GetId() || final.UID == "" {
							t.Fatal("P7 Awake full native terminal actor identity/order")
						}
					}
				}
			}
			if len(seen) != 2 {
				t.Fatal("P7 Awake records cannot borrow another native wire")
			}
		}
	}
	core, hits, manager, records := bundle.Fixtures["core"], bundle.Fixtures["hits"], bundle.Fixtures["manager"], bundle.Fixtures["records"]
	if p6NativeOriginalPerHitCaseCount(hits.Path) != 2 {
		t.Fatal("P7 Awake SHA-bound two-case denominator hook is missing")
	}
	t.Run("core", func(t *testing.T) { p6NativeAutomaticPVPOriginalTickGolden(t, core.Path, core.SHA, 2) })
	t.Run("actor44", func(t *testing.T) { p6NativeActorPropertiesFromFixture(t, "", true, core.Path, core.SHA, 2) })
	t.Run("lifetimes", func(t *testing.T) { p6NativeDetailsFromFixture(t, "", true, core.Path, core.SHA, 2) })
	t.Run("Buffmetadata", func(t *testing.T) { p6NativeBuffSourceAndValuesFromFixture(t, "", true, core.Path, core.SHA, 2) })
	t.Run("manager", func(t *testing.T) { p6NativeOriginalManagerFixture(t, manager.Path, manager.SHA, 1, 2) })
	t.Run("HP-AP-combo-control", func(t *testing.T) { p6OriginalPrimaryHPFixture(t, hits.Path, hits.SHA, 1, 2, 3, 4, 5, 6) })
	t.Run("cast-values-order", func(t *testing.T) { p6OriginalOrderedCastFixture(t, hits.Path, hits.SHA) })
	t.Run("cast-complete-collection", func(t *testing.T) { p6OriginalCastCollectionFixture(t, hits.Path, hits.SHA) })
	t.Run("cast-loads-dead", func(t *testing.T) { p6OriginalCastLifecycleFixture(t, hits.Path, hits.SHA) })
	t.Run("Buff-HP-AP", func(t *testing.T) {
		p6OriginalBuffHPFixture(t, hits.Path, hits.SHA, 1)
		p6OriginalBuffHPFixture(t, hits.Path, hits.SHA, 2)
	})
	t.Run("Buff-internal-order", func(t *testing.T) { p6OriginalOrderedBuffFixture(t, hits.Path, hits.SHA) })
	t.Run("standalone-full-collection", func(t *testing.T) { p6OriginalStandaloneMixedCollectionFixture(t, hits.Path, hits.SHA) })
	// 短样本允许敌方尚未出手；完整实际操作与全部终末HP仍逐项严格比较。
	t.Run("final-protocol-bytes-terminal-HP", func(t *testing.T) { p6NativeOriginalProtocolRecordFixture(t, records.Path, records.SHA, false, 2) })
}
