package service

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"reflect"
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

func playerGrabMySQLFixture(t *testing.T) (*p1Fixture, int64, int64) {
	t.Helper()
	if os.Getenv("HYJS_P11_MYSQL_DATABASE") == "" {
		t.Skip("requires a coordinated isolated hyjs_test_p11_ MySQL database")
	}
	if os.Getenv("HYJS_GAME_ACCOUNTS_DATABASE") != "" {
		t.Fatal("requires an isolated game and accounts fixture")
	}
	f, b, c := p11ArenaSetup(t)
	pool, err := db.GetIns().DB()
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(8)
	pool.SetMaxIdleConns(8)
	return f, b, c
}

func playerGrabDisableRandom(t *testing.T, f *p1Fixture, at time.Time) {
	t.Helper()
	if err := f.s.InitializeP11GrabRandom(f.ctx); err != nil {
		t.Fatal(err)
	}
	database := db.GetIns()
	var previousPolicy db.P11GrabRandomPolicy
	if err := database.First(&previousPolicy, 1).Error; err != nil {
		t.Fatal(err)
	}
	month, _ := p11ArenaPrizeMonth(at)
	var previousOffer db.P11GrabRandomOffer
	offerErr := database.First(&previousOffer, "month=?", month).Error
	if offerErr != nil && !errors.Is(offerErr, gorm.ErrRecordNotFound) {
		t.Fatal(offerErr)
	}
	t.Cleanup(func() {
		if err := database.Save(&previousPolicy).Error; err != nil {
			t.Errorf("restore random policy: %v", err)
		}
		if err := database.Where("month=?", month).Delete(&db.P11GrabRandomOffer{}).Error; err != nil {
			t.Errorf("clear isolated random offer: %v", err)
		} else if offerErr == nil {
			if err := database.Create(&previousOffer).Error; err != nil {
				t.Errorf("restore random offer: %v", err)
			}
		}
	})
	if err := database.Model(&db.P11GrabRandomPolicy{}).Where("id=?", 1).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Where("month=?", month).Delete(&db.P11GrabRandomOffer{}).Error; err != nil {
		t.Fatal(err)
	}
}

type playerGrabResult struct {
	messages []proto.Message
	err      error
}

func playerGrabAwait(t *testing.T, label string, ctx context.Context, abort func(), call func(context.Context) ([]proto.Message, error)) []proto.Message {
	t.Helper()
	done := make(chan playerGrabResult, 1)
	go func() {
		messages, err := call(ctx)
		done <- playerGrabResult{messages: messages, err: err}
	}()
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	select {
	case result := <-done:
		if result.err != nil {
			t.Fatalf("%s: %v", label, result.err)
		}
		return result.messages
	case <-timer.C:
		abort()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Errorf("%s did not exit after releasing locks", label)
		}
		t.Fatalf("%s waited more than 2 seconds", label)
	case <-ctx.Done():
		abort()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Errorf("%s did not exit after cancellation", label)
		}
		t.Fatalf("%s context ended: %v", label, ctx.Err())
	}
	return nil
}

