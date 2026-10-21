package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

// TestAdminPlayerLevelPreservesSaveAndRejectsStaleScope 覆盖实际存档、账号索引与保护边界。
func TestAdminPlayerLevelPreservesSaveAndRejectsStaleScope(t *testing.T) {
	f := p1Setup(t)
	if err := AdminInitialize(f.ctx, ""); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeGlobalGameAccounts(f.ctx); err != nil {
		t.Fatal(err)
	}
	user := db.AdminUser{Username: "level-editor", PasswordHash: "fixture", Role: constUtil.AdminRoleOperator, Enabled: true}
	if err := db.GetIns().Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	actor := &model.AdminPrincipal{UserID: user.ID, Username: user.Username, Role: user.Role}
	ctx := context.WithValue(f.ctx, constUtil.AdminServerScopeContextKey, int64(1))
	var before db.Profile
	if err := db.GetIns().First(&before, f.id).Error; err != nil {
		t.Fatal(err)
	}
	var prepared map[string]json.RawMessage
	if err := json.Unmarshal([]byte(*before.Player), &prepared); err != nil {
		t.Fatal(err)
	}
	prepared["exp"] = json.RawMessage("1650")
	prepared["futureField"] = json.RawMessage(`{"keep":"原样"}`)
	encoded, err := json.Marshal(prepared)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&before).Update("player", string(encoded)).Error; err != nil {
		t.Fatal(err)
	}
	preparedJSON := string(encoded)
	before.Player = &preparedJSON
	detail, err := AdminGamePlayer(ctx, f.id)
	if err != nil {
		t.Fatal(err)
	}
	req := model.AdminGamePlayerLevelRequest{Version: detail.Version, Level: 100, Reason: "isolated level edit"}
	other := context.WithValue(ctx, constUtil.AdminServerScopeContextKey, int64(2))
	if _, err := AdminSetGamePlayerLevel(other, actor, f.id, req); !errors.Is(err, constUtil.ErrAdminPermission) {
		t.Fatal("cross-realm edit accepted", err)
	}
	if err := db.GetIns().Model(&user).Update("role", constUtil.AdminRoleViewer).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := AdminSetGamePlayerLevel(ctx, actor, f.id, req); !errors.Is(err, constUtil.ErrAdminPermission) {
		t.Fatal("revoked operator edited", err)
	}
	if err := db.GetIns().Model(&user).Update("role", constUtil.AdminRoleOperator).Error; err != nil {
		t.Fatal(err)
	}
	result, err := AdminSetGamePlayerLevel(ctx, actor, f.id, req)
	if err != nil || result.Level != 100 || result.Version == detail.Version {
		t.Fatal("level edit failed", result, err)
	}
	var after db.Profile
	if err := db.GetIns().First(&after, f.id).Error; err != nil {
		t.Fatal(err)
	}
	var oldPlayer, newPlayer map[string]json.RawMessage
	if json.Unmarshal([]byte(*before.Player), &oldPlayer) != nil || json.Unmarshal([]byte(*after.Player), &newPlayer) != nil {
		t.Fatal("invalid saved player")
	}
	if string(newPlayer["exp"]) != "0" {
		t.Fatal("level experience not reset")
	}
	for _, key := range []string{"level", "exp"} {
		delete(oldPlayer, key)
		delete(newPlayer, key)
	}
	if !reflect.DeepEqual(oldPlayer, newPlayer) || *before.Game != *after.Game || !bytes.Equal(before.Session, after.Session) {
		t.Fatal("unrelated save or session changed")
	}
	var role db.GameAccountRole
	if err := db.AccountsTable(db.GetIns(), "game_account_roles").Where("player_id=? AND server_id=1", f.id).First(&role).Error; err != nil || role.Level != 100 {
		t.Fatal("account role level not synchronized", err)
	}
	if _, err := AdminSetGamePlayerLevel(ctx, actor, f.id, req); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatal("stale version accepted", err)
	}
	f.restart(t)
	cold, err := AdminGamePlayer(ctx, f.id)
	if err != nil || cold.Level != 100 || cold.Version != result.Version {
		t.Fatal("cold reload lost edit", err)
	}
}
