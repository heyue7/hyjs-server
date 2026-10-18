package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// p6NativeDiagnosticFixture admits a new captured batch only for diagnostic
// runs. Permanent golden tests retain their pinned historical bytes. Both
// paths verify the original output manifest before loading any frozen wire.
func p6NativeDiagnosticFixture(t *testing.T, file, expectedSHA, nextEnv string, strict bool) ([]byte, string) {
	t.Helper()
	path := filepath.Join("../../analysis/p6", file)
	next := !strict && os.Getenv(nextEnv) != ""
	if next {
		path = os.Getenv(nextEnv)
	}
	raw, err := os.ReadFile(path)
	if err != nil || !next && p6Digest(raw) != expectedSHA {
		t.Fatal("original diagnostic fixture hash", err)
	}
	var provenance struct {
		Manifest   string `json:"sourceManifest"`
		SHA        string `json:"sourceManifestSHA256"`
		WireSource string `json:"wireSourceManifest"`
	}
	if err = json.Unmarshal(raw, &provenance); err != nil || provenance.Manifest == "" || provenance.SHA == "" || provenance.WireSource == "" {
		t.Fatal("original diagnostic provenance missing", err)
	}
	manifestPath := provenance.Manifest
	if !filepath.IsAbs(manifestPath) {
		manifestPath = filepath.Join("../..", manifestPath)
	}
	manifest, err := os.ReadFile(manifestPath)
	if err != nil || p6Digest(manifest) != provenance.SHA {
		t.Fatal("original native output manifest hash", err)
	}
	wireRoot := p6NativeOriginalWireMirrorRoot(filepath.Dir(provenance.WireSource))
	return raw, wireRoot
}

// Preserve the original analysis domain when replaying an absolute historical
// source inside a fixed candidate. Plunder evidence belongs to analysis/p11;
// every loaded manifest/wire still has its existing exact SHA verification.
func p6NativeOriginalWireMirrorRoot(source string) string {
	if filepath.IsAbs(source) {
		if _, tail, ok := strings.Cut(filepath.ToSlash(filepath.Clean(source)), "/analysis/"); ok {
			return filepath.Join("../..", "analysis", filepath.FromSlash(tail))
		}
		return filepath.Join("../../analysis/p6", filepath.Base(source))
	}
	return filepath.Join("../..", source)
}

func TestP6NativeOriginalWireMirrorRetainsSourceDomain(t *testing.T) {
	for source, want := range map[string]string{
		"/historical/work/analysis/p11/plunder": "../../analysis/p11/plunder",
		"/historical/work/analysis/p6/golden":   "../../analysis/p6/golden",
		"analysis/p11/plunder":                  "../../analysis/p11/plunder",
		"/historical/legacy":                    "../../analysis/p6/legacy",
	} {
		if got := p6NativeOriginalWireMirrorRoot(source); got != want {
			t.Fatalf("source domain changed: %s -> %s want %s", source, got, want)
		}
	}
}