func TestPlayerConcurrencyMySQLGrabIsolation(t *testing.T) {
	f, b, c := playerGrabMySQLFixture(t)
	now := p11GrabPastDay()
	playerGrabDisableRandom(t, f, now)
	p11GrabPrepare(t, f, f.id, b, c)
	initial := p11GrabCall(t, f, f.id, &pb.CSGrab{Index: proto.Int32(1)}, 101, now).(*pb.SCGrab)
	if initial.GetGrabData().GetPlayerId() != f.id || initial.GetGrabData().GetPosition() != 1 {
		t.Fatal("first player did not occupy point 1")
	}
	database := db.GetIns()
	var original db.P11GrabSlot
	if err := database.First(&original, "position=?", 1).Error; err != nil {
		t.Fatal(err)
	}
	var originalSession db.P11GrabSession
	if err := database.First(&originalSession, "id=?", original.SessionID).Error; err != nil {
		t.Fatal(err)
	}
	if original.OwnerID == nil || *original.OwnerID != f.id || originalSession.EndedAt != 0 {
		t.Fatal("first player's occupation fixture is invalid")
	}

	ctx, cancel := context.WithTimeout(f.ctx, 30*time.Second)
	defer cancel()
	held := database.WithContext(ctx).Begin(&sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if held.Error != nil {
		t.Fatal(held.Error)
	}
	defer held.Rollback()
	var owner db.Profile
	if err := held.Clauses(clause.Locking{Strength: "UPDATE"}).First(&owner, f.id).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err := p11GrabConfig(held); err != nil {
		t.Fatal(err)
	}
	var locked db.P11GrabSlot
	if err := held.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, "position=?", 1).Error; err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(locked, original) {
		t.Fatal("held point differs from the accepted occupation")
	}
	abort := func() { held.Rollback(); cancel() }
	call := func(label string, id int64, request proto.Message, stamp int64, at time.Time) []proto.Message {
		return playerGrabAwait(t, label, ctx, abort, func(callCtx context.Context) ([]proto.Message, error) {
			return f.s.p11GrabAt(callCtx, id, request, stamp, at)
		})
	}
	second := call("second player occupation", b, &pb.CSGrab{Index: proto.Int32(2)}, 102, now)
	if len(second) != 1 {
		t.Fatal("second occupation returned no result")
	}
	secondGrab, ok := second[0].(*pb.SCGrab)
	if !ok || secondGrab.GetGrabData().GetPlayerId() != b || secondGrab.GetGrabData().GetPosition() != 2 {
		t.Fatal("second player did not occupy point 2")
	}
	firstList := call("third player list", c, &pb.CSGetGrabData{}, 103, now)
	if len(firstList) != 1 {
		t.Fatal("third player list returned no result")
	}
	list, ok := firstList[0].(*pb.SCGetGrabData)
	if !ok || len(list.GrabData) != 18 || list.GrabData[0].GetPlayerId() != f.id || list.GrabData[1].GetPlayerId() != b {
		t.Fatal("list did not show the two committed occupations")
	}
	firstInfo := call("third player time reward", c, &pb.CSGetTimeRewardInfo{}, 104, now)
	if len(firstInfo) != 1 {
		t.Fatal("third player time reward returned no result")
	}
	if info, ok := firstInfo[0].(*pb.SCGetTimeRewardInfo); !ok || info.GetMyGrabMinutes() != 0 {
		t.Fatal("third player's personal time was not empty")
	}

	afterExpiry := now.Add(3*time.Hour + time.Second)
	third := call("second player after expiration", b, &pb.CSGrab{Index: proto.Int32(3)}, 105, afterExpiry)
	if len(third) != 1 {
		t.Fatal("second player post-expiry occupation returned no result")
	}
	thirdGrab, ok := third[0].(*pb.SCGrab)
	if !ok || thirdGrab.GetGrabData().GetPlayerId() != b || thirdGrab.GetGrabData().GetPosition() != 3 {
		t.Fatal("second player did not occupy a separate point after expiry")
	}
	expiredList := call("third player expired list", c, &pb.CSGetGrabData{}, 106, afterExpiry)
	if len(expiredList) != 1 {
		t.Fatal("expired list returned no result")
	}
	list, ok = expiredList[0].(*pb.SCGetGrabData)
	if !ok || len(list.GrabData) != 18 || list.GrabData[0].GetPlayerId() != 0 || list.GrabData[0].GetIsGrab() {
		t.Fatal("expired point was not displayed as empty")
	}
	call("third player time reward after expiration", c, &pb.CSGetTimeRewardInfo{}, 107, afterExpiry)
	var stillHeld db.P11GrabSlot
	var stillOpen db.P11GrabSession
	if err := database.First(&stillHeld, "position=?", 1).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.First(&stillOpen, "id=?", original.SessionID).Error; err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(original, stillHeld) || !reflect.DeepEqual(originalSession, stillOpen) {
		t.Fatal("other players changed the held point or its session")
	}
	if err := held.Commit().Error; err != nil {
		t.Fatal(err)
	}
	reclaimed := p11GrabCall(t, f, c, &pb.CSGrab{Index: proto.Int32(1)}, 108, afterExpiry).(*pb.SCGrab)
	if reclaimed.GetGrabData().GetPlayerId() != c || reclaimed.GetGrabData().GetPosition() != 1 {
		t.Fatal("the displayed expired position was not available on its first admission")
	}
	info := p11GrabCall(t, f, f.id, &pb.CSGetTimeRewardInfo{}, 109, afterExpiry).(*pb.SCGetTimeRewardInfo)
	if info.GetMyGrabMinutes() != 180 {
		t.Fatalf("expired owner time = %d minutes, want 180", info.GetMyGrabMinutes())
	}
	var closed db.P11GrabSession
	var vacant db.P11GrabSlot
	if err := database.First(&closed, "id=?", original.SessionID).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.First(&vacant, "position=?", 1).Error; err != nil {
		t.Fatal(err)
	}
	if closed.EndedAt != original.ExpiresAt || closed.EndReason != "expired" || vacant.OwnerID == nil || *vacant.OwnerID != c {
		t.Fatal("position admission did not close the old session at its deadline")
	}
	again := p11GrabCall(t, f, f.id, &pb.CSGetTimeRewardInfo{}, 110, afterExpiry.Add(time.Hour)).(*pb.SCGetTimeRewardInfo)
	if again.GetMyGrabMinutes() != 180 {
		t.Fatal("expired session accrued additional time")
	}
}

