package service

import (
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"sync"
	"testing"
	"time"
)

func TestCopyResetOriginalVIPAllTiersAndSharedDay(t *testing.T) {
	f := p4Setup(t)
	if e := f.s.InitializeP5(f.ctx); e != nil {
		t.Fatal(e)
	}
	if e := f.s.InitializeDailyCopy(f.ctx); e != nil {
		t.Fatal(e)
	}
	if e := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(100)
		p.Vip = proto.Int32(12)
		p.Diamond = proto.Int64(1000)
		g.Run = nil
		g.Copy.PassedCopyInfo = nil
		for _, cfg := range f.s.Tables["CopyMission"] {
			g.Copy.PassedCopyInfo = append(g.Copy.PassedCopyInfo, &pb.CopySimpleInfo{ChapterId: proto.String(cfg["ChapterId"]), MissionId: proto.String(cfg["MissionId"]), Star: proto.Int32(5), ChallengeTimes: proto.Int32(3)})
		}
		g.CopyChallengeDay = p1Day(time.Now())
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	for i, price := range []int32{20, 30, 50, 100} {
		// Different targets share the conservative original VIP daily quota.
		mission := []string{"5", "7", "9", "11"}[i]
		chapter := f.s.table("CopyMission", "MissionId", mission)["ChapterId"]
		info, e := f.s.copyGetResetInfo(f.ctx, f.id, &pb.CSCopyGetResetInfo{ChapterId: proto.String(chapter), MissionId: proto.String(mission)})
		if e != nil {
			t.Fatal(e)
		}
		quote := info[0].(*pb.SCCopyGetResetInfo)
		if quote.GetRemainResetCount() != int32(4-i) || quote.GetCost()[0].GetNum() != price {
			t.Fatal("original VIP12/order price", quote)
		}
		request := &pb.CSCopyResetChallengeCount{ChapterId: proto.String(chapter), MissionId: proto.String(mission), Cost: quote.GetCost()[0]}
		bad := proto.Clone(request).(*pb.CSCopyResetChallengeCount)
		bad.Cost.Num = proto.Int32(1)
		if _, e = f.s.copyResetChallenge(f.ctx, f.id, bad, int64(i+1)); e == nil {
			t.Fatal("fake quote")
		}
		var wg sync.WaitGroup
		errs := make(chan error, 8)
		for n := 0; n < 8; n++ {
			wg.Add(1)
			go func() { defer wg.Done(); _, e := f.s.copyResetChallenge(f.ctx, f.id, request, int64(i+1)); errs <- e }()
		}
		wg.Wait()
		close(errs)
		for e := range errs {
			if e != nil {
				t.Fatal(e)
			}
		}
		p, _ := f.profile(t)
		expected := int64(1000)
		for j := 0; j <= i; j++ {
			expected -= []int64{20, 30, 50, 100}[j]
		}
		if p.GetDiamond() != expected {
			t.Fatal("eight duplicate cost")
		}
	}
	// Original callback always indexes cost[0], even when remain count becomes zero.
	info, e := f.s.copyGetResetInfo(f.ctx, f.id, &pb.CSCopyGetResetInfo{ChapterId: proto.String("1"), MissionId: proto.String("5")})
	if e != nil {
		t.Fatal(e)
	}
	quote := info[0].(*pb.SCCopyGetResetInfo)
	if quote.GetRemainResetCount() != 0 || len(quote.GetCost()) != 1 || quote.GetCost()[0].GetNum() != 100 {
		t.Fatal("zero-quota client cost index")
	}
	if e = db.GetIns().Model(&db.CopyResetRule{}).Where("id=?", 1).Update("scope", constUtil.CopyResetMissionDay).Error; e != nil {
		t.Fatal(e)
	}
	var daily db.CopyResetDaily
	if e = db.GetIns().First(&daily, "player_id=? AND day=?", f.id, p1Day(time.Now())).Error; e != nil {
		t.Fatal(e)
	}
	var state copyResetProgress
	if json.Unmarshal(daily.Payload, &state) != nil || state.Input.Scope != constUtil.CopyResetPlayerDay || state.Used["global"] != 4 {
		t.Fatal("daily frozen scope")
	}
	p, g := f.profile(t)
	// Only this domain's quote clock advances: next-day prices/scope initialize from actual rules.
	if e = db.GetIns().Transaction(func(tx *gorm.DB) error {
		_, fresh, _, remain, cost, e := f.s.copyResetQuote(tx, f.id, p, "1", "5", time.Now().AddDate(0, 0, 1))
		if e == nil && (fresh.Input.Scope != constUtil.CopyResetMissionDay || remain != 4 || cost.GetNum() != 20) {
			t.Fatal("new day")
		}
		return e
	}); e != nil {
		t.Fatal(e)
	}
	_ = g
	if p1Count(t, &db.EventFact{}, "player_id=? AND kind=?", f.id, constUtil.P1EventDiamondSpent) != 4 {
		t.Fatal("one actual fact per real reset")
	}
	t.Log("all4 original20/30/50/100 tiers, VIP12 shared4 across missions, 8once each, fake quote/zero-cost-index/frozen scope/new day")
}
