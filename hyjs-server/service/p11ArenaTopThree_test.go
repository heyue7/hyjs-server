package service

import (
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// Exercise real match offers and frozen snapshots at rank boundaries, including
// both actual players and virtual opponents.
func TestP11ArenaShowsThreeHigherRanksAndReplaysOffers(t *testing.T) {
	f, peer, peer2 := p11ArenaSetup(t)
	now := time.Now()
	p11ArenaQuery(t, f, f.id, &pb.CSPvpFindMatch{}, 10001, now)
	for id, rank := range map[int64]int64{f.id: 10, peer: 8, peer2: 7} {
		if err := db.GetIns().Model(&db.P11ArenaMember{}).Where("player_id=?", id).Update("position", rank).Error; err != nil {
			t.Fatal(err)
		}
	}
	for index, rank := range []int64{10, 101, 4, 3, 2, 1} {
		if err := db.GetIns().Model(&db.P11ArenaMember{}).Where("player_id=?", f.id).Update("position", rank).Error; err != nil {
			t.Fatal(err)
		}
		stamp := int64(10002 + index)
		query := p11ArenaQuery(t, f, f.id, &pb.CSPvpFindMatch{}, stamp, now).(*pb.SCPvpFindMatch)
		if query.GetMyrank().GetPvpRank() != rank {
			t.Fatalf("self rank %d: %v", rank, query.GetMyrank())
		}
		seen := map[int64]bool{}
		for _, target := range query.GetRankList() {
			if target.GetInfo().GetPlayerId() == f.id || seen[target.GetPvpRank()] {
				t.Fatal("self or duplicate candidate", target)
			}
			seen[target.GetPvpRank()] = true
		}
		for ahead := max(int64(1), rank-3); ahead < rank; ahead++ {
			if !seen[ahead] {
				t.Fatalf("rank %d omitted the higher rank %d", rank, ahead)
			}
		}
		if replay := p11ArenaQuery(t, f, f.id, &pb.CSPvpFindMatch{}, stamp, now.Add(time.Second)); !proto.Equal(query, replay) {
			t.Fatalf("rank %d offer changed on replay", rank)
		}
		if rank == 10 {
			for _, target := range query.GetRankList() {
				if target.GetPvpRank() < 7 || target.GetPvpRank() > 9 {
					continue
				}
				var match db.P11ArenaMatch
				if err := db.GetIns().Where("viewer_id=? AND opponent_id=? AND viewer_rank=?", f.id, target.GetInfo().GetPlayerId(), rank).First(&match).Error; err != nil {
					t.Fatal(err)
				}
				frozen, err := f.s.RecoverP11ArenaMatch(f.ctx, f.id, match.ID, now)
				if err != nil || frozen.Defender.GetPlayerId() != target.GetInfo().GetPlayerId() {
					t.Fatalf("rank %d has no valid frozen battle: %v", target.GetPvpRank(), err)
				}
			}
		}
	}
	if err := db.GetIns().Model(&db.P11ArenaMember{}).Where("player_id=?", f.id).Update("position", 10).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Create(&db.P10SocialBlack{OwnerID: f.id, TargetID: peer, CreatedAt: now.UnixMilli()}).Error; err != nil {
		t.Fatal(err)
	}
	blocked := p11ArenaQuery(t, f, f.id, &pb.CSPvpFindMatch{}, 10020, now).(*pb.SCPvpFindMatch)
	for _, target := range blocked.GetRankList() {
		if target.GetInfo().GetPlayerId() == peer {
			t.Fatal("blocked player exposed as a target")
		}
	}
}

func TestP11ArenaFightThirdHigherRankSettlesOnce(t *testing.T) {
	if constUtil.P6ArenaNativeProof == "" {
		t.Skip("original arena combat certificate pending")
	}
	f, peer := p11ArenaScopedSetup(t)
	now := time.Now()
	for _, table := range []any{&db.P11ArenaPolicy{}, &db.P11ArenaFightPolicy{}} {
		if err := db.GetIns().Model(table).Where("id=1").Update("enabled", true).Error; err != nil {
			t.Fatal(err)
		}
	}
	p11ArenaQuery(t, f, f.id, &pb.CSPvpFindMatch{}, 10100, now)
	for id, rank := range map[int64]int64{f.id: 10, peer: 7} {
		if err := db.GetIns().Model(&db.P11ArenaMember{}).Where("player_id=?", id).Update("position", rank).Error; err != nil {
			t.Fatal(err)
		}
	}
	offer := p11ArenaQuery(t, f, f.id, &pb.CSPvpFindMatch{}, 10101, now).(*pb.SCPvpFindMatch)
	found := false
	for _, target := range offer.GetRankList() {
		found = found || target.GetPvpRank() == 7 && target.GetInfo().GetPlayerId() == peer
	}
	if !found {
		t.Fatal("third higher real player not offered")
	}
	before, _ := p10LuckyProfile(t, f.id)
	request := &pb.CSPvpFightResult{PlayerId: proto.Int64(peer), ServerId: proto.Int32(1)}
	result, err := f.s.p11ArenaScopedFightAt(f.ctx, f.id, request, 10102, now)
	if err != nil {
		t.Fatal(err)
	}
	reply := result[0].(*pb.SCPvpFightResult)
	newRank := int64(10)
	if reply.GetWinPlayerId() == f.id {
		newRank = 7
	}
	if reply.GetMyrank().GetPvpRank() != newRank {
		t.Fatalf("unexpected settled rank: %d", reply.GetMyrank().GetPvpRank())
	}
	replayed, err := f.s.p11ArenaScopedFightAt(f.ctx, f.id, request, 10102, now)
	if err != nil || !proto.Equal(reply, replayed[0]) {
		t.Fatalf("fight retry changed settlement: %v", err)
	}
	after, _ := p10LuckyProfile(t, f.id)
	if after.GetEnergy() != before.GetEnergy()-2 || p1Count(t, &db.P11ArenaFightSettlement{}, "player_id=? AND stamp=?", f.id, 10102) != 1 {
		t.Fatal("fight retry changed cost or settlement count")
	}
}
