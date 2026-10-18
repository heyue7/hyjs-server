package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func navigationHas(out *pb.SCNavigationItem, kind int32) bool {
	for _, item := range out.NavItems {
		if item.GetType() == kind {
			return true
		}
	}
	return false
}
func navigationRead(t *testing.T, s *GameService, id int64) *pb.SCNavigationItem {
	t.Helper()
	out, err := s.navigationItems(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 {
		t.Fatal("original response count", out)
	}
	return out[0].(*pb.SCNavigationItem)
}

func TestNavigationOriginalServerLocalOwnership(t *testing.T) {
	flags := map[int32]bool{}
	for i := int32(1); i <= 22; i++ {
		flags[i] = true
	}
	out := navigationProjection(flags)
	if len(out.NavItems) != 17 {
		t.Fatal("original server type denominator", len(out.NavItems))
	}
	for _, item := range out.NavItems {
		if item.GetValue() != 1 {
			t.Fatal("original client ignores Value=0 and would display a false hint")
		}
		switch item.GetType() {
		case 11, 12, 19, 20, 21:
			t.Fatal("client-owned hint duplicated", item)
		}
	}
	if len(navigationProjection(map[int32]bool{1: false, 14: false}).NavItems) != 0 {
		t.Fatal("inactive hints must be omitted")
	}
}

func TestNavigationRecipeInventoryDoesNotSpend(t *testing.T) {
	p := &pb.Player{Coin: proto.Int64(10000)}
	stacks := []*pb.SimpleItem{{Id: proto.String("FishItem_007"), Num: proto.Int32(5)}, {Id: proto.String("FishItem_013"), Num: proto.Int32(5)}}
	before, _ := json.Marshal(stacks)
	if !navigationCanPay(p, stacks, "FishItem_007,5;FishItem_013,5;coin,10000") {
		t.Fatal("original027 recipe should be actionable")
	}
	for _, spec := range []string{"", "coin,10001", "FishItem_007,6", "FishItem_007,5;FishItem_007,5", "FishItem_007,-1", "FishItem_007,wrong"} {
		if navigationCanPay(p, stacks, spec) {
			t.Fatal("invalid/unaffordable recipe", spec)
		}
	}
	if navigationCanPay(p, append(stacks, stacks[0]), "FishItem_007,5") {
		t.Fatal("duplicate stack does not authorize a real craft")
	}
	after, _ := json.Marshal(stacks)
	if string(before) != string(after) || p.GetCoin() != 10000 {
		t.Fatal("navigation spent resources")
	}
}

// Finite original level/ore/food inputs isolate hint changes. Real consumers
// execute unchanged recipes and costs; no combat result or reward is injected.
func TestNavigationLifeRealCraftAndRefineRefresh(t *testing.T) {
	s, id, _ := gemFixture(t, false)
	if err := s.InitCooking(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().AutoMigrate(&db.P8LifeSkill{}); err != nil {
		t.Fatal(err)
	}
	s.P8LifeReady = true
	if err := updateProfile(context.Background(), id, func(p *pb.Player, g *model.GameState) error {
		p.Coin = proto.Int64(10000)
		g.Gems = []*pb.SimpleItem{{Id: proto.String("MineItem_007"), Num: proto.Int32(5)}}
		g.Cooking = []*pb.SimpleItem{{Id: proto.String("FishItem_007"), Num: proto.Int32(5)}, {Id: proto.String("FishItem_013"), Num: proto.Int32(5)}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	beforeP, beforeG := gemProfile(t, id)
	rawBefore, _ := json.Marshal(beforeG)
	first := navigationRead(t, s, id)
	if !navigationHas(first, 14) || !navigationHas(first, 15) {
		t.Fatal("available original life actions missing", first)
	}
	afterP, afterG := gemProfile(t, id)
	rawAfter, _ := json.Marshal(afterG)
	if !proto.Equal(beforeP, afterP) || string(rawBefore) != string(rawAfter) {
		t.Fatal("navigation changed profile")
	}
	if _, err := s.fishCooking(context.Background(), id, &pb.CSFishCooking{Fish: &pb.BaseResource{Id: proto.String("FishItem_027"), Num: proto.Int32(1)}}, 101); err != nil {
		t.Fatal(err)
	}
	if navigationHas(navigationRead(t, s, id), 15) {
		t.Fatal("consumed recipe ingredients still advertised")
	}
	if _, err := s.gemRefine(context.Background(), id, &pb.CSMineRefine{MineId: proto.String("MineItem_007"), Num: proto.Int32(5)}, 102); err != nil {
		t.Fatal(err)
	}
	if navigationHas(navigationRead(t, s, id), 14) {
		t.Fatal("consumed five ore still advertised")
	}
	// The persisted craft output cannot restore consumed ingredients on a repeat read.
	again := navigationRead(t, s, id)
	if navigationHas(again, 14) || navigationHas(again, 15) {
		t.Fatal("repeat read resurrected resource eligibility")
	}
}

func TestNavigationDailyOriginalOpenRemainingAndUninitializedFastPath(t *testing.T) {
	s, id, _ := gemFixture(t, false)
	// P7/P4/life are deliberately uninitialized; querying must not read absent tables.
	if got := navigationRead(t, s, id); len(got.NavItems) != 0 {
		t.Fatal("uninitialized domains advertised", got)
	}
	if err := s.InitializeDailyCopy(context.Background()); err != nil {
		t.Fatal(err)
	}
	p, _ := gemProfile(t, id)
	info, err := s.dailyCopyInfo(db.GetIns().WithContext(context.Background()), p, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	expected := false
	for _, kind := range info.Items {
		if kind.GetIsOpen() > 0 && kind.GetLastTime() > 0 {
			for _, stage := range kind.DailyCopyState {
				expected = expected || stage.GetIsOpen() > 0
			}
		}
	}
	if navigationHas(navigationRead(t, s, id), 13) != expected {
		t.Fatal("navigation disagrees with actual original daily page")
	}
	var states []db.DailyCopyState
	for _, r := range s.Tables["DailyCopy"] {
		_, _, limit, e := dailyCopyTypeRule(r, time.Now())
		if e != nil {
			t.Fatal(e)
		}
		states = append(states, db.DailyCopyState{PlayerID: id, Day: dailyCopyDay(time.Now()), Type: r["Type"], Used: limit, Limit: limit})
	}
	if err := db.GetIns().Create(&states).Error; err != nil {
		t.Fatal(err)
	}
	if navigationHas(navigationRead(t, s, id), 13) {
		t.Fatal("exhausted original day still advertised")
	}
}

func TestNavigationPolicyAndAwakeOriginalDayBudget(t *testing.T) {
	f := p7AwakeExploreFixture(t)
	p, _ := f.profile(t)
	at := time.Date(2026, 10, 16, 12, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
	flags := map[int32]bool{}
	if err := f.s.navigationAwake(db.GetIns().WithContext(f.ctx), p, at, flags); err != nil || !flags[22] {
		t.Fatal("original Friday501 available", flags, err)
	}
	if err := db.GetIns().Model(&db.P7AwakeState{}).Where("player_id=?", f.id).Update("used", 2).Error; err != nil {
		t.Fatal(err)
	}
	flags = map[int32]bool{}
	if err := f.s.navigationAwake(db.GetIns().WithContext(f.ctx), p, at, flags); err != nil || flags[22] {
		t.Fatal("used daily attempts remain advertised", flags, err)
	}
	flags = map[int32]bool{}
	if err := f.s.navigationAwake(db.GetIns().WithContext(f.ctx), p, at.AddDate(0, 0, 1), flags); err != nil || !flags[22] {
		t.Fatal("next open day must refresh original budget", flags, err)
	}
	if err := db.GetIns().Model(&db.P7AwakePolicy{}).Where("id=?", constUtil.P7AwakeExplorePolicyID).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	flags = map[int32]bool{}
	if err := f.s.navigationAwake(db.GetIns().WithContext(f.ctx), p, at.AddDate(0, 0, 1), flags); err != nil || flags[22] {
		t.Fatal("operator closed policy advertised", flags, err)
	}
}
