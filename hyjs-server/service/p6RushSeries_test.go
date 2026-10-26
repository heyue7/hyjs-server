package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p6RushSeriesFixture(t *testing.T) (*GameService, *pb.SCRushFightStart) {
	t.Helper()
	s := p6OriginalTables(t)
	a := p6Actor("hero_047", "copy-player", "047_1", 1, 1000)
	d := p6Actor("hero_047", "copy-enemy", "047_1", 1, 400)
	monster := &pb.BattleMonster{Formation: []*pb.BattleHero{d}, Inst: &pb.BattleMonsterInst{Anger: proto.Float64(0)}}
	start := &pb.SCRushFightStart{FightId: proto.String("wave-probe"), BattlePlayer: &pb.BattlePlayer{PlayerId: proto.Int64(42), Formation: []*pb.BattleHero{a}, Inst: &pb.BattlePlayerInst{Anger: proto.Float64(17)}}, BattleMonsterList: []*pb.BattleMonster{monster, proto.Clone(monster).(*pb.BattleMonster)}, RandSeed: proto.Int64(42), MaxRound: proto.Int32(12)}
	return s, start
}

func TestP6OriginalRushSeriesFreshFighterSameSeedLastOnlyRecord(t *testing.T) {
	s, start := p6RushSeriesFixture(t)
	frozen, err := s.FreezeP6RushSeriesInput(start, true, true, map[string][]string{"scope": {"finite mechanism probe, no original server parameters"}}, "")
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(frozen)
	out, err := s.ComputeP6RushSeries(t.Context(), frozen, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.Authoritative || out.Winner != 0 || len(out.Fights) != 2 || out.AttackerWave != 0 || out.DefenderWave != 2 {
		t.Fatal("original losing defender wave progression", out)
	}
	first, second := out.Fights[0].Outcome, out.Fights[1].Outcome
	if !p6CopySeriesJSONEqual(t, first, second) || out.Fights[0].InputSHA256 != out.Fights[1].InputSHA256 {
		t.Fatal("winning fighter was carried or global seed advanced across fresh ComputeCore")
	}
	if first.Attacker.GetFormation()[0].GetInst().GetCurHp() >= start.BattlePlayer.GetFormation()[0].GetInst().GetCurHp() || first.Attacker.GetInst().GetAnger() == start.BattlePlayer.GetInst().GetAnger() {
		t.Fatal("probe must change winner runtime HP and AP before testing reset", first.Attacker.GetFormation()[0].GetInst().GetCurHp(), start.BattlePlayer.GetFormation()[0].GetInst().GetCurHp(), first.Attacker.GetInst().GetAnger(), start.BattlePlayer.GetInst().GetAnger())
	}
	projected, projectErr := OriginalP6OperateProjection(9, second.Operates)
	if projectErr != nil || !proto.Equal(out.FinalOperates, projected) || out.FinalOperates.GetFinalRoundNum() != second.FinalRound {
		t.Fatal("original terminal record concatenated wave-local rounds")
	}
	verified, err := s.ComputeP6RushSeries(t.Context(), frozen, out.FinalOperates)
	if err != nil || !p6CopySeriesJSONEqual(t, out, verified) {
		t.Fatal("automatic full series terminal record verification", err)
	}
	bad := proto.Clone(out.FinalOperates).(*pb.BattleOperate)
	bad.FinalRoundNum = proto.Int32(bad.GetFinalRoundNum() + 1)
	if partial, computeErr := s.ComputeP6RushSeries(t.Context(), frozen, bad); computeErr == nil || partial != nil {
		t.Fatal("wrong terminal record returned partial wave result")
	}
	after, _ := json.Marshal(frozen)
	if string(before) != string(after) {
		t.Fatal("series calculation modified frozen source")
	}
	start.BattlePlayer.Formation[0].Inst.CurHp = proto.Float64(1)
	start.BattleMonsterList[1].Formation[0].Prop.Att = proto.Float64(99999)
	if frozen.Start.BattlePlayer.Formation[0].GetInst().GetCurHp() == 1 || frozen.Start.BattleMonsterList[1].Formation[0].GetProp().GetAtt() == 99999 {
		t.Fatal("freeze retained producer pointers")
	}
}

func TestP6OriginalRushSeriesLossExhaustionMalformedLaterWaveAndCancellation(t *testing.T) {
	s, start := p6RushSeriesFixture(t)
	start.BattlePlayer.Formation[0].Inst.CurHp = proto.Float64(0)
	frozen, err := s.FreezeP6RushSeriesInput(start, true, true, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	out, err := s.ComputeP6RushSeries(t.Context(), frozen, nil)
	if err != nil || out.Winner != 1 || len(out.Fights) != 1 || out.AttackerWave != 1 || out.DefenderWave != 0 || out.Authoritative {
		t.Fatal("single attacker exhausted must stop before unvisited monster", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if partial, computeErr := s.ComputeP6RushSeries(ctx, frozen, nil); !errors.Is(computeErr, context.Canceled) || partial != nil {
		t.Fatal("cancelled series produced partial result", computeErr)
	}
	if partial, freezeErr := s.FreezeP6RushSeriesInput(start, true, false, nil, ""); freezeErr == nil || partial != nil {
		t.Fatal("manual earlier-wave choices unavailable cannot invent them")
	}
	start.BattleMonsterList[1].Formation[0].Pose = proto.Int32(99)
	if partial, freezeErr := s.FreezeP6RushSeriesInput(start, true, true, nil, ""); freezeErr == nil || partial != nil {
		t.Fatal("early defeat concealed malformed later source wave")
	}
	frozen.Start.BattleMonsterList[1].Inst.Anger = proto.Float64(1)
	if partial, computeErr := s.ComputeP6RushSeries(t.Context(), frozen, nil); computeErr == nil || partial != nil {
		t.Fatal("whole source SHA failed to cover unvisited wave")
	}
}

func TestP6OriginalRushSeriesSourceAndTemplateConflict(t *testing.T) {
	s, start := p6RushSeriesFixture(t)
	start.MaxRound = nil
	f, err := s.FreezeP6RushSeriesInput(start, true, true, nil, "")
	if err != nil || f.Template.MaxRound != 40 {
		t.Fatal("original nonpositive maxRound40", err)
	}
	for _, mutate := range []func(*model.FrozenBattleInput){func(x *model.FrozenBattleInput) { x.Seed++ }, func(x *model.FrozenBattleInput) { x.IsAttToDef = true }, func(x *model.FrozenBattleInput) { x.AttackerDamageFactor = proto.Float64(0) }, func(x *model.FrozenBattleInput) { x.EngineVersion = "old" }, func(x *model.FrozenBattleInput) { x.Tables["Hero"][0]["Id"] = "replacement" }} {
		candidate, cloneErr := cloneP6CalculationInput(f.Template)
		if cloneErr != nil {
			t.Fatal(cloneErr)
		}
		mutate(candidate)
		v := *f
		v.Template = candidate
		if partial, computeErr := s.ComputeP6RushSeries(t.Context(), &v, nil); computeErr == nil || partial != nil {
			t.Fatal("current source/config/engine closure tamper accepted")
		}
	}
	if f.Template.EngineVersion != constUtil.P6EngineVersion {
		t.Fatal("series requires exact current single-pair engine")
	}
}

func TestP6OriginalRushEliteManualSingletonAndCriterionGates(t *testing.T) {
	s, start := p6RushSeriesFixture(t)
	start.BattleMonsterList = start.BattleMonsterList[:1]
	start.MaxRound = nil
	elite, err := s.FreezeP6RushSeriesInput(start, false, true, nil, "")
	if err != nil || elite.Template.BattleType != 10 || elite.Template.MaxRound != 40 || elite.Template.AttackerDamageFactor != nil || elite.Template.DefenderDamageFactor != nil {
		t.Fatal("original false elite defaults", err)
	}
	automatic, err := s.ComputeP6RushSeries(t.Context(), elite, nil)
	if err != nil {
		t.Fatal(err)
	}
	manual, err := s.FreezeP6RushSeriesInput(start, false, false, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if partial, e := s.ComputeP6RushSeries(t.Context(), manual, nil); e == nil || partial != nil {
		t.Fatal("manual source invented automatic choices")
	}
	replay, err := s.ComputeP6RushSeries(t.Context(), manual, automatic.FinalOperates)
	if err != nil || replay.Winner != automatic.Winner || replay.Authoritative || len(replay.Fights) != 1 {
		t.Fatal("original elite player PlaybackAI with automatic NPC", err)
	}
	if !proto.Equal(replay.FinalOperates, automatic.FinalOperates) && len(replay.FinalOperates.SkillOpts) != len(automatic.FinalOperates.SkillOpts) {
		t.Fatal("manual source record action identity")
	}
	start.WinKey = proto.Int32(1)
	if partial, e := s.FreezeP6RushSeriesInput(start, false, true, nil, ""); e == nil || partial != nil {
		t.Fatal("unknown business victory criterion silently dropped")
	}
	start.WinKey = nil
	start.WinValue = proto.Int32(1)
	if partial, e := s.FreezeP6RushSeriesInput(start, true, true, nil, ""); e == nil || partial != nil {
		t.Fatal("criterion value without key silently dropped")
	}
}

// Original BattleFighterData reads Inst.anger directly on both fighter shapes.
// Keep message presence distinct from a valid explicitly sourced anger zero.
func TestP6OriginalRushRequiredInstMessagesAndLegacyInvalidSource(t *testing.T) {
	s, start := p6RushSeriesFixture(t)
	for _, mutate := range []func(*pb.SCRushFightStart){
		func(x *pb.SCRushFightStart) { x.BattlePlayer.Inst = nil },
		func(x *pb.SCRushFightStart) { x.BattleMonsterList[0].Inst = nil },
		func(x *pb.SCRushFightStart) { x.BattleMonsterList[1].Inst = nil },
	} {
		invalid := proto.Clone(start).(*pb.SCRushFightStart)
		mutate(invalid)
		before, _ := proto.MarshalOptions{Deterministic: true}.Marshal(invalid)
		if partial, err := s.FreezeP6RushSeriesInput(invalid, true, true, nil, ""); err == nil || partial != nil {
			t.Fatal("original constructor missing Inst accepted as implicit anger zero")
		}
		after, _ := proto.MarshalOptions{Deterministic: true}.Marshal(invalid)
		if string(before) != string(after) {
			t.Fatal("invalid source was silently repaired")
		}
	}
	raw, err := readTestAnalysis75("../../analysis/p6/actual-local-p7-rush-inputs-v97/000.SCRushFightStart.pb")
	if err != nil {
		t.Fatal(err)
	}
	legacy := &pb.SCRushFightStart{}
	if err = proto.Unmarshal(raw, legacy); err != nil {
		t.Fatal(err)
	}
	if legacy.BattleMonsterList[0].Inst != nil {
		t.Fatal("preserve actual legacy source missing-message evidence")
	}
	if partial, err := s.FreezeP6RushSeriesInput(legacy, true, true, nil, ""); err == nil || partial != nil {
		t.Fatal("known original ctor crash source was reported executable")
	}
}
