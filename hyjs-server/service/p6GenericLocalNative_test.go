package service

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

type p6GenericLocalNativeBundleData struct {
	Schema     string `json:"schema"`
	Source     string `json:"sourceManifest"`
	SourceSHA  string `json:"sourceManifestSHA256"`
	Capture    string `json:"captureManifest"`
	CaptureSHA string `json:"captureManifestSHA256"`
	Count      int    `json:"caseCount"`
	Scope      string `json:"nativeScope"`
	Origin     string `json:"executionOrigin"`
	Authority  bool   `json:"businessAuthority"`
	Fixtures   map[string]struct {
		Path string `json:"path"`
		SHA  string `json:"sha256"`
	} `json:"fixtures"`
}

func p6GenericLocalNativeBundle(t *testing.T) *p6GenericLocalNativeBundleData {
	t.Helper()
	path := os.Getenv("HYJS_P6_GENERIC_LOCAL_BUNDLE")
	if path == "" {
		return nil
	}
	raw, err := os.ReadFile(path)
	if err != nil || p6Digest(raw) != os.Getenv("HYJS_P6_GENERIC_LOCAL_BUNDLE_SHA256") || len(os.Getenv("HYJS_P6_GENERIC_LOCAL_BUNDLE_SHA256")) != 64 {
		t.Fatal("fixed four local original bundle SHA", err)
	}
	var bundle p6GenericLocalNativeBundleData
	if json.Unmarshal(raw, &bundle) != nil || bundle.Schema != "generic-four-local-matrix-v1" || bundle.Source != "analysis/p6/generic-four-source-20261017/manifest.json" || bundle.SourceSHA != "e9e999cffb68071aced70168e48a8a4ccc9fa3c81585add6b3017698cd5802f5" || bundle.Count != 4 || bundle.Scope != "original-pvp-automatic-ai" || bundle.Authority || bundle.Origin != "local-coreclr-original-engine-not-phone-capture" || len(bundle.Fixtures) != 4 {
		t.Fatal("fixed original four-case identity/denominator/origin")
	}
	for _, binding := range [][2]string{{bundle.Source, bundle.SourceSHA}, {bundle.Capture, bundle.CaptureSHA}} {
		raw, err := os.ReadFile(p7NumericLocalNativePath(t, binding[0]))
		if err != nil || p6Digest(raw) != binding[1] || len(binding[1]) != 64 {
			t.Fatal("original four-case source binding", binding[0], err)
		}
	}
	for _, kind := range []string{"core", "hits", "manager", "records"} {
		fixture := bundle.Fixtures[kind]
		raw, err := os.ReadFile(p7NumericLocalNativePath(t, filepath.Join("analysis/p6", fixture.Path)))
		if err != nil || p6Digest(raw) != fixture.SHA || len(fixture.SHA) != 64 {
			t.Fatal("original four-case fixture binding", kind, err)
		}
	}
	return &bundle
}

// Only this exact explicit local bundle can declare four original hit cases.
func p6GenericLocalNativeHitCaseCount(name string) int {
	raw, err := os.ReadFile(os.Getenv("HYJS_P6_GENERIC_LOCAL_BUNDLE"))
	if err != nil || p6Digest(raw) != os.Getenv("HYJS_P6_GENERIC_LOCAL_BUNDLE_SHA256") {
		return 0
	}
	var bundle p6GenericLocalNativeBundleData
	if json.Unmarshal(raw, &bundle) != nil || bundle.Schema != "generic-four-local-matrix-v1" || bundle.SourceSHA != "e9e999cffb68071aced70168e48a8a4ccc9fa3c81585add6b3017698cd5802f5" || bundle.Count != 4 || bundle.Authority || bundle.Origin != "local-coreclr-original-engine-not-phone-capture" {
		return 0
	}
	hits := bundle.Fixtures["hits"]
	if hits.Path != name || filepath.IsAbs(name) || strings.HasPrefix(filepath.ToSlash(filepath.Clean(name)), "../") {
		return 0
	}
	data, err := os.ReadFile(filepath.Join("../../analysis/p6", name))
	if err != nil || p6Digest(data) != hits.SHA || len(hits.SHA) != 64 {
		return 0
	}
	return 4
}

