package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func localPlayActivitiesFixture(t *testing.T) (*p1Fixture, int64) {
	t.Helper()
	f, id := p3DemoProviderFixture(t)
	f.id = id
	// 轮盘原奖励可包含真实精灵；沿产品启动顺序先加载原P9表并建实例域。
	if err := f.s.LoadP9Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	var prior db.LocalPlayPolicy
	existed := db.GetIns().Migrator().HasTable(&prior) && db.GetIns().First(&prior, 1).Error == nil
	t.Cleanup(func() {
		if existed {
			if err := db.GetIns().Save(&prior).Error; err != nil {
				t.Error(err)
			}
		} else if db.GetIns().Migrator().HasTable(&prior) {
			if err := db.GetIns().Delete(&db.LocalPlayPolicy{}, 1).Error; err != nil {
				t.Error(err)
			}
		}
	})
	for _, init := range []func(context.Context) error{f.s.InitializeLocalPlay, f.s.InitializeLocalPlayActivities, f.s.InitializeP9Pets, f.s.InitializeTurntable, f.s.InitializeOpenServerActivity, f.s.InitializeP4, f.s.InitializeP4CopyBenefits, f.s.InitializeMonthSign} {
		if err := init(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.s.seedP4Rotations(db.GetIns().WithContext(f.ctx), time.Now()); err != nil {
		t.Fatal(err)
	}
	f.s.LocalPlayToolsEnabled = true
	if err := db.GetIns().Model(&db.LocalPlayPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	var owner db.Profile
	if err := db.GetIns().First(&owner, id).Error; err != nil {
		t.Fatal(err)
	}
	var err error
	f.ctx, err = f.s.BindLocalPlayContext(f.ctx, &owner)
	if err != nil {
		t.Fatal(err)
	}
	return f, id
}

func localPlayCycleID(t *testing.T, f *p1Fixture, id int64, key string) string {
	t.Helper()
	var binding db.LocalPlayActivityBinding
	if err := db.GetIns().First(&binding, "player_id=? AND activity_id=?", id, key).Error; err != nil {
		t.Fatal(err)
	}
	return binding.CycleID
}

func localPlayNonRedWheelSource(t *testing.T) {
	t.Helper()
	var prior db.KingRecruitRules
	if err := db.GetIns().First(&prior, 1).Error; err != nil {
		t.Fatal(err)
	}
	var pool db.GameplayPool
	if err := db.GetIns().First(&pool, "id=?", "turntable_20261011").Error; err != nil || pool.MinLevel != 1 {
		t.Fatal("original nonred level1 pool absent", err)
	}
	if err := db.GetIns().Model(&db.KingRecruitRules{}).Where("id=1").Update("wheel_pool", pool.ID).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.GetIns().Save(&prior).Error; err != nil {
			t.Error(err)
		}
	})
}

func TestLocalPlayActivitiesPrivateCyclesOldReceiptsAndOriginal249TaskConsumer(t *testing.T) {
	f, id := localPlayActivitiesFixture(t)
	before, _ := f.profile(t)
	if _, err := f.s.LocalPlayActivitiesApply(context.Background(), id, "open", "all", "unauth"); err == nil {
		t.Fatal("unbound context opened activities")
	}
	view, err := f.s.LocalPlayActivitiesApply(f.ctx, id, "open", "all", "00000000-0000-4000-8000-000000000001")
	if err != nil {
		t.Fatal(err)
	}
	if !view.TestOnly || len(view.Activities) != 36 || len(view.Results) != 36 {
		t.Fatal("full truthful catalogue", view)
	}
	for _, key := range []string{"PrizeTask", "PrizeBoss", "BattleRace", "RobTrea"} {
		for _, item := range view.Activities {
			if item.ID == key && (item.Status != "unsupported" || item.CanOpen || item.CanRefresh) {
				t.Fatal("pending gameplay was declared playable", item)
			}
		}
	}
	firstCycle := localPlayCycleID(t, f, id, "OpenHappy")
	firstWheel := localPlayCycleID(t, f, id, "BigTurnTable")
	old, err := f.s.LocalPlayActivitiesApply(f.ctx, id, "open", "all", "00000000-0000-4000-8000-000000000001")
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(view)
	b, _ := json.Marshal(old)
	if string(a) != string(b) {
		t.Fatal("administrative retry changed result")
	}
	if _, err = f.s.LocalPlayActivitiesApply(f.ctx, id, "refresh", "all", "00000000-0000-4000-8000-000000000001"); err == nil {
		t.Fatal("request ID action conflict accepted")
	}
	if _, err = f.s.openServerActivity(f.ctx, id, &pb.CSGetOpenServerReward{ItemId: proto.Int32(1)}, 101); err == nil {
		t.Fatal("opening a period fabricated a login event")
	}
	if err := db.GetIns().WithContext(f.ctx).Transaction(func(tx *gorm.DB) error { return RecordOpenServerLogin(tx, id) }); err != nil {
		t.Fatal(err)
	}
	claim, err := f.s.openServerActivity(f.ctx, id, &pb.CSGetOpenServerReward{ItemId: proto.Int32(1)}, 101)
	if err != nil {
		t.Fatal(err)
	}
	gain := claim[0].(*pb.SCGetOpenServerReward).GetGet()
	if len(gain.Simps) != 2 || gain.Simps[0].GetId() != "diamond" || gain.Simps[0].GetNum() != 100 {
		t.Fatal("original login reward", gain)
	}
	after, _ := f.profile(t)
	if after.GetDiamond() != before.GetDiamond()+100 {
		t.Fatal("original reward did not commit")
	}
	if _, err = f.s.LocalPlayActivitiesApply(f.ctx, id, "refresh", "all", "00000000-0000-4000-8000-000000000002"); err != nil {
		t.Fatal(err)
	}
	if localPlayCycleID(t, f, id, "OpenHappy") == firstCycle || localPlayCycleID(t, f, id, "BigTurnTable") == firstWheel {
		t.Fatal("refresh did not create independent cycles")
	}
	replay, err := f.s.openServerActivity(f.ctx, id, &pb.CSGetOpenServerReward{ItemId: proto.Int32(1)}, 101)
	if err != nil || !proto.Equal(replay[0], claim[0]) {
		t.Fatal("old original stamp lost receipt", err)
	}
	current, _ := f.profile(t)
	if current.GetDiamond() != after.GetDiamond() {
		t.Fatal("old receipt granted twice")
	}
	if _, err = f.s.openServerActivity(f.ctx, id, &pb.CSGetOpenServerReward{ItemId: proto.Int32(1)}, 102); err == nil {
		t.Fatal("new cycle inherited prior login qualification")
	}
	if err := db.GetIns().WithContext(f.ctx).Transaction(func(tx *gorm.DB) error { return RecordOpenServerLogin(tx, id) }); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.openServerActivity(f.ctx, id, &pb.CSGetOpenServerReward{ItemId: proto.Int32(1)}, 102); err != nil {
		t.Fatal(err)
	}
	if p1Count(t, &db.LocalPlayOpenServerReceipt{}, "player_id=?", id) != 2 || p1Count(t, &db.OpenServerActivityReceipt{}, "player_id=?", id) != 0 || p1Count(t, &db.OpenServerActivityState{}, "player_id=?", id) != 0 {
		t.Fatal("private cycles changed legacy storage")
	}
	f.restart(t)
	if _, err = f.s.LocalPlayActivitiesApply(f.ctx, id, "refresh", "all", "00000000-0000-4000-8000-000000000002"); err != nil {
		t.Fatal(err)
	}
	replay, err = f.s.openServerActivity(f.ctx, id, &pb.CSGetOpenServerReward{ItemId: proto.Int32(1)}, 101)
	if err != nil || !proto.Equal(replay[0], claim[0]) {
		t.Fatal("cold original receipt lost", err)
	}
}

func TestLocalPlayActivitiesWheelFreshCycleRetainsOriginalOldStamp(t *testing.T) {
	f, id := localPlayActivitiesFixture(t)
	localPlayNonRedWheelSource(t)
	if _, err := f.s.LocalPlayActivitiesApply(f.ctx, id, "open", "BigTurnTable", "00000000-0000-4000-8000-000000000003"); err != nil {
		t.Fatal(err)
	}
	first := localPlayCycleID(t, f, id, "BigTurnTable")
	request := &pb.CSPlayBigTurn{Count: proto.Int32(1)}
	draw, err := f.s.playBigTurn(f.ctx, id, request, 9001)
	if err != nil {
		t.Fatal(err)
	}
	before, gameBefore := f.profile(t)
	if _, err = f.s.LocalPlayActivitiesApply(f.ctx, id, "refresh", "BigTurnTable", "00000000-0000-4000-8000-000000000004"); err != nil {
		t.Fatal(err)
	}
	second := localPlayCycleID(t, f, id, "BigTurnTable")
	if second == first {
		t.Fatal("wheel reused old period")
	}
	replay, err := f.s.playBigTurn(f.ctx, id, request, 9001)
	if err != nil || !proto.Equal(draw[0], replay[0]) {
		t.Fatal("wheel old frame did not recover", err)
	}
	after, gameAfter := f.profile(t)
	a, _ := json.Marshal(gameBefore)
	b, _ := json.Marshal(gameAfter)
	if !proto.Equal(before, after) || string(a) != string(b) {
		t.Fatal("retired wheel stamp awarded twice")
	}
	if _, err = f.s.playBigTurn(f.ctx, id, request, 9002); err != nil {
		t.Fatal(err)
	}
	if p1Count(t, &db.TurntableState{}, "player_id=? AND pool_id IN ?", id, []string{first, second}) != 2 {
		t.Fatal("draw did not reach both independent original pools")
	}
	if _, err = f.s.playBigTurn(f.ctx, id, &pb.CSPlayBigTurn{Count: proto.Int32(10)}, 9001); err == nil {
		t.Fatal("cross-cycle stamp changed request")
	}
}

func TestLocalPlayActivitiesOriginalMonthSignNewPeriodAndOldStamp(t *testing.T) {
	f, id := localPlayActivitiesFixture(t)
	if _, err := f.s.LocalPlayActivitiesApply(f.ctx, id, "open", "EveryDaySign", "00000000-0000-4000-8000-000000000011"); err != nil {
		t.Fatal(err)
	}
	request := &pb.CSReceiveMonthSign{}
	first, err := f.s.monthSignAt(f.ctx, id, request, LocalPlayNow(f.ctx), 7001)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := f.profile(t)
	if _, err := f.s.LocalPlayActivitiesApply(f.ctx, id, "refresh", "EveryDaySign", "00000000-0000-4000-8000-000000000012"); err != nil {
		t.Fatal(err)
	}
	again, err := f.s.monthSignAt(f.ctx, id, request, LocalPlayNow(f.ctx), 7001)
	if err != nil || !proto.Equal(first[0], again[0]) {
		t.Fatal("retired month sign frame", err)
	}
	after, _ := f.profile(t)
	if !proto.Equal(before, after) {
		t.Fatal("old month sign frame paid twice")
	}
	if _, err := f.s.monthSignAt(f.ctx, id, request, LocalPlayNow(f.ctx), 7002); err != nil {
		t.Fatal(err)
	}
	current, _ := f.profile(t)
	if current.GetCoin()-after.GetCoin() != 1000 {
		t.Fatal("fresh original day1 reward", current.GetCoin()-after.GetCoin())
	}
	if p1Count(t, &db.MonthSignState{}, "player_id=?", id) != 0 {
		t.Fatal("private sign reset legacy month storage")
	}
	f.restart(t)
	cold, err := f.s.monthSignAt(f.ctx, id, request, LocalPlayNow(f.ctx), 7001)
	if err != nil || !proto.Equal(cold[0], first[0]) {
		t.Fatal("cold private month receipt", err)
	}
}

func TestLocalPlayActivitiesOwnerIsolationAndPastCalendarWorker(t *testing.T) {
	f, id := localPlayActivitiesFixture(t)
	// Explicit tool-clock fixture; no wallet, inventory, player or battle facts.
	if err := db.GetIns().Create(&db.LocalPlayClockState{PlayerID: id, OffsetDays: -40, Revision: 1}).Error; err != nil {
		t.Fatal(err)
	}
	before, _ := f.profile(t)
	if _, err := f.s.LocalPlayActivitiesApply(f.ctx, id, "open", "all", "00000000-0000-4000-8000-000000000021"); err != nil {
		t.Fatal(err)
	}
	var wheel db.LocalPlayActivityCycle
	if err := db.GetIns().First(&wheel, "id=?", localPlayCycleID(t, f, id, "BigTurnTable")).Error; err != nil {
		t.Fatal(err)
	}
	if wheel.EndAt >= time.Now().UnixMilli() {
		t.Fatal("fixture did not create a period in the past wall calendar")
	}
	if delta := time.Now().AddDate(0, 0, -40).UnixMilli() - wheel.CreatedAt; delta < -5000 || delta > 5000 {
		t.Fatal("apply used stale bound clock", delta)
	}
	if err := f.s.SettleTurntable(context.Background()); err != nil {
		t.Fatal(err)
	}
	var global db.TurntableGlobal
	if err := db.GetIns().First(&global, "pool_id=?", wheel.ID).Error; err != nil || global.Settled {
		t.Fatal("wall-clock worker prematurely settled a private period", err)
	}
	allowed, err := localPlayActivityAccess(db.GetIns().WithContext(context.Background()), wheel.ID, false)
	if err != nil || allowed {
		t.Fatal("unbound private cycle access", allowed, err)
	}
	for _, key := range []string{"FirstRecharge", "MonthCard", "VIPBenefits", "OpenSeverMoney"} {
		var cycle db.LocalPlayActivityCycle
		if err := db.GetIns().First(&cycle, "id=?", localPlayCycleID(t, f, id, key)).Error; err != nil {
			t.Fatal(err)
		}
		var snapshot localPlayScopedSnapshot
		if json.Unmarshal(cycle.Snapshot, &snapshot) != nil || !snapshot.EntryOnly {
			t.Fatal("lifetime entry reset paid eligibility", key)
		}
	}
	after, _ := f.profile(t)
	if !proto.Equal(before, after) {
		t.Fatal("opening tools synthesized ordinary profile resources")
	}
}

func TestLocalPlayActivitiesRealSameDateLoginAfterRefresh(t *testing.T) {
	f, id := localPlayActivitiesFixture(t)
	if err := f.s.RecordP1Login(f.ctx, id); err != nil {
		t.Fatal(err)
	}
	claimCurrent := func(expectReady bool) {
		t.Helper()
		var cycle db.LocalPlayActivityCycle
		if err := db.GetIns().First(&cycle, "id=?", localPlayCycleID(t, f, id, "LoginAward")).Error; err != nil {
			t.Fatal(err)
		}
		var snapshot localPlayScopedSnapshot
		if err := json.Unmarshal(cycle.Snapshot, &snapshot); err != nil {
			t.Fatal(err)
		}
		key := ""
		for _, period := range snapshot.Periods {
			if period.Goal == 1 {
				key = period.ID
			}
		}
		if key == "" {
			t.Fatal("original one-login source absent")
		}
		info, err := f.s.P1Activity(f.ctx, id, &pb.CSActivityInfo{ActivityId: proto.String(key)})
		if err != nil {
			t.Fatal(err)
		}
		item := info[0].(*pb.SCActivityInfo).GetItemList()[0]
		if expectReady != (item.GetStatus() == 1) {
			t.Fatal("actual private login qualification", item)
		}
		if expectReady {
			before, _ := f.profile(t)
			if _, err := f.s.P1Activity(f.ctx, id, &pb.CSActivityReward{ActivityId: proto.String(key), Index: proto.Int32(1), GetResources: item.GetRewardList()}); err != nil {
				t.Fatal(err)
			}
			after, _ := f.profile(t)
			if after.GetCoin()-before.GetCoin() != 1000 {
				t.Fatal("original login source reward was not consumed")
			}
		}
	}
	for _, key := range []string{"00000000-0000-4000-8000-000000000031", "00000000-0000-4000-8000-000000000032"} {
		if _, err := f.s.LocalPlayActivitiesApply(f.ctx, id, "refresh", "LoginAward", key); err != nil {
			t.Fatal(err)
		}
		claimCurrent(false)
		if err := f.s.RecordP1Login(f.ctx, id); err != nil {
			t.Fatal(err)
		}
		if err := f.s.RecordP1Login(f.ctx, id); err != nil {
			t.Fatal(err)
		}
		claimCurrent(true)
		var counter db.PeriodProgress
		var cycle db.LocalPlayActivityCycle
		if err := db.GetIns().First(&cycle, "id=?", localPlayCycleID(t, f, id, "LoginAward")).Error; err != nil {
			t.Fatal(err)
		}
		var snapshot localPlayScopedSnapshot
		if err := json.Unmarshal(cycle.Snapshot, &snapshot); err != nil {
			t.Fatal(err)
		}
		for _, period := range snapshot.Periods {
			counter = db.PeriodProgress{}
			if err := db.GetIns().First(&counter, "player_id=? AND instance_id=?", id, period.ID).Error; err != nil {
				t.Fatal(err)
			}
			if counter.Value != 1 {
				t.Fatal("same-date reconnect doubled private progress", counter)
			}
		}
	}
}

func TestLocalPlayActivitiesRewindKeepsPreRefreshPaymentAndEventHistory(t *testing.T) {
	f, id := localPlayActivitiesFixture(t)
	localPlayNonRedWheelSource(t)
	f.s.P3AutoConfirmDemoPayments = true
	clock := func(action, key string) {
		t.Helper()
		if _, err := f.s.LocalPlayTimeChange(f.ctx, id, action, key); err != nil {
			t.Fatal(err)
		}
		var owner db.Profile
		if err := db.GetIns().First(&owner, id).Error; err != nil {
			t.Fatal(err)
		}
		var err error
		f.ctx, err = f.s.BindLocalPlayContext(f.ctx, &owner)
		if err != nil {
			t.Fatal(err)
		}
	}
	pay := func(stamp int64) {
		t.Helper()
		if _, err := f.s.generatePayment(f.ctx, id, p3DemoProviderRequest(t, "djsy.wuyou.andp0006"), stamp, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	draw := func(first int64) {
		t.Helper()
		for _, stamp := range []int64{first, first + 1} {
			result, err := f.s.playBigTurn(f.ctx, id, &pb.CSPlayBigTurn{Count: proto.Int32(1)}, stamp)
			if err != nil {
				t.Fatal(err)
			}
			if stamp == first+1 {
				spent := int32(0)
				for _, cost := range result[0].(*pb.SCPlayBigTurn).GetCost() {
					if cost.GetId() == "diamond" {
						spent += cost.GetNum()
					}
				}
				if spent != 50 {
					t.Fatal("original paid draw did not consume50", spent)
				}
			}
		}
	}
	copyFact := func(source string) {
		t.Helper()
		// Finite trusted settlement-producer fixture. This unit test does not
		// claim a native PVE win; payment, draw and login use real domain actions.
		if err := db.GetIns().WithContext(f.ctx).Transaction(func(tx *gorm.DB) error { return f.s.RecordP1Event(tx, id, source, "copy_win", 10, LocalPlayNow(f.ctx)) }); err != nil {
			t.Fatal(err)
		}
	}
	clock("nextDay", "00000000-0000-4000-8000-000000000041")
	p3DemoProviderFund(t, f, id, "rewind-source-funded-local-wallet", 5000)
	pay(91001)
	draw(91011)
	if err := f.s.RecordP1Login(f.ctx, id); err != nil {
		t.Fatal(err)
	}
	copyFact("finite-settled-copy-before-refresh")
	clock("previousDay", "00000000-0000-4000-8000-000000000042")
	if _, err := f.s.LocalPlayActivitiesApply(f.ctx, id, "refresh", "all", "00000000-0000-4000-8000-000000000043"); err != nil {
		t.Fatal(err)
	}
	// Returning to the old future date must not import its old actual actions.
	clock("nextDay", "00000000-0000-4000-8000-000000000044")
	metric := func(activity string) int64 {
		t.Helper()
		var cycle db.LocalPlayActivityCycle
		if err := db.GetIns().First(&cycle, "id=?", localPlayCycleID(t, f, id, activity)).Error; err != nil {
			t.Fatal(err)
		}
		var snapshot localPlayScopedSnapshot
		if json.Unmarshal(cycle.Snapshot, &snapshot) != nil || len(snapshot.Payments) != 1 {
			t.Fatal("payment snapshot")
		}
		value, err := p3ActivityMetric(db.GetIns().WithContext(f.ctx), id, &snapshot.Payments[0], LocalPlayNow(f.ctx))
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	readCopy := func() int32 {
		t.Helper()
		var cycle db.LocalPlayActivityCycle
		if err := db.GetIns().First(&cycle, "id=?", localPlayCycleID(t, f, id, "MainCopy")).Error; err != nil {
			t.Fatal(err)
		}
		var snapshot localPlayScopedSnapshot
		if json.Unmarshal(cycle.Snapshot, &snapshot) != nil || len(snapshot.Periods) != 1 {
			t.Fatal("copy snapshot")
		}
		info, err := f.s.P1Activity(f.ctx, id, &pb.CSActivityInfo{ActivityId: proto.String(snapshot.Periods[0].ID)})
		if err != nil {
			t.Fatal(err)
		}
		return info[0].(*pb.SCActivityInfo).GetItemList()[0].GetLeft()
	}
	readLogin := func() int32 {
		t.Helper()
		info, err := f.s.p4Welfare(f.ctx, id, &pb.CSLoginRewardInfo{}, 0, LocalPlayNow(f.ctx))
		if err != nil {
			t.Fatal(err)
		}
		return info[0].(*pb.SCLoginRewardInfo).GetHasReward()
	}
	readRebate := func() int32 {
		t.Helper()
		key := "consumeRet_p4_" + localPlayCycleID(t, f, id, "CostReturn")
		info, err := f.s.p4Rebate(f.ctx, id, &pb.CSActivityInfo{ActivityId: proto.String(key)}, 0, LocalPlayNow(f.ctx))
		if err != nil {
			t.Fatal(err)
		}
		return info[0].(*pb.SCActivityInfo).GetItemList()[0].GetLeft()
	}
	// The original legacy calendar's already earned day is delivered first;
	// it is separate from the fresh private calendar's zero qualification.
	if readLogin() != 1 {
		t.Fatal("old earned legacy login day lost its no-ID entrance")
	}
	if _, err := f.s.p4Welfare(f.ctx, id, &pb.CSGetLoginReward{}, 92001, LocalPlayNow(f.ctx)); err != nil {
		t.Fatal(err)
	}
	if metric("RechargeTotal") != 0 || metric("CostTotal") != 0 || readCopy() != 0 || readLogin() != 0 || readRebate() != 0 {
		t.Fatal("refresh reused pre-refresh real actions after clock rewind")
	}
	pay(91002)
	draw(91013)
	if err := f.s.RecordP1Login(f.ctx, id); err != nil {
		t.Fatal(err)
	}
	copyFact("finite-settled-copy-after-refresh")
	copyFact("finite-settled-copy-after-refresh")
	if metric("RechargeTotal") != 600 || metric("CostTotal") != 50 || readCopy() != 10 || readLogin() != 1 || readRebate() != 5 {
		t.Fatal("new real actions did not qualify exactly once")
	}
	f.restart(t)
	if metric("RechargeTotal") != 600 || metric("CostTotal") != 50 || readCopy() != 10 || readLogin() != 1 || readRebate() != 5 {
		t.Fatal("cold recovery changed refresh history boundary")
	}
}
