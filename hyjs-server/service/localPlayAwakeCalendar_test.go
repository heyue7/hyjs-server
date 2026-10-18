package service

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// 原已捕获001/003/守护002队伍有限前提沿既有fixture；普通钱包和库存不补。
// 工具只授权临库demo身份，初始化实际LocalPlay两域，不关闭消费者来绕缺表。
func localAwakeCalendarFixture(t *testing.T) (*p1Fixture, *db.Profile) {
	t.Helper()
	f := p7AwakeBusinessFixture(t)
	for _, initialize := range []func(context.Context) error{f.s.InitializeLocalPlay, f.s.InitializeLocalPlayActivities} {
		if err := initialize(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.GetIns().Model(&db.Profile{}).Where("id=?", f.id).Update("uid", fmt.Sprintf("demo-awake-calendar-%d", f.id)).Error; err != nil {
		t.Fatal(err)
	}
	var owner db.Profile
	if err := db.GetIns().First(&owner, f.id).Error; err != nil {
		t.Fatal(err)
	}
	f.s.LocalPlayToolsEnabled = true
	f.ctx = context.WithValue(f.ctx, constUtil.LocalWalletAPIContextKey, true)
	f.ctx = context.WithValue(f.ctx, constUtil.LocalWalletSessionContextKey, append([]byte(nil), owner.Session...))
	return f, &owner
}

func TestLocalPlayAwakeCalendarWeekBudgetLegacyAndCold(t *testing.T) {
	f, owner := localAwakeCalendarFixture(t)
	base := f.ctx
	real := time.Date(2026, 10, 16, 12, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
	localClockEnable(t)
	ctx := localClockBind(t, f, owner)
	change := func(action string, key string) {
		t.Helper()
		if _, err := f.s.LocalPlayTimeChange(ctx, f.id, action, key); err != nil {
			t.Fatal(err)
		}
		ctx = localClockBind(t, f, owner)
	}
	catalogue := func() *pb.SCAwakeCopyData {
		t.Helper()
		replies, err := f.s.p7AwakeCatalogue(ctx, f.id, real)
		if err != nil {
			t.Fatal(err)
		}
		return replies[0].(*pb.SCAwakeCopyData)
	}
	budget, err := f.s.p7ExpeditionSetting("awakeCopyPlayTimes")
	if err != nil || budget != 2 {
		t.Fatal("original daily awake budget", budget, err)
	}
	for day := 0; day < 7; day++ {
		if day > 0 {
			change("nextDay", "awake-week-"+strconv.Itoa(day))
		}
		out := catalogue()
		if out.GetLastNum() != budget || len(out.GetCopys()) != 5 {
			t.Fatal("personal awake day quota/chapter denominator", day, out)
		}
		for _, chapter := range out.GetCopys() {
			row := f.s.table("AwakeCopyChapter", "chapterId", chapter.GetChapterId())
			want, e := p7AwakeOpenDay(row["openDay"], real.AddDate(0, 0, day))
			if e != nil || (chapter.GetIsOpen() == 1) != want {
				t.Fatal("personal weekday differs from original chapter", day, chapter, e)
			}
		}
	}
	change("reset", "awake-week-reset")
	// 默认关闭后仍走原真实日期，且不会给旧Frozen补写个人日期。
	if err = db.GetIns().Model(&db.LocalPlayPolicy{}).Where("id=1").Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	closed, err := f.s.BindLocalPlayContext(base, owner)
	if err != nil || LocalPlayContextBound(closed) {
		t.Fatal("unbound legacy path", err)
	}
	legacy, err := f.s.p7AwakeExplore(closed, f.id, &pb.CSEnterAwakeCopy{CopyId: proto.String("501")}, real, 702001)
	if err != nil || legacy[0].(*pb.SCEnterAwakeCopy).GetRefreshData().GetEndTime() <= real.UnixMilli() {
		t.Fatal("legacy actual entry", err)
	}
	var legacyRun db.P7AwakeRun
	if err = db.GetIns().Where("player_id=?", f.id).First(&legacyRun).Error; err != nil {
		t.Fatal(err)
	}
	legacyFrozen, err := p7AwakeReadFrozen(&legacyRun)
	if err != nil || legacyFrozen.BusinessDay != "" || legacyFrozen.BusinessAt != 0 {
		t.Fatal("unbound entry changed legacy freeze", err)
	}
	localClockEnable(t)
	ctx = localClockBind(t, f, owner)
	change("nextDay", "awake-legacy-next")
	if got := catalogue(); got.GetLastNum() != budget || got.GetIsAutoSettled() != 0 {
		t.Fatal("personal +1 expired real pending or used tomorrow's quota", got)
	}
	var retained db.P7AwakeRun
	if err = db.GetIns().First(&retained, "id=?", legacyRun.ID).Error; err != nil || !bytes.Equal(retained.Freeze, legacyRun.Freeze) || retained.EndAt != legacyRun.EndAt || retained.StartAt != legacyRun.StartAt {
		t.Fatal("legacy frozen bytes/real deadline rewritten", err)
	}
	if day, _, e := p7AwakeScoreClock(db.GetIns().WithContext(ctx), &legacyRun, legacyFrozen, real); e != nil || day != "2026-10-16" {
		t.Fatal("legacy reward day reinterpreted from new offset", day, e)
	}
	if _, err = f.s.p7AwakeExplore(ctx, f.id, &pb.CSAwakeCopySettle{}, real, 702002); err != nil {
		t.Fatal(err)
	}
	stamp := int64(702010)
	partial := func() {
		t.Helper()
		stamp++
		if _, e := f.s.p7AwakeExplore(ctx, f.id, &pb.CSEnterAwakeCopy{CopyId: proto.String("501")}, real, stamp); e != nil {
			t.Fatal(e)
		}
		stamp++
		out, e := f.s.p7AwakeExplore(ctx, f.id, &pb.CSAwakeCopySettle{}, real, stamp)
		if e != nil || out[0].(*pb.SCAwakeCopySettle).GetScore() != 0 || len(out[0].(*pb.SCAwakeCopySettle).GetScoreRewards()) != 0 {
			t.Fatal("empty real exploration manufactured score/win", e)
		}
	}
	partial()
	partial()
	if _, err = f.s.p7AwakeExplore(ctx, f.id, &pb.CSEnterAwakeCopy{CopyId: proto.String("501")}, real, stamp+1); err == nil {
		t.Fatal("third Saturday entry ignored original budget")
	}
	f.restart(t)
	ctx = localClockBind(t, f, owner)
	if catalogue().GetLastNum() != 0 {
		t.Fatal("cold reset Saturday's committed quota")
	}
	change("previousDay", "awake-return-friday")
	if catalogue().GetLastNum() != 1 {
		t.Fatal("Friday legacy entry disappeared on revisit")
	}
	partial()
	change("previousDay", "awake-closed-thursday")
	if catalogue().GetLastNum() != budget {
		t.Fatal("unvisited Thursday quota")
	}
	if _, err = f.s.p7AwakeExplore(ctx, f.id, &pb.CSEnterAwakeCopy{CopyId: proto.String("501")}, real, stamp+2); err == nil {
		t.Fatal("closed personal weekday accepted 501")
	}
	change("nextDay", "awake-friday-again")
	if catalogue().GetLastNum() != 0 {
		t.Fatal("Friday quota reset by rewind")
	}
	change("nextDay", "awake-saturday-again")
	if catalogue().GetLastNum() != 0 {
		t.Fatal("Saturday quota reset by revisit")
	}
}

func TestLocalPlayAwakeCalendarFrozenAwardDayActualCompletionAndCold(t *testing.T) {
	f, owner := localAwakeCalendarFixture(t)
	base := f.ctx
	localClockEnable(t)
	f.ctx = localClockBind(t, f, owner)
	real := time.Now()
	change := func(action, key string) {
		t.Helper()
		if _, err := f.s.LocalPlayTimeChange(f.ctx, f.id, action, key); err != nil {
			t.Fatal(err)
		}
		f.ctx = base
		f.ctx = localClockBind(t, f, owner)
	}
	// 通过真实工具到下一周五，+1周六也开放501。真实Run不使用历史
	// now，若把入场BusinessAt误当事件时间，回拨后必触发P1未来拒绝。
	baseDays := (int(time.Friday) - int(real.In(time.FixedZone("UTC+8", 8*3600)).Weekday()) + 7) % 7
	for day := 0; day < baseDays; day++ {
		change("nextDay", "awake-align-friday-"+strconv.Itoa(day))
	}
	acceptedDay := p1Day(real.AddDate(0, 0, baseDays+1))
	completionDay := p1Day(real.AddDate(0, 0, baseDays))
	change("nextDay", "awake-earned-next")
	first := p7AwakeExploreCall(t, f, &pb.CSEnterAwakeCopy{CopyId: proto.String("501")}, real).(*pb.SCEnterAwakeCopy)
	var firstRun db.P7AwakeRun
	if err := db.GetIns().Where("player_id=?", f.id).First(&firstRun).Error; err != nil {
		t.Fatal(err)
	}
	frozen, err := p7AwakeReadFrozen(&firstRun)
	if err != nil || frozen.BusinessDay != acceptedDay || frozen.BusinessAt != real.AddDate(0, 0, baseDays+1).UnixMilli() || firstRun.StartAt != real.UnixMilli() || first.GetRefreshData().GetEndTime() != firstRun.EndAt {
		t.Fatal("personal date or real exploration deadline not frozen precisely", frozen, err)
	}
	change("previousDay", "awake-earned-back-before-end")
	resumed := p7AwakeExploreCall(t, f, &pb.CSEnterAwakeCopy{CopyId: proto.String("501")}, real.Add(time.Second)).(*pb.SCEnterAwakeCopy)
	if !proto.Equal(first, resumed) {
		t.Fatal("rewind changed paid pending before its real EndAt")
	}
	// 每轮100分均由原四箱加既有独立type16实际End产生；不写score/赢家。
	complete := func(enter *pb.SCEnterAwakeCopy, at time.Time, settleStamp int64, wantDaily bool) *pb.SCAwakeCopySettle {
		t.Helper()
		for _, box := range enter.GetBoxes() {
			p7AwakeExploreCall(t, f, &pb.CSAwakeCopyOpenBox{BoxGuid: box.Guid}, at)
		}
		_, end := p7AwakeBusinessStartEnd(t, f, enter.GetMonsters()[0].GetGuid(), at)
		if end.GetIsWinClient() != 1 {
			t.Fatal("actual finite type16 roster lost; preserve original result")
		}
		if _, e := f.s.p7AwakeFightEnd(f.ctx, f.id, end, at); e != nil {
			t.Fatal(e)
		}
		var group sync.WaitGroup
		var replies [4]*pb.SCAwakeCopySettle
		var failures [4]error
		for i := range replies {
			group.Add(1)
			go func(index int) {
				defer group.Done()
				out, e := f.s.p7AwakeExplore(f.ctx, f.id, &pb.CSAwakeCopySettle{}, at, settleStamp)
				failures[index] = e
				if len(out) > 0 {
					replies[index] = out[0].(*pb.SCAwakeCopySettle)
				}
			}(i)
		}
		group.Wait()
		for i := range replies {
			if failures[i] != nil || replies[i] == nil || !proto.Equal(replies[0], replies[i]) {
				t.Fatal("concurrent real score settlement", i, failures[i])
			}
		}
		expectedRewards := 0
		if wantDaily {
			expectedRewards = 2
		}
		if replies[0].GetScore() != 100 || len(replies[0].GetScoreRewards()) != expectedRewards {
			t.Fatal("frozen daily-score once contract", replies[0])
		}
		return replies[0]
	}
	firstSettle := complete(first, time.Now(), 703001, true)
	var firstFact db.EventFact
	if err = db.GetIns().Where("player_id=? AND kind=?", f.id, constUtil.P7AwakeCompleteDailyEvent).First(&firstFact).Error; err != nil || firstFact.SourceKey != "awake-score-day:"+acceptedDay || firstFact.Day != completionDay {
		t.Fatal("award belongs to accepted Saturday; fact belongs to actual Friday completion", firstFact, err)
	}
	before, beforeGame := f.profile(t)
	f.restart(t)
	f.ctx = base
	f.ctx = localClockBind(t, f, owner)
	replayed := p7AwakeExploreCall(t, f, &pb.CSAwakeCopySettle{}, time.Now()).(*pb.SCAwakeCopySettle)
	after, afterGame := f.profile(t)
	if !proto.Equal(replayed, firstSettle) || !proto.Equal(before, after) || !reflect.DeepEqual(beforeGame, afterGame) {
		t.Fatal("cold score receipt repeated paid inventory")
	}
	change("nextDay", "awake-earned-saturday-return")
	secondAt := time.Now()
	second := p7AwakeExploreCall(t, f, &pb.CSEnterAwakeCopy{CopyId: proto.String("501")}, secondAt).(*pb.SCEnterAwakeCopy)
	complete(second, secondAt, 703002, false)
	change("previousDay", "awake-earned-friday-return")
	thirdAt := time.Now()
	third := p7AwakeExploreCall(t, f, &pb.CSEnterAwakeCopy{CopyId: proto.String("501")}, thirdAt).(*pb.SCEnterAwakeCopy)
	complete(third, thirdAt, 703003, true)
	if count := p1Count(t, &db.EventFact{}, "player_id=? AND kind=?", f.id, constUtil.P7AwakeCompleteDailyEvent); count != 2 {
		t.Fatal("only distinct accepted days may earn two first100 awards", count)
	}
	if count := p1Count(t, &db.EventFact{}, "player_id=? AND kind=?", f.id, constUtil.P7AwakeCompleteEvent); count != 3 {
		t.Fatal("three actual completions must retain three facts", count)
	}
	if count := p1Count(t, &db.P6BattleRecord{}, "player_id=? AND source=? AND authoritative=?", f.id, constUtil.P7AwakeBattleReplaySource, true); count != 3 {
		t.Fatal("three actual independent type16 accepted Ends required", count)
	}
	var retained db.P7AwakeRun
	if err = db.GetIns().First(&retained, "id=?", firstRun.ID).Error; err != nil || !bytes.Equal(firstRun.Freeze, retained.Freeze) || firstRun.FreezeHash != retained.FreezeHash || retained.StartAt != firstRun.StartAt || retained.EndAt != firstRun.EndAt {
		t.Fatal("current offset rewrote an old accepted run", err)
	}
	// 新字段无效必须拒绝，不能在hash校验后静默改回当前个人日。
	bad := *frozen
	bad.BusinessDay = p1Day(real.AddDate(0, 0, baseDays+2))
	if _, err = p7AwakeFrozenBusinessDay(&firstRun, &bad); err == nil {
		t.Fatal("mismatched frozen day/accepted time accepted")
	}
}
