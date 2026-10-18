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
// from refine0 to refine30 and refunds exact coin, stone and material investment.
func TestP5OriginalTreasureRefineFullScope(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "treasure-refine-full.db")
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
			account, err := LoginAccount(ctx, "p5-full-treasure-refine-"+config["id"], "isolated-test-only")
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
			for treasure.GetRefineLevel() < int32(tableNumber(config, "maxRefine")) {
				level := treasure.GetRefineLevel() + 1
				costRow := s.p9TablePair("TreasureRefineLevel", "treasureId", config["id"], "level", fmt.Sprint(level))
				if err = updateProfile(ctx, account.Id, func(p *pb.Player, g *model.GameState) error {
					for _, part := range strings.Split(costRow["objStr"], ";") {
						if strings.HasPrefix(part, "coin,") {
							continue
						}
						if _, err := s.GrantFixedReward(p, g, part); err != nil {
							return err
						}
					}
					return nil
				}); err != nil {
					t.Fatal(err)
				}
				request := &pb.CSTreasureRefine{TargetId: proto.String(guid)}
				out, err := s.treasureRefine(requestContext(request), account.Id, request, int64(level))
				if err != nil {
					t.Fatal("original refinement to highest", config["id"], level, err)
				}
				result := out[0].(*pb.SCTreasureRefine)
				for _, cost := range result.GetCost() {
					if cost.GetId() == "coin" {
						spent += int64(cost.GetNum())
					}
					if cost.GetGuid() != "" {
						materials++
					}
					if cost.GetId() == "item_012" {
						inputExp += int64(cost.GetNum())
					}
				}
				p, g, value := read()
				treasure = value
				if treasure.GetRefineLevel() != level || initial.GetCoin()-p.GetCoin() != spent || len(g.Treasures) != 1 || simpleOwned(p, g, "item_012") != 0 {
					t.Fatal("exact original refinement cost or state", config["id"], level)
				}
				replay, err := s.treasureRefine(requestContext(request), account.Id, request, int64(level))
				if err != nil || !proto.Equal(out[0], replay[0]) {
					t.Fatal("original refinement receipt", err)
				}
			}
			if _, err := s.treasureRefine(ctx, account.Id, &pb.CSTreasureRefine{TargetId: proto.String(guid)}, 31); err == nil {
				t.Fatal("past highest original refinement accepted")
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
			if afterFailed.GetCoin() != beforeRefund.GetCoin() || afterFailed.GetDiamond() != beforeRefund.GetDiamond() || afterTreasure.GetRefineLevel() != 30 || len(afterGame.Treasures) != 1 || p5KV(afterGame, "p5.invest:"+guid) != p5KV(beforeGame, "p5.invest:"+guid) {
				t.Fatal("later failed refund chunk partially persisted target, fee or returned instances")
			}
			if err = updateProfile(ctx, account.Id, func(p *pb.Player, _ *model.GameState) error { p.EquipBagSize = proto.Int32(1000); return nil }); err != nil {
				t.Fatal(err)
			}
			if _, err = s.p5TreasureReset(requestContext(reset), account.Id, reset, 1000); err != nil {
				t.Fatal("highest original actual investment rebuild", err)
			}
			p, g, treasure := read()
			if p.GetCoin() != initial.GetCoin() || treasure.GetRefineLevel() != 0 || treasure.GetCurrentExp() != 0 || simpleOwned(p, g, "item_012") != inputExp || len(g.Treasures) != materials+1 {
				t.Fatal("highest complete actual cost refund", fmt.Sprint(materials))
			}
			oldRefine := &pb.CSTreasureRefine{TargetId: proto.String(guid)}
			if _, err := s.treasureRefine(requestContext(oldRefine), account.Id, oldRefine, 30); err == nil {
				t.Fatal("old refine30 client projection accepted after reset")
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
