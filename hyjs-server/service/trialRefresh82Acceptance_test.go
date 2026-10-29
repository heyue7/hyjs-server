package service

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// trial82MySQLFixture forbids a silent SQLite fallback in the independent
// transaction checks. The runner owns and releases this schema and its user.
func trial82MySQLFixture(t *testing.T) (*p1Fixture, time.Time) {
	t.Helper()
	name := os.Getenv("HYJS_P4_MYSQL_DATABASE")
	if !strings.HasPrefix(name, "hyjs_test_p4_") {
		t.Skip("requires a separately owned P4 MySQL schema")
	}
	f := p4Setup(t)
	var actual, engine string
	if err := db.GetIns().Raw("SELECT DATABASE()").Scan(&actual).Error; err != nil || actual != name {
		t.Fatal("fixture database identity mismatch", err)
	}
	if err := db.GetIns().Raw("SELECT ENGINE FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='profiles'").Scan(&engine).Error; err != nil || engine != "InnoDB" {
		t.Fatal("fixture profiles table is not InnoDB", engine, err)
	}
	p4Fund(t, f)
	if err := db.GetIns().Create(&db.P7RushState{PlayerID: f.id, RecordStars: 150}).Error; err != nil {
		t.Fatal(err)
	}
	var cfg db.P4CommerceRule
	if err := db.GetIns().Where("kind=?", "point").First(&cfg).Error; err != nil {
		t.Fatal(err)
	}
	product := p4BaseProduct("trial82_acceptance", "item_005,1", "point,100", 1, 1)
	rules := p4CommerceRules{
		TrialRefreshVersion: 1, LiveEditable: true, Products: []p4Product{product},
		Slots: 6, FreeCap: 10, FreeSeconds: 7200, ResourceRefreshCap: 15,
		RefreshItem: "item_013,1", RefreshCost: "diamond,20",
	}
	raw, err := json.Marshal(rules)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Model(&cfg).Updates(map[string]any{"rules": raw, "frozen": nil}).Error; err != nil {
		t.Fatal(err)
	}
	return f, time.Now()
}

type trial82ConcurrentResult struct {
	stamp int64
	msg   proto.Message
	err   error
}

func trial82Concurrent(t *testing.T, f *p1Fixture, request proto.Message, stamps []int64, now time.Time) []trial82ConcurrentResult {
	t.Helper()
	start := make(chan struct{})
	results := make(chan trial82ConcurrentResult, len(stamps))
	var wait sync.WaitGroup
	for _, stamp := range stamps {
		wait.Add(1)
		go func(stamp int64) {
			defer wait.Done()
			<-start
			messages, err := f.s.p4Commerce(f.ctx, f.id, request, stamp, now)
			result := trial82ConcurrentResult{stamp: stamp, err: err}
			if err == nil && len(messages) > 0 {
				result.msg = messages[0]
			}
			results <- result
		}(stamp)
	}
	close(start)
	wait.Wait()
	close(results)
	var collected []trial82ConcurrentResult
	for result := range results {
		collected = append(collected, result)
	}
	return collected
}

func trial82ColdOpen(t *testing.T, f *p1Fixture) {
	t.Helper()
	sqlDB, err := db.GetIns().DB()
	if err != nil {
		t.Fatal(err)
	}
	if err = sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	f.open(t)
}

