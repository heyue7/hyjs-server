package service

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestP1LatePublishedLoginPeriodsUseRealDaysOnce(t *testing.T) {
	f := p1Setup(t)
	now := time.Now()
	yesterday := now.AddDate(0, 0, -1)
	f.event(t, "login:"+p1Day(yesterday), constUtil.P1EventLoginDay, 1, yesterday)
	if err := f.s.RecordP1Login(f.ctx, f.id); err != nil {
		t.Fatal(err)
	}
	before, _ := f.profile(t)
	for _, spec := range []struct {
		key  string
		goal int64
	}{{"late_login_open", 2}, {"late_login_three", 3}, {"late_login_closed", 1}} {
		end := now.Add(time.Hour)
		if spec.key == "late_login_closed" {
			end = now.Add(-time.Second)
		}
		p1Instance(t, f, spec.key, constUtil.P1EventLoginDay, "cumulative", spec.goal, end, 0)
	}
	// An unrelated source on the same actual day cannot raise login eligibility.
	f.event(t, "other-real-action", constUtil.P1EventLoginDay, 1, now)
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- f.s.RecordP1Login(f.ctx, f.id) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	open := p1ProgressValue(t, f, "late_login_open")
	three := p1ProgressValue(t, f, "late_login_three")
	// The initial unrelated action already counted through the generic event
	// path. Reconciliation adds only real distinct login days and never lowers
	// an existing progress or produces a second delivery.
	if open.Value != 2 || open.RewardID == 0 || three.Value != 2 || three.RewardID != 0 {
		t.Fatalf("wrong real login progress: %+v / %+v", open, three)
	}
	if p1Count(t, &db.PeriodProgress{}, "instance_id = ? AND player_id = ?", "late_login_closed", f.id) != 0 || p1Count(t, &db.RewardDelivery{}, "player_id = ? AND source_key = ?", f.id, "activity:late_login_open") != 1 {
		t.Fatal("closed window changed or duplicate delivery")
	}
	f.restart(t)
	if err := f.s.RecordP1Login(f.ctx, f.id); err != nil {
		t.Fatal(err)
	}
	if progress := p1ProgressValue(t, f, "late_login_open"); progress.Value != open.Value || progress.RewardID != open.RewardID {
		t.Fatal("cold login changed receipt")
	}
	after, _ := f.profile(t)
	if !proto.Equal(before, after) {
		t.Fatal("login reconciliation changed wallet")
	}
}

func TestP1LoginReconciliationPreservesFrozenWindow(t *testing.T) {
	f := p1Setup(t)
	now := time.Now()
	yesterday := now.AddDate(0, 0, -1)
	f.event(t, "login:"+p1Day(yesterday), constUtil.P1EventLoginDay, 1, yesterday)
	cfg := p1Instance(t, f, "late_login_frozen", constUtil.P1EventLoginDay, "cumulative", 3, now.Add(time.Hour), 0)
	if err := f.s.RecordP1Login(f.ctx, f.id); err != nil {
		t.Fatal(err)
	}
	old := p1ProgressValue(t, f, cfg.ID)
	if old.Value != 2 {
		t.Fatal("missing real past day")
	}
	if err := db.GetIns().Model(&db.PeriodInstance{}).Where("id = ?", cfg.ID).Updates(map[string]any{"goal": 1, "start_at": now.UnixMilli(), "rewards": "diamond,999"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.RecordP1Login(f.ctx, f.id); err != nil {
		t.Fatal(err)
	}
	current := p1ProgressValue(t, f, cfg.ID)
	if string(old.Snapshot) != string(current.Snapshot) || current.Value != 2 || current.RewardID != 0 {
		t.Fatal("hot policy changed frozen eligibility")
	}
	// Even exact old sources cannot reopen the frozen counting deadline.
	if err := updateProfileWithTx(f.ctx, f.id, func(tx *gorm.DB, _ *pb.Player, _ *model.GameState) error {
		return f.s.p1ReconcileLoginPeriods(tx, f.id, now.Add(2*time.Hour))
	}); err != nil {
		t.Fatal(fmt.Errorf("closed frozen window: %w", err))
	}
	if current = p1ProgressValue(t, f, cfg.ID); current.Value != 2 || current.RewardID != 0 {
		t.Fatal("closed snapshot changed")
	}
}

func TestP1LateCopyPeriodCountsCommittedFactsWithinFrozenWindow(t *testing.T) {
	f := p1Setup(t)
	now := time.Now()
	for i := range 13 {
		f.event(t, fmt.Sprintf("actual-copy:%d", i), constUtil.P1EventCopyWin, 1, now.Add(-time.Hour))
	}
	f.event(t, "outside-window", constUtil.P1EventCopyWin, 1, now.Add(-72*time.Hour))
	account, err := LoginAccount(f.ctx, "other-copy-"+fmt.Sprint(now.UnixNano()), "isolated-test-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(f.ctx, account.Id, "独立归属回归", "hero_043"); err != nil {
		t.Fatal(err)
	}
	if err = updateProfileWithTx(f.ctx, account.Id, func(tx *gorm.DB, _ *pb.Player, _ *model.GameState) error {
		return f.s.RecordP1Event(tx, account.Id, "other-owner", constUtil.P1EventCopyWin, 50, now)
	}); err != nil {
		t.Fatal(err)
	}
	key := "kaiGongActivity_p4_late_copy"
	p1Instance(t, f, key, constUtil.P1EventCopyWin, "cumulative", 10, now.Add(time.Hour), 0)
	closed := "kaiGongActivity_p4_closed_copy"
	p1Instance(t, f, closed, constUtil.P1EventCopyWin, "cumulative", 10, now.Add(-time.Second), 0)
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := f.s.P1Activity(f.ctx, f.id, &pb.CSActivityInfo{ActivityId: proto.String(key)})
			errs <- e
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	old := p1ProgressValue(t, f, key)
	if old.Value != 13 || old.RewardID == 0 || p1Count(t, &db.RewardDelivery{}, "player_id = ? AND source_key = ?", f.id, "activity:"+key) != 1 {
		t.Fatal("wrong actual copy total or duplicate reward")
	}
	replies, err := f.s.P1Activity(f.ctx, f.id, &pb.CSActivityInfo{ActivityId: proto.String(closed)})
	if err != nil {
		t.Fatal(err)
	}
	if replies[0].(*pb.SCActivityInfo).GetItemList()[0].GetLeft() != 0 || p1Count(t, &db.PeriodProgress{}, "player_id = ? AND instance_id = ?", f.id, closed) != 0 {
		t.Fatal("closed period backfilled")
	}
	if err = db.GetIns().Model(&db.PeriodInstance{}).Where("id = ?", key).Updates(map[string]any{"goal": 1, "rewards": "diamond,999"}).Error; err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	if _, err = f.s.P1Activity(f.ctx, f.id, &pb.CSActivityInfo{ActivityId: proto.String(key)}); err != nil {
		t.Fatal(err)
	}
	current := p1ProgressValue(t, f, key)
	if current.Value != 13 || current.RewardID != old.RewardID || string(current.Snapshot) != string(old.Snapshot) {
		t.Fatal("cold/hot policy changed frozen copy count")
	}
}
