package service

import (
	"context"
	"encoding/json"
	"path/filepath"
	"slices"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// These original protocol fixtures exercise paid run ownership/order/rewards.
// Reported wins remain explicit isolated client-result contracts, not P6 proof.
func copyEncounterOrderSetup(t *testing.T) (*GameService, context.Context, int64, func() (*pb.Player, *model.GameState), func()) {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "copy-order.db")
	if err := db.InitGame(ctx, path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool, _ := db.GetIns().DB(); pool.Close() })
	service := &GameService{}
	if err := service.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, init := range []func(context.Context) error{service.InitializeCopyDrops, service.InitializeDailyCopy, service.InitCooking} {
		if err := init(ctx); err != nil {
			t.Fatal(err)
		}
	}
	account, err := LoginAccount(ctx, "original-map-encounter-order", "isolated-only")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(ctx, account.Id, "原地图碰撞顺序", "hero_047"); err != nil {
		t.Fatal(err)
	}
	if err = updateProfile(ctx, account.Id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(100)
		p.Stamina = proto.Int32(1000)
		p.WarriorBagSize = proto.Int32(1000)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	read := func() (*pb.Player, *model.GameState) {
		t.Helper()
		var row db.Profile
		if e := db.GetIns().First(&row, account.Id).Error; e != nil {
			t.Fatal(e)
		}
		p, g, e := decodeProfile(&row)
		if e != nil {
			t.Fatal(e)
		}
		return p, g
	}
	cold := func() {
		t.Helper()
		pool, _ := db.GetIns().DB()
		if e := pool.Close(); e != nil {
			t.Fatal(e)
		}
		if e := db.InitGame(ctx, path); e != nil {
			t.Fatal(e)
		}
	}
	return service, ctx, account.Id, read, cold
}

