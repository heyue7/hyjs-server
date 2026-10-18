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

type p12WorldBossNativeBinding struct {
	Path string `json:"path"`
	SHA  string `json:"sha256"`
}

type p12WorldBossNativeBundle struct {
	Schema     string                               `json:"schema"`
	Source     string                               `json:"sourceManifest"`
	SourceSHA  string                               `json:"sourceManifestSHA256"`
	Capture    string                               `json:"captureManifest"`
	CaptureSHA string                               `json:"captureManifestSHA256"`
	Count      int                                  `json:"caseCount"`
	Scope      string                               `json:"nativeScope"`
	Authority  bool                                 `json:"businessAuthority"`
	Business   bool                                 `json:"actualBusinessStart"`
	Kind       string                               `json:"sourceKind"`
	Fixtures   map[string]p12WorldBossNativeBinding `json:"fixtures"`
}

type p12WorldBossNativeCase struct {
	Name       string   `json:"case"`
	Wire       string   `json:"wire"`
	WireSHA    string   `json:"wireSHA256"`
	Typed      string   `json:"typedCallback"`
	TypedSHA   string   `json:"typedCallbackSHA256"`
	FightID    string   `json:"fightId"`
	Seed       int64    `json:"seed"`
	Type       int32    `json:"battleType"`
	Members    int      `json:"fullBossMembers"`
	Level      int64    `json:"bossLevel"`
	ActorSHAs  []string `json:"sourceActorSHA256"`
	MemberSHAs []string `json:"sourceBossMemberSHA256"`
}

func p12WorldBossNativePath(t *testing.T, relative string) string {
	t.Helper()
	clean := filepath.Clean(relative)
	if filepath.IsAbs(clean) || !strings.HasPrefix(filepath.ToSlash(clean), "analysis/p6/") {
		t.Fatal("P12 diagnostic evidence must remain under analysis/p6", relative)
	}
	return filepath.Join("../..", clean)
}

func p12WorldBossNativeRead(t *testing.T, name, expected string) []byte {
	t.Helper()
	raw, err := os.ReadFile(p12WorldBossNativePath(t, name))
	if err != nil || len(expected) != 64 || p6Digest(raw) != expected {
		t.Fatal("P12 diagnostic immutable source SHA", name, err)
	}
	return raw
}

func p12WorldBossNativeCases(t *testing.T) []p12WorldBossNativeCase {
	t.Helper()
	raw := p12WorldBossNativeRead(t, "analysis/p6/worldboss18-actual-p5-diagnostic-20261016/manifest.json", "8d03aad70c046ef6843fc8003e850cf14bada171a541af9a962e7d004bc99e99")
	var source struct {
		Kind      string                   `json:"sourceKind"`
		Business  bool                     `json:"actualBusinessStart"`
		Compared  bool                     `json:"nativeCompared"`
		Authority bool                     `json:"authoritative"`
		Executed  bool                     `json:"engineExecuted"`
		Cases     []p12WorldBossNativeCase `json:"cases"`
	}
	if json.Unmarshal(raw, &source) != nil || source.Kind != "finite-actual-p5-original-table-diagnostic" || source.Business || source.Compared || source.Authority || source.Executed || len(source.Cases) != 2 {
		t.Fatal("P12 two-case actual-P5 diagnostic scope; no business Start or authority")
	}
	return source.Cases
}