func TestTrialRefresh82MySQLConcurrentRefreshReplay(t *testing.T) {
	f, now := trial82MySQLFixture(t)
	query := &pb.CSRefreshShopQuery{ShopType: proto.Int32(5)}
	before := p4Trade(t, f, query, 0, now).(*pb.SCRefreshShopQuery)
	if len(before.GetShopItemList()) != 6 || before.GetFreeLeftTimes() != 1 {
		t.Fatal("initial six slots or free count", before)
	}
	playerBefore, gameBefore := f.profile(t)
	request := &pb.CSRefreshShopRefresh{ShopType: proto.Int32(5)}
	results := trial82Concurrent(t, f, request, []int64{820101, 820101, 820101, 820101}, now)
	var first proto.Message
	for _, result := range results {
		if result.err != nil || result.msg == nil {
			t.Fatal("same-request refresh failed", result.err)
		}
		if first == nil {
			first = result.msg
		} else if !proto.Equal(first, result.msg) {
			t.Fatal("same-request refresh returned different receipts")
		}
	}
	response, ok := first.(*pb.SCRefreshShopRefresh)
	if !ok || len(response.GetShopItemList()) != 6 || response.GetFreeLeftTimes() != 0 || len(response.GetCost()) != 0 {
		t.Fatal("free refresh result", first)
	}
	var row db.P4CommerceState
	if err := db.GetIns().First(&row, "player_id=? AND rule_id=?", f.id, constUtil.TrialRefresh82StateID).Error; err != nil {
		t.Fatal(err)
	}
	var state trialRefresh82State
	if err := json.Unmarshal(row.Payload, &state); err != nil || state.Generation != 2 || state.FreeLeft != 0 || state.ResourceUsed != 0 {
		t.Fatal("more than one committed refresh", state, err)
	}
	if count := p1Count(t, &db.P4Receipt{}, "player_id=? AND domain=? AND timestamp=?", f.id, "commerce:5", 820101); count != 1 {
		t.Fatal("refresh receipt count", count)
	}
	trial82ColdOpen(t, f)
	if replay := p4Trade(t, f, request, 820101, now.Add(time.Minute)); !proto.Equal(first, replay) {
		t.Fatal("cold refresh replay differs")
	}
	playerAfter, gameAfter := f.profile(t)
	if playerAfter.GetPoint() != playerBefore.GetPoint() || playerAfter.GetDiamond() != playerBefore.GetDiamond() || simpleOwned(playerAfter, gameAfter, "item_013") != simpleOwned(playerBefore, gameBefore, "item_013") {
		t.Fatal("free refresh or replay charged assets")
	}
}

func TestTrialRefresh82MySQLConcurrentSlotPurchaseAndColdReplay(t *testing.T) {
	f, now := trial82MySQLFixture(t)
	query := &pb.CSRefreshShopQuery{ShopType: proto.Int32(5)}
	page := p4Trade(t, f, query, 0, now).(*pb.SCRefreshShopQuery)
	if len(page.GetShopItemList()) != 6 {
		t.Fatal("six slots missing")
	}
	slot := page.GetShopItemList()[0]
	request := &pb.CSRefreshShopBuy{ShopType: proto.Int32(5), ShopItemId: proto.String(slot.GetId())}
	playerBefore, gameBefore := f.profile(t)
	results := trial82Concurrent(t, f, request, []int64{820201, 820202}, now)
	var accepted trial82ConcurrentResult
	successes := 0
	for _, result := range results {
		if result.err == nil && result.msg != nil {
			successes++
			accepted = result
		}
	}
	if successes != 1 {
		t.Fatal("distinct requests oversold the same slot", successes)
	}
	playerAfter, gameAfter := f.profile(t)
	if playerAfter.GetPoint() != playerBefore.GetPoint()-100 || simpleOwned(playerAfter, gameAfter, "item_005") != simpleOwned(playerBefore, gameBefore, "item_005")+1 {
		t.Fatal("purchase did not atomically charge once and grant once")
	}
	if count := p1Count(t, &db.P4Receipt{}, "player_id=? AND domain=?", f.id, "commerce:5"); count != 1 {
		t.Fatal("purchase receipt count", count)
	}
	var row db.P4CommerceState
	if err := db.GetIns().First(&row, "player_id=? AND rule_id=?", f.id, constUtil.TrialRefresh82StateID).Error; err != nil {
		t.Fatal(err)
	}
	var state trialRefresh82State
	if err := json.Unmarshal(row.Payload, &state); err != nil || len(state.Bought) != 1 {
		t.Fatal("slot ownership differs", state, err)
	}
	trial82ColdOpen(t, f)
	for _, replay := range trial82Concurrent(t, f, request, []int64{accepted.stamp, accepted.stamp, accepted.stamp}, now.Add(time.Minute)) {
		if replay.err != nil || !proto.Equal(accepted.msg, replay.msg) {
			t.Fatal("concurrent cold purchase replay differs", replay.err)
		}
	}
	collision := &pb.CSRefreshShopBuy{ShopType: proto.Int32(5), ShopItemId: page.GetShopItemList()[1].Id}
	if _, err := f.s.p4Commerce(f.ctx, f.id, collision, accepted.stamp, now.Add(time.Minute)); err == nil {
		t.Fatal("different purchase reused the accepted stamp")
	}
	playerCold, gameCold := f.profile(t)
	if playerCold.GetPoint() != playerAfter.GetPoint() || simpleOwned(playerCold, gameCold, "item_005") != simpleOwned(playerAfter, gameAfter, "item_005") {
		t.Fatal("cold replay mutated assets")
	}
	if _, err := f.s.p4Commerce(f.ctx, f.id, request, 820203, now.Add(time.Minute)); err == nil {
		t.Fatal("sold slot accepted a fresh request after reopen")
	}
}

