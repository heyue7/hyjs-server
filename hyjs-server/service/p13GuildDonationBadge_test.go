package service

import (
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// This path starts with no badge fixture. Paid original donation produces P1
// mail, its actual original claim produces inventory, and inventory is donated.
func TestP13GuildDonationBadgePaidSourceMailClaimThenOriginalDonateBox(t *testing.T) {
	f, ids, now := p13GuildDonateBoxSetup(t)
	id := ids[0]
	if err := db.GetIns().Model(&db.P13GuildDonateBoxPolicy{}).Where("id=1").Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P13GuildDonationBadgePolicy{}).Where("id=1").Updates(map[string]any{"enabled": false, "published_at": int64(0)}).Error; err != nil {
		t.Fatal(err)
	}
	// The inherited finite account fixture has coins/level only; remove its badges before any source action.
	if err := updateProfile(f.ctx, id, func(player *pb.Player, game *model.GameState) error {
		var items []*pb.SimpleItem
		for _, item := range game.Items {
			if item.GetId() != "item_089" {
				items = append(items, item)
			}
		}
		game.Items = items
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// A historical successful stamp replay must not earn a new policy's reward.
	guild := p13GuildID(t, f, id, now)
	p13Join(t, f, id, ids[1], guild, 50, now)
	p13GuildCall(t, f, ids[1], &pb.CSUnionDonate{DonateType: proto.String("coin")}, 2, now)
	if err := db.GetIns().Model(&db.P13GuildDonationBadgePolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "published_at": now.Add(-time.Second).UnixMilli()}).Error; err != nil {
		t.Fatal(err)
	}
	p13GuildDonateBoxEnable(t, now)
	p13GuildCall(t, f, ids[1], &pb.CSUnionDonate{DonateType: proto.String("coin")}, 2, now)
	var historical int64
	if err := db.GetIns().Model(&db.P13GuildDonationBadgeSource{}).Where("player_id=?", ids[1]).Count(&historical).Error; err != nil || historical != 0 {
		t.Fatal("historic request was retroactively rewarded", err)
	}
	before, _ := p10LuckyProfile(t, id)
	for i := int64(0); i < 15; i++ {
		p13GuildCall(t, f, id, &pb.CSUnionDonate{DonateType: proto.String("coin")}, 100+i, now)
	}
	if _, err := f.s.p13GuildEconomy(f.ctx, id, &pb.CSUnionDonate{DonateType: proto.String("coin")}, 115, now); err == nil {
		t.Fatal("original 1500000 daily coin limit was bypassed")
	}
	afterPay, game := p10LuckyProfile(t, id)
	if before.GetCoin()-afterPay.GetCoin() != 1500000 {
		t.Fatal("badge source did not pay all original coin costs")
	}
	for _, item := range game.Items {
		if item.GetId() == "item_089" && item.GetNum() != 0 {
			t.Fatal("badge inventory existed before original P1 claim")
		}
	}
	var sources []db.P13GuildDonationBadgeSource
	if err := db.GetIns().Where("player_id=?", id).Order("ordinal").Find(&sources).Error; err != nil || len(sources) != 15 {
		t.Fatal("real paid sources missing", err)
	}
	var rewardIDs []int64
	for _, source := range sources {
		if source.SourceHash != p13GuildDonationBadgeSourceHash(source) || source.CoinCost != 100000 || source.BadgeAmount != 2 {
			t.Fatal("frozen debit/source mismatched")
		}
		rewardIDs = append(rewardIDs, source.DeliveryID)
	}
	// Stop only new supply. Old earned mail remains claimable after cold recovery.
	if err := db.GetIns().Model(&db.P13GuildDonationBadgePolicy{}).Where("id=1").Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	claimed, err := f.s.RewardCenter(f.ctx, id, &pb.CSAddRewardsToPlayer{RewardIds: rewardIDs})
	if err != nil {
		t.Fatal(err)
	}
	var granted int32
	for _, item := range claimed[0].(*pb.SCAddRewardsToPlayer).GetResult().GetSimps() {
		if item.GetId() == "item_089" {
			granted += item.GetNum()
		}
	}
	if granted != 30 {
		t.Fatal("original P1 claim did not grant exactly paid-for thirty badges")
	}
	again, err := f.s.RewardCenter(f.ctx, id, &pb.CSAddRewardsToPlayer{RewardIds: rewardIDs})
	if err != nil || len(again[0].(*pb.SCAddRewardsToPlayer).GetResult().GetSimps()) != 0 {
		t.Fatal("claimed mail duplicated inventory", err)
	}
	for i := int64(0); i < 30; i++ {
		p13GuildCall(t, f, id, &pb.CSUnionDonate{DonateType: proto.String("badge")}, 200+i, now)
	}
	store := p13GuildCall(t, f, id, &pb.CSUnionStore{}, 0, now).(*pb.SCUnionStore)
	if store.GetCurrentScore() != 255 || store.GetBoxes()[0].GetState() != 1 {
		t.Fatal("natural paid badges did not reach original first box threshold", store)
	}
	boxRequest := &pb.CSUnionOpenDonateBox{BoxId: proto.String(store.GetBoxes()[0].GetBoxId())}
	if _, err = f.s.p13GuildOpenDonateBox(f.ctx, id, boxRequest, 400, now); err != nil {
		t.Fatal(err)
	}
	final, game := p10LuckyProfile(t, id)
	if final.GetCoin() != afterPay.GetCoin()+1000 {
		t.Fatal("box award did not enter actual wallet")
	}
	for _, item := range game.Items {
		if item.GetId() == "item_089" && item.GetNum() != 0 {
			t.Fatal("badge donation created badges without a new coin cost")
		}
	}
	var count int64
	if err = db.GetIns().Model(&db.P13GuildDonationBadgeSource{}).Where("player_id=?", id).Count(&count).Error; err != nil || count != 15 {
		t.Fatal("badge consumption generated another paid source", err)
	}
}
