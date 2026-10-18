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

type p12FieldPVPNativeBinding struct {
	Path string `json:"path"`
	SHA  string `json:"sha256"`
}

type p12FieldPVPNativeBundle struct {
	Schema     string                              `json:"schema"`
	Source     string                              `json:"sourceManifest"`
	SourceSHA  string                              `json:"sourceManifestSHA256"`
	Capture    string                              `json:"captureManifest"`
	CaptureSHA string                              `json:"captureManifestSHA256"`
	Count      int                                 `json:"caseCount"`
	Scope      string                              `json:"nativeScope"`
	Authority  bool                                `json:"businessAuthority"`
	Business   bool                                `json:"actualBusinessStart"`
	Kind       string                              `json:"sourceKind"`
	Fixtures   map[string]p12FieldPVPNativeBinding `json:"fixtures"`
}

type p12FieldPVPNativeCase struct {
	Name     string `json:"case"`
	Wire     string `json:"wire"`
	WireSHA  string `json:"wireSHA256"`
	Typed    string `json:"typedCallback"`
	TypedSHA string `json:"typedCallbackSHA256"`
	FightID  string `json:"fightId"`
	Seed     int64  `json:"seed"`
	Type     int32  `json:"battleType"`
}

func p12FieldPVPNativePath(t *testing.T, relative string) string {
	t.Helper()
	clean := filepath.Clean(relative)
	if filepath.IsAbs(clean) || !strings.HasPrefix(filepath.ToSlash(clean), "analysis/p6/") {
		t.Fatal("P12 Field PK diagnostic evidence must remain under analysis/p6", relative)
	}
	return filepath.Join("../..", clean)
}

func p12FieldPVPNativeRead(t *testing.T, name, expected string) []byte {
	t.Helper()
	raw, err := os.ReadFile(p12FieldPVPNativePath(t, name))
	if err != nil || len(expected) != 64 || p6Digest(raw) != expected {
		t.Fatal("P12 Field PK diagnostic immutable source SHA", name, err)
	}
	return raw
}

func p12FieldPVPNativeCases(t *testing.T) []p12FieldPVPNativeCase {
	t.Helper()
	raw := p12FieldPVPNativeRead(t, "analysis/p6/field-pvp5-actual-actors-pending-20261016/manifest.json", "d705ccbf9ea8c1b226a229becbc5bfbe67117e1a4b7ff5da2a21d95c75cc8ece")
	var source struct {
		Cases     []p12FieldPVPNativeCase `json:"cases"`
		Compared  bool                    `json:"nativeCompared"`
		Authority bool                    `json:"authoritative"`
		Pins      map[string]string       `json:"sourcePins"`
	}
	if json.Unmarshal(raw, &source) != nil || len(source.Cases) != 4 || source.Compared || source.Authority || len(source.Pins) != 5 {
		t.Fatal("independent typed5 source identity")
	}
	for path, expected := range source.Pins {
		data, err := os.ReadFile(filepath.Join("../..", path))
		if err != nil || len(expected) != 64 || p6Digest(data) != expected {
			t.Fatal("Field PK original DLL/actual actor/NPC source SHA", path, err)
		}
	}
	return source.Cases
}

func p12FieldPVPNativeTypedInput(t *testing.T, row p12FieldPVPNativeCase) *pb.SCFightPlayback {
	t.Helper()
	base := "analysis/p6/field-pvp5-actual-actors-pending-20261016/"
	var wire pb.SCFightPlayback
	var start pb.SidePvpFightData
	if proto.Unmarshal(p12FieldPVPNativeRead(t, base+row.Wire, row.WireSHA), &wire) != nil || proto.Unmarshal(p12FieldPVPNativeRead(t, base+row.Typed, row.TypedSHA), &start) != nil {
		t.Fatal("Field PK original protobuf inputs")
	}
	if row.Type != 5 || wire.GetFightid() != row.FightID || start.GetId() != row.FightID || start.RandSeed == nil || wire.GetRandSeed() != row.Seed || start.GetRandSeed() != row.Seed || wire.GetBattleType() != 5 || wire.GetMaxRound() != 40 || start.GetMaxRoundCount() != 40 || wire.Defender2 != nil || wire.OptRecord != nil || len(wire.WinConditions) != 0 || wire.Attacker == nil || wire.Defender1 == nil || len(wire.Attacker.Formation) != 1 || len(wire.Defender1.Formation) != 1 || wire.AttackerFactor == nil || wire.GetAttackerFactor() != 1 || wire.DefenderFactor == nil || wire.GetDefenderFactor() != 1 || !proto.Equal(wire.Attacker, start.AttPlayer) || !proto.Equal(wire.Defender1, start.DefPlayer) || start.WinPlayerId != nil || len(start.AttGet) != 0 || len(start.DefLose) != 0 || start.AttFirstPlayerId != nil {
		t.Fatal("strict complete original type5 two-player automatic projection without result hints")
	}
	for _, player := range []*pb.BattlePlayer{wire.Attacker, wire.Defender1} {
		if player.Inst == nil {
			t.Fatal("original player instance required")
		}
		for _, actor := range player.Formation {
			if actor.Inst == nil || actor.Prop == nil {
				t.Fatal("complete original actors required")
			}
		}
	}
	return &wire
}

