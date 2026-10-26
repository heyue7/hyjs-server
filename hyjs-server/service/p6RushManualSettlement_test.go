package service

import (
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"testing"
)

func p6ActualManualSeries(t *testing.T) (*GameService, *model.FrozenP6RushSeries, *pb.CSRushFightEnd) {
	t.Helper()
	input := p6ActualManualAutoInput(t)
	s := &GameService{Tables: input.Tables}
	folder := "../../analysis/p6/actual-phone-rush-manual-auto-input-v161/"
	raw, err := readTestAnalysis75(folder + "000.SCRushFightStart.pb")
	if err != nil {
		t.Fatal(err)
	}
	start := &pb.SCRushFightStart{}
	if err = proto.Unmarshal(raw, start); err != nil {
		t.Fatal(err)
	}
	series, err := s.FreezeP6RushSeriesInput(start, true, false, nil, input.ClientVersion)
	if err != nil {
		t.Fatal(err)
	}
	raw, err = readTestAnalysis75(folder + "000.CSRushFightEnd.pb")
	if err != nil {
		t.Fatal(err)
	}
	end := &pb.CSRushFightEnd{}
	if err = proto.Unmarshal(raw, end); err != nil {
		t.Fatal(err)
	}
	return s, series, end
}

func TestP6RushManualActualCompleteRecordAndUnknownControls(t *testing.T) {
	s, series, end := p6ActualManualSeries(t)
	before, _ := json.Marshal(series)
	input, err := p6RushManualCalculationInput(series, end.GetOptRecord())
	if err != nil {
		t.Fatal(err)
	}
	if *input.DiagnosticManualAutoStartRoundTime != 30.100000000000158 {
		t.Fatal("control clock changed")
	}
	// Always run the independent engine without supplied playback instructions.
	out, err := s.ComputeOriginalP6ProtocolFinalHP(t.Context(), input, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(out.OriginalOperates, end.GetOptRecord()) {
		t.Fatal("actual complete live source mismatch")
	}
	for _, variant := range []string{"nil", "first-enemy", "first-slot1", "first-ult", "later-round", "negative-clock", "after-deadline", "transform", "wrong-actor"} {
		record := proto.Clone(end.GetOptRecord()).(*pb.BattleOperate)
		switch variant {
		case "nil":
			record = nil
		case "first-enemy":
			record.SkillOpts[0].Camp = proto.Int32(1)
		case "first-slot1":
			record.SkillOpts[0].SkillIndex = proto.Int32(1)
		case "first-ult":
			record.SkillOpts[0].SkillIndex = proto.Int32(2)
		case "later-round":
			record.SkillOpts[0].RoundNum = proto.Int32(2)
		case "negative-clock":
			record.SkillOpts[0].RoundTime = proto.Float64(-1)
		case "after-deadline":
			record.SkillOpts[0].RoundTime = proto.Float64(111)
		case "transform":
			record.AttackerTransOpt = &pb.BattleTransOperate{}
		case "wrong-actor":
			record.SkillOpts[0].ActorUid = proto.String("not-this-source")
		}
		if _, err = p6RushManualCalculationInput(series, record); err == nil {
			t.Fatal("unsupported control accepted", variant)
		}
	}
	after, _ := json.Marshal(series)
	if string(before) != string(after) {
		t.Fatal("source mutated")
	}
}

func TestP6RushManualCandidateCertificateExactActualEnd(t *testing.T) {
	s, series, end := p6ActualManualSeries(t)
	out, err := s.ComputeP6RushManualAutoSettlement(t.Context(), series, end.GetOptRecord(), end.GetResult())
	if err != nil {
		t.Fatal(err)
	}
	if !out.Outcome.Authoritative || ValidateP6RushManualAutoEnd(series, out, end.GetOptRecord(), end.GetResult()) != nil {
		t.Fatal("independent certificate not bound")
	}
	for _, variant := range []string{"clock", "enemy-clock", "hp", "round", "target", "reported-result", "skill-order"} {
		record := proto.Clone(end.GetOptRecord()).(*pb.BattleOperate)
		reported := end.GetResult()
		switch variant {
		case "clock":
			record.SkillOpts[0].RoundTime = proto.Float64(record.SkillOpts[0].GetRoundTime() + 0.1)
		case "enemy-clock":
			for _, opt := range record.SkillOpts {
				if opt.GetCamp() == 1 {
					opt.RoundTime = proto.Float64(opt.GetRoundTime() + 0.1)
					break
				}
			}
		case "hp":
			record.AttackerTotalLeftHp = proto.Float64(record.GetAttackerTotalLeftHp() + 1)
		case "round":
			record.FinalRoundNum = proto.Int32(record.GetFinalRoundNum() + 1)
		case "target":
			record.SkillOpts[0].ActorUid = proto.String("other")
		case "reported-result":
			reported = 1 - reported
		case "skill-order":
			record.SkillOpts[1], record.SkillOpts[2] = record.SkillOpts[2], record.SkillOpts[1]
		}
		if ValidateP6RushManualAutoEnd(series, out, record, reported) == nil {
			t.Fatal("partial/shifted record accepted", variant)
		}
	}
}
