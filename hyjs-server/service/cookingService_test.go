package service

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func cookingFixture(t *testing.T) (*GameService, int64) {
	t.Helper()
	s, id, _ := gemFixture(t, false)
	if err := s.InitCooking(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(context.Background(), id, func(_ *pb.Player, g *model.GameState) error {
		for _, key := range []string{"FishItem_007", "FishItem_013", "FishItem_019"} {
			g.Cooking = append(g.Cooking, &pb.SimpleItem{Id: proto.String(key), Num: proto.Int32(2000)})
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return s, id
}

func TestCookingRecipeAndLateFailureRollback(t *testing.T) {
	s, id := cookingFixture(t)
	ctx := context.Background()
	r := &pb.CSFishCooking{Fish: &pb.BaseResource{Id: proto.String("FishItem_029"), Num: proto.Int32(24)}}
	response, err := s.fishCooking(ctx, id, r, 100)
	if err != nil {
		t.Fatal(err)
	}
	result := response[0].(*pb.SCFishCooking)
	if result.GetLevel() != 2 || result.GetExp() != 0 || result.GetAdd() != 600 || len(result.GetCost()) != 4 || result.GetRes().GetSimps()[0].GetNum() != 24 {
		t.Fatalf("recipe result %v", result)
	}
	var before db.Profile
	db.GetIns().First(&before, id)
	s.table("FishSkill", "FishItemID", "FishItem_029")["MakingExp"] = "-1"
	if _, err = s.fishCooking(ctx, id, r, 101); err == nil {
		t.Fatal("invalid late experience accepted")
	}
	var after db.Profile
	db.GetIns().First(&after, id)
	if *before.Game != *after.Game || *before.Player != *after.Player {
		t.Fatal("late failure left inventory changes")
	}
	var receipt db.CookingMutationReceipt
	db.GetIns().Where("player_id = ?", id).Take(&receipt)
	if receipt.Timestamp != 100 {
		t.Fatal("failed recipe replaced receipt")
	}
}

func TestCookingTimedRewardTransactionAndExpiry(t *testing.T) {
	s, id := cookingFixture(t)
	now := time.Now().UnixMilli()
	effect := &db.CookingEffect{PlayerID: id, ItemID: "FishItem_037", Kind: "extraCoinExp", EndAt: now + 30*60000, Value: 0.075}
	if err := db.GetIns().Save(effect).Error; err != nil {
		t.Fatal(err)
	}
	// 奖励和时效扣除属于同一调用方事务；失败结算不应消耗料理时间。
	rollback := errors.New("late battle failure")
	err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		coin, exp, err := s.ApplyCookingRewardBonus(tx, id, now, 2190, 140, 1)
		if err != nil || coin != 2354 || exp != 150 {
			t.Fatalf("bonus coin=%d exp=%d err=%v", coin, exp, err)
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	var saved db.CookingEffect
	db.GetIns().Where("player_id = ?", id).Take(&saved)
	if saved.EndAt != effect.EndAt {
		t.Fatal("rolled back battle consumed time")
	}
	if err = db.GetIns().Transaction(func(tx *gorm.DB) error {
		_, _, err := s.ApplyCookingRewardBonus(tx, id, now, 2190, 140, 1)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	db.GetIns().Where("player_id = ?", id).Take(&saved)
	if saved.EndAt != effect.EndAt-3*60000 {
		t.Fatal("original three-minute sweep cost missing")
	}
	coin, exp, err := s.ApplyCookingRewardBonus(db.GetIns(), id, saved.EndAt, 2190, 140, 0)
	if err != nil || coin != 2190 || exp != 140 {
		t.Fatal("expired bonus still applied")
	}
	if _, _, err = s.ApplyCookingRewardBonus(db.GetIns(), id, now, 2147483647, 2147483647, 1); err == nil {
		t.Fatal("bonus overflow accepted")
	}
}

func TestCookingPermanentRuleRollbackAndRestartPolicy(t *testing.T) {
	s, id := cookingFixture(t)
	ctx := context.Background()
	if _, err := s.fishCooking(ctx, id, &pb.CSFishCooking{Fish: &pb.BaseResource{Id: proto.String("FishItem_029"), Num: proto.Int32(1)}}, 100); err != nil {
		t.Fatal(err)
	}
	p, _ := gemProfile(t, id)
	r := &pb.CSFishUsing{CostItem: &pb.BaseResource{Id: proto.String("FishItem_029"), Num: proto.Int32(1)}, HeroGuid: proto.String(p.GetHeroWarId())}
	if err := db.GetIns().Model(&db.CookingRule{}).Where("id = ?", 1).Update("gains", `{"att":-1}`).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := s.fishUsing(ctx, id, r, 100); err == nil {
		t.Fatal("negative operations rule accepted")
	}
	if err := db.GetIns().Model(&db.CookingRule{}).Where("id = ?", 1).Update("gains", `{"att":3}`).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.InitCooking(ctx); err != nil {
		t.Fatal(err)
	}
	responses, err := s.fishUsing(ctx, id, r, 100)
	if err != nil {
		t.Fatal(err)
	}
	result := responses[0].(*pb.SCFishUsing)
	if result.GetWarrior().GetCooking().GetProp().GetAtt() != 3 {
		t.Fatalf("SQL operations rule overwritten %v", result)
	}
	replay, err := s.fishUsing(ctx, id, r, 100)
	if err != nil || !proto.Equal(result, replay[0]) {
		t.Fatal("permanent receipt changed")
	}
}

// TestCookingReceiptWriteFailureRollsBackAllEffects 在回执落库阶段制造数据库错误，验证先前副作用全部回滚。
func TestCookingReceiptWriteFailureRollsBackAllEffects(t *testing.T) {
	s, id := cookingFixture(t)
	ctx := context.Background()
	p, _ := gemProfile(t, id)
	if err := updateProfile(ctx, id, func(_ *pb.Player, g *model.GameState) error {
		g.Cooking = append(g.Cooking, &pb.SimpleItem{Id: proto.String("FishItem_037"), Num: proto.Int32(1)})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	prior := &db.CookingEffect{PlayerID: id, ItemID: "FishItem_035", Kind: "extraCoin", EndAt: now + 60000, Value: 0.05}
	if err := db.GetIns().Create(prior).Error; err != nil {
		t.Fatal(err)
	}
	var before db.Profile
	if err := db.GetIns().First(&before, id).Error; err != nil {
		t.Fatal(err)
	}
	// 隔离 SQLite 触发器在事务最后阶段失败，不修改业务实现或正式数据库。
	if err := db.GetIns().Exec("CREATE TRIGGER fail_cooking_receipt BEFORE INSERT ON cooking_mutation_receipts BEGIN SELECT RAISE(ABORT, 'isolated receipt failure'); END").Error; err != nil {
		t.Fatal(err)
	}
	r := &pb.CSFishUsing{CostItem: &pb.BaseResource{Id: proto.String("FishItem_037"), Num: proto.Int32(1)}, HeroGuid: proto.String(p.GetHeroWarId())}
	if _, err := s.fishUsing(ctx, id, r, 100); err == nil {
		t.Fatal("injected receipt write failure unexpectedly succeeded")
	}
	var (
		after  db.Profile
		effect db.CookingEffect
		states int64
	)
	if err := db.GetIns().First(&after, id).Error; err != nil {
		t.Fatal(err)
	}
	if *before.Player != *after.Player || *before.Game != *after.Game {
		t.Fatal("receipt write failure committed inventory or player state")
	}
	if err := db.GetIns().Where("player_id = ?", id).Take(&effect).Error; err != nil {
		t.Fatal(err)
	}
	if effect.ItemID != prior.ItemID || effect.Kind != prior.Kind || effect.EndAt != prior.EndAt || effect.Value != prior.Value {
		t.Fatal("receipt write failure committed the replacement timed effect")
	}
	if err := db.GetIns().Model(&db.CookingCraftState{}).Where("player_id = ?", id).Count(&states).Error; err != nil || states != 0 {
		t.Fatalf("receipt write failure created skill state: states=%d err=%v", states, err)
	}
}

// TestCookingPermanentPropertyBeforePercent 对齐 UIHero.ResetAttribute，永久料理应进入百分比基础层。
func TestCookingPermanentPropertyBeforePercent(t *testing.T) {
	s, id := cookingFixture(t)
	ctx := context.Background()
	if err := updateProfile(ctx, id, func(p *pb.Player, g *model.GameState) error {
		for _, w := range g.Warriors {
			if w.GetGuid() == p.GetHeroWarId() {
				w.Breached = &pb.Breached{Level: proto.Int32(2)}
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	p, g := gemProfile(t, id)
	before, err := s.battlePlayer(p, g)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.fishCooking(ctx, id, &pb.CSFishCooking{Fish: &pb.BaseResource{Id: proto.String("FishItem_029"), Num: proto.Int32(1)}}, 100); err != nil {
		t.Fatal(err)
	}
	if _, err = s.fishUsing(ctx, id, &pb.CSFishUsing{CostItem: &pb.BaseResource{Id: proto.String("FishItem_029"), Num: proto.Int32(1)}, HeroGuid: proto.String(p.GetHeroWarId())}, 200); err != nil {
		t.Fatal(err)
	}
	p, g = gemProfile(t, id)
	after, err := s.battlePlayer(p, g)
	if err != nil {
		t.Fatal(err)
	}
	if gain := after.GetFormation()[0].GetProp().GetAtt() - before.GetFormation()[0].GetProp().GetAtt(); math.Abs(gain-106) > 1e-8 {
		t.Fatalf("permanent cooking should be scaled by the original breach bonus: attack gain=%v expected=106", gain)
	}
}
