package service

import (
	"bytes"
	"encoding/json"
	"errors"
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

func TestP11ArenaVirtualCurveFifteenWinsAndBoundedRewards(t *testing.T) {
	rank := int64(32768)
	var ranks []int64
	for rank > 500 {
		rank = p11ArenaBestTarget(rank)
		ranks = append(ranks, rank)
		if len(ranks) > 15 {
			t.Fatal("outside500 curve too slow", ranks)
		}
	}
	if len(ranks) != 15 || rank != 500 {
		t.Fatal("fifteen-win boundary", ranks)
	}
	for current, expected := range map[int64]int64{501: 500, 500: 450, 101: 100, 100: 95, 11: 10, 10: 9, 2: 1, 1: 1} {
		if got := p11ArenaBestTarget(current); got != expected {
			t.Fatal(current, got, expected)
		}
	}
	if p11ArenaDailyReward(32768) != "" || p11ArenaDailyReward(32767) != "" {
		t.Fatal("unconfigured outer ranks extrapolated rewards")
	}
	t.Log("fastest target ranks", ranks)
}

func TestP11ArenaVirtualLegacyPositionsAndLazyTopRank(t *testing.T) {
	f, b, c := p11ArenaSetup(t)
	now := time.Now()
	first := p11ArenaQuery(t, f, f.id, &pb.CSPvpFindMatch{}, 990001, now).(*pb.SCPvpFindMatch)
	if first.GetMyrank().GetPvpRank() != 32768 || len(first.GetRankList()) != 9 {
		t.Fatal("new roster entry", first)
	}
	var count int64
	db.GetIns().Model(&db.P11ArenaMember{}).Count(&count)
	if count != 3 {
		t.Fatal("NPC rows materialized", count)
	}
	// This isolated fixture represents an existing deployment whose three
	// real positions must survive the new append cursor without a mass reorder.
	for index, id := range []int64{f.id, b, c} {
		if err := db.GetIns().Model(&db.P11ArenaMember{}).Where("player_id=?", id).Update("position", int64(index+1)).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.GetIns().Model(&db.P11ArenaPolicy{}).Where("id=1").Update("next_rank", 4).Error; err != nil {
		t.Fatal(err)
	}
	listing := p11ArenaQuery(t, f, f.id, &pb.CSPvpRank{}, 0, now).(*pb.SCPvpRank)
	if listing.GetMyrank().GetPvpRank() != 1 || len(listing.GetRankList()) != 50 {
		t.Fatal("legacy/top lazy listing", listing)
	}
	for index, id := range []int64{f.id, b, c} {
		var member db.P11ArenaMember
		if err := db.GetIns().First(&member, "player_id=?", id).Error; err != nil || member.Rank != int64(index+1) {
			t.Fatal("legacy rank reordered", member, err)
		}
	}
	var cfg db.P11ArenaPolicy
	db.GetIns().First(&cfg, 1)
	if cfg.NextRank != 32771 {
		t.Fatal("virtual slot count lost", cfg.NextRank)
	}
	if !p11ArenaNPCID(listing.GetRankList()[3].GetInfo().GetPlayerId()) {
		t.Fatal("top empty slot missing NPC")
	}
	db.GetIns().Model(&db.Profile{}).Where("id>=?", constUtil.P11ArenaNPCIDBase).Count(&count)
	if count != 0 {
		t.Fatal("virtual profiles created")
	}
}

func TestP11ArenaVirtualOriginalGuideFightSlotConflictAndColdReceipt(t *testing.T) {
	f, peer, other := p11ArenaSetup(t)
	for _, id := range []int64{peer, other} {
		if err := updateProfile(f.ctx, id, func(p *pb.Player, _ *model.GameState) error { p.Level = proto.Int32(9); return nil }); err != nil {
			t.Fatal(err)
		}
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error {
		p.Level = proto.Int32(10)
		p.NoviceGuideSchedule = proto.Int32(12)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, table := range []any{&db.P11ArenaPolicy{}, &db.P11ArenaFightPolicy{}} {
		if err := db.GetIns().Model(table).Where("id=1").Update("enabled", true).Error; err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	listing, err := f.s.p11ArenaGuideAt(f.ctx, f.id, &pb.CSPvpFindMatchGuide{}, 991001, now)
	if err != nil {
		t.Fatal(err)
	}
	query := listing[0].(*pb.SCPvpFindMatchGuide)
	if query.GetMyrank().GetPvpRank() != 32768 || len(query.GetRankList()) != 9 {
		t.Fatal("single real player no targets", query)
	}
	target := query.GetRankList()[0]
	if target.GetPvpRank() != 24576 || !p11ArenaNPCID(target.GetInfo().GetPlayerId()) {
		t.Fatal("untrusted target projection", target)
	}
	info, _, err := f.s.p11ArenaNPCPreview(f.ctx, f.id, target.GetInfo().GetPlayerId(), now)
	if err != nil || len(info.GetFormationItemInfoList()) != 1 || info.GetFormationItemInfoList()[0].GetWarInfo().GetHeroId() != constUtil.P11ArenaNPCHeroID {
		t.Fatal("NPC original profile preview", info, err)
	}
	// Simulate another real member having just occupied this frozen target
	// while holding the same policy lock used by every actual rank writer.
	if err := updateProfile(f.ctx, peer, func(p *pb.Player, _ *model.GameState) error { p.Level = proto.Int32(10); return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p11ArenaQuery(f.ctx, peer, &pb.CSPvpFindMatch{}, 991002, now); err != nil {
		t.Fatal(err)
	}
	var peerMember db.P11ArenaMember
	db.GetIns().First(&peerMember, "player_id=?", peer)
	oldPeerRank := peerMember.Rank
	if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		if _, e := p11ArenaConfig(tx); e != nil {
			return e
		}
		return tx.Model(&peerMember).Update("position", target.GetPvpRank()).Error
	}); err != nil {
		t.Fatal(err)
	}
	before, _ := p10LuckyProfile(t, f.id)
	request := &pb.CSPvpFightResultGuide{PlayerId: target.GetInfo().PlayerId, ServerId: proto.Int32(1)}
	if _, err = f.s.p11ArenaGuideAt(f.ctx, f.id, request, 991003, now); err == nil {
		t.Fatal("occupied virtual rank admitted")
	}
	after, _ := p10LuckyProfile(t, f.id)
	if !proto.Equal(before, after) {
		t.Fatal("rejected occupied slot charged profile")
	}
	if err := db.GetIns().Model(&peerMember).Update("position", oldPeerRank).Error; err != nil {
		t.Fatal(err)
	}
	result, err := f.s.p11ArenaGuideAt(f.ctx, f.id, request, 991004, now)
	if err != nil {
		t.Fatal("actual original NPC settlement", err)
	}
	response := result[0].(*pb.SCPvpFightResultGuide)
	current, game := p10LuckyProfile(t, f.id)
	if current.GetEnergy() != before.GetEnergy()-2 || current.GetNoviceGuideSchedule() != 13 || len(response.GetPlayerList()) != 2 || response.GetPlayerList()[1].GetPlayerId() != target.GetInfo().GetPlayerId() {
		t.Fatal("NPC original response/cost/guide", response, current)
	}
	if response.GetWinPlayerId() == f.id && response.GetMyrank().GetPvpRank() != 24576 {
		t.Fatal("NPC winner rank not moved", response)
	}
	if response.GetWinPlayerId() != f.id || current.GetDiamond() != before.GetDiamond() {
		t.Fatal("first NPC historical rank diamonds", current.GetDiamond(), before.GetDiamond())
	}
	var rankAward db.P11ArenaPrizeAward
	if err := db.GetIns().First(&rankAward, "fight_id=?", response.GetFightId()).Error; err != nil || rankAward.RankBestBefore != 32768 || rankAward.RankBestAfter != 24576 || rankAward.RankDiamond != 8192 {
		t.Fatal("rank reward facts", rankAward, err)
	}
	var rankDelivery db.RewardDelivery
	if err := db.GetIns().First(&rankDelivery, "player_id=? AND source_key=?", f.id, p11ArenaRankRewardCenterKey(response.GetFightId())).Error; err != nil || rankDelivery.Rewards != "diamond,8192" || rankDelivery.Title != "竞技场新纪录" || rankDelivery.Content != "历史最高排名提升至第24576名。" || rankAward.RankSource != constUtil.P11ArenaRankRewardCenterSource {
		t.Fatal("rank attachment", rankDelivery, err)
	}
	if p1Count(t, &db.MailMessage{}, "player_id=?", f.id) != 0 {
		t.Fatal("automatic rank mail")
	}
	if len(game.Warriors) != 1 {
		t.Fatal("NPC granted fake characters")
	}
	raw, _ := proto.Marshal(response)
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, e := f.s.p11ArenaGuideAt(f.ctx, f.id, request, 991004, now)
			if e == nil {
				b, _ := proto.Marshal(out[0])
				if !bytes.Equal(b, raw) {
					e = fmtArenaNPCMismatch()
				}
			}
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
	dbConn, _ := db.GetIns().DB()
	dbConn.Close()
	f.open(t)
	replayed, err := f.s.p11ArenaGuideAt(f.ctx, f.id, request, 991004, now)
	if err != nil {
		t.Fatal("cold NPC receipt", err)
	}
	cold, _ := proto.Marshal(replayed[0])
	if !bytes.Equal(raw, cold) {
		t.Fatal("cold NPC response changed")
	}
	final, finalGame := p10LuckyProfile(t, f.id)
	if !proto.Equal(final, current) || len(finalGame.Warriors) != len(game.Warriors) {
		t.Fatal("retry/cold charged or generated state")
	}
	for i := 0; i < 2; i++ {
		if _, err = f.s.RewardCenter(f.ctx, f.id, &pb.CSAddRewardsToPlayer{RewardIds: []int64{rankDelivery.ID}}); err != nil {
			t.Fatal(err)
		}
	}
	dbConn, _ = db.GetIns().DB()
	dbConn.Close()
	f.open(t)
	if _, err = f.s.p11ArenaGuideAt(f.ctx, f.id, request, 991004, now); err != nil {
		t.Fatal("claimed cold receipt", err)
	}
	claimed, _ := p10LuckyProfile(t, f.id)
	if claimed.GetDiamond() != before.GetDiamond()+8192 || p1Count(t, &db.RewardDelivery{}, "player_id=? AND source_key=?", f.id, rankDelivery.SourceKey) != 1 {
		t.Fatal("rank claim duplicated")
	}
	var records, virtualMembers int64
	db.GetIns().Model(&db.P11ArenaFightSettlement{}).Where("player_id=?", f.id).Count(&records)
	db.GetIns().Model(&db.P11ArenaMember{}).Where("player_id>=?", constUtil.P11ArenaNPCIDBase).Count(&virtualMembers)
	if records != 1 || virtualMembers != 0 {
		t.Fatal("virtual state/receipt duplicated", records, virtualMembers)
	}
	var arenaRecord db.P11ArenaFightSettlement
	db.GetIns().Where("player_id=?", f.id).First(&arenaRecord)
	var originalRecord db.P6BattleRecord
	db.GetIns().First(&originalRecord, "id=?", arenaRecord.P6RecordID)
	if !originalRecord.Authoritative {
		t.Fatal("NPC bypassed original P6")
	}
	meta, _ := json.Marshal(map[string]any{"winner": response.GetWinPlayerId(), "rank": response.GetMyrank().GetPvpRank(), "energy": current.GetEnergy(), "guide": current.GetNoviceGuideSchedule(), "P6": originalRecord.Authoritative, "rankDiamond": rankAward.RankDiamond})
	t.Log(string(meta))
}

func fmtArenaNPCMismatch() error { return errors.New("NPC same-stamp original response changed") }

// Synthetic legacy seats represent old empty-server enrollment only; no live
// player data or rewards are inserted, and witnessed best ranks remain fixed.
func TestP11ArenaVirtualUntouchedLegacySeatMigratesOnlyOnce(t *testing.T) {
	f, peer, _ := p11ArenaSetup(t)
	now := time.Now()
	if _, err := f.s.p11ArenaQuery(f.ctx, f.id, &pb.CSPvpFindMatch{}, 992001, now); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P11ArenaMember{}).Where("player_id=?", f.id).Updates(map[string]any{"position": int64(1), "rank_reward_best": int64(0), "best_won_rank": int64(0)}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P11ArenaMember{}).Where("player_id=?", peer).Updates(map[string]any{"position": int64(2), "rank_reward_best": int64(0), "best_won_rank": int64(2)}).Error; err != nil {
		t.Fatal(err)
	}
	query, err := f.s.p11ArenaQuery(f.ctx, f.id, &pb.CSPvpFindMatch{}, 992002, now)
	if err != nil {
		t.Fatal(err)
	}
	first := query[0].(*pb.SCPvpFindMatch)
	if first.GetMyrank().GetPvpRank() <= 32767 || len(first.GetRankList()) < 1 {
		t.Fatal("legacy tutorial no NPC targets", first)
	}
	var migrated, witnessed db.P11ArenaMember
	if err = db.GetIns().First(&migrated, "player_id=?", f.id).Error; err != nil {
		t.Fatal(err)
	}
	if migrated.RankRewardBest != 32768 || migrated.BestWonRank != 0 {
		t.Fatal("migration fabricated reward/history", migrated)
	}
	if err = db.GetIns().First(&witnessed, "player_id=?", peer).Error; err != nil || witnessed.Rank != 2 || witnessed.BestWonRank != 2 {
		t.Fatal("witnessed legacy rank moved", witnessed, err)
	}
	if _, err = f.s.p11ArenaQuery(f.ctx, f.id, &pb.CSPvpFindMatch{}, 992003, now); err != nil {
		t.Fatal(err)
	}
	var after db.P11ArenaMember
	db.GetIns().First(&after, "player_id=?", f.id)
	if after.Rank != migrated.Rank {
		t.Fatal("legacy seat migrated twice", migrated.Rank, after.Rank)
	}
}
