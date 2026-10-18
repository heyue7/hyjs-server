package service

import (
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	"os"
	"testing"
)

// One exact actual original manual-control source preserves every native value/clock.
// The original manual wait and live Auto control are replayed as real ticks.
// Every callback value/sequence and final protocol field stays strict.
func TestP6NativeManualAutoV163OriginalAutomaticCoreAndDynamic(t *testing.T) {
	const file = "native-actual-phone-rush-manual-auto-golden-v163.json"
	const sha = "d15d580cdd6dd98e5c3d8f5167a4ffb66bd85b494c9621a1c3cf1e26248d267e"
	t.Run("core", func(t *testing.T) { p6NativeAutomaticPVPOriginalTickGolden(t, file, sha, 1) })
	t.Run("actor44", func(t *testing.T) { p6NativeActorPropertiesFromFixture(t, "", true, file, sha, 1) })
	t.Run("lifetimes", func(t *testing.T) { p6NativeDetailsFromFixture(t, "", true, file, sha, 1) })
	t.Run("Buffmetadata", func(t *testing.T) { p6NativeBuffSourceAndValuesFromFixture(t, "", true, file, sha, 1) })
	t.Run("manager", func(t *testing.T) {
		p6NativeOriginalManagerFixture(t, "native-actual-phone-rush-manual-auto-manager-golden-v163.json", "cf58a60577e8957b1d73e55b5fc0dbd35ca7dcf6551629d2580b66120f30730c", 1, 1)
	})
}

func TestP6NativeManualAutoV163OriginalAutomaticExports(t *testing.T) {
	const file = "native-actual-phone-rush-manual-auto-hit-golden-v163.json"
	const sha = "42c4d163d04fd6b946cf6b96c38dee4a868ceaaff273805c1e18f0b3c8d7dfed"
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

func TestP6NativeManualAutoV163OriginalAutomaticFinalProtocolBytes(t *testing.T) {
	p6NativeOriginalProtocolRecordFixture(t, "native-actual-phone-rush-manual-auto-original-records-v163.json", "3513bbf051735e10bc7694848bd719f8a2083b94f9996a66532364ae773ca80c", false, 1)
}

func p6NativeManualAutoFixtureInput(t *testing.T, input *model.FrozenBattleInput, wireSHA, fixtureSHA string) (*model.FrozenBattleInput, bool) {
	if phone, bound := p7NumericPhoneRushNativeFixtureInput(t, input, wireSHA, fixtureSHA); bound {
		return phone, true
	}
	if phone, bound := p7NaturalPhoneRushNativeFixtureInput(t, input, wireSHA, fixtureSHA); bound {
		return phone, true
	}
	if numeric, bound := p7NumericLocalRushNativeFixtureInput(t, input, wireSHA, fixtureSHA); bound {
		return numeric, true
	}
	if party, bound := p7PartyRushNativeFixtureInput(t, input, wireSHA, fixtureSHA); bound {
		return party, true
	}
	t.Helper()
	if elite, bound := p6NativeEliteManualFixtureInput(t, input, wireSHA, fixtureSHA); bound {
		return elite, true
	}
	switch fixtureSHA {
	case "d15d580cdd6dd98e5c3d8f5167a4ffb66bd85b494c9621a1c3cf1e26248d267e", "42c4d163d04fd6b946cf6b96c38dee4a868ceaaff273805c1e18f0b3c8d7dfed", "cf58a60577e8957b1d73e55b5fc0dbd35ca7dcf6551629d2580b66120f30730c", "3513bbf051735e10bc7694848bd719f8a2083b94f9996a66532364ae773ca80c":
	default:
		return input, false
	}
	if wireSHA != "1ffc83ab125a4b9f8b27ac53e4b66d157f8458cfcfa20cff1d9691fc45a64c21" {
		t.Fatal("manual source wire changed")
	}
	raw, err := os.ReadFile("../../analysis/p6/actual-phone-rush-manual-auto-input-v161/000.frozen-input.json")
	if err != nil || p6Digest(raw) != "647f1129df6efd57f95dba3ed8fe31135d5750d76ebe592cc34e222e0e83f9d0" {
		t.Fatal("actual manual source field hash changed", err)
	}
	var source struct {
		Policy string   `json:"diagnosticAIDriverPolicy"`
		Delay  *float64 `json:"diagnosticManualAutoStartRoundTime"`
	}
	if json.Unmarshal(raw, &source) != nil || source.Policy != constUtil.P6DiagnosticManualAutoStart || source.Delay == nil {
		t.Fatal("actual source driver fields missing")
	}
	cloned, err := cloneP6CalculationInput(input)
	if err != nil {
		t.Fatal(err)
	}
	cloned.DiagnosticAIDriverPolicy = source.Policy
	cloned.DiagnosticManualAutoStartRoundTime = proto.Float64(*source.Delay)
	return cloned, true
}
