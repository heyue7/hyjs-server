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

type p12WantedNativeBinding struct {
	Path string `json:"path"`
	SHA  string `json:"sha256"`
}

type p12WantedNativeBundle struct {
	Schema     string                            `json:"schema"`
	Source     string                            `json:"sourceManifest"`
	SourceSHA  string                            `json:"sourceManifestSHA256"`
	Capture    string                            `json:"captureManifest"`
	CaptureSHA string                            `json:"captureManifestSHA256"`
	Count      int                               `json:"caseCount"`
	Scope      string                            `json:"nativeScope"`
	Authority  bool                              `json:"businessAuthority"`
	Business   bool                              `json:"actualBusinessStart"`
	Kind       string                            `json:"sourceKind"`
	Fixtures   map[string]p12WantedNativeBinding `json:"fixtures"`
}

type p12WantedNativeCase struct {
	Name       string  `json:"case"`
	Wire       string  `json:"wire"`
	WireSHA    string  `json:"wireSHA256"`
	Typed      string  `json:"typedCallback"`
	TypedSHA   string  `json:"typedCallbackSHA256"`
	FightID    string  `json:"fightId"`
	Seed       int64   `json:"seed"`
	Type       int32   `json:"battleType"`
	AttackType int32   `json:"attackType"`
	Factor     float64 `json:"damageFactor"`
}

func p12WantedNativePath(t *testing.T, relative string) string {
	t.Helper()
	clean := filepath.Clean(relative)
	if filepath.IsAbs(clean) || (!strings.HasPrefix(filepath.ToSlash(clean), "analysis/p6/") && !strings.HasPrefix(filepath.ToSlash(clean), "analysis/p12/wanted-playable-20261016/")) {
		t.Fatal("P12 Wanted diagnostic evidence must remain under analysis/p6", relative)
	}
	return filepath.Join("../..", clean)
}

func p12WantedNativeRead(t *testing.T, name, expected string) []byte {
	t.Helper()
	raw, err := os.ReadFile(p12WantedNativePath(t, name))
	if err != nil || len(expected) != 64 || p6Digest(raw) != expected {
		t.Fatal("P12 Wanted diagnostic immutable source SHA", name, err)
	}
	return raw
}

func p12WantedNativeCases(t *testing.T) []p12WantedNativeCase {
	t.Helper()
	raw := p12WantedNativeRead(t, "analysis/p12/wanted-playable-20261016/native-inputs/manifest.json", "a5087605dffb192314f04c456056860873546bb822523d6c11d75177e727800a")
	var source struct {
		Cases       []p12WantedNativeCase `json:"cases"`
		Compared    bool                  `json:"nativeCompared"`
		Authority   bool                  `json:"authoritative"`
		Tables      map[string]string     `json:"originalTableSHA256"`
		ActorSource string                `json:"actorSource"`
		ActorSHA    string                `json:"actorSourceSHA256"`
	}
	if json.Unmarshal(raw, &source) != nil || len(source.Cases) != 4 || source.Compared || source.Authority || len(source.Tables) != 7 {
		t.Fatal("independent full four-case typed8 source identity")
	}
	for name, expected := range source.Tables {
		data, err := os.ReadFile(filepath.Join("../../analysis/tables", "Table"+name+".tsv"))
		if err != nil || len(expected) != 64 || p6Digest(data) != expected {
			t.Fatal("Wanted original table SHA", name, err)
		}
	}
	if source.ActorSource != "analysis/p6/actual-demo4-vs-demo3-friend11-input-v128b/000.frozen-input.json" || source.ActorSHA != "d0e70186dc665f5e894eaae0a30915cb8032beb2ab86e0950ebd3e0ada41fa94" {
		t.Fatal("actual party provenance")
	}
	p12WantedNativeRead(t, source.ActorSource, source.ActorSHA)
	return source.Cases
}

