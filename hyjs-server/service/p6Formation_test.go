package service

import (
	"encoding/json"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p6FormationInput(t *testing.T, s *GameService) *model.FrozenBattleInput {
	t.Helper()
	source, err := s.FreezeBattleInput(model.FrozenBattleInput{Attacker: &pb.BattlePlayer{PlayerId: proto.Int64(42), Formation: []*pb.BattleHero{p6Actor("hero_047", "formation-first", "047_1", 3, 80000), p6Actor("hero_047", "formation-second", "047_1", 7, 80000)}, Inst: &pb.BattlePlayerInst{}}, Monster: &pb.BattleMonster{Formation: []*pb.BattleHero{p6Actor("monster_016", "formation-enemy", "1016_1", 3, 1000)}, Inst: &pb.BattleMonsterInst{}}, Seed: 42, MaxRound: 6, BattleType: 1, AttackerDamageFactor: proto.Float64(0), DefenderDamageFactor: proto.Float64(1)})
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func TestP6OriginalFormationMainPositionAndPlaybackRunOrder(t *testing.T) {
	for _, item := range []struct {
		file, sha string
		terms     []string
	}{
		{"formation-position-v94-original.il.txt", "c4a57834f5ca5706e03cb22f31e05d5064cd0e171574176d4b4d5efd551ba808", []string{"METHOD Battle.Engine.FPosition::SetFPID", "0009 stfld Battle.Engine.FPosition::row", "001b stfld Battle.Engine.FPosition::col"}},
		{"formation-record-v94-original.il.txt", "7f3f9817e3e97e84c020f8fa332657aff5cecfb8ce50512ba822d2cd3e412c44", []string{"0027 ldfld Battle.Engine.LogicModel.FActor::MainPosition", "0033 callvirt Battle.Engine.FPosition::SetFPID", "00b5 callvirt Add", "003a callvirt Battle.Engine.ComputeCore::RoundBegin", "0050 callvirt Battle.Engine.AI.EngineAI::NextRound"}},
	} {
		raw, err := readTestAnalysis75("../../analysis/p6/" + item.file)
		if err != nil || p6Digest(raw) != item.sha {
			t.Fatal("original formation rule evidence", err)
		}
		for _, term := range item.terms {
			if !strings.Contains(string(raw), term) {
				t.Fatal("original position rule", term)
			}
		}
	}
	s := p6OriginalTables(t)
	f := p6FormationInput(t, s)
	e := &combatRoundEngine{service: &GameService{Tables: f.Tables}, input: f, round: 1, camp: 0, originalRecordMode: true, rng: NewCombatRandom(42), operate: &pb.BattleOperate{}}
	if err := e.initialize(); err != nil {
		t.Fatal(err)
	}
	change := &pb.BattleTransOperate{RoundNum: proto.Int32(1), ActorUids: []string{"formation-first", "formation-second"}, Fpids: []int32{7, 3}}
	e.supplied = &pb.BattleOperate{AttackerTransOpt: change}
	if err := e.validateOperates(e.supplied); err != nil {
		t.Fatal(err)
	}
	if err := e.beginRound(); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, e.conditionProperty(e.players[0].actors[0], "row"), 1)
	if err := e.applyOriginalFormationRecord(); err != nil {
		t.Fatal(err)
	}
	first, second := e.players[0].actors[0], e.players[0].actors[1]
	closeCombat(t, e.conditionProperty(first, "row"), 2)
	if first.hero.GetPose() != 3 || second.hero.GetPose() != 7 || first.position() != 7 || second.position() != 3 || !proto.Equal(e.operate.AttackerTransOpt, change) || e.rng.Draws() != 0 {
		t.Fatal("DoTrans changed source hero or omitted runtime position")
	}
	target, err := e.selectTargets(e.players[1].actors[0], 3, nil, nil)
	if err != nil || len(target) != 1 || target[0] != second {
		t.Fatal("runtime position failed original front target order", err)
	}
	original := proto.Clone(change).(*pb.BattleTransOperate)
	e.operate.AttackerTransOpt.Fpids[0] = 15
	if !proto.Equal(change, original) {
		t.Fatal("record position arrays alias caller")
	}
	// The first source event is the sole round1 transform. Repeated updates
	// cannot duplicate its record or mutate the original formation array.
	count := len(e.events)
	if err = e.applyOriginalFormationRecord(); err != nil || len(e.events) != count {
		t.Fatal("duplicate source formation applied", err)
	}
}

func TestP6OriginalFormationWholeRecordedPlaybackAndFinalSlots(t *testing.T) {
	s := p6OriginalTables(t)
	f := p6FormationInput(t, s)
	automatic, err := s.ComputeOriginalP6ProtocolBattle(t.Context(), f, nil)
	if err != nil {
		t.Fatal(err)
	}
	record := proto.Clone(automatic.OriginalOperates).(*pb.BattleOperate)
	record.AttackerTransOpt = &pb.BattleTransOperate{RoundNum: proto.Int32(1), ActorUids: []string{"formation-first", "formation-second"}, Fpids: []int32{4, 8}}
	before, _ := json.Marshal(f)
	recordBefore, _ := proto.MarshalOptions{Deterministic: true}.Marshal(record)
	out, err := s.ComputeOriginalP6ProtocolBattle(t.Context(), f, record)
	if err != nil || !proto.Equal(out.OriginalOperates.AttackerTransOpt, record.AttackerTransOpt) || !proto.Equal(out.SuppliedOriginalOperates, record) || out.Outcome.Authoritative {
		t.Fatal("original formation whole replay", err)
	}
	audit, err := s.ComputeOriginalP6ProtocolFinalHP(t.Context(), f, record)
	if err != nil || len(audit.FinalActors) != 3 || audit.OriginalRecordSHA256 != p6Digest(recordBefore) || !p6CopySeriesJSONEqual(t, out.Outcome, audit.Outcome) {
		t.Fatal("original record finalHP audit", err)
	}
	for _, actor := range audit.FinalActors {
		if actor.Camp == 0 {
			want := int32(4)
			initial := int32(3)
			if actor.GUID == "formation-second" {
				want = 8
				initial = 7
			}
			if actor.Pose != initial || actor.RuntimePose != want || actor.InitialActorSHA256 == "" {
				t.Fatal("source HP slot conflated with runtime formation", actor)
			}
		} else if actor.RuntimePose != 0 {
			t.Fatal("unchanged runtime slot emitted a transform")
		}
	}
	after, _ := json.Marshal(f)
	recordAfter, _ := proto.MarshalOptions{Deterministic: true}.Marshal(record)
	if string(before) != string(after) || string(recordBefore) != string(recordAfter) {
		t.Fatal("formation source mutated")
	}
	bad := proto.Clone(record).(*pb.BattleOperate)
	bad.AttackerTransOpt.Fpids[0] = 5
	if p6OriginalProtocolRecordEqual(record, bad) {
		t.Fatal("original record equality ignored transformed positions")
	}
	changed, err := s.ComputeOriginalP6ProtocolFinalHP(t.Context(), f, bad)
	if err != nil || changed.CalculationSourceSHA256 == audit.CalculationSourceSHA256 || changed.OriginalRecordSHA256 == audit.OriginalRecordSHA256 {
		t.Fatal("formation source record omitted from calculation digest", err)
	}

	if _, err = s.ComputeFrozenBattle(t.Context(), f, record); err == nil {
		t.Fatal("historical generic diagnostic replay silently enabled original PVE transformation")
	}
}

func TestP6OriginalFormationRejectsMalformedUnavailableAndWrongDriver(t *testing.T) {
	s := p6OriginalTables(t)
	f := p6FormationInput(t, s)
	for _, variant := range []string{"mismatch", "zero", "range", "unknown", "duplicate", "opposite-round", "after-end", "defender", "pvp"} {
		record := &pb.BattleOperate{AttackerTransOpt: &pb.BattleTransOperate{RoundNum: proto.Int32(1), ActorUids: []string{"formation-first", "formation-second"}, Fpids: []int32{7, 3}}}
		input, err := cloneP6CalculationInput(f)
		if err != nil {
			t.Fatal(err)
		}
		switch variant {
		case "mismatch":
			record.AttackerTransOpt.Fpids = record.AttackerTransOpt.Fpids[:1]
		case "zero":
			record.AttackerTransOpt.RoundNum = proto.Int32(0)
		case "range":
			record.AttackerTransOpt.Fpids[0] = 16
		case "unknown":
			record.AttackerTransOpt.ActorUids[0] = "missing"
		case "duplicate":
			record.AttackerTransOpt.ActorUids[1] = record.AttackerTransOpt.ActorUids[0]
		case "opposite-round":
			record.AttackerTransOpt.RoundNum = proto.Int32(2)
		case "after-end":
			record.AttackerTransOpt.RoundNum = proto.Int32(7)
		case "defender":
			record.DefenderTransOpt = record.AttackerTransOpt
			record.AttackerTransOpt = nil
		case "pvp":
			input.BattleType = 14
		}
		before, _ := json.Marshal(input)
		if result, err := s.ComputeOriginalP6ProtocolBattle(t.Context(), input, record); err == nil || result != nil {
			t.Fatal("invalid or unaudited original transformation admitted", variant)
		}
		after, _ := json.Marshal(input)
		if string(before) != string(after) {
			t.Fatal("invalid transform mutated source", variant)
		}
	}
}

// Initial DEAD belongs to runtime actors even though it does not enter the
// initial deadActors revival queue. Original DoTrans resolves UID without a
// state filter; the source HP and formation slot remain immutable.
func TestP6OriginalFormationRetainsInitiallyDeadRuntimeMember(t *testing.T) {
	s := p6OriginalTables(t)
	f := p6FormationInput(t, s)
	f.Attacker.Formation[0].Inst.CurHp = proto.Float64(0)
	e := &combatRoundEngine{service: &GameService{Tables: f.Tables}, input: f, round: 1, camp: 0, originalRecordMode: true, rng: NewCombatRandom(42), operate: &pb.BattleOperate{}}
	if err := e.initialize(); err != nil {
		t.Fatal(err)
	}
	e.supplied = &pb.BattleOperate{AttackerTransOpt: &pb.BattleTransOperate{RoundNum: proto.Int32(1), ActorUids: []string{"formation-first", "formation-second"}, Fpids: []int32{7, 3}}}
	if err := e.validateOperates(e.supplied); err != nil {
		t.Fatal(err)
	}
	if err := e.applyOriginalFormationRecord(); err != nil {
		t.Fatal(err)
	}
	actor := e.players[0].actors[0]
	if actor.state != 4 || actor.hp != 0 || actor.position() != 7 || actor.hero.GetPose() != 3 {
		t.Fatal("initial DEAD runtime member disappeared or gained HP", actor.state, actor.hp, actor.position())
	}
}