type playerGrabRaceKey struct{}

func TestPlayerConcurrencyMySQLGrabSameSlot(t *testing.T) {
	f, b, _ := playerGrabMySQLFixture(t)
	now := p11GrabPastDay()
	playerGrabDisableRandom(t, f, now)
	ids := []int64{f.id, b}
	p11GrabPrepare(t, f, ids...)
	for _, id := range ids {
		if err := updateProfile(f.ctx, id, func(player *pb.Player, _ *model.GameState) error {
			player.Energy = proto.Int32(100)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	type result struct {
		id    int64
		stamp int64
		err   error
	}
	ctx, cancel := context.WithTimeout(f.ctx, 12*time.Second)
	start, ready := make(chan struct{}), make(chan struct{}, len(ids))
	observed, continueObserved := make(chan bool, len(ids)), make(chan struct{})
	done := make(chan result, len(ids))
	var startOnce, continueOnce sync.Once
	var seen sync.Map
	release := func() { startOnce.Do(func() { close(start) }) }
	continueRace := func() { continueOnce.Do(func() { close(continueObserved) }) }
	const callback = "test:grab-race-observed-empty"
	database := db.GetIns()
	if err := database.Callback().Query().After("gorm:query").Register(callback, func(tx *gorm.DB) {
		id, ok := tx.Statement.Context.Value(playerGrabRaceKey{}).(int64)
		if !ok || tx.Statement.Table != "p11_grab_slots" || tx.Error != nil {
			return
		}
		slot, ok := tx.Statement.Dest.(*db.P11GrabSlot)
		if !ok || slot.Position != 1 {
			return
		}
		if _, loaded := seen.LoadOrStore(id, true); loaded {
			return
		}
		observed <- slot.OwnerID == nil
		select {
		case <-continueObserved:
		case <-ctx.Done():
		}
	}); err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Callback().Query().Remove(callback) })
	var group sync.WaitGroup
	defer func() { release(); continueRace(); cancel(); group.Wait() }()
	for i, id := range ids {
		stamp := int64(201 + i)
		group.Add(1)
		go func(id, stamp int64) {
			defer group.Done()
			ready <- struct{}{}
			select {
			case <-start:
			case <-ctx.Done():
				done <- result{id: id, stamp: stamp, err: ctx.Err()}
				return
			}
			callCtx := context.WithValue(ctx, playerGrabRaceKey{}, id)
			_, err := f.s.p11GrabAt(callCtx, id, &pb.CSGrab{Index: proto.Int32(1)}, stamp, now)
			done <- result{id: id, stamp: stamp, err: err}
		}(id, stamp)
	}
	for range ids {
		select {
		case <-ready:
		case <-ctx.Done():
			t.Fatal("same-point contenders did not become ready")
		}
	}
	release()
	for range ids {
		select {
		case empty := <-observed:
			if !empty {
				t.Fatal("contender did not observe the same empty point")
			}
		case <-ctx.Done():
			t.Fatal("contenders did not both observe the empty point")
		}
	}
	continueRace()
	var results []result
	for range ids {
		select {
		case value := <-done:
			results = append(results, value)
		case <-ctx.Done():
			t.Fatal("same-point contenders did not complete")
		}
	}
	winner, loser := int64(0), int64(0)
	var winnerStamp, loserStamp int64
	for _, value := range results {
		if value.err == nil {
			if winner != 0 {
				t.Fatal("both contenders occupied the same point")
			}
			winner, winnerStamp = value.id, value.stamp
		} else {
			loser, loserStamp = value.id, value.stamp
		}
	}
	if winner == 0 || loser == 0 {
		t.Fatal("same-point race did not have exactly one winner")
	}
	var slot db.P11GrabSlot
	if err := db.GetIns().First(&slot, "position=?", 1).Error; err != nil {
		t.Fatal(err)
	}
	if slot.OwnerID == nil || *slot.OwnerID != winner {
		t.Fatal("point owner did not match the successful contender")
	}
	winnerPlayer, _ := p10LuckyProfile(t, winner)
	loserPlayer, _ := p10LuckyProfile(t, loser)
	if winnerPlayer.GetEnergy() != 90 || loserPlayer.GetEnergy() != 100 {
		t.Fatal("same-point race charged the wrong amount of energy")
	}
	if p1Count(t, &db.P11ArenaReceipt{}, "player_id=? AND operation=? AND stamp=?", winner, "grab-empty", winnerStamp) != 1 ||
		p1Count(t, &db.P11ArenaReceipt{}, "player_id=? AND operation=? AND stamp=?", loser, "grab-empty", loserStamp) != 0 ||
		p1Count(t, &db.P11GrabSession{}, "player_id=?", winner) != 1 ||
		p1Count(t, &db.P11GrabSession{}, "player_id=?", loser) != 0 {
		t.Fatal("same-point race persisted an incorrect receipt or session")
	}
}

type playerGrabPolicyWriterKey struct{}

func TestPlayerConcurrencyMySQLGrabFrozenOffers(t *testing.T) {
	f, b, _ := playerGrabMySQLFixture(t)
	at := p11GrabRandomTestAt()
	playerGrabDisableRandom(t, f, at)
	p11GrabRandomPublish(t, at)
	database := db.GetIns()
	var beforeA, beforeB db.Profile
	if err := database.First(&beforeA, f.id).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.First(&beforeB, b).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p11GrabRandomAt(f.ctx, f.id, 0, false, at); err != nil {
		t.Fatal(err)
	}
	month, _ := p11ArenaPrizeMonth(at)
	var frozen db.P11GrabRandomOffer
	if err := database.First(&frozen, "month=?", month).Error; err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(f.ctx, 20*time.Second)
	defer cancel()
	held := database.WithContext(ctx).Begin(&sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if held.Error != nil {
		t.Fatal(held.Error)
	}
	defer held.Rollback()
	if _, _, err := p11GrabConfig(held); err != nil {
		t.Fatal(err)
	}
	offer, err := f.s.p11GrabRandomOffer(held, at)
	if err != nil || offer == nil || !reflect.DeepEqual(*offer, frozen) {
		t.Fatalf("frozen offer could not be held: %v", err)
	}
	abort := func() { held.Rollback(); cancel() }
	response := playerGrabAwait(t, "other player's frozen random offer", ctx, abort, func(callCtx context.Context) ([]proto.Message, error) {
		return f.s.p11GrabRandomAt(callCtx, b, 0, false, at)
	})
	if len(response) != 1 {
		t.Fatal("random reward query returned no result")
	}
	points, ok := response[0].(*pb.SCRedPoint)
	if !ok || len(points.GetRedPointList()) != 1 || points.GetRedPointList()[0].GetType() != constUtil.P11GrabRandomRedPoint || points.GetRedPointList()[0].GetValue() != 0 {
		t.Fatal("random reward query returned the wrong personal red point")
	}
	entered := make(chan struct{})
	var enteredOnce sync.Once
	const callback = "test:grab-random-policy-writer"
	if err := database.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Context.Value(playerGrabPolicyWriterKey{}) == true && tx.Statement.Table == "p11_grab_random_policies" {
			enteredOnce.Do(func() { close(entered) })
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Callback().Update().Remove(callback) })
	writerDone := make(chan error, 1)
	writerReceived := false
	defer func() {
		abort()
		if !writerReceived {
			select {
			case <-writerDone:
			case <-time.After(3 * time.Second):
				t.Error("random policy writer did not exit")
			}
		}
	}()
	go func() {
		writerCtx := context.WithValue(ctx, playerGrabPolicyWriterKey{}, true)
		writerDone <- database.WithContext(writerCtx).Model(&db.P11GrabRandomPolicy{}).Where("id=?", 1).Update("daily_limit", 2).Error
	}()
	select {
	case <-entered:
	case err := <-writerDone:
		writerReceived = true
		t.Fatalf("policy writer ended before attempting its update: %v", err)
	case <-ctx.Done():
		t.Fatal("policy writer did not start")
	}
	select {
	case err := <-writerDone:
		writerReceived = true
		t.Fatalf("policy writer did not wait for the frozen offer reader: %v", err)
	case <-time.After(200 * time.Millisecond):
	}
	if err := held.Commit().Error; err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-writerDone:
		writerReceived = true
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("policy writer did not complete after release")
	}
	var afterA, afterB db.Profile
	var afterOffer db.P11GrabRandomOffer
	var changed db.P11GrabRandomPolicy
	for _, query := range []func() error{
		func() error { return database.First(&afterA, f.id).Error },
		func() error { return database.First(&afterB, b).Error },
		func() error { return database.First(&afterOffer, "month=?", month).Error },
		func() error { return database.First(&changed, 1).Error },
	} {
		if err := query(); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(beforeA, afterA) || !reflect.DeepEqual(beforeB, afterB) || !reflect.DeepEqual(frozen, afterOffer) || changed.DailyLimit != 2 ||
		p1Count(t, &db.P11GrabRandomAward{}, "month=? AND player_id IN ?", month, []int64{f.id, b}) != 0 {
		t.Fatal("frozen offer query changed assets or the frozen offer")
	}
}
