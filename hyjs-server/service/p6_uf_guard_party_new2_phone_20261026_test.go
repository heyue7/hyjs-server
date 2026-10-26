package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

const (
	p6UFGuardPartyNew2Phone20261026Root       = "guild-guard-party-new2-phone-golden-20261026-r2/"
	p6UFGuardPartyNew2Phone20261026BundleSHA  = "e38e0fd103304ded6dfb472b2bc04b51340dbd5e5d16485445e83c9c324ea14b"
	p6UFGuardPartyNew2Phone20261026CoreSHA    = "1b6002fb76102e8ef9dd8c51206ea9249035ca1e9d6ad891ee473bb1648e4be3"
	p6UFGuardPartyNew2Phone20261026HitsSHA    = "c84625ca563e91fc72c085abf817b9e170fd1177a4de65b6bffa718b0091e13b"
	p6UFGuardPartyNew2Phone20261026ManagerSHA = "11b941ba845168bd0dc518d6c9ff652bee655de3a55fbbde0af5730da3f1619c"
	p6UFGuardPartyNew2Phone20261026RecordsSHA = "87a85b91ae05a81ca21250c4f9c86db0310d06d3913d7c2df4d97d309daafce7"
)

// A new exact new two-case numerical fixture registration only. All historical
// denominators retain their existing branches. Original14 evidence is reused
// separately, not reexecuted or turned into natural business continuation.
// No engine/proof scope changes.
func p6UFGuardPartyNew2Phone20261026HitCaseCount(name string) int {
	if name != p6UFGuardPartyNew2Phone20261026Root+"hits.json" {
		return 0
	}
	raw, err := os.ReadFile(filepath.Join("../../analysis/p6", name))
	if err != nil || p6Digest(raw) != p6UFGuardPartyNew2Phone20261026HitsSHA {
		return 0
	}
	var fixture struct {
		Cases []json.RawMessage `json:"cases"`
	}
	if json.Unmarshal(raw, &fixture) != nil || len(fixture.Cases) != 2 {
		return 0
	}
	return 2
}

func p6UFGuardPartyNew2Phone20261026Bind(t *testing.T) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("../../analysis/p6", p6UFGuardPartyNew2Phone20261026Root+"bundle.json"))
	if err != nil || p6Digest(raw) != p6UFGuardPartyNew2Phone20261026BundleSHA {
		t.Fatal("exact guard party-new2 phone bundle SHA", err)
	}
	var bundle struct {
		Schema             string `json:"schema"`
		Source             string `json:"sourceManifest"`
		SourceSHA          string `json:"sourceManifestSHA256"`
		Capture            string `json:"captureManifest"`
		CaptureSHA         string `json:"captureManifestSHA256"`
		Count              int    `json:"caseCount"`
		Scope              string `json:"nativeScope"`
		Origin             string `json:"executionOrigin"`
		Authority          bool   `json:"authoritative"`
		WarApplied         bool   `json:"warApplied"`
		Parent14Reexecuted bool   `json:"parent14Reexecuted"`
		Parent14Reused     bool   `json:"parent14EvidenceReused"`
		New2Independent    bool   `json:"new2Independent"`
		TypedSourceOnly    bool   `json:"typedCallbackSourceOnly"`
		TypedExecuted      bool   `json:"typedCallbackExecuted"`
		Fixtures           map[string]struct {
			Path string `json:"path"`
			SHA  string `json:"sha256"`
		} `json:"fixtures"`
	}
	if json.Unmarshal(raw, &bundle) != nil || bundle.Schema != "guild-guard-party-new2-phone-fixtures-v1" || bundle.Count != 2 || bundle.Scope != "original-pve-automatic-ai" || bundle.Origin != "actual-phone-original-automatic-engine" || bundle.Authority || bundle.WarApplied || bundle.Parent14Reexecuted || !bundle.Parent14Reused || !bundle.New2Independent || !bundle.TypedSourceOnly || bundle.TypedExecuted || len(bundle.Fixtures) != 4 || bundle.Source != "analysis/p6/guild-guard-party16-new2-inputs-20261026-r1/manifest.json" || bundle.SourceSHA != "9c0f4ad70100c01db04e54ace672eaf1ce0f64dd6ee9fde3a9e50cc2a3e0925e" || bundle.Capture != "analysis/p6/guild-guard-party16-new2-phone-20261026-r1/manifest.json" || bundle.CaptureSHA != "fa71f55f8872feb5364f36b6bf85bff604ffb73404d98cefc91c0315781d9a79" {
		t.Fatal("exact guard party-new2 phone source/denominator/origin boundary")
	}
	for _, binding := range [][2]string{{bundle.Source, bundle.SourceSHA}, {bundle.Capture, bundle.CaptureSHA}} {
		raw, err := os.ReadFile(filepath.Join("../..", binding[0]))
		if err != nil || p6Digest(raw) != binding[1] {
			t.Fatal("guard party-new2 input/capture manifest binding", binding[0], err)
		}
	}
	for kind, wantSHA := range map[string]string{"core": p6UFGuardPartyNew2Phone20261026CoreSHA, "hits": p6UFGuardPartyNew2Phone20261026HitsSHA, "manager": p6UFGuardPartyNew2Phone20261026ManagerSHA, "records": p6UFGuardPartyNew2Phone20261026RecordsSHA} {
		item := bundle.Fixtures[kind]
		if item.Path != p6UFGuardPartyNew2Phone20261026Root+kind+".json" || item.SHA != wantSHA {
			t.Fatal("guard party-new2 fixture binding", kind)
		}
		raw, err := os.ReadFile(filepath.Join("../../analysis/p6", item.Path))
		if err != nil || p6Digest(raw) != wantSHA {
			t.Fatal("guard party-new2 fixture SHA", kind, err)
		}
	}
	if p6NativeOriginalPerHitCaseCount(p6UFGuardPartyNew2Phone20261026Root+"hits.json") != 2 {
		t.Fatal("exact new two-case hit registration absent")
	}
}

