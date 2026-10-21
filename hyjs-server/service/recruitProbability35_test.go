package service

import (
	"hyjs-server/db"
	"math"
	"testing"
)

func TestRecruitProbability35Curve(t *testing.T) {
	for _, h := range []int32{10, 20, 30, 90, 100, 1000} {
		prev := int64(0)
		for miss := int32(0); miss < h; miss++ {
			p := recruitHighProbability35(6, 1000, miss, h)
			if p < prev || p > 1000000 {
				t.Fatalf("H=%d miss=%d p=%d", h, miss, p)
			}
			if miss+1 <= h*8/10 && p != 6000 {
				t.Fatal("base boundary")
			}
			prev = p
		}
		if prev != 1000000 {
			t.Fatal("hard pity")
		}
	}
	if recruitHighProbability35(6, 1000, 2147483647, 90) != 1000000 {
		t.Fatal("since overflow")
	}
}
func TestRecruitProbability35Groups(t *testing.T) {
	high := db.GameplayPoolReward{ID: 1, Weight: 6, Guaranteed: true}
	low := db.GameplayPoolReward{ID: 2, Weight: 994}
	for _, rs := range [][]db.GameplayPoolReward{nil, {{Weight: 0}}, {{Weight: -1}}, {{Weight: 1000000000001}}, {low}} {
		if _, _, _, e := recruitProbabilityGroups35(rs, 0, 10); e == nil {
			t.Fatal("invalid accepted")
		}
	}
	if _, _, _, e := recruitProbabilityGroups35([]db.GameplayPoolReward{low}, 0, 0); e != nil {
		t.Fatal(e)
	}
	for _, h := range []int32{0, 10} {
		r, e := chooseRecruitProbabilityReward([]db.GameplayPoolReward{high}, 0, h)
		if e != nil || r.ID != 1 {
			t.Fatal(r, e)
		}
	}
	// A hard-pity hit resets the next draw to base probability within the same ten-pull.
	if recruitHighProbability35(6, 1000, 9, 10) != 1000000 || recruitHighProbability35(6, 1000, 0, 10) != 6000 {
		t.Fatal("reset")
	}
	if _, _, _, e := recruitProbabilityGroups35([]db.GameplayPoolReward{high, low}, -1, 10); e == nil {
		t.Fatal("negative since")
	}
}

func TestRecruitProbability35Overflow(t *testing.T) {
	if _, e := recruitAddWeight35(math.MaxInt64, 1); e == nil {
		t.Fatal("overflow accepted")
	}
	if sum, e := recruitAddWeight35(math.MaxInt64-1, 1); e != nil || sum != math.MaxInt64 {
		t.Fatal(sum, e)
	}
}
