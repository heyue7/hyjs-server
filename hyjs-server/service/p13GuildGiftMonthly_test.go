package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p13GiftAt(month string, day int) time.Time {
	at, _ := time.ParseInLocation("2006-01", month, time.FixedZone("Asia/Shanghai", 8*3600))
	return at.AddDate(0, 0, day-1).Add(12 * time.Hour)
}

// Gift tests own these seven new tables in p13GuildSetup's explicitly isolated
// database. The older shared fixture predates this domain and intentionally
// cannot reset its policy, archived heads or outbox between MySQL test cases.
func p13GuildGiftSetup(t *testing.T) (*p1Fixture, []int64) {
	t.Helper()
	f, ids := p13GuildSetup(t)
	owned := []any{&db.P13GuildGiftAward{}, &db.P13GuildGiftCutoff{}, &db.P13GuildGiftCredit{}, &db.P13GuildGiftHead{}, &db.P13GuildGiftArchive{}, &db.P13GuildGiftRule{}, &db.P13GuildGiftPolicy{}}
	clear := func(cleanup bool) {
		for _, table := range owned {
			if err := db.GetIns().Where("1=1").Delete(table).Error; err != nil {
				if cleanup {
					t.Error("gift isolated cleanup", err)
				} else {
					t.Fatal("gift isolated setup", err)
				}
			}
		}
		// Only this new domain's test delivery sources; no other P1 history.
		if err := db.GetIns().Where("source_key LIKE ?", "p13-guild-gift:%").Delete(&db.RewardDelivery{}).Error; err != nil {
			if cleanup {
				t.Error("gift delivery cleanup", err)
			} else {
				t.Fatal("gift delivery setup", err)
			}
		}
	}
	clear(false)
	t.Cleanup(func() { clear(true) })
	if err := f.s.InitializeP13GuildGiftMonthly(f.ctx); err != nil {
		t.Fatal(err)
	}
	return f, ids
}

func p13GiftContextDate(ctx context.Context, at time.Time) context.Context {
	loc := at.Location()
	real := time.Now().In(loc)
	realDay := time.Date(real.Year(), real.Month(), real.Day(), 0, 0, 0, 0, loc)
	targetDay := time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, loc)
	ctx = context.WithValue(ctx, constUtil.LocalPlayBoundKey, true)
	return context.WithValue(ctx, constUtil.LocalPlayOffsetDaysKey, int64(targetDay.Sub(realDay)/(24*time.Hour)))
}

