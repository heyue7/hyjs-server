package service

import (
	"context"
	"testing"
	"time"

	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func p11LimitDailyFiniteSetup(t *testing.T, period string) (*p1Fixture, *time.Time, time.Time) {
	t.Helper()
	f, _, _ := p11ArenaSetup(t)
	if err := f.s.LoadP11LimitDailyTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP11LimitDaily(f.ctx); err != nil {
		t.Fatal(err)
	}
	local := time.Now().In(time.FixedZone("UTC+8", 8*3600))
	at := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location()).AddDate(0, 0, -2)
	now := at.Add(time.Hour)
	f.ctx = context.WithValue(f.ctx, constUtil.P11LimitDailyClockContextKey{}, func() time.Time { return now })
	p11OpenLimit(t, now, period)
	if err := db.GetIns().Model(&db.P11LimitPolicy{}).Where("id=1").Update("ends_at", at.Add(4*24*time.Hour).UnixMilli()).Error; err != nil {
		t.Fatal(err)
	}
	// All point sources below are real original entry calls. The clock and
	// prepublished operator dates are explicit finite SQLite preparation only.
	p11LimitCall(t, f, f.id, &pb.CSLimitPvpFindMatch{}, 19000, now)
	var count int64
	db.GetIns().Model(&db.P11LimitDailyOffer{}).Count(&count)
	if count != 0 {
		t.Fatal("defaultoff froze history", count)
	}
	if err := db.GetIns().Model(&db.P11LimitDailyPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "published_at": at.Add(-time.Hour).UnixMilli(), "start_at": at.UnixMilli(), "end_at": at.Add(2 * 24 * time.Hour).UnixMilli()}).Error; err != nil {
		t.Fatal(err)
	}
	p11LimitCall(t, f, f.id, &pb.CSLimitPvpFindMatch{}, 19001, now)
	return f, &now, at
}

func TestP11LimitDailyOriginalScoreRangesAndEntryOnlyNoAward(t *testing.T) {
	f, clock, at := p11LimitDailyFiniteSetup(t, "daily-real-entry-only")
	var offer db.P11LimitDailyOffer
	if err := db.GetIns().First(&offer, "period_id=?", "daily-real-entry-only").Error; err != nil {
		t.Fatal(err)
	}
	*clock = at.Add(24*time.Hour + time.Second)
	if err := f.s.TickP11LimitDaily(f.ctx, *clock); err != nil {
		t.Fatal(err)
	}
	var snap db.P11LimitDailySnapshot
	if err := db.GetIns().First(&snap, "id=?", offer.ID).Error; err != nil {
		t.Fatal(err)
	}
	if snap.Status != constUtil.P11LimitDailyComplete {
		t.Fatal("legitimate entry balance unresolved", snap)
	}
	var awards int64
	db.GetIns().Model(&db.P11LimitDailyAward{}).Count(&awards)
	if awards != 0 {
		t.Fatal("entry/rank-only qualified", awards)
	}
	var member db.P11LimitMember
	db.GetIns().First(&member, "period_id=? AND player_id=?", offer.PeriodID, f.id)
	if member.Score != 10 {
		t.Fatal("unplayed day gained rank points", member)
	}
	for rank, want := range map[int64]int64{1: 50, 2: 40, 3: 40, 4: 30, 10: 30, 11: 25, 50: 25, 51: 20, 100: 20, 101: 15, 500: 15, 501: 10, 9999: 10, 10000: 0} {
		if got := p11LimitDailyBonus(rank); got != want {
			t.Fatal("original seven ranges", rank, got, want)
		}
	}
	if err := f.s.TickP11LimitDaily(f.ctx, *clock); err != nil {
		t.Fatal(err)
	}
	var count int64
	db.GetIns().Model(&db.P11LimitDailySnapshot{}).Where("id=?", offer.ID).Count(&count)
	if count != 1 {
		t.Fatal("same cutoff duplicated", count)
	}
}

func TestP11LimitDailyUnverifiedBalanceRemainsPending(t *testing.T) {
	f, clock, at := p11LimitDailyFiniteSetup(t, "daily-reject-unverified-balance")
	// A negative corruption fixture never becomes a winner or score source.
	db.GetIns().Model(&db.P11LimitMember{}).Where("period_id=? AND player_id=?", "daily-reject-unverified-balance", f.id).Update("score", 999)
	*clock = at.Add(24*time.Hour + time.Second)
	if err := f.s.TickP11LimitDaily(f.ctx, *clock); err != nil {
		t.Fatal(err)
	}
	var snap db.P11LimitDailySnapshot
	db.GetIns().First(&snap, "period_id=?", "daily-reject-unverified-balance")
	if snap.Status != constUtil.P11LimitDailyPending || snap.Reason != "period-score-source-unverified" {
		t.Fatal("unverified points became reward", snap)
	}
	var count int64
	db.GetIns().Model(&db.P11LimitDailyAward{}).Where("period_id=?", snap.PeriodID).Count(&count)
	if count != 0 {
		t.Fatal("pending partially awarded", count)
	}
}
