package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"hyjs-server/db"
	"hyjs-server/model"
	"hyjs-server/service"
)

// TestAdminFieldHTTP validates actual auth/CSRF/routes and every original scheme
// reference without running six duplicate gameplay journeys.
func TestAdminFieldHTTP(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	if err := db.InitGame(ctx, filepath.Join(dir, "field-admin.sqlite")); err != nil {
		t.Fatal(err)
	}
	pool, _ := db.GetIns().DB()
	t.Cleanup(func() { pool.Close() })
	boot := filepath.Join(dir, "bootstrap.json")
	if err := os.WriteFile(boot, []byte(`{"username":"fieldfixtureadmin","password":"isolated-field-password-12345"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := service.AdminInitialize(ctx, boot); err != nil {
		t.Fatal(err)
	}
	if err := service.InitializeAdminOperations(ctx); err != nil {
		t.Fatal(err)
	}
	prior := gameService
	t.Cleanup(func() { gameService = prior })
	gameService = &service.GameService{}
	for _, load := range []func(string) error{gameService.LoadTables, gameService.LoadP8Tables, gameService.LoadP12WorldBossTables} {
		if err := load("../../analysis/tables"); err != nil {
			t.Fatal(err)
		}
	}
	if err := gameService.InitializeAdminFields(ctx); err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterAdminRoutes(router, "")
	call := func(method, path, body, csrf string, cookies []*http.Cookie) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/admin/api"+path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if csrf != "" {
			r.Header.Set("X-CSRF-Token", csrf)
		}
		for _, cookie := range cookies {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	if w := call("GET", "/fields/templates", "", "", nil); w.Code != 401 {
		t.Fatal("anonymous fields read", w.Code)
	}
	login := call("POST", "/auth/login", `{"username":"fieldfixtureadmin","password":"isolated-field-password-12345"}`, "", nil)
	if login.Code != 200 {
		t.Fatal("isolated login", login.Code)
	}
	var session struct {
		Data struct {
			CSRF string `json:"csrfToken"`
		}
	}
	json.Unmarshal(login.Body.Bytes(), &session)
	cookies := login.Result().Cookies()
	read := call("GET", "/fields/templates", "", "", cookies)
	if read.Code != 200 {
		t.Fatal("global scheme read", read.Code, read.Body.String())
	}
	var catalog struct{ Data model.AdminFieldListResponse }
	if err := json.Unmarshal(read.Body.Bytes(), &catalog); err != nil || len(catalog.Data.Items) != 6 {
		t.Fatal("scheme reference list", err)
	}
	for _, scheme := range catalog.Data.Items {
		req := model.AdminFieldSaveRequest{Name: scheme.Name + " HTTP保存", Configuration: scheme.Configuration}
		raw, _ := json.Marshal(req)
		if w := call("POST", "/fields/templates", string(raw), "", cookies); w.Code != 403 {
			t.Fatal("CSRF scheme save", w.Code)
		}
		if w := call("POST", "/fields/templates", string(raw), session.Data.CSRF, cookies); w.Code != 200 {
			t.Fatal("original reference rejected", scheme.Configuration.Scene, w.Code, w.Body.String())
		}
	}
	copyConfiguration := func() model.AdminFieldConfiguration {
		raw, err := json.Marshal(catalog.Data.Items[0].Configuration)
		if err != nil {
			t.Fatal(err)
		}
		var cfg model.AdminFieldConfiguration
		if err := json.Unmarshal(raw, &cfg); err != nil {
			t.Fatal(err)
		}
		return cfg
	}
	invalid := model.AdminFieldSaveRequest{Name: "非法点位", Configuration: copyConfiguration()}
	invalid.Configuration.MinePoints[0].ID = "invented-point"
	raw, _ := json.Marshal(invalid)
	if w := call("POST", "/fields/templates", string(raw), session.Data.CSRF, cookies); w.Code != 400 {
		t.Fatal("nonexistent client point accepted", w.Code)
	}
	badMember := model.AdminFieldSaveRequest{Name: "非法成员", Configuration: copyConfiguration()}
	badMember.Configuration.MonsterGroups[0].Members[0].Hp = -1
	raw, _ = json.Marshal(badMember)
	if w := call("POST", "/fields/templates", string(raw), session.Data.CSRF, cookies); w.Code != 400 {
		t.Fatal("negative monster HP accepted", w.Code)
	}
	badMember = model.AdminFieldSaveRequest{Name: "非法成员身份", Configuration: copyConfiguration()}
	badMember.Configuration.MonsterGroups[0].Members[0].HeroID = "invented-hero"
	raw, _ = json.Marshal(badMember)
	if w := call("POST", "/fields/templates", string(raw), session.Data.CSRF, cookies); w.Code != 400 {
		t.Fatal("replaced monster identity accepted", w.Code)
	}
	badBoss := model.AdminFieldSaveRequest{Name: "非法普通首领", Configuration: copyConfiguration()}
	badBoss.Configuration.OrdinaryBossGroups[0].RebornHours = 0
	raw, _ = json.Marshal(badBoss)
	if w := call("POST", "/fields/templates", string(raw), session.Data.CSRF, cookies); w.Code != 400 {
		t.Fatal("invalid ordinary boss cooldown accepted", w.Code)
	}
	badWorld := model.AdminFieldSaveRequest{Name: "非法世界首领", Configuration: copyConfiguration()}
	level := int64(51)
	badWorld.Configuration.WorldBossLevel = &level
	raw, _ = json.Marshal(badWorld)
	if w := call("POST", "/fields/templates", string(raw), session.Data.CSRF, cookies); w.Code != 400 {
		t.Fatal("invalid world boss level accepted", w.Code)
	}
	for _, check := range []struct {
		name   string
		mutate func(*model.AdminFieldConfiguration)
	}{
		{"空小怪成员", func(c *model.AdminFieldConfiguration) { c.MonsterGroups[0].Members = []model.AdminFieldCombatMember{} }},
		{"缺少小怪成员", func(c *model.AdminFieldConfiguration) { c.MonsterGroups[0].Members = c.MonsterGroups[0].Members[:1] }},
		{"重复小怪成员", func(c *model.AdminFieldConfiguration) { c.MonsterGroups[0].Members[1] = c.MonsterGroups[0].Members[0] }},
		{"小怪攻击越界", func(c *model.AdminFieldConfiguration) { c.MonsterGroups[0].Members[0].Attack = 1e12 + 1 }},
		{"普通首领组清空", func(c *model.AdminFieldConfiguration) { c.OrdinaryBossGroups = []model.AdminFieldBossGroup{} }},
		{"普通首领成员清空", func(c *model.AdminFieldConfiguration) {
			c.OrdinaryBossGroups[0].Members = []model.AdminFieldCombatMember{}
		}},
		{"普通首领刷新越界", func(c *model.AdminFieldConfiguration) { c.OrdinaryBossGroups[0].RebornHours = 169 }},
		{"世界首领成员清空", func(c *model.AdminFieldConfiguration) { c.WorldBossMembers = []model.AdminFieldCombatMember{} }},
		{"世界首领阵位改变", func(c *model.AdminFieldConfiguration) { c.WorldBossMembers[0].Location++ }},
		{"世界首领概率越界", func(c *model.AdminFieldConfiguration) { c.WorldBossMembers[0].Hit = 10001 }},
	} {
		t.Run(check.name, func(t *testing.T) {
			req := model.AdminFieldSaveRequest{Name: check.name, Configuration: copyConfiguration()}
			check.mutate(&req.Configuration)
			raw, _ := json.Marshal(req)
			// omitempty fields are explicitly present to exercise removal rather than legacy omission.
			var body map[string]any
			json.Unmarshal(raw, &body)
			cfg := body["configuration"].(map[string]any)
			if req.Configuration.MonsterGroups[0].Members != nil {
				cfg["monsterGroups"].([]any)[0].(map[string]any)["members"] = req.Configuration.MonsterGroups[0].Members
			}
			if req.Configuration.OrdinaryBossGroups != nil {
				cfg["ordinaryBossGroups"] = req.Configuration.OrdinaryBossGroups
			}
			if req.Configuration.WorldBossMembers != nil {
				cfg["worldBossMembers"] = req.Configuration.WorldBossMembers
			}
			if req.Configuration.OrdinaryBossGroups != nil && len(req.Configuration.OrdinaryBossGroups) > 0 {
				cfg["ordinaryBossGroups"] = []any{}
				for _, g := range req.Configuration.OrdinaryBossGroups {
					r, _ := json.Marshal(g)
					var group map[string]any
					json.Unmarshal(r, &group)
					if g.Members != nil {
						group["members"] = g.Members
					}
					cfg["ordinaryBossGroups"] = append(cfg["ordinaryBossGroups"].([]any), group)
				}
			}
			raw, _ = json.Marshal(body)
			if w := call("POST", "/fields/templates", string(raw), session.Data.CSRF, cookies); w.Code != 400 {
				t.Fatal("invalid combat configuration accepted", w.Code, w.Body.String())
			}
		})
	}
	if w := call("GET", "/fields/deployments", "", "", cookies); w.Code != 400 {
		t.Fatal("missing realm fell back to all realms", w.Code)
	}
}
