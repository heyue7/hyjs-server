package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Every original usable treasure consumes real untouched material instances
// from strength1 to strength100, retains remainder and refunds exact investment.
func TestP5OriginalTreasureHighestFullScope(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "treasure-full.db")
	open := func() {
		t.Helper()
		var err error
		if schema := os.Getenv("HYJS_P5_TALENT_MYSQL_DATABASE"); schema != "" {
			if !strings.HasPrefix(schema, "hyjs_test_p5_talent_") {
				t.Fatal("dedicated full growth schema required")
			}
			err = db.InitGameMySQL(ctx, db.MySQLConfig{Host: os.Getenv("HYJS_MYSQL_HOST"), User: os.Getenv("HYJS_MYSQL_USER"), Password: os.Getenv("HYJS_MYSQL_PASSWORD"), Database: schema})
		} else {
			err = db.InitGame(ctx, path)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	open()
	defer func() { pool, _ := db.GetIns().DB(); pool.Close() }()
	s := &GameService{}
	for _, load := range []func(string) error{s.LoadTables, s.LoadP5Tables} {
		if err := load("../../analysis/tables"); err != nil {
			t.Fatal(err)
		}
	}
	for _, init := range []func(context.Context) error{s.InitializeP1, s.InitializeP5, s.InitializeWarriorGrowth, s.InitializeGameplayPools, s.InitializeP2, s.InitializeOpenServerActivity, s.InitializeDailyTasks} {
		if err := init(ctx); err != nil {
			t.Fatal(err)
		}
	}
	s.P5Ready = true
	requestContext := func(request proto.Message) context.Context {
		return context.WithValue(context.WithValue(ctx, constUtil.GameServiceContextKey, s), constUtil.GameRequestContextKey, request)
	}
	count := 0
	resetReceipts := map[int64]*pb.CStreasureRebuild{}
	for _, config := range s.Tables["Treasure"] {
		if tableNumber(config, "maxStrengthen") == 0 {
			continue
		}
		t.Run(config["id"], func(t *testing.T) {
			account, err := LoginAccount(ctx, "p5-full-treasure-"+config["id"], "isolated-test-only")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = CreatePlayer(ctx, account.Id, "原饰品最高阶", "hero_043"); err != nil {
				t.Fatal(err)
			}
			guid := ""
			if err = updateProfile(ctx, account.Id, func(p *pb.Player, g *model.GameState) error {
				p.Level = proto.Int32(100)
				p.Coin = proto.Int64(1000000000)
				p.Diamond = proto.Int64(1000)
				p.EquipBagSize = proto.Int32(1000)
				gain, err := s.GrantFixedReward(p, g, config["id"]+",1")
				if err == nil {
					guid = gain.GetTreas()[0].GetGuid()
				}
				return err
			}); err != nil {
				t.Fatal(err)
			}
			read := func() (*pb.Player, *model.GameState, *pb.Treasure) {
				t.Helper()
				var profile db.Profile
				if err := db.GetIns().First(&profile, account.Id).Error; err != nil {
					t.Fatal(err)
				}
				p, g, err := decodeProfile(&profile)
				if err != nil {
					t.Fatal(err)
				}
				value, err := s.treasureRecord(g, guid)
				if err != nil {
					t.Fatal(err)
				}
				return p, g, value
			}
			initial, _, treasure := read()
			var (
				materials int
				spent     int64
				inputExp  int64
			)
			for treasure.GetStrengthLevel() < int32(tableNumber(config, "maxStrengthen")) {
				request := &pb.CSTreasureStrengthen{TargetId: proto.String(guid)}
				if err = updateProfile(ctx, account.Id, func(p *pb.Player, g *model.GameState) error {
					// Four original untouched red treasures are finite test provision;
					// no cultivated instance or target experience is injected.
					gain, err := s.GrantFixedReward(p, g, "trea_015,4")
					if err == nil {
						for _, material := range gain.GetTreas() {
							request.Guids = append(request.Guids, material.GetGuid())
						}
					}
					return err
				}); err != nil {
					t.Fatal(err)
				}
				out, err := s.treasureStrengthen(requestContext(request), account.Id, request)
				if err != nil {
					t.Fatal("original material path to highest strength", config["id"], treasure.GetStrengthLevel(), err)
				}
				result := out[0].(*pb.SCTreasureStrengthen)
				for _, cost := range result.GetCost() {
					if cost.GetId() == "coin" {
						spent += int64(cost.GetNum())
					}
				}
				inputExp += int64(tableNumber(s.table("Treasure", "id", "trea_015"), "exp")) * 4
				materials += 4
				p, g, value := read()
				treasure = value
				if initial.GetCoin()-p.GetCoin() != spent || len(g.Treasures) != 1 {
					t.Fatal("original exact fees or materials lost", config["id"])
				}
				if _, err = s.treasureStrengthen(requestContext(request), account.Id, request); err == nil {
					t.Fatal("consumed original GUID replay accepted")
				}
			}
			var thresholdSum int64
			for level := int32(1); level < treasure.GetStrengthLevel(); level++ {
				exp, err := s.treasureLevelExp(treasure, level)
				if err != nil {
					t.Fatal(err)
				}
				thresholdSum += exp
			}
			if int64(treasure.GetCurrentExp()) != inputExp-thresholdSum || treasure.GetCurrentExp() <= 0 {
				t.Fatal("highest original remainder lost", config["id"], treasure.GetCurrentExp())
			}
			if err = s.treasureProperties(treasure, map[string]float64{}, map[string]float64{}); err != nil {
				t.Fatal("highest original treasure attributes", err)
			}
			reset := &pb.CStreasureRebuild{TargetId: proto.String(guid)}
			// A later bounded refund chunk must not leave the earlier chunk or
			// restored target persisted when the actual equipment bag is too small.
			if err = updateProfile(ctx, account.Id, func(p *pb.Player, _ *model.GameState) error { p.EquipBagSize = proto.Int32(110); return nil }); err != nil {
				t.Fatal(err)
			}
			beforeRefund, beforeGame, _ := read()
			if _, err = s.p5TreasureReset(requestContext(reset), account.Id, reset, 999); err == nil {
				t.Fatal("multi-chunk refund exceeded original inventory capacity")
			}
			afterFailed, afterGame, afterTreasure := read()
			if afterFailed.GetCoin() != beforeRefund.GetCoin() || afterFailed.GetDiamond() != beforeRefund.GetDiamond() || afterTreasure.GetStrengthLevel() != 100 || len(afterGame.Treasures) != 1 || p5KV(afterGame, "p5.invest:"+guid) != p5KV(beforeGame, "p5.invest:"+guid) {
				t.Fatal("later failed refund chunk partially persisted target, fee or returned instances")
			}
			if err = updateProfile(ctx, account.Id, func(p *pb.Player, _ *model.GameState) error { p.EquipBagSize = proto.Int32(1000); return nil }); err != nil {
				t.Fatal(err)
			}
			if _, err = s.p5TreasureReset(requestContext(reset), account.Id, reset, 1000); err != nil {
				t.Fatal("highest original actual investment rebuild", err)
			}
			p, g, treasure := read()
			if p.GetCoin() != initial.GetCoin() || treasure.GetStrengthLevel() != 1 || treasure.GetCurrentExp() != 0 || len(g.Treasures) != materials+1 {
				t.Fatal("highest complete actual cost refund", fmt.Sprint(materials))
			}
			resetReceipts[account.Id] = reset
		})
		count++
	}
	if count != 16 {
		t.Fatal("original usable treasure full scope drift", count)
	}
	pool, _ := db.GetIns().DB()
	pool.Close()
	open()
	for playerID, request := range resetReceipts {
		var before db.Profile
		if err := db.GetIns().First(&before, playerID).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := s.p5TreasureReset(requestContext(request), playerID, request, 1000); err != nil {
			t.Fatal("highest original treasury rebuild restart", err)
		}
		var after db.Profile
		if err := db.GetIns().First(&after, playerID).Error; err != nil || *after.Game != *before.Game || *after.Player != *before.Player {
			t.Fatal("highest original treasury rebuild replay duplicated instances", err)
		}
	}
}
