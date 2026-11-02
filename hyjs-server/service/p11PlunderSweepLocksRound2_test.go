package service

import (
	"context"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/config"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

type p11SweepRound2LockKey struct{}

func p11SweepRound2HasID(expr clause.Expression, id int64) bool {
	switch value := expr.(type) {
	case clause.Where:
		for _, nested := range value.Exprs {
			if p11SweepRound2HasID(nested, id) {
				return true
			}
		}
	case clause.AndConditions:
		for _, nested := range value.Exprs {
			if p11SweepRound2HasID(nested, id) {
				return true
			}
		}
	case clause.IN:
		column, ok := value.Column.(clause.Column)
		if !ok || column.Name != clause.PrimaryKey && column.Name != "id" {
			return false
		}
		for _, item := range value.Values {
			if candidate, ok := item.(int64); ok && candidate == id {
				return true
			}
		}
	case clause.Eq:
		column, ok := value.Column.(clause.Column)
		candidate, valid := value.Value.(int64)
		return ok && valid && (column.Name == clause.PrimaryKey || column.Name == "id") && candidate == id
	}
	return false
}

func TestP11PlunderSweepRound2MySQLActualParticipantLocks(t *testing.T) {
	name := os.Getenv("HYJS_P11_MYSQL_DATABASE")
	if name == "" {
		t.Skip("requires an isolated hyjs_test_p11_ MySQL database")
	}
	if !strings.HasPrefix(name, "hyjs_test_p11_") || len(name) == len("hyjs_test_p11_") || os.Getenv("HYJS_GAME_ACCOUNTS_DATABASE") != "" || config.AppConfigIns != nil && config.AppConfigIns.Game.AccountsDatabase != "" {
		t.Fatal("requires an isolated game and accounts fixture")
	}
	f, chosen, unused, now := p11PlunderFightSetup(t)
	if err := f.s.loadP7Table("../../analysis/tables", "BuffInfExtend"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP11PlunderSweep(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(f.ctx, f.id, func(player *pb.Player, _ *model.GameState) error {
		player.Level = proto.Int32(100)
		player.Energy = proto.Int32(10)
		player.EnergyLastRefreshTime = proto.Int64(time.Now().UnixMilli())
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	const fragment = "frag_trea_001_1"
	p11PlunderFiniteItems(t, f, chosen, fragment+",1")
	p11PlunderFiniteItems(t, f, unused, fragment+",1")
	p11PlunderPublish(t, now)
	if err := db.GetIns().Model(&db.P11PlunderFightPolicy{}).Where("id=1").Updates(map[string]any{"drop_ppm": 0, "level_difference": 100, "sweep_enabled": true}).Error; err != nil {
		t.Fatal(err)
	}
	listing, err := f.s.p11PlunderListAt(f.ctx, f.id, fragment, now)
	if err != nil {
		t.Fatal(err)
	}
	candidates := listing[0].(*pb.SCTreasureBattleList).InfoList
	if len(candidates) < 2 || candidates[0].GetIsRobot() != 0 || candidates[0].GetSimplePlayerInfo().GetPlayerId() != chosen {
		t.Fatal("isolated fixture must select its first real holder first")
	}
	unusedListed := false
	for _, candidate := range candidates {
		unusedListed = unusedListed || candidate.GetIsRobot() == 0 && candidate.GetSimplePlayerInfo().GetPlayerId() == unused
	}
	if !unusedListed {
		t.Fatal("isolated fixture must include the other real holder in the candidate list")
	}

	database := db.GetIns()
	request := &pb.CSSweepTreasureFight{FragId: proto.String(fragment), SweepNumber: proto.Int32(1), AutoAddEnergy: proto.Int32(0)}
	type sweepResult struct {
		replies []proto.Message
		err     error
	}
	ctx, cancel := context.WithTimeout(f.ctx, 90*time.Second)
	done := make(chan sweepResult, 1)
	var held *gorm.DB
	pending := false
	registered := false
	const callback = "test:p11-sweep-round2-selected-lock"
	defer func() {
		cancel()
		if held != nil {
			_ = held.Rollback().Error
		}
		if pending {
			select {
			case <-done:
			case <-time.After(8 * time.Second):
				t.Error("sweep goroutine did not stop after cancellation and lock release")
			}
		}
		if registered {
			_ = database.Callback().Query().Remove(callback)
		}
	}()
	lock := func(id int64) {
		t.Helper()
		held = database.WithContext(ctx).Begin()
		if held.Error != nil {
			t.Fatal(held.Error)
		}
		var profile db.Profile
		if err := held.Select("id").Clauses(clause.Locking{Strength: "UPDATE"}).First(&profile, id).Error; err != nil {
			t.Fatal(err)
		}
	}
	release := func() {
		t.Helper()
		if err := held.Commit().Error; err != nil {
			t.Fatal(err)
		}
		held = nil
	}
	start := func(callCtx context.Context, stamp int64) {
		t.Helper()
		pending = true
		go func() {
			replies, err := f.s.p11PlunderSweepAt(callCtx, f.id, request, stamp, now)
			done <- sweepResult{replies: replies, err: err}
		}()
	}
	collect := func() sweepResult {
		t.Helper()
		select {
		case result := <-done:
			pending = false
			if result.err != nil || len(result.replies) != 2 {
				t.Fatalf("actual sweep failed: %v", result.err)
			}
			return result
		case <-time.After(30 * time.Second):
			t.Fatal("actual sweep did not finish before timeout")
			return sweepResult{}
		}
	}
	verify := func(result sweepResult, stamp int64, before *pb.Player) *pb.Player {
		t.Helper()
		response, ok := result.replies[0].(*pb.SCSweepTreasureFight)
		if !ok || len(response.RewardList) != 1 || len(response.Cost) != 1 || response.Cost[0].GetId() != "energy" || response.Cost[0].GetNum() != 2 {
			t.Fatal("one actual child and its energy cost were not returned")
		}
		var receipt db.P11PlunderSweepReceipt
		if err := database.First(&receipt, "player_id=? AND stamp=?", f.id, stamp).Error; err != nil {
			t.Fatal(err)
		}
		var plan p11PlunderSweepPlan
		if json.Unmarshal(receipt.Plan, &plan) != nil || len(plan.Children) != 1 || !slices.Contains(plan.Peers, chosen) || !slices.Contains(plan.Peers, unused) {
			t.Fatal("receipt did not retain both actual candidates and one child")
		}
		var child db.P11PlunderFightSettlement
		if err := database.First(&child, "player_id=? AND stamp=?", f.id, plan.Children[0].Stamp).Error; err != nil {
			t.Fatal(err)
		}
		if child.OpponentID != chosen || child.EnergySpent != 2 || child.FragmentTransferred || child.DropPPM != 0 {
			t.Fatal("accepted child did not use the selected real holder")
		}
		after, game := p10LuckyProfile(t, f.id)
		owned, err := p11PlunderFragmentCount(game, fragment)
		if err != nil || owned != 0 || after.GetEnergy() != before.GetEnergy()-2 || after.GetCoin() != before.GetCoin()+int64(child.RewardCoin) || after.GetExp() != before.GetExp()+int64(child.RewardExp) {
			t.Fatalf("actual energy, gain or fragment accounting changed: %v", err)
		}
		for _, peer := range []int64{chosen, unused} {
			_, otherGame := p10LuckyProfile(t, peer)
			count, err := p11PlunderFragmentCount(otherGame, fragment)
			if err != nil || count != 1 {
				t.Fatalf("finite holder inventory changed: %v", err)
			}
		}
		return after
	}

	before, _ := p10LuckyProfile(t, f.id)
	lock(unused)
	start(ctx, 862001)
	unblocked := collect()
	// The actual sweep completed while the unselected candidate was still locked.
	release()
	beforeSelected := verify(unblocked, 862001, before)

	started := make(chan struct{}, 1)
	if err := database.Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Context.Value(p11SweepRound2LockKey{}) != true || tx.Statement.Table != "profiles" || len(tx.Statement.Selects) != 1 || tx.Statement.Selects[0] != "id" {
			return
		}
		locking, ok := tx.Statement.Clauses["FOR"].Expression.(clause.Locking)
		where, exists := tx.Statement.Clauses["WHERE"]
		if !ok || locking.Strength != "UPDATE" || !exists || !p11SweepRound2HasID(where.Expression, chosen) {
			return
		}
		select {
		case started <- struct{}{}:
		default:
		}
	}); err != nil {
		t.Fatal(err)
	}
	registered = true
	lock(chosen)
	tagged := context.WithValue(ctx, p11SweepRound2LockKey{}, true)
	start(tagged, 862002)
	select {
	case <-started:
	case result := <-done:
		pending = false
		t.Fatalf("selected lock was never attempted: %v", result.err)
	case <-time.After(30 * time.Second):
		t.Fatal("selected participant lock was not reached")
	}
	select {
	case result := <-done:
		pending = false
		t.Fatalf("selected participant lock did not wait: %v", result.err)
	case <-time.After(100 * time.Millisecond):
	}
	release()
	selected := collect()
	after := verify(selected, 862002, beforeSelected)
	replayed, err := f.s.p11PlunderSweepAt(ctx, f.id, request, 862002, now)
	if err != nil || len(replayed) != 2 || !proto.Equal(replayed[0], selected.replies[0]) {
		t.Fatalf("committed sweep receipt changed on replay: %v", err)
	}
	final, _ := p10LuckyProfile(t, f.id)
	if final.GetEnergy() != after.GetEnergy() || final.GetCoin() != after.GetCoin() || final.GetExp() != after.GetExp() {
		t.Fatal("receipt replay charged or paid again")
	}
	var count int64
	if err := database.Model(&db.P11PlunderSweepReceipt{}).Where("player_id=? AND stamp=?", f.id, 862002).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("expected one sweep receipt, got %d: %v", count, err)
	}
}
