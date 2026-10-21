package service

import (
	"context"
	"encoding/json"
	"slices"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func TestActivityDirectoryAbsentWheelAndIncompleteEntriesKeepHealthyTabs(t *testing.T) {
	f := p4Setup(t)
	if err := f.s.LoadP9Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, initialize := range []func(context.Context) error{f.s.InitializePayments, f.s.InitializeActivities, f.s.InitializeTurntable, f.s.InitializeP4CopyBenefits} {
		if err := initialize(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	var pool db.GameplayPool
	if err := db.GetIns().First(&pool, "id=?", "turntable_20261011").Error; err != nil {
		t.Fatal(err)
	}
	beforeP, _ := f.profile(t)
	list := func() *pb.SCActivityList {
		t.Helper()
		out, err := f.s.turntableActivityList(f.ctx, f.id)
		if err != nil || len(out) != 1 {
			t.Fatal("activity root directory", err)
		}
		message := out[0].(*pb.SCActivityList)
		raw, err := proto.Marshal(message)
		if err != nil {
			t.Fatal(err)
		}
		decoded := &pb.SCActivityList{}
		if err := proto.Unmarshal(raw, decoded); err != nil {
			t.Fatal(err)
		}
		return decoded
	}
	has := func(v *pb.SCActivityList, id string) bool {
		return slices.ContainsFunc(v.GetActivityList(), func(tab *pb.ActivityTab) bool { return tab.GetActivityId() == id })
	}
	if err := db.GetIns().Delete(&pool).Error; err != nil {
		t.Fatal(err)
	}
	missing := list()
	if has(missing, "bigTurnTable_20261011") || !has(missing, "openServerFund_local_20261011") || !has(missing, "monthCard_local_p3") {
		t.Fatal("missing wheel hid healthy activities", missing)
	}
	if err := db.GetIns().Create(&pool).Error; err != nil {
		t.Fatal(err)
	}
	if !has(list(), "bigTurnTable_20261011") {
		t.Fatal("healthy wheel not visible")
	}
	var rules db.TurntableRules
	if err := db.GetIns().First(&rules, "pool_id=?", pool.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Delete(&rules).Error; err != nil {
		t.Fatal(err)
	}
	if got := list(); has(got, "bigTurnTable_20261011") || !has(got, "openServerFund_local_20261011") {
		t.Fatal("missing wheel rules broke directory")
	}
	if err := db.GetIns().Create(&rules).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.GameplayPoolReward{}).Where("pool_id=? AND id=?", pool.ID, 0).Update("resource_id", "missing-resource").Error; err != nil {
		t.Fatal(err)
	}
	if got := list(); has(got, "bigTurnTable_20261011") || !has(got, "openServerFund_local_20261011") {
		t.Fatal("corrupt wheel exposed or blocked healthy activities")
	}
	if _, err := f.s.bigTurnInfo(f.ctx, f.id); err == nil {
		t.Fatal("hidden invalid wheel bypassed entry validation")
	}
	// 单条周期和兑换草稿损坏不应拖垮同屏合法活动。
	now := time.Now()
	badPeriod := db.PeriodInstance{ID: "costTotal_p4_invalid", Enabled: true, StartAt: now.Add(-time.Hour).UnixMilli(), EndAt: now.Add(time.Hour).UnixMilli(), RewardEndAt: now.Add(2 * time.Hour).UnixMilli(), Frozen: []byte("{")}
	if err := db.GetIns().Create(&badPeriod).Error; err != nil {
		t.Fatal(err)
	}
	badCommerce := db.P4CommerceRule{ID: "halfDiscountSale_p4_invalid", Kind: "half", Enabled: true, StartAt: now.Add(-time.Hour).UnixMilli(), EndAt: now.Add(time.Hour).UnixMilli(), ClaimEndAt: now.Add(time.Hour).UnixMilli(), Rules: []byte("{")}
	if err := db.GetIns().Create(&badCommerce).Error; err != nil {
		t.Fatal(err)
	}
	badSeasonal := db.P15SeasonalBundle{ID: "festival_p4_invalid", Kind: "festival", Enabled: true, StartAt: now.Add(-time.Hour).UnixMilli(), EndAt: now.Add(time.Hour).UnixMilli(), RewardEndAt: now.Add(2 * time.Hour).UnixMilli(), Children: []byte("{")}
	if err := db.GetIns().Create(&badSeasonal).Error; err != nil {
		t.Fatal(err)
	}
	got := list()
	if has(got, badPeriod.ID) || has(got, badCommerce.ID) || has(got, badSeasonal.ID) || !has(got, "openServerFund_local_20261011") || !has(got, "warriorWeightImprove") {
		t.Fatal("invalid draft or disabled rotation leaked", got)
	}
	if _, err := f.s.P1Activity(f.ctx, f.id, &pb.CSActivityReward{ActivityId: proto.String(badPeriod.ID)}); err == nil {
		t.Fatal("invalid hidden period awarded")
	}
	if _, err := f.s.p4Commerce(f.ctx, f.id, &pb.CSActivityShopInfo{ActivityId: proto.String(badCommerce.ID)}, 90001, now); err == nil {
		t.Fatal("invalid hidden shop accepted")
	}
	afterP, _ := f.profile(t)
	if !proto.Equal(beforeP, afterP) {
		t.Fatal("directory or rejected entry changed player resource state")
	}
}

func TestActivityDirectoryNoSeasonalContentAndBadRotationDoesNotBlock(t *testing.T) {
	f := p4Setup(t)
	now := time.Now()
	bad := db.P4RecruitRotation{ID: "p4:recruit:invalid", Kind: 2, Enabled: true, StartAt: now.Add(-time.Hour).UnixMilli(), EndAt: now.Add(time.Hour).UnixMilli(), Frozen: []byte("{"), Rewards: []byte("[]")}
	if err := db.GetIns().Create(&bad).Error; err != nil {
		t.Fatal(err)
	}
	tabs, err := f.s.P4ActivityTabs(f.ctx, now)
	if err != nil || !slices.ContainsFunc(tabs, func(tab *pb.ActivityTab) bool { return tab.GetActivityId() == "warriorWeightImprove" }) {
		t.Fatal("bad rotation caused root list failure or exposure", err)
	}
	// 开放完整的原强者轮换后，仅该入口恢复；无额外活动奖励写入。
	var base []db.GameplayPoolReward
	if err := db.GetIns().Where("pool_id=?", "recruit_2").Find(&base).Error; err != nil {
		t.Fatal(err)
	}
	bad.Frozen, err = json.Marshal(base)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Save(&bad).Error; err != nil {
		t.Fatal(err)
	}
	tabs, err = f.s.P4ActivityTabs(f.ctx, now)
	if err != nil || !slices.ContainsFunc(tabs, func(tab *pb.ActivityTab) bool { return tab.GetActivityId() == "warriorWeightImprove" }) {
		t.Fatal("valid operator rotation not visible", err)
	}
}

func TestActivityDirectorySeasonalMissingChildKeepsHealthySibling(t *testing.T) {
	f := p4Setup(t)
	now := time.Now()
	start, end, rewardEnd := now.Add(-time.Hour).UnixMilli(), now.Add(time.Hour).UnixMilli(), now.Add(2*time.Hour).UnixMilli()
	good := db.PeriodInstance{ID: "totalLoginFestival_p4_festival_directory_good", Title: "有效登录活动", Kind: "login-day", Mode: "cumulative", Goal: 1, Enabled: true, StartAt: start, EndAt: end, RewardEndAt: rewardEnd, Rewards: "coin,1", Source: "isolated fixture"}
	if err := db.GetIns().Create(&good).Error; err != nil {
		t.Fatal(err)
	}
	children, err := json.Marshal([]p15SeasonalChild{{ID: good.ID, Title: good.Title, Domain: "period"}, {ID: "totalLoginFestival_p4_festival_directory_missing", Title: "未配置子活动", Domain: "period"}})
	if err != nil {
		t.Fatal(err)
	}
	parent := db.P15SeasonalBundle{ID: "festival_p4_directory_guard", Kind: "festival", Title: "目录完整性", Enabled: true, StartAt: start, EndAt: end, RewardEndAt: rewardEnd, Children: children, Source: "isolated fixture"}
	if err := db.GetIns().Create(&parent).Error; err != nil {
		t.Fatal(err)
	}
	tabs, err := f.s.p15SeasonalTabs(db.GetIns(), now)
	if err != nil || !slices.ContainsFunc(tabs, func(tab *pb.ActivityTab) bool { return tab.GetActivityId() == parent.ID }) {
		t.Fatal("healthy seasonal sibling hidden", err)
	}
	out, err := f.s.p15SeasonalList(f.ctx, f.id, &pb.CSActivityList{ActivityId: proto.String(parent.ID)}, now)
	if err != nil || len(out[0].(*pb.SCActivityList).GetActivityList()) != 1 || out[0].(*pb.SCActivityList).ActivityList[0].GetActivityId() != good.ID {
		t.Fatal("missing seasonal child blocked valid sibling", err)
	}
	if err := db.GetIns().Model(&good).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	tabs, err = f.s.p15SeasonalTabs(db.GetIns(), now)
	if err != nil || slices.ContainsFunc(tabs, func(tab *pb.ActivityTab) bool { return tab.GetActivityId() == parent.ID }) {
		t.Fatal("empty seasonal parent exposed", err)
	}
	out, err = f.s.p15SeasonalList(f.ctx, f.id, &pb.CSActivityList{ActivityId: proto.String(parent.ID)}, now)
	if err != nil || len(out[0].(*pb.SCActivityList).GetActivityList()) != 0 {
		t.Fatal("empty seasonal directory not a valid response", err)
	}
}