func p12WantedNativeTypedInput(t *testing.T, row p12WantedNativeCase) *pb.SCFightPlayback {
	t.Helper()
	base := "analysis/p12/wanted-playable-20261016/native-inputs/"
	var wire pb.SCFightPlayback
	var start pb.SCRebelFightStart
	if proto.Unmarshal(p12WantedNativeRead(t, base+row.Wire, row.WireSHA), &wire) != nil || proto.Unmarshal(p12WantedNativeRead(t, base+row.Typed, row.TypedSHA), &start) != nil {
		t.Fatal("Wanted original protobuf inputs")
	}
	if row.Type != 8 || row.Name != row.FightID || wire.GetFightid() != row.FightID || start.GetFightId() != row.FightID || start.RandSeed == nil || wire.GetRandSeed() != row.Seed || start.GetRandSeed() != row.Seed || wire.GetBattleType() != 8 || wire.GetMaxRound() != 20 || start.GetMaxround() != 20 || start.GetFightRoomId() != "map_grass_001" || wire.Defender1 != nil || wire.OptRecord != nil || len(wire.WinConditions) != 0 || wire.Attacker == nil || wire.Defender2 == nil || len(start.BattleMonsterList) != 1 || len(wire.Attacker.Formation) != 2 || len(wire.Defender2.Formation) != 4 || wire.AttackerFactor == nil || wire.GetAttackerFactor() != row.Factor || start.GetDamageFactor() != row.Factor || wire.DefenderFactor == nil || wire.GetDefenderFactor() != 1 || !proto.Equal(wire.Attacker, start.BattlePlayer) || !proto.Equal(wire.Defender2, start.BattleMonsterList[0]) {
		t.Fatal("strict complete typed8 normal/deadly projection")
	}
	expected := [][]string{
		{"bf72e8364dd340aeea87a59f3001f046679c2a51833c526e17b9bc4b10368ef7", "01325a107d0e67d2bb97261af19c3e697ac13dacc1dd4edfabb4f1f7b28413be"},
		{"b08a58e43afd108996b4bfd1aa964742f712eb54cb7c402c363d04dc529d47de", "fe4b615600ea0dc06baccd0b7168f39e50751a13b93f1dab24c0242c9b4f62e3", "6f0a21e225e89a0fce49f9b951eeb4bbd44815e26883f9a1f540225968dca963", "fbc37eb5b74a85d84e043644049ef87494fa4fcf68b6c075a32e6a94f889ecc0"},
	}
	for camp, formation := range [][]*pb.BattleHero{wire.Attacker.Formation, wire.Defender2.Formation} {
		for i, actor := range formation {
			raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(actor)
			if err != nil || actor.Prop == nil || actor.Inst == nil || p6Digest(raw) != expected[camp][i] {
				t.Fatal("complete unchanged actual party/original Wanted NPC bytes", camp, i, err)
			}
		}
	}

	return &wire
}

func p12WantedNativeLoadBundle(t *testing.T) *p12WantedNativeBundle {
	t.Helper()
	name := os.Getenv("HYJS_P12_WANTED_NATIVE_BUNDLE")
	if name == "" {
		return nil
	}
	raw, err := os.ReadFile(name)
	expected := os.Getenv("HYJS_P12_WANTED_NATIVE_BUNDLE_SHA256")
	if err != nil || len(expected) != 64 || p6Digest(raw) != expected {
		t.Fatal("explicit P12 Wanted native bundle SHA", err)
	}
	var bundle p12WantedNativeBundle
	if json.Unmarshal(raw, &bundle) != nil || bundle.Schema != "p12-wanted-native-matrix-v1" || bundle.Source != "analysis/p12/wanted-playable-20261016/native-inputs/manifest.json" || bundle.SourceSHA != "a5087605dffb192314f04c456056860873546bb822523d6c11d75177e727800a" || bundle.Count != 4 || bundle.Scope != "original-wanted-automatic-ai" || bundle.Authority || bundle.Business || bundle.Kind != "finite-historical-actual001003-original-wanted1-table" || len(bundle.Fixtures) != 4 {
		t.Fatal("bounded P12 Wanted original typed8 diagnostic bundle identity/denominator/authority")
	}
	p12WantedNativeRead(t, bundle.Source, bundle.SourceSHA)
	captureRaw := p12WantedNativeRead(t, bundle.Capture, bundle.CaptureSHA)
	var capture struct {
		Source    string                `json:"sourceManifest"`
		SourceSHA string                `json:"sourceManifestSHA256"`
		Selector  string                `json:"selector"`
		Cases     []p12WantedNativeCase `json:"cases"`
	}
	if json.Unmarshal(captureRaw, &capture) != nil || capture.SourceSHA != bundle.SourceSHA || capture.Selector != "highest" || len(capture.Cases) != 4 {
		t.Fatal("P12 Wanted original capture source/selector/denominator")
	}
	actualSource, err := filepath.Abs(capture.Source)
	expectedSource, pathErr := filepath.Abs(p12WantedNativePath(t, bundle.Source))
	if err != nil || pathErr != nil || actualSource != expectedSource {
		t.Fatal("P12 Wanted capture must bind the exact diagnostic source manifest")
	}
	seen := map[string]bool{}
	for _, row := range capture.Cases {
		for _, original := range p12WantedNativeCases(t) {
			if row.WireSHA == original.WireSHA && row.TypedSHA == original.TypedSHA && row.FightID == original.FightID && row.Seed == original.Seed && !seen[row.WireSHA] {
				seen[row.WireSHA] = true
			}
		}
	}
	if len(seen) != 4 {
		t.Fatal("P12 Wanted capture cannot borrow another typed8 source")
	}
	for _, kind := range []string{"core", "hits", "manager", "records"} {
		fixture := bundle.Fixtures[kind]
		if fixture.Path == "" || len(fixture.SHA) != 64 {
			t.Fatal("all P12 Wanted native comparison layers required", kind)
		}
		p12WantedNativePath(t, filepath.Join("analysis/p6", fixture.Path))
	}
	return &bundle
}

