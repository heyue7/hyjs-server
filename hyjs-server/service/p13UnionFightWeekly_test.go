package service

import (
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"sync/atomic"
	"testing"
	"time"
)

func TestP13WeeklyActualTwoTimedRoundsSundayOfflineCold(t *testing.T) {
	p13ZeroFightActualTimedRoundAndLocalInbox(t, true, false, true, true)
}
func TestP13WeeklyActualRefusedCombatKeepsWeekUnsettled(t *testing.T) {
	p13ZeroFightActualTimedRoundAndLocalInbox(t, true, false, true, true, true)
}

func TestP13WeeklyActualUnmatchedEligibleDeclarationKeepsWeekUnsettled(t *testing.T) {
	p13ZeroFightActualTimedRoundAndLocalInbox(t, true, false, true, true, false, true)
}

func p13WeeklyActualSundayClaims(t *testing.T, f *p1Fixture, match *db.P13UnionFightMatch, ids []int64, clock *atomic.Int64, pending bool, resolvedTwo bool) {
	t.Helper()
	sub := int32(match.Group[0] - 'A' + 1)
	query := &pb.CSUFRankInfo{RankType: proto.Int32(1), SubType: proto.Int32(sub)}
	frames, handled, err := f.s.DispatchP13UnionFight(f.ctx, ids[0], query, 100)
	if err != nil || !handled {
		t.Fatal("real weekly query", err)
	}
	rows := frames[0].(*pb.SCUFRankInfo).GetRankIno().GetList()
	if len(rows) != 2 || rows[0].GetRank() != 1 || len(rows[0].GetValues()) != 2 || rows[0].GetValues()[0] != func() int32 {
		if !resolvedTwo {
			return 50
		}
		return 100
	}() {
		t.Fatal("actual prestige ranking", rows)
	}
	day, _ := time.ParseInLocation("2006-01-02", match.Day, time.FixedZone("Asia/Shanghai", 8*3600))
	week := p13UFWeek(day)
	start, _ := p13UFWeeklyStart(week)
	sunday := start.AddDate(0, 0, 6)
	if err = f.s.TickP13UnionFightWeekly(f.ctx, sunday.Add(-time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if p1Count(t, &db.P13UnionFightWeeklyResult{}, "week_start=? AND `group`=?", week, match.Group) != 0 {
		t.Fatal("early weekly reward")
	}
	// Current operator closure and cold restart cannot edit the already frozen publication.
	if err = db.GetIns().Model(&db.P13UnionFightWeeklyPolicy{}).Where("week_start=? AND `group`=?", week, match.Group).Updates(map[string]any{"enabled": false, "rewards": `[{"rank":1,"rewards":"item_311,999"}]`}).Error; err != nil {
		t.Fatal(err)
	}
	sql, _ := db.GetIns().DB()
	sql.Close()
	f.open(t)
	clock.Store(sunday.UnixMilli())
	for i := 0; i < 8; i++ {
		if err = f.s.TickP13UnionFightWeekly(f.ctx, sunday); err != nil {
			t.Fatal(err)
		}
	}
	count := p1Count(t, &db.P13UnionFightWeeklyResult{}, "week_start=? AND `group`=?", week, match.Group)
	if pending {
		if count != 0 {
			t.Fatal("pending war manufactured weekly result")
		}
		return
	}
	if count != 1 {
		t.Fatal("missing weekly result", count)
	}
	var result db.P13UnionFightWeeklyResult
	if err = db.GetIns().First(&result, "week_start=? AND `group`=?", week, match.Group).Error; err != nil {
		t.Fatal(err)
	}
	var awards []p13UFWeeklyAward
	if json.Unmarshal(result.Awards, &awards) != nil || len(awards) != 2 {
		t.Fatal("actual weekly participants", awards)
	}
	source := "p13-week:" + week + ":" + match.Group
	if p1Count(t, &db.RewardDelivery{}, "source_key=?", source) != 2 {
		t.Fatal("once per actual participant")
	}
	for _, award := range awards {
		messages, err := f.s.RewardCenter(f.ctx, award.PlayerID, &pb.CSQueryUserReward{})
		if err != nil {
			t.Fatal(err)
		}
		var rewardIDs []int64
		for _, item := range messages[0].(*pb.SCQueryUserReward).GetRewardItemInfoList() {
			rewardIDs = append(rewardIDs, item.GetRewardId())
		}
		before := p13WeeklyBadgeCount(t, award.PlayerID)
		if _, err = f.s.RewardCenter(f.ctx, award.PlayerID, &pb.CSAddRewardsToPlayer{RewardIds: rewardIDs}); err != nil {
			t.Fatal(err)
		}
		if _, err = f.s.RewardCenter(f.ctx, award.PlayerID, &pb.CSAddRewardsToPlayer{RewardIds: rewardIDs}); err != nil {
			t.Fatal(err)
		}
		want := int64(4)
		if award.Rank == 1 {
			want = 8
		}
		if p13WeeklyBadgeCount(t, award.PlayerID)-before != want {
			t.Fatal("weekly frozen quantity or duplicate claim", award)
		}
	}
}
func p13WeeklyBadgeCount(t *testing.T, id int64) int64 {
	t.Helper()
	var profile db.Profile
	if err := db.GetIns().First(&profile, id).Error; err != nil {
		t.Fatal(err)
	}
	_, game, err := decodeProfile(&profile)
	if err != nil {
		t.Fatal(err)
	}
	var total int64
	for _, item := range game.Items {
		if item.GetId() == "item_311" {
			total += int64(item.GetNum())
		}
	}
	return total
}