// p12WorldBossNativeTypedInput binds the complete original typed18 callback to
// its playback projection. No diagnostic AI override or result hint is added.
func p12WorldBossNativeTypedInput(t *testing.T, row p12WorldBossNativeCase) *pb.SCFightPlayback {
	t.Helper()
	base := "analysis/p6/worldboss18-actual-p5-diagnostic-20261016/"
	var (
		wire  pb.SCFightPlayback
		start pb.SCWorldBossFight
	)
	if proto.Unmarshal(p12WorldBossNativeRead(t, base+row.Wire, row.WireSHA), &wire) != nil || proto.Unmarshal(p12WorldBossNativeRead(t, base+row.Typed, row.TypedSHA), &start) != nil {
		t.Fatal("P12 original protocol bytes")
	}
	if row.Type != 18 || row.Members != 4 || row.Level != 1 || row.Name != row.FightID || wire.GetFightid() != row.FightID || start.GetFightId() != row.FightID || start.RandSeed == nil || wire.GetRandSeed() != row.Seed || start.GetRandSeed() != row.Seed || wire.GetBattleType() != 18 || wire.GetMaxRound() != 40 || start.GetMaxround() != 40 || wire.Defender1 != nil || wire.Defender2 == nil || wire.Attacker == nil || len(start.BattleMonsterList) != 1 || len(wire.Defender2.Formation) != 4 || len(wire.Attacker.Formation) != 1 || wire.OptRecord != nil || start.GetFightScene() == "" || wire.AttackerFactor == nil || wire.GetAttackerFactor() != 1 || wire.DefenderFactor == nil || wire.GetDefenderFactor() != 1 || !proto.Equal(wire.Attacker, start.BattlePlayer) || !proto.Equal(wire.Defender2, start.BattleMonsterList[0]) || start.Code != nil || start.Damange != nil {
		t.Fatal("P12 exact typed18, four-body, factors1/1 original automatic diagnostic input")
	}
	if len(row.ActorSHAs) != 1 || len(row.MemberSHAs) != 4 {
		t.Fatal("P12 complete actor source denominator")
	}
	poses := []int32{13, 7, 9, 3}
	blood := []int32{2, 1, 1, 1}
	for camp, formation := range [][]*pb.BattleHero{wire.Attacker.Formation, wire.Defender2.Formation} {
		for index, actor := range formation {
			encoded, err := proto.MarshalOptions{Deterministic: true}.Marshal(actor)
			if err != nil || actor.Prop == nil || actor.Inst == nil {
				t.Fatal("P12 complete source actor", err)
			}
			if camp == 0 {
				if p6Digest(encoded) != row.ActorSHAs[index] || p6Digest(encoded) != "8a845f36d3a79dfcf8ebf2ee3498a791ca0b32ea920a72d4cb84c8975ed591df" || actor.GetId() != "hero_047" {
					t.Fatal("P12 unchanged actual paid P5 BattleHero")
				}
			} else if p6Digest(encoded) != row.MemberSHAs[index] || actor.GetPose() != poses[index] || actor.GetBloodNumber() != blood[index] || !strings.HasPrefix(actor.GetGuid(), "worldboss-") {
				t.Fatal("P12 full original body/talent/skill/blood source")
			}
		}
	}
	return &wire
}

func p12WorldBossNativeLoadBundle(t *testing.T) *p12WorldBossNativeBundle {
	t.Helper()
	name := os.Getenv("HYJS_P12_WORLDBOSS_NATIVE_BUNDLE")
	if name == "" {
		return nil
	}
	raw, err := os.ReadFile(name)
	expected := os.Getenv("HYJS_P12_WORLDBOSS_NATIVE_BUNDLE_SHA256")
	if err != nil || len(expected) != 64 || p6Digest(raw) != expected {
		t.Fatal("explicit P12 native bundle SHA", err)
	}
	var bundle p12WorldBossNativeBundle
	if json.Unmarshal(raw, &bundle) != nil || bundle.Schema != "p12-worldboss-native-matrix-v1" || bundle.Source != "analysis/p6/worldboss18-actual-p5-diagnostic-20261016/manifest.json" || bundle.SourceSHA != "8d03aad70c046ef6843fc8003e850cf14bada171a541af9a962e7d004bc99e99" || bundle.Count != 2 || bundle.Scope != "original-worldboss-automatic-ai" || bundle.Authority || bundle.Business || bundle.Kind != "finite-actual-p5-original-table-diagnostic" || len(bundle.Fixtures) != 4 {
		t.Fatal("bounded P12 original typed18 diagnostic bundle identity/denominator/authority")
	}
	p12WorldBossNativeRead(t, bundle.Source, bundle.SourceSHA)
	captureRaw := p12WorldBossNativeRead(t, bundle.Capture, bundle.CaptureSHA)
	var capture struct {
		Source    string                   `json:"sourceManifest"`
		SourceSHA string                   `json:"sourceManifestSHA256"`
		Selector  string                   `json:"selector"`
		Cases     []p12WorldBossNativeCase `json:"cases"`
	}
	if json.Unmarshal(captureRaw, &capture) != nil || capture.SourceSHA != bundle.SourceSHA || capture.Selector != "highest" || len(capture.Cases) != 2 {
		t.Fatal("P12 original capture source/selector/denominator")
	}
	actualSource, err := filepath.Abs(capture.Source)
	expectedSource, pathErr := filepath.Abs(p12WorldBossNativePath(t, bundle.Source))
	if err != nil || pathErr != nil || actualSource != expectedSource {
		t.Fatal("P12 capture must bind the exact diagnostic source manifest")
	}
	seen := map[string]bool{}
	for _, row := range capture.Cases {
		for _, original := range p12WorldBossNativeCases(t) {
			if row.WireSHA == original.WireSHA && row.TypedSHA == original.TypedSHA && row.FightID == original.FightID && row.Seed == original.Seed && !seen[row.WireSHA] {
				seen[row.WireSHA] = true
			}
		}
	}
	if len(seen) != 2 {
		t.Fatal("P12 capture cannot borrow another typed18 source")
	}
	for _, kind := range []string{"core", "hits", "manager", "records"} {
		fixture := bundle.Fixtures[kind]
		if fixture.Path == "" || len(fixture.SHA) != 64 {
			t.Fatal("all P12 native comparison layers required", kind)
		}
		p12WorldBossNativePath(t, filepath.Join("analysis/p6", fixture.Path))
	}
	return &bundle
}