// The count hook admits this exact independent bundle; earlier P7/other fixture
// denominators remain unchanged. Missing or malformed opt-in evidence cannot pass.
func p12WantedNativeHitCaseCount(name string) int {
	raw, err := os.ReadFile(os.Getenv("HYJS_P12_WANTED_NATIVE_BUNDLE"))
	if err != nil || p6Digest(raw) != os.Getenv("HYJS_P12_WANTED_NATIVE_BUNDLE_SHA256") {
		return 0
	}
	var bundle p12WantedNativeBundle
	if json.Unmarshal(raw, &bundle) != nil || bundle.Schema != "p12-wanted-native-matrix-v1" || bundle.Count != 4 || bundle.Authority || bundle.Business || bundle.Kind != "finite-historical-actual001003-original-wanted1-table" || bundle.Scope != "original-wanted-automatic-ai" || bundle.Source != "analysis/p12/wanted-playable-20261016/native-inputs/manifest.json" || bundle.SourceSHA != "a5087605dffb192314f04c456056860873546bb822523d6c11d75177e727800a" || len(bundle.Fixtures) != 4 {
		return 0
	}
	hits := bundle.Fixtures["hits"]
	if hits.Path != name || filepath.IsAbs(name) || strings.HasPrefix(filepath.ToSlash(filepath.Clean(name)), "../") {
		return 0
	}
	for _, binding := range []p12WantedNativeBinding{{bundle.Source, bundle.SourceSHA}, {bundle.Capture, bundle.CaptureSHA}, {filepath.ToSlash(filepath.Join("analysis/p6", name)), hits.SHA}} {
		clean := filepath.ToSlash(filepath.Clean(binding.Path))
		// 唯一新增P7路径是已钉source manifest；capture/fixtures仍限定原analysis/p6。
		if filepath.IsAbs(binding.Path) || (!strings.HasPrefix(clean, "analysis/p6/") && clean != "analysis/p12/wanted-playable-20261016/native-inputs/manifest.json") {
			return 0
		}
		data, err := os.ReadFile(filepath.Join("../..", binding.Path))
		if err != nil || len(binding.SHA) != 64 || p6Digest(data) != binding.SHA {
			return 0
		}
	}
	return 4
}

func p12WantedNativeBoundInput(t *testing.T, input *model.FrozenBattleInput, wireSHA, fixtureSHA string) bool {
	t.Helper()
	bundle := p12WantedNativeLoadBundle(t)
	if bundle == nil {
		return false
	}
	bound := false
	for _, fixture := range bundle.Fixtures {
		if fixture.SHA == fixtureSHA {
			p12WantedNativeRead(t, filepath.ToSlash(filepath.Join("analysis/p6", fixture.Path)), fixture.SHA)
			bound = true
		}
	}
	if !bound {
		return false
	}
	for _, row := range p12WantedNativeCases(t) {
		if row.WireSHA != wireSHA {
			continue
		}
		wire := p12WantedNativeTypedInput(t, row)
		if input == nil || input.BattleType != 8 || input.Monster == nil || input.Defender != nil || !proto.Equal(input.Attacker, wire.Attacker) || !proto.Equal(input.Monster, wire.Defender2) || input.Seed != wire.GetRandSeed() || input.MaxRound != 20 || input.DefenderFirst || input.IsAttToDef || input.KFReplay != nil || input.KFReplayViewer != nil || len(input.WinConditions) != 0 || input.PVECompatibilityRule != "" || input.DiagnosticAIDriverPolicy != "" || input.DiagnosticManualAutoStartRoundTime != nil || input.AttackerDamageFactor == nil || *input.AttackerDamageFactor != wire.GetAttackerFactor() || input.DefenderDamageFactor == nil || *input.DefenderDamageFactor != 1 {
			t.Fatal("P12 Wanted original typed8 complete frozen projection differs")
		}
		return true
	}
	t.Fatal("P12 Wanted fixture attempted another input family")
	return false
}