func p13GiftAdditionalPlayer(t *testing.T, f *p1Fixture, i int) int64 {
	t.Helper()
	login, err := LoginAccount(f.ctx, fmt.Sprintf("gift-%s-%d-%d", t.Name(), i, time.Now().UnixNano()), "isolated-local-pass")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(f.ctx, login.Id, fmt.Sprintf("月礼成员%d", i), "hero_047"); err != nil {
		t.Fatal(err)
	}
	// Finite access/donation funding only. No active score, credit or gift
	// qualification is manufactured by this account preparation.
	if err = updateProfile(f.ctx, login.Id, func(p *pb.Player, g *model.GameState) error {
		p.Level, p.Diamond, p.Coin = proto.Int32(30), proto.Int64(1000), proto.Int64(20000000)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return login.Id
}

func p13GiftItemCount(t *testing.T, id int64, item string) int64 {
	t.Helper()
	var row db.Profile
	if err := db.GetIns().First(&row, id).Error; err != nil {
		t.Fatal(err)
	}
	_, game, err := decodeProfile(&row)
	if err != nil {
		t.Fatal(err)
	}
	var n int64
	for _, entry := range game.Items {
		if entry.GetId() == item {
			n += int64(entry.GetNum())
		}
	}
	return n
}

// This uses real guild creation/join/promotion/donation/upgrade and 750 actual
// login-domain transactions. Parent phone acceptance additionally runs the full
// original login protocol; this test does not claim that native flow.
func TestP13GuildGiftMonthlyReal25Members30DaysOriginalRolesP1Cold(t *testing.T) {
	f, ids := p13GuildGiftSetup(t)
	p13GuildOpen(t, true)
	joined := p13GiftAt("2026-10", 30)
	p13GuildCall(t, f, ids[0], &pb.CSUnionCreate{Name: proto.String("月礼验证")}, 1, joined)
	guild := p13GuildID(t, f, ids[0], joined)
	for len(ids) < 25 {
		ids = append(ids, p13GiftAdditionalPlayer(t, f, len(ids)))
	}
	for i, id := range ids[1:] {
		p13Join(t, f, ids[0], id, guild, int64(10+i*2), joined)
	}
	// Deliberate untrusted legacy-data fixture. It is preserved until activation,
	// archived permanently, and never contributes to verified award eligibility.
	if err := db.GetIns().Model(&db.P13Guild{}).Where("id=?", guild).Update("active_score", 12345).Error; err != nil {
		t.Fatal(err)
	}
	old := p13GuildCall(t, f, ids[0], &pb.CSGetUnionInfo{}, 0, p13GiftAt("2026-10", 31)).(*pb.SCGetUnionInfo)
	if old.GetActiveScore() != 12345 || p1Count(t, &db.P13GuildGiftHead{}, "guild_id=?", guild) != 0 {
		t.Fatal("legacy changed early")
	}
	for day := 1; day <= 30; day++ {
		for _, id := range ids {
			if err := f.s.p13GuildLoginAt(f.ctx, id, p13GiftAt("2026-11", day)); err != nil {
				t.Fatal(day, id, err)
			}
		}
	}
	last := p13GiftAt("2026-11", 30)
	for _, id := range ids {
		if err := f.s.p13GuildLoginAt(f.ctx, id, last); err != nil {
			t.Fatal(err)
		}
	}
	var head db.P13GuildGiftHead
	if err := db.GetIns().First(&head, "guild_id=?", guild).Error; err != nil {
		t.Fatal(err)
	}
	if head.Score != 7500 || head.CreditCount != 750 || p1Count(t, &db.P13GuildGiftCredit{}, "guild_id=?", guild) != 750 {
		t.Fatal("not actual750 daily credits", head)
	}
	var archive db.P13GuildGiftArchive
	if err := db.GetIns().First(&archive, "guild_id=?", guild).Error; err != nil {
		t.Fatal(err)
	}
	if archive.Score != 12345 || archive.GuildHash != p6Digest(archive.GuildRaw) {
		t.Fatal("legacy archive missing")
	}
	// Reach level2 through actual original100k coin donations and badge cost.
	// No guild asset or level UPDATE is used to manufacture the upgrade.
	for i, id := range ids {
		for j := 0; j < 4; j++ {
			p13GuildCall(t, f, id, &pb.CSUnionDonate{DonateType: proto.String("coin")}, int64(10000+i*4+j), last)
		}
	}
	for i := 0; i < 10; i++ {
		p13GuildCall(t, f, ids[0], &pb.CSUnionDonate{DonateType: proto.String("badge")}, int64(20000+i), last)
	}
	p13GuildCall(t, f, ids[0], &pb.CSUnionUpgrade{}, 30000, last)
	p13GuildCall(t, f, ids[0], &pb.CSPromotionViceChairman{PlayerId: proto.Int64(ids[1])}, 30001, last)
	info := p13GuildCall(t, f, ids[0], &pb.CSGetUnionInfo{}, 0, last).(*pb.SCGetUnionInfo)
	if info.GetUnionInfo().GetLevel() != 2 || info.GetActiveScore() != 7500 {
		t.Fatal("original level2/7500", info)
	}
	end := p13GiftAt("2026-12", 1).Add(-12 * time.Hour)
	if err := f.s.p13GuildLoginAt(f.ctx, ids[0], end.Add(-time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if p1Count(t, &db.P13GuildGiftCutoff{}, "guild_id=?", guild) != 0 {
		t.Fatal("last millisecond closed old month")
	}
	// The first post-boundary role mutation must freeze old identities first.
	p13GuildCall(t, f, ids[0], &pb.CSTransferChairman{PlayerId: proto.Int64(ids[2])}, 40000, end)
	var cut db.P13GuildGiftCutoff
	if err := db.GetIns().First(&cut, "guild_id=? AND month=?", guild, "2026-11").Error; err != nil {
		t.Fatal(err)
	}
	if !cut.Qualified || cut.Score != 7500 || cut.Level != 2 || cut.CutoffAt != end.UnixMilli() {
		t.Fatal("wrong certified cutoff", cut)
	}
	if p1Count(t, &db.P13GuildGiftAward{}, "guild_id=?", guild) != 25 || p1Count(t, &db.RewardDelivery{}, "source_key=?", "p13-guild-gift:2026-11:"+guild) != 0 {
		t.Fatal("cutoff did inline P1 work or missed recipients")
	}
	f.ctx = p13GiftContextDate(f.ctx, end)
	var damaged db.P13GuildGiftAward
	if err := db.GetIns().Order("id").First(&damaged, "guild_id=?", guild).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P13GuildGiftAward{}).Where("id=?", damaged.ID).Update("rewards", "item_072,999").Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.TickP13GuildGiftMonthly(f.ctx, LocalPlayNow(f.ctx)); err == nil {
		t.Fatal("damaged outbox became P1 delivery")
	}
	if p1Count(t, &db.RewardDelivery{}, "source_key=?", "p13-guild-gift:2026-11:"+guild) != 0 {
		t.Fatal("bad first outbox did not stop before delivery")
	}
	if err := db.GetIns().Save(&damaged).Error; err != nil {
		t.Fatal(err)
	}
	// Later live table edits and disabling new credits cannot edit old obligations.
	for _, r := range f.s.Tables["UnionConfig"] {
		r["CommonReward"], r["ViceReward"], r["ChairmanReward"] = "item_070,999", "item_071,999", "item_072,999"
	}
	if err := db.GetIns().Model(&db.P13GuildGiftPolicy{}).Where("id=1").Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	pool, _ := db.GetIns().DB()
	pool.Close()
	f.open(t)
	for i := 0; i < 2; i++ {
		if err := f.s.TickP13GuildGiftMonthly(f.ctx, LocalPlayNow(f.ctx)); err != nil {
			t.Fatal(err)
		}
	}
	if p1Count(t, &db.RewardDelivery{}, "source_key=?", "p13-guild-gift:2026-11:"+guild) != 25 {
		t.Fatal("cold exact once P1 outbox")
	}
	for i, id := range ids {
		item := "item_070"
		if i == 0 {
			item = "item_072"
		}
		if i == 1 {
			item = "item_071"
		}
		var award db.P13GuildGiftAward
		if err := db.GetIns().First(&award, "guild_id=? AND month=? AND player_id=?", guild, "2026-11", id).Error; err != nil {
			t.Fatal(err)
		}
		if err := P13GuildGiftVerifyAward(db.GetIns().WithContext(f.ctx), &award); err != nil {
			t.Fatal(err)
		}
		if award.Rewards != item+",1" || award.DeliveryID <= 0 {
			t.Fatal("cutoff role/table changed", award)
		}
		before := p13GiftItemCount(t, id, item)
		if _, err := f.s.RewardCenter(f.ctx, id, &pb.CSQueryUserReward{}); err != nil {
			t.Fatal(err)
		}
		for repeat := 0; repeat < 2; repeat++ {
			if _, err := f.s.RewardCenter(f.ctx, id, &pb.CSAddRewardsToPlayer{RewardIds: []int64{award.DeliveryID}}); err != nil {
				t.Fatal(err)
			}
		}
		if p13GiftItemCount(t, id, item)-before != 1 {
			t.Fatal("P1 original gift not exactly once", id, item)
		}
	}
}

func TestP13GuildGiftMonthlyLegacyDisabledRollbackAndCorruption(t *testing.T) {
	f, ids := p13GuildGiftSetup(t)
	p13GuildOpen(t, true)
	start := p13GiftAt("2026-11", 1)
	p13GuildCall(t, f, ids[0], &pb.CSUnionCreate{Name: proto.String("月礼边界")}, 1, start.AddDate(0, -1, 0))
	guild := p13GuildID(t, f, ids[0], start.AddDate(0, -1, 0))
	if err := db.GetIns().Model(&db.P13Guild{}).Where("id=?", guild).Update("active_score", 999999).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.p13GuildLoginAt(f.ctx, ids[0], start); err != nil {
		t.Fatal(err)
	}
	var head db.P13GuildGiftHead
	if err := db.GetIns().First(&head, "guild_id=?", guild).Error; err != nil {
		t.Fatal(err)
	}
	if head.Score != 10 || head.CreditCount != 1 {
		t.Fatal("legacy score certified", head)
	}
	if err := f.s.p13GuildLoginAt(f.ctx, ids[0], start.Add(-time.Hour)); err == nil {
		t.Fatal("same-month backward clock accepted")
	}
	if err := db.GetIns().Model(&db.P13GuildGiftPolicy{}).Where("id=1").Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.p13GuildLoginAt(f.ctx, ids[0], start.AddDate(0, 0, 1)); err != nil {
		t.Fatal(err)
	}
	if p1Count(t, &db.P13GuildGiftCredit{}, "guild_id=?", guild) != 1 {
		t.Fatal("disabled credit")
	}
	if err := db.GetIns().Model(&db.P13GuildGiftPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	// A failed enclosing real TX must not retain an inserted credit or head total.
	rollback := errors.New("explicit rollback probe")
	err := db.GetIns().WithContext(f.ctx).Transaction(func(tx *gorm.DB) error {
		if _, e := f.s.P13GuildGiftBeforeMutation(tx, guild, start.AddDate(0, 0, 2)); e != nil {
			return e
		}
		var g db.P13Guild
		var m db.P13GuildMember
		if e := tx.First(&g, "id=?", guild).Error; e != nil {
			return e
		}
		if e := tx.First(&m, "player_id=?", ids[0]).Error; e != nil {
			return e
		}
		if _, e := f.s.p13GuildGiftCredit(tx, &g, &m, start.AddDate(0, 0, 2)); e != nil {
			return e
		}
		return rollback
	})
	if !errors.Is(err, rollback) || p1Count(t, &db.P13GuildGiftCredit{}, "guild_id=?", guild) != 1 {
		t.Fatal("partial credit after rollback", err)
	}
	if err = db.GetIns().Model(&db.P13GuildGiftHead{}).Where("guild_id=?", guild).Update("score", 7000).Error; err != nil {
		t.Fatal(err)
	}
	if err = f.s.p13GuildLoginAt(f.ctx, ids[0], start.AddDate(0, 0, 2)); err == nil {
		t.Fatal("damaged head accepted")
	}
	if err = db.GetIns().Save(&head).Error; err != nil {
		t.Fatal(err)
	}
	// No invented intervening months and no retro gift after a long cold gap.
	if err = f.s.TickP13GuildGiftMonthly(f.ctx, p13GiftAt("2027-03", 1)); err != nil {
		t.Fatal(err)
	}
	if p1Count(t, &db.P13GuildGiftCutoff{}, "guild_id=?", guild) != 1 || p1Count(t, &db.P13GuildGiftAward{}, "guild_id=?", guild) != 0 {
		t.Fatal("manufactured missing months or old-score reward")
	}
	var current db.P13Guild
	if err = db.GetIns().First(&current, "id=?", guild).Error; err != nil {
		t.Fatal(err)
	}
	if current.ActiveScore != 0 || current.Exp != 10 {
		t.Fatal("monthly reset changed permanent exp", current)
	}
}

func TestP13GuildDisplayRankOriginalDirectoryOrderAndExactLookup(t *testing.T) {
	f, ids := p13GuildGiftSetup(t)
	p13GuildOpen(t, false)
	now := p13GiftAt("2026-10", 26)
	for i, id := range ids {
		p13GuildCall(t, f, id, &pb.CSUnionCreate{Name: proto.String(fmt.Sprintf("目录会%d", i))}, int64(i+1), now)
	}
	page := p13GuildCall(t, f, ids[0], &pb.CSUnionSearch{Name: proto.String(""), Page: proto.Int32(1)}, 0, now).(*pb.SCUnionSearch)
	if len(page.Unions) != 5 {
		t.Fatal(page)
	}
	for i, info := range page.Unions {
		if info.GetRank() != int32(i+1) {
			t.Fatal("directory ordinal", info)
		}
		exact := p13GuildCall(t, f, ids[0], &pb.CSUnionSearch{Name: info.UnionId, Page: proto.Int32(1)}, 0, now).(*pb.SCUnionSearch)
		if len(exact.Unions) != 1 || exact.Unions[0].GetRank() != info.GetRank() {
			t.Fatal("exact lookup renumbered result")
		}
	}
	for _, id := range ids {
		info := p13GuildCall(t, f, id, &pb.CSGetUnionInfo{}, 0, now).(*pb.SCGetUnionInfo).GetUnionInfo()
		for _, listed := range page.Unions {
			if listed.GetUnionId() == info.GetUnionId() && listed.GetRank() != info.GetRank() {
				t.Fatal("detail differs")
			}
		}
	}
	if !db.GetIns().Migrator().HasIndex(&db.P13Guild{}, "idx_p13_guild_order") {
		t.Fatal("metadata rank index missing")
	}
}

func TestP13GuildGiftMonthlyAdminCannotForgeActiveScore(t *testing.T) {
	f, ids := p13GuildGiftSetup(t)
	p13GuildOpen(t, true)
	actor := adminGuildTestActor(t, constUtil.AdminRoleOperator)
	if err := f.s.InitializeAdminGuildSettings(f.ctx); err != nil {
		t.Fatal(err)
	}
	now := p13GiftAt("2026-11", 2)
	real := time.Now().In(now.Location())
	now = time.Date(now.Year(), now.Month(), now.Day(), real.Hour(), real.Minute(), real.Second(), real.Nanosecond(), now.Location())
	p13GuildCall(t, f, ids[0], &pb.CSUnionCreate{Name: proto.String("月礼后台")}, 1, now)
	guild := p13GuildID(t, f, ids[0], now)
	if err := f.s.p13GuildLoginAt(f.ctx, ids[0], now); err != nil {
		t.Fatal(err)
	}
	// Trusted isolated logical calendar only; no score/award fixture injection.
	_, realStart, _ := p13GuildGiftMonth(time.Now())
	local := time.Now().In(realStart.Location())
	realDay := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
	targetDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	ctx := context.WithValue(f.ctx, constUtil.LocalPlayBoundKey, true)
	ctx = context.WithValue(ctx, constUtil.LocalPlayOffsetDaysKey, int64(targetDay.Sub(realDay)/(24*time.Hour)))
	ctx = context.WithValue(ctx, constUtil.AdminServerScopeContextKey, int64(1))
	detail, err := f.s.AdminGuildDetail(ctx, guild)
	if err != nil {
		t.Fatal(err)
	}
	req := model.AdminGuildUpdateRequest{Version: detail.Version, Reason: "非法认证分修改测试", Name: detail.Name, Level: detail.Level, Notice: detail.Notice, Declaration: detail.Declaration, Flag: detail.Flag, Coin: detail.Coin, Badge: detail.Badge, Exp: detail.Exp, DonateScore: detail.DonateScore, ActiveScore: 7000, HeroStatueOpen: detail.HeroStatueOpen, GuardTreeOpen: detail.GuardTreeOpen}
	if _, err = f.s.AdminUpdateGuild(ctx, actor, guild, req); err == nil || !errors.Is(err, constUtil.ErrAdminInput) || !strings.Contains(err.Error(), constUtil.P13GuildGiftAdminActive) {
		t.Fatal("admin forged certified score", err)
	}
	var current db.P13Guild
	if err = db.GetIns().First(&current, "id=?", guild).Error; err != nil {
		t.Fatal(err)
	}
	if current.ActiveScore != 10 {
		t.Fatal("admin refusal changed score")
	}
	req.ActiveScore = 10
	req.Notice = "正常公告修改"
	req.Reason = "保留真实认证分"
	if _, err = f.s.AdminUpdateGuild(ctx, actor, guild, req); err != nil {
		t.Fatal("unrelated admin edit blocked", err)
	}
}

func TestP13GuildGiftMonthlyPrestartRewindCannotReturnToLegacy(t *testing.T) {
	f, ids := p13GuildGiftSetup(t)
	p13GuildOpen(t, true)
	legacy := p13GiftAt("2026-10", 30)
	p13GuildCall(t, f, ids[0], &pb.CSUnionCreate{Name: proto.String("月礼回拨")}, 1, legacy)
	guild := p13GuildID(t, f, ids[0], legacy)
	if err := f.s.p13GuildLoginAt(f.ctx, ids[0], legacy); err != nil {
		t.Fatal("uninitialized legacy login rejected", err)
	}
	if err := f.s.TickP13GuildGiftMonthly(f.ctx, legacy); err != nil {
		t.Fatal("ordinary prestart worker should be no-op", err)
	}
	if p1Count(t, &db.P13GuildGiftHead{}, "guild_id=?", guild) != 0 {
		t.Fatal("legacy path created authenticated head")
	}
	if err := f.s.p13GuildLoginAt(f.ctx, ids[0], p13GiftAt("2026-11", 1)); err != nil {
		t.Fatal(err)
	}
	var before db.P13Guild
	var head db.P13GuildGiftHead
	var member db.P13GuildMember
	if err := db.GetIns().First(&before, "id=?", guild).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().First(&head, "guild_id=?", guild).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().First(&member, "player_id=?", ids[0]).Error; err != nil {
		t.Fatal(err)
	}
	rewound := p13GiftAt("2026-10", 31)
	if err := f.s.p13GuildLoginAt(f.ctx, ids[0], rewound); err == nil || err.Error() != constUtil.P13GuildGiftConflict {
		t.Fatal("certified head reverted to legacy login arithmetic", err)
	}
	err := db.GetIns().WithContext(f.ctx).Transaction(func(tx *gorm.DB) error {
		g, m := before, member
		_, err := f.s.p13GuildGiftCredit(tx, &g, &m, rewound)
		return err
	})
	if err == nil || err.Error() != constUtil.P13GuildGiftConflict {
		t.Fatal("direct credit bypassed prestart rewind guard", err)
	}
	if err := f.s.TickP13GuildGiftMonthly(f.ctx, rewound); err == nil || err.Error() != constUtil.P13GuildGiftConflict {
		t.Fatal("worker accepted prestart rewind with certified head", err)
	}
	var after db.P13Guild
	var afterHead db.P13GuildGiftHead
	var afterMember db.P13GuildMember
	if err := db.GetIns().First(&after, "id=?", guild).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().First(&afterHead, "guild_id=?", guild).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().First(&afterMember, "player_id=?", ids[0]).Error; err != nil {
		t.Fatal(err)
	}
	if p13GuildGiftDigest(before) != p13GuildGiftDigest(after) || head.SourceHash != afterHead.SourceHash || p13GuildGiftDigest(member) != p13GuildGiftDigest(afterMember) || p1Count(t, &db.P13GuildGiftCredit{}, "guild_id=?", guild) != 1 {
		t.Fatal("rewind refusal changed guild/head/login/credits")
	}
}
