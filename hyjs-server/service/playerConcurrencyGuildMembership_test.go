package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func TestPlayerConcurrencyMySQLGuildMembershipIsolation(t *testing.T) {
	name := os.Getenv("HYJS_P13_MYSQL_DATABASE")
	if name == "" {
		t.Skip("requires a coordinated isolated hyjs_test_p13_ MySQL database")
	}
	if !strings.HasPrefix(name, "hyjs_test_p13_") || os.Getenv("HYJS_GAME_ACCOUNTS_DATABASE") != "" {
		t.Fatal("requires an isolated guild and accounts fixture")
	}

	f, ids := p13GuildSetup(t)
	ids = append(ids, p13GiftAdditionalPlayer(t, f, 99))
	database := db.GetIns()
	if database.Dialector.Name() != "mysql" {
		t.Fatal("requires MySQL for guild locking")
	}
	pool, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(8)
	pool.SetMaxIdleConns(8)
	var giftPolicy db.P13GuildGiftPolicy
	if err := database.First(&giftPolicy, 1).Error; err != nil || giftPolicy.StartsAt <= 0 {
		t.Fatalf("read monthly gift start: %v", err)
	}
	// All guild membership and requests use the same preactivation time.
	now := time.UnixMilli(giftPolicy.StartsAt).Add(-12 * time.Hour)
	p13GuildOpen(t, true)
	p13GuildCall(t, f, ids[0], &pb.CSUnionCreate{Name: proto.String("成员隔离甲")}, 101, now)
	guildA := p13GuildID(t, f, ids[0], now)
	p13GuildCall(t, f, ids[1], &pb.CSUnionCreate{Name: proto.String("成员隔离乙")}, 102, now)
	guildB := p13GuildID(t, f, ids[1], now)
	if guildA == "" || guildB == "" || guildA == guildB {
		t.Fatal("fixture must create two distinct guilds")
	}
	p13Join(t, f, ids[0], ids[2], guildA, 103, now)
	p13Join(t, f, ids[1], ids[3], guildB, 104, now)
	p13GuildCall(t, f, ids[4], &pb.CSUnionApply{UnionId: proto.String(guildB)}, 105, now)

	ctx, cancel := context.WithTimeout(f.ctx, 25*time.Second)
	type result struct {
		messages []proto.Message
		err      error
	}
	type markerKey struct{}
	type marker struct{ index int }
	noticeMarker := &marker{index: -1}
	raceMarkers := [2]*marker{{index: 0}, {index: 1}}
	noticeCtx := context.WithValue(ctx, markerKey{}, noticeMarker)
	var held *gorm.DB
	var locked, beforeRegistered, afterRegistered bool
	var otherPending, personalPending, noticePending <-chan result
	var racePending [2]<-chan result
	const beforeName = "test:player-concurrency-guild-membership-before"
	const afterName = "test:player-concurrency-guild-membership-after"
	noticeAtLock := make(chan struct{})
	raceEntered := make(chan int, 2)
	raceRelease := make(chan struct{})
	var noticeOnce, releaseOnce sync.Once
	var raceOnce [2]sync.Once
	releaseRace := func() { releaseOnce.Do(func() { close(raceRelease) }) }
	defer func() {
		if locked {
			if err := held.Rollback().Error; err != nil && !errors.Is(err, sql.ErrTxDone) {
				t.Errorf("release held guild: %v", err)
			}
		}
		releaseRace()
		cancel()
		for _, pending := range []<-chan result{otherPending, personalPending, noticePending, racePending[0], racePending[1]} {
			if pending != nil {
				<-pending
			}
		}
		if afterRegistered {
			if err := database.Callback().Query().Remove(afterName); err != nil {
				t.Errorf("remove guild barrier callback: %v", err)
			}
		}
		if beforeRegistered {
			if err := database.Callback().Query().Remove(beforeName); err != nil {
				t.Errorf("remove guild query callback: %v", err)
			}
		}
	}()
	if err := database.Callback().Query().Before("gorm:query").Register(beforeName, func(tx *gorm.DB) {
		locking, ok := tx.Statement.Clauses["FOR"].Expression.(clause.Locking)
		if ok && locking.Strength == "UPDATE" && tx.Statement.Table == "p13_guilds" && len(tx.Statement.Selects) == 1 && tx.Statement.Selects[0] == "id" && tx.Statement.Context.Value(markerKey{}) == noticeMarker {
			noticeOnce.Do(func() { close(noticeAtLock) })
		}
	}); err != nil {
		t.Fatalf("register guild query callback: %v", err)
	}
	beforeRegistered = true
	if err := database.Callback().Query().After("gorm:query").Register(afterName, func(tx *gorm.DB) {
		locking, ok := tx.Statement.Clauses["FOR"].Expression.(clause.Locking)
		which, marked := tx.Statement.Context.Value(markerKey{}).(*marker)
		if !ok || !marked || which.index < 0 || which.index > 1 || which != raceMarkers[which.index] || tx.Error != nil || tx.RowsAffected != 1 || locking.Strength != "UPDATE" || tx.Statement.Table != "p13_guilds" || len(tx.Statement.Selects) != 1 || tx.Statement.Selects[0] != "id" {
			return
		}
		raceOnce[which.index].Do(func() {
			raceEntered <- which.index
			select {
			case <-raceRelease:
			case <-tx.Statement.Context.Done():
				tx.AddError(tx.Statement.Context.Err())
			}
		})
	}); err != nil {
		t.Fatalf("register guild barrier callback: %v", err)
	}
	afterRegistered = true

	held = database.WithContext(ctx).Begin()
	if held.Error != nil {
		t.Fatalf("begin held guild transaction: %v", held.Error)
	}
	locked = true
	if err := ServerGameClockGuardTransaction(held); err != nil {
		t.Fatal(err)
	}
	var policy db.P13GuildPolicy
	if err := held.Clauses(clause.Locking{Strength: "SHARE"}).First(&policy, 1).Error; err != nil {
		t.Fatalf("hold shared guild policy: %v", err)
	}
	var guild db.P13Guild
	if err := held.Select("id").Clauses(clause.Locking{Strength: "UPDATE"}).First(&guild, "id=?", guildA).Error; err != nil {
		t.Fatalf("hold guild A: %v", err)
	}
	start := func(callCtx context.Context, actor int64, request proto.Message, stamp int64, economy bool) <-chan result {
		out := make(chan result, 1)
		go func() {
			var messages []proto.Message
			var err error
			if economy {
				messages, err = f.s.p13GuildEconomy(callCtx, actor, request, stamp, now)
			} else {
				messages, err = f.s.p13GuildManage(callCtx, actor, request, stamp, now)
			}
			out <- result{messages: messages, err: err}
		}()
		return out
	}
	wait := func(pending <-chan result, label string, timeout time.Duration) result {
		t.Helper()
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		select {
		case r := <-pending:
			return r
		case <-timer.C:
			t.Fatalf("%s waited for the held guild", label)
		case <-ctx.Done():
			t.Fatalf("%s timed out: %v", label, ctx.Err())
		}
		return result{}
	}

	otherPending = start(ctx, ids[1], &pb.CSUnionAgreeApply{PlayerId: proto.Int64(ids[4])}, 201, false)
	other := wait(otherPending, "other guild approval", 2*time.Second)
	otherPending = nil
	if other.err != nil || len(other.messages) == 0 {
		t.Fatalf("other guild approval: %v", other.err)
	}
	personalPending = start(ctx, ids[3], &pb.CSGetUnionStamina{}, 202, true)
	personal := wait(personalPending, "other guild personal stamina", 2*time.Second)
	personalPending = nil
	if personal.err != nil || len(personal.messages) == 0 {
		t.Fatalf("other guild personal stamina: %v", personal.err)
	}

	notice := &pb.CSUnionNoticeChange{Content: proto.String("同会等待后生效")}
	noticePending = start(noticeCtx, ids[0], notice, 203, false)
	select {
	case <-noticeAtLock:
	case r := <-noticePending:
		noticePending = nil
		t.Fatalf("same-guild management ended before its lock: %v", r.err)
	case <-ctx.Done():
		t.Fatalf("same-guild management did not reach its lock: %v", ctx.Err())
	}
	timer := time.NewTimer(100 * time.Millisecond)
	defer timer.Stop()
	select {
	case r := <-noticePending:
		noticePending = nil
		t.Fatalf("same-guild management passed the held guild: %v", r.err)
	case <-timer.C:
	case <-ctx.Done():
		t.Fatalf("same-guild management wait timed out: %v", ctx.Err())
	}
	if err := held.Commit().Error; err != nil {
		t.Fatalf("release guild A: %v", err)
	}
	locked = false
	changed := wait(noticePending, "same-guild management after release", 5*time.Second)
	noticePending = nil
	if changed.err != nil || len(changed.messages) == 0 {
		t.Fatalf("same-guild management after release: %v", changed.err)
	}
	if err := database.WithContext(ctx).First(&guild, "id=?", guildA).Error; err != nil || guild.Notice != notice.GetContent() {
		t.Fatalf("same-guild management did not persist: %v", err)
	}

	for i, id := range []string{guildA, guildB} {
		if _, err := f.s.p13GuildApply(ctx, ids[5], &pb.CSUnionApply{UnionId: proto.String(id)}, int64(301+i), now); err != nil {
			t.Fatalf("prepare contested application %d: %v", i, err)
		}
	}
	var applicantBefore db.Profile
	if err := database.WithContext(ctx).First(&applicantBefore, ids[5]).Error; err != nil {
		t.Fatal(err)
	}
	beforePlayer, _, err := decodeProfile(&applicantBefore)
	if err != nil {
		t.Fatal(err)
	}
	request := &pb.CSUnionAgreeApply{PlayerId: proto.Int64(ids[5])}
	const approvalStamp int64 = 303
	for i := range racePending {
		raceCtx := context.WithValue(ctx, markerKey{}, raceMarkers[i])
		racePending[i] = start(raceCtx, ids[i], request, approvalStamp, false)
	}
	seen := [2]bool{}
	for i := 0; i < 2; i++ {
		select {
		case index := <-raceEntered:
			if seen[index] {
				t.Fatal("one approval entered the barrier twice")
			}
			seen[index] = true
		case r := <-racePending[0]:
			racePending[0] = nil
			t.Fatalf("guild A approval ended before both held their guilds: %v", r.err)
		case r := <-racePending[1]:
			racePending[1] = nil
			t.Fatalf("guild B approval ended before both held their guilds: %v", r.err)
		case <-ctx.Done():
			t.Fatalf("both approvals did not reach their guild locks: %v", ctx.Err())
		}
	}
	releaseRace()
	var outcomes [2]result
	for i := range racePending {
		outcomes[i] = wait(racePending[i], "contested approval", 5*time.Second)
		racePending[i] = nil
	}
	winner := -1
	for i, r := range outcomes {
		if r.err == nil && len(r.messages) > 0 {
			if winner >= 0 {
				t.Fatal("both guilds approved the same applicant")
			}
			winner = i
			continue
		}
		if !errors.Is(r.err, gorm.ErrRecordNotFound) && (r.err == nil || r.err.Error() != "角色已有公会") {
			t.Fatalf("approval %d did not return a membership rejection: %v", i, r.err)
		}
	}
	if winner < 0 {
		t.Fatal("neither guild approved the applicant")
	}
	expectedGuild := []string{guildA, guildB}[winner]
	var member db.P13GuildMember
	if err := database.WithContext(ctx).First(&member, "player_id=?", ids[5]).Error; err != nil || member.GuildID != expectedGuild {
		t.Fatalf("applicant membership does not match winning guild: %v", err)
	}
	var count int64
	if err := database.WithContext(ctx).Model(&db.P13GuildMember{}).Where("player_id=? AND guild_id<>?", ids[5], "").Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("applicant membership count: %d, %v", count, err)
	}
	if err := database.WithContext(ctx).Model(&db.P13GuildApply{}).Where("player_id=?", ids[5]).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("applicant still has pending applications: %d, %v", count, err)
	}
	var profile db.Profile
	if err := database.WithContext(ctx).First(&profile, ids[5]).Error; err != nil {
		t.Fatal(err)
	}
	player, _, err := decodeProfile(&profile)
	if err != nil || player.GetPlayerId() != ids[5] || player.GetCoin() != beforePlayer.GetCoin() || player.GetDiamond() != beforePlayer.GetDiamond() {
		t.Fatalf("applicant profile identity or assets changed: %v", err)
	}
	view, err := f.s.p13GuildQuery(ctx, ids[5], &pb.CSGetUnionInfo{}, now)
	if err != nil || len(view) == 0 {
		t.Fatalf("applicant guild projection: %v", err)
	}
	info, ok := view[0].(*pb.SCGetUnionInfo)
	if !ok || info.GetUnionInfo().GetUnionId() != expectedGuild {
		t.Fatal("applicant guild projection differs from membership")
	}
	assertReceipt := func() {
		t.Helper()
		var count int64
		if err := database.WithContext(ctx).Model(&db.P13GuildReceipt{}).Where("player_id=? AND operation=? AND stamp=?", ids[winner], "CSUnionAgreeApply", approvalStamp).Count(&count).Error; err != nil || count != 1 {
			t.Fatalf("winning approval receipt count: %d, %v", count, err)
		}
	}
	replay, err := f.s.p13GuildManage(ctx, ids[winner], request, approvalStamp, now)
	if err != nil || len(replay) == 0 || !proto.Equal(replay[0], outcomes[winner].messages[0]) {
		t.Fatalf("winning approval replay changed: %v", err)
	}
	assertReceipt()
	var replayMember db.P13GuildMember
	if err := database.WithContext(ctx).First(&replayMember, "player_id=?", ids[5]).Error; err != nil || replayMember != member {
		t.Fatalf("winning approval replay changed membership: %v", err)
	}
	if _, err := f.s.p13GuildManage(ctx, ids[winner], &pb.CSDissolveUnion{}, 304, now); err != nil {
		t.Fatalf("dissolve winning guild: %v", err)
	}
	replay, err = f.s.p13GuildManage(ctx, ids[winner], request, approvalStamp, now)
	if err != nil || len(replay) == 0 || !proto.Equal(replay[0], outcomes[winner].messages[0]) {
		t.Fatalf("winning approval replay after dissolution changed: %v", err)
	}
	assertReceipt()
	if scope, _, err := P13GuildScope(database.WithContext(ctx), ids[5]); err != nil || scope != "" {
		t.Fatalf("approval replay restored a dissolved membership: %q, %v", scope, err)
	}
	if err := database.WithContext(ctx).First(&profile, ids[5]).Error; err != nil {
		t.Fatal(err)
	}
	player, _, err = decodeProfile(&profile)
	if err != nil || player.GetPlayerId() != ids[5] || player.GetCoin() != beforePlayer.GetCoin() || player.GetDiamond() != beforePlayer.GetDiamond() {
		t.Fatalf("dissolution and replay changed applicant identity or assets: %v", err)
	}
}

