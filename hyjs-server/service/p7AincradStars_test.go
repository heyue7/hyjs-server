package service

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Reuse the independently certified real solo trial helper; neither old star
// totals, EventFact rows, winners, rewards nor P7 receipts are inserted.
func TestP7AincradPersonalNewEarnedStarsOriginalGenericRewardResetAndCold(t *testing.T) {
	f := p7SectionFixture(t)
	for _, init := range []func(context.Context) error{f.s.InitializeShop, f.s.InitializeOpenServerActivity, f.s.InitializeP4, f.s.InitializeLocalPlay, f.s.InitializeLocalPlayActivities} {
		if err := init(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	// Create an actual eligible demo account. Only level22 is finite preparation.
	account, err := LoginAccount(f.ctx, fmt.Sprintf("demo-aincrad-%d", time.Now().UnixNano()), "isolated-aincrad-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(f.ctx, account.Id, "试炼新期星数", "hero_047"); err != nil {
		t.Fatal(err)
	}
	f.id = account.Id
	if err = updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error { p.Level = proto.Int32(22); return nil }); err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Model(&db.LocalPlayPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	f.s.LocalPlayToolsEnabled = true
	var owner db.Profile
	if err = db.GetIns().First(&owner, f.id).Error; err != nil {
		t.Fatal(err)
	}
	f.ctx = context.WithValue(f.ctx, constUtil.LocalWalletAPIContextKey, true)
	f.ctx = context.WithValue(f.ctx, constUtil.LocalWalletSessionContextKey, append([]byte(nil), owner.Session...))
	f.ctx, err = f.s.BindLocalPlayContext(f.ctx, &owner)
	if err != nil {
		t.Fatal(err)
	}
	p7SectionPublish(t)
	p, _ := f.profile(t)
	if p.GetPoint() != 0 || p.GetCoin() != 0 || p.GetDiamond() != 0 {
		t.Fatal("no resource fixture on source account")
	}
	closed, err := f.s.LocalPlayActivitiesApply(f.ctx, f.id, "open", "AincradStars", "91500000-0000-4000-8000-000000000001")
	if err != nil || len(closed.Results) != 1 {
		t.Fatal("original source availability", closed, err)
	}
	// Managed suites can share a template already enabled by another test's
	// real Trial. Do not erase that publication just to manufacture initial-off.
	if closed.Results[0].Status == "closed" {
		if p1Count(t, &db.LocalPlayActivityBinding{}, "player_id=? AND activity_id=?", f.id, "AincradStars") != 0 {
			t.Fatal("closed source created a private period")
		}
	} else if closed.Results[0].Status != "open" {
		t.Fatal("unknown original source status", closed)
	}
	// An actual accepted old three-star floor enables the global source only.
	p7SectionPlayFloor(t, f, 1, 3, 94201)
	var global db.PeriodInstance
	if err = db.GetIns().Where("kind=? AND enabled=?", constUtil.P7RushStarEvent, true).First(&global).Error; err != nil {
		t.Fatal(err)
	}
	if global.Mode != "single" || global.Goal != 3 || global.Rewards != "point,100" {
		t.Fatal("global local first tier changed")
	}
	apply := func(action, key string) string {
		t.Helper()
		view, e := f.s.LocalPlayActivitiesApply(f.ctx, f.id, action, "AincradStars", key)
		if e != nil || len(view.Results) != 1 || view.Results[0].Status != "open" {
			t.Fatal("personal original source open", e, view)
		}
		cycle := localPlayCycleID(t, f, f.id, "AincradStars")
		var row db.PeriodInstance
		if e = db.GetIns().Where("id=?", "aincradStars_p4_"+cycle).First(&row).Error; e != nil {
			t.Fatal(e)
		}
		if row.Kind != constUtil.P7RushEarnedStarEvent || row.Mode != "cumulative" {
			t.Fatal("personal source reused old cumulative peak")
		}
		return row.ID
	}
	activity := apply("refresh", "91500000-0000-4000-8000-000000000002")
	info := func(key string, left, status int32) *pb.SCActivityInfo {
		t.Helper()
		rows, e := f.s.P1Activity(f.ctx, f.id, &pb.CSActivityInfo{ActivityId: proto.String(key)})
		if e != nil {
			t.Fatal(e)
		}
		view := rows[0].(*pb.SCActivityInfo)
		if len(view.ItemList) != 1 || view.ItemList[0].GetLeft() != left || view.ItemList[0].GetStatus() != status {
			t.Fatal("original generic activity state", view)
		}
		return view
	}
	info(activity, 0, 0)
	end2 := p7SectionPlayFloor(t, f, 2, 1, 94202)
	info(activity, 1, 0)
	// Query/replayed real End cannot manufacture another new star.
	if _, err = f.s.p7End(f.ctx, f.id, end2); err != nil {
		t.Fatal(err)
	}
	info(activity, 1, 0)
	p7SectionPlayFloor(t, f, 3, 1, 94203)
	info(activity, 2, 0)
	if _, err = f.s.P7RushMutation(f.ctx, f.id, &pb.CSRushResetChallengeCount{}, 94204); err != nil {
		t.Fatal("real completed-section reset", err)
	}
	info(activity, 2, 0)
	p7SectionPlayFloor(t, f, 1, 1, 94205)
	ready := info(activity, 3, 1)
	var cycleBefore db.LocalPlayActivityCycle
	if err = db.GetIns().First(&cycleBefore, "id=?", localPlayPrivateCycleID(activity)).Error; err != nil {
		t.Fatal(err)
	}
	newest := apply("refresh", "91500000-0000-4000-8000-000000000003")
	if newest == activity {
		t.Fatal("refresh reused old personal period")
	}
	info(newest, 0, 0)
	info(activity, 3, 1)
	// A true pending old award remains claimable by its original ID after refresh.
	claim := &pb.CSActivityReward{ActivityId: proto.String(activity), Index: proto.Int32(1), GetResources: ready.ItemList[0].RewardList}
	if _, err = f.s.P1Activity(f.ctx, f.id, claim); err != nil {
		t.Fatal("owned retired original award", err)
	}
	info(activity, 3, 2)
	if _, err = f.s.P1Activity(f.ctx, f.id, claim); err != nil {
		t.Fatal(err)
	}
	p, _ = f.profile(t)
	if p.GetPoint() != 140 {
		t.Fatal("four real floors plus one100point award", p.GetPoint())
	}
	f.restart(t)
	info(newest, 0, 0)
	info(activity, 3, 2)
	p7SectionPlayFloor(t, f, 2, 3, 94206)
	finalReady := info(newest, 3, 1)
	info(activity, 3, 2)
	nextClaim := &pb.CSActivityReward{ActivityId: proto.String(newest), Index: proto.Int32(1), GetResources: finalReady.ItemList[0].RewardList}
	if _, err = f.s.P1Activity(f.ctx, f.id, nextClaim); err != nil {
		t.Fatal(err)
	}
	request := &pb.CSShopBuy{ShopType: proto.Int32(5), ShopItemId: proto.String("p4Point_1"), BuyCount: proto.Int32(1)}
	paid, err := f.s.p4Commerce(f.ctx, f.id, request, 94207, LocalPlayNow(f.ctx))
	if err != nil {
		t.Fatal(err)
	}
	p, g := f.profile(t)
	if p.GetPoint() != 150 || simpleOwned(p, g, "item_021") != 1 || p.GetCoin() != 0 || p.GetDiamond() != 0 {
		t.Fatal("real earned activity point downstream consumer")
	}
	var old db.LocalPlayActivityCycle
	if err = db.GetIns().First(&old, "id=?", cycleBefore.ID).Error; err != nil {
		t.Fatal(err)
	}
	beforeRaw, _ := json.Marshal(cycleBefore)
	oldRaw, _ := json.Marshal(old)
	if string(beforeRaw) != string(oldRaw) {
		t.Fatal("old cycle snapshot/watermark modified")
	}
	if p1Count(t, &db.EventFact{}, "player_id=? AND kind=?", f.id, constUtil.P7RushStarEvent) != 5 || p1Count(t, &db.EventFact{}, "player_id=? AND kind=?", f.id, constUtil.P7RushEarnedStarEvent) != 5 {
		t.Fatal("queries/retries/reset duplicated trial facts")
	}
	f.restart(t)
	again, e := f.s.p4Commerce(f.ctx, f.id, request, 94207, LocalPlayNow(f.ctx))
	if e != nil || !proto.Equal(again[0], paid[0]) {
		t.Fatal("cold downstream exact receipt", e)
	}
	if _, e = f.s.P1Activity(f.ctx, f.id, nextClaim); e != nil {
		t.Fatal(e)
	}
	p, g = f.profile(t)
	if p.GetPoint() != 150 || simpleOwned(p, g, "item_021") != 1 {
		t.Fatal("cold duplicated activity points/material")
	}
	t.Log("actual certified Trial 3old then1+1+reset+1 -> newperiod3; oldaward afterrefresh; fresh3 ->point100 ->originaltype5essence; two cold; no inserted event/winner/currency")
}
