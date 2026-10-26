package service

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

const (
	p6UFPartyPVPPhone20261026Root       = "guild-party-pvp-phone-golden-20261026-r1/"
	p6UFPartyPVPPhone20261026BundleSHA  = "8e88c32f1eda4014cd066f498a2db488cb815dbf9ef8cc0875484011b5380d87"
	p6UFPartyPVPPhone20261026CoreSHA    = "1585ee3c7dac650a7438f62e1c44964011c288beee73189244b0ab25b70d2847"
	p6UFPartyPVPPhone20261026HitsSHA    = "df4f82ec860016329809958f58e4c5dbb020c9f2d1927f478c1f44e6438ce094"
	p6UFPartyPVPPhone20261026ManagerSHA = "ccee232ac7dd584a6f6aec594aff049209d50f54e5d6bebfe07a925fcbcaedb9"
	p6UFPartyPVPPhone20261026RecordsSHA = "6d2b9a97f1acfff3e3f4601fc81a18c629ac10665cf3fd642eaec62b008018ea"
)

// A new exact independent one-case numerical fixture registration only. All historical
// denominators retain their existing branches. Original14 evidence is reused
// separately, not reexecuted or turned into natural business continuation.
// No engine/proof scope changes.
func p6UFPartyPVPPhone20261026HitCaseCount(name string) int {
	if name != p6UFPartyPVPPhone20261026Root+"hits.json" {
		return 0
	}
	raw, err := readTestAnalysis75(filepath.Join("../../analysis/p6", name))
	if err != nil || p6Digest(raw) != p6UFPartyPVPPhone20261026HitsSHA {
		return 0
	}
	var fixture struct {
		Cases []json.RawMessage `json:"cases"`
	}
	if json.Unmarshal(raw, &fixture) != nil || len(fixture.Cases) != 1 {
		return 0
	}
	return 1
}

func p6UFPartyPVPPhone20261026Bind(t *testing.T) {
	t.Helper()
	raw, err := readTestAnalysis75(filepath.Join("../../analysis/p6", p6UFPartyPVPPhone20261026Root+"bundle.json"))
	if err != nil || p6Digest(raw) != p6UFPartyPVPPhone20261026BundleSHA {
		t.Fatal("exact party PK1 phone bundle SHA", err)
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
		NewPKIndependent   bool   `json:"newPKIndependent"`
		Prior2Reexecuted   bool   `json:"new2Reexecuted"`
		TypedSourceOnly    bool   `json:"typedCallbackSourceOnly"`
		TypedExecuted      bool   `json:"typedCallbackExecuted"`
		Fixtures           map[string]struct {
			Path string `json:"path"`
			SHA  string `json:"sha256"`
		} `json:"fixtures"`
	}
	if json.Unmarshal(raw, &bundle) != nil || bundle.Schema != "guild-party-pvp-phone-fixtures-v1" || bundle.Count != 1 || bundle.Scope != "original-pvp-automatic-ai" || bundle.Origin != "actual-phone-original-automatic-engine" || bundle.Authority || bundle.WarApplied || bundle.Parent14Reexecuted || !bundle.NewPKIndependent || bundle.Prior2Reexecuted || !bundle.TypedSourceOnly || bundle.TypedExecuted || len(bundle.Fixtures) != 4 || bundle.Source != "analysis/p6/guild-party-pvp-input-20261026-r1/manifest.json" || bundle.SourceSHA != "5f973f3880d05a4b6e732c80b3e66d334f793fba1a09639f7e4c947525bd0eb6" || bundle.Capture != "analysis/p6/guild-party-pvp-phone-20261026-r1/manifest.json" || bundle.CaptureSHA != "0cef1440f9afefd50a96bffac7f30ae28389bc84671cdfda016a09746450f103" {
		t.Fatal("exact party PK1 phone source/denominator/origin boundary")
	}
	for _, binding := range [][2]string{{bundle.Source, bundle.SourceSHA}, {bundle.Capture, bundle.CaptureSHA}} {
		raw, err := readTestAnalysis75(filepath.Join("../..", binding[0]))
		if err != nil || p6Digest(raw) != binding[1] {
			t.Fatal("party PK1 input/capture manifest binding", binding[0], err)
		}
	}
	for kind, wantSHA := range map[string]string{"core": p6UFPartyPVPPhone20261026CoreSHA, "hits": p6UFPartyPVPPhone20261026HitsSHA, "manager": p6UFPartyPVPPhone20261026ManagerSHA, "records": p6UFPartyPVPPhone20261026RecordsSHA} {
		item := bundle.Fixtures[kind]
		if item.Path != p6UFPartyPVPPhone20261026Root+kind+".json" || item.SHA != wantSHA {
			t.Fatal("party PK1 fixture binding", kind)
		}
		raw, err := readTestAnalysis75(filepath.Join("../../analysis/p6", item.Path))
		if err != nil || p6Digest(raw) != wantSHA {
			t.Fatal("party PK1 fixture SHA", kind, err)
		}
	}
	if p6NativeOriginalPerHitCaseCount(p6UFPartyPVPPhone20261026Root+"hits.json") != 1 {
		t.Fatal("exact independent one-case hit registration absent")
	}
}

