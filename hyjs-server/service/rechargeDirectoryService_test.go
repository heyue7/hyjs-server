package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func rechargeDirectoryFixture(t *testing.T) *p1Fixture {
	t.Helper()
	f := p4Setup(t)
	if err := f.s.LoadPaymentTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.LoadP9Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, initialize := range []func(context.Context) error{f.s.InitializePayments, f.s.InitializeActivities, f.s.InitializeTurntable, f.s.InitializeP4CopyBenefits} {
		if err := initialize(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.s.InitializeClientUpdates(f.ctx); err != nil {
		t.Fatal(err)
	}
	session := rechargeDirectoryBindClient(t, f, f.id, constUtil.RechargeTotalDirectoryMinBuild, constUtil.RechargeTotalDirectoryMinVersion)
	f.ctx = context.WithValue(f.ctx, ggoNativeSessionKey{}, session)
	return f
}

func rechargeDirectoryBindClient(t *testing.T, f *p1Fixture, id, build int64, version string) []byte {
	t.Helper()
	var row db.Profile
	if err := db.GetIns().First(&row, id).Error; err != nil {
		t.Fatal(err)
	}
	appVersion := fmt.Sprintf("hyjs/%d/%032x", build, id)
	if version != "" {
		appVersion += "/" + version
	}
	if err := f.s.BindClientLogin(f.ctx, &row, appVersion); err != nil {
		t.Fatal(err)
	}
	return append([]byte(nil), row.Session...)
}

func rechargeDirectoryQuery(t *testing.T, f *p1Fixture) *pb.SCActivityInfo {
	t.Helper()
	result, err := f.s.P1Activity(f.ctx, f.id, &pb.CSActivityInfo{ActivityId: proto.String(constUtil.RechargeTotalDirectoryID)})
	if err != nil {
		t.Fatal(err)
	}
	return result[0].(*pb.SCActivityInfo)
}

func rechargeDirectoryItem(t *testing.T, info *pb.SCActivityInfo, domain, instance string, child int32) *pb.ActivityItem {
	t.Helper()
	var member db.RechargeDirectoryMember
	if err := db.GetIns().Where("domain=? AND instance_id=? AND child_index=?", domain, instance, child).First(&member).Error; err != nil {
		t.Fatal(err)
	}
	for _, item := range info.GetItemList() {
		if int64(item.GetIndex()) == member.IndexID {
			return item
		}
	}
	t.Fatalf("missing mapped reward %s/%s/%d", domain, instance, child)
	return nil
}

func rechargeDirectoryClaim(t *testing.T, f *p1Fixture, item *pb.ActivityItem) *pb.SCActivityReward {
	t.Helper()
	request := &pb.CSActivityReward{ActivityId: proto.String(constUtil.RechargeTotalDirectoryID), Index: item.Index, GetResources: item.RewardList}
	result, err := f.s.P1Activity(f.ctx, f.id, request)
	if err != nil {
		t.Fatal(err)
	}
	return result[0].(*pb.SCActivityReward)
}

// Actual cumulative-recharge pages, transaction-backed claims, direct legacy
// entries and persistent item identities share one isolated SQLite database.
func TestRechargeDirectoryMixedClaimsReplayAndCold(t *testing.T) {
	f := rechargeDirectoryFixture(t)
	now := time.Now()
	p1 := db.PeriodInstance{ID: "rechargeTotal_p4_personal_test", Title: "个人累充", Enabled: true, Kind: "payment_amount", Mode: "cumulative", Goal: 600, Rewards: "coin,10", StartAt: now.Add(-time.Hour).UnixMilli(), EndAt: now.Add(time.Hour).UnixMilli(), RewardEndAt: now.Add(2 * time.Hour).UnixMilli(), Source: "isolated recharge"}
	p1.Milestones, _ = json.Marshal([]p1PeriodMilestone{{Goal: 600, Rewards: "coin,10"}, {Goal: 1200, Rewards: "diamond,2"}})
	other := db.PeriodInstance{ID: "rechargeTotal_p4_calendar_test", Title: "全服累充", Enabled: true, Kind: "payment_amount", Mode: "cumulative", Goal: 600, Rewards: "coin,7", StartAt: p1.StartAt, EndAt: now.Add(3 * time.Hour).UnixMilli(), RewardEndAt: now.Add(4 * time.Hour).UnixMilli(), Source: "isolated recharge"}
	// A 128-character original P3 ID cannot be truncated in the directory map.
	longID := "rechargeTotal_" + strings.Repeat("x", 105) + "_local_p3"
	if len(longID) != 128 {
		t.Fatal("bad test ID size", len(longID))
	}
	p3 := db.PaymentActivityRule{ID: longID, Kind: "total", Enabled: true, Threshold: 600, Reward: "coin,3", Name: "旧累充", Source: "isolated recharge", StartAt: p1.StartAt, EndAt: p1.EndAt, RewardEndAt: p1.RewardEndAt}
	for _, row := range []any{&p1, &other, &p3} {
		if err := db.GetIns().Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	recharge75Order(t, f, "directory-confirmed", constUtil.P3PaymentConfirmed, now.Add(-time.Minute), 1200)
	list := func() []*pb.ActivityTab {
		t.Helper()
		messages, err := f.s.turntableActivityList(f.ctx, f.id)
		if err != nil {
			t.Fatal(err)
		}
		return messages[0].(*pb.SCActivityList).GetActivityList()
	}
	assertTab := func() {
		t.Helper()
		count := 0
		for _, tab := range list() {
			if strings.HasPrefix(tab.GetActivityId(), "rechargeTotal_") {
				count++
				if tab.GetActivityId() != constUtil.RechargeTotalDirectoryID {
					t.Fatal("unmerged tab", tab)
				}
			}
		}
		if count != 1 {
			t.Fatal("wrong number of recharge tabs", count)
		}
	}
	assertTab()
	if n := p1Count(t, &db.PaymentActivityState{}, "player_id=? AND activity_id=?", f.id, p3.ID); n != 0 {
		t.Fatal("listing froze payment activity", n)
	}
	// Exercise the authenticated binary request and response framing as well as
	// the underlying transactions; the directory ID takes the ordinary P1 route.
	wire := copyOverflowWire(t, f, &pb.CSActivityInfo{ActivityId: proto.String(constUtil.RechargeTotalDirectoryID)})
	if len(wire) != 1 {
		t.Fatal("unexpected directory wire response", wire)
	}
	info, ok := wire[0].(*pb.SCActivityInfo)
	if !ok {
		t.Fatal("directory wire route failed", wire)
	}
	if len(info.ItemList) != 4 || p1Count(t, &db.PaymentActivityState{}, "player_id=? AND activity_id=?", f.id, p3.ID) != 1 {
		t.Fatal("combined view lost tiers or failed to freeze P3", info)
	}
	a := rechargeDirectoryItem(t, info, rechargeDirectoryP1, p1.ID, 1)
	b := rechargeDirectoryItem(t, info, rechargeDirectoryP1, p1.ID, 2)
	c := rechargeDirectoryItem(t, info, rechargeDirectoryP1, other.ID, 1)
	d := rechargeDirectoryItem(t, info, rechargeDirectoryP3, p3.ID, 1)
	if a.GetStatus() != 1 || b.GetStatus() != 1 || c.GetStatus() != 1 || d.GetStatus() != 1 || d.GetValue() != "600" || a.GetParam() != constUtil.RechargeTotalDirectoryItemParam || d.GetParam() != constUtil.RechargeTotalDirectoryItemParam || !strings.Contains(a.GetMessage(), "个人累充") || !strings.Contains(c.GetMessage(), "全服累充") || !strings.Contains(d.GetMessage(), "领奖截止") {
		t.Fatal("wrong source, eligibility, units or caption", info)
	}
	if a.GetIndex() == b.GetIndex() || b.GetIndex() == c.GetIndex() || c.GetIndex() == d.GetIndex() {
		t.Fatal("duplicate permanent indices", info)
	}
	legacyInfo, err := f.s.paymentActivity(f.ctx, f.id, &pb.CSActivityInfo{ActivityId: proto.String(p3.ID)}, now)
	if err != nil || legacyInfo[0].(*pb.SCActivityInfo).GetItemList()[0].GetValue() != "6" {
		t.Fatal("direct P3 currency display changed", legacyInfo, err)
	}
	if err := db.GetIns().Model(&db.PaymentActivityRule{}).Where("id=?", p3.ID).Updates(map[string]any{"threshold": 9000, "reward": "diamond,99"}).Error; err != nil {
		t.Fatal(err)
	}
	frozenP3 := rechargeDirectoryItem(t, rechargeDirectoryQuery(t, f), rechargeDirectoryP3, p3.ID, 1)
	if frozenP3.GetValue() != "600" || !p1SameResources(frozenP3.RewardList, d.RewardList) {
		t.Fatal("first observation did not freeze P3 rule", frozenP3)
	}
	// A forged reward and a valid index from another directory cannot grant.
	for _, bad := range []*pb.CSActivityReward{
		{ActivityId: proto.String(constUtil.RechargeTotalDirectoryID), Index: a.Index, GetResources: []*pb.BaseResource{{Id: proto.String("diamond"), Num: proto.Int32(999)}}},
		{ActivityId: proto.String(constUtil.RechargeTotalDirectoryID), Index: proto.Int32(999999), GetResources: a.RewardList},
	} {
		if _, err := f.s.P1Activity(f.ctx, f.id, bad); err == nil {
			t.Fatal("forged claim accepted", bad)
		}
	}
	var delivery db.RewardDelivery
	if err := db.GetIns().Where("player_id=? AND source_key=?", f.id, "activity:"+p1.ID).First(&delivery).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.RewardCenter(f.ctx, f.id, &pb.CSAddRewardsToPlayer{RewardIds: []int64{delivery.ID}}); err == nil {
		t.Fatal("reward center bypassed the cumulative-recharge page")
	}
	// The direct P1 path and the combined page share exactly the same receipt.
	direct := &pb.CSActivityReward{ActivityId: proto.String(p1.ID), Index: proto.Int32(1), GetResources: a.RewardList}
	if _, err := f.s.P1Activity(f.ctx, f.id, direct); err != nil {
		t.Fatal("direct P1 claim", err)
	}
	if got := rechargeDirectoryClaim(t, f, a); len(got.GetGet().GetSimps()) != 0 {
		t.Fatal("direct then directory paid twice", got)
	}
	if got := rechargeDirectoryClaim(t, f, b); len(got.ItemList) != 4 || len(got.GetGet().GetSimps()) != 1 {
		t.Fatal("claim omitted siblings or reward", got)
	}
	if got := rechargeDirectoryClaim(t, f, d); len(got.ItemList) != 4 || len(got.GetGet().GetSimps()) != 1 {
		t.Fatal("P3 claim failed", got)
	}
	if got := rechargeDirectoryClaim(t, f, d); len(got.GetGet().GetSimps()) != 0 {
		t.Fatal("P3 replay paid twice", got)
	}
	forgedP3 := proto.Clone(d.RewardList[0]).(*pb.BaseResource)
	forgedP3.Guid = proto.String("forged")
	if _, err := f.s.P1Activity(f.ctx, f.id, &pb.CSActivityReward{ActivityId: proto.String(constUtil.RechargeTotalDirectoryID), Index: d.Index, GetResources: []*pb.BaseResource{forgedP3}}); err == nil {
		t.Fatal("P3 replay accepted forged resource GUID")
	}
	if _, err := f.s.paymentActivity(f.ctx, f.id, &pb.CSActivityReward{ActivityId: proto.String(p3.ID), Index: proto.Int32(1), GetResources: d.RewardList}, now); err == nil {
		t.Fatal("old P3 endpoint granted an already-claimed reward")
	}
	if got := rechargeDirectoryClaim(t, f, c); len(got.GetGet().GetSimps()) != 1 {
		t.Fatal("calendar claim failed", got)
	}
	if replay, err := f.s.P1Activity(f.ctx, f.id, &pb.CSActivityReward{ActivityId: proto.String(other.ID), Index: proto.Int32(1), GetResources: c.RewardList}); err != nil || len(replay[0].(*pb.SCActivityReward).GetGet().GetSimps()) != 0 {
		t.Fatal("directory then direct P1 paid twice", replay, err)
	}
	assertTab()
	all := rechargeDirectoryQuery(t, f)
	for _, item := range all.GetItemList() {
		if item.GetStatus() != 2 {
			t.Fatal("claimed item missing", all)
		}
	}
	// Disable and replace a child; its old index remains bound and cannot claim
	// the replacement even when resources and amount match.
	if err := db.GetIns().Model(&other).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	replacement := other
	replacement.ID = "rechargeTotal_p4_new_calendar_test"
	replacement.Enabled = true
	if err := db.GetIns().Create(&replacement).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.P1Activity(f.ctx, f.id, &pb.CSActivityReward{ActivityId: proto.String(constUtil.RechargeTotalDirectoryID), Index: c.Index, GetResources: c.RewardList}); err == nil {
		t.Fatal("disabled child index claimed replacement")
	}
	info = rechargeDirectoryQuery(t, f)
	newItem := rechargeDirectoryItem(t, info, rechargeDirectoryP1, replacement.ID, 1)
	if newItem.GetIndex() == c.GetIndex() || rechargeDirectoryItem(t, info, rechargeDirectoryP3, p3.ID, 1).GetIndex() != d.GetIndex() {
		t.Fatal("index shifted after replacing child")
	}
	f.restart(t)
	if err := f.s.InitializeClientUpdates(f.ctx); err != nil {
		t.Fatal(err)
	}
	info = rechargeDirectoryQuery(t, f)
	if rechargeDirectoryItem(t, info, rechargeDirectoryP1, p1.ID, 1).GetIndex() != a.GetIndex() || rechargeDirectoryItem(t, info, rechargeDirectoryP3, p3.ID, 1).GetIndex() != d.GetIndex() || rechargeDirectoryItem(t, info, rechargeDirectoryP1, replacement.ID, 1).GetIndex() != newItem.GetIndex() {
		t.Fatal("cold restart changed directory mappings")
	}
}

func TestRechargeDirectoryReservedCollisionAndIndexTypes(t *testing.T) {
	f := rechargeDirectoryFixture(t)
	now := time.Now()
	period := db.PeriodInstance{ID: constUtil.RechargeTotalDirectoryID, Title: "冲突", Enabled: false, Kind: "payment_amount", Mode: "cumulative", Goal: 1, Rewards: "coin,1", StartAt: now.Add(-time.Hour).UnixMilli(), EndAt: now.Add(time.Hour).UnixMilli(), RewardEndAt: now.Add(time.Hour).UnixMilli(), Source: "fixture"}
	if err := db.GetIns().Create(&period).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.P1Activity(f.ctx, f.id, &pb.CSActivityInfo{ActivityId: proto.String(constUtil.RechargeTotalDirectoryID)}); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatal("reserved P1 collision did not fail closed", err)
	}
	if _, err := f.s.AvailableActivities(f.ctx, f.id); err != nil {
		t.Fatal("disabled recharge collision blocked unrelated activity tabs", err)
	}
	tabs := []*pb.ActivityTab{{ActivityId: proto.String("rechargeTotal_p4_visible_fixture")}}
	if _, err := f.s.rechargeDirectoryTabs(db.GetIns().WithContext(f.ctx), f.id, now.UnixMilli(), tabs); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatal("reserved collision leaked through a visible recharge directory", err)
	}
	if err := f.s.InitializeP1(f.ctx); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatal("startup did not detect reserved P1 collision", err)
	}
	if err := f.s.validateP1Period(period); err == nil {
		t.Fatal("reserved P1 ID passed validation")
	}
	if err := db.GetIns().Delete(&period).Error; err != nil {
		t.Fatal(err)
	}
	payment := db.PaymentActivityRule{ID: constUtil.RechargeTotalDirectoryID, Kind: "total", Source: "fixture", Name: "冲突", Reward: "coin,1", Threshold: 1, StartAt: period.StartAt, EndAt: period.EndAt, RewardEndAt: period.RewardEndAt}
	if err := db.GetIns().Create(&payment).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializePayments(f.ctx); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatal("startup did not detect reserved P3 collision", err)
	}
	if err := db.GetIns().Delete(&payment).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := rechargeDirectoryIndex(db.GetIns(), rechargeDirectoryCandidate{domain: rechargeDirectoryP1, id: "totalLogin_p1_test"}, 1); err == nil {
		t.Fatal("login index accepted by recharge directory")
	}
	if _, err := rechargeDirectoryIndex(db.GetIns(), rechargeDirectoryCandidate{domain: "other", id: "rechargeTotal_p4_test"}, 1); err == nil {
		t.Fatal("unknown domain accepted")
	}
	unicodeID := "rechargeTotal_" + strings.Repeat("🐟", 114)
	index, err := rechargeDirectoryIndex(db.GetIns(), rechargeDirectoryCandidate{domain: rechargeDirectoryP3, id: unicodeID}, 1)
	if err != nil {
		t.Fatal("full UTF-8 P3 identifier rejected", err)
	}
	var mapped db.RechargeDirectoryMember
	if err := db.GetIns().First(&mapped, "index_id=?", index).Error; err != nil || mapped.InstanceID != unicodeID {
		t.Fatal("UTF-8 P3 identifier changed", mapped, err)
	}
	if _, err := f.s.AdminCreateActivity(f.ctx, nil, model.AdminActivityCreateRequest{TemplateID: "period:rechargeTotal_p4_directory", NewID: "period:" + constUtil.RechargeTotalDirectoryID}); !errors.Is(err, constUtil.ErrAdminInput) {
		t.Fatal("admin copy accepted reserved ID", err)
	}
	if err := f.s.adminPersistTypedActivity(db.GetIns(), adminActivityDefinition{domain: "period"}, constUtil.RechargeTotalDirectoryID, model.AdminActivityForm{}, false, false); !errors.Is(err, constUtil.ErrAdminInput) {
		t.Fatal("typed admin accepted reserved ID", err)
	}
	if err := f.s.adminNativeApplyPayment(db.GetIns(), &adminNativeSnapshot{payment: payment}, model.AdminNativeActivitySaveRequest{}); !errors.Is(err, constUtil.ErrAdminInput) {
		t.Fatal("native payment edit accepted reserved ID", err)
	}
	if err := f.s.adminNativeApplyCelebration(db.GetIns(), &adminNativeSnapshot{period: period}, model.AdminNativeActivitySaveRequest{}); !errors.Is(err, constUtil.ErrAdminInput) {
		t.Fatal("native period edit accepted reserved ID", err)
	}
	if err := f.s.adminNativeApplyFrozenState(db.GetIns(), &adminNativeSnapshot{payment: payment}, true); !errors.Is(err, constUtil.ErrAdminInput) {
		t.Fatal("native enable accepted reserved ID", err)
	}
}