func p12FieldPVPNativeLoadBundle(t *testing.T) *p12FieldPVPNativeBundle {
	t.Helper()
	name := os.Getenv("HYJS_P12_FIELD_PVP_NATIVE_BUNDLE")
	if name == "" {
		return nil
	}
	raw, err := os.ReadFile(name)
	expected := os.Getenv("HYJS_P12_FIELD_PVP_NATIVE_BUNDLE_SHA256")
	if err != nil || len(expected) != 64 || p6Digest(raw) != expected {
		t.Fatal("explicit P12 Field PK native bundle SHA", err)
	}
	var bundle p12FieldPVPNativeBundle
	if json.Unmarshal(raw, &bundle) != nil || bundle.Schema != "p12-field-pvp5-native-matrix-v1" || bundle.Source != "analysis/p6/field-pvp5-actual-actors-pending-20261016/manifest.json" || bundle.SourceSHA != "d705ccbf9ea8c1b226a229becbc5bfbe67117e1a4b7ff5da2a21d95c75cc8ece" || bundle.Count != 4 || bundle.Scope != "original-field-pvp5-field002-automatic-ai" || bundle.Authority || bundle.Business || bundle.Kind != "" || len(bundle.Fixtures) != 4 {
		t.Fatal("bounded P12 Field PK original typed5 diagnostic bundle identity/denominator/authority")
	}
	p12FieldPVPNativeRead(t, bundle.Source, bundle.SourceSHA)
	captureRaw := p12FieldPVPNativeRead(t, bundle.Capture, bundle.CaptureSHA)
	var capture struct {
		Source    string                  `json:"sourceManifest"`
		SourceSHA string                  `json:"sourceManifestSHA256"`
		Selector  string                  `json:"selector"`
		Cases     []p12FieldPVPNativeCase `json:"cases"`
	}
	if json.Unmarshal(captureRaw, &capture) != nil || capture.SourceSHA != bundle.SourceSHA || capture.Selector != "highest" || len(capture.Cases) != 4 {
		t.Fatal("P12 Field PK original capture source/selector/denominator")
	}
	actualSource, err := filepath.Abs(capture.Source)
	expectedSource, pathErr := filepath.Abs(p12FieldPVPNativePath(t, bundle.Source))
	if err != nil || pathErr != nil || actualSource != expectedSource {
		t.Fatal("P12 Field PK capture must bind the exact diagnostic source manifest")
	}
	seen := map[string]bool{}
	for _, row := range capture.Cases {
		for _, original := range p12FieldPVPNativeCases(t) {
			if row.WireSHA == original.WireSHA && row.TypedSHA == original.TypedSHA && row.FightID == original.FightID && row.Seed == original.Seed && !seen[row.WireSHA] {
				seen[row.WireSHA] = true
			}
		}
	}
	if len(seen) != 4 {
		t.Fatal("P12 Field PK capture cannot borrow another typed5 source")
	}
	for _, kind := range []string{"core", "hits", "manager", "records"} {
		fixture := bundle.Fixtures[kind]
		if fixture.Path == "" || len(fixture.SHA) != 64 {
			t.Fatal("all P12 Field PK native comparison layers required", kind)
		}
		p12FieldPVPNativePath(t, filepath.Join("analysis/p6", fixture.Path))
	}
	return &bundle
}

