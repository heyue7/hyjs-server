package service

import (
	"context"
	"math"
	"strconv"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// TestCookingOriginalFullScope reaches level six through actual crafting and
// consumes every original craftable dish. Ingredient/coin/box balances and
// injury/cooldown are explicit finite prerequisites, not natural-source proof.
func TestCookingOriginalFullScope(t *testing.T) {
	s, id, path := gemFixture(t, false)
	base := context.Background()
	if err := s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, init := range []func(context.Context) error{s.InitializeP1, s.InitializeP5, s.InitializeP8Gather, s.InitializeP8Life, s.InitCooking} {
		if err := init(base); err != nil {
			t.Fatal(err)
		}
	}
	s.P5Ready = true
	if err := updateProfile(base, id, func(p *pb.Player, g *model.GameState) error {
		p.Coin = proto.Int64(1000000000)
		g.Cooking = nil
		for _, row := range s.Tables["FishSkill"] {
			if row["Type"] == "fish" {
				g.Cooking = append(g.Cooking, &pb.SimpleItem{Id: proto.String(row["FishItemID"]), Num: proto.Int32(10000)})
			}
		}
		_, err := s.GrantFixedReward(p, g, "item_043,1")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	stamp := int64(1000000)
	contextFor := func(request proto.Message) context.Context {
		return context.WithValue(context.WithValue(base, constUtil.GameServiceContextKey, s), constUtil.GameRequestContextKey, request)
	}
	makeDish := func(key string, count int32) *pb.SCFishCooking {
		t.Helper()
		stamp++
		req := &pb.CSFishCooking{Fish: &pb.BaseResource{Id: proto.String(key), Num: proto.Int32(count)}}
		out, err := s.fishCooking(contextFor(req), id, req, stamp)
		if err != nil {
			t.Fatal("actual original craft", key, err)
		}
		return out[0].(*pb.SCFishCooking)
	}
	for _, step := range []struct {
		key   string
		count int32
		level int32
	}{{"FishItem_026", 24, 2}, {"FishItem_030", 167, 3}, {"FishItem_034", 500, 4}, {"FishItem_038", 500, 5}, {"FishItem_042", 480, 6}} {
		out := makeDish(step.key, step.count)
		if out.GetLevel() != step.level {
			t.Fatal("actual cooking level transition", step, out)
		}
	}
	level, exp, err := s.CookingCraftLevel(base, id)
	if err != nil || level != 6 || exp != 0 {
		t.Fatal("actual highest original cooking sentinel", level, exp, err)
	}
	blocked := &pb.CSFishCooking{Fish: &pb.BaseResource{Id: proto.String("FishItem_050"), Num: proto.Int32(1)}}
	stamp++
	if _, err := s.fishCooking(contextFor(blocked), id, blocked, stamp); err == nil {
		t.Fatal("advanced original book gate bypassed")
	}
	stamp++
	if _, err := s.p5BoxUse(base, id, &pb.CSItemUse{CostItem: &pb.BaseResource{Id: proto.String("item_043"), Num: proto.Int32(1)}}, stamp); err != nil {
		t.Fatal(err)
	}
	stamp++
	book := &pb.CSItemUse{CostItem: &pb.BaseResource{Id: proto.String("item_065"), Num: proto.Int32(1)}}
	if _, handled, err := s.DispatchP8Life(base, id, book, stamp); err != nil || !handled {
		t.Fatal("actual original box/book qualification", err)
	}
	if _, err := s.fieldTransfer(base, id, &pb.CSTransferScene{Target: proto.String("field_001")}); err != nil {
		t.Fatal(err)
	}
	var rule db.CookingRule
	if err := db.GetIns().First(&rule, 1).Error; err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, row := range s.Tables["FishSkill"] {
		if row["Type"] != "dish" || row["IsDish"] != "1" || row["Material"] == "" {
			continue
		}
		count++
		key := row["FishItemID"]
		made := makeDish(key, 1)
		if made.GetLevel() != 6 || made.GetExp() != 0 {
			t.Fatal("terminal cooking was upgraded beyond original six", key)
		}
		p, g := gemProfile(t, id)
		warrior, err := p5Warrior(g, p.GetHeroWarId())
		if err != nil {
			t.Fatal(err)
		}
		before := cookingProperties(warrior.GetCooking().GetProp())
		stamp++
		if row["ValueType"] == "onetimeRecover" {
			if err := updateProfile(base, id, func(_ *pb.Player, g *model.GameState) error {
				g.Position.Current = &pb.PointNew{X: proto.Float32(1000), Z: proto.Float32(1000)}
				g.Field.Side.CurHp = proto.Float64(0.1)
				g.Field.Side.State = proto.Int32(0)
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
				t.Fatal("actual original heal food", key, err)
			}
			_, g = gemProfile(t, id)
			percent, _ := strconv.ParseFloat(row["Value"], 64)
			if math.Abs(g.Field.Side.GetCurHp()-(0.1+percent/100)) > 1e-9 {
				t.Fatal("original heal percent", key, g.Field.Side.GetCurHp())
			}
		} else {
			food := &pb.CSFishUsing{HeroGuid: proto.String(p.GetHeroWarId()), CostItem: &pb.BaseResource{Id: proto.String(key), Num: proto.Int32(1)}}

			used, err := s.fishUsing(contextFor(food), id, food, stamp)
			if err != nil {
				t.Fatal("actual original use", key, err)
			}
			if strings.HasPrefix(row["ValueType"], "permanent") {
				gains, _, err := s.cookingPermanentPolicy(&rule, row)
				if err != nil {
					t.Fatal(err)
				}
				after := cookingProperties(used[0].(*pb.SCFishUsing).GetWarrior().GetCooking().GetProp())
				expected := map[string]bool{}
				for _, attr := range strings.Split(row["Value"], ";") {
					expected[attr] = true
				}
				for attr, value := range after {
					delta := 0.0
					if expected[attr] {
						delta = gains[attr]
					}
					if math.Abs(value-before[attr]-delta) > 1e-9 {
						t.Fatal("declared real permanent effect", key, attr)
					}
				}
			} else {
				effect, err := cookingEffect(db.GetIns(), id, time.Now().UnixMilli())
				if err != nil || effect == nil || effect.ItemID != key || effect.Kind != row["ValueType"] {
					t.Fatal("original timed effect", key, err)
				}
				parts := strings.Split(row["Value"], ";")
				percent, _ := strconv.ParseFloat(parts[1], 64)
				if math.Abs(effect.Value-percent/100) > 1e-9 {
					t.Fatal("original timed reward percent", key)
				}
			}
			repeated, err := s.fishUsing(contextFor(food), id, food, stamp)
			if err != nil || !proto.Equal(used[0], repeated[0]) {
				t.Fatal("unchanged same original use", key, err)
			}
		}
	}
	if count != 25 {
		t.Fatal("whole original craftable dish range", count)
	}

	// Original return meal051 has no recipe/source in recovered tables.
	// Explicit finite inventory validates its distinct use without inventing
	// a natural award or allowing it into the craft catalogue.
	stamp++
	nonRecipe := &pb.CSFishCooking{Fish: &pb.BaseResource{Id: proto.String("FishItem_051"), Num: proto.Int32(1)}}
	if _, err := s.fishCooking(contextFor(nonRecipe), id, nonRecipe, stamp); err == nil {
		t.Fatal("return meal without recipe became craftable")
	}
	if err := updateProfile(base, id, func(p *pb.Player, g *model.GameState) error {
		return s.cookingInventoryDelta(p, g, map[string]int64{"FishItem_051": 1})
	}); err != nil {
		t.Fatal(err)
	}
	player, _ := gemProfile(t, id)
	stamp++
	returnFood := &pb.CSFishUsing{HeroGuid: proto.String(player.GetHeroWarId()), CostItem: &pb.BaseResource{Id: proto.String("FishItem_051"), Num: proto.Int32(1)}}
	began := time.Now().UnixMilli()
	if _, err := s.fishUsing(contextFor(returnFood), id, returnFood, stamp); err != nil {
		t.Fatal("original return meal use", err)
	}
	effect, err := cookingEffect(db.GetIns(), id, began)
	if err != nil || effect == nil || effect.ItemID != "FishItem_051" || effect.Kind != "extraExp" || effect.Value != 3 || effect.EndAt < began+1440*60000 {
		t.Fatal("original return meal300percent/1440minutes", err)
	}
	pool, _ := db.GetIns().DB()
	pool.Close()
	if err := db.InitGame(base, path); err != nil {
		t.Fatal(err)
	}
	level, exp, err = s.CookingCraftLevel(base, id)
	if err != nil || level != 6 || exp != 0 {
		t.Fatal("cold highest cooking", err)
	}
	t.Logf("all25 original recipes crafted and consumed, including user-approved golden80 caps; cooking1→6 and original book gate; permanent policy: %s", rule.Source)
}
