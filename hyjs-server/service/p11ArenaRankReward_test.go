package service

import (
	"hyjs-server/db"
	"testing"
)

func TestP11ArenaHistoricalRankDiamondBoundary(t *testing.T) {
	cases := []struct {
		name                            string
		member                          db.P11ArenaMember
		achieved, wantBest, wantDiamond int64
	}{
		{"first-deep-win", db.P11ArenaMember{Rank: 32768, RankRewardBest: 32768}, 24576, 24576, 8192},
		{"legacy-20-dropped-25", db.P11ArenaMember{Rank: 25, BestWonRank: 20}, 18, 18, 2},
		{"former-first-returns", db.P11ArenaMember{Rank: 10, BestWonRank: 1}, 1, 1, 0},
		{"later-player-above-cap", db.P11ArenaMember{Rank: 40000, RankRewardBest: 32768}, 38000, 32768, 0},
		{"later-player-crosses-cap", db.P11ArenaMember{Rank: 38000, RankRewardBest: 32768}, 30000, 30000, 2768},
		{"already-paid-deep-boundary", db.P11ArenaMember{Rank: 30000, BestWonRank: 24576, RankRewardBest: 24576}, 24576, 24576, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := p11ArenaRankRewardBoundary(tc.member)
			after, gain := p11ArenaRankRewardDelta(before, tc.achieved, true)
			if after != tc.wantBest || gain != tc.wantDiamond {
				t.Fatal(before, after, gain)
			}
		})
	}
	best := int64(32768)
	total := int64(0)
	for rank := int64(32768); rank > 500; {
		rank = p11ArenaBestTarget(rank)
		var gain int64
		best, gain = p11ArenaRankRewardDelta(best, rank, true)
		total += gain
	}
	if total != 32268 {
		t.Fatal("15-win cumulative", total)
	}
	_, total = p11ArenaRankRewardDelta(40000, 1, true)
	if total != 32767 {
		t.Fatal("maximum lifetime diamonds", total)
	}
}