// The count hook admits this exact independent bundle; earlier P7/other fixture
// denominators remain unchanged. Missing or malformed opt-in evidence cannot pass.
func p12WorldBossNativeHitCaseCount(name string) int {
	raw, err := os.ReadFile(os.Getenv("HYJS_P12_WORLDBOSS_NATIVE_BUNDLE"))
	if err != nil || p6Digest(raw) != os.Getenv("HYJS_P12_WORLDBOSS_NATIVE_BUNDLE_SHA256") {
		return 0
	}
	var bundle p12WorldBossNativeBundle
	if json.Unmarshal(raw, &bundle) != nil || bundle.Schema != "p12-worldboss-native-matrix-v1" || bundle.Count != 2 || bundle.Authority || bundle.Business || bundle.Kind != "finite-actual-p5-original-table-diagnostic" || bundle.Scope != "original-worldboss-automatic-ai" || bundle.Source != "analysis/p6/worldboss18-actual-p5-diagnostic-20261016/manifest.json" || bundle.SourceSHA != "8d03aad70c046ef6843fc8003e850cf14bada171a541af9a962e7d004bc99e99" || len(bundle.Fixtures) != 4 {
		return 0
	}
	hits := bundle.Fixtures["hits"]
	if hits.Path != name || filepath.IsAbs(name) || strings.HasPrefix(filepath.ToSlash(filepath.Clean(name)), "../") {
		return 0
	}
	for _, binding := range []p12WorldBossNativeBinding{{bundle.Source, bundle.SourceSHA}, {bundle.Capture, bundle.CaptureSHA}, {filepath.ToSlash(filepath.Join("analysis/p6", name)), hits.SHA}} {
		if filepath.IsAbs(binding.Path) || !strings.HasPrefix(filepath.ToSlash(filepath.Clean(binding.Path)), "analysis/p6/") {
			return 0
		}
		data, err := os.ReadFile(filepath.Join("../..", binding.Path))
		if err != nil || len(binding.SHA) != 64 || p6Digest(data) != binding.SHA {
			return 0
		}
	}
	return 2
}

func p12WorldBossNativeBoundInput(t *testing.T, input *model.FrozenBattleInput, wireSHA, fixtureSHA string) bool {
	t.Helper()
	bundle := p12WorldBossNativeLoadBundle(t)
	if bundle == nil {
		return false
	}
	bound := false
	for _, fixture := range bundle.Fixtures {
		if fixture.SHA == fixtureSHA {
			p12WorldBossNativeRead(t, filepath.ToSlash(filepath.Join("analysis/p6", fixture.Path)), fixture.SHA)
			bound = true
		}
	}
	if !bound {
		return false
	}
	for _, row := range p12WorldBossNativeCases(t) {
		if row.WireSHA != wireSHA {
			continue
		}
		wire := p12WorldBossNativeTypedInput(t, row)
		if input == nil || input.BattleType != 18 || input.Monster == nil || input.Defender != nil || !proto.Equal(input.Attacker, wire.Attacker) || !proto.Equal(input.Monster, wire.Defender2) || input.Seed != wire.GetRandSeed() || input.MaxRound != 40 || input.DefenderFirst || input.IsAttToDef || input.KFReplay != nil || input.KFReplayViewer != nil || len(input.WinConditions) != 0 || input.PVECompatibilityRule != "" || input.DiagnosticAIDriverPolicy != "" || input.DiagnosticManualAutoStartRoundTime != nil || input.AttackerDamageFactor == nil || *input.AttackerDamageFactor != 1 || input.DefenderDamageFactor == nil || *input.DefenderDamageFactor != 1 {
			t.Fatal("P12 original typed18 complete frozen projection differs")
		}
		return true
	}
	t.Fatal("P12 fixture attempted another input family")
	return false
}