// Actual phone original UF14 RunAutomatic on these two new prospective party inputs,
// factors1/1. The typed replay sidecar proves input identity only: the runtime
// header does not claim execution of its typed callback. Thirteen existing
// numerical comparison families do not add production/business authority.
func TestP6UFGuardPartyNew2Phone20261026OriginalAllLayers(t *testing.T) {
	p6UFGuardPartyNew2Phone20261026Bind(t)
	core, hits := p6UFGuardPartyNew2Phone20261026Root+"core.json", p6UFGuardPartyNew2Phone20261026Root+"hits.json"
	t.Run("core", func(t *testing.T) {
		p6NativeAutomaticPVPOriginalTickGolden(t, core, p6UFGuardPartyNew2Phone20261026CoreSHA, 2)
	})
	t.Run("actor44", func(t *testing.T) {
		p6NativeActorPropertiesFromFixture(t, "", true, core, p6UFGuardPartyNew2Phone20261026CoreSHA, 2)
	})
	t.Run("lifetimes", func(t *testing.T) {
		p6NativeDetailsFromFixture(t, "", true, core, p6UFGuardPartyNew2Phone20261026CoreSHA, 2)
	})
	t.Run("Buffmetadata", func(t *testing.T) {
		p6NativeBuffSourceAndValuesFromFixture(t, "", true, core, p6UFGuardPartyNew2Phone20261026CoreSHA, 2)
	})
	t.Run("manager", func(t *testing.T) {
		p6NativeOriginalManagerFixture(t, p6UFGuardPartyNew2Phone20261026Root+"manager.json", p6UFGuardPartyNew2Phone20261026ManagerSHA, 1, 2)
	})
	t.Run("HP-AP-combo-control", func(t *testing.T) {
		p6OriginalPrimaryHPFixture(t, hits, p6UFGuardPartyNew2Phone20261026HitsSHA, 1, 2, 3, 4, 5, 6)
	})
	t.Run("cast-values-order", func(t *testing.T) { p6OriginalOrderedCastFixture(t, hits, p6UFGuardPartyNew2Phone20261026HitsSHA) })
	t.Run("cast-complete-collection", func(t *testing.T) { p6OriginalCastCollectionFixture(t, hits, p6UFGuardPartyNew2Phone20261026HitsSHA) })
	t.Run("cast-loads-dead", func(t *testing.T) { p6OriginalCastLifecycleFixture(t, hits, p6UFGuardPartyNew2Phone20261026HitsSHA) })
	t.Run("Buff-HP-AP", func(t *testing.T) {
		p6OriginalBuffHPFixture(t, hits, p6UFGuardPartyNew2Phone20261026HitsSHA, 1)
		p6OriginalBuffHPFixture(t, hits, p6UFGuardPartyNew2Phone20261026HitsSHA, 2)
	})
	t.Run("Buff-internal-order", func(t *testing.T) { p6OriginalOrderedBuffFixture(t, hits, p6UFGuardPartyNew2Phone20261026HitsSHA) })
	t.Run("standalone-full-collection", func(t *testing.T) {
		p6OriginalStandaloneMixedCollectionFixture(t, hits, p6UFGuardPartyNew2Phone20261026HitsSHA)
	})
	t.Run("final-protocol-bytes-terminal-HP", func(t *testing.T) {
		p6NativeOriginalProtocolFixtureWithTerminalHP(t, p6UFGuardPartyNew2Phone20261026Root+"records.json", p6UFGuardPartyNew2Phone20261026RecordsSHA, false, true, 2)
	})
}