func TestCopyOriginalMapOrdinaryOutOfOrderActiveOwnershipAndAllEncounterCompletion(t *testing.T) {
	service, ctx, id, read, cold := copyEncounterOrderSetup(t)
	mission := service.table("CopyMission", "MissionId", "8")
	chapter := mission["ChapterId"]
	if err := updateProfile(ctx, id, func(_ *pb.Player, g *model.GameState) error {
		// Finite prerequisite progress only; the tested run itself pays original
		// entry stamina and uses actual mission8 jobs/enemies without edits.
		service.refreshCopyProgress(g)
		for _, info := range g.Copy.PassedCopyInfo {
			if info.GetMissionId() == "7" {
				info.Star = proto.Int32(5)
			}
		}
		if mission["OpenId"] != "" {
			g.Quests = append(g.Quests, mission["OpenId"])
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	before, _ := read()
	if _, err := service.copyEnter(ctx, id, &pb.CSCopyEnter{ChapterId: proto.String(chapter), MissionId: proto.String("8")}); err != nil {
		t.Fatal(err)
	}
	paid, _ := read()
	if before.GetStamina()-paid.GetStamina() != int32(tableNumber(mission, "PowerCost")) {
		t.Fatal("original entry cost changed")
	}
	start := func(task string) *pb.SCCopyFightStart {
		t.Helper()
		out, e := service.copyFightStart(ctx, id, &pb.CSCopyFightStart{ChapterId: proto.String(chapter), MissionId: proto.String("8"), TaskId: proto.String(task)})
		if e != nil {
			t.Fatal(task, e)
		}
		return out[0].(*pb.SCCopyFightStart)
	}
	reject := func(ch, mi, task string) {
		t.Helper()
		p, g := read()
		before, _ := json.Marshal([]any{p, g})
		if _, e := service.copyFightStart(ctx, id, &pb.CSCopyFightStart{ChapterId: proto.String(ch), MissionId: proto.String(mi), TaskId: proto.String(task)}); e == nil {
			t.Fatal("invalid encounter accepted", ch, mi, task)
		}
		p, g = read()
		after, _ := json.Marshal([]any{p, g})
		if string(before) != string(after) {
			t.Fatal("rejected collision mutated run/profile")
		}
	}
	finish := func(source *pb.SCCopyFightStart, won int32) *pb.SCCopyFightEnd {
		t.Helper()
		out, e := service.copyFightEnd(ctx, id, &pb.CSCopyFightEnd{FightId: source.FightId, Result: proto.Int32(won), BattlePlayer: source.BattlePlayer})
		if e != nil {
			t.Fatal(e)
		}
		return out[0].(*pb.SCCopyFightEnd)
	}
	first := start("31")
	if !proto.Equal(first, start("31")) {
		t.Fatal("same active collision not frozen")
	}
	reject(chapter, "8", "30")
	reject(chapter, "8", "32")
	reject(chapter, "8", "999999")
	reject(chapter, "8", "33")
	reject(chapter, "9", "31")
	reject("999", "8", "31")
	loss := finish(first, 0)
	if loss.GetIsCopyComplete() != 0 {
		t.Fatal("loss completed run")
	}
	_, g := read()
	if slices.Contains(g.Run.Passed, "31") {
		t.Fatal("loss consumed encounter")
	}
	retry := start("31")
	if retry.GetFightId() == first.GetFightId() {
		t.Fatal("settled loss not given new attempt")
	}
	if finish(retry, 1).GetIsCopyComplete() != 0 {
		t.Fatal("one encounter completed mission")
	}
	reject(chapter, "8", "31")
	boss := start("32")
	if finish(boss, 1).GetIsCopyComplete() != 0 {
		t.Fatal("early boss bypassed remaining encounters")
	}
	cold()
	enter, err := service.copyEnter(ctx, id, &pb.CSCopyEnter{ChapterId: proto.String(chapter), MissionId: proto.String("8")})
	if err != nil {
		t.Fatal(err)
	}
	jobs := enter[0].(*pb.SCCopyEnter).JobList
	tasks := []string{}
	for _, job := range jobs {
		tasks = append(tasks, job.GetTaskId())
	}
	if !slices.Equal(tasks, []string{"29", "30"}) {
		t.Fatal("cold map did not preserve dead/live encounter set", tasks)
	}
	afterEnter, _ := read()
	if afterEnter.GetStamina() != paid.GetStamina() {
		t.Fatal("resumed paid run charged again")
	}
	if finish(start("30"), 1).GetIsCopyComplete() != 0 {
		t.Fatal("missing task29 bypassed")
	}
	finalStart := start("29")
	end := finish(finalStart, 1)
	if end.GetIsCopyComplete() != 1 {
		t.Fatal("all actual encounters did not complete")
	}
	p, g := read()
	if !g.Run.Complete || !slices.Equal(g.Run.Passed, []string{"31", "32", "30", "29"}) {
		t.Fatal("actual collision order not retained")
	}
	count := int32(0)
	for _, info := range g.Copy.PassedCopyInfo {
		if info.GetMissionId() == "8" {
			count = info.GetChallengeTimes()
			if info.GetStar() != 5 {
				t.Fatal("completion star missing")
			}
		}
	}
	if count != 1 {
		t.Fatal("mission challenge counted before/after allencounters", count)
	}
	beforeReplay, _ := json.Marshal([]any{p, g})
	if !proto.Equal(end, finish(finalStart, 1)) {
		t.Fatal("settled original end replay drift")
	}
	p, g = read()
	afterReplay, _ := json.Marshal([]any{p, g})
	if string(beforeReplay) != string(afterReplay) {
		t.Fatal("settlement replay rewarded twice")
	}
}

func TestCopyOriginalScriptedDemoKeepsPriorEncounterAndLegalCollisionGates(t *testing.T) {
	service, ctx, id, read, _ := copyEncounterOrderSetup(t)
	if _, err := service.copyEnter(ctx, id, &pb.CSCopyEnter{ChapterId: proto.String("1"), MissionId: proto.String("1")}); err != nil {
		t.Fatal(err)
	}
	p, g := read()
	before, _ := json.Marshal([]any{p, g})
	if _, err := service.copyFightStart(ctx, id, &pb.CSCopyFightStart{ChapterId: proto.String("1"), MissionId: proto.String("1"), TaskId: proto.String("4")}); err == nil {
		t.Fatal("scripted first-clear demo skipped prior encounter")
	}
	p, g = read()
	after, _ := json.Marshal([]any{p, g})
	if string(before) != string(after) {
		t.Fatal("scripted rejection partially mutated run")
	}
	task := service.table("CopyTask", "TaskId", "1")
	born := task["BornType"]
	task["BornType"] = "1"
	if _, err := service.copyFightStart(ctx, id, &pb.CSCopyFightStart{ChapterId: proto.String("1"), MissionId: proto.String("1"), TaskId: proto.String("1")}); err == nil {
		t.Fatal("unknown scripted spawn admitted")
	}
	task["BornType"] = born
	result, err := service.copyFightStart(ctx, id, &pb.CSCopyFightStart{ChapterId: proto.String("1"), MissionId: proto.String("1"), TaskId: proto.String("1")})
	if err != nil {
		t.Fatal(err)
	}
	start := result[0].(*pb.SCCopyFightStart)
	if _, err = service.copyFightEnd(ctx, id, &pb.CSCopyFightEnd{FightId: start.FightId, Result: proto.Int32(1), BattlePlayer: start.BattlePlayer}); err != nil {
		t.Fatal(err)
	}
	demo, err := service.copyFightStart(ctx, id, &pb.CSCopyFightStart{ChapterId: proto.String("1"), MissionId: proto.String("1"), TaskId: proto.String("4")})
	if err != nil || len(demo[0].(*pb.SCCopyFightStart).BattlePlayer.Formation) <= 1 {
		t.Fatal("original scripted helper source changed", err)
	}
}
