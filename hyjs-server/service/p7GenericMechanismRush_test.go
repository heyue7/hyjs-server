package service

import (
	"encoding/json"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestP7GenericMechanismRushFullGrowthAndStrictEnd(t *testing.T) {
	s, source, _ := p6ActualManualSeries(t)
	grown := p6GenericGrownActors(t)
	for _, probe := range []struct {
		kind int32
		ids  []string
	}{{constUtil.P7RushTrial, []string{"hero_025", "hero_011"}}, {constUtil.P7RushElite, []string{"hero_071", "hero_001"}}} {
		t.Run(probe.ids[0], func(t *testing.T) {
			start := proto.Clone(source.Start).(*pb.SCRushFightStart)
			start.FightId = proto.String("generic-original-mechanism-short")
			start.RandSeed = proto.Int64((1 << 47) + 42)
			start.MaxRound = proto.Int32(6)
			start.RushType = proto.Int32(probe.kind)
			start.BattlePlayer.Formation = nil
			for i, id := range probe.ids {
				actor := proto.Clone(grown[id]).(*pb.BattleHero)
				actor.Guid = proto.String([]string{"00000000-0000-4000-8000-000000000001", "00000000-0000-4000-8000-000000000002"}[i])
				actor.Pose = proto.Int32([]int32{3, 7}[i])
				actor.Prop.Att = proto.Float64(actor.GetProp().GetAtt() + .25)
				start.BattlePlayer.Formation = append(start.BattlePlayer.Formation, actor)
			}
			series, err := s.FreezeP6RushSeriesInput(start, probe.kind == constUtil.P7RushTrial, false, nil, constUtil.P6OriginalClientHash)
			if err != nil {
				t.Fatal(err)
			}
			before, _ := json.Marshal(series)
			family, err := p7ManualRushFamilyForSeries(series)
			if err != nil || !family.Generic || family.ScopeProof != constUtil.P7GenericMechanismCompatibilityProof {
				t.Fatal("full-growth actual supported actors still ID gated", err)
			}
			restored, err := p7ManualRushFamilyForReceipt(probe.kind, family.DriverSource, family.ScopeProof)
			if err != nil || !restored.Generic {
				t.Fatal("named mechanism receipt lost", err)
			}
			preview, err := family.Preview(t.Context(), s, series)
			if err != nil {
				t.Fatal("original driver preview", err)
			}
			reported := int32(0)
			if preview.Outcome.Winner == 0 {
				reported = 1
			}
			if len(preview.OriginalOperates.SkillOpts) == 0 {
				t.Fatal("actual original PVE control not recorded")
			}
			terminal, err := restored.ComputeEnd(t.Context(), &GameService{}, series, preview.OriginalOperates, reported)
			if err != nil || !proto.Equal(terminal.OriginalOperates, preview.OriginalOperates) {
				t.Fatal("independently recomputed complete End", err)
			}
			if terminal.Outcome.DiagnosticAIDriverPolicy != constUtil.P6DiagnosticManualAutoStart {
				t.Fatal("simultaneous live Auto source not frozen")
			}
			if restored.ValidateEnd(series, terminal, terminal.OriginalOperates, reported) != nil {
				t.Fatal("cold dual-driver receipt")
			}
			t.Logf("actual driver=%s; fullPB=true; seed=%d", terminal.Outcome.DiagnosticAIDriverPolicy, series.Template.Seed)
			wrongDriver := *terminal
			wrongOutcome := *terminal.Outcome
			wrongDriver.Outcome = &wrongOutcome
			wrongDriver.Outcome.DiagnosticAIDriverPolicy = ""
			if restored.ValidateEnd(series, &wrongDriver, terminal.OriginalOperates, reported) == nil {
				t.Fatal("cold receipt accepted wrong actual driver")
			}
			bad := proto.Clone(terminal.OriginalOperates).(*pb.BattleOperate)
			bad.SkillOpts[0].SkillIndex = proto.Int32((bad.SkillOpts[0].GetSkillIndex() + 1) % 3)
			if restored.ValidateEnd(series, terminal, bad, reported) == nil || restored.ValidateEnd(series, terminal, terminal.OriginalOperates, 1-reported) == nil {
				t.Fatal("forged full operation or winner accepted")
			}
			terminal.FinalActors[0].CurrentHP++
			if restored.ValidateEnd(series, terminal, terminal.OriginalOperates, reported) == nil {
				t.Fatal("forged terminal HP accepted")
			}
			after, _ := json.Marshal(series)
			if string(before) != string(after) {
				t.Fatal("frozen actual actors rewritten")
			}
			missing := proto.Clone(start).(*pb.SCRushFightStart)
			missing.BattlePlayer.Formation[0].TalentList = append(missing.BattlePlayer.Formation[0].TalentList, "tal_unknown_generic_rush")
			unknown, err := s.FreezeP6RushSeriesInput(missing, probe.kind == constUtil.P7RushTrial, false, nil, constUtil.P6OriginalClientHash)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = p7GenericMechanismRushProof(unknown); err == nil || !strings.Contains(err.Error(), "tal_unknown_generic_rush") {
				t.Fatal("unknown factory source did not name exact missing branch", err)
			}
		})
	}
}

// This legal original mechanism fixture uses identical simple melee actors and
// equal-row position shifts, so the recorded commands remain valid after DoTrans.
// It supplements the original phone formation/manual evidence; no native claim is added.
func TestP7GenericMechanismRushMultipleCommandsAndFormation(t *testing.T) {
	s, original, _ := p6ActualManualSeries(t)
	formation := p6FormationInput(t, s)
	for _, a := range formation.Attacker.Formation {
		a.Prop.Att = proto.Float64(1)
	}
	formation.Monster.Formation[0].Prop.Att = proto.Float64(1)
	start := proto.Clone(original.Start).(*pb.SCRushFightStart)
	start.FightId = proto.String("generic-manual-formation")
	start.RandSeed = proto.Int64((1 << 47) + 42)
	start.MaxRound = proto.Int32(1)
	start.BattlePlayer = formation.Attacker
	start.BattleMonsterList = []*pb.BattleMonster{formation.Monster}
	series, err := s.FreezeP6RushSeriesInput(start, true, false, nil, constUtil.P6OriginalClientHash)
	if err != nil {
		t.Fatal(err)
	}
	family, err := p7ManualRushFamilyForSeries(series)
	if err != nil || !family.Generic {
		t.Fatal("mechanism family", err)
	}
	preview, err := family.Preview(t.Context(), s, series)
	if err != nil {
		t.Fatal("preview", err)
	}
	record := proto.Clone(preview.OriginalOperates).(*pb.BattleOperate)
	record.AttackerTransOpt = &pb.BattleTransOperate{RoundNum: proto.Int32(1), ActorUids: []string{"formation-first", "formation-second"}, Fpids: []int32{4, 8}}
	// Delay every player press by one exact original 0.1 clock tick. These two
	// identical low-damage actors survive the whole round, so the same complete
	// terminal record can prove a real manual sequence differing from Preview.
	for _, op := range record.SkillOpts {
		if op.GetCamp() != 0 {
			continue
		}
		clock := float64(0)
		for clock <= op.GetRoundTime()+0.05 {
			clock += 0.1
		}
		op.RoundTime = proto.Float64(clock)
	}

	commands := 0
	for _, op := range record.SkillOpts {
		if op.GetCamp() == 0 {
			commands++
		}
	}
	if commands < 2 {
		t.Fatal("fixture lacks multiple actual player commands", commands)
	}
	reported := int32(0)
	if preview.Outcome.Winner == 0 {
		reported = 1
	}
	before, _ := json.Marshal(series)
	out, err := family.ComputeEnd(t.Context(), &GameService{}, series, record, reported)
	if err != nil {
		t.Fatal("actual multiple commands plus original formation", err)
	}
	if out.Outcome.DiagnosticAIDriverPolicy != "" {
		t.Fatal("actual manual commands replaced by automatic candidate")
	}
	if !proto.Equal(out.OriginalOperates, record) || out.OriginalRecordSHA256 == "" || out.CalculationSourceSHA256 == preview.CalculationSourceSHA256 {
		t.Fatal("actual record/source binding lost")
	}
	positions := map[string]int32{"formation-first": 4, "formation-second": 8}
	for _, actor := range out.FinalActors {
		if actor.Camp == 0 && actor.RuntimePose != positions[actor.GUID] {
			t.Fatal("original runtime formation lost", actor)
		}
	}
	for _, variant := range []string{"actor", "position", "cooldown", "hp", "winner"} {
		bad := proto.Clone(record).(*pb.BattleOperate)
		result := reported
		switch variant {
		case "actor":
			bad.SkillOpts[0].ActorUid = proto.String("foreign")
		case "position":
			bad.AttackerTransOpt.Fpids[1] = bad.AttackerTransOpt.Fpids[0]
		case "cooldown":
		case "hp":
			bad.AttackerTotalLeftHp = proto.Float64(bad.GetAttackerTotalLeftHp() + 1)
		case "winner":
			result = 1 - reported
		}
		if variant == "cooldown" {
			for i, op := range bad.SkillOpts {
				if op.GetCamp() == 0 {
					copied := proto.Clone(op).(*pb.BattleSkillOperate)
					bad.SkillOpts = append(bad.SkillOpts[:i], append([]*pb.BattleSkillOperate{copied}, bad.SkillOpts[i:]...)...)
					break
				}
			}
		}
		if _, err = family.ComputeEnd(t.Context(), &GameService{}, series, bad, result); err == nil {
			t.Fatal("forged actual End accepted", variant)
		}
	}
	altered := *out
	altered.FinalActors = append([]model.CombatFinalActorHP(nil), out.FinalActors...)
	altered.FinalActors[0].RuntimePose = 15
	if family.ValidateEnd(series, &altered, record, reported) == nil {
		t.Fatal("forged runtime position accepted")
	}
	after, _ := json.Marshal(series)
	if string(before) != string(after) {
		t.Fatal("actual End mutated frozen series")
	}
	t.Logf("actual player commands=%d; original transform=%v; winner=%d; completePB=true", commands, record.AttackerTransOpt.Fpids, out.Outcome.Winner)
}
