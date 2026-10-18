package service

import (
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func p13GuildDonateBoxSetup(t *testing.T) (*p1Fixture, []int64, time.Time) {
	t.Helper()
	f, ids := p13GuildSetup(t)
	if err := f.s.InitializeP13GuildDonateBox(f.ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	// Only finite base account balances/level come from p13GuildSetup. Points and claims use real actions.
	p13GuildOpen(t, true)
	p13GuildCall(t, f, ids[0], &pb.CSUnionCreate{Name: proto.String("捐箱真链")}, 1, now)
	return f, ids, now
}

func p13GuildDonateBoxEnable(t *testing.T, now time.Time) {
	t.Helper()
	if err := db.GetIns().Model(&db.P13GuildDonateBoxPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "published_at": now.Add(-time.Second).UnixMilli()}).Error; err != nil {
		t.Fatal(err)
	}
}

func p13GuildDonateBoxEarnFirst(t *testing.T, f *p1Fixture, id int64, now time.Time) {
	t.Helper()
	for i := int64(0); i < 10; i++ {
		p13GuildCall(t, f, id, &pb.CSUnionDonate{DonateType: proto.String("coin")}, 100+i, now)
	}
	for i := int64(0); i < 30; i++ {
		p13GuildCall(t, f, id, &pb.CSUnionDonate{DonateType: proto.String("badge")}, 200+i, now)
	}
}

func TestP13GuildDonateBoxActualDonationClaimRetryColdAndSpend(t *testing.T) {
	f, ids, now := p13GuildDonateBoxSetup(t)
	id := ids[0]
	// Original historic guild totals cannot create local score after enabling.
	p13GuildCall(t, f, id, &pb.CSUnionDonate{DonateType: proto.String("coin")}, 2, now)
	p13GuildDonateBoxEnable(t, now)
	store := p13GuildCall(t, f, id, &pb.CSUnionStore{}, 0, now).(*pb.SCUnionStore)
	if len(store.GetBoxes()) != 4 || store.GetCurrentScore() != 0 || store.GetBoxes()[0].GetState() != 0 {
		t.Fatal("historic donation must not count towards new policy", store)
	}
	p13GuildDonateBoxEarnFirst(t, f, id, now)
	store = p13GuildCall(t, f, id, &pb.CSUnionStore{}, 0, now).(*pb.SCUnionStore)
	if store.GetCurrentScore() != 250 || store.GetBoxes()[0].GetState() != 1 || store.GetBoxes()[1].GetState() != 0 || store.GetBoxes()[0].GetReward() != "coin,1000" {
		t.Fatal("real source did not reach first original UI node", store)
	}
	before, _ := p10LuckyProfile(t, id)
	request := &pb.CSUnionOpenDonateBox{BoxId: proto.String(store.GetBoxes()[0].GetBoxId())}
	// An earned day is honored after closing new accrual.
	if err := db.GetIns().Model(&db.P13GuildDonateBoxPolicy{}).Where("id=1").Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	first, err := f.s.p13GuildOpenDonateBox(f.ctx, id, request, 300, now)
	if err != nil || first[0].(*pb.SCUnionOpenDonateBox).GetGet() == nil {
		t.Fatal("earned box was not honored", err)
	}
	after, _ := p10LuckyProfile(t, id)
	if after.GetCoin()-before.GetCoin() != 1000 {
		t.Fatal("claim must credit only the published coin amount")
	}
	f.restart(t)
	retry, err := f.s.p13GuildOpenDonateBox(f.ctx, id, request, 300, now)
	if err != nil || !proto.Equal(first[0], retry[0]) {
		t.Fatal("same original request did not restore its response", err)
	}
	if _, err = f.s.p13GuildOpenDonateBox(f.ctx, id, request, 301, now); err == nil {
		t.Fatal("new stamp duplicated a claimed tier")
	}
	p13GuildCall(t, f, id, &pb.CSUnionDonate{DonateType: proto.String("coin")}, 302, now)
	spent, _ := p10LuckyProfile(t, id)
	if spent.GetCoin() != after.GetCoin()-100000 {
		t.Fatal("reward coin did not enter the original donation consumer")
	}
	store = p13GuildCall(t, f, id, &pb.CSUnionStore{}, 0, now).(*pb.SCUnionStore)
	if store.GetCurrentScore() != 250 || store.GetBoxes()[0].GetState() != 2 {
		t.Fatal("closed policy accrued score or lost claim state")
	}
	if _, err = f.s.p13GuildOpenDonateBox(f.ctx, id, request, 303, now.Add(24*time.Hour)); err == nil {
		t.Fatal("old daily box was claimable in a later day")
	}
}

func TestP13GuildDonateBoxMembershipConcurrentClaimAndSourceIntegrity(t *testing.T) {
	f, ids, now := p13GuildDonateBoxSetup(t)
	id := ids[0]
	p13GuildDonateBoxEnable(t, now)
	p13GuildDonateBoxEarnFirst(t, f, id, now)
	guild := p13GuildID(t, f, id, now)
	p13Join(t, f, id, ids[1], guild, 50, now)
	store := p13GuildCall(t, f, ids[1], &pb.CSUnionStore{}, 0, now).(*pb.SCUnionStore)
	request := &pb.CSUnionOpenDonateBox{BoxId: proto.String(store.GetBoxes()[0].GetBoxId())}
	if store.GetBoxes()[0].GetState() != 0 {
		t.Fatal("late member inherited personal donation eligibility")
	}
	if _, err := f.s.p13GuildOpenDonateBox(f.ctx, ids[1], request, 300, now); err == nil {
		t.Fatal("member without a current-term donation claimed")
	}
	p13GuildCall(t, f, ids[1], &pb.CSUnionDonate{DonateType: proto.String("coin")}, 100, now)
	before, _ := p10LuckyProfile(t, ids[1])
	var wg sync.WaitGroup
	errs := make(chan error, 6)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := f.s.p13GuildOpenDonateBox(f.ctx, ids[1], request, 301, now)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	after, _ := p10LuckyProfile(t, ids[1])
	if after.GetCoin()-before.GetCoin() != 1000 {
		t.Fatal("concurrent original retries multiplied reward")
	}
	// Corrupting an aggregate cannot manufacture a later tier without donation sources.
	if err := db.GetIns().Model(&db.P13GuildDonateBoxDay{}).Where("guild_id=? AND day=?", guild, p13GuildDay(now)).Update("score", 600).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p13GuildQuery(f.ctx, id, &pb.CSUnionStore{}, now); err == nil {
		t.Fatal("aggregate without matching actual donation sources was trusted")
	}
}