func TestRechargeDirectoryHalfOpenWindowsAndInvalidRules(t *testing.T) {
	f := rechargeDirectoryFixture(t)
	now := time.Now().UnixMilli()
	start, end, deadline := now-1000, now+1000, now+2000
	period := db.PeriodInstance{ID: "rechargeTotal_p4_window", Title: "窗口", Enabled: true, Kind: "payment_amount", Mode: "cumulative", Goal: 100, Rewards: "coin,1", StartAt: start, EndAt: end, RewardEndAt: deadline, Source: "fixture"}
	payment := db.PaymentActivityRule{ID: "rechargeTotal_window_local_p3", Name: "旧窗口", Enabled: true, Kind: "total", Threshold: 100, Reward: "coin,1", StartAt: start, EndAt: end, RewardEndAt: deadline, Source: "fixture"}
	bad := period
	bad.ID, bad.Goal = "rechargeTotal_p4_invalid_window", 0
	badPayment := payment
	badPayment.ID, badPayment.Reward = "rechargeTotal_invalid_local_p3", "unknown-item,1"
	for _, row := range []any{&period, &payment, &bad, &badPayment} {
		if err := db.GetIns().Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, check := range []struct {
		at    int64
		count int
	}{{start - 1, 0}, {start, 2}, {end, 2}, {deadline - 1, 2}, {deadline, 0}} {
		got, err := f.s.rechargeDirectoryCandidates(db.GetIns().WithContext(f.ctx), f.id, check.at)
		if err != nil || len(got) != check.count {
			t.Fatalf("at %d: got %d candidates, want %d: %v", check.at, len(got), check.count, err)
		}
	}
}

func TestRechargeDirectoryRejectsForeignPersonalInstance(t *testing.T) {
	f := rechargeDirectoryFixture(t)
	if err := db.GetIns().AutoMigrate(&db.AdminActivityOccurrence{}, &db.AdminCharacterActivityInstance{}, &db.AdminActivityTemplateOwner{}, &db.AdminActivityScheduleRevision{}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	other, err := LoginAccount(f.ctx, fmt.Sprintf("directory-peer-%d", now.UnixNano()), "fixture-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(f.ctx, other.Id, "目录角色乙", "hero_043"); err != nil {
		t.Fatal(err)
	}
	key := "rechargeTotal_p4_admin_character_owner"
	period := db.PeriodInstance{ID: key, Title: "个人累充", Enabled: true, Kind: "payment_amount", Mode: "cumulative", Goal: 600, Rewards: "coin,8", StartAt: now.Add(-time.Hour).UnixMilli(), EndAt: now.Add(time.Hour).UnixMilli(), RewardEndAt: now.Add(time.Hour).UnixMilli(), Source: "fixture"}
	occ := db.AdminActivityOccurrence{ID: "directory-role-schedule", ServerID: f.s.OriginalServerID(), TemplateID: "directory-role-template", Mode: "recharge-total", Timing: "character-day", Enabled: true, StartAt: period.StartAt, EndAt: period.EndAt, ClaimEndAt: period.RewardEndAt, FormJSON: []byte(`{"mode":"recharge-total","repeatPolicy":"repeat"}`)}
	issued := db.AdminCharacterActivityInstance{PlayerID: f.id, OccurrenceID: occ.ID, ServerID: occ.ServerID, ConsumerKey: key, ConsumerID: "period:" + key, Mode: occ.Mode, StartAt: period.StartAt, EndAt: period.EndAt, ClaimEndAt: period.RewardEndAt, FormJSON: occ.FormJSON}
	for _, row := range []any{&period, &occ, &issued} {
		if err = db.GetIns().Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	// The role must exist before its confirmed order for character-day rules.
	if err = db.GetIns().Model(&db.Profile{}).Where("id=?", f.id).Update("created_at", now.Add(-2*time.Hour).UnixMilli()).Error; err != nil {
		t.Fatal(err)
	}
	recharge75Order(t, f, "directory-private-paid", constUtil.P3PaymentConfirmed, now.Add(-time.Minute), 600)
	ownerCtx := context.WithValue(recharge75Context(f.id, 0), constUtil.GameServiceContextKey, f.s)
	peerCtx := context.WithValue(recharge75Context(other.Id, 0), constUtil.GameServiceContextKey, f.s)
	ownerSession := rechargeDirectoryBindClient(t, f, f.id, constUtil.RechargeTotalDirectoryMinBuild, constUtil.RechargeTotalDirectoryMinVersion)
	peerSession := rechargeDirectoryBindClient(t, f, other.Id, constUtil.RechargeTotalDirectoryMinBuild, constUtil.RechargeTotalDirectoryMinVersion)
	ownerCtx = context.WithValue(ownerCtx, ggoNativeSessionKey{}, ownerSession)
	peerCtx = context.WithValue(peerCtx, ggoNativeSessionKey{}, peerSession)
	f.ctx = ownerCtx
	item := rechargeDirectoryItem(t, rechargeDirectoryQuery(t, f), rechargeDirectoryP1, key, 1)
	forged := &pb.CSActivityReward{ActivityId: proto.String(constUtil.RechargeTotalDirectoryID), Index: item.Index, GetResources: item.RewardList}
	if _, err = f.s.P1Activity(peerCtx, other.Id, forged); err == nil {
		t.Fatal("foreign character claimed private activity")
	}
	if _, err = f.s.P1Activity(ownerCtx, f.id, forged); err != nil {
		t.Fatal("owner could not claim", err)
	}
	if err = db.GetIns().Model(&occ).Update("archived", true).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.P1Activity(ownerCtx, f.id, forged); err == nil {
		t.Fatal("archived personal activity remained claimable")
	}
}

// Transaction rollback cannot leave an index bound to a different child.
func TestRechargeDirectoryIndexRollback(t *testing.T) {
	rechargeDirectoryFixture(t)
	candidate := rechargeDirectoryCandidate{domain: rechargeDirectoryP1, id: "rechargeTotal_p4_rollback"}
	sentinel := errors.New("rollback")
	if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		if _, err := rechargeDirectoryIndex(tx, candidate, 1); err != nil {
			return err
		}
		return sentinel
	}); !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
	if p1Count(t, &db.RechargeDirectoryMember{}, "instance_id=?", candidate.id) != 0 {
		t.Fatal("rolled-back index persisted")
	}
}

func TestRechargeDirectoryDoesNotTruncateCombinedTiers(t *testing.T) {
	f := recharge75Fixture(t)
	if err := f.s.InitializeClientUpdates(f.ctx); err != nil {
		t.Fatal(err)
	}
	session := rechargeDirectoryBindClient(t, f, f.id, constUtil.RechargeTotalDirectoryMinBuild, constUtil.RechargeTotalDirectoryMinVersion)
	f.ctx = context.WithValue(f.ctx, ggoNativeSessionKey{}, session)
	now := time.Now()
	for i := 0; i < 4; i++ {
		tiers := make([]p1PeriodMilestone, 0, 31)
		for n := int64(1); n <= 31; n++ {
			tiers = append(tiers, p1PeriodMilestone{Goal: n, Rewards: "coin,1"})
		}
		milestones, err := json.Marshal(tiers)
		if err != nil {
			t.Fatal(err)
		}
		row := db.PeriodInstance{ID: fmt.Sprintf("rechargeTotal_p4_many_%d", i), Title: "同期累充", Enabled: true, Kind: "payment_amount", Mode: "cumulative", Goal: 1, Rewards: "coin,1", Milestones: milestones, StartAt: now.Add(-time.Hour).UnixMilli(), EndAt: now.Add(time.Hour).UnixMilli(), RewardEndAt: now.Add(time.Hour).UnixMilli(), Source: "fixture"}
		if err = db.GetIns().Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	info := rechargeDirectoryQuery(t, f)
	if len(info.GetItemList()) != 124 {
		t.Fatalf("combined page truncated tiers: %d", len(info.GetItemList()))
	}
	seen := map[int32]bool{}
	for _, item := range info.GetItemList() {
		if item.GetIndex() <= 0 || seen[item.GetIndex()] || item.GetParam() != constUtil.RechargeTotalDirectoryItemParam {
			t.Fatal("invalid combined tier", item)
		}
		seen[item.GetIndex()] = true
	}
}

func TestRechargeDirectorySessionVersionRouting(t *testing.T) {
	f := rechargeDirectoryFixture(t)
	if err := InitializeLoginSecurity(f.ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for _, key := range []string{"rechargeTotal_p4_version_a", "rechargeTotal_p4_version_b"} {
		period := db.PeriodInstance{ID: key, Title: "累充版本测试", Enabled: true, Kind: "payment_amount", Mode: "cumulative", Goal: 100, Rewards: "coin,1", StartAt: now.Add(-time.Hour).UnixMilli(), EndAt: now.Add(time.Hour).UnixMilli(), RewardEndAt: now.Add(time.Hour).UnixMilli(), Source: "fixture"}
		if err := db.GetIns().Create(&period).Error; err != nil {
			t.Fatal(err)
		}
	}
	assertMode := func(messages []proto.Message, merged bool) {
		t.Helper()
		if len(messages) != 1 {
			t.Fatal("unexpected activity list response", messages)
		}
		list, ok := messages[0].(*pb.SCActivityList)
		if !ok {
			t.Fatal("activity list protocol failed", messages)
		}
		count, directories := 0, 0
		for _, tab := range list.GetActivityList() {
			if strings.HasPrefix(tab.GetActivityId(), "rechargeTotal_") {
				count++
				if tab.GetActivityId() == constUtil.RechargeTotalDirectoryID {
					directories++
				}
			}
		}
		if (merged && (count != 1 || directories != 1)) || (!merged && (count != 2 || directories != 0)) {
			t.Fatalf("merged=%t recharge tabs=%d directory=%d", merged, count, directories)
		}
	}
	for _, old := range []struct {
		build   int64
		version string
	}{{2026110104, "1.1.4"}, {2026110105, "1.1.5"}, {constUtil.RechargeTotalDirectoryMinBuild, ""}, {constUtil.RechargeTotalDirectoryMinBuild - 1, "1.1.6"}, {constUtil.RechargeTotalDirectoryMinBuild, "1.1.5"}} {
		session := rechargeDirectoryBindClient(t, f, f.id, old.build, old.version)
		f.ctx = context.WithValue(f.ctx, ggoNativeSessionKey{}, session)
		assertMode(copyOverflowWire(t, f, &pb.CSActivityList{}), false)
		assertMode(bossHandshake86Call(t, f, f.id, 57005, &pb.CSActivityList{}), false)
		if _, err := f.s.P1Activity(f.ctx, f.id, &pb.CSActivityInfo{ActivityId: proto.String(constUtil.RechargeTotalDirectoryID)}); err == nil {
			t.Fatal("old or unknown client accessed combined page", old)
		}
	}
	if err := db.GetIns().Where("player_id=?", f.id).Delete(&db.ClientUpdateSession{}).Error; err != nil {
		t.Fatal(err)
	}
	assertMode(copyOverflowWire(t, f, &pb.CSActivityList{}), false)
	rechargeDirectoryBindClient(t, f, f.id, 2026110105, "1.1.5")
	legacy := copyOverflowWire(t, f, &pb.CSActivityInfo{ActivityId: proto.String("rechargeTotal_p4_version_a")})
	if info, ok := legacy[0].(*pb.SCActivityInfo); !ok || len(info.ItemList) != 1 || info.ItemList[0].GetParam() == constUtil.RechargeTotalDirectoryItemParam {
		t.Fatal("old direct activity response changed", legacy)
	}
	var original db.Profile
	if err := db.GetIns().First(&original, f.id).Error; err != nil {
		t.Fatal(err)
	}
	oldSession := append([]byte(nil), original.Session...)
	// The authenticated HTTP protocol runs the actual account login and binds
	// its reported version to the newly issued session.
	appVersion := fmt.Sprintf("hyjs/%d/%032x/%s", constUtil.RechargeTotalDirectoryMinBuild, f.id, constUtil.RechargeTotalDirectoryMinVersion)
	loginHTTP := copyOverflowWire(t, f, &pb.CSUserLogin{PlatformUid: proto.String(original.Uid), AuthCode: proto.String("isolated-password"), AppVersion: proto.String(appVersion)})
	if len(loginHTTP) != 1 {
		t.Fatal("new client login failed", loginHTTP)
	}
	login, ok := loginHTTP[0].(*pb.SCUserLogin)
	if !ok || len(login.GetSessionKey()) != 16 {
		t.Fatal("new client session missing", loginHTTP)
	}
	newSession := append([]byte(nil), login.GetSessionKey()...)
	if string(newSession) == string(oldSession) {
		t.Fatal("login did not rotate session")
	}
	f.ctx = context.WithValue(f.ctx, ggoNativeSessionKey{}, newSession)
	stale := context.WithValue(f.ctx, ggoNativeSessionKey{}, oldSession)
	if supported, err := rechargeDirectoryClientSupported(db.GetIns().WithContext(stale), f.id); err != nil || supported {
		t.Fatal("old request inherited newer session capabilities", supported, err)
	}
	if _, err := f.s.P1Activity(stale, f.id, &pb.CSActivityInfo{ActivityId: proto.String(constUtil.RechargeTotalDirectoryID)}); err == nil {
		t.Fatal("old request accessed new session directory")
	}
	assertMode(copyOverflowWire(t, f, &pb.CSActivityList{}), true)
	assertMode(bossHandshake86Call(t, f, f.id, 57005, &pb.CSActivityList{}), true)
	if err := f.s.ClientUpdateConnected(f.ctx, f.id, newSession); err != nil {
		t.Fatal(err)
	}
	if err := f.s.ClientUpdateDisconnected(f.ctx, f.id, newSession); err != nil {
		t.Fatal(err)
	}
	assertMode(copyOverflowWire(t, f, &pb.CSActivityList{}), true)
	// A server restart and TCP reconnect keep the same bound capability.
	f.restart(t)
	if err := f.s.InitializeClientUpdates(f.ctx); err != nil {
		t.Fatal(err)
	}
	reconnect := bossHandshake86Call(t, f, f.id, 20031, &pb.CSSocketLogin{SessionKey: newSession})
	if len(reconnect) != 1 || reconnect[0].(*pb.SCSocketLogin).GetResult() != 100 {
		t.Fatal("TCP reconnect failed", reconnect)
	}
	assertMode(bossHandshake86Call(t, f, f.id, 57005, &pb.CSActivityList{}), true)
	// Reproduce the interval after the profile session rotates and before the
	// next BindClientLogin; the old bound capability must stop immediately.
	if _, err := LoginAccount(f.ctx, original.Uid, "isolated-password"); err != nil {
		t.Fatal(err)
	}
	if supported, err := rechargeDirectoryClientSupported(db.GetIns().WithContext(f.ctx), f.id); err != nil || supported {
		t.Fatal("rotated profile accepted the previous session binding", supported, err)
	}
	if _, err := f.s.P1Activity(f.ctx, f.id, &pb.CSActivityInfo{ActivityId: proto.String(constUtil.RechargeTotalDirectoryID)}); err == nil {
		t.Fatal("rotated profile allowed old-session directory access")
	}
}
