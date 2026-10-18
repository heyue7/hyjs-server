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
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p11ArenaDailySetup(t *testing.T) (*p1Fixture, int64, *time.Time, time.Time) {
	t.Helper()
	f, peer := p11ArenaScopedSetup(t)
	if err := f.s.LoadP11ArenaDailyTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP11ArenaDaily(f.ctx); err != nil {
		t.Fatal(err)
	}
	for _, table := range []any{&db.P11ArenaDailyChallenge{}, &db.P11ArenaDailyAward{}, &db.P11ArenaDailySnapshot{}, &db.P11ArenaDailyOffer{}} {
		if err := db.GetIns().Where("1=1").Delete(table).Error; err != nil {
			t.Fatal(err)
		}
	}
	at := time.Now().Truncate(time.Second)
	f.ctx = context.WithValue(f.ctx, constUtil.P11ArenaDailyClockContextKey{}, func() time.Time { return at })
	local := at.In(time.FixedZone("UTC+8", 28800))
	cutoff := int64(local.Hour()*3600 + local.Minute()*60 + local.Second())
	start, end := p11ArenaDailyWindow(at, cutoff)
	// Only local publication timing and finite account levels are fixture inputs;
	// the challenge, energy payment, rank transition and rewards are actual APIs.
	policy := db.P11ArenaDailyPolicy{ID: 1, Enabled: true, Version: "isolated-local-daily-v1", Source: constUtil.P11ArenaDailySource, PublishedAt: start.Add(-time.Second).UnixMilli(), StartAt: start.UnixMilli(), EndAt: end.UnixMilli(), CutoffSecond: cutoff, MinChallenges: 1}
	if err := db.GetIns().Save(&policy).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P11ArenaPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P11ArenaFightPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{f.id, peer} {
		level := int32(20)
		if id == peer {
			level = 100
		}
		if err := updateProfile(f.ctx, id, func(p *pb.Player, g *model.GameState) error { p.Level = proto.Int32(level); return nil }); err != nil {
			t.Fatal(err)
		}
	}
	// Daily original-tier coverage is exercised with a preserved legacy top rank.
	// Fresh entrants now start after the virtual NPC slots, outside these tiers.
	if err := db.GetIns().Model(&db.P11ArenaDailyPolicy{}).Where("id=1").Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p11ArenaQuery(f.ctx, f.id, &pb.CSPvpFindMatch{}, 39099, at); err != nil {
		t.Fatal(err)
	}
	var members []db.P11ArenaMember
	if err := db.GetIns().Order("position").Find(&members).Error; err != nil {
		t.Fatal(err)
	}
	for i, member := range members {
		if err := db.GetIns().Model(&db.P11ArenaMember{}).Where("player_id=?", member.PlayerID).Update("position", i+1).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.GetIns().Model(&db.P11ArenaDailyPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	return f, peer, &at, end
}

func TestP11ArenaDailyRealChallengerCutoffOfflineClaimAndColdOnce(t *testing.T) {
	if constUtil.P6ArenaNativeProof == "" {
		t.Skip("typed2 original certificate pending")
	}
	f, challenger, clock, end := p11ArenaDailySetup(t)
	requestNow := *clock
	if _, err := f.s.p11ArenaQuery(f.ctx, challenger, &pb.CSPvpFindMatch{}, 39101, requestNow); err != nil {
		t.Fatal(err)
	}
	var n int64
	if err := db.GetIns().Model(&db.P11ArenaDailyAward{}).Count(&n).Error; err != nil || n != 0 {
		t.Fatal("initial rank granted daily reward", n, err)
	}
	*clock = clock.Add(2 * time.Second)
	request := &pb.CSPvpFightResult{PlayerId: proto.Int64(f.id), ServerId: proto.Int32(1)}
	out, err := f.s.p11ArenaScopedFightAt(f.ctx, challenger, request, 39102, requestNow)
	if err != nil {
		t.Fatal(err)
	}
	response := out[0].(*pb.SCPvpFightResult)
	if response.GetWinPlayerId() != challenger {
		t.Fatal("real stronger challenger did not win")
	}
	original, _ := proto.MarshalOptions{Deterministic: true}.Marshal(response)
	var fact db.P11ArenaDailyChallenge
	if err = db.GetIns().First(&fact, "player_id=?", challenger).Error; err != nil || fact.AcceptedAt != clock.UnixMilli() {
		t.Fatal("admission followed request timestamp", fact, err)
	}
	var row db.P11ArenaFightSettlement
	if err = db.GetIns().First(&row, "fight_id=?", fact.FightID).Error; err != nil || row.CreatedAt != requestNow.UnixMilli() || fact.AcceptedAt <= row.CreatedAt {
		t.Fatal("request/admission boundary not exercised", err)
	}
	before, _ := p10LuckyProfile(t, challenger)
	*clock = end.Add(time.Second)
	// Close honors the first immutable offer even when the operator disables the
	// source and changes its current version before the deadline worker catches up.
	if err = db.GetIns().Model(&db.P11ArenaDailyPolicy{}).Where("id=1").Updates(map[string]any{"enabled": false, "version": "changed-after-freeze", "min_challenges": 100}).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err = f.s.TickP11ArenaDaily(f.ctx, *clock); err != nil {
			t.Fatal(err)
		}
	}
	var awards []db.P11ArenaDailyAward
	if err = db.GetIns().Order("player_id").Find(&awards).Error; err != nil || len(awards) != 1 {
		t.Fatal("daily initial/defense-only participant award", awards, err)
	}
	award := awards[0]
	if award.PlayerID != challenger || award.Rank != 1 || award.Rewards != "popularity,5000;coin,120000;item_007,50" || award.SourceHash != p11ArenaDailyAwardHash(award) {
		t.Fatal("original tier or immutable award changed", award)
	}
	after, _ := p10LuckyProfile(t, challenger)
	if !proto.Equal(before, after) {
		t.Fatal("offline delivery mutated wallet before original claim")
	}
	// Cold reconnection reads the same committed P1 attachment and paid fight.
	connection, _ := db.GetIns().DB()
	if err = connection.Close(); err != nil {
		t.Fatal(err)
	}
	f.open(t)
	if err = f.s.TickP11ArenaDaily(f.ctx, *clock); err != nil {
		t.Fatal(err)
	}
	paid, err := f.s.p11ArenaScopedFightAt(f.ctx, challenger, request, 39102, requestNow)
	if err != nil {
		t.Fatal(err)
	}
	replay, _ := proto.MarshalOptions{Deterministic: true}.Marshal(paid[0])
	if !bytes.Equal(original, replay) {
		t.Fatal("cutoff/cold changed old original battle body")
	}
	claim := &pb.CSAddRewardsToPlayer{RewardIds: []int64{award.DeliveryID}}
	if _, err = f.s.RewardCenter(f.ctx, challenger, claim); err != nil {
		t.Fatal(err)
	}
	got, game := p10LuckyProfile(t, challenger)
	var item007 int32
	for _, item := range game.Items {
		if item.GetId() == "item_007" {
			item007 += item.GetNum()
		}
	}
	if got.GetCoin() != before.GetCoin()+120000 || got.GetPopularity() != before.GetPopularity()+5000 || item007 != 50 {
		t.Fatal("daily original reward did not reach current inventory", got.GetCoin(), got.GetPopularity(), game.Items)
	}
	stable, _ := json.Marshal([]any{got, game})
	if _, err = f.s.RewardCenter(f.ctx, challenger, claim); err != nil {
		t.Fatal(err)
	}
	got, game = p10LuckyProfile(t, challenger)
	again, _ := json.Marshal([]any{got, game})
	if !bytes.Equal(stable, again) {
		t.Fatal("late claim repeated reward")
	}
	if err = db.GetIns().Model(&db.P11ArenaDailyChallenge{}).Count(&n).Error; err != nil || n != 1 {
		t.Fatal("paid old retry created new daily qualification", n, err)
	}
}

func TestP11ArenaDailyOriginalSevenTiersAndUnpublishedWindow(t *testing.T) {
	f, _, clock, end := p11ArenaDailySetup(t)
	for rank, want := range map[int64]string{1: "popularity,5000;coin,120000;item_007,50", 5: "popularity,4000;coin,100000;item_007,40", 20: "popularity,3500;coin,80000;item_007,35", 50: "popularity,3000;coin,50000;item_007,30", 100: "popularity,2500;coin,40000;item_007,25", 500: "popularity,2000;coin,20000;item_007,15", 1000: "popularity,1000;coin,10000;item_007,10", 1001: ""} {
		if got := p11ArenaDailyReward(rank); got != want {
			t.Fatal(rank, got)
		}
	}
	// A worker first started after the published window must not reconstruct a past
	// offer or use initial ranks as a historical tournament result.
	*clock = end.Add(time.Second)
	if err := f.s.TickP11ArenaDaily(f.ctx, *clock); err != nil {
		t.Fatal(err)
	}
	for _, table := range []any{&db.P11ArenaDailyOffer{}, &db.P11ArenaDailySnapshot{}, &db.P11ArenaDailyAward{}} {
		var n int64
		if err := db.GetIns().Model(table).Count(&n).Error; err != nil || n != 0 {
			t.Fatal("manufactured historical source", n, err)
		}
	}
	var policy db.P11ArenaDailyPolicy
	if err := db.GetIns().First(&policy, 1).Error; err != nil {
		t.Fatal(err)
	}
	policy.CutoffSecond = 86400
	if p11ArenaDailyPolicyValid(&policy) {
		t.Fatal("invalid cutoff accepted")
	}
}

// A missing supported ranking is a pending cutoff, with no partial deliveries.
// The deliberately invalid position models missing historical transition data;
// it never grants rank, battle authority or a positive test qualification.
func TestP11ArenaDailyMissingRankTransitionPendingNoPartialDelivery(t *testing.T) {
	if constUtil.P6ArenaNativeProof == "" {
		t.Skip("typed2 original certificate pending")
	}
	f, challenger, clock, end := p11ArenaDailySetup(t)
	if _, err := f.s.p11ArenaQuery(f.ctx, challenger, &pb.CSPvpFindMatch{}, 39201, *clock); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p11ArenaScopedFightAt(f.ctx, challenger, &pb.CSPvpFightResult{PlayerId: proto.Int64(f.id), ServerId: proto.Int32(1)}, 39202, *clock); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P11ArenaMember{}).Where("player_id=?", challenger).Update("position", 999).Error; err != nil {
		t.Fatal(err)
	}
	*clock = end.Add(time.Second)
	if err := f.s.TickP11ArenaDaily(f.ctx, *clock); err != nil {
		t.Fatal(err)
	}
	var snapshot db.P11ArenaDailySnapshot
	if err := db.GetIns().First(&snapshot).Error; err != nil || snapshot.Status != constUtil.P11ArenaDailyPending || snapshot.Reason != "current-rank-not-supported-by-last-transition" || snapshot.RanksJSON != "[]" {
		t.Fatal("unsupported historical rank was awarded", snapshot, err)
	}
	var count int64
	if err := db.GetIns().Model(&db.P11ArenaDailyAward{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("partial pending award", count, err)
	}
	if err := db.GetIns().Model(&db.RewardDelivery{}).Where("source_key LIKE 'arena-daily:%'").Count(&count).Error; err != nil || count != 0 {
		t.Fatal("pending cutoff leaked P1 attachment", count, err)
	}
}
