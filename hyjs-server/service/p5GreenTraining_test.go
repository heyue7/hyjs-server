package service

import (
	"context"
	"path/filepath"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Original six quality12 materials have no HeroTrainingMaxMin rows. A forged
// high-level request must not invent training caps or charge resources.
func TestP5OriginalGreenTrainingMissingConfiguration(t *testing.T) {
	ctx := context.Background()
	if e := db.InitGame(ctx, filepath.Join(t.TempDir(), "green-training.db")); e != nil {
		t.Fatal(e)
	}
	defer func() { pool, _ := db.GetIns().DB(); pool.Close() }()
	s := &GameService{}
	for _, load := range []func(string) error{s.LoadTables, s.LoadP5Tables} {
		if e := load("../../analysis/tables"); e != nil {
			t.Fatal(e)
		}
	}
	for _, init := range []func(context.Context) error{s.InitializeP1, s.InitializeP5, s.InitializeWarriorGrowth, s.InitializeGameplayPools, s.InitializeP2, s.InitializeOpenServerActivity, s.InitializeDailyTasks} {
		if e := init(ctx); e != nil {
			t.Fatal(e)
		}
	}
	s.P5Ready = true
	account, e := LoginAccount(ctx, "green-training-missing-original-table", "isolated-test-only")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = CreatePlayer(ctx, account.Id, "原绿色材料培养边界", "hero_043"); e != nil {
		t.Fatal(e)
	}
	count := 0
	for _, hero := range s.Tables["Hero"] {
		if hero["heroClass"] != "0" || hero["quality"] != "12" {
			continue
		}
		count++
		for _, row := range s.Tables["HeroTrainingMaxMin"] {
			if row["warId"] == hero["id"] {
				t.Fatal("original green unexpectedly has training configuration")
			}
		}
		var guid string
		if e = updateProfile(ctx, account.Id, func(p *pb.Player, g *model.GameState) error {
			p.Level = proto.Int32(100)
			p.Diamond = proto.Int64(10000)
			p.WarriorBagSize = proto.Int32(1000)
			gain, e := s.GrantFixedReward(p, g, hero["id"]+",1;item_007,100")
			if e != nil {
				return e
			}
			guid = gain.GetWars()[0].GetGuid()
			w, e := p5Warrior(g, guid)
			if e != nil {
				return e
			}
			w.Level = proto.Int32(100)
			return nil
		}); e != nil {
			t.Fatal(e)
		}
		var before, after db.Profile
		if e = db.GetIns().First(&before, account.Id).Error; e != nil {
			t.Fatal(e)
		}
		request := &pb.CSWarriorTraining{Guid: proto.String(guid), CostType: proto.Int32(3), NumType: proto.Int32(3)}
		requestCtx := context.WithValue(context.WithValue(ctx, constUtil.GameServiceContextKey, s), constUtil.GameRequestContextKey, request)
		if _, e = s.warriorTraining(requestCtx, account.Id, request, int64(count)); e == nil {
			t.Fatal("invented green training caps accepted")
		}
		if e = db.GetIns().First(&after, account.Id).Error; e != nil || *before.Player != *after.Player || *before.Game != *after.Game {
			t.Fatal("green missing-table rejection charged or changed inventory", e)
		}
	}
	if count != 6 {
		t.Fatal("original all six green scope", count)
	}
}
