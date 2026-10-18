package service

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// TestP8PackagedCookingSource acquires original first/second-map ingredients
// through real frozen gathering before craft/use. Level/coin and cooking-level3
// are declared finite qualifications. Original200 fatigue remains, first gather
// per map waits its real clock; later ready/rebirth rows are isolated accelerated
// boundaries. No fish/dish reward is prepared, and missing maps remain gated.
func TestP8PackagedCookingSource(t *testing.T) {
	if os.Getenv("HYJS_P8_PACKAGED_COOKING_SOURCE") != "1" {
		t.Skip("explicit bounded real original packaged-map source chain")
	}
	base := context.Background()
	records := []map[string]any{}
	for dish := 26; dish <= 37; dish++ {
		scene := "field_001"
		if dish >= 30 {
			scene = "field_002"
		}
		targetRecipe := "FishItem_0" + strconv.Itoa(dish)
		t.Run(scene+"/"+targetRecipe, func(t *testing.T) {
			s, id, path := gemFixture(t, false)
			for _, load := range []func(string) error{s.LoadP5Tables, s.LoadP8Tables} {
				if err := load("../../analysis/tables"); err != nil {
					t.Fatal(err)
				}
			}
			for _, init := range []func(context.Context) error{s.InitializeP1, s.InitializeP5, s.InitializeP8Gather, s.InitializeP8Life, s.InitCooking} {
				if err := init(base); err != nil {
					t.Fatal(err)
				}
			}
			s.P5Ready = true
			if err := updateProfile(base, id, func(p *pb.Player, g *model.GameState) error {
				p.Level = proto.Int32(100)
				p.Coin = proto.Int64(1000000000)
				g.Cooking = nil
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if err := db.GetIns().Save(&db.CookingCraftState{PlayerID: id, Level: 3}).Error; err != nil {
				t.Fatal(err)
			}
			if _, err := s.fieldTransfer(base, id, &pb.CSTransferScene{Target: proto.String(scene)}); err != nil {
				t.Fatal("original packaged field transfer", err)
			}
			var recipes []map[string]string
			needed := map[string]int64{}
			for _, row := range s.Tables["FishSkill"] {
				level := int(tableNumber(row, "LevelLimit"))
				if row["Type"] != "dish" || row["Material"] == "" || row["FishItemID"] != targetRecipe || level < 1 || level > 3 {
					continue
				}
				recipes = append(recipes, row)
				for _, part := range strings.Split(row["Material"], ";") {
					pair := strings.Split(part, ",")
					n, _ := strconv.ParseInt(pair[1], 10, 32)
					if pair[0] != "coin" {
						needed[pair[0]] += n
					}
				}
			}
			stamp := int64(12000000)
			contextFor := func(req proto.Message) context.Context {
				return context.WithValue(context.WithValue(base, constUtil.GameServiceContextKey, s), constUtil.GameRequestContextKey, req)
			}
			count := func(g *model.GameState, key string) int64 {
				for _, item := range g.Cooking {
					if item.GetId() == key {
						return int64(item.GetNum())
					}
				}
				return 0
			}
			ready := func(g *model.GameState) bool {
				for key, n := range needed {
					if count(g, key) < n {
						return false
					}
				}
				return true
			}
			gathers := 0
			for ; gathers < 100; gathers++ {
				_, game := gemProfile(t, id)
				if ready(game) {
					break
				}
				if err := updateProfileWithTx(base, id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
					_, err := s.RefreshP8Resources(tx, p, g, time.Now(), true)
					return err
				}); err != nil {
					t.Fatal(err)
				}
				rows, err := P8GatherNodes(db.GetIns(), id)
				if err != nil {
					t.Fatal(err)
				}
				var selected *db.P8ResourceNode
				best := -1
				for i := range rows {
					node := &rows[i]
					cfg, err := p8DecodeConfig(node.Snapshot)
					if err != nil {
						t.Fatal(err)
					}
					if node.Scene != scene || cfg.Kind != "fish" || node.Remaining == 0 {
						continue
					}
					missing := 0
					for _, entry := range strings.Split(cfg.Reward, "|") {
						key := strings.Split(entry, ":")[0]
						if needed[key] > count(game, key) {
							missing++
						}
					}
					if missing > best {
						selected = node
						best = missing
					}
				}
				if selected == nil {
					if err := db.GetIns().Model(&db.P8ResourceNode{}).Where("player_id=? AND scene=? AND remaining=0", id, scene).Update("refresh_at", time.Now().UnixMilli()-1).Error; err != nil {
						t.Fatal(err)
					}
					gathers--
					continue
				}
				cfg, _ := p8DecodeConfig(selected.Snapshot)
				if err := updateProfile(base, id, func(_ *pb.Player, g *model.GameState) error {
					g.Position.Current = proto.Clone(cfg.Point).(*pb.PointNew)
					return nil
				}); err != nil {
					t.Fatal(err)
				}
				stamp++
				req := &pb.CSFieldResourceStartGather{PosId: proto.String(selected.PosID)}
				out, err := s.p8StartGather(contextFor(req), id, req, stamp)
				if err != nil || out[0].(*pb.SCFieldResourceStartGather).GetResult() != 1 {
					t.Fatal("actual original fish start", err)
				}
				if gathers == 0 {
					time.Sleep(time.Duration(cfg.GatherMS+50) * time.Millisecond)
				} else if err := db.GetIns().Model(&db.P8GatherAttempt{}).Where("player_id=?", id).Update("ready_at", time.Now().UnixMilli()-1).Error; err != nil {
					t.Fatal(err)
				}
				stamp++
				claim := &pb.CSFieldGetResource{}
				claimed, err := s.p8ClaimGather(contextFor(claim), id, claim, stamp)
				if err != nil || claimed[0].(*pb.SCFieldGetResource).GetCode() != 1 {
					t.Fatal("actual original fish claim", err)
				}
				if selected.Remaining == 1 {
					if err := db.GetIns().Model(&db.P8ResourceNode{}).Where("player_id=? AND scene=? AND pos_id=?", id, scene, selected.PosID).Update("refresh_at", time.Now().UnixMilli()-1).Error; err != nil {
						t.Fatal(err)
					}
				}
			}
			player, game := gemProfile(t, id)
			if !ready(game) {
				t.Fatal("original200 fatigue bounded random source incomplete; no extra attempt or prepared material", scene, targetRecipe, gathers)
			}
			if game.Field.Side.GetTiredness() != 200-int32(2*gathers) {
				t.Fatal("original actual source fatigue was reset", gathers, game.Field.Side.GetTiredness())
			}
			for _, recipe := range recipes {
				key := recipe["FishItemID"]
				stamp++
				req := &pb.CSFishCooking{Fish: &pb.BaseResource{Id: proto.String(key), Num: proto.Int32(1)}}
				out, err := s.fishCooking(contextFor(req), id, req, stamp)
				if err != nil || out[0].(*pb.SCFishCooking).GetRes().GetSimps()[0].GetNum() != 1 {
					t.Fatal("actual natural ingredients craft", key, err)
				}
				player, game = gemProfile(t, id)
				warrior, _ := p5Warrior(game, player.GetHeroWarId())
				before := cookingProperties(warrior.GetCooking().GetProp())
				stamp++
				if recipe["ValueType"] == "onetimeRecover" {
					if err := updateProfile(base, id, func(_ *pb.Player, g *model.GameState) error {
						g.Position.Current = &pb.PointNew{X: proto.Float32(1000), Z: proto.Float32(1000)}
						g.Field.Side.CurHp = proto.Float64(.1)
						g.Field.Side.BattlePosHps = nil
						return nil
					}); err != nil {
						t.Fatal(err)
					}
					if err := db.GetIns().Model(&db.P8LifeState{}).Where("player_id=?", id).Update("food_ready_at", 0).Error; err != nil {
						t.Fatal(err)
					}
					food := &pb.CSHpRecover{CostItem: &pb.BaseResource{Id: proto.String(key), Num: proto.Int32(1)}}
					if _, err := s.p8HpRecover(contextFor(food), id, food, stamp); err != nil {
						t.Fatal(key, err)
					}
					_, game = gemProfile(t, id)
					percent, _ := strconv.ParseFloat(recipe["Value"], 64)
					if math.Abs(game.Field.Side.GetCurHp()-.1-percent/100) > 1e-9 {
						t.Fatal("natural food heal effect", key)
					}
				} else {
					food := &pb.CSFishUsing{HeroGuid: proto.String(player.GetHeroWarId()), CostItem: &pb.BaseResource{Id: proto.String(key), Num: proto.Int32(1)}}
					used, err := s.fishUsing(contextFor(food), id, food, stamp)
					if err != nil {
						t.Fatal("natural cooked food use", key, err)
					}
					if strings.HasPrefix(recipe["ValueType"], "permanent") {
						after := cookingProperties(used[0].(*pb.SCFishUsing).GetWarrior().GetCooking().GetProp())
						for _, attr := range strings.Split(recipe["Value"], ";") {
							if after[attr] <= before[attr] {
								t.Fatal("natural permanent effect missing", key, attr)
							}
						}
					} else {
						effect, err := cookingEffect(db.GetIns(), id, time.Now().UnixMilli())
						if err != nil || effect == nil || effect.ItemID != key {
							t.Fatal("natural timed reward effect", key, err)
						}
					}
				}
				_, game = gemProfile(t, id)
				if count(game, key) != 0 {
					t.Fatal("natural actual consumption did not remove one serving", key)
				}
			}
			pool, _ := db.GetIns().DB()
			pool.Close()
			if err := db.InitGame(base, path); err != nil {
				t.Fatal(err)
			}
			_, game = gemProfile(t, id)
			if game.Field.Scene != scene || game.Field.Side.GetTiredness() != 200-int32(2*gathers) {
				t.Fatal("cold source scene/fatigue changed")
			}
			records = append(records, map[string]any{"scene": scene, "recipe": targetRecipe, "actualGathers": gathers, "actualFatigueCost": 2 * gathers, "actualRecipes": len(recipes), "targetIngredientsPrepared": false, "coldSceneAndFatiguePreserved": true})
		})
	}
	if output := os.Getenv("HYJS_P8_SOURCE_OUTPUT"); output != "" {
		raw, _ := json.MarshalIndent(map[string]any{"scope": "two actual packaged original scenes;12 independent legitimate profiles, each original200 fatigue; random source ->12 original recipes ->real consumption/effect", "cases": records, "phoneVerified": false, "higherMapsUnavailable": []string{"field_003", "field_004", "field_005", "field_006"}, "boundaries": "finite character100/cooking3/coin qualifications and injury; first real5s gather per map, later isolated ready/rebirth/CD acceleration; original200 daily fatigue never reset; local SQL permanent gain compatibility unchanged"}, "", "  ")
		if err := os.WriteFile(output, append(raw, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
	}
}
