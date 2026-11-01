package service

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

type copyFormationMySQLRoleKey struct{}

type copyFormationMySQLResult struct {
	role     string
	messages []proto.Message
	err      error
}

// copyFormationMySQLRace pauses the winning real service transaction after
// its profile SELECT FOR UPDATE, and observes the other request reach the same
// locking query before releasing it. No service behavior is replaced.
func copyFormationMySQLRace(t *testing.T, f *p1Fixture, winner string, start *pb.CSCopyFightStart, change *pb.CSFormationOn, stamp int64) (map[string]copyFormationMySQLResult, *db.Profile) {
	t.Helper()
	database := db.GetIns()
	ctx, cancel := context.WithTimeout(f.ctx, 20*time.Second)
	defer cancel()
	ctx = context.WithValue(ctx, constUtil.GameServiceContextKey, f.s)
	loser := "start"
	if winner == "start" {
		loser = "change"
	}
	held := make(chan struct{})
	release := make(chan struct{})
	waiting := make(chan struct{}, 1)
	changeSnapshot := make(chan db.Profile, 1)
	var releaseOnce, heldOnce sync.Once
	lockedProfile := func(tx *gorm.DB) bool {
		if tx.Statement.Table != "profiles" {
			return false
		}
		entry, ok := tx.Statement.Clauses["FOR"]
		if !ok {
			return false
		}
		lock, ok := entry.Expression.(clause.Locking)
		return ok && lock.Strength == "UPDATE"
	}
	beforeName, afterName := "copy_formation_mysql_before", "copy_formation_mysql_after"
	if err := database.Callback().Query().Before("gorm:query").Register(beforeName, func(tx *gorm.DB) {
		if tx.Statement.Context.Value(copyFormationMySQLRoleKey{}) == loser && lockedProfile(tx) {
			select {
			case waiting <- struct{}{}:
			default:
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	if err := database.Callback().Query().After("gorm:query").Register(afterName, func(tx *gorm.DB) {
		if tx.Error != nil || !lockedProfile(tx) {
			return
		}
		role, _ := tx.Statement.Context.Value(copyFormationMySQLRoleKey{}).(string)
		row, ok := tx.Statement.Dest.(*db.Profile)
		if !ok || row.Id != f.id {
			return
		}
		if role == "change" && winner == "start" {
			select {
			case changeSnapshot <- *row:
			default:
			}
		}
		if role == winner {
			heldOnce.Do(func() {
				close(held)
				select {
				case <-release:
				case <-ctx.Done():
				}
			})
		}
	}); err != nil {
		_ = database.Callback().Query().Remove(beforeName)
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	defer func() {
		releaseOnce.Do(func() { close(release) })
		cancel()
		workers.Wait()
		_ = database.Callback().Query().Remove(afterName)
		_ = database.Callback().Query().Remove(beforeName)
	}()
	results := make(chan copyFormationMySQLResult, 2)
	run := func(role string) {
		requestCtx := context.WithValue(ctx, copyFormationMySQLRoleKey{}, role)
		if role == "start" {
			requestCtx = context.WithValue(requestCtx, constUtil.GameRequestContextKey, start)
			messages, err := f.s.copyFightStart(requestCtx, f.id, start)
			results <- copyFormationMySQLResult{role: role, messages: messages, err: err}
			return
		}
		requestCtx = context.WithValue(requestCtx, constUtil.GameRequestContextKey, change)
		messages, handled, err := f.s.DispatchP5Next(requestCtx, f.id, change, stamp)
		if !handled && err == nil {
			err = errors.New("original formation request was not dispatched")
		}
		results <- copyFormationMySQLResult{role: role, messages: messages, err: err}
	}
	launch := func(role string) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			run(role)
		}()
	}
	launch(winner)
	select {
	case <-held:
	case early := <-results:
		t.Fatalf("%s finished before acquiring the profile row: %v", early.role, early.err)
	case <-ctx.Done():
		t.Fatal("winner did not acquire the profile row")
	}
	launch(loser)
	select {
	case <-waiting:
	case early := <-results:
		t.Fatalf("%s finished before the competing locking query: %v", early.role, early.err)
	case <-ctx.Done():
		t.Fatal("competing request did not reach the profile lock")
	}
	select {
	case early := <-results:
		t.Fatalf("%s completed while the profile lock was held: %v", early.role, early.err)
	case <-time.After(200 * time.Millisecond):
	}
	releaseOnce.Do(func() { close(release) })
	out := map[string]copyFormationMySQLResult{}
	for len(out) < 2 {
		select {
		case result := <-results:
			out[result.role] = result
		case <-ctx.Done():
			t.Fatal("competing requests did not finish after profile unlock")
		}
	}
	var snapshot *db.Profile
	if winner == "start" {
		select {
		case row := <-changeSnapshot:
			snapshot = &row
		default:
			t.Fatal("did not observe the rejected request's locked profile")
		}
	}
	return out, snapshot
}

// TestCopyFormationContinuityMySQLInterleaving uses one isolated role. First,
// a legal change wins before the first fight freezes a party. After settling
// that fight, the next fight wins the lock and a competing change rolls back.
func TestCopyFormationContinuityMySQLInterleaving(t *testing.T) {
	name := os.Getenv("HYJS_P1_MYSQL_DATABASE")
	if name == "" {
		t.Skip("isolated managed MySQL allocation required")
	}
	if !strings.HasPrefix(name, "hyjs_test_p1_") || len(name) == len("hyjs_test_p1_") || os.Getenv("HYJS_GAME_ACCOUNTS_DATABASE") != "" {
		t.Fatal("a dedicated isolated P1 database and local accounts are required")
	}
	f := p1Setup(t)
	database := db.GetIns()
	if database.Dialector.Name() != "mysql" {
		t.Fatal("MySQL is required")
	}
	var selected, engine string
	if err := database.Raw("SELECT DATABASE()").Scan(&selected).Error; err != nil || selected != name {
		t.Fatal("connection does not target the owned test database")
	}
	if err := database.Raw("SELECT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='profiles'").Scan(&engine).Error; err != nil || engine != "InnoDB" {
		t.Fatal("profiles must use InnoDB")
	}
	sqlDB, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(4)
	sqlDB.SetMaxIdleConns(4)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, initialize := range []func(context.Context) error{
		f.s.InitializeP5, f.s.InitializeOpenServerActivity, f.s.InitializeDailyTasks,
		f.s.InitializeCopyDrops, f.s.InitializeDailyCopy, f.s.InitCooking,
	} {
		if err := initialize(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	f.s.P5Ready = true
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level, p.Stamina, p.WarriorBagSize = proto.Int32(100), proto.Int32(1000), proto.Int32(1000)
		_, err := f.s.GrantFixedReward(p, g, "hero_020,1;hero_011,1")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	_, game := f.profile(t)
	var firstHero, secondHero *pb.Warrior
	for _, hero := range game.Warriors {
		switch hero.GetId() {
		case "hero_020":
			firstHero = hero
		case "hero_011":
			secondHero = hero
		}
	}
	if firstHero == nil || secondHero == nil {
		t.Fatal("isolated owned heroes missing")
	}
	firstOn := &pb.CSFormationOn{Id: firstHero.Id, Guid: firstHero.Guid, FormatPos: proto.Int32(2)}
	requestCtx := context.WithValue(context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s), constUtil.GameRequestContextKey, firstOn)
	if _, err := f.s.p5FormationOnNative(requestCtx, f.id, firstOn, 880001); err != nil {
		t.Fatal(err)
	}
	beforeEnter, _ := f.profile(t)
	enter := &pb.CSCopyEnter{ChapterId: proto.String("1"), MissionId: proto.String("1")}
	if _, err := f.s.copyEnter(f.ctx, f.id, enter); err != nil {
		t.Fatal(err)
	}
	afterEnter, game := f.profile(t)
	cost := int32(tableNumber(f.s.table("CopyMission", "MissionId", "1"), "PowerCost"))
	if cost <= 0 || afterEnter.GetStamina() != beforeEnter.GetStamina()-cost || game.Run == nil || game.Run.Party != nil {
		t.Fatal("paid run was not created with an unfrozen party")
	}
	tasks := f.s.tasks("1")
	if len(tasks) < 2 || tasks[0]["TaskId"] != "1" {
		t.Fatal("expected the original multi-encounter mission")
	}
	startRequest := func(task string) *pb.CSCopyFightStart {
		return &pb.CSCopyFightStart{ChapterId: proto.String("1"), MissionId: proto.String("1"), TaskId: proto.String(task)}
	}
	secondOn := &pb.CSFormationOn{Id: secondHero.Id, Guid: secondHero.Guid, FormatPos: proto.Int32(2)}
	firstRace, _ := copyFormationMySQLRace(t, f, "change", startRequest(tasks[0]["TaskId"]), secondOn, 880002)
	if result := firstRace["change"]; result.err != nil || len(result.messages) == 0 {
		t.Fatalf("first formation change failed: %v", result.err)
	} else if _, ok := result.messages[0].(*pb.SCFormationOn); !ok {
		t.Fatal("unexpected formation response")
	}
	firstStartResult := firstRace["start"]
	if firstStartResult.err != nil || len(firstStartResult.messages) == 0 {
		t.Fatalf("first fight did not start: %v", firstStartResult.err)
	}
	firstStart, ok := firstStartResult.messages[0].(*pb.SCCopyFightStart)
	if !ok {
		t.Fatal("unexpected first fight response")
	}
	contains := func(party *pb.BattlePlayer, guid string) bool {
		for _, hero := range party.GetFormation() {
			if hero.GetGuid() == guid {
				return true
			}
		}
		return false
	}
	player, game := f.profile(t)
	if !contains(firstStart.GetBattlePlayer(), secondHero.GetGuid()) || contains(firstStart.GetBattlePlayer(), firstHero.GetGuid()) ||
		game.Run.Party == nil || !contains(game.Run.Party, secondHero.GetGuid()) || game.Run.Fight == nil ||
		!proto.Equal(game.Run.Fight.Start, firstStart) || player.GetStamina() != afterEnter.GetStamina() {
		t.Fatal("first fight did not freeze the committed formation or retained stamina")
	}
	end := &pb.CSCopyFightEnd{FightId: firstStart.FightId, Result: proto.Int32(1), BattlePlayer: proto.Clone(firstStart.BattlePlayer).(*pb.BattlePlayer)}
	ended, err := f.s.copyFightEnd(f.ctx, f.id, end)
	if err != nil || len(ended) == 0 {
		t.Fatalf("first fight settlement: %v", err)
	}
	firstEnd, ok := ended[0].(*pb.SCCopyFightEnd)
	if !ok || firstEnd.GetGain() == nil {
		t.Fatal("missing accepted first-fight reward")
	}
	var rewardGUID string
	for _, hero := range firstEnd.GetGain().GetWars() {
		if hero.GetId() == "hero_exp_006" {
			rewardGUID = hero.GetGuid()
		}
	}
	if rewardGUID == "" {
		t.Fatal("original guaranteed first-fight reward missing")
	}
	player, game = f.profile(t)
	if game.Run == nil || game.Run.Party == nil || game.Run.Fight == nil || !game.Run.Fight.Settled || len(game.Run.Passed) != 1 ||
		player.GetStamina() != afterEnter.GetStamina() {
		t.Fatal("first settlement or paid run not persisted")
	}
	frozenParty := proto.Clone(game.Run.Party).(*pb.BattlePlayer)
	frozenGain := proto.Clone(game.Run.Gain).(*pb.ResultObject)
	// The accepted result is replayable before the next fight and cannot grant
	// its guaranteed reward twice.
	replayed, err := f.s.copyFightEnd(f.ctx, f.id, end)
	if err != nil || len(replayed) == 0 || !proto.Equal(replayed[0], firstEnd) {
		t.Fatal("accepted first-fight receipt did not replay", err)
	}
	rejectedStamp := int64(880003)
	secondRace, snapshot := copyFormationMySQLRace(t, f, "start", startRequest(tasks[1]["TaskId"]), firstOn, rejectedStamp)
	secondStartResult := secondRace["start"]
	if secondStartResult.err != nil || len(secondStartResult.messages) == 0 {
		t.Fatalf("second fight did not start: %v", secondStartResult.err)
	}
	secondStart, ok := secondStartResult.messages[0].(*pb.SCCopyFightStart)
	if !ok || !contains(secondStart.GetBattlePlayer(), secondHero.GetGuid()) || contains(secondStart.GetBattlePlayer(), firstHero.GetGuid()) {
		t.Fatal("second fight did not retain the established formation")
	}
	if result := secondRace["change"]; result.err == nil || result.err.Error() != constUtil.CopyContinuityLineupChanged {
		t.Fatalf("concurrent replacement should be rejected: %v", result.err)
	}
	var stored db.Profile
	if err := database.First(&stored, f.id).Error; err != nil || !reflect.DeepEqual(*snapshot, stored) {
		t.Fatal("rejected replacement changed the locked profile", err)
	}
	var receiptCount, successfulReceiptCount int64
	if err := database.Model(&db.P5MutationReceipt{}).Where("player_id=? AND operation=? AND timestamp=?", f.id, constUtil.P5FormationOnOperation, rejectedStamp).Count(&receiptCount).Error; err != nil || receiptCount != 0 {
		t.Fatal("rejected replacement saved a mutation receipt", err)
	}
	if err := database.Model(&db.P5MutationReceipt{}).Where("player_id=? AND operation=? AND timestamp=?", f.id, constUtil.P5FormationOnOperation, 880002).Count(&successfulReceiptCount).Error; err != nil || successfulReceiptCount != 1 {
		t.Fatal("previous accepted formation receipt was lost or duplicated", err)
	}
	player, game = f.profile(t)
	if game.Run == nil || !proto.Equal(game.Run.Party, frozenParty) || !proto.Equal(game.Run.Gain, frozenGain) ||
		game.Run.Fight == nil || game.Run.Fight.Settled || !proto.Equal(game.Run.Fight.Start, secondStart) ||
		len(game.Run.Passed) != 1 || player.GetStamina() != afterEnter.GetStamina() {
		t.Fatal("rejected replacement lost the accepted fight, prior reward, or stamina")
	}
	var owned int
	for _, hero := range game.Warriors {
		if hero.GetGuid() == rewardGUID && hero.GetId() == "hero_exp_006" {
			owned++
		}
	}
	if owned != 1 {
		t.Fatal("first-fight reward instance was lost or duplicated")
	}
	t.Log("both lock orders completed for one isolated player; first reward hero_exp_006 retained exactly once")
}
