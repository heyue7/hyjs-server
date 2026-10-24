package service

import (
	"encoding/json"
	"math"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// The existing captured series supplies real actors/tables. The terminal is an actual
// calculation, with the exact original control record and source digest bound below.
func p7Admission63Fixture(t *testing.T) (*model.FrozenP6RushSeries, *model.CombatFinalHPOutcome, int32) {
	t.Helper()
	s, series, _ := p6ActualManualSeries(t)
	input, err := p7GenericMechanismRushInputAtAdmitted(series, 0)
	if err != nil {
		t.Fatal(err)
	}
	preview, _, err := s.ComputeP6GenericMechanismBattle(t.Context(), input, nil)
	if err != nil {
		t.Fatal(err)
	}
	input, err = p7GenericMechanismRushAutoCalculationInputAdmitted(series, preview.OriginalOperates)
	if err != nil {
		t.Fatal(err)
	}
	terminal, _, err := s.ComputeP6GenericMechanismBattle(t.Context(), input, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(preview.OriginalOperates, terminal.OriginalOperates) {
		t.Fatal("actual auto candidate changed record")
	}
	terminal.Outcome.Authoritative = true
	terminal.Outcome.CapabilityHash = constUtil.P7GenericMechanismCompatibilityProof
	terminal.Outcome.DiagnosticAIDriverPolicy = constUtil.P6DiagnosticManualAutoStart
	terminal.OriginalRecordSHA256, terminal.CalculationSourceSHA256, err = p7GenericMechanismRushManualSourceDigest(input, terminal.OriginalOperates)
	if err != nil {
		t.Fatal(err)
	}
	reported := int32(0)
	if terminal.Outcome.Winner == 0 {
		reported = 1
	}
	return series, terminal, reported
}

// Run alone with -covermode=count: the original Proof entry must execute exactly once
// despite full auto/end validation. The fixture deliberately does not call that entry.
func TestP7GenericAdmission63SingleProof(t *testing.T) {
	series, terminal, reported := p7Admission63Fixture(t)
	admission, err := newP7GenericMechanismRushAdmission(series)
	if err != nil {
		t.Fatal(err)
	}
	if err = admission.validateEnd(series, terminal, terminal.OriginalOperates, reported); err != nil {
		t.Fatal(err)
	}
	if err = admission.validateEnd(series, terminal, terminal.OriginalOperates, reported); err != nil {
		t.Fatal("same stack verification changed", err)
	}
}

func TestP7GenericAdmission63RejectsChangedInputRecordAndTerminal(t *testing.T) {
	series, terminal, reported := p7Admission63Fixture(t)
	admission, err := newP7GenericMechanismRushAdmission(series)
	if err != nil {
		t.Fatal(err)
	}
	original, _ := json.Marshal(series)
	restore := func() {
		*series = model.FrozenP6RushSeries{}
		if err := json.Unmarshal(original, series); err != nil {
			t.Fatal(err)
		}
	}
	for _, variant := range []string{"table", "actor", "start"} {
		restore()
		switch variant {
		case "table":
			series.Template.Tables["Skill"][0]["admission63Tamper"] = "changed"
		case "actor":
			series.Template.Attacker.Formation[0].Prop.Hp = proto.Float64(series.Template.Attacker.Formation[0].Prop.GetHp() + 1)
		case "start":
			series.Start.FightId = proto.String("changed-start")
		}
		if admission.validateEnd(series, terminal, terminal.OriginalOperates, reported) == nil {
			t.Fatal("changed sealed series accepted", variant)
		}
		if _, err := newP7GenericMechanismRushAdmission(series); err == nil {
			t.Fatal("strict constructor accepted changed source", variant)
		}
	}
	restore()
	var copied model.FrozenP6RushSeries
	if json.Unmarshal(original, &copied) != nil {
		t.Fatal("copy")
	}
	if admission.validateEnd(&copied, terminal, terminal.OriginalOperates, reported) == nil {
		t.Fatal("token crossed series pointer")
	}
	bad := proto.Clone(terminal.OriginalOperates).(*pb.BattleOperate)
	bad.SkillOpts[0].SkillIndex = proto.Int32((bad.SkillOpts[0].GetSkillIndex() + 1) % 3)
	if admission.validateEnd(series, terminal, bad, reported) == nil {
		t.Fatal("changed exact record accepted")
	}
	if admission.validateEnd(series, terminal, terminal.OriginalOperates, 1-reported) == nil {
		t.Fatal("changed winner accepted")
	}
	raw, _ := json.Marshal(terminal)
	var corrupt model.CombatFinalHPOutcome
	if json.Unmarshal(raw, &corrupt) != nil {
		t.Fatal("terminal copy")
	}
	corrupt.FinalActors[0].CurrentHP++
	if admission.validateEnd(series, &corrupt, terminal.OriginalOperates, reported) == nil {
		t.Fatal("changed terminal HP accepted")
	}
}

func TestP7GenericAdmission63ControlsReadOnlyAndCopiesIndependent(t *testing.T) {
	series, terminal, _ := p7Admission63Fixture(t)
	before, _ := json.Marshal(series)
	if err := p7GenericMechanismRushValidateControls(series, terminal.OriginalOperates); err != nil {
		t.Fatal(err)
	}
	for _, variant := range []string{"nil", "actor", "camp", "negative-clock", "nan-clock", "empty"} {
		record := proto.Clone(terminal.OriginalOperates).(*pb.BattleOperate)
		switch variant {
		case "nil":
			record = nil
		case "actor":
			record.SkillOpts[0].ActorUid = proto.String("unowned-actor")
		case "camp":
			record.SkillOpts[0].Camp = proto.Int32(2)
		case "negative-clock":
			record.SkillOpts[0].RoundTime = proto.Float64(-1)
		case "nan-clock":
			record.SkillOpts[0].RoundTime = proto.Float64(math.NaN())
		case "empty":
			record.SkillOpts = nil
		}
		if p7GenericMechanismRushValidateControls(series, record) == nil {
			t.Fatal("invalid control accepted", variant)
		}
	}
	input, err := p7GenericMechanismRushAutoCalculationInputAdmitted(series, terminal.OriginalOperates)
	if err != nil {
		t.Fatal(err)
	}
	input.Tables["Skill"][0]["independent-copy63"] = "changed"
	input.Attacker.Formation[0].Prop.Hp = proto.Float64(1)
	after, _ := json.Marshal(series)
	if string(before) != string(after) {
		t.Fatal("read controls or constructed copy changed frozen series")
	}
	second, err := p7GenericMechanismRushAutoCalculationInputAdmitted(series, terminal.OriginalOperates)
	if err != nil {
		t.Fatal(err)
	}
	if second.Tables["Skill"][0]["independent-copy63"] != "" || second.Attacker.Formation[0].Prop.GetHp() == 1 {
		t.Fatal("separate calculation inputs share mutable state")
	}
	// The original finite-metadata failure still wins before malformed controls.
	bad := *series.Template
	bad.Attacker = proto.Clone(series.Template.Attacker).(*pb.BattlePlayer)
	bad.Attacker.Formation[0].Prop.Hp = proto.Float64(math.NaN())
	oldErrInput, oldErr := cloneP6CalculationInput(&bad)
	if oldErrInput != nil || oldErr == nil {
		t.Fatal("finite reference gate")
	}
	view := *series
	view.Template = &bad
	newErr := p7GenericMechanismRushValidateControls(&view, nil)
	if newErr == nil || newErr.Error() != oldErr.Error() {
		t.Fatal("finite gate or failure ordering changed", oldErr, newErr)
	}
}
