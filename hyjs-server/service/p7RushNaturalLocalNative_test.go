package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

type p7NaturalLocalNativeFixture struct {
	Path string `json:"path"`
	SHA  string `json:"sha256"`
}

type p7NaturalLocalNativeBundle struct {
	Schema     string                                 `json:"schema"`
	Source     string                                 `json:"sourceManifest"`
	SourceSHA  string                                 `json:"sourceManifestSHA256"`
	Capture    string                                 `json:"captureManifest"`
	CaptureSHA string                                 `json:"captureManifestSHA256"`
	Count      int                                    `json:"caseCount"`
	Scope      string                                 `json:"nativeScope"`
	Origin     string                                 `json:"executionOrigin"`
	Authority  bool                                   `json:"businessAuthority"`
	Fixtures   map[string]p7NaturalLocalNativeFixture `json:"fixtures"`
}

func p7NaturalLocalNativePath(t *testing.T, relative string) string {
	t.Helper()
	clean := filepath.Clean(relative)
	if filepath.IsAbs(clean) || !strings.HasPrefix(filepath.ToSlash(clean), "analysis/p6/") {
		t.Fatal("native fixture must remain under project analysis", relative)
	}
	return filepath.Join("../..", clean)
}

func p7NaturalLocalRushNativeBundle(t *testing.T) *p7NaturalLocalNativeBundle {
	t.Helper()
	file := os.Getenv("HYJS_P7_NATURAL_LOCAL_BUNDLE")
	if file == "" {
		return nil
	}
	raw, err := os.ReadFile(file)
	if err != nil || len(os.Getenv("HYJS_P7_NATURAL_LOCAL_BUNDLE_SHA256")) != 64 || p6Digest(raw) != os.Getenv("HYJS_P7_NATURAL_LOCAL_BUNDLE_SHA256") {
		t.Fatal("explicit P7 native bundle SHA", err)
	}
	var bundle p7NaturalLocalNativeBundle
	if json.Unmarshal(raw, &bundle) != nil || bundle.Schema != "p7-natural047025-local-matrix-v1" || bundle.Source != "analysis/p6/p7-natural047025-local-inputs-r1-20261017/manifest.json" || bundle.SourceSHA != "e7bd26ff96f85de6ab38df71535557efe443aa04655f2850a10325d6a638709f" || bundle.Count != 4 || bundle.Scope != "original-pve-manual-then-auto-ai" || bundle.Authority || bundle.Origin != "local-coreclr-original-engine-not-phone-capture" || len(bundle.Fixtures) != 4 {
		t.Fatal("bounded original party bundle identity/denominator/authority")
	}
	for _, binding := range [][2]string{{bundle.Source, bundle.SourceSHA}, {bundle.Capture, bundle.CaptureSHA}} {
		data, err := os.ReadFile(p7NaturalLocalNativePath(t, binding[0]))
		if err != nil || len(binding[1]) != 64 || p6Digest(data) != binding[1] {
			t.Fatal("native source/capture manifest SHA", binding[0], err)
		}
	}
	for _, kind := range []string{"core", "hits", "manager", "records"} {
		fixture := bundle.Fixtures[kind]
		if fixture.Path == "" || len(fixture.SHA) != 64 {
			t.Fatal("all original matrix layers are required", kind)
		}
		p7NaturalLocalNativePath(t, filepath.Join("analysis/p6", fixture.Path))
	}
	return &bundle
}

