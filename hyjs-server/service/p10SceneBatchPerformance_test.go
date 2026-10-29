package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

type p10BatchSQLTrace struct {
	logger.Interface
	sql []string
}

func (trace *p10BatchSQLTrace) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	query, rows := fc()
	trace.sql = append(trace.sql, query)
	trace.Interface.Trace(ctx, begin, func() (string, int64) { return query, rows }, err)
}

func p10BatchFrame(t *testing.T, id int64) []byte {
	t.Helper()
	raw, err := proto.Marshal(&pb.MovePlayer{Info: &pb.MovePlayerInfo{PlayerId: proto.Int64(id)}})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestP10SceneBatchDirectoryVisibilityAndQueryCount(t *testing.T) {
	t.Setenv("HYJS_P10_MYSQL_DATABASE", "")
	f, _, _ := p10SceneSetup(t)
	var template db.Profile
	if err := db.GetIns().First(&template, f.id).Error; err != nil {
		t.Fatal(err)
	}
	var ids []int64
	for i := int64(1); i <= 19; i++ {
		row := template
		row.Id, row.Uid = 10000+i, fmt.Sprintf("p10-batch-%d", i)
		if err := db.GetIns().Create(&row).Error; err != nil {
			t.Fatal(err)
		}
		presence := db.P10ScenePresence{PlayerID: row.Id, Scene: "town_main", Line: 1, SessionHash: p6Digest(row.Session), Frame: p10BatchFrame(t, row.Id)}
		if err := db.GetIns().Create(&presence).Error; err != nil {
			t.Fatal(err)
		}
		key := p10ClientKey{f.s.p10ClientOwner(), row.Id}
		sink := &p10ClientSink{session: append([]byte(nil), row.Session...)}
		p10ClientSinks.Store(key, sink)
		t.Cleanup(func() { p10ClientSinks.CompareAndDelete(key, sink) })
		ids = append(ids, row.Id)
	}
	trace := &p10BatchSQLTrace{Interface: logger.Default.LogMode(logger.Silent)}
	tx := db.GetIns().WithContext(f.ctx).Session(&gorm.Session{Logger: trace})
	peers, err := f.s.p10ScenePeers(tx, "town_main", 1, 0)
	if err != nil || len(peers) != len(ids) {
		t.Fatalf("expected 19 visible peers: %d, %v", len(peers), err)
	}
	if len(trace.sql) != 2 || !strings.Contains(strings.ToLower(trace.sql[1]), "profiles") || !strings.Contains(strings.ToLower(trace.sql[1]), " in ") {
		t.Fatalf("19 peers should use one presence and one indexed profile read: %v", trace.sql)
	}
	for _, query := range trace.sql {
		if strings.Contains(strings.ToLower(query), " join ") {
			t.Fatalf("directory query unexpectedly joined another table: %s", query)
		}
	}
	t.Logf("19 online peers: %d directory queries, 0 JOIN", len(trace.sql))
	for i, peer := range peers {
		if peer.GetInfo().GetPlayerId() != ids[i] {
			t.Fatalf("presence order or frame changed: %v", peer)
		}
	}
	// Fresh calls must reject a rotated session, a changed real scope and an invalid frame.
	rotated := append([]byte(nil), template.Session...)
	rotated[0] ^= 0x80
	if err := db.GetIns().Model(&db.Profile{}).Where("id=?", ids[0]).Update("session", rotated).Error; err != nil {
		t.Fatal(err)
	}
	_, game, err := decodeProfile(&template)
	if err != nil {
		t.Fatal(err)
	}
	game.Position.Scene = proto.String("field_001")
	game.PositionLoading = false
	gameJSON, err := json.Marshal(game)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.Profile{}).Where("id=?", ids[1]).Update("game", string(gameJSON)).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P10ScenePresence{}).Where("player_id=?", ids[2]).Update("frame", p10BatchFrame(t, ids[0])).Error; err != nil {
		t.Fatal(err)
	}
	peers, err = f.s.p10ScenePeers(db.GetIns().WithContext(f.ctx), "town_main", 1, 0)
	if err != nil || len(peers) != 16 {
		t.Fatalf("stale session/scope/frame remained visible: %d, %v", len(peers), err)
	}
}

