package service

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// Only the original test accounts' initial levels/ranks and a finite published
// source are fixture inputs. Every accepted outcome, quota and attachment below
// is produced by the original query, native fight and P1 delivery consumers.
func TestArenaClock75ArenaNativeForwardRewindWorkerAndCold(t *testing.T) {
	if constUtil.P6ArenaNativeProof == "" {
		t.Skip("typed2 original certificate pending")
	}
	f, challenger, _, _ := p11ArenaDailySetup(t)
	// The legacy fixture's injected wall clock must not mask the real Admin clock.
	f.ctx = context.WithValue(f.ctx, constUtil.P11ArenaDailyClockContextKey{}, nil)
	if err := f.s.InitializeServerGameClock(f.ctx); err != nil {
		t.Fatal(err)
	}
	actor := adminWalletTestActor(t, f, constUtil.AdminRoleAdmin)
	for _, id := range []int64{f.id, challenger} {
		if _, err := f.s.fieldTransfer(f.ctx, id, &pb.CSTransferScene{Target: proto.String("town_main")}); err != nil {
			t.Fatal(err)
		}
	}
	ctx := guildClock75Move(t, f, actor, challenger, 6)
	forward := LocalPlayNow(ctx)
	published := forward.Add(-time.Second)
	start, _ := p11ArenaDailyWindow(published, 0)
	policy := db.P11ArenaDailyPolicy{ID: 1, Enabled: true, Permanent: true, Version: constUtil.P11ArenaDailyPermanentVersion, Source: constUtil.P11ArenaDailyPermanentSource, PublishedAt: published.UnixMilli(), StartAt: start.UnixMilli(), MinChallenges: 1}
	if err := db.GetIns().Save(&policy).Error; err != nil {
		t.Fatal(err)
	}
	policyRaw, _ := json.Marshal(policy)
	query := func(ctx context.Context, stamp int64) db.P11ArenaMatch {
		t.Helper()
		now := LocalPlayNow(ctx)
		if _, err := f.s.p11ArenaQuery(ctx, challenger, &pb.CSPvpFindMatch{}, stamp, now); err != nil {
			t.Fatal(err)
		}
		var match db.P11ArenaMatch
		if err := db.GetIns().Where("viewer_id=? AND opponent_id=? AND created_at<=?", challenger, f.id, now.UnixMilli()).Order("created_at DESC,id DESC").First(&match).Error; err != nil {
			t.Fatal(err)
		}
		return match
	}
	request := &pb.CSPvpFightResult{PlayerId: proto.Int64(f.id), ServerId: proto.Int32(1)}
	fight := func(ctx context.Context, stamp int64) []proto.Message {
		t.Helper()
		out, err := f.s.p11ArenaScopedFightAt(ctx, challenger, request, stamp, LocalPlayNow(ctx))
		if err != nil {
			t.Fatal(err)
		}
		if out[0].(*pb.SCPvpFightResult).GetWinPlayerId() != challenger {
			t.Fatal("real stronger challenger did not win")
		}
		return out
	}
	query(ctx, 750101)
	first := fight(ctx, 750102)
	firstRaw, _ := proto.MarshalOptions{Deterministic: true}.Marshal(first[0])
	var firstRow db.P11ArenaFightSettlement
	if err := db.GetIns().First(&firstRow, "player_id=? AND stamp=?", challenger, 750102).Error; err != nil || firstRow.EnergySpent != 2 {
		t.Fatal("forward actual fight", err)
	}
	// This unused future match is generated after the real rank transition, so
	// rank mismatch cannot accidentally conceal selection of a future snapshot.
	future := query(ctx, 750103)
	ctx = guildClock75Move(t, f, actor, challenger, -15)
	back := LocalPlayNow(ctx)
	before, _ := p10LuckyProfile(t, challenger)
	if _, err := f.s.p11ArenaScopedFightAt(ctx, challenger, request, 750104, back); err == nil {
		t.Fatal("future-only match accepted after rewind")
	}
	after, _ := p10LuckyProfile(t, challenger)
	if !proto.Equal(before, after) {
		t.Fatal("future match rejection spent resources")
	}
	current := query(ctx, 750105)
	fight(ctx, 750106)
	var paid db.P11ArenaFightSettlement
	if err := db.GetIns().First(&paid, "player_id=? AND stamp=?", challenger, 750106).Error; err != nil || paid.MatchID != current.ID || paid.MatchID == future.ID || paid.EnergySpent != 2 {
		t.Fatal("rewind selected stale future match", paid.MatchID, err)
	}
	var facts []db.P11ArenaDailyChallenge
	if err := db.GetIns().Where("player_id=?", challenger).Order("id").Find(&facts).Error; err != nil || len(facts) != 2 || dailyCopyDay(time.UnixMilli(facts[0].AcceptedAt)) != dailyCopyDay(forward) || dailyCopyDay(time.UnixMilli(facts[1].AcceptedAt)) != dailyCopyDay(back) {
		t.Fatal("actual admissions used wall or double-offset date", len(facts), err)
	}
	day, _ := p11ArenaDailyWindow(back, 0)
	var offer db.P11ArenaDailyOffer
	if err := db.GetIns().First(&offer, "day=?", day.Format("20060102")).Error; err != nil {
		t.Fatal("rewind permanent source absent", err)
	}
	if _, err := p11ArenaDailyOfferRules(&offer); err != nil {
		t.Fatal(err)
	}
	var unchanged db.P11ArenaDailyPolicy
	if err := db.GetIns().First(&unchanged, 1).Error; err != nil {
		t.Fatal(err)
	}
	unchangedRaw, _ := json.Marshal(unchanged)
	if !bytes.Equal(policyRaw, unchangedRaw) {
		t.Fatal("rewind persisted a backdated source")
	}
	// The unbound background worker must bind the persisted game clock itself.
	ctx = guildClock75Move(t, f, actor, challenger, 1)
	if err := f.s.TickP11ArenaDaily(f.ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	var awards []db.P11ArenaDailyAward
	if err := db.GetIns().Order("day,player_id").Find(&awards).Error; err != nil || len(awards) != 1 {
		t.Fatal("rewound actual day did not close once", len(awards), err)
	}
	award := awards[0]
	if award.Day != offer.Day || award.PlayerID != challenger || award.Rank != 1 || award.SourceHash != p11ArenaDailyAwardHash(award) {
		t.Fatal("daily award lost actual challenger/rank proof", award)
	}
	frozenAward, _ := json.Marshal(award)
	before, _ = p10LuckyProfile(t, challenger)
	claim := &pb.CSAddRewardsToPlayer{RewardIds: []int64{award.DeliveryID}}
	if _, err := f.s.RewardCenter(ctx, challenger, claim); err != nil {
		t.Fatal(err)
	}
	after, _ = p10LuckyProfile(t, challenger)
	if after.GetCoin() != before.GetCoin()+120000 || after.GetPopularity() != before.GetPopularity()+5000 {
		t.Fatal("original daily attachment not paid", after.GetCoin()-before.GetCoin(), after.GetPopularity()-before.GetPopularity())
	}
	// Visit the same actual played day again and close it again. Qualification
	// and the already claimed attachment retain their original immutable identity.
	ctx = guildClock75Move(t, f, actor, challenger, -1)
	query(ctx, 750107)
	ctx = guildClock75Move(t, f, actor, challenger, 1)
	if err := f.s.TickP11ArenaDaily(f.ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	connection, _ := db.GetIns().DB()
	if err := connection.Close(); err != nil {
		t.Fatal(err)
	}
	f.open(t)
	if err := f.s.TickP11ArenaDaily(f.ctx, time.Now()); err != nil {
		t.Fatal("cold background clock", err)
	}
	if _, err := f.s.RewardCenter(ctx, challenger, claim); err != nil {
		t.Fatal(err)
	}
	stable, _ := p10LuckyProfile(t, challenger)
	if !proto.Equal(after, stable) {
		t.Fatal("revisit or cold replay granted twice")
	}
	var n int64
	if err := db.GetIns().Model(&db.P11ArenaDailyAward{}).Count(&n).Error; err != nil || n != 1 {
		t.Fatal("daily reward duplicated", n, err)
	}
	var saved db.P11ArenaDailyAward
	if err := db.GetIns().First(&saved, "day=? AND player_id=?", award.Day, challenger).Error; err != nil {
		t.Fatal(err)
	}
	savedRaw, _ := json.Marshal(saved)
	if !bytes.Equal(frozenAward, savedRaw) {
		t.Fatal("immutable day award rewritten")
	}
	ctx = guildClock75Move(t, f, actor, challenger, 14)
	replay, err := f.s.p11ArenaScopedFightAt(ctx, challenger, request, 750102, LocalPlayNow(ctx))
	if err != nil {
		t.Fatal("forward paid fight replay", err)
	}
	replayRaw, _ := proto.MarshalOptions{Deterministic: true}.Marshal(replay[0])
	if !bytes.Equal(firstRaw, replayRaw) {
		t.Fatal("forward battle receipt changed after rewind/cold")
	}
	var savedFirst db.P11ArenaFightSettlement
	if err := db.GetIns().First(&savedFirst, "player_id=? AND stamp=?", challenger, 750102).Error; err != nil || !bytes.Equal(firstRow.Terminal, savedFirst.Terminal) || firstRow.ResponseHash != savedFirst.ResponseHash {
		t.Fatal("immutable actual battle changed", err)
	}
	if err := db.GetIns().Model(&db.P11ArenaDailyChallenge{}).Count(&n).Error; err != nil || n != 2 {
		t.Fatal("replay created extra challenge", n, err)
	}
}

func TestArenaClock75LimitNativeDisabledDailyAndFutureMatch(t *testing.T) {
	f, b, c := p11ArenaSetup(t)
	if err := f.s.InitializeServerGameClock(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP11LimitFight(f.ctx); err != nil {
		t.Fatal(err)
	}
	var dailyPolicy db.P11LimitDailyPolicy
	if err := db.GetIns().First(&dailyPolicy, 1).Error; err != nil || dailyPolicy.Enabled {
		t.Fatal("optional daily source must remain default closed", err)
	}
	actor := adminWalletTestActor(t, f, constUtil.AdminRoleAdmin)
	for _, id := range []int64{f.id, b, c} {
		if _, err := f.s.fieldTransfer(f.ctx, id, &pb.CSTransferScene{Target: proto.String("town_main")}); err != nil {
			t.Fatal(err)
		}
	}
	real := time.Now()
	starts, ends := real.AddDate(0, 0, -10), real.AddDate(0, 0, 8)
	// Explicit finite operator calendar only; score, quota and battle results
	// remain outputs of original mode20 consumers, including the entry award.
	if err := db.GetIns().Model(&db.P11LimitPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "period_id": "arena-clock75-finite-mode20", "starts_at": starts.UnixMilli(), "ends_at": ends.UnixMilli()}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P11LimitFightPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "published_at": starts.Add(-time.Second).UnixMilli(), "start_at": starts.UnixMilli(), "end_at": ends.UnixMilli()}).Error; err != nil {
		t.Fatal(err)
	}
	ctx := guildClock75Move(t, f, actor, f.id, 6)
	query := func(ctx context.Context, id, stamp int64) *pb.SCLimitPvpFindMatch {
		t.Helper()
		replies, err := f.s.p11LimitAt(ctx, id, &pb.CSLimitPvpFindMatch{}, stamp, LocalPlayNow(ctx))
		if err != nil {
			t.Fatal(err)
		}
		return replies[0].(*pb.SCLimitPvpFindMatch)
	}
	query(ctx, b, 751101)
	query(ctx, c, 751102)
	query(ctx, f.id, 751103)
	var future db.P11LimitFightMatch
	if err := db.GetIns().Where("player_id=? AND opponent_id=?", f.id, b).Order("created_at DESC,id DESC").First(&future).Error; err != nil {
		t.Fatal(err)
	}
	ctx = guildClock75Move(t, f, actor, f.id, -15)
	back := LocalPlayNow(ctx)
	query(ctx, b, 751104)
	queried := query(ctx, f.id, 751105)
	if queried.GetMyrank().GetPvpIntegral() != 20 || queried.GetFightTimes() != 0 {
		t.Fatal("original per-day entry/free quota", queried)
	}
	var current db.P11LimitFightMatch
	if err := db.GetIns().Where("player_id=? AND opponent_id=? AND created_at<=?", f.id, b, back.Add(time.Minute).UnixMilli()).Order("created_at DESC,id DESC").First(&current).Error; err != nil {
		t.Fatal(err)
	}
	request := &pb.CSLimitPvpFightResult{PlayerId: proto.Int64(b), ServerId: proto.Int32(1)}
	out, err := f.s.p11LimitAt(ctx, f.id, request, 751106, LocalPlayNow(ctx))
	if err != nil {
		t.Fatal("actual mode20 rejected while optional daily rank source closed", err)
	}
	raw, _ := proto.MarshalOptions{Deterministic: true}.Marshal(out[0])
	var paid db.P11LimitFightSettlement
	if err := db.GetIns().First(&paid, "player_id=? AND stamp=?", f.id, 751106).Error; err != nil || paid.MatchID != current.ID || paid.MatchID == future.ID || paid.Day != dailyCopyDay(back) || paid.EnergySpent != 2 || paid.BeforeUsed != 0 || paid.AfterUsed != 1 {
		t.Fatal("actual mode20 chose future match or wrong day", paid, err)
	}
	before, _ := p10LuckyProfile(t, f.id)
	replay, err := f.s.p11LimitAt(ctx, f.id, request, 751106, LocalPlayNow(ctx))
	if err != nil {
		t.Fatal(err)
	}
	again, _ := proto.MarshalOptions{Deterministic: true}.Marshal(replay[0])
	after, _ := p10LuckyProfile(t, f.id)
	if !bytes.Equal(raw, again) || !proto.Equal(before, after) {
		t.Fatal("mode20 retry spent again")
	}
	ctx = guildClock75Move(t, f, actor, f.id, 15)
	query(ctx, f.id, 751107)
	ctx = guildClock75Move(t, f, actor, f.id, -15)
	revisited := query(ctx, f.id, 751108)
	if revisited.GetFirstRewardScore() != 0 || revisited.GetFightTimes() != 1 || revisited.GetMyrank().GetPvpIntegral() != paid.AfterScore {
		t.Fatal("revisit reset original day quota/entry", revisited)
	}
	connection, _ := db.GetIns().DB()
	if err := connection.Close(); err != nil {
		t.Fatal(err)
	}
	f.open(t)
	if err := f.s.TickP11LimitDaily(f.ctx, time.Now()); err != nil {
		t.Fatal("closed-source background worker", err)
	}
	replay, err = f.s.p11LimitAt(ctx, f.id, request, 751106, LocalPlayNow(ctx))
	if err != nil {
		t.Fatal("cold mode20 replay", err)
	}
	again, _ = proto.MarshalOptions{Deterministic: true}.Marshal(replay[0])
	if !bytes.Equal(raw, again) {
		t.Fatal("cold battle receipt changed")
	}
	var saved db.P11LimitFightSettlement
	if err := db.GetIns().First(&saved, "player_id=? AND stamp=?", f.id, 751106).Error; err != nil || paid.ResponseHash != saved.ResponseHash || paid.TerminalHash != saved.TerminalHash {
		t.Fatal("actual native proof changed", err)
	}
	var fact db.P11LimitDailyChallenge
	if err := db.GetIns().First(&fact, "fight_id=?", paid.FightID).Error; err != nil || fact.SourceHash != p11LimitDailyChallengeHash(fact) || dailyCopyDay(time.UnixMilli(fact.AcceptedAt)) != paid.Day {
		t.Fatal("actual mode20 journal lost virtual admission", err)
	}
	for _, table := range []any{&db.P11LimitDailyAward{}, &db.P11LimitDailyOffer{}} {
		var n int64
		if err := db.GetIns().Model(table).Count(&n).Error; err != nil || n != 0 {
			t.Fatal("disabled optional source fabricated day rewards", n, err)
		}
	}
}
