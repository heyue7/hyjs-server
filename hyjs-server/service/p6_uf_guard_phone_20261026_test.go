package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

const (
	p6UFGuardPhone20261026Root       = "guild-guard-phone-golden-20261026-r2/"
	p6UFGuardPhone20261026BundleSHA  = "46e642359287d6d11aaf8071d9017a0c00e4eec874e9511320a04bb5c7dd1783"
	p6UFGuardPhone20261026CoreSHA    = "2b709e1805773a70d6e9ed67a1b9b3ea25c18c7037c3c9daf254f5d2e274b689"
	p6UFGuardPhone20261026HitsSHA    = "ccc4f8ebab9ecf89d8f5bf6e88cd52e7315581e4430ed8790005a7874b148488"
	p6UFGuardPhone20261026ManagerSHA = "8165a91ac4ea05d02cfca36ef4b7b7eaa92eeea6680958dfba4cc3807274aa11"
	p6UFGuardPhone20261026RecordsSHA = "0389d74c58c3500148eabcae5f4dace9c2a0d5efc8efe4b319158d7294d5efb3"
)

// A new exact ten-case numerical fixture registration only. All historical
// denominators retain their existing branches. No engine/proof scope changes.
func p6UFGuardPhone20261026HitCaseCount(name string) int {
	if name != p6UFGuardPhone20261026Root+"hits.json" {
		return 0
	}
	raw, err := os.ReadFile(filepath.Join("../../analysis/p6", name))
	if err != nil || p6Digest(raw) != p6UFGuardPhone20261026HitsSHA {
		return 0
	}
	var fixture struct {
		Cases []json.RawMessage `json:"cases"`
	}
	if json.Unmarshal(raw, &fixture) != nil || len(fixture.Cases) != 10 {
		return 0
	}
	return 10
}

func p6UFGuardPhone20261026Bind(t *testing.T) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("../../analysis/p6", p6UFGuardPhone20261026Root+"bundle.json"))
	if err != nil || p6Digest(raw) != p6UFGuardPhone20261026BundleSHA {
		t.Fatal("exact guard10 phone bundle SHA", err)
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
	if json.Unmarshal(raw, &bundle) != nil || bundle.Schema != "guild-guard10-phone-fixtures-v1" || bundle.Count != 10 || bundle.Scope != "original-pve-automatic-ai" || bundle.Origin != "actual-phone-original-automatic-engine" || bundle.Authority || bundle.WarApplied || len(bundle.Fixtures) != 4 || bundle.Source != "analysis/p6/guild-guard-inputs-20261025-native/manifest.json" || bundle.SourceSHA != "462e6144ab013f7cc228cc3ebcdb01bd1b76ecbc6b2bfadc8c0640579fe73bb0" || bundle.Capture != "analysis/p6/guild-guard-inputs-20261026-phone/manifest.json" || bundle.CaptureSHA != "fcef9fc77ee651cffd22ed7bdc1821473575ebf90eec5934450126c6f1df83e5" {
		t.Fatal("exact guard10 phone source/denominator/origin boundary")
	}
	for _, binding := range [][2]string{{bundle.Source, bundle.SourceSHA}, {bundle.Capture, bundle.CaptureSHA}} {
		raw, err := os.ReadFile(filepath.Join("../..", binding[0]))
		if err != nil || p6Digest(raw) != binding[1] {
			t.Fatal("guard10 input/capture manifest binding", binding[0], err)
		}
	}
	for kind, wantSHA := range map[string]string{"core": p6UFGuardPhone20261026CoreSHA, "hits": p6UFGuardPhone20261026HitsSHA, "manager": p6UFGuardPhone20261026ManagerSHA, "records": p6UFGuardPhone20261026RecordsSHA} {
		item := bundle.Fixtures[kind]
		if item.Path != p6UFGuardPhone20261026Root+kind+".json" || item.SHA != wantSHA {
			t.Fatal("guard10 fixture binding", kind)
		}
		raw, err := os.ReadFile(filepath.Join("../../analysis/p6", item.Path))
		if err != nil || p6Digest(raw) != wantSHA {
			t.Fatal("guard10 fixture SHA", kind, err)
		}
	}
	if p6NativeOriginalPerHitCaseCount(p6UFGuardPhone20261026Root+"hits.json") != 10 {
		t.Fatal("exact ten-case hit registration absent")
	}
}

// Actual phone original UF14 RunAutomatic on these ten frozen guard inputs,
// factors1/1. The typed replay sidecar proves input identity only: the runtime
// header does not claim execution of its typed callback. Thirteen existing
// numerical comparison families do not add production/business authority.
func TestP6UFGuardPhone20261026OriginalAllLayers(t *testing.T) {
	p6UFGuardPhone20261026Bind(t)
	core, hits := p6UFGuardPhone20261026Root+"core.json", p6UFGuardPhone20261026Root+"hits.json"
	t.Run("core", func(t *testing.T) { p6NativeAutomaticPVPOriginalTickGolden(t, core, p6UFGuardPhone20261026CoreSHA, 10) })
	t.Run("actor44", func(t *testing.T) {
		p6NativeActorPropertiesFromFixture(t, "", true, core, p6UFGuardPhone20261026CoreSHA, 10)
	})
	t.Run("lifetimes", func(t *testing.T) { p6NativeDetailsFromFixture(t, "", true, core, p6UFGuardPhone20261026CoreSHA, 10) })
	t.Run("Buffmetadata", func(t *testing.T) {
		p6NativeBuffSourceAndValuesFromFixture(t, "", true, core, p6UFGuardPhone20261026CoreSHA, 10)
	})
	t.Run("manager", func(t *testing.T) {
		p6NativeOriginalManagerFixture(t, p6UFGuardPhone20261026Root+"manager.json", p6UFGuardPhone20261026ManagerSHA, 1, 10)
	})
	t.Run("HP-AP-combo-control", func(t *testing.T) {
		p6OriginalPrimaryHPFixture(t, hits, p6UFGuardPhone20261026HitsSHA, 1, 2, 3, 4, 5, 6)
	})
	t.Run("cast-values-order", func(t *testing.T) { p6OriginalOrderedCastFixture(t, hits, p6UFGuardPhone20261026HitsSHA) })
	t.Run("cast-complete-collection", func(t *testing.T) { p6OriginalCastCollectionFixture(t, hits, p6UFGuardPhone20261026HitsSHA) })
	t.Run("cast-loads-dead", func(t *testing.T) { p6OriginalCastLifecycleFixture(t, hits, p6UFGuardPhone20261026HitsSHA) })
	t.Run("Buff-HP-AP", func(t *testing.T) {
		p6OriginalBuffHPFixture(t, hits, p6UFGuardPhone20261026HitsSHA, 1)
		p6OriginalBuffHPFixture(t, hits, p6UFGuardPhone20261026HitsSHA, 2)
	})
	t.Run("Buff-internal-order", func(t *testing.T) { p6OriginalOrderedBuffFixture(t, hits, p6UFGuardPhone20261026HitsSHA) })
	t.Run("standalone-full-collection", func(t *testing.T) { p6OriginalStandaloneMixedCollectionFixture(t, hits, p6UFGuardPhone20261026HitsSHA) })
	t.Run("final-protocol-bytes-terminal-HP", func(t *testing.T) {
		p6NativeOriginalProtocolFixtureWithTerminalHP(t, p6UFGuardPhone20261026Root+"records.json", p6UFGuardPhone20261026RecordsSHA, false, true, 10)
	})
}