// LOCAL original-Engine observations only. No production code calls these helpers.
// p7NaturalLocalRushNativeFixtureInput 只给显式SHA绑定的诊断fixtures选择原PVE手动时钟。
// 根集成在p6NativeManualAutoFixtureInput首部调用；生产权限入口不引用此测试helper。
func p7NaturalLocalRushNativeFixtureInput(t *testing.T, input *model.FrozenBattleInput, wireSHA, fixtureSHA string) (*model.FrozenBattleInput, bool) {
	t.Helper()
	bundle := p7NaturalLocalRushNativeBundle(t)
	if bundle == nil {
		return input, false
	}
	bound := false
	for _, fixture := range bundle.Fixtures {
		if fixture.SHA == fixtureSHA {
			bound = true
		}
	}
	if !bound {
		return input, false
	}
	raw, err := os.ReadFile(p7NaturalLocalNativePath(t, bundle.Source))
	if err != nil {
		t.Fatal(err)
	}
	var source struct {
		Cases []p7PartyRushCase `json:"cases"`
	}
	if json.Unmarshal(raw, &source) != nil || len(source.Cases) != 4 {
		t.Fatal("party source cases")
	}
	for _, row := range source.Cases {
		if row.WireSHA != wireSHA {
			continue
		}
		typed, err := os.ReadFile(filepath.Join(filepath.Dir(p7NaturalLocalNativePath(t, bundle.Source)), row.Typed))
		if err != nil || p6Digest(typed) != row.TypedSHA {
			t.Fatal("native party typed input SHA", err)
		}
		start := &pb.SCRushFightStart{}
		if proto.Unmarshal(typed, start) != nil || input.BattleType != 9 || input.Defender != nil || input.Monster == nil || len(start.BattleMonsterList) != 1 || !proto.Equal(input.Attacker, start.BattlePlayer) || !proto.Equal(input.Monster, start.BattleMonsterList[0]) || input.Seed != start.GetRandSeed() || input.MaxRound != start.GetMaxRound() || input.DefenderFirst || len(input.WinConditions) != 0 || input.PVECompatibilityRule != "" || input.KFReplay != nil || input.AttackerDamageFactor == nil || *input.AttackerDamageFactor != 1 || input.DefenderDamageFactor == nil || *input.DefenderDamageFactor != 1 {
			t.Fatal("original full-party typed9 wrapper source projection differs")
		}
		cloned, err := cloneP6CalculationInput(input)
		if err != nil {
			t.Fatal(err)
		}
		cloned.DiagnosticAIDriverPolicy = constUtil.P6DiagnosticManualAutoStart
		cloned.DiagnosticManualAutoStartRoundTime = proto.Float64(row.AutoAt)
		if err = ValidateP6AIDriverPolicy(cloned); err != nil {
			t.Fatal(err)
		}
		return cloned, true
	}
	t.Fatal("native party fixture attempted another wire family")
	return input, false
}

// p7NaturalLocalRushNativeHitCaseCount 仅给显式bundle内已校验命中的新fixture声明四例分母。
// 旧fixture继续使用原helper的既有固定分母。
func p7NaturalLocalRushNativeHitCaseCount(name string) int {
	raw, err := os.ReadFile(os.Getenv("HYJS_P7_NATURAL_LOCAL_BUNDLE"))
	if err != nil || p6Digest(raw) != os.Getenv("HYJS_P7_NATURAL_LOCAL_BUNDLE_SHA256") {
		return 0
	}
	var bundle p7NaturalLocalNativeBundle
	if json.Unmarshal(raw, &bundle) != nil || bundle.Schema != "p7-natural047025-local-matrix-v1" || bundle.Count != 4 || bundle.Authority || bundle.Origin != "local-coreclr-original-engine-not-phone-capture" || bundle.SourceSHA != "e7bd26ff96f85de6ab38df71535557efe443aa04655f2850a10325d6a638709f" {
		return 0
	}
	hits := bundle.Fixtures["hits"]
	if hits.Path != name || filepath.IsAbs(name) || strings.HasPrefix(filepath.ToSlash(filepath.Clean(name)), "../") {
		return 0
	}
	data, err := os.ReadFile(filepath.Join("../../analysis/p6", name))
	if err != nil || len(hits.SHA) != 64 || p6Digest(data) != hits.SHA {
		return 0
	}
	return 4
}

