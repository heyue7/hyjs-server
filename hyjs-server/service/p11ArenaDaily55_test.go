package service

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func TestP11ArenaDaily55PermanentMidnightRealClaimColdOnce(t *testing.T) {
	if constUtil.P6ArenaNativeProof == "" {
		t.Skip("typed2 original certificate pending")
	}
	f, challenger, clock, _ := p11ArenaDailySetup(t)
	start, end := p11ArenaDailyWindow(*clock, 0)
	policy := db.P11ArenaDailyPolicy{ID: 1, Enabled: true, Permanent: true, Version: constUtil.P11ArenaDailyPermanentVersion, Source: constUtil.P11ArenaDailyPermanentSource, PublishedAt: clock.UnixMilli(), StartAt: start.UnixMilli(), CutoffSecond: 0, MinChallenges: 1}
	if err := db.GetIns().Save(&policy).Error; err != nil {
		t.Fatal(err)
	}
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

func TestP11ArenaDaily55MigrationFirstDayNoChallengeAndBeyond32Days(t *testing.T) {
	f, _, clock, _ := p11ArenaDailySetup(t)
	zero := db.P11ArenaDailyPolicy{ID: 1, Version: "local-arena-daily-rank-v1", Source: constUtil.P11ArenaDailySource, MinChallenges: 1}
	if err := db.GetIns().Save(&zero).Error; err != nil {
		t.Fatal(err)
	}
	before := time.Now().UnixMilli()
	if err := f.s.InitializeP11ArenaDaily(f.ctx); err != nil {
		t.Fatal(err)
	}
	var p db.P11ArenaDailyPolicy
	if err := db.GetIns().First(&p, 1).Error; err != nil {
		t.Fatal(err)
	}
	if !p.Enabled || !p.Permanent || p.CutoffSecond != 0 || p.PublishedAt < before || !p11ArenaDailyPolicyValid(&p) {
		t.Fatal("permanent migration invalid", p)
	}
	frozen, _ := json.Marshal(p)
	if err := f.s.InitializeP11ArenaDaily(f.ctx); err != nil {
		t.Fatal(err)
	}
	var again db.P11ArenaDailyPolicy
	db.GetIns().First(&again, 1)
	stable, _ := json.Marshal(again)
	if !bytes.Equal(frozen, stable) {
		t.Fatal("publication timestamp reset")
	}
	*clock = time.UnixMilli(p.PublishedAt).Add(time.Second)
	if err := f.s.TickP11ArenaDaily(f.ctx, *clock); err != nil {
		t.Fatal(err)
	}
	_, end := p11ArenaDailyWindow(*clock, 0)
	*clock = end.Add(time.Second)
	if err := f.s.TickP11ArenaDaily(f.ctx, *clock); err != nil {
		t.Fatal(err)
	}
	var awards int64
	if err := db.GetIns().Model(&db.P11ArenaDailyAward{}).Count(&awards).Error; err != nil || awards != 0 {
		t.Fatal("unearned first-day reward", awards, err)
	}
	*clock = end.AddDate(0, 0, 40)
	if err := f.s.TickP11ArenaDaily(f.ctx, *clock); err != nil {
		t.Fatal(err)
	}
	start, _ := p11ArenaDailyWindow(*clock, 0)
	var offer db.P11ArenaDailyOffer
	if err := db.GetIns().First(&offer, "day=?", start.Format("20060102")).Error; err != nil {
		t.Fatal("permanent stopped after32days", err)
	}
	if offer.CapturedAt != clock.UnixMilli() {
		t.Fatal("historical offer invented")
	}
	var count int64
	db.GetIns().Model(&db.P11ArenaDailyOffer{}).Count(&count)
	if count != 3 {
		t.Fatal("missed days retroactively offered", count)
	}
	// Explicit finite operator publication remains unchanged on restart.
	p.Permanent = false
	p.Enabled = false
	p.Source = constUtil.P11ArenaDailySource
	p.Version = "explicit-finite"
	p.PublishedAt = p.StartAt - 1
	p.EndAt = p.StartAt + 86400000
	if err := db.GetIns().Save(&p).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP11ArenaDaily(f.ctx); err != nil {
		t.Fatal(err)
	}
	db.GetIns().First(&again, 1)
	if again.Permanent || again.Enabled || again.Version != "explicit-finite" {
		t.Fatal("operator finite policy overwritten")
	}
}
