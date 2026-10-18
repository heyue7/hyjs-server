package service

import (
	"context"
	"errors"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	"testing"
	"time"
)

func TestAdminGameServersMetadataScopeAndOriginalOwnership(t *testing.T) {
	f := p1Setup(t)
	if err := AdminInitialize(f.ctx, ""); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeAdminGameServers(f.ctx); err != nil {
		t.Fatal(err)
	}
	u := db.AdminUser{Username: "realm-operator", PasswordHash: "fixture", Role: constUtil.AdminRoleOperator, Enabled: true}
	if err := db.GetIns().Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	actor := &model.AdminPrincipal{UserID: u.ID, Role: u.Role, Username: u.Username}
	list, err := AdminGameServers(f.ctx)
	if err != nil || len(list.Items) != 1 || list.Items[0].ID != 1 || !list.Items[0].RuntimeBound {
		t.Fatal(list, err)
	}
	planned, err := AdminCreateGameServer(f.ctx, actor, model.AdminGameServerCreateRequest{Name: "二区", OpenAt: time.Now().UnixMilli()})
	if err != nil || planned.Enabled || planned.RuntimeBound {
		t.Fatal(planned, err)
	}
	if _, err = AdminUpdateGameServer(f.ctx, actor, planned.ID, model.AdminGameServerUpdateRequest{Version: planned.Version, Name: planned.Name, OpenAt: planned.OpenAt, Enabled: true}); !errors.Is(err, constUtil.ErrAdminConfiguration) {
		t.Fatal("unbound realm enabled", err)
	}
	original := list.Items[0]
	updated, err := AdminUpdateGameServer(f.ctx, actor, original.ID, model.AdminGameServerUpdateRequest{Version: original.Version, Name: "一区·花开", OpenAt: original.OpenAt, Enabled: true})
	if err != nil || updated.Name != "一区·花开" {
		t.Fatal(updated, err)
	}
	if _, err = AdminUpdateGameServer(f.ctx, actor, original.ID, model.AdminGameServerUpdateRequest{Version: original.Version, Name: "stale", OpenAt: original.OpenAt}); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatal("stale overwritten", err)
	}
	own := context.WithValue(f.ctx, constUtil.AdminServerScopeContextKey, int64(1))
	other := context.WithValue(f.ctx, constUtil.AdminServerScopeContextKey, planned.ID)
	ownList, err := AdminGamePlayers(own, "", 1, 100)
	if err != nil || ownList.Total < 1 {
		t.Fatal(ownList, err)
	}
	foreign, err := AdminGamePlayers(other, "", 1, 100)
	if err != nil || foreign.Total != 0 {
		t.Fatal("cross realm listed", foreign, err)
	}
	if _, err := AdminGamePlayer(other, f.id); !errors.Is(err, constUtil.ErrAdminPermission) {
		t.Fatal("cross realm detail", err)
	}
	detail, err := AdminGamePlayer(own, f.id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AdminRenameGamePlayer(other, actor, f.id, model.AdminGamePlayerNameRequest{Version: detail.Version, Name: "不应改名", Reason: "cross scope"}); !errors.Is(err, constUtil.ErrAdminPermission) {
		t.Fatal(err)
	}
	if _, err := f.s.AdminCreateGamePlayer(other, actor, model.AdminGamePlayerCreateRequest{ServerID: planned.ID, Username: "unbound-create", Password: "fixture-password", Name: "错服", HeroID: "hero_043", Reason: "guard"}); !errors.Is(err, constUtil.ErrAdminPermission) {
		t.Fatal("unbound created", err)
	}
	f.restart(t)
	if err := f.s.InitializeAdminGameServers(f.ctx); err != nil {
		t.Fatal(err)
	}
	again, err := AdminGamePlayer(own, f.id)
	if err != nil || again.ServerID != 1 || again.Version != detail.Version {
		t.Fatal("ownership migration changed save/session", err)
	}
	runtime, err := f.s.runtimeGameServer(f.ctx)
	if err != nil || runtime.Name != "一区·花开" {
		t.Fatal(runtime, err)
	}
}