func TestP12WorldBossNativeSourceBinding(t *testing.T) {
	// Verify actual P5 provenance before admitting this finite diagnostic source.
	p12WorldBossNativeRead(t, "analysis/p6/p5-combined-all76-highest-actors-20261014.json", "7d59f21702ff17c3c28bd861edc6840c66cda5224bc8c585e740972e82071772")
	rows := p12WorldBossNativeCases(t)
	for index, row := range rows {
		if row.Seed != []int64{7, 314159}[index] {
			t.Fatal("prepublished finite original seeds changed")
		}
		p12WorldBossNativeTypedInput(t, row)
	}
}

func TestP12WorldBossNativeAllLayers(t *testing.T) {
	bundle := p12WorldBossNativeLoadBundle(t)
	if bundle == nil {
		t.Skip("PENDING: independent two-case full-four-body typed18 native capture and SHA-bound diagnostic bundle")
	}
	TestP12WorldBossNativeSourceBinding(t)
	for kind, fixture := range bundle.Fixtures {
		fixtureRaw := p12WorldBossNativeRead(t, filepath.ToSlash(filepath.Join("analysis/p6", fixture.Path)), fixture.SHA)
		var tree any
		if json.Unmarshal(fixtureRaw, &tree) != nil {
			t.Fatal("P12 derived fixture JSON", kind)
		}
		allowed := map[string]bool{}
		for _, row := range p12WorldBossNativeCases(t) {
			allowed[row.WireSHA] = true
		}
		seenWires := map[string]bool{}
		var walk func(any)
		walk = func(node any) {
			switch value := node.(type) {
			case map[string]any:
				if hash, ok := value["wireSHA256"].(string); ok {
					if !allowed[hash] {
						t.Fatal("P12 comparison layer attempted another wire family", kind)
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
			t.Fatal("P12 every comparison layer must retain both diagnostic sources", kind)
		}
		if kind == "records" {
			var records struct {
				Cases []struct {
					WireSHA string                     `json:"wireSHA256"`
					Actors  []model.CombatFinalActorHP `json:"originalFinalActorHP"`
				} `json:"cases"`
			}
			raw := p12WorldBossNativeRead(t, filepath.ToSlash(filepath.Join("analysis/p6", fixture.Path)), fixture.SHA)
			if json.Unmarshal(raw, &records) != nil || len(records.Cases) != 2 {
				t.Fatal("P12 native complete terminal actor cases required")
			}
			seen := map[string]bool{}
			for _, record := range records.Cases {
				for _, row := range p12WorldBossNativeCases(t) {
					if record.WireSHA != row.WireSHA {
						continue
					}
					wire := p12WorldBossNativeTypedInput(t, row)
					if seen[record.WireSHA] || len(record.Actors) != 5 {
						t.Fatal("P12 native five-actor terminal denominator")
					}
					seen[record.WireSHA] = true
					for index, actor := range append(append([]*pb.BattleHero{}, wire.Attacker.Formation...), wire.Defender2.Formation...) {
						final := record.Actors[index]
						camp := int32(1)
						if index == 0 {
							camp = 0
						}
						if final.Camp != camp || final.HeroID != actor.GetId() || final.UID == "" {
							t.Fatal("P12 full native terminal actor identity/order")
						}
					}
				}
			}
			if len(seen) != 2 {
				t.Fatal("P12 records cannot borrow another native wire")
			}
		}
	}
	core, hits, manager, records := bundle.Fixtures["core"], bundle.Fixtures["hits"], bundle.Fixtures["manager"], bundle.Fixtures["records"]
	if p6NativeOriginalPerHitCaseCount(hits.Path) != 2 {
		t.Fatal("P12 SHA-bound two-case denominator hook is missing")
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
	// Native records contain twenty camp1 actions in each case. Require those
	// original container entries as well as the helper's mandatory terminal HP.
	t.Run("final-protocol-bytes-terminal-HP", func(t *testing.T) { p6NativeOriginalProtocolRecordFixture(t, records.Path, records.SHA, true, 2) })
}
