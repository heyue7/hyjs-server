package service

import (
	"encoding/json"
	"math"
	"strconv"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Original fixed guild reward strings must survive the same P1 parser used by
// monthly arming, inbox delivery and atomic collection. No war result is made.
func TestP13GuildOriginalWarRewardResourcesContributionP1(t *testing.T) {
	f, ids := p13GuildSetup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"UnionFightRankReward", "UnionFightRankSpecialReward"} {
		for _, row := range f.s.Tables[table] {
			ok, err := f.s.openServerRewardSupported(row["reward"])
			if err != nil || !ok {
				t.Fatal(table, row["rank"], "original reward refused", err)
			}
		}
	}
	var defs []p13UFWeeklyReward
	for _, row := range f.s.Tables["UnionFightRankSpecialReward"] {
		rank, err := strconv.Atoi(row["rank"])
		if err != nil {
			t.Fatal(err)
		}
		defs = append(defs, p13UFWeeklyReward{Rank: int32(rank), Rewards: row["reward"]})
	}
	raw, _ := json.Marshal(defs)
	policy := db.P13UnionFightMonthlyPolicy{Month: "2099-01", Enabled: true, Version: "original-resource-acceptance", Source: "original20 fixed reward rows, no historical award", PublishedAt: time.Now().Add(-time.Second).UnixMilli(), Rewards: string(raw), MemberTitle: true, ChairmanTitle: true}
	if err := db.GetIns().Create(&policy).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.p13UFMonthlyArm(db.GetIns(), time.Now(), policy.Month); err != nil {
		t.Fatal("original monthly reward must arm", err)
	}
	if p1Count(t, &db.P13UnionFightMonthlyOffer{}, "month=?", policy.Month) != 1 {
		t.Fatal("missing actual future offer")
	}
	f.id = ids[0]
	before, _ := f.profile(t)
	reward := f.reward(t, "guild-original-contribution", "diamond,1500;coin,2000000;contribution,2000", 0)
	claim := &pb.CSAddRewardsToPlayer{RewardIds: []int64{reward.ID}}
	if _, err := f.s.RewardCenter(f.ctx, f.id, claim); err != nil {
		t.Fatal(err)
	}
	after, _ := f.profile(t)
	if after.GetContribution() != before.GetContribution()+2000 || after.GetCoin() != before.GetCoin()+2000000 || after.GetDiamond() != before.GetDiamond()+1500 {
		t.Fatal("original mixed reward wallet mismatch")
	}
	if _, err := f.s.RewardCenter(f.ctx, f.id, claim); err != nil {
		t.Fatal(err)
	}
	duplicate, _ := f.profile(t)
	if !proto.Equal(after, duplicate) {
		t.Fatal("duplicate claim changed wallet")
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error { p.Contribution = proto.Int32(math.MaxInt32); return nil }); err != nil {
		t.Fatal(err)
	}
	full, _ := f.profile(t)
	overflow := f.reward(t, "guild-contribution-overflow", "coin,10;contribution,1", 0)
	if _, err := f.s.RewardCenter(f.ctx, f.id, &pb.CSAddRewardsToPlayer{RewardIds: []int64{overflow.ID}}); err == nil {
		t.Fatal("overflow accepted")
	}
	unchanged, _ := f.profile(t)
	if !proto.Equal(full, unchanged) {
		t.Fatal("failed mixed claim partially granted")
	}
	var saved db.RewardDelivery
	if err := db.GetIns().First(&saved, overflow.ID).Error; err != nil || saved.ClaimedAt != 0 {
		t.Fatal("failed claim consumed receipt", err)
	}
}
