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

type p7NumericPhoneNativeFixture struct {
	Path string `json:"path"`
	SHA  string `json:"sha256"`
}

type p7NumericPhoneNativeBundle struct {
	Schema     string                                 `json:"schema"`
	Source     string                                 `json:"sourceManifest"`
	SourceSHA  string                                 `json:"sourceManifestSHA256"`
	Capture    string                                 `json:"captureManifest"`
	CaptureSHA string                                 `json:"captureManifestSHA256"`
	Count      int                                    `json:"caseCount"`
	Scope      string                                 `json:"nativeScope"`
	Origin     string                                 `json:"executionOrigin"`
	Authority  bool                                   `json:"businessAuthority"`
	Fixtures   map[string]p7NumericPhoneNativeFixture `json:"fixtures"`
}

func p7NumericPhoneNativePath(t *testing.T, relative string) string {
	t.Helper()
	clean := filepath.Clean(relative)
	if filepath.IsAbs(clean) || !strings.HasPrefix(filepath.ToSlash(clean), "analysis/p6/") {
		t.Fatal("native fixture must remain under project analysis", relative)
	}
	return filepath.Join("../..", clean)
}

func p7NumericPhoneRushNativeBundle(t *testing.T) *p7NumericPhoneNativeBundle {
	t.Helper()
	file := os.Getenv("HYJS_P7_NUMERIC_PHONE_BUNDLE")
	if file == "" {
		return nil
	}
	raw, err := readTestAnalysis75(file)
	if err != nil || len(os.Getenv("HYJS_P7_NUMERIC_PHONE_BUNDLE_SHA256")) != 64 || p6Digest(raw) != os.Getenv("HYJS_P7_NUMERIC_PHONE_BUNDLE_SHA256") {
		t.Fatal("explicit P7 native bundle SHA", err)
	}
	var bundle p7NumericPhoneNativeBundle
	if json.Unmarshal(raw, &bundle) != nil || bundle.Schema != "p7-numeric047025-android-native-matrix-v1" || bundle.Source != "analysis/p6/p7-numeric047025-local-inputs-r1-20261017/manifest.json" || bundle.SourceSHA != "7e74d64e647ee3bc0de9f48afd10cea8bae14ea14461652342d06b88ab8eceac" || bundle.Count != 4 || bundle.Scope != "original-pve-manual-then-auto-ai" || bundle.Authority || bundle.Origin != "android-original-engine-native-batch" || bundle.CaptureSHA != "d48077b4f44aaea01bafe09ca004d0aa8575e15b867e9d4d35c7ffa4aaaa08ff" || len(bundle.Fixtures) != 4 {
		t.Fatal("bounded original party bundle identity/denominator/authority")
	}
	for _, binding := range [][2]string{{bundle.Source, bundle.SourceSHA}, {bundle.Capture, bundle.CaptureSHA}} {
		data, err := readTestAnalysis75(p7NumericPhoneNativePath(t, binding[0]))
		if err != nil || len(binding[1]) != 64 || p6Digest(data) != binding[1] {
			t.Fatal("native source/capture manifest SHA", binding[0], err)
		}
	}
	for _, kind := range []string{"core", "hits", "manager", "records"} {
		fixture := bundle.Fixtures[kind]
		if fixture.Path == "" || len(fixture.SHA) != 64 {
			t.Fatal("all original matrix layers are required", kind)
		}
		p7NumericPhoneNativePath(t, filepath.Join("analysis/p6", fixture.Path))
	}
	return &bundle
}

// p7NumericPhoneRushNativeFixtureInput 只给显式SHA绑定的诊断fixtures选择原PVE手动时钟。
// 根集成在p6NativeManualAutoFixtureInput首部调用；生产权限入口不引用此测试helper。
func p7NumericPhoneRushNativeFixtureInput(t *testing.T, input *model.FrozenBattleInput, wireSHA, fixtureSHA string) (*model.FrozenBattleInput, bool) {
	t.Helper()
	bundle := p7NumericPhoneRushNativeBundle(t)
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
	raw, err := readTestAnalysis75(p7NumericPhoneNativePath(t, bundle.Source))
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
		typed, err := readTestAnalysis75(filepath.Join(filepath.Dir(p7NumericPhoneNativePath(t, bundle.Source)), row.Typed))
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

// p7NumericPhoneRushNativeHitCaseCount 仅给显式bundle内已校验命中的新fixture声明四例分母。
// 旧fixture继续使用原helper的既有固定分母。
func p7NumericPhoneRushNativeHitCaseCount(name string) int {
	raw, err := readTestAnalysis75(os.Getenv("HYJS_P7_NUMERIC_PHONE_BUNDLE"))
	if err != nil || p6Digest(raw) != os.Getenv("HYJS_P7_NUMERIC_PHONE_BUNDLE_SHA256") {
		return 0
	}
	var bundle p7NumericPhoneNativeBundle
	if json.Unmarshal(raw, &bundle) != nil || bundle.Schema != "p7-numeric047025-android-native-matrix-v1" || bundle.Count != 4 || bundle.Authority || bundle.Origin != "android-original-engine-native-batch" || bundle.CaptureSHA != "d48077b4f44aaea01bafe09ca004d0aa8575e15b867e9d4d35c7ffa4aaaa08ff" || bundle.SourceSHA != "7e74d64e647ee3bc0de9f48afd10cea8bae14ea14461652342d06b88ab8eceac" {
		return 0
	}
	hits := bundle.Fixtures["hits"]
	if hits.Path != name || filepath.IsAbs(name) || strings.HasPrefix(filepath.ToSlash(filepath.Clean(name)), "../") {
		return 0
	}
	data, err := readTestAnalysis75(filepath.Join("../../analysis/p6", name))
	if err != nil || len(hits.SHA) != 64 || p6Digest(data) != hits.SHA {
		return 0
	}
	return 4
}

// The unchanged 13 comparison families do not compare platform stack strings.
// Full-schema Android/CoreCLR FAIL remains an independent retained report.
func TestP7NumericPhoneRushNativeAllLayers(t *testing.T) {
	bundle := p7NumericPhoneRushNativeBundle(t)
	if bundle == nil {
		t.Skip("PENDING: explicit four-case original typed9 captures and SHA-bound bundle required")
	}
	// Verify every derived fixture once before invoking the unchanged highest helpers.
	for kind, fixture := range bundle.Fixtures {
		raw, err := readTestAnalysis75(p7NumericPhoneNativePath(t, filepath.Join("analysis/p6", fixture.Path)))
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
