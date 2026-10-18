package service

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p5LifeItemCount(g *model.GameState, id string) int32 {
	var count int32
	for _, item := range g.Items {
		if item.GetId() == id {
			count += item.GetNum()
		}
	}
	return count
}

// TestP5LifeBoxes 通过真实角色行锁验证原箱、生活库存、镶嵌用途及回执恢复。
func TestP5LifeBoxes(t *testing.T) {
	ctx := context.Background()
	schema := os.Getenv("HYJS_P5_LIFE_MYSQL_DATABASE")
	path := filepath.Join(t.TempDir(), "life-box.db")
	open := func() {
		t.Helper()
		var err error
		if schema == "" {
			err = db.InitGame(ctx, path)
		} else {
			if !strings.HasPrefix(schema, "hyjs_test_p5life_") {
				t.Fatal("isolated life box schema required")
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
	if err := s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, init := range []func(context.Context) error{s.InitializeP1, s.InitializeP5, s.InitGems, s.InitCooking} {
		if err := init(ctx); err != nil {
			t.Fatal(err)
		}
	}
	state := func(id int64) (*pb.Player, *model.GameState) {
		t.Helper()
		var row db.Profile
		if err := db.GetIns().First(&row, id).Error; err != nil {
			t.Fatal(err)
		}
		p, g, err := decodeProfile(&row)
		if err != nil {
			t.Fatal(err)
		}
		return p, g
	}
	fresh := func(name, resources string) int64 {
		t.Helper()
		row, err := LoginAccount(ctx, "p5-life-"+name, "isolated-password")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = CreatePlayer(ctx, row.Id, "原宝石箱", "hero_043"); err != nil {
			t.Fatal(err)
		}
		if err = updateProfile(ctx, row.Id, func(p *pb.Player, g *model.GameState) error {
			p.Level = proto.Int32(75)
			_, e := s.GrantFixedReward(p, g, resources)
			return e
		}); err != nil {
			t.Fatal(err)
		}
		return row.Id
	}
	stamp := int64(9840000)
	use := func(id int64, box string, num, index int32, timestamp int64) ([]proto.Message, error) {
		return s.p5BoxUse(ctx, id, &pb.CSItemUse{CostItem: &pb.BaseResource{Id: proto.String(box), Num: proto.Int32(num)}, GetItemIndex: proto.Int32(index)}, timestamp)
	}
	t.Run("all-original-normal-box-rows-and-selection-colours", func(t *testing.T) {
		for _, box := range []string{"item_076", "item_123", "item_137", "item_342", "item_343", "item_344", "item_345"} {
			id := fresh(box, box+",100")
			item := s.table("Item", "itemId", box)
			// 原每个结果均通过统一奖励，再按原选择索引或随机权重实际消费箱。
			for _, option := range s.Tables["ItemUse"] {
				if option["itemId"] == box {
					p, g := state(id)
					if _, err := s.GrantFixedReward(p, g, option["result"]); err != nil {
						t.Fatal(box, option["id"], err)
					}
				}
			}
			indices := []int32{0}
			if item["category"] == "5" {
				indices = []int32{1, 2, 3, 4, 5, 6}
			}
			for _, index := range indices {
				stamp++
				messages, err := use(id, box, 1, index, stamp)
				if err != nil {
					t.Fatal(box, index, err)
				}
				gain := messages[0].(*pb.SCItemUse).GetGet()
				// 原随机箱同时有金币、伙伴经验与饰品，不以单一宝石结果替代原奖池。
				counts := map[string]int32{}
				for _, v := range gain.GetSimps() {
					counts[v.GetId()] += v.GetNum()
				}
				for _, v := range gain.GetWars() {
					counts[v.GetId()]++
				}
				for _, v := range gain.GetTreas() {
					counts[v.GetTreasureId()]++
				}
				matched := false
				for _, r := range s.Tables["ItemUse"] {
					if r["itemId"] != box {
						continue
					}
					parts := strings.Split(r["result"], ",")
					if len(parts) != 2 {
						continue
					}
					amount, _ := strconv.ParseInt(parts[1], 10, 32)
					if len(counts) == 1 && counts[parts[0]] == int32(amount) {
						matched = true
					}
				}
				if !matched {
					t.Fatal("output is absent from original box pool", box, gain)
				}
				_, g := state(id)
				for key, amount := range counts {
					if strings.HasPrefix(key, "MineItem_") && (gemCount(g, key) != amount || p5LifeItemCount(g, key) != 0) {
						t.Fatal("reward landed in wrong inventory", box)
					}
				}
			}
		}
	})
	t.Run("concurrent-choice-to-mount-unmount-and-restart-receipt", func(t *testing.T) {
		id := fresh("concurrent", "item_342,1;equip_037,1")
		const timestamp = int64(9899000)
		var wg sync.WaitGroup
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				r, e := use(id, "item_342", 1, 1, timestamp)
				if e != nil || r[0].(*pb.SCItemUse).GetGet().Simps[0].GetId() != "MineItem_025" {
					t.Errorf("concurrent original box receipt: %v", e)
				}
			}()
		}
		wg.Wait()
		_, g := state(id)
		if gemCount(g, "MineItem_025") != 1 || p5LifeItemCount(g, "item_342") != 0 {
			t.Fatal("concurrent box duplicated or not consumed")
		}
		guid := g.Equipment[0].GetGuid()
		if _, err := s.gemMount(ctx, id, &pb.CSGemMount{Guid: proto.String(guid), GemId: proto.String("MineItem_025"), Pos: proto.Int32(0)}, timestamp+1); err != nil {
			t.Fatal("actual box gem must be mountable", err)
		}
		_, g = state(id)
		if gemCount(g, "MineItem_025") != 0 || g.Equipment[0].GetGems()[0] != "MineItem_025" {
			t.Fatal("physical gem mounting did not transfer ownership")
		}
		pool, _ := db.GetIns().DB()
		pool.Close()
		open()
		if _, err := use(id, "item_342", 1, 1, timestamp); err != nil {
			t.Fatal("persisted original receipt", err)
		}
		_, g = state(id)
		if gemCount(g, "MineItem_025") != 0 || g.Equipment[0].GetGems()[0] != "MineItem_025" {
			t.Fatal("old reply manufactured another physical gem")
		}
	})
	t.Run("invalid-index-overflow-duplicates-and-mixed-memory-rollback", func(t *testing.T) {
		id := fresh("reject", "item_342,5")
		if _, err := use(id, "item_342", 1, 7, 9899100); err == nil {
			t.Fatal("invalid selection accepted")
		}
		if err := updateProfile(ctx, id, func(p *pb.Player, g *model.GameState) error {
			g.Gems = []*pb.SimpleItem{{Id: proto.String("MineItem_025"), Num: proto.Int32(math.MaxInt32)}}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := use(id, "item_342", 1, 1, 9899101); err == nil {
			t.Fatal("overflow accepted")
		}
		p, g := state(id)
		if p5LifeItemCount(g, "item_342") != 5 || gemCount(g, "MineItem_025") != math.MaxInt32 {
			t.Fatal("failed box consumed inventory")
		}
		before, _ := json.Marshal(g)
		coin := p.GetCoin()
		if _, err := s.GrantFixedReward(p, g, "coin,7;FishItem_007,2;MineItem_025,1"); err == nil {
			t.Fatal("mixed overflow accepted")
		}
		after, _ := json.Marshal(g)
		if p.GetCoin() != coin || string(before) != string(after) {
			t.Fatal("mixed failed reward changed live memory")
		}
		g.Gems = append(g.Gems, proto.Clone(g.Gems[0]).(*pb.SimpleItem))
		if _, err := s.GrantFixedReward(p, g, "MineItem_019,1"); err == nil {
			t.Fatal("duplicate inventory accepted")
		}
	})
}
