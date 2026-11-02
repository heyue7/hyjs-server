package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/config"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// The in-memory actor retains current state only in its own attempt. Recovery
// has a separate consumption baseline from the persisted one on every load.
func TestP11PlunderSweepReuseRound2ActorScopeAndBaselines(t *testing.T) {
	tx := &gorm.DB{Statement: &gorm.Statement{Context: context.Background()}}
	otherTx := &gorm.DB{Statement: &gorm.Statement{Context: context.Background()}}
	now := time.Now().UnixMilli()
	player := &pb.Player{PlayerId: proto.Int64(71), Energy: proto.Int32(20), Stamina: proto.Int32(20), EnergyLastRefreshTime: proto.Int64(now - constUtil.EnergyRecoveryIntervalMS - 100), StaminaLastRefreshTime: proto.Int64(now)}
	game := &model.GameState{Items: []*pb.SimpleItem{{Id: proto.String("fragment"), Num: proto.Int32(0)}}}
	marshal := func(value any) string {
		t.Helper()
		body, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return string(body)
	}
	storedPlayer, storedGame := marshal(player), marshal(game)
	game.ProfileStorageBaseline = &model.ProfileStorageBaseline{Player: storedPlayer, Game: storedGame}
	actor := &p11PlunderSweepActor{row: db.Profile{Id: 71, Player: &storedPlayer, Game: &storedGame}, player: player, game: game}
	first := newP11PlunderSweepActors(tx, []int64{71})
	first.loaded[71] = actor
	second := newP11PlunderSweepActors(otherTx, []int64{71})
	if second.loaded[71] != nil {
		t.Fatal("a new attempt inherited an actor")
	}
	if err := first.require(otherTx, 71); err == nil {
		t.Fatal("actor scope accepted a different transaction")
	}
	var missing *errP11PlunderSweepParticipant
	if err := first.require(tx, 72); !errors.As(err, &missing) || missing.PlayerID != 72 {
		t.Fatal("unlocked participant must trigger a retry", err)
	}
	for step := int32(0); step < 5; step++ {
		current, err := first.load(71)
		if err != nil || current != actor {
			t.Fatal("same-transaction actor was not reused", err)
		}
		var consumption pb.Player
		if err := json.Unmarshal([]byte(*actor.row.Player), &consumption); err != nil {
			t.Fatal(err)
		}
		if consumption.GetEnergy() != 21-2*step || actor.game.ProfileStorageBaseline.Player != storedPlayer || actor.game.ProfileStorageBaseline.Game != storedGame {
			t.Fatal("recovery or persisted baseline changed", step, consumption.GetEnergy())
		}
		actor.player.Energy = proto.Int32(actor.player.GetEnergy() - 2)
		actor.game.Items[0].Num = proto.Int32(step + 1)
		storedPlayer, storedGame = marshal(actor.player), marshal(actor.game)
		actor.game.ProfileStorageBaseline = &model.ProfileStorageBaseline{Player: storedPlayer, Game: storedGame}
		if err := actor.adoptSavedBaseline(); err != nil {
			t.Fatal(err)
		}
		if *actor.row.Player != storedPlayer || *actor.row.Game != storedGame {
			t.Fatal("save did not advance both row baselines")
		}
	}
	if actor.player.GetEnergy() != 11 || actor.game.Items[0].GetNum() != 5 {
		t.Fatal("successive mutations did not accumulate")
	}
}

func TestP11PlunderSweepReuseRound2ProjectionIsolation(t *testing.T) {
	side := &pb.PlayerSide{CurHp: proto.Float64(0.5)}
	game := &model.GameState{
		Formation:             []*pb.FormationItem{{FormatPos: proto.Int32(1), BattlePos: proto.Int32(3)}},
		Field:                 &model.FieldState{Side: side},
		GuildPrayerProjection: &model.P5GuildPrayerProjection{PlayerID: 71},
	}
	projection := p11PlunderSweepProjectionGame(game)
	projection.Formation[0].BattlePos = proto.Int32(7)
	projection.Field.Side.CurHp = proto.Float64(0.1)
	projection.GuildPrayerProjection = nil
	if game.Formation[0].GetBattlePos() != 3 || game.Field.Side.GetCurHp() != 0.5 || game.GuildPrayerProjection == nil {
		t.Fatal("projection modified the authoritative actor")
	}
}

type p11SweepReuseRound2Key struct{}

