package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// This helper sets up a locked offer window for payment-focused tests. The
// story and sweep entry points are exercised independently below.
func luxuryTestStart(t *testing.T, ctx context.Context, s *GameService, id int64, now time.Time) {
	t.Helper()
	if err := updateProfileWithTx(ctx, id, func(tx *gorm.DB, _ *pb.Player, _ *model.GameState) error {
		return s.triggerLuxuryDragonLocked(tx, id, now)
	}); err != nil {
		t.Fatal(err)
	}
}

// The win inputs exercise the real story/sweep transactions in an isolated
// fixture; they do not certify the original client combat engine.
func TestLuxuryDragonStorySweepCyclesAndPaidClaim(t *testing.T) {
	f, id := p3DemoProviderFixture(t)
	f.id = id
	f.s.P3AutoConfirmDemoPayments = true
	for _, initialize := range []func() error{
		func() error { return f.s.InitCooking(f.ctx) },
		func() error { return f.s.InitializeDailyCopy(f.ctx) },
		func() error { return f.s.InitializeCopyDrops(f.ctx) },
	} {
		if err := initialize(); err != nil {
			t.Fatal(err)
		}
	}
	if err := updateProfile(f.ctx, id, func(p *pb.Player, _ *model.GameState) error {
		p.WarriorBagSize = proto.Int32(1000)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	open := &pb.CSOpenLuxuryEquipment{Chapter: proto.Int32(6), Mission: proto.Int32(1)}
	query, err := f.s.paymentOffer(f.ctx, id, open, time.Now())
	if err != nil || query[0].(*pb.SCOpenLuxuryEquipment).GetOpenOrNot() != 0 {
		t.Fatal("query created an offer without a successful action", err)
	}
	if _, err = f.s.copySweepAt(f.ctx, id, &pb.CSCopySweep{ChapterId: proto.String("1"), MissionId: proto.String("1")}, 10, time.Now()); err == nil {
		t.Fatal("invalid sweep succeeded")
	}
	if count := p1Count(t, &db.PaymentLuxuryOffer{}, "player_id=?", id); count != 0 {
		t.Fatal("invalid sweep opened an offer", count)
	}
	if _, err = f.s.copyEnter(f.ctx, id, &pb.CSCopyEnter{ChapterId: proto.String("1"), MissionId: proto.String("1")}); err != nil {
		t.Fatal(err)
	}
	var final *pb.CSCopyFightEnd
	tasks := f.s.tasks("1")
	for i, task := range tasks {
		messages, e := f.s.copyFightStart(f.ctx, id, &pb.CSCopyFightStart{ChapterId: proto.String("1"), MissionId: proto.String("1"), TaskId: proto.String(task["TaskId"])})
		if e != nil {
			t.Fatal(e)
		}
		start := messages[0].(*pb.SCCopyFightStart)
		final = &pb.CSCopyFightEnd{FightId: start.FightId, Result: proto.Int32(1), BattlePlayer: start.BattlePlayer}
		result, e := f.s.copyFightEnd(f.ctx, id, final)
		if e != nil {
			t.Fatal(e)
		}
		count := p1Count(t, &db.PaymentLuxuryOffer{}, "player_id=?", id)
		if i < len(tasks)-1 && count != 0 || i == len(tasks)-1 && (count != 1 || result[0].(*pb.SCCopyFightEnd).GetIsCopyComplete() != 1) {
			t.Fatal("story offer was not bound to whole-mission completion", i, count)
		}
	}
	var first db.PaymentLuxuryOffer
	if err = db.GetIns().Where("player_id=? AND offer_id=?", id, luxuryDragonOfferID).Take(&first).Error; err != nil {
		t.Fatal(err)
	}
	if first.Cents != 28800 || first.EndAt-first.StartAt != int64(12*time.Hour/time.Millisecond) {
		t.Fatal("wrong original offer window", first)
	}
	if _, err = f.s.copyFightEnd(f.ctx, id, final); err != nil {
		t.Fatal("story receipt replay", err)
	}
	var replayed db.PaymentLuxuryOffer
	if err = db.GetIns().Where("player_id=? AND offer_id=?", id, luxuryDragonOfferID).Take(&replayed).Error; err != nil || replayed != first {
		t.Fatal("story retry changed the offer", err)
	}
	// Move the isolated window into the past so real payment confirmations
	// remain within the test server's authoritative current-time boundary.
	first.StartAt = time.Now().Add(-25 * time.Hour).UnixMilli()
	first.EndAt = first.StartAt + int64(12*time.Hour/time.Millisecond)
	if err = db.GetIns().Model(&db.PaymentLuxuryOffer{}).Where("player_id=? AND offer_id=?", id, luxuryDragonOfferID).
		Updates(map[string]any{"start_at": first.StartAt, "end_at": first.EndAt}).Error; err != nil {
		t.Fatal(err)
	}
	if err = updateProfile(f.ctx, id, func(p *pb.Player, g *model.GameState) error {
		p.Level, p.Stamina = proto.Int32(14), proto.Int32(1000)
		for _, info := range g.Copy.PassedCopyInfo {
			if info.GetChapterId() == "1" && info.GetMissionId() == "1" {
				info.Star = proto.Int32(5)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	p3DemoProviderFund(t, f, id, "luxury-partial", 3000)
	if _, err = f.s.generatePayment(f.ctx, id, p3DemoProviderRequest(t, "djsy.wuyou.andp0030"), 30001, time.UnixMilli(first.StartAt).Add(time.Minute)); err != nil {
		t.Fatal("partial real order", err)
	}
	sweep := &pb.CSCopySweep{ChapterId: proto.String("1"), MissionId: proto.String("1")}
	if _, err = f.s.copySweepAt(f.ctx, id, sweep, 111, time.UnixMilli(first.EndAt-1)); err != nil {
		t.Fatal("in-window sweep", err)
	}
	if _, err = f.s.copySweepAt(f.ctx, id, sweep, 111, time.UnixMilli(first.EndAt+1)); err != nil {
		t.Fatal("cross-window sweep replay", err)
	}
	if err = db.GetIns().Where("player_id=? AND offer_id=?", id, luxuryDragonOfferID).Take(&replayed).Error; err != nil || replayed != first {
		t.Fatal("in-window action or replay restarted the offer", err)
	}
	if _, err = f.s.copySweepAt(f.ctx, id, sweep, 112, time.UnixMilli(first.EndAt)); err != nil {
		t.Fatal("new successful sweep after expiration", err)
	}
	var second db.PaymentLuxuryOffer
	if err = db.GetIns().Where("player_id=? AND offer_id=?", id, luxuryDragonOfferID).Take(&second).Error; err != nil || second.StartAt != first.EndAt || second.EndAt-second.StartAt != first.EndAt-first.StartAt {
		t.Fatal("new window missing at 12-hour boundary", second, err)
	}
	var history db.PaymentLuxuryRound
	if err = db.GetIns().Where("player_id=? AND offer_id=? AND start_at=?", id, luxuryDragonOfferID, first.StartAt).Take(&history).Error; err != nil || history.EndAt != first.EndAt || history.ConfirmedCents != 3000 {
		t.Fatal("expired partial-payment window not retained", history, err)
	}
	info, err := f.s.paymentOffer(f.ctx, id, &pb.CSLuxuryEquipmentInfo{}, time.UnixMilli(second.StartAt))
	if err != nil || info[0].(*pb.SCLuxuryEquipmentInfo).GetRewardState() != 0 {
		t.Fatal("partial payment leaked into the next window", err)
	}
	p3DemoProviderFund(t, f, id, "luxury-full", 28800)
	if _, err = f.s.generatePayment(f.ctx, id, p3DemoProviderRequest(t, "djsy.wuyou.andp0288"), 30002, time.UnixMilli(second.StartAt).Add(time.Minute)); err != nil {
		t.Fatal("qualifying real order", err)
	}
	if _, err = f.s.copySweepAt(f.ctx, id, sweep, 113, time.UnixMilli(second.EndAt)); err != nil {
		t.Fatal("paid player sweep", err)
	}
	if err = db.GetIns().Where("player_id=? AND offer_id=?", id, luxuryDragonOfferID).Take(&replayed).Error; err != nil || replayed != second {
		t.Fatal("paid unclaimed offer restarted", err)
	}
	info, err = f.s.paymentOffer(f.ctx, id, &pb.CSLuxuryEquipmentInfo{}, time.UnixMilli(second.EndAt))
	if err != nil || info[0].(*pb.SCLuxuryEquipmentInfo).GetId() != 2 || info[0].(*pb.SCLuxuryEquipmentInfo).GetRewardState() != 1 {
		t.Fatal("earned unclaimed reward disappeared after expiry", err)
	}
	if _, err = f.s.paymentOffer(f.ctx, id, &pb.CSLuxuryEquipmentReward{}, time.UnixMilli(second.EndAt)); err != nil {
		t.Fatal("earned reward could not be claimed", err)
	}
	if _, err = f.s.paymentOffer(f.ctx, id, &pb.CSLuxuryEquipmentReward{}, time.UnixMilli(second.EndAt+1)); err == nil {
		t.Fatal("duplicate reward claim succeeded")
	}
	_, state := f.profile(t)
	items := map[string]int{}
	for _, item := range state.Equipment {
		items[item.GetEquipId()]++
	}
	for _, key := range []string{"equip_033", "equip_034", "equip_035", "equip_036"} {
		if items[key] != 1 {
			t.Fatal("missing or duplicate dragon item", key, items)
		}
	}
	f.restart(t)
	if _, err = f.s.copySweepAt(f.ctx, id, sweep, 114, time.UnixMilli(second.EndAt).Add(30*time.Minute)); err != nil {
		t.Fatal("cold paid player sweep", err)
	}
	if err = db.GetIns().Where("player_id=? AND offer_id=?", id, luxuryDragonOfferID).Take(&replayed).Error; err != nil || !replayed.Claimed || replayed.StartAt != second.StartAt {
		t.Fatal("claimed offer restarted after cold recovery", err)
	}
}

func TestLuxuryDragonTriggerRollsBackWithStory(t *testing.T) {
	f, id := p3DemoProviderFixture(t)
	forced := errors.New("isolated rollback")
	err := updateProfileWithTx(f.ctx, id, func(tx *gorm.DB, _ *pb.Player, _ *model.GameState) error {
		if e := f.s.triggerLuxuryDragonLocked(tx, id, time.Now()); e != nil {
			return e
		}
		return forced
	})
	if !errors.Is(err, forced) || p1Count(t, &db.PaymentLuxuryOffer{}, "player_id=?", id) != 0 {
		t.Fatal("offer escaped the enclosing rollback", err)
	}
}

func TestLuxuryDragonConcurrentRenewalMySQL(t *testing.T) {
	f, id := p3DemoProviderFixture(t)
	if db.GetIns().Dialector.Name() != "mysql" {
		t.Skip("requires isolated InnoDB")
	}
	old := time.Now().Add(-25 * time.Hour).Truncate(time.Millisecond)
	luxuryTestStart(t, f.ctx, f.s, id, old)
	at := old.Add(13 * time.Hour)
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	start := make(chan struct{})
	for i := 0; i < cap(errs); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs <- updateProfileWithTx(f.ctx, id, func(tx *gorm.DB, _ *pb.Player, _ *model.GameState) error {
				return f.s.triggerLuxuryDragonLocked(tx, id, at)
			})
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal("concurrent trigger", err)
		}
	}
	var current db.PaymentLuxuryOffer
	if err := db.GetIns().Where("player_id=? AND offer_id=?", id, luxuryDragonOfferID).Take(&current).Error; err != nil || current.StartAt != at.UnixMilli() {
		t.Fatal("concurrent renewal did not retain one window", current, err)
	}
	if got := p1Count(t, &db.PaymentLuxuryRound{}, "player_id=? AND offer_id=? AND start_at=?", id, luxuryDragonOfferID, old.UnixMilli()); got != 1 {
		t.Fatal("concurrent renewal did not retain one history row", got)
	}
}