func TestP12WantedNativeSourceBinding(t *testing.T) {
	// Verify actual account4 and original NPC provenance before admitting this finite diagnostic source.

	rows := p12WantedNativeCases(t)
	for index, row := range rows {
		if row.Seed != []int64{7, 314159}[index%2] || row.AttackType != int32(index/2+1) || row.Factor != []float64{1, 2.5}[index/2] {
			t.Fatal("prepublished finite original seeds changed")
		}
		p12WantedNativeTypedInput(t, row)
	}
}

func TestP12WantedNativeAllLayers(t *testing.T) {
	bundle := p12WantedNativeLoadBundle(t)
	if bundle == nil {
		t.Skip("PENDING: independent four-case full-four-actor typed8 native capture and SHA-bound diagnostic bundle")
	}
	TestP12WantedNativeSourceBinding(t)
	for kind, fixture := range bundle.Fixtures {
		fixtureRaw := p12WantedNativeRead(t, filepath.ToSlash(filepath.Join("analysis/p6", fixture.Path)), fixture.SHA)
		var tree any
		if json.Unmarshal(fixtureRaw, &tree) != nil {
			t.Fatal("P12 Wanted derived fixture JSON", kind)
		}
		allowed := map[string]bool{}
		for _, row := range p12WantedNativeCases(t) {
			allowed[row.WireSHA] = true
		}
		seenWires := map[string]bool{}
		var walk func(any)
		walk = func(node any) {
			switch value := node.(type) {
			case map[string]any:
				if hash, ok := value["wireSHA256"].(string); ok {
					if !allowed[hash] {
						t.Fatal("P12 Wanted comparison layer attempted another wire family", kind)
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
			t.Fatal("P12 Wanted every comparison layer must retain both diagnostic sources", kind)
		}
		if kind == "records" {
			var records struct {
				Cases []struct {
					WireSHA string                     `json:"wireSHA256"`
					Actors  []model.CombatFinalActorHP `json:"originalFinalActorHP"`
				} `json:"cases"`
			}
			raw := p12WantedNativeRead(t, filepath.ToSlash(filepath.Join("analysis/p6", fixture.Path)), fixture.SHA)
			if json.Unmarshal(raw, &records) != nil || len(records.Cases) != 4 {
				t.Fatal("P12 Wanted native complete terminal actor cases required")
			}
			seen := map[string]bool{}
			for _, record := range records.Cases {
				for _, row := range p12WantedNativeCases(t) {
					if record.WireSHA != row.WireSHA {
						continue
					}
					wire := p12WantedNativeTypedInput(t, row)
					if seen[record.WireSHA] || len(record.Actors) != 6 {
						t.Fatal("P12 Wanted native four-actor terminal denominator")
					}
					seen[record.WireSHA] = true
					for index, actor := range append(append([]*pb.BattleHero{}, wire.Attacker.Formation...), wire.Defender2.Formation...) {
						final := record.Actors[index]
						camp := int32(1)
						if index < 2 {
							camp = 0
						}
						if final.Camp != camp || final.HeroID != actor.GetId() || final.UID == "" {
							t.Fatal("P12 Wanted full native terminal actor identity/order")
						}
					}
				}
			}
			if len(seen) != 4 {
				t.Fatal("P12 Wanted records cannot borrow another native wire")
			}
		}
	}
	core, hits, manager, records := bundle.Fixtures["core"], bundle.Fixtures["hits"], bundle.Fixtures["manager"], bundle.Fixtures["records"]
	if p6NativeOriginalPerHitCaseCount(hits.Path) != 4 {
		t.Fatal("P12 Wanted SHA-bound four-case denominator hook is missing")
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
