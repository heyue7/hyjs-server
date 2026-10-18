package service

import (
	"context"
	"encoding/json"
	"math"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// TestCookingGemHalfCatalogue 对用户决定的单、双、全属性档位使用明确期望值。
func TestCookingGemHalfCatalogue(t *testing.T) {
	s := &GameService{}
	if err := s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	// 本例只核单份曲线；默认策略另行要求累计食用上限。
	rule := &db.CookingRule{Gains: `{"gemScale":0.5}`}
	cases := []struct {
		id   string
		want map[string]float64
	}{
		{"FishItem_027", map[string]float64{"pdef": 30}},
		{"FishItem_029", map[string]float64{"att": 100}},
		{"FishItem_033", map[string]float64{"hp": 760}},
		{"FishItem_040", map[string]float64{"cri": 1.2, "antiCri": 1.2}},
		{"FishItem_041", map[string]float64{"att": 500, "hp": 1900}},
		{"FishItem_049", map[string]float64{"att": 750, "hp": 2850}},
		{"FishItem_050", map[string]float64{"att": 2000, "hp": 7600, "pdef": 600, "mdef": 600, "cri": 3, "antiCri": 3}},
	}
	for _, tc := range cases {
		gains, _, err := s.cookingPermanentPolicy(rule, s.table("FishSkill", "FishItemID", tc.id))
		if err != nil || len(gains) != len(tc.want) {
			t.Fatalf("%s: gains=%v err=%v", tc.id, gains, err)
		}
		for key, want := range tc.want {
			if math.Abs(gains[key]-want) > 1e-9 {
				t.Fatalf("%s %s: got %v want %v", tc.id, key, gains[key], want)
			}
		}
	}
}

// TestCookingMixedAttributeCapRollback 验证任一属性已满就整份拒绝，保留库存与回执。
func TestCookingMixedAttributeCapRollback(t *testing.T) {
	cookingMixedAttributeCapRollback(t, false)
}

func TestCookingMixedAttributeCapMySQL(t *testing.T) {
	cookingMixedAttributeCapRollback(t, true)
}

func cookingMixedAttributeCapRollback(t *testing.T, mysql bool) {
	s, id, _ := gemFixture(t, mysql)
	ctx := context.Background()
	if err := s.InitCooking(ctx); err != nil {
		t.Fatal(err)
	}
	policy, err := json.Marshal(map[string]float64{"gemScale": 0.5, "limit4.att": 1000, "limit4.hp": 3800})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Model(&db.CookingRule{}).Where("id = ?", 1).Update("gains", string(policy)).Error; err != nil {
		t.Fatal(err)
	}
	p, _ := gemProfile(t, id)
	if err = updateProfile(ctx, id, func(_ *pb.Player, g *model.GameState) error {
		for _, w := range g.Warriors {
			if w.GetGuid() == p.GetHeroWarId() {
				w.Cooking = &pb.Cooking{Prop: &pb.BaseProp{Att: proto.Float64(900), Hp: proto.Float64(1900)}}
			}
		}
		g.Cooking = append(g.Cooking, &pb.SimpleItem{Id: proto.String("FishItem_041"), Num: proto.Int32(2)})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	r := &pb.CSFishUsing{CostItem: &pb.BaseResource{Id: proto.String("FishItem_041"), Num: proto.Int32(1)}, HeroGuid: proto.String(p.GetHeroWarId())}
	responses, err := s.fishUsing(ctx, id, r, 100)
	if err != nil {
		t.Fatal(err)
	}
	prop := responses[0].(*pb.SCFishUsing).GetWarrior().GetCooking().GetProp()
	if prop.GetAtt() != 1000 || prop.GetHp() != 3800 {
		t.Fatalf("cap crossing %v", prop)
	}
	var before, after db.Profile
	if err = db.GetIns().First(&before, id).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = s.fishUsing(ctx, id, r, 101); err == nil {
		t.Fatal("full paired food accepted")
	}
	if err = db.GetIns().First(&after, id).Error; err != nil {
		t.Fatal(err)
	}
	if *before.Game != *after.Game || *before.Player != *after.Player {
		t.Fatal("rejected food consumed inventory or changed stats")
	}
	var receipt db.CookingMutationReceipt
	if err = db.GetIns().Where("player_id = ? AND opcode = ?", id, constUtil.CookingUseOpcode).Take(&receipt).Error; err != nil || receipt.Timestamp != 100 {
		t.Fatal("rejected food replaced receipt", err)
	}
	replay, err := s.fishUsing(ctx, id, r, 100)
	if err != nil || !proto.Equal(responses[0], replay[0]) {
		t.Fatal("food retry changed frozen result", err)
	}
}

func TestGemRefineUserRuleMySQL(t *testing.T) {
	s, id, _ := gemFixture(t, true)
	ctx := context.Background()
	r := &pb.CSMineRefine{MineId: proto.String("MineItem_007"), Num: proto.Int32(50)}
	out, err := s.gemRefine(ctx, id, r, 100)
	if err != nil {
		t.Fatal(err)
	}
	result := out[0].(*pb.SCMineRefine)
	var materials int32
	for _, item := range result.GetRes().GetSimps() {
		for _, key := range []string{"MineItem_013", "MineItem_014", "MineItem_015", "MineItem_016", "MineItem_017", "MineItem_018"} {
			if item.GetId() == key {
				materials += item.GetNum()
			}
		}
	}
	if materials != 10 {
		t.Fatalf("50 ores must yield 10 total ordinary materials, got %d", materials)
	}
	retry, err := s.gemRefine(ctx, id, r, 100)
	if err != nil || !proto.Equal(result, retry[0]) {
		t.Fatal("refining rerolled or recharged retry", err)
	}
	_, g := gemProfile(t, id)
	if gemCount(g, "MineItem_007") != 1950 {
		t.Fatal("ore double charge")
	}
}
