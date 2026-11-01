package service

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p10RouteFixture(t *testing.T) (context.Context, *GameService, *db.Profile) {
	t.Helper()
	ctx := context.Background()
	if err := db.InitGame(ctx, filepath.Join(t.TempDir(), "route.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		sqlDB, err := db.GetIns().DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.GetIns().AutoMigrate(&db.P10ScenePresence{}); err != nil {
		t.Fatal(err)
	}
	player := `{"warriorBagSize":10,"equipBagSize":10,"petBagSize":10}`
	game := `{"position":{"scene":"town_main"}}`
	profile := &db.Profile{Id: 1, Uid: "route-test", Session: []byte("0123456789abcdef"), Player: &player, Game: &game}
	if err := db.GetIns().Create(profile).Error; err != nil {
		t.Fatal(err)
	}
	return ctx, &GameService{}, profile
}

func p10RouteCountingSnapshot(count *int) *p10SceneRouteSnapshot {
	return &p10SceneRouteSnapshot{decode: func(profile *db.Profile) (*pb.Player, *model.GameState, error) {
		(*count)++
		return decodeProfile(profile)
	}}
}

func TestP10SceneRouteSnapshotKeepsBothReadsAndReusesDecode(t *testing.T) {
	ctx, service, profile := p10RouteFixture(t)
	queries, decodes := 0, 0
	callback := db.GetIns().Callback().Query()
	if err := callback.After("gorm:query").Register("p10_route_profile_reads", func(tx *gorm.DB) {
		if tx.Statement.Table == "profiles" {
			queries++
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer callback.Remove("p10_route_profile_reads")
	route := p10RouteCountingSnapshot(&decodes)
	if handled, err := service.syncP13UnionFightField(ctx, profile.Id, false, route); handled || err != nil {
		t.Fatalf("ordinary scene handled by union fight: handled=%v error=%v", handled, err)
	}
	if handled, err := service.syncP13GuildRoom(ctx, profile.Id, false, route); handled || err != nil {
		t.Fatalf("ordinary scene handled by guild room: handled=%v error=%v", handled, err)
	}
	if queries != 2 || decodes != 1 {
		t.Fatalf("expected both profile reads and one complete decode: reads=%d decodes=%d", queries, decodes)
	}
	// The public methods remain independent calls, each with its own read.
	if handled, err := service.SyncP13UnionFightField(ctx, profile.Id, false); handled || err != nil {
		t.Fatalf("standalone union route: handled=%v error=%v", handled, err)
	}
	if handled, err := service.SyncP13GuildRoom(ctx, profile.Id, false); handled || err != nil {
		t.Fatalf("standalone guild route: handled=%v error=%v", handled, err)
	}
	if queries != 4 {
		t.Fatalf("standalone methods must each read the profile: reads=%d", queries)
	}
	// Exercise the public composition as well as the two individually counted routes.
	if err := service.SyncP10Scene(ctx, profile.Id, false); err != nil {
		t.Fatal(err)
	}
	if queries != 6 {
		t.Fatalf("public scene sync must keep both route observations: reads=%d", queries)
	}
}

func TestP10SceneRouteSnapshotSeesSceneAndProfileChanges(t *testing.T) {
	ctx, service, profile := p10RouteFixture(t)
	decodes := 0
	route := p10RouteCountingSnapshot(&decodes)
	if handled, err := service.syncP13UnionFightField(ctx, profile.Id, false, route); handled || err != nil {
		t.Fatalf("initial route: handled=%v error=%v", handled, err)
	}
	// A transfer may commit after the union-fight read and before the guild read.
	room := `{"position":{"scene":"town_union"}}`
	if err := db.GetIns().Model(&db.Profile{}).Where("id=?", profile.Id).Update("game", room).Error; err != nil {
		t.Fatal(err)
	}
	if handled, err := service.syncP13GuildRoom(ctx, profile.Id, false, route); !handled || err != nil {
		t.Fatalf("committed guild transfer was missed: handled=%v error=%v", handled, err)
	}
	if decodes != 2 {
		t.Fatalf("changed profile should be decoded again: %d", decodes)
	}
	field := `{"position":{"scene":"fight_union"}}`
	if err := db.GetIns().Model(&db.Profile{}).Where("id=?", profile.Id).Update("game", field).Error; err != nil {
		t.Fatal(err)
	}
	if handled, err := service.SyncP13UnionFightField(ctx, profile.Id, false); !handled || err != nil {
		t.Fatalf("union fight scene was missed: handled=%v error=%v", handled, err)
	}
	// Even an unrelated column change prevents reuse of the previous row.
	var current db.Profile
	if err := db.GetIns().First(&current, profile.Id).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := route.routeGame(&current); err != nil {
		t.Fatal(err)
	}
	changed := current
	changed.Uid = "changed-uid"
	if _, err := route.routeGame(&changed); err != nil {
		t.Fatal(err)
	}
	if decodes != 4 {
		t.Fatalf("whole-profile changes must invalidate the decoded state: %d", decodes)
	}
}

func TestP10SceneRouteSnapshotReadFailureAndInvalidInputs(t *testing.T) {
	ctx, service, profile := p10RouteFixture(t)
	queries, decodes := 0, 0
	callback := db.GetIns().Callback().Query()
	if err := callback.Before("gorm:query").Register("p10_route_fail_first_read", func(tx *gorm.DB) {
		if tx.Statement.Table == "profiles" {
			queries++
			if queries == 1 {
				tx.AddError(errors.New("injected temporary profile read failure"))
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	route := p10RouteCountingSnapshot(&decodes)
	if handled, err := service.syncP13UnionFightField(ctx, profile.Id, false, route); handled || err != nil {
		t.Fatalf("initial read failure must retain original behavior: handled=%v error=%v", handled, err)
	}
	if handled, err := service.syncP13GuildRoom(ctx, profile.Id, false, route); handled || err != nil {
		t.Fatalf("guild route must retry the read: handled=%v error=%v", handled, err)
	}
	if queries != 2 || decodes != 1 {
		t.Fatalf("temporary read failure prevented retry: reads=%d decodes=%d", queries, decodes)
	}
	if err := callback.Remove("p10_route_fail_first_read"); err != nil {
		t.Fatal(err)
	}
	badGame := `{"position":{"scene":"town_main"},"warriors":[}`
	if err := db.GetIns().Model(&db.Profile{}).Where("id=?", profile.Id).Update("game", badGame).Error; err != nil {
		t.Fatal(err)
	}
	if handled, err := service.syncP13UnionFightField(ctx, profile.Id, false, route); handled || err == nil {
		t.Fatalf("corrupt complete profile must fail: handled=%v error=%v", handled, err)
	}
	if route.profile != nil || route.game != nil {
		t.Fatal("a failed decode retained an earlier cached result")
	}
	if err := db.GetIns().Model(&db.Profile{}).Where("id=?", profile.Id).Update("game", nil).Error; err != nil {
		t.Fatal(err)
	}
	if handled, err := service.SyncP13UnionFightField(ctx, profile.Id, false); handled || err != nil {
		t.Fatalf("nil game union route: handled=%v error=%v", handled, err)
	}
	if handled, err := service.SyncP13GuildRoom(ctx, profile.Id, false); handled || err != nil {
		t.Fatalf("nil game guild route: handled=%v error=%v", handled, err)
	}
	validGame := `{"position":{"scene":"town_main"}}`
	if err := db.GetIns().Model(&db.Profile{}).Where("id=?", profile.Id).Updates(map[string]any{"game": validGame, "player": nil}).Error; err != nil {
		t.Fatal(err)
	}
	if handled, err := service.SyncP13UnionFightField(ctx, profile.Id, false); handled || err == nil {
		t.Fatalf("nil player union route must reject the profile: handled=%v error=%v", handled, err)
	}
	if handled, err := service.SyncP13GuildRoom(ctx, profile.Id, false); handled || err != nil {
		t.Fatalf("nil player guild route must skip it: handled=%v error=%v", handled, err)
	}
}

func TestP10SceneRouteSnapshotRetainsGuildExitCleanup(t *testing.T) {
	ctx, service, profile := p10RouteFixture(t)
	old := db.P10ScenePresence{PlayerID: profile.Id, Scene: constUtil.P13GuildRoomScopePrefix + "old", Line: 1, SessionHash: p6Digest(profile.Session), Frame: []byte{}}
	if err := db.GetIns().Create(&old).Error; err != nil {
		t.Fatal(err)
	}
	key := p10ClientKey{service.p10ClientOwner(), profile.Id}
	sink := &p10ClientSink{session: append([]byte(nil), profile.Session...)}
	p10ClientSinks.Store(key, sink)
	t.Cleanup(func() { p10ClientSinks.CompareAndDelete(key, sink) })
	decodes := 0
	route := p10RouteCountingSnapshot(&decodes)
	if handled, err := service.syncP13UnionFightField(ctx, profile.Id, false, route); handled || err != nil {
		t.Fatalf("union preflight: handled=%v error=%v", handled, err)
	}
	if handled, err := service.syncP13GuildRoom(ctx, profile.Id, false, route); handled || err != nil {
		t.Fatalf("guild exit: handled=%v error=%v", handled, err)
	}
	var after db.P10ScenePresence
	if err := db.GetIns().First(&after, "player_id=?", profile.Id).Error; err != nil {
		t.Fatal(err)
	}
	if after.SessionHash != "" || decodes != 1 {
		t.Fatalf("old guild presence was not cleaned: row=%+v decodes=%d", after, decodes)
	}
}

func TestP10SceneRouteSnapshotReauthorizesSessionAfterPreflight(t *testing.T) {
	ctx, service, profile := p10RouteFixture(t)
	old := db.P10ScenePresence{PlayerID: profile.Id, Scene: constUtil.P13GuildRoomScopePrefix + "old", Line: 1, SessionHash: p6Digest(profile.Session), Frame: []byte{}}
	if err := db.GetIns().Create(&old).Error; err != nil {
		t.Fatal(err)
	}
	key := p10ClientKey{service.p10ClientOwner(), profile.Id}
	sink := &p10ClientSink{session: append([]byte(nil), profile.Session...)}
	p10ClientSinks.Store(key, sink)
	t.Cleanup(func() { p10ClientSinks.CompareAndDelete(key, sink) })
	decodes := 0
	route := p10RouteCountingSnapshot(&decodes)
	if handled, err := service.syncP13UnionFightField(ctx, profile.Id, false, route); handled || err != nil {
		t.Fatalf("union preflight: handled=%v error=%v", handled, err)
	}
	var hookErr error
	profileReads := 0
	callback := db.GetIns().Callback().Query()
	if err := callback.After("gorm:query").Register("p10_route_rotate_after_preflight", func(tx *gorm.DB) {
		if tx.Statement.Table == "profiles" {
			profileReads++
			if profileReads == 1 {
				hookErr = tx.Session(&gorm.Session{NewDB: true}).Model(&db.Profile{}).Where("id=?", profile.Id).Update("session", []byte("fedcba9876543210")).Error
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer callback.Remove("p10_route_rotate_after_preflight")
	handled, err := service.syncP13GuildRoom(ctx, profile.Id, false, route)
	if hookErr != nil || !handled || err == nil || !strings.Contains(err.Error(), "会话无效") {
		t.Fatalf("rotated session was not reauthorized: handled=%v error=%v hook=%v", handled, err, hookErr)
	}
	var after db.P10ScenePresence
	if err := db.GetIns().First(&after, "player_id=?", profile.Id).Error; err != nil {
		t.Fatal(err)
	}
	if after.SessionHash != old.SessionHash || decodes != 1 || profileReads != 2 {
		t.Fatalf("preflight changed auth or cleanup: presence=%+v decodes=%d reads=%d", after, decodes, profileReads)
	}
}

func TestP10SceneRouteSnapshotRechecksSceneAfterPreflight(t *testing.T) {
	ctx, service, profile := p10RouteFixture(t)
	room := `{"position":{"scene":"town_union"}}`
	if err := db.GetIns().Model(&db.Profile{}).Where("id=?", profile.Id).Update("game", room).Error; err != nil {
		t.Fatal(err)
	}
	old := db.P10ScenePresence{PlayerID: profile.Id, Scene: constUtil.P13GuildRoomScopePrefix + "old", Line: 1, SessionHash: p6Digest(profile.Session), Frame: []byte{}}
	if err := db.GetIns().Create(&old).Error; err != nil {
		t.Fatal(err)
	}
	key := p10ClientKey{service.p10ClientOwner(), profile.Id}
	sink := &p10ClientSink{session: append([]byte(nil), profile.Session...)}
	p10ClientSinks.Store(key, sink)
	t.Cleanup(func() { p10ClientSinks.CompareAndDelete(key, sink) })
	decodes := 0
	route := p10RouteCountingSnapshot(&decodes)
	if handled, err := service.syncP13UnionFightField(ctx, profile.Id, false, route); handled || err != nil {
		t.Fatalf("union preflight: handled=%v error=%v", handled, err)
	}
	var hookErr error
	profileReads := 0
	callback := db.GetIns().Callback().Query()
	if err := callback.After("gorm:query").Register("p10_route_leave_after_preflight", func(tx *gorm.DB) {
		if tx.Statement.Table == "profiles" {
			profileReads++
			if profileReads == 1 {
				town := `{"position":{"scene":"town_main"}}`
				hookErr = tx.Session(&gorm.Session{NewDB: true}).Model(&db.Profile{}).Where("id=?", profile.Id).Update("game", town).Error
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer callback.Remove("p10_route_leave_after_preflight")
	handled, err := service.syncP13GuildRoom(ctx, profile.Id, false, route)
	if hookErr != nil || handled || err != nil {
		t.Fatalf("committed guild exit was not rechecked under the lock: handled=%v error=%v hook=%v", handled, err, hookErr)
	}
	var after db.P10ScenePresence
	if err := db.GetIns().First(&after, "player_id=?", profile.Id).Error; err != nil {
		t.Fatal(err)
	}
	if after.SessionHash != "" || decodes != 1 || profileReads != 2 {
		t.Fatalf("locked position recheck did not clean the old room: presence=%+v decodes=%d reads=%d", after, decodes, profileReads)
	}
}
