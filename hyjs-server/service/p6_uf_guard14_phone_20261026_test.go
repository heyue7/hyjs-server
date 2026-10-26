package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

const (
	p6UFGuard14Phone20261026Root       = "guild-guard14-phone-golden-20261026-r1/"
	p6UFGuard14Phone20261026BundleSHA  = "f28f6896c44c9693273160f7d18090883ac5781c1d601e13d0e73d031bcdd9fe"
	p6UFGuard14Phone20261026CoreSHA    = "062bdc3dfe35eee4fc32e0d1e6e84164bb58193b94f499131b754efe20c42798"
	p6UFGuard14Phone20261026HitsSHA    = "3825b609189ce715742b66027d82bd49cbf6668d24ce6ca10fd4f29c6556ff21"
	p6UFGuard14Phone20261026ManagerSHA = "0ed278290278509ee528308afebc46df2d978fd4abcb5fd9ba77da2a14d2b73e"
	p6UFGuard14Phone20261026RecordsSHA = "ace1e4d2e34845d045b18a09a62d385f1b49eccee513c70a75717c6ad07c7171"
)

// A new exact fourteen-case numerical fixture registration only. All historical
// denominators retain their existing branches. No engine/proof scope changes.
func p6UFGuard14Phone20261026HitCaseCount(name string) int {
	if name != p6UFGuard14Phone20261026Root+"hits.json" {
		return 0
	}
	raw, err := os.ReadFile(filepath.Join("../../analysis/p6", name))
	if err != nil || p6Digest(raw) != p6UFGuard14Phone20261026HitsSHA {
		return 0
	}
	var fixture struct {
		Cases []json.RawMessage `json:"cases"`
	}
	if json.Unmarshal(raw, &fixture) != nil || len(fixture.Cases) != 14 {
		return 0
	}
	return 14
}

func p6UFGuard14Phone20261026Bind(t *testing.T) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("../../analysis/p6", p6UFGuard14Phone20261026Root+"bundle.json"))
	if err != nil || p6Digest(raw) != p6UFGuard14Phone20261026BundleSHA {
		t.Fatal("exact guard14 phone bundle SHA", err)
	}
	var bundle struct {
		Schema     string `json:"schema"`
		Source     string `json:"sourceManifest"`
		SourceSHA  string `json:"sourceManifestSHA256"`
		Capture    string `json:"captureManifest"`
		CaptureSHA string `json:"captureManifestSHA256"`
		Count      int    `json:"caseCount"`
		Scope      string `json:"nativeScope"`
		Origin     string `json:"executionOrigin"`
		Authority  bool   `json:"authoritative"`
		WarApplied bool   `json:"warApplied"`
		Fixtures   map[string]struct {
			Path string `json:"path"`
			SHA  string `json:"sha256"`
		} `json:"fixtures"`
	}
	if json.Unmarshal(raw, &bundle) != nil || bundle.Schema != "guild-guard14-phone-fixtures-v1" || bundle.Count != 14 || bundle.Scope != "original-pve-automatic-ai" || bundle.Origin != "actual-phone-original-automatic-engine" || bundle.Authority || bundle.WarApplied || len(bundle.Fixtures) != 4 || bundle.Source != "analysis/p6/guild-guard-prospective-inputs-20261026-r1/manifest.json" || bundle.SourceSHA != "a4fd9c297a43aca5793c4be171b45abfa23dd3e0252d6830957f5935d7f26644" || bundle.Capture != "analysis/p6/guild-guard-prospective-phone-20261026-r1/manifest.json" || bundle.CaptureSHA != "324fb1bb0c36c4b1ed6d1ba19a28e45b8deec687130c77e2f321bb5a8cace14f" {
		t.Fatal("exact guard14 phone source/denominator/origin boundary")
	}
	for _, binding := range [][2]string{{bundle.Source, bundle.SourceSHA}, {bundle.Capture, bundle.CaptureSHA}} {
		raw, err := os.ReadFile(filepath.Join("../..", binding[0]))
		if err != nil || p6Digest(raw) != binding[1] {
			t.Fatal("guard14 input/capture manifest binding", binding[0], err)
		}
	}
	for kind, wantSHA := range map[string]string{"core": p6UFGuard14Phone20261026CoreSHA, "hits": p6UFGuard14Phone20261026HitsSHA, "manager": p6UFGuard14Phone20261026ManagerSHA, "records": p6UFGuard14Phone20261026RecordsSHA} {
		item := bundle.Fixtures[kind]
		if item.Path != p6UFGuard14Phone20261026Root+kind+".json" || item.SHA != wantSHA {
			t.Fatal("guard14 fixture binding", kind)
		}
		raw, err := os.ReadFile(filepath.Join("../../analysis/p6", item.Path))
		if err != nil || p6Digest(raw) != wantSHA {
			t.Fatal("guard14 fixture SHA", kind, err)
		}
	}
	if p6NativeOriginalPerHitCaseCount(p6UFGuard14Phone20261026Root+"hits.json") != 14 {
		t.Fatal("exact fourteen-case hit registration absent")
	}
}

