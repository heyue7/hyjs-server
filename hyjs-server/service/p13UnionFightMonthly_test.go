package service

import (
	"encoding/json"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// This cheap boundary test uses actual original guild creation/join/transfer/quit.
// Month clock and existing membership funding are explicit finite prerequisites;
// missing genuine wars remain pending and never manufacture a rank or title.
func TestP13MonthlyPublishedBoundaryAndNoRetro(t *testing.T) {
	f, ids := p13GuildSetup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP13UnionFightMonthly(f.ctx); err != nil {
		t.Fatal(err)
	}
	if n := p1Count(t, &db.P13UnionFightMonthlyPolicy{}, "1=1"); n != 0 {
		t.Fatal("monthly initialized enabled publication")
	}
	loc := time.FixedZone("Asia/Shanghai", 8*3600)
	publish := time.Date(2026, 10, 31, 12, 0, 0, 0, loc)
	start, cutoff, err := p13UFMonth("2026-11")
	if err != nil {
		t.Fatal(err)
	}
	rule := db.P13UnionFightMonthlyPolicy{Month: "2026-11", Enabled: true, Version: "isolated-local-future-month-v1", Source: "explicit local8/4 badge policy; original private monthly payouts unknown", PublishedAt: publish.UnixMilli() - 1, Rewards: `[{"rank":1,"rewards":"item_311,8"},{"rank":2,"rewards":"item_311,4"}]`, MemberTitle: true, ChairmanTitle: true}
	if err = db.GetIns().Create(&rule).Error; err != nil {
		t.Fatal(err)
	}
	if err = f.s.TickP13UnionFightMonthly(f.ctx, publish); err != nil {
		t.Fatal(err)
	}
	var offer db.P13UnionFightMonthlyOffer
	if err = db.GetIns().First(&offer, "month=?", rule.Month).Error; err != nil || offer.StartAt != start.UnixMilli() {
		t.Fatal("missing pre-month offer", err)
	}
	if err = db.GetIns().Model(&rule).Updates(map[string]any{"enabled": false, "rewards": `[{"rank":1,"rewards":"item_311,999"}]`}).Error; err != nil {
		t.Fatal(err)
	}
	p13GuildOpen(t, false)
	joined := start.AddDate(0, 0, 1)
	p13GuildCall(t, f, ids[0], &pb.CSUnionCreate{Name: proto.String("实际月末身份")}, 1, joined)
	guild := p13GuildID(t, f, ids[0], joined)
	p13Join(t, f, ids[0], ids[1], guild, 2, joined)
	if err = f.s.TickP13UnionFightMonthly(f.ctx, cutoff.Add(-time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if p1Count(t, &db.P13UnionFightMonthlyCutoff{}, "month=?", rule.Month) != 0 {
		t.Fatal("early cutoff identity")
	}
	// Central original mutation hook seals the old chairman before actual transfer.
	p13GuildCall(t, f, ids[0], &pb.CSTransferChairman{PlayerId: proto.Int64(ids[1])}, 3, cutoff)
	p13GuildCall(t, f, ids[0], &pb.CSQuitUnion{}, 4, cutoff.Add(time.Second))
	var saved db.P13UnionFightMonthlyCutoff
	if err = db.GetIns().First(&saved, "month=?", rule.Month).Error; err != nil {
		t.Fatal("missing mandatory pre-mutation cutoff", err)
	}
	if saved.SnapshotHash != p13UFMonthlyCutoffHash(&saved) || saved.CutoffAt != cutoff.UnixMilli() {
		t.Fatal("bad boundary snapshot")
	}
	var members []p13UFMonthlyIdentity
	if err = json.Unmarshal(saved.Members, &members); err != nil {
		t.Fatal(err)
	}
	got := map[int64]p13UFMonthlyIdentity{}
	for _, m := range members {
		got[m.PlayerID] = m
	}
	if got[ids[0]].GuildID != guild || got[ids[0]].Role != 2 || got[ids[1]].Role != 0 {
		t.Fatal("cutoff membership drifted to current identity", members)
	}
	for i := 0; i < 8; i++ {
		if err = f.s.TickP13UnionFightMonthly(f.ctx, cutoff.Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	if p1Count(t, &db.P13UnionFightMonthlyResult{}, "month=?", rule.Month) != 0 || p1Count(t, &db.P13UnionFightMonthlyTitle{}, "month=?", rule.Month) != 0 || p1Count(t, &db.RewardDelivery{}, "source_key=?", "p13-month:"+rule.Month) != 0 {
		t.Fatal("unplayed month manufactured ranking/reward/title")
	}
	var after db.P13UnionFightMonthlyOffer
	if err = db.GetIns().First(&after, "month=?", rule.Month).Error; err != nil || after.RulesHash != offer.RulesHash {
		t.Fatal("live operator change edited frozen month", err)
	}
	late := rule
	late.Month = "2026-10"
	late.Enabled = true
	late.PublishedAt = publish.AddDate(0, -1, 0).UnixMilli()
	if err = db.GetIns().Create(&late).Error; err != nil {
		t.Fatal(err)
	}
	if err = f.s.TickP13UnionFightMonthly(f.ctx, cutoff); err != nil {
		t.Fatal(err)
	}
	if p1Count(t, &db.P13UnionFightMonthlyOffer{}, "month=?", late.Month) != 0 {
		t.Fatal("late historical policy backfilled offer")
	}
	pool, _ := db.GetIns().DB()
	pool.Close()
	f.open(t)
	if p1Count(t, &db.P13UnionFightMonthlyCutoff{}, "month=?", rule.Month) != 1 {
		t.Fatal("cold cutoff missing")
	}
}

// An unexplained historical prestige row is a negative corruption fixture. It
// must never disappear from reconciliation merely because no match is archived.
func TestP13MonthlyUnverifiedWeekLedgerNeverRanks(t *testing.T) {
	f, _ := p13GuildSetup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP13UnionFightMonthly(f.ctx); err != nil {
		t.Fatal(err)
	}
	start, end, err := p13UFMonth("2026-11")
	if err != nil {
		t.Fatal(err)
	}
	pub := start.Add(-time.Hour)
	policy := db.P13UnionFightMonthlyPolicy{Month: "2026-11", Enabled: true, Version: "negative-unverified-ledger-only", Source: "negative fault fixture; never a real source", PublishedAt: pub.UnixMilli() - 1, Rewards: "[]"}
	if err = db.GetIns().Create(&policy).Error; err != nil {
		t.Fatal(err)
	}
	if err = f.s.TickP13UnionFightMonthly(f.ctx, pub); err != nil {
		t.Fatal(err)
	}
	var offer db.P13UnionFightMonthlyOffer
	if err = db.GetIns().First(&offer, "month=?", policy.Month).Error; err != nil {
		t.Fatal(err)
	}
	// No real match or winner is fabricated; only the unverified ledger fault exists.
	fault := db.P13UnionFightSettlementPrestige{WeekStart: p13UFWeek(end.Add(-time.Millisecond)), GuildID: "unverified-orphan", Score: 123}
	if err = db.GetIns().Create(&fault).Error; err != nil {
		t.Fatal(err)
	}
	if ranks, _, err := p13UFMonthlyRanks(db.GetIns(), &offer); err == nil || len(ranks) > 0 {
		t.Fatal("orphan week ledger was silently omitted", ranks, err)
	}
	if p1Count(t, &db.P13UnionFightMonthlyResult{}, "month=?", policy.Month) != 0 || p1Count(t, &db.P13UnionFightMonthlyTitle{}, "month=?", policy.Month) != 0 {
		t.Fatal("unverified source created entitlement")
	}
}
