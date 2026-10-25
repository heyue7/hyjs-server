package service

import (
	"context"
	"encoding/json"
	"math"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func copyContinuity69Fixture() (*GameService, *pb.BattlePlayer) {
	s := &GameService{Tables: map[string][]map[string]string{"GameConfigure": {{"StaticName": "UTIL_MAXANGER", "NameValue": "300"}}}}
	party := &pb.BattlePlayer{PlayerId: proto.Int64(1), Inst: &pb.BattlePlayerInst{Anger: proto.Float64(0)}}
	for _, guid := range []string{"alive", "dead"} {
		party.Formation = append(party.Formation, &pb.BattleHero{Guid: proto.String(guid), Id: proto.String("hero_047"), Pose: proto.Int32(int32(len(party.Formation) + 2)), Prop: &pb.BaseProp{Hp: proto.Float64(100), Att: proto.Float64(25)}, Inst: &pb.BattleHeroInst{CurHp: proto.Float64(100)}})
	}
	return s, party
}

func TestCopyContinuity69FrozenIdentityDeadAndDemoCold(t *testing.T) {
	s, original := copyContinuity69Fixture()
	start := proto.Clone(original).(*pb.BattlePlayer)
	demo := proto.Clone(start.Formation[0]).(*pb.BattleHero)
	demo.Guid = proto.String("demo-only")
	start.Formation = append(start.Formation, demo)
	reported := proto.Clone(start).(*pb.BattlePlayer)
	reported.Inst.Anger = proto.Float64(173)
	reported.Formation[0].Inst.CurHp = proto.Float64(61)
	reported.Formation[1].Inst.CurHp = proto.Float64(0)
	reported.Formation[0].Prop.Att = proto.Float64(999999)
	reported.Formation[0].Pose = proto.Int32(99)
	run := &model.CopyRun{Party: original, Fight: &model.CopyFight{Start: &pb.SCCopyFightStart{BattlePlayer: start}}}
	accepted, err := s.copyContinuityAccepted(run, reported)
	if err != nil || len(accepted.GetFormation()) != 2 || accepted.Formation[0].GetProp().GetAtt() != 25 || accepted.Formation[0].GetPose() != 2 {
		t.Fatal("frozen identity or demo isolation failed", accepted, err)
	}
	run.Party = accepted
	raw, err := json.Marshal(run)
	if err != nil {
		t.Fatal(err)
	}
	var cold model.CopyRun
	if err = json.Unmarshal(raw, &cold); err != nil {
		t.Fatal(err)
	}
	fresh := proto.Clone(original).(*pb.BattlePlayer)
	fresh.Formation[0].Prop.Hp = proto.Float64(50)
	fresh.Formation[0].Pose = proto.Int32(7)
	restored, err := s.copyContinuityRestore(fresh, cold.Party)
	if err != nil || restored.GetInst().GetAnger() != 173 || restored.Formation[0].GetInst().GetCurHp() != 50 || restored.Formation[1].GetInst().GetCurHp() != 0 || restored.Formation[0].GetPose() != 7 {
		t.Fatal("cold state, maxHP clamp, death or legal current position lost", restored, err)
	}
	if original.GetInst().GetAnger() != 0 || fresh.Formation[0].GetInst().GetCurHp() != 100 {
		t.Fatal("source mutated")
	}
	fresh.Formation[1].Guid = proto.String("replacement")
	if _, err = s.copyContinuityRestore(fresh, cold.Party); err == nil {
		t.Fatal("replacement restored a fresh member")
	}
}

func TestCopyContinuity69InvalidAngerAndHP(t *testing.T) {
	s, source := copyContinuity69Fixture()
	run := &model.CopyRun{Party: source, Fight: &model.CopyFight{Start: &pb.SCCopyFightStart{BattlePlayer: source}}}
	for _, anger := range []float64{-1, 301, math.NaN(), math.Inf(1)} {
		reported := proto.Clone(source).(*pb.BattlePlayer)
		reported.Inst.Anger = proto.Float64(anger)
		if _, err := s.copyContinuityAccepted(run, reported); err == nil {
			t.Fatal("invalid anger accepted", anger)
		}
	}
	for _, hp := range []float64{-1, 101, math.NaN(), math.Inf(1)} {
		reported := proto.Clone(source).(*pb.BattlePlayer)
		reported.Formation[0].Inst.CurHp = proto.Float64(hp)
		if _, err := s.copyContinuityAccepted(run, reported); err == nil {
			t.Fatal("invalid HP accepted", hp)
		}
	}
}

// Client result fixtures exercise the existing observed settlement contract;
// they do not establish native battle calculation or settlement authority.
func TestCopyContinuity69TasksReceiptColdAndNewRun(t *testing.T) {
	f := p1Setup(t)
	s, ctx, id := f.s, f.ctx, f.id
	for _, initialize := range []func(context.Context) error{s.InitializeCopyDrops, s.InitializeDailyCopy, s.InitCooking} {
		if err := initialize(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err := updateProfile(ctx, id, func(player *pb.Player, _ *model.GameState) error {
		player.Level = proto.Int32(100)
		player.Stamina = proto.Int32(1000)
		player.WarriorBagSize = proto.Int32(1000)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	read := func() (*pb.Player, *model.GameState) { return f.profile(t) }
	cold := func() { f.restart(t) }
	chapter, mission := "1", "1"
	if _, err := s.copyEnter(ctx, id, &pb.CSCopyEnter{ChapterId: proto.String(chapter), MissionId: proto.String(mission)}); err != nil {
		t.Fatal(err)
	}
	tasks := s.tasks(mission)
	if len(tasks) < 2 {
		t.Fatal("need representative multi-task mission")
	}
	var acceptedHP float64
	for i, task := range tasks {
		out, err := s.copyFightStart(ctx, id, &pb.CSCopyFightStart{ChapterId: proto.String(chapter), MissionId: proto.String(mission), TaskId: proto.String(task["TaskId"])})
		if err != nil {
			t.Fatal(err)
		}
		start := out[0].(*pb.SCCopyFightStart)
		if i > 0 && (start.BattlePlayer.GetInst().GetAnger() != 117 || start.BattlePlayer.Formation[0].GetInst().GetCurHp() != acceptedHP) {
			t.Fatal("next task discarded accepted state", start)
		}
		party := proto.Clone(start.BattlePlayer).(*pb.BattlePlayer)
		if i == 0 {
			acceptedHP = party.Formation[0].GetInst().GetCurHp() / 2
			party.Formation[0].Inst.CurHp = proto.Float64(acceptedHP)
		}
		party.Inst.Anger = proto.Float64(117)
		end := &pb.CSCopyFightEnd{FightId: start.FightId, Result: proto.Int32(1), BattlePlayer: party}
		if i == 0 {
			bad := proto.Clone(end).(*pb.CSCopyFightEnd)
			bad.BattlePlayer.Inst.Anger = proto.Float64(301)
			before, _ := read()
			if _, err = s.copyFightEnd(ctx, id, bad); err == nil {
				t.Fatal("invalid anger settled")
			}
			after, state := read()
			if !proto.Equal(before, after) || state.Run.Fight.Settled || len(state.Run.Passed) != 0 {
				t.Fatal("rejected terminal mutated rewards or progress")
			}
		}
		settled, err := s.copyFightEnd(ctx, id, end)
		if err != nil {
			t.Fatal(err)
		}
		cold()
		retried, err := s.copyFightEnd(ctx, id, end)
		_, state := read()
		if err != nil || !proto.Equal(settled[0], retried[0]) || state.Run.Party.GetInst().GetAnger() != 117 {
			t.Fatal("cold receipt or party failed", err)
		}
	}
	if _, err := s.copyEnter(ctx, id, &pb.CSCopyEnter{ChapterId: proto.String(chapter), MissionId: proto.String(mission)}); err != nil {
		t.Fatal(err)
	}
	out, err := s.copyFightStart(ctx, id, &pb.CSCopyFightStart{ChapterId: proto.String(chapter), MissionId: proto.String(mission), TaskId: proto.String(tasks[0]["TaskId"])})
	if err != nil {
		t.Fatal(err)
	}
	party := out[0].(*pb.SCCopyFightStart).BattlePlayer
	if party.GetInst().GetAnger() != 0 || party.Formation[0].GetInst().GetCurHp() != party.Formation[0].GetProp().GetHp() {
		t.Fatal("new run inherited old terminal state")
	}
}
