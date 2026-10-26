package service

import (
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	"testing"
)

// One exact actual original manual-control source preserves every native value/clock.
// The original manual wait and live Auto control are replayed as real ticks.
// Every callback value/sequence and final protocol field stays strict.
func TestP6NativeEliteManualV168OriginalAutomaticCoreAndDynamic(t *testing.T) {
	const file = "native-elite-manual-golden-v168.json"
	const sha = "630f02f5f31b3497f07414b17be7791f1639437a9bfc8d21b7e3b247bab0dd20"
	t.Run("core", func(t *testing.T) { p6NativeAutomaticPVPOriginalTickGolden(t, file, sha, 2) })
	t.Run("actor44", func(t *testing.T) { p6NativeActorPropertiesFromFixture(t, "", true, file, sha, 2) })
	t.Run("lifetimes", func(t *testing.T) { p6NativeDetailsFromFixture(t, "", true, file, sha, 2) })
	t.Run("Buffmetadata", func(t *testing.T) { p6NativeBuffSourceAndValuesFromFixture(t, "", true, file, sha, 2) })
	t.Run("manager", func(t *testing.T) {
		p6NativeOriginalManagerFixture(t, "native-elite-manual-manager-golden-v168.json", "e74c435a15b8af96dec98d02348fb608014d43858a4893ea5343ed7f7c7358ab", 1, 2)
	})
}

func TestP6NativeEliteManualV168OriginalAutomaticExports(t *testing.T) {
	const file = "native-elite-manual-hit-golden-v168.json"
	const sha = "66b485dc9a3cd09a036f7dbff62993e5f385e3b49631fedad30195900782ad20"
	t.Run("HP-AP-combo-control", func(t *testing.T) { p6OriginalPrimaryHPFixture(t, file, sha, 1, 2, 3, 4, 5, 6) })
	t.Run("cast-values-order", func(t *testing.T) { p6OriginalOrderedCastFixture(t, file, sha) })
	t.Run("cast-complete-collection", func(t *testing.T) { p6OriginalCastCollectionFixture(t, file, sha) })
	t.Run("cast-loads-dead", func(t *testing.T) { p6OriginalCastLifecycleFixture(t, file, sha) })
	t.Run("Buff-HP-AP", func(t *testing.T) {
		p6OriginalBuffHPFixture(t, file, sha, 1)
		p6OriginalBuffHPFixture(t, file, sha, 2)
	})
	t.Run("Buff-internal-order", func(t *testing.T) { p6OriginalOrderedBuffFixture(t, file, sha) })
	t.Run("standalone-full-collection", func(t *testing.T) { p6OriginalStandaloneMixedCollectionFixture(t, file, sha) })
}

func TestP6NativeEliteManualV168OriginalAutomaticFinalProtocolBytes(t *testing.T) {
	p6NativeOriginalProtocolRecordFixture(t, "native-elite-manual-original-records-v168.json", "d9d3cb72f4f3d8a84edde9c60e30bef9198fc919b9163bdbb96fa10160db27eb", false, 2)
}

// Match only immutable manual driver evidence. Production capability checks are independent.
func p6NativeEliteManualFixtureInput(t *testing.T, input *model.FrozenBattleInput, wireSHA, fixtureSHA string) (*model.FrozenBattleInput, bool) {
	t.Helper()
	switch fixtureSHA {
	case "630f02f5f31b3497f07414b17be7791f1639437a9bfc8d21b7e3b247bab0dd20", "66b485dc9a3cd09a036f7dbff62993e5f385e3b49631fedad30195900782ad20", "e74c435a15b8af96dec98d02348fb608014d43858a4893ea5343ed7f7c7358ab", "d9d3cb72f4f3d8a84edde9c60e30bef9198fc919b9163bdbb96fa10160db27eb":
	default:
		return input, false
	}
	var name, expected string
	switch wireSHA {
	case "0e128dfb2959e31dfe74073c1a447be617f511955a0e22014c6e6ce2966a2c79":
		name = "000"
		expected = "853273ccfe677c025415a09284f3d6e8f0761be93e9c97f6cd38d1d9aa7dad29"
	case "58d425d7e79ff6a321cd7451f8784676bd951d9fb618327dc58d188bff2d887a":
		name = "001"
		expected = "388c6d9467e510b28693839ceaee88d2a38d104992580f89e37eabcfee16289f"
	default:
		t.Fatal("elite manual wire identity changed")
	}
	raw, err := readTestAnalysis75("../../analysis/p6/original-elite-manual-inputs-v165/" + name + ".frozen-input.json")
	if err != nil || p6Digest(raw) != expected {
		t.Fatal("elite manual source SHA", err)
	}
	var source model.FrozenBattleInput
	if json.Unmarshal(raw, &source) != nil || source.DiagnosticAIDriverPolicy != constUtil.P6DiagnosticManualAutoStart || source.DiagnosticManualAutoStartRoundTime == nil {
		t.Fatal("elite explicit source driver missing")
	}
	cloned, err := cloneP6CalculationInput(input)
	if err != nil {
		t.Fatal(err)
	}
	cloned.DiagnosticAIDriverPolicy = source.DiagnosticAIDriverPolicy
	cloned.DiagnosticManualAutoStartRoundTime = proto.Float64(*source.DiagnosticManualAutoStartRoundTime)
	return cloned, true
}
