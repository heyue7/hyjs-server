package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
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

func TestPlayerConcurrencyMySQLP7PolicyIsolation(t *testing.T) {
	name := os.Getenv("HYJS_P7_MYSQL_DATABASE")
	if name == "" {
		t.Skip("requires a coordinated isolated hyjs_test_p7_ MySQL database")
	}
	if !strings.HasPrefix(name, "hyjs_test_p7_") || os.Getenv("HYJS_GAME_ACCOUNTS_DATABASE") != "" {
		t.Fatal("requires an isolated game and accounts fixture")
	}

	f := p7Setup(t)
	database := db.GetIns()
	if database.Dialector.Name() != "mysql" {
		t.Fatal("requires MySQL for policy row locking")
	}
	pool, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(6)
	pool.SetMaxIdleConns(6)

	account, err := LoginAccount(f.ctx, fmt.Sprintf("p7-policy-peer-%d", time.Now().UnixNano()), "isolated-p7-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CreatePlayer(f.ctx, account.Id, "试炼政策并发", "hero_047"); err != nil {
		t.Fatal(err)
	}
	peer := account.Id
	if err := updateProfile(f.ctx, peer, func(player *pb.Player, _ *model.GameState) error {
		player.Level = proto.Int32(22)
		player.Diamond = proto.Int64(1000)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var beforeState db.P7RushState
	if err := database.First(&beforeState, "player_id=?", peer).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("read initial player B rush state: %v", err)
	}
	if beforeState.EliteBought != 0 {
		t.Fatalf("new player already has %d elite purchases", beforeState.EliteBought)
	}
	for _, update := range []struct {
		table  any
		id     int64
		values map[string]any
	}{
		{&db.P7RushPolicy{}, constUtil.P7RushPolicyID, map[string]any{"enabled": true}},
		{&db.P7ReRushPolicy{}, 1, map[string]any{"enabled": true, "source": constUtil.P7ReRushSource}},
	} {
		result := database.Model(update.table).Where("id=?", update.id).Updates(update.values)
		if result.Error != nil || result.RowsAffected != 1 {
			t.Fatalf("enable isolated policy: rows=%d error=%v", result.RowsAffected, result.Error)
		}
	}

	ctx, cancel := context.WithTimeout(f.ctx, 20*time.Second)
	type buyResult struct {
		messages []proto.Message
		err      error
	}
	type updateResult struct {
		rows int64
		err  error
	}
	type adminMarkerKey struct{}
	marker := new(int)
	var pendingBuy <-chan buyResult
	var pendingAdmin <-chan updateResult
	var held *gorm.DB
	var locked, registered bool
	const callbackName = "test:player-concurrency-p7-policy-admin"
	defer func() {
		if locked {
			if err := held.Rollback().Error; err != nil && !errors.Is(err, sql.ErrTxDone) {
				t.Errorf("release policy transaction: %v", err)
			}
		}
		cancel()
		if pendingBuy != nil {
			<-pendingBuy
		}
		if pendingAdmin != nil {
			<-pendingAdmin
		}
		if registered {
			if err := database.Callback().Update().Remove(callbackName); err != nil {
				t.Errorf("remove policy callback: %v", err)
			}
		}
	}()

	held = database.WithContext(ctx).Begin()
	if held.Error != nil {
		t.Fatalf("begin first player transaction: %v", held.Error)
	}
	locked = true
	if err := ServerGameClockGuardTransaction(held); err != nil {
		t.Fatal(err)
	}
	var firstProfile db.Profile
	if err := held.Clauses(clause.Locking{Strength: "UPDATE"}).First(&firstProfile, f.id).Error; err != nil {
		t.Fatalf("lock first player profile: %v", err)
	}
	policy, err := f.s.p7Policy(held)
	if err != nil || !policy.Enabled || policy.EliteBuyCost <= 0 {
		t.Fatalf("hold enabled P7 policy: %+v, %v", policy, err)
	}

	request := &pb.CSRushEliteBossBuyTime{}
	const stamp int64 = 171001
	startBuy := func(requestStamp int64) <-chan buyResult {
		out := make(chan buyResult, 1)
		go func() {
			messages, err := f.s.P7RushMutation(ctx, peer, request, requestStamp)
			out <- buyResult{messages: messages, err: err}
		}()
		return out
	}
	waitBuy := func(ch <-chan buyResult, label string) buyResult {
		t.Helper()
		timer := time.NewTimer(2 * time.Second)
		defer timer.Stop()
		select {
		case result := <-ch:
			return result
		case <-timer.C:
			t.Fatalf("%s waited for another player's policy lease", label)
		case <-ctx.Done():
			t.Fatalf("%s timed out: %v", label, ctx.Err())
		}
		return buyResult{}
	}
	checkState := func() {
		t.Helper()
		for _, expected := range []struct {
			id      int64
			diamond int64
		}{{f.id, 1000}, {peer, 1000 - int64(policy.EliteBuyCost)}} {
			var row db.Profile
			if err := database.WithContext(ctx).First(&row, expected.id).Error; err != nil {
				t.Fatalf("read player profile: %v", err)
			}
			player, _, err := decodeProfile(&row)
			if err != nil || player.GetDiamond() != expected.diamond {
				t.Fatalf("player diamond mismatch: got=%d want=%d error=%v", player.GetDiamond(), expected.diamond, err)
			}
		}
		var state db.P7RushState
		if err := database.WithContext(ctx).First(&state, "player_id=?", peer).Error; err != nil || state.EliteBought != beforeState.EliteBought+1 {
			t.Fatalf("player B elite purchase count: %d, %v", state.EliteBought, err)
		}
		var count int64
		if err := database.WithContext(ctx).Model(&db.P7RushReceipt{}).Where("player_id=? AND domain=?", peer, "elite-buy").Count(&count).Error; err != nil || count != 1 {
			t.Fatalf("player B purchase receipts: %d, %v", count, err)
		}
	}

	pendingBuy = startBuy(stamp)
	first := waitBuy(pendingBuy, "player B purchase")
	pendingBuy = nil
	if first.err != nil || len(first.messages) != 1 {
		t.Fatalf("player B purchase: %v", first.err)
	}
	response, ok := first.messages[0].(*pb.SCRushEliteBossBuyTime)
	if !ok || response == nil || len(response.GetCost()) != 1 || response.GetCost()[0].GetId() != "diamond" || response.GetCost()[0].GetNum() != policy.EliteBuyCost {
		t.Fatal("player B purchase did not return the configured diamond cost")
	}
	checkState()
	pendingBuy = startBuy(stamp)
	replay := waitBuy(pendingBuy, "player B purchase replay")
	pendingBuy = nil
	if replay.err != nil || len(replay.messages) != 1 || !proto.Equal(first.messages[0], replay.messages[0]) {
		t.Fatalf("player B purchase replay changed the result: %v", replay.err)
	}
	checkState()

	entered := make(chan struct{})
	var enteredOnce sync.Once
	if err := database.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == "p7_rush_policies" && tx.Statement.Context.Value(adminMarkerKey{}) == marker {
			enteredOnce.Do(func() { close(entered) })
		}
	}); err != nil {
		t.Fatalf("register policy update callback: %v", err)
	}
	registered = true
	adminCtx := context.WithValue(ctx, adminMarkerKey{}, marker)
	adminCh := make(chan updateResult, 1)
	pendingAdmin = adminCh
	go func() {
		result := database.WithContext(adminCtx).Model(&db.P7RushPolicy{}).
			Where("id=?", constUtil.P7RushPolicyID).Update("enabled", false)
		adminCh <- updateResult{rows: result.RowsAffected, err: result.Error}
	}()
	select {
	case <-entered:
	case result := <-pendingAdmin:
		pendingAdmin = nil
		t.Fatalf("policy update ended before reaching the database: %v", result.err)
	case <-ctx.Done():
		t.Fatalf("policy update did not start: %v", ctx.Err())
	}
	timer := time.NewTimer(200 * time.Millisecond)
	defer timer.Stop()
	select {
	case result := <-pendingAdmin:
		pendingAdmin = nil
		t.Fatalf("policy update passed the held shared lease: rows=%d error=%v", result.rows, result.err)
	case <-timer.C:
	case <-ctx.Done():
		t.Fatalf("policy update wait timed out: %v", ctx.Err())
	}
	if err := held.Commit().Error; err != nil {
		t.Fatalf("commit first player transaction: %v", err)
	}
	locked = false
	select {
	case result := <-pendingAdmin:
		pendingAdmin = nil
		if result.err != nil || result.rows != 1 {
			t.Fatalf("policy update after release: rows=%d error=%v", result.rows, result.err)
		}
	case <-ctx.Done():
		t.Fatalf("policy update did not finish after release: %v", ctx.Err())
	}
	var closed db.P7RushPolicy
	if err := database.WithContext(ctx).First(&closed, constUtil.P7RushPolicyID).Error; err != nil || closed.Enabled {
		t.Fatalf("policy did not close: enabled=%t error=%v", closed.Enabled, err)
	}
	_, err = f.s.P7RushMutation(ctx, peer, request, stamp+1)
	if err == nil || err.Error() != constUtil.P7RushGateClosed {
		t.Fatalf("purchase after policy closure was not rejected by the gate: %v", err)
	}
	checkState()
}
