package service

import (
	"encoding/hex"
	"encoding/json"
	"hyjs-server/constUtil"
	"math"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestOriginalLifestealConvertsPercentBeforeDamage(t *testing.T) {
	const damage = 27132.573829965255
	got := originalLifestealAmount(damage, 1, 0, 0, 0)
	if math.Float64bits(got) != math.Float64bits(damage) {
		t.Fatalf("100 percent lifesteal = %.17g, want %.17g", got, damage)
	}
	hp := 597482.8388156233 + got
	if math.Float64bits(hp) != math.Float64bits(624615.4126455885) {
		t.Fatalf("resulting HP = %.17g", hp)
	}
}

// Reuse the immutable original547 manual End to guard the public settlement
// fallback after the shared math change. The separate numerical case above
// preserves the lost diagnostic's HP boundary; this fixture is a different fight.
func TestOriginalLifestealStrictOriginalEndRegression(t *testing.T) {
	s, series, ends := p7LiveRecordedOriginal34(t)
	end := ends[0]
	out, err := p7GenericMechanismRushComputeEnd(t.Context(), s, series, end.GetOptRecord(), end.GetResult())
	if err != nil {
		t.Fatal("original547 manual End", err)
	}
	want, err := proto.MarshalOptions{Deterministic: true}.Marshal(end.GetOptRecord())
	if err != nil {
		t.Fatal(err)
	}
	actual, err := proto.MarshalOptions{Deterministic: true}.Marshal(out.OriginalOperates)
	if err != nil || string(actual) != string(want) {
		t.Fatal("complete original operation protobuf changed", err)
	}
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	var cold model.CombatFinalHPOutcome
	if err = json.Unmarshal(raw, &cold); err != nil {
		t.Fatal(err)
	}
	if err = p7GenericMechanismRushValidateEnd(series, &cold, end.GetOptRecord(), end.GetResult()); err != nil {
		t.Fatal("cold original receipt", err)
	}
	for _, target := range []float64{math.Inf(-1), math.Inf(1)} {
		changed := proto.Clone(end).(*pb.CSRushFightEnd)
		hp := math.Nextafter(changed.OptRecord.GetAttackerTotalLeftHp(), target)
		changed.OptRecord.AttackerTotalLeftHp = &hp
		if err = p7GenericMechanismRushValidateEnd(series, &cold, changed.GetOptRecord(), changed.GetResult()); err == nil {
			t.Fatal("cold receipt accepted one-ULP terminal HP change")
		}
		if _, err = p7GenericMechanismRushComputeEnd(t.Context(), s, series, changed.GetOptRecord(), changed.GetResult()); err == nil {
			t.Fatal("settlement accepted one-ULP terminal HP change")
		}
	}
	wrongResult := int32(1) - end.GetResult()
	if _, err = p7GenericMechanismRushComputeEnd(t.Context(), s, series, end.GetOptRecord(), wrongResult); err == nil {
		t.Fatal("settlement accepted wrong result with authentic original operations")
	}
}

// This new isolated original547 source includes temporary 7.5% lifesteal stacks,
// their native unload residue, and a fractional terminal HP below its maximum.
// It is distinct from the unavailable historical rejected production End.
func TestOriginalLifestealFractionResidueCompleteOriginalEnd(t *testing.T) {
	var fixture struct{ StartHex, EndHex, StartSHA256, EndSHA256 string }
	raw, err := readTestAnalysis75("testdata/p7_lifesteal_original314.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	decode := func(encoded, digest string, message proto.Message) {
		t.Helper()
		body, decodeErr := hex.DecodeString(encoded)
		if decodeErr != nil || p6Digest(body) != digest {
			t.Fatal("original547 fixture digest", decodeErr)
		}
		if decodeErr = proto.Unmarshal(body, message); decodeErr != nil {
			t.Fatal(decodeErr)
		}
	}
	start, end := &pb.SCRushFightStart{}, &pb.CSRushFightEnd{}
	decode(fixture.StartHex, fixture.StartSHA256, start)
	decode(fixture.EndHex, fixture.EndSHA256, end)
	s, _, _ := p7LiveRecordedOriginal34(t)
	series, err := s.FreezeP6RushSeriesInput(start, true, false, nil, constUtil.P6OriginalClientHash)
	if err != nil {
		t.Fatal(err)
	}
	out, err := p7GenericMechanismRushComputeEnd(t.Context(), s, series, end.GetOptRecord(), end.GetResult())
	if err != nil {
		t.Fatal("complete original547 lifesteal End", err)
	}
	if !proto.Equal(out.OriginalOperates, end.GetOptRecord()) {
		t.Fatal("complete original547 operation protobuf changed")
	}
	if out.OriginalOperates.GetAttackerTotalLeftHp() != 344170.1701108697 || out.OriginalOperates.GetFinalRoundNum() != 7 {
		t.Fatal("original547 terminal changed")
	}
	body, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	var cold model.CombatFinalHPOutcome
	if err = json.Unmarshal(body, &cold); err != nil {
		t.Fatal(err)
	}
	if err = p7GenericMechanismRushValidateEnd(series, &cold, end.GetOptRecord(), end.GetResult()); err != nil {
		t.Fatal("cold original547 receipt", err)
	}
	changed := proto.Clone(end).(*pb.CSRushFightEnd)
	changed.OptRecord.AttackerTotalLeftHp = proto.Float64(math.Nextafter(changed.OptRecord.GetAttackerTotalLeftHp(), math.Inf(1)))
	if err = p7GenericMechanismRushValidateEnd(series, &cold, changed.GetOptRecord(), changed.GetResult()); err == nil {
		t.Fatal("one ULP forged End accepted")
	}
}
