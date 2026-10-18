package service

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p10EnergySetup(t *testing.T) (*p1Fixture, int64, int64) {
	t.Helper()
	f, b, c := p10Setup(t)
	if err := f.s.InitializeP10FriendEnergy(f.ctx); err != nil {
		t.Fatal(err)
	}
	for _, table := range []any{&db.P10FriendEnergyPolicy{}, &db.P10FriendEnergyGift{}, &db.P10FriendEnergyDaily{}} {
		if err := db.GetIns().Where("1=1").Delete(table).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := f.s.InitializeP10FriendEnergy(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P10FriendEnergyPolicy{}).Where("id=?", 1).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	p10Call(t, f, f.id, &pb.CSApplyFriend{FriendId: proto.Int64(b)}, 1, now)
	p10Call(t, f, b, &pb.CSAgreeApply{FriendId: proto.Int64(f.id)}, 1, now)
	if err := updateProfile(f.ctx, b, func(p *pb.Player, g *model.GameState) error { p.Energy = proto.Int32(28); return nil }); err != nil {
		t.Fatal(err)
	}
	return f, b, c
}
func p10EnergyCall(t *testing.T, f *p1Fixture, id int64, request proto.Message, stamp int64, now time.Time) proto.Message {
	t.Helper()
	r, err := f.s.p10FriendEnergy(f.ctx, id, request, stamp, now)
	if err != nil {
		t.Fatal(err)
	}
	return r[0]
}
func TestP10FriendEnergyFrozenAmountConcurrentSingleClaimAndClosedFulfillment(t *testing.T) {
	f, b, c := p10EnergySetup(t)
	now := time.Now()
	gift := &pb.CSGiveEnergy{FriendId: proto.Int64(b)}
	p10EnergyCall(t, f, f.id, gift, 1, now)
	p10EnergyCall(t, f, f.id, gift, 2, now)
	if p1Count(t, &db.P10FriendEnergyGift{}, "sender_id=?", f.id) != 1 || p1Count(t, &db.EventFact{}, "player_id=? AND kind=?", f.id, "friend_energy_sent") != 1 {
		t.Fatal("duplicate give")
	}
	if _, err := f.s.p10FriendEnergy(f.ctx, c, &pb.CSGetEnergy{FriendId: proto.Int64(f.id)}, 1, now); err == nil {
		t.Fatal("foreign gift claim")
	}
	if err := db.GetIns().Model(&db.P10FriendEnergyPolicy{}).Where("id=?", 1).Updates(map[string]any{"amount": 5, "enabled": false}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p10FriendEnergy(f.ctx, f.id, &pb.CSGiveEnergy{FriendId: proto.Int64(b)}, 3, now); err == nil {
		t.Fatal("closed new gift")
	}
	var wg sync.WaitGroup
	errs := make(chan error, 6)
	results := make(chan proto.Message, 6)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := f.s.p10FriendEnergy(f.ctx, b, &pb.CSGetEnergy{FriendId: proto.Int64(f.id)}, 1, now)
			errs <- err
			if err == nil {
				results <- r[0]
			}
		}()
	}
	wg.Wait()
	close(errs)
	close(results)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	for r := range results {
		if r.(*pb.SCGetEnergy).GetCurrentEnergy() != 29 {
			t.Fatal("live amount or concurrent receipt changed reward")
		}
	}
	if p1Count(t, &db.EventFact{}, "player_id=? AND kind=?", b, "friend_energy_received") != 1 {
		t.Fatal("receipt repeated reward fact")
	}
	f.restart(t)
	if err := f.s.InitializeP10FriendEnergy(f.ctx); err != nil {
		t.Fatal(err)
	}
	if p10EnergyCall(t, f, b, &pb.CSGetEnergy{FriendId: proto.Int64(f.id)}, 1, now).(*pb.SCGetEnergy).GetCurrentEnergy() != 29 {
		t.Fatal("receipt restart")
	}
}
func TestP10FriendEnergyUnlinkReaddDoesNotRestoreOldGiftAndDayQuota(t *testing.T) {
	f, b, _ := p10EnergySetup(t)
	now := time.Now().AddDate(0, 0, -1)
	p10EnergyCall(t, f, f.id, &pb.CSGiveEnergy{FriendId: proto.Int64(b)}, 1, now)
	p10Call(t, f, b, &pb.CSDeleteFriend{FriendId: proto.Int64(f.id)}, 1, now)
	p10Call(t, f, f.id, &pb.CSApplyFriend{FriendId: proto.Int64(b)}, 2, now)
	p10Call(t, f, b, &pb.CSAgreeApply{FriendId: proto.Int64(f.id)}, 2, now)
	if _, err := f.s.p10FriendEnergy(f.ctx, b, &pb.CSGetEnergy{FriendId: proto.Int64(f.id)}, 1, now); err == nil {
		t.Fatal("same-millisecond readd restored invalid gift")
	}
	p10EnergyCall(t, f, f.id, &pb.CSGiveEnergy{FriendId: proto.Int64(b)}, 2, now)
	var gift db.P10FriendEnergyGift
	if err := db.GetIns().Where("sender_id=? AND receiver_id=? AND day=?", f.id, b, p1Day(now)).First(&gift).Error; err != nil {
		t.Fatal(err)
	}
	if !gift.Invalidated {
		t.Fatal("daily gift was recreated")
	}
	next := time.Now()
	p10EnergyCall(t, f, f.id, &pb.CSGiveEnergy{FriendId: proto.Int64(b)}, 3, next)
	r := p10EnergyCall(t, f, b, &pb.CSGetEnergy{FriendId: proto.Int64(f.id)}, 2, next).(*pb.SCGetEnergy)
	if r.GetCurrentEnergy() != 29 {
		t.Fatal("next day real new gift")
	}
}
func TestP10FriendEnergyBatchCapacityFifteenAndNoBalanceOverflow(t *testing.T) {
	f, b, c := p10EnergySetup(t)
	now := time.Now()
	senders := []int64{f.id, c}
	p10Call(t, f, c, &pb.CSApplyFriend{FriendId: proto.Int64(b)}, 1, now)
	p10Call(t, f, b, &pb.CSAgreeApply{FriendId: proto.Int64(c)}, 3, now)
	for i := 0; i < 14; i++ {
		a, err := LoginAccount(f.ctx, fmt.Sprintf("p10-gift-%d-%d", i, time.Now().UnixNano()), "local-test-password")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = CreatePlayer(f.ctx, a.Id, "耐力好友", "hero_047"); err != nil {
			t.Fatal(err)
		}
		p10Call(t, f, a.Id, &pb.CSApplyFriend{FriendId: proto.Int64(b)}, 1, now)
		p10Call(t, f, b, &pb.CSAgreeApply{FriendId: proto.Int64(a.Id)}, int64(4+i), now)
		senders = append(senders, a.Id)
	}
	if err := updateProfile(f.ctx, b, func(p *pb.Player, g *model.GameState) error { p.Energy = proto.Int32(0); return nil }); err != nil {
		t.Fatal(err)
	}
	for _, id := range senders {
		p10EnergyCall(t, f, id, &pb.CSBatchSendFriendEnergy{}, 1, now)
	}
	r := p10EnergyCall(t, f, b, &pb.CSBatchGetFriendEnergy{}, 1, now).(*pb.SCBatchGetFriendEnergy)
	if len(r.GetGetIds()) != 15 || r.GetAdd().GetSimps()[0].GetNum() != 15 {
		t.Fatal("original fifteen receive cap", r)
	}
	if !proto.Equal(r, p10EnergyCall(t, f, b, &pb.CSBatchGetFriendEnergy{}, 1, now)) {
		t.Fatal("batch retry")
	}
	second := p10EnergyCall(t, f, b, &pb.CSBatchGetFriendEnergy{}, 2, now).(*pb.SCBatchGetFriendEnergy)
	if len(second.GetGetIds()) != 0 {
		t.Fatal("daily cap bypass")
	}
	var daily db.P10FriendEnergyDaily
	if err := db.GetIns().Where("player_id=? AND day=?", b, p1Day(now)).First(&daily).Error; err != nil {
		t.Fatal(err)
	}
	if daily.Claimed != 15 {
		t.Fatal("daily count")
	}
	if p1Count(t, &db.P10FriendEnergyGift{}, "receiver_id=? AND claimed=?", b, true) != 15 {
		t.Fatal("batch lost pending gift")
	}
}
func TestP10FriendEnergyFullBalanceRollbackAndRealFlags(t *testing.T) {
	f, b, _ := p10EnergySetup(t)
	now := time.Now()
	p10EnergyCall(t, f, f.id, &pb.CSGiveEnergy{FriendId: proto.Int64(b)}, 1, now)
	if err := updateProfile(f.ctx, b, func(p *pb.Player, g *model.GameState) error { p.Energy = proto.Int32(30); return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p10FriendEnergy(f.ctx, b, &pb.CSGetEnergy{FriendId: proto.Int64(f.id)}, 1, now); err == nil {
		t.Fatal("full balance claimed")
	}
	if p1Count(t, &db.P10FriendEnergyGift{}, "receiver_id=? AND claimed=?", b, true) != 0 || p1Count(t, &db.P10SocialReceipt{}, "player_id=? AND domain=?", b, "energy-get") != 0 {
		t.Fatal("failed reward partially consumed")
	}
	list := p10Call(t, f, b, &pb.CSFriendList{}, 0, now).(*pb.SCFriendList)
	if list.GetFriendList()[0].GetCanGet() != 1 || list.GetFriendList()[0].GetCanGive() != 1 {
		t.Fatal("real pending flag", list)
	}
	if err := updateProfile(f.ctx, b, func(p *pb.Player, g *model.GameState) error { p.Energy = proto.Int32(29); return nil }); err != nil {
		t.Fatal(err)
	}
	p10EnergyCall(t, f, b, &pb.CSGetEnergy{FriendId: proto.Int64(f.id)}, 1, now)
	list = p10Call(t, f, b, &pb.CSFriendList{}, 0, now).(*pb.SCFriendList)
	if list.GetGetEnergyNum() != 1 || list.GetFriendList()[0].GetCanGet() != 0 {
		t.Fatal("claimed flag/count")
	}
	p10Call(t, f, b, &pb.CSSendToBlackList{FriendId: proto.Int64(f.id)}, 1, now)
	list = p10Call(t, f, b, &pb.CSFriendList{}, 0, now).(*pb.SCFriendList)
	if len(list.GetFriendList()) != 0 || list.GetGetEnergyNum() != 1 {
		t.Fatal("empty friend list erased actual daily claim count", list)
	}
}