// The count hook admits this exact independent bundle; earlier P7/other fixture
// denominators remain unchanged. Missing or malformed opt-in evidence cannot pass.
func p12FieldPVPNativeHitCaseCount(name string) int {
	raw, err := os.ReadFile(os.Getenv("HYJS_P12_FIELD_PVP_NATIVE_BUNDLE"))
	if err != nil || p6Digest(raw) != os.Getenv("HYJS_P12_FIELD_PVP_NATIVE_BUNDLE_SHA256") {
		return 0
	}
	var bundle p12FieldPVPNativeBundle
	if json.Unmarshal(raw, &bundle) != nil || bundle.Schema != "p12-field-pvp5-native-matrix-v1" || bundle.Count != 4 || bundle.Authority || bundle.Business || bundle.Kind != "" || bundle.Scope != "original-field-pvp5-field002-automatic-ai" || bundle.Source != "analysis/p6/field-pvp5-actual-actors-pending-20261016/manifest.json" || bundle.SourceSHA != "d705ccbf9ea8c1b226a229becbc5bfbe67117e1a4b7ff5da2a21d95c75cc8ece" || len(bundle.Fixtures) != 4 {
		return 0
	}
	hits := bundle.Fixtures["hits"]
	if hits.Path != name || filepath.IsAbs(name) || strings.HasPrefix(filepath.ToSlash(filepath.Clean(name)), "../") {
		return 0
	}
	for _, binding := range []p12FieldPVPNativeBinding{{bundle.Source, bundle.SourceSHA}, {bundle.Capture, bundle.CaptureSHA}, {filepath.ToSlash(filepath.Join("analysis/p6", name)), hits.SHA}} {
		clean := filepath.ToSlash(filepath.Clean(binding.Path))
		// All independent field PK inputs and captures remain SHA-bound under analysis/p6.
		if filepath.IsAbs(binding.Path) || (!strings.HasPrefix(clean, "analysis/p6/") && clean != "analysis/p6/field-pvp5-actual-actors-pending-20261016/manifest.json") {
			return 0
		}
		data, err := os.ReadFile(filepath.Join("../..", binding.Path))
		if err != nil || len(binding.SHA) != 64 || p6Digest(data) != binding.SHA {
			return 0
		}
	}
	return 4
}

func p12FieldPVPNativeBoundInput(t *testing.T, input *model.FrozenBattleInput, wireSHA, fixtureSHA string) bool {
	t.Helper()
	bundle := p12FieldPVPNativeLoadBundle(t)
	if bundle == nil {
		return false
	}
	bound := false
	for _, fixture := range bundle.Fixtures {
		if fixture.SHA == fixtureSHA {
			p12FieldPVPNativeRead(t, filepath.ToSlash(filepath.Join("analysis/p6", fixture.Path)), fixture.SHA)
			bound = true
		}
	}
	if !bound {
		return false
	}
	for _, row := range p12FieldPVPNativeCases(t) {
		if row.WireSHA != wireSHA {
			continue
		}
		wire := p12FieldPVPNativeTypedInput(t, row)
		if input == nil || input.BattleType != 5 || input.Monster != nil || input.Defender == nil || !proto.Equal(input.Attacker, wire.Attacker) || !proto.Equal(input.Defender, wire.Defender1) || input.Seed != wire.GetRandSeed() || input.MaxRound != 40 || input.DefenderFirst != (wire.Attacker.GetInst().GetFirstOrder() == 0 && wire.Defender1.GetInst().GetFirstOrder() == 1) || input.IsAttToDef || input.KFReplay != nil || input.KFReplayViewer != nil || len(input.WinConditions) != 0 || input.PVECompatibilityRule != "" || input.DiagnosticAIDriverPolicy != "" || input.DiagnosticManualAutoStartRoundTime != nil || input.AttackerDamageFactor == nil || *input.AttackerDamageFactor != 1 || input.DefenderDamageFactor == nil || *input.DefenderDamageFactor != 1 {
			t.Fatal("P12 Field PK original typed5 complete frozen projection differs")
		}
		return true
	}
	t.Fatal("P12 Field PK fixture attempted another input family")
	return false
}

func TestP12FieldPVPNativeSourceBinding(t *testing.T) {
	// Verify actual account4 and original NPC provenance before admitting this finite diagnostic source.

	rows := p12FieldPVPNativeCases(t)
	for index, row := range rows {
		if row.Seed != []int64{7, 314159, 7, 314159}[index] {
			t.Fatal("prepublished finite original seeds changed")
		}
		p12FieldPVPNativeTypedInput(t, row)
	}
}

