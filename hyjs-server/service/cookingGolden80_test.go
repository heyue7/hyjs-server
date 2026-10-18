package service

import (
	"context"
	"encoding/json"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestCookingGolden80AccumulatedCap(t *testing.T)      { cookingGolden80AccumulatedCap(t, false) }
func TestCookingGolden80AccumulatedCapMySQL(t *testing.T) { cookingGolden80AccumulatedCap(t, true) }

// cookingGolden80AccumulatedCap checks a final serving across prior mixed-dish totals,
// then rejects a whole meal if any affected attribute is full, without a second consumption.
func cookingGolden80AccumulatedCap(t *testing.T, mysql bool) {
	s, id, _ := gemFixture(t, mysql)
	ctx := context.Background()
	if err := s.InitCooking(ctx); err != nil {
		t.Fatal(err)
	}
	var rule db.CookingRule
	if err := db.GetIns().First(&rule, 1).Error; err != nil {
		t.Fatal(err)
	}
	want := map[string]float64{"att": 160000, "hp": 608000, "pdef": 48000, "mdef": 48000, "cri": 240, "antiCri": 240}
	gains, caps, err := s.cookingPermanentPolicy(&rule, s.table("FishSkill", "FishItemID", "FishItem_050"))
	if err != nil || len(caps) != 6 {
		t.Fatal("golden80 default", err)
	}
	for key, limit := range want {
		if caps[key] != limit || gains[key]*80 != limit {
			t.Fatalf("%s gain=%v cap=%v", key, gains[key], caps[key])
		}
	}
	p, _ := gemProfile(t, id)
	if err := updateProfile(ctx, id, func(_ *pb.Player, g *model.GameState) error {
		for _, w := range g.Warriors {
			if w.GetGuid() == p.GetHeroWarId() {
				w.Cooking = &pb.Cooking{Prop: &pb.BaseProp{Att: proto.Float64(159999), Hp: proto.Float64(607999), Pdef: proto.Float64(47999), Mdef: proto.Float64(47999), Cri: proto.Float64(239), AntiCri: proto.Float64(239)}}
			}
		}
		g.Cooking = append(g.Cooking, &pb.SimpleItem{Id: proto.String("FishItem_050"), Num: proto.Int32(2)})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	req := &pb.CSFishUsing{HeroGuid: p.HeroWarId, CostItem: &pb.BaseResource{Id: proto.String("FishItem_050"), Num: proto.Int32(1)}}
	out, err := s.fishUsing(ctx, id, req, 990001)
	if err != nil {
		t.Fatal(err)
	}
	got := cookingProperties(out[0].(*pb.SCFishUsing).GetWarrior().GetCooking().GetProp())
	for key, limit := range want {
		if got[key] != limit {
			t.Fatalf("%s accumulated=%v want=%v", key, got[key], limit)
		}
	}
	beforeP, beforeG := gemProfile(t, id)
	beforeRaw, _ := json.Marshal(beforeG)
	if _, err := s.fishUsing(ctx, id, req, 990002); err == nil {
		t.Fatal("golden final cap bypassed")
	}
	afterP, afterG := gemProfile(t, id)
	afterRaw, _ := json.Marshal(afterG)
	if !proto.Equal(beforeP, afterP) || string(beforeRaw) != string(afterRaw) {
		t.Fatal("full golden meal consumed inventory or changed profile")
	}
	// Lower the other five fixture attributes; a single capped attack still blocks the full meal.
	if err := updateProfile(ctx, id, func(_ *pb.Player, g *model.GameState) error {
		for _, w := range g.Warriors {
			if w.GetGuid() == p.GetHeroWarId() {
				w.Cooking = &pb.Cooking{Prop: &pb.BaseProp{Att: proto.Float64(160000)}}
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	oneFullP, oneFullG := gemProfile(t, id)
	oneFullRaw, _ := json.Marshal(oneFullG)
	if _, err := s.fishUsing(ctx, id, req, 990003); err == nil {
		t.Fatal("golden meal accepted with one full attribute")
	}
	stillP, stillG := gemProfile(t, id)
	stillRaw, _ := json.Marshal(stillG)
	if !proto.Equal(oneFullP, stillP) || string(oneFullRaw) != string(stillRaw) {
		t.Fatal("single full attribute consumed meal")
	}
	// Restore the exact accepted fixture snapshot so receipt projection remains valid.
	if err := updateProfile(ctx, id, func(_ *pb.Player, g *model.GameState) error {
		for _, w := range g.Warriors {
			if w.GetGuid() == p.GetHeroWarId() {
				w.Cooking = proto.Clone(out[0].(*pb.SCFishUsing).GetWarrior().GetCooking()).(*pb.Cooking)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// A fresh service reuses persisted totals and frozen successful receipt.
	cold := &GameService{}
	if err := cold.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	again, err := cold.fishUsing(ctx, id, req, 990001)
	if err != nil || !proto.Equal(out[0], again[0]) {
		t.Fatal("golden cold receipt", err)
	}
}
