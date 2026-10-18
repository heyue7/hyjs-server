package service

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestP8Life(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "p8-life.db")
	schema := os.Getenv("HYJS_P8_MYSQL_DATABASE")
	open := func() {
		t.Helper()
		var err error
		if schema == "" {
			err = db.InitGame(ctx, path)
		} else {
			if !strings.HasPrefix(schema, "hyjs_test_p8_") {
				t.Fatal("P8 isolated schema rejected")
			}
			err = db.InitGameMySQL(ctx, db.MySQLConfig{Host: os.Getenv("HYJS_MYSQL_HOST"), User: os.Getenv("HYJS_MYSQL_USER"), Password: os.Getenv("HYJS_MYSQL_PASSWORD"), Database: schema})
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	open()
	defer func() { pool, _ := db.GetIns().DB(); pool.Close() }()
	s := &GameService{}
	for _, load := range []func(string) error{s.LoadTables, s.LoadP5Tables, s.LoadP8Tables} {
		if err := load("../../analysis/tables"); err != nil {
			t.Fatal(err)
		}
	}
	for _, init := range []func(context.Context) error{s.InitializeP1, s.InitializeP5, s.InitializeP8Gather, s.InitializeP8Life, s.InitGems, s.InitCooking} {
		if err := init(ctx); err != nil {
			t.Fatal(err)
		}
	}
	state := func(id int64) (*pb.Player, *model.GameState, db.Profile) {
		t.Helper()
		var row db.Profile
		if err := db.GetIns().First(&row, id).Error; err != nil {
			t.Fatal(err)
		}
		p, g, err := decodeProfile(&row)
		if err != nil {
			t.Fatal(err)
		}
		return p, g, row
	}
	fresh := func(name string) int64 {
		t.Helper()
		row, err := LoginAccount(ctx, "p8-life-"+name, "isolated-password")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = CreatePlayer(ctx, row.Id, "生存隔离回归", "hero_043"); err != nil {
			t.Fatal(err)
		}
		if err = updateProfile(ctx, row.Id, func(p *pb.Player, g *model.GameState) error {
			p.Level = proto.Int32(18)
			p.Diamond = proto.Int64(100)
			_, err := s.GrantFixedReward(p, g, "coin,10000000")
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if _, err = s.fieldTransfer(ctx, row.Id, &pb.CSTransferScene{Target: proto.String("field_001")}); err != nil {
			t.Fatal(err)
		}
		return row.Id
	}
	stamp := int64(190000)
	call := func(id int64, request proto.Message) []proto.Message {
		t.Helper()
		raw, err := proto.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		b := request.ProtoReflect().Type().New().Interface()
		if err = proto.Unmarshal(raw, b); err != nil {
			t.Fatal(err)
		}
		stamp++
		out, handled, err := s.DispatchP8Life(ctx, id, b, stamp)
		if err != nil || !handled {
			t.Fatalf("%T handled=%v err=%v", request, handled, err)
		}
		return out
	}
	deny := func(id int64, request proto.Message) {
		t.Helper()
		_, _, before := state(id)
		stamp++
		if _, handled, err := s.DispatchP8Life(ctx, id, request, stamp); !handled || err == nil {
			t.Fatal("expected denial", request, err)
		}
		_, _, after := state(id)
		if *before.Player != *after.Player || *before.Game != *after.Game {
			t.Fatal("failed mutation changed profile")
		}
	}
	grant := func(id int64, resources string) {
		t.Helper()
		if err := updateProfile(ctx, id, func(p *pb.Player, g *model.GameState) error {
			for _, spec := range strings.Split(resources, ";") {
				pair := strings.Split(spec, ",")
				amount, err := strconv.ParseInt(pair[1], 10, 32)
				if err != nil {
					return err
				}
				switch {
				case strings.HasPrefix(pair[0], "MineItem_"):
					if err = s.gemInventoryDelta(p, g, map[string]int64{pair[0]: amount}); err != nil {
						return err
					}
				case strings.HasPrefix(pair[0], "FishItem_"):
					if err = s.cookingInventoryDelta(p, g, map[string]int64{pair[0]: amount}); err != nil {
						return err
					}
				default:
					if _, err = s.GrantFixedReward(p, g, spec); err != nil {
						return err
					}
				}
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	hp := func(id int64, value float64) {
		t.Helper()
		if err := updateProfile(ctx, id, func(_ *pb.Player, g *model.GameState) error {
			g.Position.Current = &pb.PointNew{X: proto.Float32(1000), Y: proto.Float32(0), Z: proto.Float32(1000)}
			g.Field.Side.CurHp = proto.Float64(value)
			g.Field.Side.BattlePosHps = nil
			g.Field.Fight = nil
			g.Field.Side.State = proto.Int32(0)
			if value == 0 {
				g.Field.Side.State = proto.Int32(5)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	book := func(id int64, box string, item string) *pb.CSItemUse {
		t.Helper()
		grant(id, box+",1")
		stamp++
		if _, err := s.p5BoxUse(ctx, id, &pb.CSItemUse{CostItem: &pb.BaseResource{Id: proto.String(box), Num: proto.Int32(1)}}, stamp); err != nil {
			t.Fatal(err)
		}
		return &pb.CSItemUse{CostItem: &pb.BaseResource{Id: proto.String(item), Num: proto.Int32(1)}}
	}
	t.Run("native-box-books-concurrent-receipt-and-reopen", func(t *testing.T) {
		id := fresh("books")
		req := book(id, "item_042", "item_063")
		stamp++
		frozen := stamp
		var wg sync.WaitGroup
		errs := make(chan error, 8)
		replies := make(chan string, 8)
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				out, _, err := s.DispatchP8Life(ctx, id, req, frozen)
				if err != nil {
					errs <- err
					return
				}
				data, err := proto.Marshal(out[0])
				if err != nil {
					errs <- err
				} else {
					replies <- string(data)
				}
			}()
		}
		wg.Wait()
		close(errs)
		close(replies)
		for err := range errs {
			t.Fatal(err)
		}
		first := ""
		for r := range replies {
			if first == "" {
				first = r
			}
			if first != r {
				t.Fatal("receipt diverged")
			}
		}
		level := call(id, &pb.CSCookingLevel{})[0].(*pb.SCCookingLevel)
		if len(level.GetSkills()) != 1 || level.GetSkills()[0] != "LifeSkill_085" {
			t.Fatal(level)
		}
		deny(id, req)
		pool, _ := db.GetIns().DB()
		pool.Close()
		open()
		out, _, err := s.DispatchP8Life(ctx, id, req, frozen)
		if err != nil {
			t.Fatal(err)
		}
		data, _ := proto.Marshal(out[0])
		if string(data) != first {
			t.Fatal("restart receipt")
		}
		call(id, book(id, "item_044", "item_064"))
		call(id, book(id, "item_043", "item_065"))
		level = call(id, &pb.CSCookingLevel{})[0].(*pb.SCCookingLevel)
		if len(level.GetSkills()) != 3 {
			t.Fatal(level)
		}
	})
	t.Run("real-book-qualification-unlocks-complete-recipes", func(t *testing.T) {
		id := fresh("recipes")
		// Finite advanced material prerequisites validate qualification and deduction;
		// their later-field gathering sources remain a separate P15 boundary.
		grant(id, "MineItem_083,30;MineItem_087,10;MineItem_084,240;FishItem_012,5;FishItem_018,5;FishItem_025,1")
		if err := db.GetIns().Save(&db.GemCraftState{PlayerID: id, Level: 7}).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.GetIns().Save(&db.CookingCraftState{PlayerID: id, Level: 6}).Error; err != nil {
			t.Fatal(err)
		}
		gem := &pb.CSGemMake{GemId: proto.String("MineItem_079"), Num: proto.Int32(1)}
		fish := &pb.CSFishCooking{Fish: &pb.BaseResource{Id: proto.String("FishItem_050"), Num: proto.Int32(1)}}
		stamp++
		if _, err := s.gemMake(ctx, id, gem, stamp); err == nil {
			t.Fatal("book gate bypassed")
		}
		stamp++
		if _, err := s.fishCooking(ctx, id, fish, stamp); err == nil {
			t.Fatal("cooking book gate bypassed")
		}
		call(id, book(id, "item_042", "item_063"))
		call(id, book(id, "item_043", "item_065"))
		stamp++
		if _, err := s.gemMake(ctx, id, gem, stamp); err != nil {
			t.Fatal(err)
		}
		stamp++
		if _, err := s.fishCooking(ctx, id, fish, stamp); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("real-dish-food-heal-cooldown-and-replay", func(t *testing.T) {
		id := fresh("food")
		grant(id, "FishItem_007,10")
		hp(id, 0.4)
		stamp++
		if _, err := s.fishCooking(ctx, id, &pb.CSFishCooking{Fish: &pb.BaseResource{Id: proto.String("FishItem_026"), Num: proto.Int32(2)}}, stamp); err != nil {
			t.Fatal(err)
		}
		req := &pb.CSHpRecover{CostItem: &pb.BaseResource{Id: proto.String("FishItem_026"), Num: proto.Int32(1)}}
		out := call(id, req)
		frozen := stamp
		if len(out[0].(*pb.SCHpRecover).GetCost()) != 1 {
			t.Fatal(out)
		}
		_, g, saved := state(id)
		if math.Abs(g.Field.Side.GetCurHp()-0.45) > 1e-8 {
			t.Fatal(g.Field.Side)
		}
		deny(id, req)
		pool, _ := db.GetIns().DB()
		pool.Close()
		open()
		if _, _, err := s.DispatchP8Life(ctx, id, req, frozen); err != nil {
			t.Fatal(err)
		}
		_, _, after := state(id)
		if *saved.Game != *after.Game || *saved.Player != *after.Player {
			t.Fatal("replay repeated heal")
		}
		if err := db.GetIns().Model(&db.P8LifeState{}).Where("player_id=?", id).Update("food_ready_at", time.Now().UnixMilli()-1).Error; err != nil {
			t.Fatal(err)
		}
		call(id, req)
		_, g, _ = state(id)
		if math.Abs(g.Field.Side.GetCurHp()-0.5) > 1e-8 {
			t.Fatal(g.Field.Side)
		}
	})
	t.Run("native-shortcuts-cooldown-potion-and-reopen", func(t *testing.T) {
		id := fresh("shortcuts")
		call(id, &pb.CSSideSetShortCut{Position: proto.Int32(1), Id: proto.String("FishItem_026")})
		call(id, &pb.CSSideSetShortCut{Position: proto.Int32(2), Id: proto.String("item_068")})
		deny(id, &pb.CSSideSetShortCut{Position: proto.Int32(0), Id: proto.String("item_068")})
		deny(id, &pb.CSSideSetShortCut{Position: proto.Int32(1), Id: proto.String("FishItem_027")})
		list := call(id, &pb.CSGemAndCookingList{})[0].(*pb.SCGemAndCookingList)
		if list.GetShortCut1() != "FishItem_026" || list.GetShortCut2() != "item_068" || list.GetNextUseTime() != 0 {
			t.Fatal(list)
		}
		hp(id, 0.2)
		grant(id, "item_068,1")
		call(id, &pb.CSHpRecover{CostItem: &pb.BaseResource{Id: proto.String("item_068"), Num: proto.Int32(1)}})
		_, g, _ := state(id)
		if math.Abs(g.Field.Side.GetCurHp()-0.5) > 1e-8 {
			t.Fatal(g.Field.Side)
		}
		list = call(id, &pb.CSGemAndCookingList{})[0].(*pb.SCGemAndCookingList)
		if list.GetNextUseTime() <= time.Now().UnixMilli()+59000 {
			t.Fatal("original millisecond cooldown", list)
		}
		pool, _ := db.GetIns().DB()
		pool.Close()
		open()
		loaded := call(id, &pb.CSGemAndCookingList{})[0].(*pb.SCGemAndCookingList)
		if !proto.Equal(list, loaded) {
			t.Fatal("shortcut/cooldown restart", loaded)
		}
		call(id, &pb.CSSideSetShortCut{Position: proto.Int32(1), Id: proto.String("")})
		if call(id, &pb.CSGemAndCookingList{})[0].(*pb.SCGemAndCookingList).GetShortCut1() != "" {
			t.Fatal("clear shortcut")
		}
	})
	t.Run("paid-revive-native-cost-exact-receipt-and-protection", func(t *testing.T) {
		id := fresh("paid")
		deny(id, &pb.CSResurrection{})
		hp(id, 0)
		out := call(id, &pb.CSResurrection{})[0].(*pb.SCResurrenttion)
		frozen := stamp
		p, g, saved := state(id)
		if p.GetDiamond() != 50 || g.Field.Side.GetCurHp() != 1 || out.GetCost()[0].GetNum() != 50 {
			t.Fatal(p, g.Field.Side, out)
		}
		active, err := P8IsInvincible(db.GetIns(), id, time.Now())
		if err != nil || !active {
			t.Fatal("native revival protection", err)
		}
		if _, _, err := s.DispatchP8Life(ctx, id, &pb.CSResurrection{}, frozen); err != nil {
			t.Fatal(err)
		}
		_, _, after := state(id)
		if *saved.Player != *after.Player || *saved.Game != *after.Game {
			t.Fatal("repeated paid revival")
		}
		active, err = P8IsInvincible(db.GetIns(), id, time.Now().Add(6*time.Second))
		if err != nil || active {
			t.Fatal("protection expiry", err)
		}
		hp(id, 0)
		if err := updateProfile(ctx, id, func(p *pb.Player, _ *model.GameState) error { p.Diamond = proto.Int64(49); return nil }); err != nil {
			t.Fatal(err)
		}
		deny(id, &pb.CSResurrection{})
	})
	t.Run("ten-second-free-revive-original-safe-point", func(t *testing.T) {
		id := fresh("free")
		hp(id, 0)
		call(id, &pb.CSGetHp{})
		deny(id, &pb.CSTransfer2Safe{})
		if err := db.GetIns().Model(&db.P8LifeState{}).Where("player_id=?", id).Update("death_at", time.Now().UnixMilli()-10001).Error; err != nil {
			t.Fatal(err)
		}
		call(id, &pb.CSTransfer2Safe{})
		p, g, _ := state(id)
		point, err := fieldPoint(s.table("Field", "FieldId", "field_001")["Resurrection"])
		if err != nil {
			t.Fatal(err)
		}
		if p.GetDiamond() != 100 || math.Abs(g.Field.Side.GetCurHp()-0.1) > 1e-8 || !proto.Equal(point, g.Position.GetCurrent()) || g.Field.Side.GetState() != 0 {
			t.Fatal(p, g.Position, g.Field.Side)
		}
	})
	t.Run("safe-region-clock-no-outside-accrual", func(t *testing.T) {
		id := fresh("safe")
		hp(id, 0.3)
		now := time.Now()
		if err := updateProfileWithTx(ctx, id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
			g.Position.Current, _ = fieldPoint(s.table("Field", "FieldId", "field_001")["Resurrection"])
			if _, err := s.RefreshP8Survival(tx, p, g, now); err != nil {
				return err
			}
			if _, err := s.RefreshP8Survival(tx, p, g, now.Add(20*time.Second)); err != nil {
				return err
			}
			if math.Abs(g.Field.Side.GetCurHp()-0.5) > 1e-8 {
				t.Fatal(g.Field.Side)
			}
			g.Position.Current = &pb.PointNew{X: proto.Float32(1000), Y: proto.Float32(0), Z: proto.Float32(1000)}
			if _, err := s.RefreshP8Survival(tx, p, g, now.Add(21*time.Second)); err != nil {
				return err
			}
			g.Position.Current, _ = fieldPoint(s.table("Field", "FieldId", "field_001")["Resurrection"])
			_, err := s.RefreshP8Survival(tx, p, g, now.Add(time.Hour))
			return err
		}); err != nil {
			t.Fatal(err)
		}
		_, g, _ := state(id)
		if math.Abs(g.Field.Side.GetCurHp()-0.5) > 1e-8 {
			t.Fatal("outside accrual", g.Field.Side)
		}
	})
}
