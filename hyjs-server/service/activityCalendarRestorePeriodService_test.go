package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func TestActivityCalendarRestorePeriodRequiresTransaction(t *testing.T) {
	conn, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "period.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	pool, err := conn.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := RestoreActivityCalendarPeriodWindow(conn, "period", "request", FrozenJSONWindow{1, 2, 3}, FrozenJSONWindow{4, 5, 6}); err == nil {
		t.Fatal("expected non-transactional call to fail")
	}
}

func TestActivityCalendarRestorePeriodPreservesFactsAndRollback(t *testing.T) {
	for _, scenario := range []string{"success", "outer-rollback", "settlement", "snapshot-identity", "snapshot-window", "delivery-deadline", "reward-identity", "payment-without-carry", "payment-with-carry", "ranked", "snapshot-ranked", "archive-write-failure"} {
		t.Run(scenario, func(t *testing.T) {
			tx, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "period.db")), &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			pool, err := tx.DB()
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			if err = tx.AutoMigrate(&db.PeriodInstance{}, &db.PeriodProgress{}, &db.PeriodSettlement{}, &db.RewardDelivery{}, &db.ActivityRechargeCarry{}, &db.ActivityCalendarPeriodArchive{}); err != nil {
				t.Fatal(err)
			}
			before := FrozenJSONWindow{1000, 2000, 3000}
			after := FrozenJSONWindow{4000, 5000, 6000}
			live := db.PeriodInstance{ID: "period", RewardSourceID: "original", Enabled: true, Kind: "kill", Mode: "cumulative", Goal: 3, Rewards: "coin,1", StartAt: 1000, EndAt: 2000, RewardEndAt: 3000, Milestones: []byte(`[{"goal":3,"rewards":"coin,1"},{"goal":6,"rewards":"coin,2"}]`)}
			if scenario == "ranked" {
				live.RankPlaces = 1
			}
			if scenario == "payment-without-carry" || scenario == "payment-with-carry" {
				live.Kind = "payment_amount"
			}
			raw, _ := json.Marshal(live)
			live.Frozen = raw
			progress := db.PeriodProgress{InstanceID: live.ID, PlayerID: 7, Snapshot: append([]byte(nil), raw...), Value: 99, LastDay: "2026-10-27", RewardID: 1}
			claimed := db.RewardDelivery{ID: 1, PlayerID: 7, SourceKey: "activity:original", Rewards: "coin,1", ClaimedAt: 1500, ExpiresAt: 3000, CreatedAt: 1100, Result: []byte{0, 1, 2, 255}}
			pending := db.RewardDelivery{ID: 2, PlayerID: 7, SourceKey: "activity:original:tier:2", Rewards: "coin,2", ExpiresAt: 3000, CreatedAt: 1200, Result: []byte("untouched")}
			if scenario == "snapshot-identity" {
				wrong := live
				wrong.ID = "other"
				progress.Snapshot, _ = json.Marshal(wrong)
			}
			if scenario == "snapshot-ranked" {
				wrong := live
				wrong.RankPlaces = 1
				progress.Snapshot, _ = json.Marshal(wrong)
			}
			if scenario == "snapshot-window" {
				wrong := live
				wrong.EndAt++
				progress.Snapshot, _ = json.Marshal(wrong)
			}
			if scenario == "delivery-deadline" {
				pending.ExpiresAt++
			}
			if scenario == "reward-identity" {
				progress.RewardID = 2
			}
			for _, row := range []any{&live, &progress, &claimed, &pending} {
				if err = tx.Create(row).Error; err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "settlement" {
				if err = tx.Create(&db.PeriodSettlement{InstanceID: live.ID, SettledAt: 2000, Snapshot: []byte("frozen")}).Error; err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "archive-write-failure" {
				settledRaw, _ := json.Marshal(p1Settlement{Rules: live, Ranks: []p1Rank{{PlayerID: 7, Value: 99, Rank: 1}}})
				if err = tx.Create(&db.PeriodSettlement{InstanceID: live.ID, SettledAt: 2000, Snapshot: settledRaw}).Error; err != nil {
					t.Fatal(err)
				}
				if err = tx.Callback().Create().Before("gorm:create").Register("reject_owned_archive", func(inner *gorm.DB) {
					if inner.Statement.Table == "activity_calendar_period_archives" {
						inner.AddError(errors.New("archive write interrupted"))
					}
				}); err != nil {
					t.Fatal(err)
				}
			}
			interrupt := errors.New("outer interrupted")
			err = tx.Transaction(func(inner *gorm.DB) error {
				if scenario == "payment-with-carry" {
					carry := db.ActivityRechargeCarry{InstanceID: live.ID, PlayerID: 7, RequestID: "request", OpeningAt: after.StartAt, RealCutoff: 9000, Mode: live.Mode, Facts: []byte("[]"), Baseline: []byte("[]")}
					carry.Digest = activityRechargeCarryDigest(carry)
					if err := SaveActivityRechargeCarry(inner, &carry); err != nil {
						return err
					}
				}
				if err := RestoreActivityCalendarPeriodWindow(inner, live.ID, "request", before, after); err != nil {
					return err
				}
				if scenario == "outer-rollback" {
					return interrupt
				}
				return nil
			})
			success := scenario == "success" || scenario == "payment-with-carry"
			if success && err != nil {
				t.Fatal(err)
			}
			if !success && err == nil {
				t.Fatal("accepted unresolved dependency")
			}
			if scenario == "archive-write-failure" {
				var count int64
				if err = tx.Model(&db.PeriodSettlement{}).Where("instance_id=?", live.ID).Count(&count).Error; err != nil || count != 1 {
					t.Fatalf("archive failure deleted settlement: %d %v", count, err)
				}
				if err = tx.Model(&db.ActivityCalendarPeriodArchive{}).Where("instance_id=?", live.ID).Count(&count).Error; err != nil || count != 0 {
					t.Fatalf("archive failure retained partial archive: %d %v", count, err)
				}
			}
			var gotLive db.PeriodInstance
			var gotProgress db.PeriodProgress
			var gotClaimed, gotPending db.RewardDelivery
			for _, item := range []struct {
				row any
				id  any
			}{{&gotLive, live.ID}, {&gotProgress, nil}, {&gotClaimed, int64(1)}, {&gotPending, int64(2)}} {
				query := tx
				if item.row == &gotProgress {
					query = query.Where("instance_id=? AND player_id=?", live.ID, 7)
				} else {
					query = query.Where("id=?", item.id)
				}
				if err = query.Take(item.row).Error; err != nil {
					t.Fatal(err)
				}
			}
			if !reflect.DeepEqual(claimed, gotClaimed) {
				t.Fatalf("claimed receipt changed: %+v", gotClaimed)
			}
			if success {
				var snapshot db.PeriodInstance
				if err = json.Unmarshal(gotProgress.Snapshot, &snapshot); err != nil {
					t.Fatal(err)
				}
				if snapshot.StartAt != 4000 || snapshot.EndAt != 5000 || snapshot.RewardEndAt != 6000 {
					t.Fatalf("snapshot window: %+v", snapshot)
				}
				gotLive.StartAt, gotLive.EndAt, gotLive.RewardEndAt = live.StartAt, live.EndAt, live.RewardEndAt
				var frozen db.PeriodInstance
				if err = json.Unmarshal(gotLive.Frozen, &frozen); err != nil {
					t.Fatal(err)
				}
				if frozen.StartAt != 4000 || frozen.EndAt != 5000 || frozen.RewardEndAt != 6000 {
					t.Fatal("frozen window not migrated")
				}
				if gotPending.ExpiresAt != after.ClaimEndAt {
					t.Fatal("pending delivery deadline not migrated")
				}
				frozen.StartAt, frozen.EndAt, frozen.RewardEndAt = live.StartAt, live.EndAt, live.RewardEndAt
				frozen.Frozen = live.Frozen
				if !reflect.DeepEqual(frozen, live) {
					t.Fatal("frozen rules changed")
				}
				snapshot.StartAt, snapshot.EndAt, snapshot.RewardEndAt = live.StartAt, live.EndAt, live.RewardEndAt
				snapshot.Frozen = live.Frozen
				if !reflect.DeepEqual(snapshot, live) {
					t.Fatal("progress frozen rules changed")
				}
				gotLive.Frozen = live.Frozen
				gotProgress.Snapshot = progress.Snapshot
				gotPending.ExpiresAt = pending.ExpiresAt
			}
			if !reflect.DeepEqual(live, gotLive) || !reflect.DeepEqual(progress, gotProgress) || !reflect.DeepEqual(pending, gotPending) {
				t.Fatalf("unexpected data change success=%v live=%+v progress=%+v pending=%+v", success, gotLive, gotProgress, gotPending)
			}
		})
	}
}