func TestP10SceneBatchOfflineAccountRevisionAndExpiry(t *testing.T) {
	t.Setenv("HYJS_P10_MYSQL_DATABASE", "")
	f, b, c := p10SceneSetup(t)
	if err := db.GetIns().AutoMigrate(&db.GameAccount{}); err != nil {
		t.Fatal(err)
	}
	ids := []int64{b, c}
	for i, id := range ids {
		var profile db.Profile
		if err := db.GetIns().First(&profile, id).Error; err != nil {
			t.Fatal(err)
		}
		_, game, err := decodeProfile(&profile)
		if err != nil {
			t.Fatal(err)
		}
		game.Position.Scene = proto.String(constUtil.GGOFieldScene)
		game.PositionLoading = false
		game.GGOField = &model.GGOFieldState{}
		raw, err := json.Marshal(game)
		if err != nil {
			t.Fatal(err)
		}
		account := db.GameAccount{ID: int64(700 + i), Username: fmt.Sprintf("batch-offline-%d", i), PasswordSalt: []byte{1}, PasswordHash: []byte{1}, Enabled: true, AuthRevision: 3, Revision: 1}
		if err := db.GetIns().Create(&account).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.GetIns().Model(&db.Profile{}).Where("id=?", id).Updates(map[string]any{"game": string(raw), "account_id": account.ID, "account_auth_revision": account.AuthRevision}).Error; err != nil {
			t.Fatal(err)
		}
		row := db.P10ScenePresence{PlayerID: id, Scene: constUtil.GGOFieldScene, Line: 1, Frame: p10BatchFrame(t, id), OfflineUntil: time.Now().Add(time.Minute).UnixMilli(), OfflinePKUsed: i == 0}
		if err := db.GetIns().Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	trace := &p10BatchSQLTrace{Interface: logger.Default.LogMode(logger.Silent)}
	tx := db.GetIns().WithContext(f.ctx).Session(&gorm.Session{Logger: trace})
	peers, err := f.s.p10ScenePeers(tx, constUtil.GGOFieldScene, 1, 0)
	if err != nil || len(peers) != 2 || len(trace.sql) != 4 {
		t.Fatalf("offline 60-second directory needs two presence plus batched profile/account reads: %d peers, SQL=%v, error=%v", len(peers), trace.sql, err)
	}
	for _, query := range trace.sql {
		if strings.Contains(strings.ToLower(query), " join ") {
			t.Fatalf("offline directory query unexpectedly joined another table: %s", query)
		}
	}
	t.Logf("2 offline peers: %d directory queries, 0 JOIN", len(trace.sql))
	if err := db.GetIns().Model(&db.GameAccount{}).Where("id=?", 700).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.Profile{}).Where("id=?", c).Update("account_auth_revision", 4).Error; err != nil {
		t.Fatal(err)
	}
	peers, err = f.s.p10ScenePeers(db.GetIns(), constUtil.GGOFieldScene, 1, 0)
	if err != nil || len(peers) != 0 {
		t.Fatalf("revoked offline accounts remained visible: %d, %v", len(peers), err)
	}
	if err := db.GetIns().Model(&db.GameAccount{}).Where("id=?", 700).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.Profile{}).Where("id=?", c).Update("account_auth_revision", 3).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P10ScenePresence{}).Where("player_id=?", b).Update("offline_until", time.Now().Add(-time.Second).UnixMilli()).Error; err != nil {
		t.Fatal(err)
	}
	peers, err = f.s.p10ScenePeers(db.GetIns(), constUtil.GGOFieldScene, 1, 0)
	if err != nil || len(peers) != 1 || peers[0].GetInfo().GetPlayerId() != c {
		t.Fatalf("offline expiry: %v, %v", peers, err)
	}
}

