package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// 有限五星/体力前提隔离日限回归，真实副本来源沿独立原机链验收。
func TestLocalPlayCopyCalendarResetQuoteRewindAndCold(t *testing.T) {
	f, id, owner := localClockFixture(t)
	for _, init := range []func(context.Context) error{f.s.InitializeP5, f.s.InitializeDailyCopy, f.s.InitializeCopyDrops, f.s.InitializeP4CopyBenefits, f.s.InitCooking} {
		if err := init(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err := updateProfile(f.ctx, id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(100)
		p.Vip = proto.Int32(12)
		p.Diamond = proto.Int64(1000)
		p.Stamina = proto.Int32(1000)
		g.Run = nil
		f.s.refreshCopyProgress(g)
		for _, info := range g.Copy.PassedCopyInfo {
			info.Star = proto.Int32(5)
			info.ChallengeTimes = proto.Int32(3)
		}
		g.CopyChallengeDay = p1Day(time.Now())
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	localClockEnable(t)
	f.ctx = context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	ctx := localClockBind(t, f, owner)
	mission := f.s.table("CopyMission", "MissionId", "5")
	infoRequest := &pb.CSCopyGetResetInfo{ChapterId: proto.String(mission["ChapterId"]), MissionId: proto.String("5")}
	quote := func() *pb.SCCopyGetResetInfo {
		t.Helper()
		out, err := f.s.copyGetResetInfo(ctx, id, infoRequest)
		if err != nil {
			t.Fatal(err)
		}
		return out[0].(*pb.SCCopyGetResetInfo)
	}
	reset := func(stamp int64) {
		t.Helper()
		q := quote()
		b := &pb.CSCopyResetChallengeCount{ChapterId: infoRequest.ChapterId, MissionId: infoRequest.MissionId, Cost: q.Cost[0]}
		if _, err := f.s.copyResetChallenge(ctx, id, b, stamp); err != nil {
			t.Fatal(err)
		}
	}
	if q := quote(); q.GetRemainResetCount() != 4 || q.Cost[0].GetNum() != 20 {
		t.Fatal("initial reset quote", q)
	}
	reset(919001)
	if q := quote(); q.GetRemainResetCount() != 3 || q.Cost[0].GetNum() != 30 {
		t.Fatal("committed reset quota", q)
	}
	shift := func(action string, n int) {
		t.Helper()
		if _, err := f.s.LocalPlayTimeChange(ctx, id, action, fmt.Sprintf("copy-day-%d", n)); err != nil {
			t.Fatal(err)
		}
		ctx = localClockBind(t, f, owner)
		if _, err := f.s.copyPassedInfo(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	shift("nextDay", 1)
	if q := quote(); q.GetRemainResetCount() != 4 || q.Cost[0].GetNum() != 20 {
		t.Fatal("next personal date did not restore reset allowance", q)
	}
	_, game := f.profile(t)
	for _, info := range game.Copy.PassedCopyInfo {
		if info.GetChallengeTimes() != 0 {
			t.Fatal("next personal date retained yesterday challenge count", info)
		}
	}
	// 第二天实际扫荡一包；回拨后不能把该笔挑战清空。
	sweep := &pb.CSCopySweep{ChapterId: proto.String("1"), MissionId: proto.String("1")}
	out, err := f.s.copySweep(ctx, id, sweep, 919002)
	if err != nil {
		t.Fatal(err)
	}
	settled := out[0].(*pb.SCCopySweep)
	afterSweep, _ := f.profile(t)
	shift("previousDay", 2)
	if q := quote(); q.GetRemainResetCount() != 3 || q.Cost[0].GetNum() != 30 {
		t.Fatal("rewind erased paid reset history", q)
	}
	_, oldDay := f.profile(t)
	for _, info := range oldDay.Copy.PassedCopyInfo {
		want := int32(3)
		if info.GetMissionId() == "5" {
			want = 0
		}
		if info.GetChallengeTimes() != want {
			t.Fatal("rewind did not restore exact original date quota", info)
		}
	}
	shift("nextDay", 3)
	f.restart(t)
	ctx = localClockBind(t, f, owner)
	retry, err := f.s.copySweep(ctx, id, sweep, 919002)
	replayed, saved := f.profile(t)
	if err != nil || !proto.Equal(settled, retry[0]) || !proto.Equal(afterSweep, replayed) {
		t.Fatal("cold replay after clock changes paid or rewarded twice", err)
	}
	for _, info := range saved.Copy.PassedCopyInfo {
		if info.GetMissionId() == "1" && info.GetChallengeTimes() != 1 {
			t.Fatal("next-date challenge count reset by rewind/revisit", info)
		}
	}
}