func TestActivityCalendarRestoreSettledPeriodLifecycle(t *testing.T) {
	f := p1Setup(t)
	tx := db.GetIns()
	if err := tx.AutoMigrate(&db.ActivityCalendarPeriodArchive{}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	live := p1Instance(t, f, "p1:calendar-reopen-period", "calendar-reopen-event", "cumulative", 3, now.Add(2*time.Second), 0)
	f.event(t, "reopen:old", live.Kind, 3, now)
	progress := p1ProgressValue(t, f, live.ID)
	if progress.Value != 3 {
		t.Fatalf("old progress=%d", progress.Value)
	}
	request := &pb.CSActivityReward{ActivityId: proto.String(live.ID), Index: proto.Int32(1), GetResources: []*pb.BaseResource{{Id: proto.String("coin"), Num: proto.Int32(17)}, {Id: proto.String("item_001"), Num: proto.Int32(1)}}}
	if _, err := f.s.P1Activity(f.ctx, f.id, request); err != nil {
		t.Fatal(err)
	}
	progress = p1ProgressValue(t, f, live.ID)
	var oldReward db.RewardDelivery
	if err := tx.First(&oldReward, progress.RewardID).Error; err != nil {
		t.Fatal(err)
	}
	if oldReward.ClaimedAt == 0 {
		t.Fatal("old reward was not claimed")
	}
	paidPlayer, _ := f.profile(t)
	originalCoin, originalDiamond := paidPlayer.GetCoin(), paidPlayer.GetDiamond()
	time.Sleep(max(time.Duration(0), time.Until(time.UnixMilli(live.EndAt))) + 20*time.Millisecond)
	if err := f.s.SettleP1Periods(f.ctx); err != nil {
		t.Fatal(err)
	}
	var oldSettlement db.PeriodSettlement
	if err := tx.Where("instance_id=?", live.ID).Take(&oldSettlement).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Where("id=?", live.ID).Take(&live).Error; err != nil {
		t.Fatal(err)
	}
	before := FrozenJSONWindow{live.StartAt, live.EndAt, live.RewardEndAt}
	wall := time.Now()
	after := FrozenJSONWindow{wall.Add(-time.Minute).UnixMilli(), wall.Add(3 * time.Second).UnixMilli(), wall.Add(time.Hour).UnixMilli()}
	migrate := func(request string) error {
		return tx.Transaction(func(inner *gorm.DB) error {
			return RestoreActivityCalendarPeriodWindow(inner, live.ID, request, before, after)
		})
	}
	interrupted := errors.New("interrupt after archive and delete")
	if err := tx.Transaction(func(inner *gorm.DB) error {
		if err := RestoreActivityCalendarPeriodWindow(inner, live.ID, "restore", before, after); err != nil {
			return err
		}
		return interrupted
	}); !errors.Is(err, interrupted) {
		t.Fatal(err)
	}
	if p1Count(t, &db.PeriodSettlement{}, "instance_id=?", live.ID) != 1 || p1Count(t, &db.ActivityCalendarPeriodArchive{}, "instance_id=?", live.ID) != 0 {
		t.Fatal("interruption did not roll back archive/delete")
	}
	if err := migrate("restore"); err != nil {
		t.Fatal(err)
	}
	if err := migrate("restore"); err != nil {
		t.Fatal("same request retry", err)
	}
	if err := migrate("other"); err == nil {
		t.Fatal("different request accepted")
	}
	changedTarget := after
	changedTarget.StartAt++
	if err := tx.Transaction(func(inner *gorm.DB) error {
		return RestoreActivityCalendarPeriodWindow(inner, live.ID, "restore", before, changedTarget)
	}); err == nil {
		t.Fatal("same request accepted changed target")
	}

	var archive db.ActivityCalendarPeriodArchive
	if err := tx.Where("instance_id=?", live.ID).Take(&archive).Error; err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(archive.Snapshot, oldSettlement.Snapshot) || archive.SettledAt != oldSettlement.SettledAt {
		t.Fatal("archive changed historical settlement")
	}
	f.event(t, "reopen:old", live.Kind, 3, time.Now())
	if got := p1ProgressValue(t, f, live.ID); got.Value != 3 {
		t.Fatal("old fact counted again after reopen")
	}
	f.event(t, "reopen:new", live.Kind, 2, time.Now())
	if got := p1ProgressValue(t, f, live.ID); got.Value != 5 || got.RewardID != oldReward.ID {
		t.Fatalf("reopened progress=%+v", got)
	}
	f.event(t, "reopen:new", live.Kind, 2, time.Now())
	if got := p1ProgressValue(t, f, live.ID); got.Value != 5 {
		t.Fatalf("event retry counted twice: %+v", got)
	}
	if _, err := f.s.P1Activity(f.ctx, f.id, request); err != nil {
		t.Fatal(err)
	}
	time.Sleep(max(time.Duration(0), time.Until(time.UnixMilli(after.EndAt))) + 20*time.Millisecond)
	if err := f.s.SettleP1Periods(f.ctx); err != nil {
		t.Fatal(err)
	}
	var newSettlement db.PeriodSettlement
	if err := tx.Where("instance_id=?", live.ID).Take(&newSettlement).Error; err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(newSettlement.Snapshot, oldSettlement.Snapshot) {
		t.Fatal("new cutoff reused old snapshot")
	}
	if err := f.s.SettleP1Periods(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := migrate("restore"); err != nil {
		t.Fatal("retry after new cutoff", err)
	}
	var again db.PeriodSettlement
	if err := tx.Where("instance_id=?", live.ID).Take(&again).Error; err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(again, newSettlement) {
		t.Fatal("restore retry consumed new cutoff")
	}
	var reward db.RewardDelivery
	if err := tx.First(&reward, oldReward.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(reward, oldReward) || p1Count(t, &db.RewardDelivery{}, "player_id=? AND source_key=?", f.id, "activity:"+live.ID) != 1 {
		t.Fatal("old reward changed or duplicated")
	}
	finalPlayer, _ := f.profile(t)
	if finalPlayer.GetCoin() != originalCoin || finalPlayer.GetDiamond() != originalDiamond {
		t.Fatal("restore or reward retry changed player assets")
	}
	f.event(t, "reopen:late", live.Kind, 99, time.Now())
	if got := p1ProgressValue(t, f, live.ID); got.Value != 5 {
		t.Fatal("new cutoff failed to freeze progress")
	}
}

func calendarPeriodOwnershipFixture(t *testing.T) (*gorm.DB, db.PeriodInstance, db.PeriodProgress, db.RewardDelivery) {
	t.Helper()
	conn, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "period-ownership.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	pool, err := conn.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Close() })
	if err := conn.AutoMigrate(&db.PeriodInstance{}, &db.PeriodProgress{}, &db.PeriodSettlement{}, &db.RewardDelivery{}, &db.ActivityCalendarPeriodArchive{}); err != nil {
		t.Fatal(err)
	}
	live := db.PeriodInstance{ID: "character-seven", RewardSourceID: "shared-original", Enabled: true, Kind: "kill", Mode: "cumulative", Goal: 3, Rewards: "coin,1", StartAt: 1000, EndAt: 2000, RewardEndAt: 3000}
	raw := adminJSON(live)
	live.Frozen = append([]byte(nil), raw...)
	progress := db.PeriodProgress{InstanceID: live.ID, PlayerID: 7, Snapshot: append([]byte(nil), raw...), Value: 9, LastDay: "2026-10-27", RewardID: 1}
	delivery := db.RewardDelivery{ID: 1, PlayerID: 7, SourceKey: "activity:shared-original", Title: "original", Content: "retained", Rewards: "coin,1", CreatedAt: 1200, ExpiresAt: 3000, Result: []byte{0, 255, 1}}
	for _, row := range []any{&live, &progress, &delivery} {
		if err := conn.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	return conn, live, progress, delivery
}

func TestActivityCalendarRestorePeriodSharedSourceOwnership(t *testing.T) {
	for _, scenario := range []string{"different-players", "outer-rollback", "global-multiple-owners", "same-player-alias", "archived-alias", "invalid-historical-snapshot"} {
		t.Run(scenario, func(t *testing.T) {
			conn, live, progress, delivery := calendarPeriodOwnershipFixture(t)
			before, after := FrozenJSONWindow{1000, 2000, 3000}, FrozenJSONWindow{4000, 5000, 6000}
			peer := live
			peer.ID, peer.EndAt, peer.RewardEndAt, peer.Frozen = "other-character-period", 2500, 9999, nil
			peerProgress := db.PeriodProgress{InstanceID: peer.ID, PlayerID: 8, Snapshot: adminJSON(peer), Value: 100, LastDay: "2026-10-26", RewardID: 2}
			peerDelivery := db.RewardDelivery{ID: 2, PlayerID: 8, SourceKey: delivery.SourceKey, Title: "other owner", Rewards: "coin,1", CreatedAt: 1100, ExpiresAt: 9999, Result: []byte{128, 0, 255}}
			alias := scenario == "same-player-alias" || scenario == "archived-alias" || scenario == "invalid-historical-snapshot"
			if scenario == "global-multiple-owners" {
				peerProgress.InstanceID, peerProgress.Snapshot = live.ID, append([]byte(nil), progress.Snapshot...)
				peerDelivery.ExpiresAt = before.ClaimEndAt
			} else {
				if err := conn.Create(&peer).Error; err != nil {
					t.Fatal(err)
				}
			}
			if alias {
				peerProgress.PlayerID, peerProgress.RewardID = 7, delivery.ID
				if scenario == "invalid-historical-snapshot" {
					peerProgress.Snapshot = []byte("invalid historical snapshot")
				}
			} else if err := conn.Create(&peerDelivery).Error; err != nil {
				t.Fatal(err)
			}
			if err := conn.Create(&peerProgress).Error; err != nil {
				t.Fatal(err)
			}
			if scenario == "archived-alias" {
				archive := db.ActivityCalendarPeriodArchive{InstanceID: peer.ID, RequestID: "prior-archive", SettledAt: peer.EndAt, Snapshot: adminJSON(p1Settlement{Rules: peer}), BeforeStartAt: peer.StartAt, BeforeEndAt: peer.EndAt, BeforeClaimEndAt: peer.RewardEndAt, AfterStartAt: peer.StartAt, AfterEndAt: peer.EndAt, AfterClaimEndAt: peer.RewardEndAt}
				archive.Digest = activityCalendarPeriodArchiveDigest(archive)
				if err := conn.Create(&archive).Error; err != nil {
					t.Fatal(err)
				}
				if err := conn.Delete(&db.PeriodInstance{}, "id=?", peer.ID).Error; err != nil {
					t.Fatal(err)
				}
			}
			interrupted := errors.New("interrupt shared-source restore")
			err := conn.Transaction(func(tx *gorm.DB) error {
				if err := RestoreActivityCalendarPeriodWindow(tx, live.ID, "restore", before, after); err != nil {
					return err
				}
				if scenario == "outer-rollback" {
					return interrupted
				}
				return nil
			})
			success := scenario == "different-players" || scenario == "global-multiple-owners"
			if success && err != nil {
				t.Fatal(err)
			}
			if !success && err == nil {
				t.Fatal("ambiguous or interrupted restore committed")
			}
			if scenario == "outer-rollback" && !errors.Is(err, interrupted) {
				t.Fatalf("ownership prevented the expected rollback path: %v", err)
			}
			var gotLive db.PeriodInstance
			var gotProgress, gotPeerProgress db.PeriodProgress
			var gotDelivery db.RewardDelivery
			if err := conn.First(&gotLive, "id=?", live.ID).Error; err != nil {
				t.Fatal(err)
			}
			if err := conn.First(&gotProgress, "instance_id=? AND player_id=?", live.ID, 7).Error; err != nil {
				t.Fatal(err)
			}
			if err := conn.First(&gotPeerProgress, "instance_id=? AND player_id=?", peerProgress.InstanceID, peerProgress.PlayerID).Error; err != nil {
				t.Fatal(err)
			}
			if err := conn.First(&gotDelivery, delivery.ID).Error; err != nil {
				t.Fatal(err)
			}
			if success {
				if gotLive.StartAt != after.StartAt || gotDelivery.ExpiresAt != after.ClaimEndAt {
					t.Fatalf("owned window/deadline not migrated: %+v %+v", gotLive, gotDelivery)
				}
				gotDelivery.ExpiresAt = delivery.ExpiresAt
				gotProgress.Snapshot = progress.Snapshot
			} else if !reflect.DeepEqual(gotLive, live) {
				t.Fatal("failed ownership validation changed live window")
			}
			if !reflect.DeepEqual(gotProgress, progress) || !reflect.DeepEqual(gotDelivery, delivery) {
				t.Fatalf("progress or delivery bytes changed: %+v %+v", gotProgress, gotDelivery)
			}
			if !alias {
				var gotPeerDelivery db.RewardDelivery
				if err := conn.First(&gotPeerDelivery, peerDelivery.ID).Error; err != nil {
					t.Fatal(err)
				}
				if scenario == "global-multiple-owners" {
					if gotPeerDelivery.ExpiresAt != after.ClaimEndAt {
						t.Fatal("global period second owner deadline not migrated")
					}
					gotPeerDelivery.ExpiresAt = peerDelivery.ExpiresAt
					gotPeerProgress.Snapshot = peerProgress.Snapshot
				}
				if !reflect.DeepEqual(gotPeerDelivery, peerDelivery) {
					t.Fatalf("other player's delivery bytes/deadline changed: %+v", gotPeerDelivery)
				}
			}
			if !reflect.DeepEqual(gotPeerProgress, peerProgress) {
				t.Fatal("historical/shared-source progress changed")
			}
		})
	}
}

func TestActivityCalendarRestorePeriodOwnershipScanBounds(t *testing.T) {
	for _, scope := range []string{"current", "historical"} {
		t.Run(scope, func(t *testing.T) {
			conn, live, progress, delivery := calendarPeriodOwnershipFixture(t)
			if scope == "current" {
				if err := conn.Delete(&progress).Error; err != nil {
					t.Fatal(err)
				}
			}
			rows := make([]db.PeriodProgress, 10001)
			for i := range rows {
				rows[i] = progress
				if scope == "current" {
					rows[i].PlayerID = int64(i + 100)
					rows[i].RewardID = 0
				} else {
					cfg := live
					cfg.ID, cfg.RewardSourceID, cfg.Frozen = fmt.Sprintf("unrelated-%05d", i), "", nil
					rows[i].InstanceID, rows[i].Snapshot, rows[i].RewardID = cfg.ID, adminJSON(cfg), 0
				}
			}
			if err := conn.CreateInBatches(rows, 100).Error; err != nil {
				t.Fatal(err)
			}
			before, after := FrozenJSONWindow{1000, 2000, 3000}, FrozenJSONWindow{4000, 5000, 6000}
			if err := conn.Transaction(func(tx *gorm.DB) error {
				return RestoreActivityCalendarPeriodWindow(tx, live.ID, "over-bound", before, after)
			}); err == nil {
				t.Fatal("ownership scan silently truncated at bound")
			}
			var got db.PeriodInstance
			var receipt db.RewardDelivery
			if err := conn.First(&got, "id=?", live.ID).Error; err != nil || !reflect.DeepEqual(got, live) {
				t.Fatalf("bound failure changed live: %+v %v", got, err)
			}
			if err := conn.First(&receipt, delivery.ID).Error; err != nil || !reflect.DeepEqual(receipt, delivery) {
				t.Fatalf("bound failure changed receipt: %+v %v", receipt, err)
			}
			if err := conn.Where("instance_id=? AND player_id=?", rows[0].InstanceID, rows[0].PlayerID).Delete(&db.PeriodProgress{}).Error; err != nil {
				t.Fatal(err)
			}
			// Exactly the bound remains valid without changing dates or history.
			if err := conn.Transaction(func(tx *gorm.DB) error {
				return RestoreActivityCalendarPeriodWindow(tx, live.ID, "at-bound", before, before)
			}); err != nil {
				t.Fatal("exact ownership bound rejected", err)
			}
		})
	}
}

func TestActivityCalendarRestorePeriodRewardSourceTierBound(t *testing.T) {
	for _, count := range []int{31, 32} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			conn, live, progress, _ := calendarPeriodOwnershipFixture(t)
			tiers := make([]p1PeriodMilestone, count)
			for i := range tiers {
				tiers[i] = p1PeriodMilestone{Goal: int64(i + 1), Rewards: "coin,1"}
			}
			live.Goal, live.Milestones, live.Frozen = 1, adminJSON(tiers), nil
			raw := adminJSON(live)
			live.Frozen, progress.Snapshot = raw, append([]byte(nil), raw...)
			if err := conn.Save(&live).Error; err != nil {
				t.Fatal(err)
			}
			if err := conn.Save(&progress).Error; err != nil {
				t.Fatal(err)
			}
			before := FrozenJSONWindow{1000, 2000, 3000}
			err := conn.Transaction(func(tx *gorm.DB) error {
				return RestoreActivityCalendarPeriodWindow(tx, live.ID, "tier-bound", before, before)
			})
			if count == 31 && err != nil {
				t.Fatal("original maximum tier count rejected", err)
			}
			if count == 32 && err == nil {
				t.Fatal("unbounded reward source list accepted")
			}
		})
	}
}