func TestP7NaturalLocalRushNativeAllLayers(t *testing.T) {
	bundle := p7NaturalLocalRushNativeBundle(t)
	if bundle == nil {
		t.Skip("PENDING: explicit four-case LOCAL Engine observations; no phone or authority proof")
	}
	// Verify every derived fixture once before invoking the unchanged highest helpers.
	for kind, fixture := range bundle.Fixtures {
		raw, err := os.ReadFile(p7NaturalLocalNativePath(t, filepath.Join("analysis/p6", fixture.Path)))
		if err != nil || p6Digest(raw) != fixture.SHA {
			t.Fatal("native fixture SHA", kind, err)
		}
	}
	core := bundle.Fixtures["core"]
	hits := bundle.Fixtures["hits"]
	manager := bundle.Fixtures["manager"]
	records := bundle.Fixtures["records"]
	if p6NativeOriginalPerHitCaseCount(hits.Path) != 4 {
		t.Fatal("P7 SHA-bound four-case denominator hook is not integrated")
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
	t.Run("final-protocol-bytes-terminal-HP", func(t *testing.T) { p6NativeOriginalProtocolRecordFixture(t, records.Path, records.SHA, false, 4) })
}

// Actual tutorial talent1/companion3 is retained. Only Player22/32 is a
// finite prerequisite. This test uses the production projector and no DB.
func TestP7NaturalLocalBattlePlayerProjection(t *testing.T) {
	raw, err := os.ReadFile("../../analysis/growth-first-20261016/m1-natural-phone-20261017/94-real-equipment-source-profile.json")
	if err != nil {
		t.Fatal(err)
	}
	var actual struct {
		Player *pb.Player       `json:"player"`
		Game   *model.GameState `json:"game"`
	}
	if json.Unmarshal(raw, &actual) != nil || actual.Player == nil || actual.Game == nil || actual.Player.GetLevel() != 4 {
		t.Fatal("actual tutorial profile source")
	}
	var old struct {
		Player *pb.Player       `json:"player"`
		Game   *model.GameState `json:"game"`
	}
	raw, err = os.ReadFile("../../analysis/growth-first-20261016/m1-natural-phone-20261017/89-mission3-result-profile.json")
	if err != nil || json.Unmarshal(raw, &old) != nil || old.Game.Run == nil {
		t.Fatal("original observed talent1 Start", err)
	}
	s := p6OriginalTables(t)
	for _, load := range []func(string) error{s.LoadTables, s.LoadP5Tables, s.LoadP9Tables} {
		if err = load("../../analysis/tables"); err != nil {
			t.Fatal(err)
		}
	}
	if err = s.loadP7Table("../../analysis/tables", "BuffInfExtend"); err != nil {
		t.Fatal(err)
	}
	observed := old.Game.Run.Fight.Start.GetBattlePlayer()
	actual.Player.Level = proto.Int32(observed.GetLevel())
	baseline, err := s.battlePlayer(actual.Player, actual.Game)
	if err != nil || !proto.Equal(baseline, observed) {
		t.Fatal("latest unworn equipment changed observed combat source", err)
	}
	for i, level := range []int32{22, 32} {
		actual.Player.Level = proto.Int32(level)
		expected, err := s.battlePlayer(actual.Player, actual.Game)
		if err != nil {
			t.Fatal(err)
		}
		name := []string{"000", "002"}[i]
		raw, err = os.ReadFile("../../analysis/p6/p7-natural047025-local-inputs-r1-20261017/" + name + ".SCRushFightStart.pb")
		if err != nil {
			t.Fatal(err)
		}
		start := &pb.SCRushFightStart{}
		if proto.Unmarshal(raw, start) != nil || !proto.Equal(expected, start.BattlePlayer) {
			t.Fatal("finite22/32 source differs from real production projection", level)
		}
		if !proto.Equal(baseline.Formation[1], expected.Formation[1]) || expected.Formation[1].GetLevel() != 3 || len(expected.Formation[0].GetTalentList()) != 1 || expected.Formation[0].TalentList[0] != "tal_1" {
			t.Fatal("mandatory tutorial growth lost")
		}
		series, err := s.FreezeP6RushSeriesInput(start, true, false, nil, constUtil.P6OriginalClientHash)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = p7ManualRushFamilyForSeries(series); err == nil {
			t.Fatal("new unanchored natural roster gained production authority")
		}
	}
}
