package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p11ArenaScopedSetup(t *testing.T) (*p1Fixture, int64) {
	t.Helper()
	f, peer, _ := p11ArenaSetup(t)
	for _, table := range []any{&db.P11ArenaFightSettlement{}, &db.P11ArenaFightPolicy{}} {
		if db.GetIns().Migrator().HasTable(table) {
			if err := db.GetIns().Where("1=1").Delete(table).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := f.s.InitializeP11ArenaFight(f.ctx); err != nil {
		t.Fatal(err)
	}
	return f, peer
}

func TestP11ArenaScopedDefaultOffUnknownAndPendingProofNoMutation(t *testing.T) {
	f, peer := p11ArenaScopedSetup(t)
	now := time.Now()
	account, err := LoginAccount(f.ctx, fmt.Sprintf("arena-unknown-main-%d", now.UnixNano()), "local-isolated-only")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(f.ctx, account.Id, "未知竞技主角", "hero_044"); err != nil {
		t.Fatal(err)
	}
	if err = updateProfile(f.ctx, account.Id, func(p *pb.Player, g *model.GameState) error { p.Level = proto.Int32(20); return nil }); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P11ArenaPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "duel_enabled": true}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p11ArenaQuery(f.ctx, f.id, &pb.CSPvpFindMatch{}, 701, now); err != nil {
		t.Fatal(err)
	}
	profiles := func() []byte {
		t.Helper()
		a, b := p10LuckyProfile(t, f.id)
		c, d := p10LuckyProfile(t, peer)
		e, g := p10LuckyProfile(t, account.Id)
		raw, err := json.Marshal([]any{a, b, c, d, e, g})
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	before := profiles()
	reject := func(target, stamp int64) {
		t.Helper()
		out, err := f.s.p11ArenaScopedFightAt(f.ctx, f.id, &pb.CSPvpFightResult{PlayerId: proto.Int64(target), ServerId: proto.Int32(1)}, stamp, now)
		if err == nil || out != nil {
			t.Fatal("unsupported public arena succeeded", err)
		}
		for _, table := range []any{&db.P11ArenaFightSettlement{}, &db.P6BattleRecord{}} {
			var n int64
			if err := db.GetIns().Model(table).Where("player_id=?", f.id).Count(&n).Error; err != nil || n != 0 {
				t.Fatal("partial cost/rank/capture/receipt", n, err)
			}
		}
		if !bytes.Equal(profiles(), before) {
			t.Fatal("rejected arena changed profile/energy")
		}
		var members []db.P11ArenaMember
		if err := db.GetIns().Where("best_won_rank>0").Find(&members).Error; err != nil || len(members) != 0 {
			t.Fatal("failed/initial rank produced winning qualification", err)
		}
	}
	var policy db.P11ArenaFightPolicy
	if err := db.GetIns().First(&policy, 1).Error; err != nil || policy.Enabled || policy.Energy != 2 {
		t.Fatal("new challenge not independent defaultoff", err)
	}
	reject(peer, 0)
	reject(peer, 702)
	if err := db.GetIns().Model(&policy).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	if constUtil.P6ArenaNativeProof == "" {
		reject(peer, 703)
	}
	reject(account.Id, 704)
	// Even a legacy/corrupted numeric best rank must have an actual accepted
	// winner witness. A query's initial localposition1 cannot grant all five titles.
	if err := db.GetIns().Model(&db.P11ArenaMember{}).Where("player_id=?", f.id).Update("best_won_rank", 1).Error; err != nil {
		t.Fatal(err)
	}
	rank, proof, err := P11ArenaVerifiedBestWonRank(db.GetIns().WithContext(f.ctx), f.id)
	if err != nil || rank != 0 || proof != "" {
		t.Fatal("unwitnessed initial/legacy numeric rank granted title", rank, proof, err)
	}
}

// Each mutation below is isolated in one transaction boundary. The earlier
// business rejection must occur before the pending native certificate gate.
func TestP11ArenaScopedRejectedCostsRanksAndMatchesRemainAtomic(t *testing.T) {
	f, peer := p11ArenaScopedSetup(t)
	now := time.Now()
	if err := db.GetIns().Model(&db.P11ArenaPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P11ArenaFightPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p11ArenaQuery(f.ctx, f.id, &pb.CSPvpFindMatch{}, 801, now); err != nil {
		t.Fatal(err)
	}
	var frozen db.P11ArenaMatch
	if err := db.GetIns().Where("viewer_id=? AND opponent_id=?", f.id, peer).First(&frozen).Error; err != nil {
		t.Fatal(err)
	}
	request := &pb.CSPvpFightResult{PlayerId: proto.Int64(peer), ServerId: proto.Int32(1)}
	snapshot := func() []byte {
		t.Helper()
		var profiles []db.Profile
		var ranks []db.P11ArenaMember
		if err := db.GetIns().Where("id IN ?", []int64{f.id, peer}).Order("id").Find(&profiles).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.GetIns().Order("player_id").Find(&ranks).Error; err != nil {
			t.Fatal(err)
		}
		raw, err := json.Marshal([]any{profiles, ranks})
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	reject := func(want string, at time.Time) {
		t.Helper()
		before := snapshot()
		out, err := f.s.p11ArenaScopedFightAt(f.ctx, f.id, request, 802, at)
		if err == nil || err.Error() != want || out != nil {
			t.Fatalf("wrong pre-cost boundary: got %v want %q", err, want)
		}
		if !bytes.Equal(before, snapshot()) {
			t.Fatal("rejected challenge modified profile, energy or rank")
		}
		for _, table := range []any{&db.P6BattleRecord{}, &db.P11ArenaFightSettlement{}} {
			var n int64
			if err := db.GetIns().Model(table).Count(&n).Error; err != nil || n != 0 {
				t.Fatal("rejected challenge retained capture/receipt", n, err)
			}
		}
	}
	reject(constUtil.P11ArenaFightRefreshRequired, now.Add(6*time.Minute))
	// A modified rank with unchanged query snapshot is rejected rather than
	// silently challenging a different rank position.
	if err := db.GetIns().Model(&db.P11ArenaMember{}).Where("player_id=?", peer).Update("position", 100).Error; err != nil {
		t.Fatal(err)
	}
	reject(constUtil.P11ArenaFightRefreshRequired, now)
	if err := db.GetIns().Model(&db.P11ArenaMember{}).Where("player_id=?", peer).Update("position", frozen.OpponentRank).Error; err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error { p.Energy = proto.Int32(1); return nil }); err != nil {
		t.Fatal(err)
	}
	reject(constUtil.P11ArenaFightEnergyRequired, now)
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error { p.Energy = proto.Int32(100); return nil }); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P11ArenaFightPolicy{}).Where("id=1").Update("source", "unpublished-cost-change").Error; err != nil {
		t.Fatal(err)
	}
	reject(constUtil.P11ArenaClosed, now)
	if err := db.GetIns().Model(&db.P11ArenaFightPolicy{}).Where("id=1").Updates(map[string]any{"source": constUtil.P11ArenaFightPolicySource, "energy": 3}).Error; err != nil {
		t.Fatal(err)
	}
	reject(constUtil.P11ArenaClosed, now)
}

// The future positive checks are intentionally unavailable until original typed2
// evidence fixes an independent certificate. They never substitute Friend11.
func TestP11ArenaScopedRealWinnerPaymentRanksConcurrentColdAndQualification(t *testing.T) {
	if constUtil.P6ArenaNativeProof == "" {
		t.Skip("original typed2 native certificate pending")
	}
	for _, strongAttacker := range []bool{true, false} {
		t.Run(fmt.Sprintf("strongAttacker-%t", strongAttacker), func(t *testing.T) {
			f, peer := p11ArenaScopedSetup(t)
			now := time.Now()
			challenger, defender := peer, f.id
			for _, id := range []int64{challenger, defender} {
				level := int32(20)
				if (id == challenger) == strongAttacker {
					level = 100
				}
				if err := updateProfile(f.ctx, id, func(p *pb.Player, g *model.GameState) error { p.Level = proto.Int32(level); return nil }); err != nil {
					t.Fatal(err)
				}
			}
			if err := db.GetIns().Model(&db.P11ArenaPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.GetIns().Model(&db.P11ArenaFightPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
				t.Fatal(err)
			}
			if _, err := f.s.p11ArenaQuery(f.ctx, challenger, &pb.CSPvpFindMatch{}, 901, now); err != nil {
				t.Fatal(err)
			}
			var own, beforeDef db.P11ArenaMember
			if err := db.GetIns().First(&own, "player_id=?", challenger).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.GetIns().First(&beforeDef, "player_id=?", defender).Error; err != nil {
				t.Fatal(err)
			}
			if own.Rank <= beforeDef.Rank {
				t.Fatal("actual eligible challenger must start below actual opponent", own.Rank, beforeDef.Rank)
			}
			beforeOwner, _ := p10LuckyProfile(t, challenger)
			beforeOther, otherGame := p10LuckyProfile(t, defender)
			request := &pb.CSPvpFightResult{PlayerId: proto.Int64(defender), ServerId: proto.Int32(1)}
			workers := 1
			if f.mysql.Database != "" {
				workers = 8
			}
			replies := make([][]byte, workers)
			failures := make([]error, workers)
			var group sync.WaitGroup
			for index := 0; index < workers; index++ {
				group.Add(1)
				go func(index int) {
					defer group.Done()
					result, err := f.s.p11ArenaScopedFightAt(f.ctx, challenger, request, 902, now)
					if err == nil {
						if len(result) != 2 {
							failures[index] = fmt.Errorf("missing current SCPlayer")
							return
						}
						replies[index], err = proto.MarshalOptions{Deterministic: true}.Marshal(result[0])
					}
					failures[index] = err
				}(index)
			}
			group.Wait()
			for index, err := range failures {
				if err != nil || !bytes.Equal(replies[0], replies[index]) {
					t.Fatal("concurrent arena not once", index, err)
				}
			}
			var receipt db.P11ArenaFightSettlement
			if err := db.GetIns().First(&receipt, "player_id=? AND stamp=902", challenger).Error; err != nil {
				t.Fatal(err)
			}
			response := &pb.SCPvpFightResult{}
			if err := proto.Unmarshal(replies[0], response); err != nil {
				t.Fatal(err)
			}
			if (response.GetWinPlayerId() == challenger) != strongAttacker {
				t.Fatal("real terminal failed independently native-covered unequal source", response.GetWinPlayerId())
			}
			ownerAfter, _ := p10LuckyProfile(t, challenger)
			otherAfter, afterOtherGame := p10LuckyProfile(t, defender)
			if ownerAfter.GetEnergy() != beforeOwner.GetEnergy()-2 || !proto.Equal(beforeOther, otherAfter) {
				t.Fatal("payment not exactly challenger energy2")
			}
			a, _ := json.Marshal(otherGame)
			b, _ := json.Marshal(afterOtherGame)
			if !bytes.Equal(a, b) {
				t.Fatal("defender inventory changed")
			}
			expectedOwn, expectedDef := own.Rank, beforeDef.Rank
			if strongAttacker {
				expectedOwn, expectedDef = expectedDef, expectedOwn
			}
			if receipt.AttackerNewRank != expectedOwn || receipt.DefenderNewRank != expectedDef || response.GetMyrank().GetPvpRank() != expectedOwn {
				t.Fatal("rank transition not actual win-only exchange", receipt)
			}
			var liveOwn, liveDef db.P11ArenaMember
			if err := db.GetIns().First(&liveOwn, "player_id=?", challenger).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.GetIns().First(&liveDef, "player_id=?", defender).Error; err != nil {
				t.Fatal(err)
			}
			if liveOwn.Rank != expectedOwn || liveDef.Rank != expectedDef {
				t.Fatal("half rank exchange")
			}
			winnerID := response.GetWinPlayerId()
			rank, proof, err := P11ArenaVerifiedBestWonRank(db.GetIns(), winnerID)
			expectedBest := expectedDef
			if winnerID == challenger {
				expectedBest = expectedOwn
			}
			if err != nil || rank != expectedBest || proof == "" {
				t.Fatal("actual winner source cannot support qualification", rank, proof, err)
			}
			loserID := challenger
			if winnerID == challenger {
				loserID = defender
			}
			rank, proof, err = P11ArenaVerifiedBestWonRank(db.GetIns(), loserID)
			if err != nil || rank != 0 || proof != "" {
				t.Fatal("loser granted title qualification", rank, proof, err)
			}
			for _, table := range []any{&db.P6BattleRecord{}, &db.P11ArenaFightSettlement{}} {
				var n int64
				if err := db.GetIns().Model(table).Where("player_id=?", challenger).Count(&n).Error; err != nil || n != 1 {
					t.Fatal("multiple paid results", n, err)
				}
			}
			// Accepted paid source survives later roster/rank/config changes and block;
			// new challenges stay closed and no retry can consume another payment.
			if err := updateProfile(f.ctx, defender, func(p *pb.Player, g *model.GameState) error { p.Level = proto.Int32(21); return nil }); err != nil {
				t.Fatal(err)
			}
			if err := db.GetIns().Model(&db.P11ArenaPolicy{}).Where("id=1").Update("enabled", false).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.GetIns().Model(&db.P11ArenaFightPolicy{}).Where("id=1").Update("enabled", false).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.GetIns().Create(&db.P10SocialBlack{OwnerID: defender, TargetID: challenger, CreatedAt: now.UnixMilli()}).Error; err != nil {
				t.Fatal(err)
			}
			f.s.Tables["Skill"][0]["Duration"] = "999"
			f.restart(t)
			result, err := f.s.p11ArenaScopedFightAt(f.ctx, challenger, request, 902, now.Add(time.Hour))
			if err != nil {
				t.Fatal("paid historical result lost", err)
			}
			replay, err := proto.MarshalOptions{Deterministic: true}.Marshal(result[0])
			if err != nil || !bytes.Equal(replay, replies[0]) {
				t.Fatal("old paid result recomputed", err)
			}
			replayPlayer := result[1].(*pb.SCPlayer).GetPlayer()
			if replayPlayer.GetEnergy() != beforeOwner.GetEnergy()-2 {
				t.Fatal("retry did not send current real balance")
			}
			if out, err := f.s.p11ArenaScopedFightAt(f.ctx, challenger, request, 903, now); err == nil || out != nil {
				t.Fatal("blocked/closed new fight succeeded")
			}
			if err := db.GetIns().Model(&receipt).Update("scope_proof", "unknown-certificate").Error; err != nil {
				t.Fatal(err)
			}
			if out, err := f.s.p11ArenaScopedFightAt(f.ctx, challenger, request, 902, now); err == nil || out != nil {
				t.Fatal("unrecognized paid receipt succeeded")
			}
			rank, proof, err = P11ArenaVerifiedBestWonRank(db.GetIns(), winnerID)
			if err == nil || rank != 0 || proof != "" {
				t.Fatal("corrupt receipt unlocked rank title", rank, proof, err)
			}
		})
	}
}