func TestP10SceneBatchUnchangedPresenceAndMovement(t *testing.T) {
	t.Setenv("HYJS_P10_MYSQL_DATABASE", "")
	f, b, _ := p10SceneSetup(t)
	self, peer := &p10SceneRecorder{}, &p10SceneRecorder{}
	p10SceneConnect(t, f, f.id, self)
	p10SceneConnect(t, f, b, peer)
	self.drain()
	peer.drain()
	oldAt := time.Now().Add(-8 * 24 * time.Hour).UnixMilli()
	if err := db.GetIns().Model(&db.P10ScenePresence{}).Where("player_id=?", f.id).Update("updated_at", oldAt).Error; err != nil {
		t.Fatal(err)
	}
	var writes, directoryReads atomic.Int64
	var lastFields map[string]any
	queryCallback := db.GetIns().Callback().Query()
	if err := queryCallback.After("gorm:query").Register("p10_perf_directory_read", func(tx *gorm.DB) {
		query := strings.ToLower(tx.Statement.SQL.String())
		if tx.Statement.Table == "p10_scene_presence" && strings.Contains(query, "scene=? and line=? and player_id<>?") {
			directoryReads.Add(1)
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer queryCallback.Remove("p10_perf_directory_read")
	callback := db.GetIns().Callback().Update()
	if err := callback.Before("gorm:update").Register("p10_perf_presence_write", func(tx *gorm.DB) {
		if tx.Statement.Table == "p10_scene_presence" {
			writes.Add(1)
			lastFields, _ = tx.Statement.Dest.(map[string]any)
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer callback.Remove("p10_perf_presence_write")
	before := time.Now().UnixMilli()
	if err := f.s.SyncP10Scene(f.ctx, f.id, false); err != nil {
		t.Fatal(err)
	}
	var own db.P10ScenePresence
	if err := db.GetIns().First(&own, "player_id=?", f.id).Error; err != nil {
		t.Fatal(err)
	}
	if writes.Load() != 1 || len(lastFields) != 1 || lastFields["updated_at"] == nil || own.UpdatedAt < before || len(peer.drain()) != 0 {
		t.Fatalf("unchanged heartbeat must refresh only the activity timestamp: writes=%d fields=%v at=%d", writes.Load(), lastFields, own.UpdatedAt)
	}
	yes, offlineDays, returnDays := true, int64(7), int64(3)
	cfg := db.AdminSproutSetting{Enabled: true, ReturnEnabled: &yes, ReturnOfflineDays: &offlineDays, ReturnDays: &returnDays}
	oldLogin := db.Profile{LastSproutLoginAt: oldAt}
	now := time.Now()
	if sproutReturnStart(cfg, oldLogin, oldAt, now) == 0 || sproutReturnStart(cfg, oldLogin, own.UpdatedAt, now) != 0 {
		t.Fatal("heartbeat did not preserve return-sprout last-active semantics")
	}
	t.Log("unchanged heartbeat: 1 timestamp-only write; return-sprout activity preserved")
	// Reconnect must still clear the 60-second offline marker and PK flag.
	if err := db.GetIns().Model(&db.P10ScenePresence{}).Where("player_id=?", f.id).Updates(map[string]any{"offline_until": time.Now().Add(time.Minute).UnixMilli(), "offline_pk_used": true}).Error; err != nil {
		t.Fatal(err)
	}
	writes.Store(0)
	if err := f.s.SyncP10Scene(f.ctx, f.id, false); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().First(&own, "player_id=?", f.id).Error; err != nil {
		t.Fatal(err)
	}
	if writes.Load() != 1 || own.OfflineUntil != 0 || own.OfflinePKUsed {
		t.Fatalf("reconnect marker was not cleared: writes=%d row=%+v", writes.Load(), own)
	}
	writes.Store(0)
	directoryReads.Store(0)
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, game *model.GameState) error {
		game.Position.Current = &pb.PointNew{X: proto.Float32(8), Y: proto.Float32(0), Z: proto.Float32(9)}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := f.s.SyncP10Scene(f.ctx, f.id, true); err != nil {
		t.Fatal(err)
	}
	messages := peer.drain()
	if writes.Load() != 1 || lastFields["frame"] == nil || directoryReads.Load() != 2 || len(messages) != 1 {
		t.Fatalf("movement write/directory/push: %d, %d, %v", writes.Load(), directoryReads.Load(), messages)
	}
	t.Logf("changed same-line move: %d presence directory reads, %d changed-frame write", directoryReads.Load(), writes.Load())
	move, ok := messages[0].(*pb.SCMovePush)
	if !ok || len(move.GetPosition()) != 1 || move.GetPosition()[0].GetCurrent().GetX() != 8 {
		t.Fatalf("incorrect movement frame: %v", messages)
	}
	if err := db.GetIns().Model(&db.P10ScenePresence{}).Where("player_id=?", f.id).Update("updated_at", time.Now().Add(-time.Second).UnixMilli()).Error; err != nil {
		t.Fatal(err)
	}
	writes.Store(0)
	directoryReads.Store(0)
	if err := f.s.SyncP10Scene(f.ctx, f.id, true); err != nil {
		t.Fatal(err)
	}
	if writes.Load() != 1 || len(lastFields) != 1 || lastFields["updated_at"] == nil || directoryReads.Load() != 2 || len(peer.drain()) != 1 {
		t.Fatalf("unchanged move should refresh activity and still push: writes=%d fields=%v reads=%d", writes.Load(), lastFields, directoryReads.Load())
	}
	t.Logf("unchanged move: %d directory reads, timestamp-only write", directoryReads.Load())
}

// Simulate a peer's scene commit while the caller's presence write is pending.
// The post-write directory check must prevent a push to that peer's old scene.
func TestP10SceneBatchRefreshAfterPresenceWrite(t *testing.T) {
	t.Setenv("HYJS_P10_MYSQL_DATABASE", "")
	f, b, _ := p10SceneSetup(t)
	self, peer := &p10SceneRecorder{}, &p10SceneRecorder{}
	p10SceneConnect(t, f, f.id, self)
	p10SceneConnect(t, f, b, peer)
	self.drain()
	peer.drain()
	var profile db.Profile
	if err := db.GetIns().First(&profile, b).Error; err != nil {
		t.Fatal(err)
	}
	_, game, err := decodeProfile(&profile)
	if err != nil {
		t.Fatal(err)
	}
	game.Position.Scene = proto.String("field_001")
	game.PositionLoading = false
	raw, err := json.Marshal(game)
	if err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, game *model.GameState) error {
		game.Position.Current = &pb.PointNew{X: proto.Float32(12), Y: proto.Float32(0), Z: proto.Float32(9)}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var hookErr error
	var changed atomic.Bool
	callback := db.GetIns().Callback().Update()
	if err := callback.Before("gorm:update").Register("p10_perf_pending_scene_change", func(tx *gorm.DB) {
		if tx.Statement.Table == "p10_scene_presence" && changed.CompareAndSwap(false, true) {
			hookErr = tx.Session(&gorm.Session{NewDB: true}).Model(&db.Profile{}).Where("id=?", b).Update("game", string(raw)).Error
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer callback.Remove("p10_perf_pending_scene_change")
	if err := f.s.SyncP10Scene(f.ctx, f.id, true); err != nil {
		t.Fatal(err)
	}
	if !changed.Load() || hookErr != nil || len(peer.drain()) != 0 {
		t.Fatalf("peer with changed scene received a stale push: invoked=%v error=%v", changed.Load(), hookErr)
	}
}

func TestP10SceneBatchProjectionMatchesCanonicalPower(t *testing.T) {
	t.Setenv("HYJS_P10_MYSQL_DATABASE", "")
	f, _, _ := p10SceneSetup(t)
	player, game := f.profile(t)
	frame, err := f.s.p10SceneMovePlayer(player, game, db.GetIns().WithContext(f.ctx))
	if err != nil {
		t.Fatal(err)
	}
	power, err := f.s.formationFightPower(player, game)
	if err != nil || frame.GetInfo().GetPower() != int64(power) {
		t.Fatalf("scene power diverged from canonical current-formation power: frame=%d canonical=%d error=%v", frame.GetInfo().GetPower(), power, err)
	}
}
