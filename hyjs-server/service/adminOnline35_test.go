package service

import (
	"context"
	"encoding/json"
	"errors"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	"strconv"
	"strings"
	"testing"
)

func TestAdminOnline35SnapshotAuthorizationAndFilters(t *testing.T) {
	f := p1Setup(t)
	if err := AdminInitialize(f.ctx, ""); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().AutoMigrate(&db.AdminGameServer{}, &db.GameAccount{}); err != nil {
		t.Fatal(err)
	}
	actorRow := db.AdminUser{Username: "online-viewer", PasswordHash: "unused", Role: constUtil.AdminRoleViewer, Enabled: true}
	if err := db.GetIns().Create(&actorRow).Error; err != nil {
		t.Fatal(err)
	}
	actor := &model.AdminPrincipal{UserID: actorRow.ID, Role: actorRow.Role}
	servers := []db.AdminGameServer{{ID: f.s.OriginalServerID(), Name: "在线一区"}, {ID: 99, Name: "规划区"}}
	if err := db.GetIns().Create(&servers).Error; err != nil {
		t.Fatal(err)
	}
	var row db.Profile
	if err := db.GetIns().First(&row, f.id).Error; err != nil {
		t.Fatal(err)
	}
	key := p10ClientKey{f.s.p10ClientOwner(), f.id}
	t.Cleanup(func() { p10ClientSinks.Delete(key) })
	get := func(ctx context.Context, q string, page, size int64) *model.AdminOnlinePlayersResponse {
		t.Helper()
		out, err := f.s.AdminOnlinePlayers(ctx, actor, q, page, size)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	if out := get(f.ctx, "", 1, 20); out.Total != 0 {
		t.Fatal("persisted session counted as online")
	}
	other := &GameService{}
	otherKey := p10ClientKey{other, f.id}
	p10ClientSinks.Store(otherKey, &p10ClientSink{session: row.Session})
	defer p10ClientSinks.Delete(otherKey)
	if out := get(f.ctx, "", 1, 20); out.Total != 0 {
		t.Fatal("another runtime counted")
	}
	old := &p10ClientSink{session: append([]byte(nil), row.Session...)}
	p10ClientSinks.Store(key, old)
	out := get(f.ctx, "", 1, 20)
	if out.Total != 1 || len(out.Items) != 1 || out.Servers[0].Online != 1 || !out.Servers[0].Connected || out.Servers[1].Connected || out.Servers[1].Online != 0 || out.ObservedAt <= 0 {
		t.Fatalf("snapshot: %+v", out)
	}
	if out := get(f.ctx, strconv.FormatInt(f.id, 10), 1, 1); out.Total != 1 {
		t.Fatal("exact ID filter")
	}
	if out := get(f.ctx, out.Items[0].Name, 1, 1); out.Total != 1 {
		t.Fatal("name filter")
	}
	if out := get(f.ctx, "no-match", 1, 1); out.Total != 0 || out.Servers[0].Online != 1 {
		t.Fatal("search changed server count")
	}
	if out := get(f.ctx, "", 2, 1); out.Total != 1 || len(out.Items) != 0 {
		t.Fatal("pagination")
	}
	scoped := context.WithValue(f.ctx, constUtil.AdminServerScopeContextKey, int64(99))
	if out := get(scoped, "", 1, 20); out.Total != 0 || out.Servers[0].Online != 1 {
		t.Fatal("server filtering changed count")
	}
	raw, _ := json.Marshal(out)
	for _, secret := range []string{`"session"`, `"password"`, `"game"`, `"accountId"`, `"uid"`} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("secret exposed", secret)
		}
	}
	// 旧socket失效后排除；旧注销动作不得移除后来注册的新连接。
	newSession := []byte("online-session35")
	if err := db.GetIns().Model(&row).Update("session", newSession).Error; err != nil {
		t.Fatal(err)
	}
	if out := get(f.ctx, "", 1, 20); out.Total != 0 {
		t.Fatal("old session counted")
	}
	latest := &p10ClientSink{session: newSession}
	p10ClientSinks.Store(key, latest)
	if p10ClientSinks.CompareAndDelete(key, old) {
		t.Fatal("old disconnect deleted replacement")
	}
	if out := get(f.ctx, "", 1, 20); out.Total != 1 {
		t.Fatal("replacement missing")
	}
	account := db.GameAccount{Username: "online-account", Nickname: "account", Enabled: true, AuthRevision: 2, PasswordSalt: []byte{}, PasswordHash: []byte{}}
	if err := db.GetIns().Create(&account).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&row).Updates(map[string]any{"account_id": account.ID, "account_auth_revision": 2}).Error; err != nil {
		t.Fatal(err)
	}
	if out := get(f.ctx, "", 1, 20); out.Total != 1 {
		t.Fatal("valid global account missing")
	}
	if err := db.GetIns().Model(&account).Update("auth_revision", 3).Error; err != nil {
		t.Fatal(err)
	}
	if out := get(f.ctx, "", 1, 20); out.Total != 0 {
		t.Fatal("revoked account session counted")
	}
	if err := db.GetIns().Model(&account).Updates(map[string]any{"auth_revision": 2, "enabled": false}).Error; err != nil {
		t.Fatal(err)
	}
	if out := get(f.ctx, "", 1, 20); out.Total != 0 {
		t.Fatal("disabled account counted")
	}
	if err := db.GetIns().Model(&actorRow).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.AdminOnlinePlayers(f.ctx, actor, "", 1, 20); !errors.Is(err, constUtil.ErrAdminPermission) {
		t.Fatal("disabled operator allowed", err)
	}
}
