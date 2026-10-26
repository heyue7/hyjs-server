package service

import (
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// 真实三星胜利与原试炼星活动共同产生110点，原type5交易不依赖预置点数。
func TestP7EarnedTrialStarsActivityPointShopMaterialColdOnce(t *testing.T) {
	f := p7SectionFixture(t)
	if err := f.s.InitializeShop(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP4(f.ctx); err != nil {
		t.Fatal(err)
	}
	p7StarActivityOperations75(t, f)
	p7PointShopOperations75(t, f)
	initial, game := f.profile(t)
	if initial.GetPoint() != 0 || simpleOwned(initial, game, "item_021") != 0 {
		t.Fatal("source account must have no pre-funded point or essence")
	}
	p7SectionPlayFloor(t, f, 1, 3, 18701)
	earned, _ := f.profile(t)
	if earned.GetPoint() != 10 {
		t.Fatal("actual Trial floor did not award point10")
	}
	var activity db.PeriodInstance
	if err := db.GetIns().Where("kind=? AND enabled=?", constUtil.P7RushStarEvent, true).First(&activity).Error; err != nil {
		t.Fatal(err)
	}
	info, err := f.s.P1Activity(f.ctx, f.id, &pb.CSActivityInfo{ActivityId: proto.String(activity.ID)})
	if err != nil {
		t.Fatal(err)
	}
	view := info[0].(*pb.SCActivityInfo)
	if len(view.ItemList) != 1 || view.ItemList[0].GetStatus() != 1 {
		t.Fatal("real three-star event did not unlock activity", view)
	}
	claim := &pb.CSActivityReward{ActivityId: proto.String(activity.ID), Index: proto.Int32(1), GetResources: view.ItemList[0].RewardList}
	if _, err = f.s.P1Activity(f.ctx, f.id, claim); err != nil {
		t.Fatal(err)
	}
	funded, _ := f.profile(t)
	if funded.GetPoint() != 110 {
		t.Fatal("real activity source differs from local published100point reward")
	}
	request := &pb.CSShopBuy{ShopType: proto.Int32(5), ShopItemId: proto.String("p4Point_1"), BuyCount: proto.Int32(1)}
	replies, err := f.s.p4Commerce(f.ctx, f.id, request, 18702, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	paid := replies[0].(*pb.SCShopBuy)
	if len(paid.Cost) != 1 || paid.Cost[0].GetId() != "point" || paid.Cost[0].GetNum() != 100 {
		t.Fatal("original point shop cost", paid.Cost)
	}
	after, items := f.profile(t)
	if after.GetPoint() != 10 || simpleOwned(after, items, "item_021") != 1 || after.GetCoin() != initial.GetCoin() || after.GetDiamond() != initial.GetDiamond() {
		t.Fatal("earned point did not buy actual essence")
	}
	if _, err = f.s.p4Commerce(f.ctx, f.id, &pb.CSShopBuy{ShopType: proto.Int32(5), ShopItemId: proto.String("p4Point_2"), BuyCount: proto.Int32(1)}, 18703, time.Now()); err == nil {
		t.Fatal("one earned essence bypassed the red equipment ten-essence cost")
	}
	f.restart(t)
	retry, err := f.s.p4Commerce(f.ctx, f.id, request, 18702, time.Now())
	if err != nil || !proto.Equal(retry[0], paid) {
		t.Fatal("cold point purchase receipt", err)
	}
	if _, err = f.s.P1Activity(f.ctx, f.id, claim); err != nil {
		t.Fatal(err)
	}
	still, stillItems := f.profile(t)
	if still.GetPoint() != 10 || simpleOwned(still, stillItems, "item_021") != 1 {
		t.Fatal("cold activity or point shop duplicated earned currency")
	}
}
