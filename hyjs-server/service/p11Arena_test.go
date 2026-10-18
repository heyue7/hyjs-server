package service

import (
	"context"
	"encoding/json"
	"fmt"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func p11ArenaSetup(t *testing.T) (*p1Fixture, int64, int64) {
	t.Helper()
	f := &p1Fixture{ctx: context.Background(), path: filepath.Join(t.TempDir(), "p11.db")}
	if name := os.Getenv("HYJS_P11_MYSQL_DATABASE"); name != "" {
		if !strings.HasPrefix(name, "hyjs_test_p11_") {
			t.Fatal("P11 isolation guard")
		}
		f.mysql = db.MySQLConfig{Host: os.Getenv("HYJS_P11_MYSQL_HOST"), User: os.Getenv("HYJS_P11_MYSQL_USER"), Password: os.Getenv("HYJS_P11_MYSQL_PASSWORD"), Database: name}
	}
	f.open(t)
	f.s = &GameService{}
	if err := f.s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.LoadP11Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP1(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP6(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP10Social(f.ctx); err != nil {
		t.Fatal(err)
	}
	for _, table := range []any{&db.P10SocialRelation{}, &db.P10SocialBlack{}} {
		if err := db.GetIns().Where("1=1").Delete(table).Error; err != nil {
			t.Fatal(err)
		}
	}
	var ids []int64
	for i := 0; i < 3; i++ {
		login, err := LoginAccount(f.ctx, fmt.Sprintf("p11-%s-%d-%d", t.Name(), i, time.Now().UnixNano()), "local-isolated-password")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = CreatePlayer(f.ctx, login.Id, fmt.Sprintf("竞技真实%d", i), "hero_047"); err != nil {
			t.Fatal(err)
		}
		if err = updateProfile(f.ctx, login.Id, func(p *pb.Player, g *model.GameState) error { p.Level = proto.Int32(20); return nil }); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, login.Id)
	}
	f.id = ids[0]
	b, c := ids[1], ids[2]
	t.Cleanup(func() { d, _ := db.GetIns().DB(); d.Close() })
	if err := f.s.InitializeP11Arena(f.ctx); err != nil {
		t.Fatal(err)
	}
	for _, table := range []any{&db.P11ArenaPolicy{}, &db.P11ArenaMember{}, &db.P11ArenaMatch{}, &db.P11ArenaReceipt{}, &db.P11FriendDuel{}, &db.P11FriendFightSettlement{}, &db.P11ArenaFightSettlement{}, &db.P11ArenaFightPolicy{}, &db.P11LimitPolicy{}, &db.P11LimitPeriod{}, &db.P11LimitMember{}, &db.P11LimitDaily{}, &db.P11LimitRewardClaim{}, &db.P11PlunderPolicy{}, &db.P11PlunderState{}, &db.P11GrabPolicy{}, &db.P11GrabSlot{}, &db.P11GrabSession{}, &db.P11GrabDaily{}, &db.P11GrabRewardClaim{}} {
		if err := db.GetIns().Where("1=1").Delete(table).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := f.s.InitializeP11Arena(f.ctx); err != nil {
		t.Fatal(err)
	}
	return f, b, c
}
func p11ArenaQuery(t *testing.T, f *p1Fixture, id int64, request proto.Message, stamp int64, now time.Time) proto.Message {
	t.Helper()
	result, err := f.s.p11ArenaQuery(f.ctx, id, request, stamp, now)
	if err != nil {
		t.Fatal(err)
	}
	return result[0]
}
func TestP11ArenaRealRegistryUniqueRanksMatchSnapshotsAndNoResources(t *testing.T) {
	f, b, c := p11ArenaSetup(t)
	now := time.Now()
	beforeP, beforeG := p10LuckyProfile(t, f.id)
	before, _ := json.Marshal(beforeG)
	query := p11ArenaQuery(t, f, f.id, &pb.CSPvpFindMatch{}, 100, now).(*pb.SCPvpFindMatch)
	if query.GetMyrank().GetInfo().GetPlayerId() != f.id || query.GetMyrank().GetPvpRank() < 1 {
		t.Fatal("real self rank", query)
	}
	found := map[int64]bool{}
	ranks := map[int64]bool{}
	for _, candidate := range query.GetRankList() {
		if candidate.GetInfo().GetPlayerId() == f.id || candidate.GetInfo().GetFightPower() <= 0 || ranks[candidate.GetPvpRank()] {
			t.Fatal("fake/duplicate rank candidate", candidate)
		}
		ranks[candidate.GetPvpRank()] = true
		found[candidate.GetInfo().GetPlayerId()] = true
	}
	if !found[b] || !found[c] {
		t.Fatal("missing actual fixture peers", query)
	}
	var match db.P11ArenaMatch
	if err := db.GetIns().Where("viewer_id=? AND opponent_id=?", f.id, b).First(&match).Error; err != nil {
		t.Fatal(err)
	}
	frozen, err := f.s.RecoverP11ArenaMatch(f.ctx, f.id, match.ID, now)
	if err != nil || frozen.Attacker.GetPlayerId() != f.id || frozen.Defender.GetPlayerId() != b || frozen.BattleType != 2 || frozen.MaxRound != 40 {
		t.Fatal("original frozen pair", err)
	}
	if _, err := f.s.RecoverP11ArenaMatch(f.ctx, c, match.ID, now); err == nil {
		t.Fatal("foreign matched pair exposed")
	}
	if err := updateProfile(f.ctx, b, func(p *pb.Player, g *model.GameState) error { g.Warriors[0].Level = proto.Int32(2); return nil }); err != nil {
		t.Fatal(err)
	}
	recovered, err := f.s.RecoverP11ArenaMatch(f.ctx, f.id, match.ID, now)
	if err != nil || !proto.Equal(frozen.Defender, recovered.Defender) {
		t.Fatal("opponent changed old pair", err)
	}
	repeat := p11ArenaQuery(t, f, f.id, &pb.CSPvpFindMatch{}, 100, now.Add(time.Second))
	if !proto.Equal(query, repeat) {
		t.Fatal("same request returned a new match")
	}
	p11ArenaQuery(t, f, f.id, &pb.CSPvpFindMatch{}, 101, now.Add(time.Second))
	afterP, afterG := p10LuckyProfile(t, f.id)
	after, _ := json.Marshal(afterG)
	if !proto.Equal(beforeP, afterP) || string(before) != string(after) {
		t.Fatal("readonly arena query changed original resources")
	}
	var won int64
	db.GetIns().Model(&db.P11ArenaMember{}).Where("best_won_rank>0").Count(&won)
	if won != 0 {
		t.Fatal("entry or query pretended a battle win")
	}
}
func TestP11ArenaConcurrentRealRegistrationReceiptAndRestart(t *testing.T) {
	f, b, _ := p11ArenaSetup(t)
	now := time.Now()
	var wg sync.WaitGroup
	errors := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := f.s.p11ArenaQuery(f.ctx, f.id, &pb.CSPvpFindMatch{}, 100, now)
			errors <- err
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	var receipts int64
	db.GetIns().Model(&db.P11ArenaReceipt{}).Where("player_id=?", f.id).Count(&receipts)
	if receipts != 1 {
		t.Fatal("query receipt duplicated", receipts)
	}
	var match db.P11ArenaMatch
	db.GetIns().Where("viewer_id=? AND opponent_id=?", f.id, b).First(&match)
	original, err := f.s.RecoverP11ArenaMatch(f.ctx, f.id, match.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	restarted := &GameService{}
	if err := restarted.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := restarted.LoadP11Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	reloaded, err := restarted.RecoverP11ArenaMatch(context.WithoutCancel(f.ctx), f.id, match.ID, now)
	if err != nil || !proto.Equal(original.Attacker, reloaded.Attacker) || !proto.Equal(original.Defender, reloaded.Defender) || original.Seed != reloaded.Seed {
		t.Fatal("persisted pair changed on restart", err)
	}
	if _, err := restarted.RecoverP11ArenaMatch(f.ctx, f.id, match.ID, now.Add(6*time.Minute)); err == nil {
		t.Fatal("expired old opponent accepted")
	}
	var duplicate struct{ Count int64 }
	if err := db.GetIns().Raw("SELECT COUNT(*) AS count FROM (SELECT position,COUNT(*) n FROM p11_arena_members GROUP BY position HAVING n>1) d").Scan(&duplicate).Error; err != nil || duplicate.Count != 0 {
		t.Fatal("duplicate real positions", err)
	}
}
func TestP11ArenaBlackCorruptOpponentLowLevelAndStrictNativeGate(t *testing.T) {
	f, b, c := p11ArenaSetup(t)
	now := time.Now()
	if err := db.GetIns().Create(&db.P10SocialBlack{OwnerID: f.id, TargetID: b, CreatedAt: now.UnixMilli()}).Error; err != nil {
		t.Fatal(err)
	}
	reply := p11ArenaQuery(t, f, f.id, &pb.CSPvpFindMatch{}, 100, now).(*pb.SCPvpFindMatch)
	for _, candidate := range reply.GetRankList() {
		if candidate.GetInfo().GetPlayerId() == b {
			t.Fatal("black peer offered as target")
		}
	}
	var original db.Profile
	if err := db.GetIns().First(&original, c).Error; err != nil {
		t.Fatal(err)
	}
	// 损坏候选专项结束恢复原隔离行，保留后续真实冷启动的数据前提。
	t.Cleanup(func() {
		if err := db.GetIns().Model(&db.Profile{}).Where("id=?", c).Update("game", original.Game).Error; err != nil {
			t.Error(err)
		}
	})
	if err := db.GetIns().Model(&db.Profile{}).Where("id=?", c).Update("game", []byte("invalid-game")).Error; err != nil {
		t.Fatal(err)
	}
	reply = p11ArenaQuery(t, f, f.id, &pb.CSPvpFindMatch{}, 101, now).(*pb.SCPvpFindMatch)
	for _, candidate := range reply.GetRankList() {
		if candidate.GetInfo().GetPlayerId() == c {
			t.Fatal("corrupt profile invented as opponent")
		}
	}
	beforeP, beforeG := p10LuckyProfile(t, f.id)
	before, _ := json.Marshal(beforeG)
	if err := db.GetIns().Model(&db.P11ArenaPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "duel_enabled": true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().AutoMigrate(&db.P6CombatRule{}); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P6CombatRule{}).Where("id=1").Updates(map[string]any{"authoritative_enabled": true, "native_capability_hash": P6CapabilityHash(), "client_hash": constUtil.P6OriginalClientHash}).Error; err != nil {
		t.Fatal(err)
	}
	for _, request := range []proto.Message{&pb.CSPvpFightResult{PlayerId: proto.Int64(b), ServerId: proto.Int32(1)}, &pb.CSPvpFight{PlayerId: proto.Int64(b)}, &pb.CSPvpFightEnd{FightId: proto.String("invented"), WinPlayerId: proto.Int64(f.id)}, &pb.CSPvpFightResultGuide{PlayerId: proto.Int64(b)}} {
		if _, err := f.s.p11ArenaChallenge(f.ctx, f.id, request); err == nil {
			t.Fatal("SQL flag or client winner bypassed original parity")
		}
	}
	afterP, afterG := p10LuckyProfile(t, f.id)
	after, _ := json.Marshal(afterG)
	if !proto.Equal(beforeP, afterP) || string(before) != string(after) {
		t.Fatal("closed battle charged energy or reward")
	}
	var attempts int64
	db.GetIns().Model(&db.P11ArenaReceipt{}).Where("operation<>?", "match-query").Count(&attempts)
	if attempts != 0 {
		t.Fatal("closed challenge created reward receipt")
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error { p.Level = proto.Int32(9); return nil }); err != nil {
		t.Fatal(err)
	}
	low := p11ArenaQuery(t, f, f.id, &pb.CSPvpFindMatch{}, 102, now).(*pb.SCPvpFindMatch)
	if low.GetMyrank().GetPvpRank() != 0 || len(low.GetRankList()) != 0 {
		t.Fatal("low level joined competitive membership")
	}
}
func TestP11ArenaMatchedDiagnosticRealOriginalPartyNoSettlement(t *testing.T) {
	f, b, _ := p11ArenaSetup(t)
	now := time.Now()
	p11ArenaQuery(t, f, f.id, &pb.CSPvpFindMatch{}, 100, now)
	var matched db.P11ArenaMatch
	if err := db.GetIns().Where("viewer_id=? AND opponent_id=?", f.id, b).First(&matched).Error; err != nil {
		t.Fatal(err)
	}
	beforeP, beforeG := p10LuckyProfile(t, f.id)
	before, _ := json.Marshal(beforeG)
	outcome, err := f.s.p11ProbeArenaMatch(f.ctx, f.id, matched.ID, now)
	if err != nil {
		if !strings.Contains(err.Error(), constUtil.CombatUnsupported) {
			t.Fatal(err)
		}
	} else {
		if outcome.Authoritative || outcome.Operates == nil {
			t.Fatal("diagnostic became authoritative")
		}
	}
	afterP, afterG := p10LuckyProfile(t, f.id)
	after, _ := json.Marshal(afterG)
	if !proto.Equal(beforeP, afterP) || string(before) != string(after) {
		t.Fatal("diagnostic changed resources")
	}
	var wins int64
	db.GetIns().Model(&db.P11ArenaMember{}).Where("best_won_rank>0").Count(&wins)
	if wins != 0 {
		t.Fatal("diagnostic mutated battle ranks")
	}
}

func TestP11FriendDuelRealRelationConcurrentFrozenType11AndPermissionRecovery(t *testing.T) {
	f, b, c := p11ArenaSetup(t)
	now := time.Now()
	beforeP, beforeG := p10LuckyProfile(t, f.id)
	before, _ := json.Marshal(beforeG)
	if _, err := f.s.prepareP11FriendDuel(f.ctx, f.id, b, 100, now); err == nil {
		t.Fatal("stranger duel frozen")
	}
	low, high := p10Pair(f.id, b)
	if err := db.GetIns().Create(&db.P10SocialRelation{LowID: low, HighID: high, CreatedAt: now.UnixMilli()}).Error; err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan *db.P11FriendDuel, 8)
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			row, err := f.s.prepareP11FriendDuel(f.ctx, f.id, b, 100, now)
			results <- row
			errs <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var original *db.P11FriendDuel
	for row := range results {
		if original == nil {
			original = row
		}
		if row.ID != original.ID {
			t.Fatal("duel receipt duplicated")
		}
	}
	frozen, err := f.s.recoverP11FriendDuel(f.ctx, f.id, original.ID, now)
	if err != nil || frozen.BattleType != 11 || frozen.MaxRound != 40 || frozen.Attacker.GetPlayerId() != f.id || frozen.Defender.GetPlayerId() != b {
		t.Fatal("original friend type11 freeze", err)
	}
	if _, err := f.s.recoverP11FriendDuel(f.ctx, c, original.ID, now); err == nil {
		t.Fatal("foreign duel snapshot")
	}
	if err := updateProfile(f.ctx, b, func(p *pb.Player, g *model.GameState) error { g.Warriors[0].Level = proto.Int32(2); return nil }); err != nil {
		t.Fatal(err)
	}
	restart := &GameService{}
	if err := restart.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := restart.LoadP11Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	restored, err := restart.recoverP11FriendDuel(f.ctx, f.id, original.ID, now)
	if err != nil || !proto.Equal(frozen.Defender, restored.Defender) || frozen.Seed != restored.Seed {
		t.Fatal("duel restart/changed opponent", err)
	}
	if _, err := restart.recoverP11FriendDuel(f.ctx, f.id, original.ID, now.Add(6*time.Minute)); err == nil {
		t.Fatal("expired friend duel")
	}
	if err := db.GetIns().Create(&db.P10SocialBlack{OwnerID: b, TargetID: f.id, CreatedAt: now.UnixMilli()}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := restart.recoverP11FriendDuel(f.ctx, f.id, original.ID, now); err == nil {
		t.Fatal("blocked friend old freeze reused")
	}
	if _, err := f.s.prepareP11FriendDuel(f.ctx, f.id, b, 100, now); err == nil {
		t.Fatal("blocked friend old receipt reused")
	}
	afterP, afterG := p10LuckyProfile(t, f.id)
	after, _ := json.Marshal(afterG)
	if !proto.Equal(beforeP, afterP) || string(before) != string(after) {
		t.Fatal("friend preparation spent resources")
	}
	var count int64
	db.GetIns().Model(&db.P11FriendDuel{}).Where("viewer_id=?", f.id).Count(&count)
	if count != 1 {
		t.Fatal("duplicate actual friend duel", count)
	}
}

// TestP11NativeDuelSnapshot由专库原HTTP关系测试传真实双方ID，不创建替代玩家或虚构关系。
func TestP11NativeDuelSnapshot(t *testing.T) {
	rawID := os.Getenv("HYJS_P11_NATIVE_VIEWER")
	if rawID == "" {
		t.Skip("requires owned original HTTP relationship fixture")
	}
	name := os.Getenv("HYJS_P11_MYSQL_DATABASE")
	if !strings.HasPrefix(name, "hyjs_test_p11_") {
		t.Fatal("P11 native isolation guard")
	}
	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	target, err := strconv.ParseInt(os.Getenv("HYJS_P11_NATIVE_TARGET"), 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	f := &p1Fixture{ctx: context.Background(), mysql: db.MySQLConfig{Host: os.Getenv("HYJS_P11_MYSQL_HOST"), User: os.Getenv("HYJS_P11_MYSQL_USER"), Password: os.Getenv("HYJS_P11_MYSQL_PASSWORD"), Database: name}}
	f.open(t)
	t.Cleanup(func() { sqlDB, _ := db.GetIns().DB(); sqlDB.Close() })
	f.s = &GameService{}
	if err = f.s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.LoadP11Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	beforeP, beforeG := p10LuckyProfile(t, id)
	before, _ := json.Marshal(beforeG)
	now := time.Now()
	row, err := f.s.prepareP11FriendDuel(f.ctx, id, target, 9911, now)
	if os.Getenv("HYJS_P11_NATIVE_DUEL_DENIED") == "1" {
		if err == nil {
			t.Fatal("deleted/blocked original friendship permitted old receipt")
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	frozen, err := f.s.recoverP11FriendDuel(f.ctx, id, row.ID, now)
	if err != nil || frozen.BattleType != 11 || frozen.Attacker.GetPlayerId() != id || frozen.Defender.GetPlayerId() != target {
		t.Fatal("native relation did not freeze original pair", err)
	}
	repeated, err := f.s.prepareP11FriendDuel(f.ctx, id, target, 9911, now)
	if err != nil || repeated.ID != row.ID {
		t.Fatal("native freeze receipt changed", err)
	}
	afterP, afterG := p10LuckyProfile(t, id)
	after, _ := json.Marshal(afterG)
	if !proto.Equal(beforeP, afterP) || string(before) != string(after) {
		t.Fatal("native duel preparation resources changed")
	}
}
