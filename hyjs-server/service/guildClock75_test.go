package service

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func guildClock75Move(t *testing.T, f *p1Fixture, actor *model.AdminPrincipal, id, delta int64) context.Context {
	t.Helper()
	state, err := f.s.AdminServerGameClock(f.ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.AdminChangeServerGameClock(f.ctx, actor, model.ServerGameClockChangeRequest{DeltaDays: delta, Revision: state.Revision, RequestID: uuid.NewString()}); err != nil {
		t.Fatal(err)
	}
	var profile db.Profile
	if err = db.GetIns().First(&profile, id).Error; err != nil {
		t.Fatal(err)
	}
	ctx, err := f.s.BindServerGameClockContext(f.ctx, &profile)
	if err != nil {
		t.Fatal(err)
	}
	return ctx
}

func TestGuildClock75PrayerFuturePublicationRewindMembershipAndCold(t *testing.T) {
	f, ids, _ := p13PrayerSetup(t)
	if err := f.s.InitializeServerGameClock(f.ctx); err != nil {
		t.Fatal(err)
	}
	actor := adminWalletTestActor(t, f, constUtil.AdminRoleAdmin)
	id := ids[0]
	ctx := guildClock75Move(t, f, actor, id, 6)
	forward := LocalPlayNow(ctx)
	p13PrayerPublish(t, forward.Add(-time.Second))
	// The real member joined on the future test date. Rewinding may not turn a
	// new paid prayer into an empty effect, or revive it after a different term.
	joined := forward.Add(-time.Second).UnixMilli()
	if err := db.GetIns().Model(&db.P13GuildMember{}).Where("player_id=?", id).Update("joined_at", joined).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p13GuildPrayer(ctx, id, &pb.CSUnionPray{}, time.Now().UnixMilli(), forward); err != nil {
		t.Fatal(err)
	}
	var first db.P13GuildPrayerRoll
	if err := db.GetIns().First(&first, "player_id=?", id).Error; err != nil {
		t.Fatal(err)
	}
	var head db.P13GuildGiftHead
	var member db.P13GuildMember
	if err := db.GetIns().First(&member, "player_id=?", id).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().First(&head, "guild_id=?", member.GuildID).Error; err != nil {
		t.Fatal(err)
	}
	before, _ := p10LuckyProfile(t, id)
	ctx = guildClock75Move(t, f, actor, id, -15)
	back := LocalPlayNow(ctx)
	prop, hash, _, err := f.s.P13GuildPrayerProperties(ctx, db.GetIns(), id, back)
	if err != nil || hash != "" || !proto.Equal(prop, &pb.BaseProp{}) {
		t.Fatal("future prayer was borrowed or blocked the old date", prop, hash, err)
	}
	stamp := time.Now().UnixMilli() + 100
	if _, err = f.s.p13GuildPrayer(ctx, id, &pb.CSUnionPray{}, stamp, back); err != nil {
		t.Fatal(err)
	}
	prop, hash, _, err = f.s.P13GuildPrayerProperties(ctx, db.GetIns(), id, back)
	if err != nil || hash == "" || proto.Equal(prop, &pb.BaseProp{}) {
		t.Fatal("same term paid prayer lost its effect after rewind", prop, hash, err)
	}
	if _, err = f.s.p13GuildPrayer(ctx, id, &pb.CSUnionPray{}, stamp, back); err != nil {
		t.Fatal(err)
	}
	after, _ := p10LuckyProfile(t, id)
	if before.GetContribution()-after.GetContribution() != 100 {
		t.Fatal("prayer retry charged twice")
	}
	if err = f.s.TickP13GuildGiftMonthly(f.ctx, time.Now()); err != nil {
		t.Fatal("background monthly ledger blocked after trusted rewind", err)
	}
	var preserved db.P13GuildGiftHead
	if err = db.GetIns().First(&preserved, "guild_id=?", member.GuildID).Error; err != nil || preserved.SourceHash != head.SourceHash {
		t.Fatal("rewind rewrote forward-only monthly ledger", err)
	}
	calendarQuota75Cold(t)
	var unchanged db.P13GuildPrayerRoll
	if err = db.GetIns().First(&unchanged, "player_id=? AND stamp=?", id, first.Stamp).Error; err != nil || unchanged.SourceHash != first.SourceHash {
		t.Fatal("future immutable source changed", err)
	}
	prop, hash, _, err = f.s.P13GuildPrayerProperties(ctx, db.GetIns(), id, back)
	if err != nil || hash == "" || proto.Equal(prop, &pb.BaseProp{}) {
		t.Fatal("cold prayer projection", err)
	}
	if err = db.GetIns().Model(&db.P13GuildMember{}).Where("player_id=?", id).Update("joined_at", joined+1).Error; err != nil {
		t.Fatal(err)
	}
	prop, hash, _, err = f.s.P13GuildPrayerProperties(ctx, db.GetIns(), id, back)
	if err != nil || hash != "" || !proto.Equal(prop, &pb.BaseProp{}) {
		t.Fatal("different membership term reactivated prayer", prop, hash, err)
	}
	if err = db.GetIns().Model(&db.P13GuildPrayerPolicy{}).Where("id=1").Update("published_at", time.Now().AddDate(0, 0, 7).UnixMilli()).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p13GuildPrayer(ctx, id, &pb.CSUnionPray{}, stamp+1, back); err == nil {
		t.Fatal("policy beyond visited clock high-water accepted")
	}
}

func TestGuildClock75TechRetainsIrreversibleUpgradeOnRewind(t *testing.T) {
	f, ids, _, guild := p13TechSetup(t)
	if err := f.s.InitializeServerGameClock(f.ctx); err != nil {
		t.Fatal(err)
	}
	actor := adminWalletTestActor(t, f, constUtil.AdminRoleAdmin)
	id := ids[0]
	ctx := guildClock75Move(t, f, actor, id, 6)
	forward := LocalPlayNow(ctx)
	p13TechPublish(t, forward.Add(-time.Second))
	request := &pb.CSUnionHeroStatueUpgrade{Type: proto.String("peratt")}
	if _, err := f.s.p13GuildTechUpgrade(ctx, id, request, "peratt", constUtil.P13GuildTechHeroStatue, time.Now().UnixMilli(), forward); err != nil {
		t.Fatal(err)
	}
	var first db.P13GuildTechUpgrade
	if err := db.GetIns().First(&first, "guild_id=?", guild).Error; err != nil {
		t.Fatal(err)
	}
	ctx = guildClock75Move(t, f, actor, id, -15)
	back := LocalPlayNow(ctx)
	if _, err := f.s.p13GuildTechQuery(ctx, id, constUtil.P13GuildTechHeroStatue, back); err != nil {
		t.Fatal("rewind lost paid technology", err)
	}
	stamp := time.Now().UnixMilli() + 100
	if _, err := f.s.p13GuildTechUpgrade(ctx, id, request, "peratt", constUtil.P13GuildTechHeroStatue, stamp, back); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p13GuildTechUpgrade(ctx, id, request, "peratt", constUtil.P13GuildTechHeroStatue, stamp, back); err != nil {
		t.Fatal(err)
	}
	calendarQuota75Cold(t)
	var rows []db.P13GuildTechUpgrade
	if err := db.GetIns().Where("guild_id=?", guild).Order("level").Find(&rows).Error; err != nil || len(rows) != 2 || rows[0].SourceHash != first.SourceHash || rows[1].AppliedAt < rows[0].AppliedAt {
		t.Fatal("immutable paid history or retry changed", len(rows), err)
	}
	if _, _, err := f.s.P13GuildTechProperties(ctx, db.GetIns(), id, back); err != nil {
		t.Fatal("cold irreversible technology projection", err)
	}
}

func TestGuildClock75PublicationKeepsVisitedPositiveAndNegativeDates(t *testing.T) {
	f, _, _ := p13GuildDonateBoxSetup(t)
	now := time.Now()
	forward := now.AddDate(0, 0, 6)
	p13GuildDonateBoxEnable(t, forward)
	ctx := context.WithValue(f.ctx, constUtil.ServerGameClockBoundKey, true)
	ctx = context.WithValue(ctx, constUtil.ServerGameClockRevisionKey, int64(1))
	ctx = context.WithValue(ctx, constUtil.ServerGameClockHighDaysKey, int64(6))
	ctx = context.WithValue(ctx, constUtil.ServerGameClockOffsetKey, int64(6))
	if _, err := p13GuildDonateBoxPolicy(db.GetIns().WithContext(ctx), forward); err != nil {
		t.Fatal("current +6 publication rejected", err)
	}
	ctx = context.WithValue(ctx, constUtil.ServerGameClockOffsetKey, int64(-9))
	back := now.AddDate(0, 0, -9)
	policy, err := p13GuildDonateBoxPolicy(db.GetIns().WithContext(ctx), back)
	if err != nil || policy == nil || policy.PublishedAt > back.UnixMilli() {
		t.Fatal("visited positive publication blocked negative day", err)
	}
	var saved db.P13GuildDonateBoxPolicy
	if err = db.GetIns().First(&saved, 1).Error; err != nil || saved.PublishedAt <= now.UnixMilli() {
		t.Fatal("read rewrote original publication", err)
	}
	if err = db.GetIns().Model(&db.P13GuildDonateBoxPolicy{}).Where("id=1").Update("published_at", now.AddDate(0, 0, 7).UnixMilli()).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = p13GuildDonateBoxPolicy(db.GetIns().WithContext(ctx), back); err == nil {
		t.Fatal("source beyond authenticated high-water accepted")
	}
}