func TestP6GenericLocalOriginalAllLayers(t *testing.T) {
	bundle := p6GenericLocalNativeBundle(t)
	if bundle == nil {
		t.Skip("explicit four local original observations only; no global authority")
	}
	core, hits, manager, records := bundle.Fixtures["core"], bundle.Fixtures["hits"], bundle.Fixtures["manager"], bundle.Fixtures["records"]
	if p6NativeOriginalPerHitCaseCount(hits.Path) != 4 {
		t.Fatal("fixed four-case hit registration absent")
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
	t.Run("zero-tolerance-terminal", func(t *testing.T) { p6GenericLocalStrictTerminal(t, records.Path, records.SHA) })
}

func p6GenericLocalStrictTerminal(t *testing.T, file, digest string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("../../analysis/p6", file))
	if err != nil || p6Digest(raw) != digest {
		t.Fatal("original terminal record SHA", err)
	}
	var source struct {
		Cases []struct {
			Name   string                     `json:"case"`
			Wire   string                     `json:"wire"`
			SHA    string                     `json:"wireSHA256"`
			Win    bool                       `json:"attackerWon"`
			Final  []model.CombatFinalActorHP `json:"originalFinalActorHP"`
			Record struct {
				Binary string `json:"protobufBase64"`
				SHA    string `json:"protobufSHA256"`
			} `json:"originalFinalRecord"`
		} `json:"cases"`
	}
	if json.Unmarshal(raw, &source) != nil || len(source.Cases) != 4 {
		t.Fatal("all four original strict terminals required")
	}
	s := p6OriginalTables(t)
	for _, row := range source.Cases {
		t.Run(row.Name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("../..", row.Wire))
			if err != nil || p6Digest(raw) != row.SHA {
				t.Fatal("original wire source SHA", err)
			}
			wire := &pb.SCFightPlayback{}
			if err = proto.Unmarshal(raw, wire); err != nil {
				t.Fatal(err)
			}
			input, err := s.FreezeBattleInput(model.FrozenBattleInput{Attacker: wire.Attacker, Defender: wire.Defender1, Seed: wire.GetRandSeed(), MaxRound: wire.GetMaxRound(), BattleType: wire.GetBattleType(), AttackerDamageFactor: wire.AttackerFactor, DefenderDamageFactor: wire.DefenderFactor})
			if err != nil {
				t.Fatal(err)
			}
			got, qualification, err := s.ComputeP6GenericMechanismBattle(t.Context(), input, nil)
			if err != nil || got.Outcome.Authoritative || qualification.NativeCertified || (got.Outcome.Winner == 0) != row.Win || len(got.FinalActors) != len(row.Final) {
				t.Fatal("full original final scope", err)
			}
			want, err := base64.StdEncoding.Strict().DecodeString(row.Record.Binary)
			if err != nil || p6Digest(want) != row.Record.SHA {
				t.Fatal("original final bytes SHA", err)
			}
			encoded, err := proto.MarshalOptions{Deterministic: true}.Marshal(got.OriginalOperates)
			if err != nil || !bytes.Equal(want, encoded) {
				t.Fatal("original GetRecordOperates byte drift", err)
			}
			for i, original := range row.Final {
				actual := got.FinalActors[i]
				if actual.UID != original.UID || actual.Camp != original.Camp || actual.HeroID != original.HeroID || actual.State != original.State || actual.CurrentHP != original.CurrentHP || actual.MaxHP != original.MaxHP {
					t.Fatal("zero-tolerance original identity/state/HP drift", original, actual)
				}
			}
		})
	}
}