func TestP12FieldPVPNativeAllLayers(t *testing.T) {
	bundle := p12FieldPVPNativeLoadBundle(t)
	if bundle == nil {
		t.Skip("PENDING: independent four-case full-two-actor typed5 native capture and SHA-bound diagnostic bundle")
	}
	TestP12FieldPVPNativeSourceBinding(t)
	for kind, fixture := range bundle.Fixtures {
		fixtureRaw := p12FieldPVPNativeRead(t, filepath.ToSlash(filepath.Join("analysis/p6", fixture.Path)), fixture.SHA)
		var tree any
		if json.Unmarshal(fixtureRaw, &tree) != nil {
			t.Fatal("P12 Field PK derived fixture JSON", kind)
		}
		allowed := map[string]bool{}
		for _, row := range p12FieldPVPNativeCases(t) {
			allowed[row.WireSHA] = true
		}
		seenWires := map[string]bool{}
		var walk func(any)
		walk = func(node any) {
			switch value := node.(type) {
			case map[string]any:
				if hash, ok := value["wireSHA256"].(string); ok {
					if !allowed[hash] {
						t.Fatal("P12 Field PK comparison layer attempted another wire family", kind)
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
		if len(seenWires) != 4 {
			t.Fatal("P12 Field PK every comparison layer must retain all four diagnostic sources", kind)
		}
		if kind == "records" {
			var records struct {
				Cases []struct {
					WireSHA string                     `json:"wireSHA256"`
					Actors  []model.CombatFinalActorHP `json:"originalFinalActorHP"`
				} `json:"cases"`
			}
			raw := p12FieldPVPNativeRead(t, filepath.ToSlash(filepath.Join("analysis/p6", fixture.Path)), fixture.SHA)
			if json.Unmarshal(raw, &records) != nil || len(records.Cases) != 4 {
				t.Fatal("P12 Field PK native complete terminal actor cases required")
			}
			seen := map[string]bool{}
			for _, record := range records.Cases {
				for _, row := range p12FieldPVPNativeCases(t) {
					if record.WireSHA != row.WireSHA {
						continue
					}
					wire := p12FieldPVPNativeTypedInput(t, row)
					if seen[record.WireSHA] || len(record.Actors) != 2 {
						t.Fatal("P12 Field PK native two-actor terminal denominator")
					}
					seen[record.WireSHA] = true
					for index, actor := range append(append([]*pb.BattleHero{}, wire.Attacker.Formation...), wire.Defender1.Formation...) {
						final := record.Actors[index]
						camp := int32(1)
						if index < 1 {
							camp = 0
						}
						if final.Camp != camp || final.HeroID != actor.GetId() || final.UID == "" {
							t.Fatal("P12 Field PK full native terminal actor identity/order")
						}
					}
				}
			}
			if len(seen) != 4 {
				t.Fatal("P12 Field PK records cannot borrow another native wire")
			}
		}
	}
	core, hits, manager, records := bundle.Fixtures["core"], bundle.Fixtures["hits"], bundle.Fixtures["manager"], bundle.Fixtures["records"]
	if p6NativeOriginalPerHitCaseCount(hits.Path) != 4 {
		t.Fatal("P12 Field PK SHA-bound four-case denominator hook is missing")
	}
	t.Run("core", func(t *testing.T) { p6NativeAutomaticPVPOriginalTickGolden(t, core.Path, core.SHA, 4) })
	t.Run("actor44", func(t *testing.T) { p6NativeActorPropertiesFromFixture(t, "", true, core.Path, core.SHA, 4) })
	t.Run("lifetimes", func(t *testing.T) { p6NativeDetailsFromFixture(t, "", true, core.Path, core.SHA, 4) })
	t.Run("Buffmetadata", func(t *testing.T) { p6NativeBuffSourceAndValuesFromFixture(t, "", true, core.Path, core.SHA, 4) })
	t.Run("manager", func(t *testing.T) { p6NativeOriginalManagerFixture(t, manager.Path, manager.SHA, 1, 4) })
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
	t.Run("final-protocol-bytes-terminal-HP", func(t *testing.T) { p6NativeOriginalProtocolRecordFixture(t, records.Path, records.SHA, false, 4) })
}
