package service

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sync"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// TestCopyFirstDropFrozenLifecycle uses actual first-mission progression and all
// original first-drop rows. Reported wins are isolated contract fixtures.
func TestCopyFirstDropFrozenLifecycle(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "copy-first.db")
	if err := db.InitGame(ctx, path); err != nil {
		t.Fatal(err)
	}
	defer func() { pool, _ := db.GetIns().DB(); pool.Close() }()
	s := &GameService{}
	if err := s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, init := range []func(context.Context) error{s.InitializeCopyDrops, s.InitializeDailyCopy, s.InitCooking} {
		if err := init(ctx); err != nil {
			t.Fatal(err)
		}
	}
	entries := 0
	for _, task := range s.Tables["CopyTask"] {
		input, err := s.copyFirstDropInput(db.GetIns(), task["FirstDropOdds"])
		if err != nil {
			t.Fatal(task["TaskId"], err)
		}
		entries += len(input.Entries)
	}
	if entries < 100 {
		t.Fatal("original first-drop content omitted", entries)
	}
	a, err := LoginAccount(ctx, "first-source", "isolated-only")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CreatePlayer(ctx, a.Id, "首掉测试", "hero_043"); err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(ctx, a.Id, func(p *pb.Player, g *model.GameState) error { p.WarriorBagSize = proto.Int32(1000); return nil }); err != nil {
		t.Fatal(err)
	}
	read := func() (*pb.Player, *model.GameState) {
		t.Helper()
		var row db.Profile
		if err := db.GetIns().First(&row, a.Id).Error; err != nil {
			t.Fatal(err)
		}
		p, g, e := decodeProfile(&row)
		if e != nil {
			t.Fatal(e)
		}
		return p, g
	}
	if _, err := s.copyEnter(ctx, a.Id, &pb.CSCopyEnter{ChapterId: proto.String("1"), MissionId: proto.String("1")}); err != nil {
		t.Fatal(err)
	}
	for _, task := range s.tasks("1") {
		msgs, err := s.copyFightStart(ctx, a.Id, &pb.CSCopyFightStart{ChapterId: proto.String("1"), MissionId: proto.String("1"), TaskId: proto.String(task["TaskId"])})
		if err != nil {
			t.Fatal(err)
		}
		start := msgs[0].(*pb.SCCopyFightStart)
		end := &pb.CSCopyFightEnd{FightId: start.FightId, Result: proto.Int32(1), BattlePlayer: start.BattlePlayer}
		if task["TaskId"] != "4" {
			if _, err := s.copyFightEnd(ctx, a.Id, end); err != nil {
				t.Fatal(err)
			}
			continue
		}
		var frozen db.CopyDropSnapshot
		if err := db.GetIns().First(&frozen, "player_id=? AND fight_id=?", a.Id, start.GetFightId()).Error; err != nil {
			t.Fatal(err)
		}
		var input copyDropInput
		if err := json.Unmarshal(frozen.Input, &input); err != nil || input.First == nil || len(input.First.Entries) != 1 || len(input.First.Entries[0].Options) != 4 {
			t.Fatal("actual boss four-option first source not frozen", err)
		}
		original := task["FirstDropOdds"]
		task["FirstDropOdds"] = "bad_source:1,100"
		if err := db.GetIns().Model(&db.CopyDropRule{}).Where("id=?", constUtil.CopyFirstDropRuleID).Updates(map[string]any{"enabled": false, "mode": "invalid"}).Error; err != nil {
			t.Fatal(err)
		}
		p, g := read()
		before := len(g.Warriors)
		if err := updateProfile(ctx, a.Id, func(p *pb.Player, g *model.GameState) error {
			p.WarriorBagSize = proto.Int32(int32(len(g.Warriors)))
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.copyFightEnd(ctx, a.Id, end); err == nil {
			t.Fatal("full bag accepted frozen random reward")
		}
		_, g = read()
		if len(g.Warriors) != before || g.Run.Fight.Settled {
			t.Fatal("failed material source partially committed")
		}
		if err := updateProfile(ctx, a.Id, func(p *pb.Player, g *model.GameState) error { p.WarriorBagSize = proto.Int32(1000); return nil }); err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		out := make(chan proto.Message, 8)
		errs := make(chan error, 8)
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				m, e := s.copyFightEnd(ctx, a.Id, end)
				errs <- e
				if e == nil {
					out <- m[0]
				}
			}()
		}
		wg.Wait()
		close(out)
		close(errs)
		for e := range errs {
			if e != nil {
				t.Fatal(e)
			}
		}
		var reply proto.Message
		for r := range out {
			if reply == nil {
				reply = r
			}
			if !proto.Equal(reply, r) {
				t.Fatal("concurrent first reward rerolled")
			}
		}
		p, g = read()
		delta := len(g.Warriors) - before
		if delta < 4 || delta > 8 {
			t.Fatal("boss first3..5 + ordinary1..3 natural instances missing", delta)
		}
		savedPlayer := proto.Clone(p)
		savedGame, _ := json.Marshal(g)
		pool, _ := db.GetIns().DB()
		pool.Close()
		if err := db.InitGame(ctx, path); err != nil {
			t.Fatal(err)
		}
		cold, e := s.copyFightEnd(ctx, a.Id, end)
		if e != nil || !proto.Equal(reply, cold[0]) {
			t.Fatal("cold first source receipt changed", e)
		}
		p, g = read()
		raw, _ := json.Marshal(g)
		if !proto.Equal(savedPlayer, p) || string(raw) != string(savedGame) {
			t.Fatal("cold source replay changed entire save")
		}
		task["FirstDropOdds"] = original
	}
	t.Logf("original638 tasks parsed, non-guaranteed first entries=%d; real task4 frozen random instances/capacity rollback/eight-way single settlement/cold full save passed; isolated client-result fixture", entries)
}