// Actual phone original UF14 RunAutomatic on these one new prospective Player/Player4slot source including initialdead partners,
// factors1/1. The typed replay sidecar proves input identity only: the runtime
// header does not claim execution of its typed callback. Thirteen existing
// numerical comparison families do not add production/business authority.
func TestP6UFPartyPVPPhone20261026OriginalAllLayers(t *testing.T) {
	p6UFPartyPVPPhone20261026Bind(t)
	core, hits := p6UFPartyPVPPhone20261026Root+"core.json", p6UFPartyPVPPhone20261026Root+"hits.json"
	t.Run("core", func(t *testing.T) {
		p6NativeAutomaticPVPOriginalTickGolden(t, core, p6UFPartyPVPPhone20261026CoreSHA, 1)
	})
	t.Run("actor44", func(t *testing.T) {
		p6NativeActorPropertiesFromFixture(t, "", true, core, p6UFPartyPVPPhone20261026CoreSHA, 1)
	})
	t.Run("lifetimes", func(t *testing.T) {
		p6NativeDetailsFromFixture(t, "", true, core, p6UFPartyPVPPhone20261026CoreSHA, 1)
	})
	t.Run("Buffmetadata", func(t *testing.T) {
		p6NativeBuffSourceAndValuesFromFixture(t, "", true, core, p6UFPartyPVPPhone20261026CoreSHA, 1)
	})
	t.Run("manager", func(t *testing.T) {
		p6NativeOriginalManagerFixture(t, p6UFPartyPVPPhone20261026Root+"manager.json", p6UFPartyPVPPhone20261026ManagerSHA, 1, 1)
	})
	t.Run("HP-AP-combo-control", func(t *testing.T) {
		p6OriginalPrimaryHPFixture(t, hits, p6UFPartyPVPPhone20261026HitsSHA, 1, 2, 3, 4, 5, 6)
	})
	t.Run("cast-values-order", func(t *testing.T) { p6OriginalOrderedCastFixture(t, hits, p6UFPartyPVPPhone20261026HitsSHA) })
	t.Run("cast-complete-collection", func(t *testing.T) { p6OriginalCastCollectionFixture(t, hits, p6UFPartyPVPPhone20261026HitsSHA) })
	t.Run("cast-loads-dead", func(t *testing.T) { p6OriginalCastLifecycleFixture(t, hits, p6UFPartyPVPPhone20261026HitsSHA) })
	t.Run("Buff-HP-AP", func(t *testing.T) {
		p6OriginalBuffHPFixture(t, hits, p6UFPartyPVPPhone20261026HitsSHA, 1)
		p6OriginalBuffHPFixture(t, hits, p6UFPartyPVPPhone20261026HitsSHA, 2)
	})
	t.Run("Buff-internal-order", func(t *testing.T) { p6OriginalOrderedBuffFixture(t, hits, p6UFPartyPVPPhone20261026HitsSHA) })
	t.Run("standalone-full-collection", func(t *testing.T) {
		p6OriginalStandaloneMixedCollectionFixture(t, hits, p6UFPartyPVPPhone20261026HitsSHA)
	})
	t.Run("final-protocol-bytes-terminal-HP", func(t *testing.T) {
		p6NativeOriginalProtocolFixtureWithTerminalHP(t, p6UFPartyPVPPhone20261026Root+"records.json", p6UFPartyPVPPhone20261026RecordsSHA, false, true, 1)
	})
}