func TestPlayerConcurrencyMySQLGuildMonthlyIdentityBoundary(t *testing.T) {
	name := os.Getenv("HYJS_P13_MYSQL_DATABASE")
	if name == "" {
		t.Skip("requires a coordinated isolated hyjs_test_p13_ MySQL database")
	}
	if !strings.HasPrefix(name, "hyjs_test_p13_") || os.Getenv("HYJS_GAME_ACCOUNTS_DATABASE") != "" {
		t.Fatal("requires an isolated guild and accounts fixture")
	}
	f, ids := p13GuildSetup(t)
	database := db.GetIns()
	if database.Dialector.Name() != "mysql" {
		t.Fatal("requires MySQL for the identity cutoff lock")
	}
	pool, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(8)
	pool.SetMaxIdleConns(8)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP13UnionFightMonthly(f.ctx); err != nil {
		t.Fatal(err)
	}
	loc := time.FixedZone("Asia/Shanghai", 8*3600)
	publish := time.Date(2026, 10, 31, 12, 0, 0, 0, loc)
	start, cutoff, err := p13UFMonth("2026-11")
	if err != nil {
		t.Fatal(err)
	}
	rule := db.P13UnionFightMonthlyPolicy{Month: "2026-11", Enabled: true, Version: "isolated-local-future-month-v1", Source: "explicit local8/4 badge policy; original private monthly payouts unknown", PublishedAt: publish.UnixMilli() - 1, Rewards: `[{"rank":1,"rewards":"item_311,8"},{"rank":2,"rewards":"item_311,4"}]`, MemberTitle: true, ChairmanTitle: true}
	if err := database.Create(&rule).Error; err != nil {
		t.Fatalf("publish isolated monthly rule: %v", err)
	}
	if err := f.s.TickP13UnionFightMonthly(f.ctx, publish); err != nil {
		t.Fatalf("arm monthly offer: %v", err)
	}
	var offer db.P13UnionFightMonthlyOffer
	if err := database.First(&offer, "month=?", rule.Month).Error; err != nil || offer.StartAt != start.UnixMilli() || offer.CutoffAt != cutoff.UnixMilli() {
		t.Fatalf("missing armed monthly offer: %v", err)
	}
	p13GuildOpen(t, true)
	joined := start.AddDate(0, 0, 1)
	p13GuildCall(t, f, ids[0], &pb.CSUnionCreate{Name: proto.String("月赛边界甲")}, 401, joined)
	guildA := p13GuildID(t, f, ids[0], joined)
	p13Join(t, f, ids[0], ids[1], guildA, 402, joined)
	p13GuildCall(t, f, ids[2], &pb.CSUnionCreate{Name: proto.String("月赛边界乙")}, 403, joined)
	guildB := p13GuildID(t, f, ids[2], joined)
	p13Join(t, f, ids[2], ids[3], guildB, 404, joined)
	p13GuildCall(t, f, ids[4], &pb.CSUnionApply{UnionId: proto.String(guildB)}, 405, joined)
	if guildA == "" || guildB == "" || guildA == guildB {
		t.Fatal("fixture must create two distinct guilds")
	}
	if p1Count(t, &db.P13UnionFightMonthlyCutoff{}, "month=?", rule.Month) != 0 {
		t.Fatal("monthly identity was archived before its cutoff")
	}

	ctx, cancel := context.WithTimeout(f.ctx, 30*time.Second)
	type markerKey struct{}
	marker := new(int)
	boundaryCtx := context.WithValue(ctx, markerKey{}, marker)
	type result struct {
		messages []proto.Message
		err      error
	}
	var held *gorm.DB
	var locked, registered bool
	var boundaryPending, personalPending <-chan result
	const callbackName = "test:player-concurrency-guild-monthly-writer"
	writerEntered := make(chan struct{})
	var writerOnce sync.Once
	defer func() {
		if locked {
			if err := held.Rollback().Error; err != nil && !errors.Is(err, sql.ErrTxDone) {
				t.Errorf("release held guild: %v", err)
			}
		}
		cancel()
		for _, pending := range []<-chan result{boundaryPending, personalPending} {
			if pending != nil {
				<-pending
			}
		}
		if registered {
			if err := database.Callback().Query().Remove(callbackName); err != nil {
				t.Errorf("remove monthly writer callback: %v", err)
			}
		}
	}()
	if err := database.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		locking, ok := tx.Statement.Clauses["FOR"].Expression.(clause.Locking)
		if ok && locking.Strength == "UPDATE" && tx.Statement.Table == "p13_guild_policies" && tx.Statement.Context.Value(markerKey{}) == marker {
			writerOnce.Do(func() { close(writerEntered) })
		}
	}); err != nil {
		t.Fatalf("register monthly writer callback: %v", err)
	}
	registered = true
	held = database.WithContext(ctx).Begin()
	if held.Error != nil {
		t.Fatalf("begin held guild transaction: %v", held.Error)
	}
	locked = true
	if err := ServerGameClockGuardTransaction(held); err != nil {
		t.Fatal(err)
	}
	var guildPolicy db.P13GuildPolicy
	if err := held.Clauses(clause.Locking{Strength: "SHARE"}).First(&guildPolicy, 1).Error; err != nil {
		t.Fatalf("hold shared guild policy: %v", err)
	}
	var guild db.P13Guild
	if err := held.Select("id").Clauses(clause.Locking{Strength: "UPDATE"}).First(&guild, "id=?", guildA).Error; err != nil {
		t.Fatalf("hold guild A: %v", err)
	}
	boundaryCh := make(chan result, 1)
	boundaryPending = boundaryCh
	go func() {
		messages, err := f.s.p13GuildManage(boundaryCtx, ids[2], &pb.CSUnionAgreeApply{PlayerId: proto.Int64(ids[4])}, 406, cutoff)
		boundaryCh <- result{messages: messages, err: err}
	}()
	select {
	case <-writerEntered:
	case r := <-boundaryPending:
		boundaryPending = nil
		t.Fatalf("boundary mutation ended before requesting the identity writer: %v", r.err)
	case <-ctx.Done():
		t.Fatalf("boundary mutation did not request the identity writer: %v", ctx.Err())
	}
	personalCh := make(chan result, 1)
	personalPending = personalCh
	go func() {
		messages, err := f.s.p13GuildEconomy(ctx, ids[3], &pb.CSGetUnionStamina{}, 407, cutoff)
		personalCh <- result{messages: messages, err: err}
	}()
	timer := time.NewTimer(100 * time.Millisecond)
	defer timer.Stop()
	select {
	case r := <-boundaryPending:
		boundaryPending = nil
		t.Fatalf("identity writer passed the held shared policy: %v", r.err)
	case <-timer.C:
	case <-ctx.Done():
		t.Fatalf("identity writer wait timed out: %v", ctx.Err())
	}
	if p1Count(t, &db.P13UnionFightMonthlyCutoff{}, "month=?", rule.Month) != 0 {
		t.Fatal("identity cutoff appeared before the held policy was released")
	}
	if err := held.Commit().Error; err != nil {
		t.Fatalf("release guild A: %v", err)
	}
	locked = false
	for _, pending := range []*<-chan result{&boundaryPending, &personalPending} {
		select {
		case r := <-*pending:
			*pending = nil
			if r.err != nil || len(r.messages) == 0 {
				t.Fatalf("boundary mutation or personal stamina failed: %v", r.err)
			}
		case <-ctx.Done():
			t.Fatalf("boundary mutation or personal stamina did not finish: %v", ctx.Err())
		}
	}
	var saved db.P13UnionFightMonthlyCutoff
	if err := database.WithContext(ctx).First(&saved, "month=?", rule.Month).Error; err != nil {
		t.Fatalf("missing pre-mutation identity cutoff: %v", err)
	}
	if saved.CutoffAt != cutoff.UnixMilli() || saved.OfferHash != offer.RulesHash || saved.SnapshotHash != p13UFMonthlyCutoffHash(&saved) {
		t.Fatal("monthly identity cutoff does not match its published offer")
	}
	var identities []p13UFMonthlyIdentity
	if err := json.Unmarshal(saved.Members, &identities); err != nil {
		t.Fatal(err)
	}
	byPlayer := make(map[int64]p13UFMonthlyIdentity)
	for _, identity := range identities {
		byPlayer[identity.PlayerID] = identity
	}
	if byPlayer[ids[0]].GuildID != guildA || byPlayer[ids[0]].Role != constUtil.P13GuildChairman ||
		byPlayer[ids[1]].GuildID != guildA || byPlayer[ids[1]].Role != constUtil.P13GuildMember ||
		byPlayer[ids[2]].GuildID != guildB || byPlayer[ids[2]].Role != constUtil.P13GuildChairman ||
		byPlayer[ids[3]].GuildID != guildB || byPlayer[ids[3]].Role != constUtil.P13GuildMember {
		t.Fatal("monthly cutoff lost pre-boundary guild identities")
	}
	if _, found := byPlayer[ids[4]]; found {
		t.Fatal("post-boundary applicant polluted the old monthly identity")
	}
	var member db.P13GuildMember
	if err := database.WithContext(ctx).First(&member, "player_id=?", ids[4]).Error; err != nil || member.GuildID != guildB || member.JoinedAt != cutoff.UnixMilli() {
		t.Fatalf("post-boundary approval did not persist after the archive: %v", err)
	}
}