// Actual phone original UF14 RunAutomatic on these fourteen frozen guard inputs,
// factors1/1. The typed replay sidecar proves input identity only: the runtime
// header does not claim execution of its typed callback. Thirteen existing
// numerical comparison families do not add production/business authority.
func TestP6UFGuard14Phone20261026OriginalAllLayers(t *testing.T) {
	p6UFGuard14Phone20261026Bind(t)
	core, hits := p6UFGuard14Phone20261026Root+"core.json", p6UFGuard14Phone20261026Root+"hits.json"
	t.Run("core", func(t *testing.T) {
		p6NativeAutomaticPVPOriginalTickGolden(t, core, p6UFGuard14Phone20261026CoreSHA, 14)
	})
	t.Run("actor44", func(t *testing.T) {
		p6NativeActorPropertiesFromFixture(t, "", true, core, p6UFGuard14Phone20261026CoreSHA, 14)
	})
	t.Run("lifetimes", func(t *testing.T) { p6NativeDetailsFromFixture(t, "", true, core, p6UFGuard14Phone20261026CoreSHA, 14) })
	t.Run("Buffmetadata", func(t *testing.T) {
		p6NativeBuffSourceAndValuesFromFixture(t, "", true, core, p6UFGuard14Phone20261026CoreSHA, 14)
	})
	t.Run("manager", func(t *testing.T) {
		p6NativeOriginalManagerFixture(t, p6UFGuard14Phone20261026Root+"manager.json", p6UFGuard14Phone20261026ManagerSHA, 1, 14)
	})
	t.Run("HP-AP-combo-control", func(t *testing.T) {
		p6OriginalPrimaryHPFixture(t, hits, p6UFGuard14Phone20261026HitsSHA, 1, 2, 3, 4, 5, 6)
	})
	t.Run("cast-values-order", func(t *testing.T) { p6OriginalOrderedCastFixture(t, hits, p6UFGuard14Phone20261026HitsSHA) })
	t.Run("cast-complete-collection", func(t *testing.T) { p6OriginalCastCollectionFixture(t, hits, p6UFGuard14Phone20261026HitsSHA) })
	t.Run("cast-loads-dead", func(t *testing.T) { p6OriginalCastLifecycleFixture(t, hits, p6UFGuard14Phone20261026HitsSHA) })
	t.Run("Buff-HP-AP", func(t *testing.T) {
		p6OriginalBuffHPFixture(t, hits, p6UFGuard14Phone20261026HitsSHA, 1)
		p6OriginalBuffHPFixture(t, hits, p6UFGuard14Phone20261026HitsSHA, 2)
	})
	t.Run("Buff-internal-order", func(t *testing.T) { p6OriginalOrderedBuffFixture(t, hits, p6UFGuard14Phone20261026HitsSHA) })
	t.Run("standalone-full-collection", func(t *testing.T) {
		p6OriginalStandaloneMixedCollectionFixture(t, hits, p6UFGuard14Phone20261026HitsSHA)
	})
	t.Run("final-protocol-bytes-terminal-HP", func(t *testing.T) {
		p6NativeOriginalProtocolFixtureWithTerminalHP(t, p6UFGuard14Phone20261026Root+"records.json", p6UFGuard14Phone20261026RecordsSHA, false, true, 14)
	})
}