// A previously accepted Re battle must finish with its frozen old reward even
// when the reviewed maintenance migration has repriced the live stage row.
func TestTrial82RePendingRewardSurvivesEconomyMigration(t *testing.T) {
	if !strings.HasPrefix(os.Getenv("HYJS_P7_MYSQL_DATABASE"), "hyjs_test_p7_") {
		t.Skip("requires a separately owned P7 MySQL schema")
	}
	f := p7PartyBusinessFixture(t)
	for _, initialize := range []func() error{
		func() error { return f.s.InitializeGameplayPools(f.ctx) },
		func() error { return f.s.InitializeShop(f.ctx) },
		func() error { return f.s.InitializeP4(f.ctx) },
		func() error { return AdminInitialize(f.ctx, "") },
		func() error { return InitializeAdminOperations(f.ctx) },
		func() error { return f.s.InitializeClientUpdates(f.ctx) },
		func() error { return f.s.InitializeServerGameClock(f.ctx) },
	} {
		if err := initialize(); err != nil {
			t.Fatal("migration fixture initialization", err)
		}
	}
	user := db.AdminUser{Username: "trial82-pending-test-admin", PasswordHash: "unused-isolated-fixture", Role: constUtil.AdminRoleAdmin, Enabled: true}
	if err := db.GetIns().Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	actor := &model.AdminPrincipal{UserID: user.ID, Username: user.Username, Role: user.Role}
	if err := db.GetIns().Model(&db.ClientUpdatePolicy{}).Where("id=?", 1).Update("maintenance", true).Error; err != nil {
		t.Fatal(err)
	}
	var source db.P4CommerceRule
	if err := db.GetIns().Where("kind=? AND id NOT LIKE ?", "point", "shop_schedule_%").First(&source).Error; err != nil {
		t.Fatal(err)
	}
	var plan growth74Plan
	if err := json.Unmarshal(growth74Parameters, &plan); err != nil {
		t.Fatal(err)
	}
	rules := p4CommerceRules{LiveEditable: true}
	for i, spec := range plan.Stores["trial"].Products {
		product := p4BaseProduct(fmt.Sprintf("trial82_pending_%d", i), fmt.Sprintf("%s,%d", spec.Reward, spec.Quantity), fmt.Sprintf("point,%d", spec.Price), int32(i+1), max(22, int32(spec.MinimumPlayerLevel)))
		product.Priority, product.MinTrialStars = int32(spec.Menu*1000)+int32(i), 30
		if spec.Menu == 4 {
			product.TotalLimit = 1
		}
		rules.Products = append(rules.Products, product)
	}
	if err := db.GetIns().Model(&source).Updates(map[string]any{"rules": adminJSON(rules), "frozen": nil}).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.PublishP7ReRush(f.ctx); err != nil {
		t.Fatal(err)
	}
	legacy := db.GetIns().Exec("UPDATE p7_rush_stage_rules SET reward=CONCAT('coin,',(2000+floor*500)*star,';point,',(50+floor*10)*star) WHERE kind=? AND source=?", constUtil.P7RushTrial, constUtil.P7ReRushSource)
	if legacy.Error != nil || legacy.RowsAffected != 306 {
		t.Fatal("old Re catalogue setup", legacy.RowsAffected, legacy.Error)
	}
	before, _ := f.profile(t)
	start, err := f.s.p7Start(f.ctx, f.id, &pb.CSRushFightStart{RushType: proto.Int32(1), TaskId: proto.String("local_rush_1"), Star: proto.Int32(1)}, 821001, false)
	if err != nil {
		t.Fatal("old Re start", err)
	}
	var fight db.P7RushFight
	if err = db.GetIns().First(&fight, "id=? AND player_id=?", start.GetFightId(), f.id).Error; err != nil {
		t.Fatal(err)
	}
	var frozen db.P7RushStageRule
	if err = json.Unmarshal(fight.Rule, &frozen); err != nil || frozen.Reward != "coin,2500;point,60" || p6Digest(fight.Rule) != fight.RuleHash {
		t.Fatal("old accepted reward was not frozen", frozen.Reward, err)
	}
	if err = f.s.ApplyTrial82Config(f.ctx, actor); err != nil {
		t.Fatal("trial82 maintenance migration", err)
	}
	var current db.P7RushStageRule
	if err = db.GetIns().First(&current, "id=?", "re-rush:1:1").Error; err != nil || current.Reward != "coin,2500;point,11" {
		t.Fatal("live stage was not migrated", current.Reward, err)
	}
	series, err := f.s.FreezeP6RushSeriesInput(start, true, false, nil, constUtil.P6OriginalClientHash)
	if err != nil {
		t.Fatal(err)
	}
	input, err := p7GenericMechanismRushInputAt(series, 0)
	if err != nil {
		t.Fatal(err)
	}
	client, err := f.s.ComputeOriginalP6ProtocolFinalHP(f.ctx, input, nil)
	if err != nil || client.Outcome.Authoritative || client.Outcome.Winner != 0 {
		t.Fatal("independent original client did not win", err)
	}
	end := &pb.CSRushFightEnd{FightId: start.FightId, Result: proto.Int32(1), OptRecord: client.OriginalOperates}
	first, handled, err := f.s.DispatchP7(f.ctx, f.id, end, 0)
	if err != nil || !handled || len(first) == 0 || first[0].(*pb.SCRushFightEnd).GetResult() != 1 {
		t.Fatal("old frozen fight did not settle", err)
	}
	after, _ := f.profile(t)
	if after.GetPoint()-before.GetPoint() != 60 {
		t.Fatal("old in-flight point reward changed", after.GetPoint()-before.GetPoint())
	}
	next, handled, err := f.s.DispatchP7(f.ctx, f.id, &pb.CSRushCurrentInfo{}, 0)
	if err != nil || !handled || len(next) != 1 {
		t.Fatal("next stage query", err)
	}
	tasks := next[0].(*pb.SCRushCurrentInfo).GetTaskInfoList()
	if len(tasks) != 3 || tasks[0].GetTaskId() != "local_rush_2" || tasks[0].GetPoint() != 12 {
		t.Fatal("next stage did not use migrated points", tasks)
	}
	trial82ColdOpen(t, f)
	replay, handled, err := f.s.DispatchP7(f.ctx, f.id, end, 0)
	if err != nil || !handled || len(replay) == 0 || !proto.Equal(first[0], replay[0]) {
		t.Fatal("cold old fight replay changed", err)
	}
	cold, _ := f.profile(t)
	if cold.GetPoint() != after.GetPoint() {
		t.Fatal("cold old fight replay granted twice")
	}
}