// Run in the isolated MySQL suite: five real P6 children must load each locked
// profile once in the committing transaction, while their resources accumulate.
func TestP11PlunderSweepReuseRound2MySQLFiveRealChildren(t *testing.T) {
	name := os.Getenv("HYJS_P11_MYSQL_DATABASE")
	if name == "" {
		t.Skip("requires an isolated hyjs_test_p11_ MySQL database")
	}
	if !strings.HasPrefix(name, "hyjs_test_p11_") || len(name) == len("hyjs_test_p11_") || os.Getenv("HYJS_GAME_ACCOUNTS_DATABASE") != "" || config.AppConfigIns != nil && config.AppConfigIns.Game.AccountsDatabase != "" {
		t.Fatal("requires isolated game and accounts fixtures")
	}
	f, peer, _, now := p11PlunderFightSetup(t)
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
	p11PlunderFiniteItems(t, f, peer, fragment+",1")
	p11PlunderPublish(t, now)
	if err := db.GetIns().Model(&db.P11PlunderFightPolicy{}).Where("id=1").Updates(map[string]any{"drop_ppm": 0, "level_difference": 100, "sweep_enabled": true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().WithContext(f.ctx).Transaction(func(tx *gorm.DB) error {
		var locked db.Profile
		if err := tx.Select("id").Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, f.id).Error; err != nil {
			return err
		}
		actors := newP11PlunderSweepActors(tx, []int64{f.id})
		actor, err := actors.load(f.id)
		if err != nil {
			return err
		}
		beforePlayer, err := json.Marshal(actor.player)
		if err != nil {
			return err
		}
		beforeGame, err := json.Marshal(actor.game)
		if err != nil {
			return err
		}
		beforePrayer := actor.game.GuildPrayerProjection
		wantInfo, wantBattle, err := f.s.p11ArenaProjection(&actor.row, tx)
		if err != nil {
			return err
		}
		info, battle, err := actors.projection(f.s, f.id)
		if err != nil {
			return err
		}
		afterPlayer, err := json.Marshal(actor.player)
		if err != nil {
			return err
		}
		afterGame, err := json.Marshal(actor.game)
		if err != nil {
			return err
		}
		if !proto.Equal(wantInfo, info) || !proto.Equal(wantBattle, battle) || string(beforePlayer) != string(afterPlayer) || string(beforeGame) != string(afterGame) || actor.game.GuildPrayerProjection != beforePrayer {
			return errors.New("reused projection differs from the decoded projection or changed the actor")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	type counts struct{ owner, peer, matches int }
	var mu sync.Mutex
	byTransaction := map[any]*counts{}
	entry := func(tx *gorm.DB) *counts {
		key := tx.Statement.ConnPool
		if byTransaction[key] == nil {
			byTransaction[key] = &counts{}
		}
		return byTransaction[key]
	}
	database := db.GetIns()
	const queryCallback = "test:p11-sweep-reuse-round2-query"
	const createCallback = "test:p11-sweep-reuse-round2-create"
	if err := database.Callback().Query().Before("gorm:query").Register(queryCallback, func(tx *gorm.DB) {
		if tx.Statement.Context.Value(p11SweepReuseRound2Key{}) != true || tx.Statement.Schema == nil || tx.Statement.Schema.Table != "profiles" || len(tx.Statement.Selects) != 0 {
			return
		}
		if _, ok := tx.Statement.Dest.(*db.Profile); !ok {
			return
		}
		where, ok := tx.Statement.Clauses["WHERE"]
		if !ok {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		counts := entry(tx)
		if p11SweepRound2HasID(where.Expression, f.id) {
			counts.owner++
		}
		if p11SweepRound2HasID(where.Expression, peer) {
			counts.peer++
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Callback().Query().Remove(queryCallback) })
	if err := database.Callback().Create().Before("gorm:create").Register(createCallback, func(tx *gorm.DB) {
		if tx.Statement.Context.Value(p11SweepReuseRound2Key{}) != true || tx.Statement.Schema == nil || tx.Statement.Schema.Table != "p11_plunder_fight_matches" {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		entry(tx).matches++
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Callback().Create().Remove(createCallback) })
	ctx := context.WithValue(f.ctx, p11SweepReuseRound2Key{}, true)
	req := &pb.CSSweepTreasureFight{FragId: proto.String(fragment), SweepNumber: proto.Int32(5), AutoAddEnergy: proto.Int32(0)}
	out, err := f.s.p11PlunderSweepAt(ctx, f.id, req, 862100, now)
	if err != nil || len(out) != 2 || len(out[0].(*pb.SCSweepTreasureFight).RewardList) != 5 {
		t.Fatal("five real child fights", err)
	}
	mu.Lock()
	var committing *counts
	var discoveredInPriorAttempt bool
	for _, current := range byTransaction {
		if current.matches == 0 && current.owner == 1 && current.peer >= 1 {
			discoveredInPriorAttempt = true
		}
		if current.matches == 5 {
			if committing != nil {
				mu.Unlock()
				t.Fatal("multiple transactions produced five matches")
			}
			committing = current
		}
	}
	mu.Unlock()
	if committing == nil || committing.owner != 1 || committing.peer != 1 {
		t.Fatalf("committing transaction profile loads: %+v", committing)
	}
	if !discoveredInPriorAttempt {
		t.Fatal("missing rolled-back participant discovery and independent reload")
	}
	owner, ownerGame := p10LuckyProfile(t, f.id)
	_, peerGame := p10LuckyProfile(t, peer)
	owned, err := p11PlunderFragmentCount(ownerGame, fragment)
	if err != nil {
		t.Fatal(err)
	}
	held, err := p11PlunderFragmentCount(peerGame, fragment)
	if err != nil || owner.GetEnergy() != 0 || owned != 0 || held != 1 {
		t.Fatal("five costs or fragment conservation failed", err, owner.GetEnergy(), owned, held)
	}
}
