package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

// TestAdminPlayerProfile37ActualFieldsColdAndGuards 覆盖真实资料、资源、主线与历史保护。
func TestAdminPlayerProfile37ActualFieldsColdAndGuards(t *testing.T) {
	f := p1Setup(t)
	if err := AdminInitialize(f.ctx, ""); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeGlobalGameAccounts(f.ctx); err != nil {
		t.Fatal(err)
	}
	user := db.AdminUser{Username: "profile37-editor", PasswordHash: "fixture", Role: constUtil.AdminRoleOperator, Enabled: true}
	if err := db.GetIns().Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	actor := &model.AdminPrincipal{UserID: user.ID, Username: user.Username, Role: user.Role}
	ctx := context.WithValue(f.ctx, constUtil.AdminServerScopeContextKey, int64(1))
	var row db.Profile
	if err := db.GetIns().First(&row, f.id).Error; err != nil {
		t.Fatal(err)
	}
	// 在原存档放入未知扩展字段，验证编辑不会通过重编码丢弃兼容数据。
	var rawP, rawG map[string]json.RawMessage
	_ = json.Unmarshal([]byte(*row.Player), &rawP)
	_ = json.Unmarshal([]byte(*row.Game), &rawG)
	rawP["futurePlayer"] = json.RawMessage(`9223372036854775807`)
	rawG["futureGame"] = json.RawMessage(`{"keep":"扩展"}`)
	rawG["quests"] = json.RawMessage(`["existing-story"]`)
	encodedP, _ := json.Marshal(rawP)
	encodedG, _ := json.Marshal(rawG)
	if err := db.GetIns().Model(&row).Updates(map[string]any{"player": string(encodedP), "game": string(encodedG)}).Error; err != nil {
		t.Fatal(err)
	}
	detail, err := AdminGamePlayer(ctx, f.id)
	if err != nil {
		t.Fatal(err)
	}
	options, err := f.s.AdminGamePlayerEditOptions(ctx, f.id)
	if err != nil || options.LevelMax != 100 || options.VIPMax != 12 || len(options.Chapters) != 27 || len(options.BalanceLimits) != 10 {
		t.Fatal("edit options", options, err)
	}
	balances := map[string]int64{"coin": 123456, "diamond": 3000000001, "energy": 19, "stamina": 20, "light": 21, "point": 22, "popularity": 23, "score": 24, "coinpet": 25, "contribution": 26}
	req := model.AdminGamePlayerProfileRequest{Version: detail.Version, Name: "资料测试", Level: 20, Exp: 123, VIP: 3, Chapter: "1", Mission: "2", Scene: "town_main", CreatedAt: time.Now().Add(-72 * time.Hour).UnixMilli(), Balances: balances, Reason: "isolated actual profile editing"}
	other := context.WithValue(ctx, constUtil.AdminServerScopeContextKey, int64(2))
	if _, err := f.s.AdminUpdateGamePlayerProfile(other, actor, f.id, req); !errors.Is(err, constUtil.ErrAdminPermission) {
		t.Fatal("cross realm accepted", err)
	}
	result, err := f.s.AdminUpdateGamePlayerProfile(ctx, actor, f.id, req)
	if err != nil || result.Name != req.Name || result.Level != 20 || result.Exp != 123 || result.Vip != 3 || result.Mission != "2" || result.CreatedAt != req.CreatedAt {
		t.Fatal("profile did not save", result, err)
	}
	for key, value := range balances {
		if result.Balances[key] != value {
			t.Fatal("resource value differs", key, result.Balances[key])
		}
	}
	var after db.Profile
	if err := db.GetIns().First(&after, f.id).Error; err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(row.Session, after.Session) || !bytes.Equal(row.PasswordHash, after.PasswordHash) || !strings.Contains(*after.Player, `"futurePlayer":9223372036854775807`) || !strings.Contains(*after.Game, `"futureGame":{"keep":"扩展"}`) {
		t.Fatal("identity, credentials or unknown fields changed")
	}
	p, g := f.profile(t)
	if p.GetEnergyLastRefreshTime() == 0 || p.GetStaminaLastRefreshTime() == 0 || !copyMissionPassed(g, "1", "1") {
		t.Fatal("resource timer/progress was not updated")
	}
	f.s.refreshCopyProgress(g)
	if g.Copy.GetCurrentMissionId() != "2" {
		t.Fatal("progress reverted on normal game refresh")
	}
	if !strings.Contains(strings.Join(g.Quests, ","), "existing-story") || len(g.Items) != 0 || len(g.Warriors) != 1 {
		t.Fatal("history or inventory changed")
	}
	if _, err := f.s.AdminUpdateGamePlayerProfile(ctx, actor, f.id, req); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatal("stale save accepted", err)
	}
	f.restart(t)
	cold, err := AdminGamePlayer(ctx, f.id)
	if err != nil || cold.Version != result.Version || cold.Level != 20 || cold.CreatedAt != req.CreatedAt {
		t.Fatal("cold reload lost state", err)
	}
	// 权限撤销对持有旧principal的请求立即生效。
	if err := db.GetIns().Model(&user).Update("role", constUtil.AdminRoleViewer).Error; err != nil {
		t.Fatal(err)
	}
	req.Version = cold.Version
	if _, err := f.s.AdminUpdateGamePlayerProfile(ctx, actor, f.id, req); !errors.Is(err, constUtil.ErrAdminPermission) {
		t.Fatal("revoked user edited", err)
	}
}

// TestAdminSavedJSONMerge37PreservesUnknownNestedMembers 验证增删集合后仍保留同一GUID的未知属性。
func TestAdminSavedJSONMerge37PreservesUnknownNestedMembers(t *testing.T) {
	saved := `{"warriors":[{"guid":"a","level":1,"hidden":{"n":9223372036854775807}},{"guid":"b","level":2,"hidden":"keep"}],"unrelated":true}`
	before := map[string]any{"warriors": []any{map[string]any{"guid": "a", "level": 1}, map[string]any{"guid": "b", "level": 2}}}
	after := map[string]any{"warriors": []any{map[string]any{"guid": "b", "level": 3}}}
	result, err := adminMergeSavedJSON(saved, before, after)
	if err != nil || !strings.Contains(result, `"hidden":"keep"`) || strings.Contains(result, `"guid":"a"`) || !strings.Contains(result, `"unrelated":true`) {
		t.Fatal("nested fields lost", result, err)
	}
	// 只更新单个属性，不将未保存的类型化默认字段写成null。
	before = map[string]any{"level": 1, "defaulted": map[string]any{}}
	after = map[string]any{"level": 2, "defaulted": map[string]any{}}
	result, err = adminMergeSavedJSON(`{"level":1,"other":true}`, before, after)
	if err != nil || strings.Contains(result, "defaulted") {
		t.Fatal("projection default leaked", result, err)
	}
}
