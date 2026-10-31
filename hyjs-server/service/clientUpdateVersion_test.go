package service

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

func TestClientUpdateSemanticVersions(t *testing.T) {
	for _, test := range []struct {
		version string
		valid   bool
	}{
		{"0.0.0", true}, {"1.10.0", true}, {"999.999.999", true},
		{"", false}, {"1.1", false}, {"1.1.1.0", false}, {"01.1.1", false},
		{"1.01.1", false}, {"+1.1.1", false}, {"-1.1.1", false},
		{"1000.0.0", false}, {"1.2.3-beta", false}, {" 1.2.3", false}, {"１.2.3", false},
	} {
		if got := ClientUpdateVersionValid(test.version); got != test.valid {
			t.Errorf("ClientUpdateVersionValid(%q)=%v", test.version, got)
		}
	}
	for _, test := range []struct {
		version, minimum string
		below            bool
	}{
		{"1.9.999", "1.10.0", true}, {"1.10.0", "1.9.999", false},
		{"1.10.0", "1.10.0", false}, {"1.10.1", "1.10.0", false},
		{"", "1.0.0", true}, {"1.0.0", "", false}, {"1.0.0", "bad", true},
	} {
		if got := clientUpdateVersionBelow(test.version, test.minimum); got != test.below {
			t.Errorf("below(%q,%q)=%v", test.version, test.minimum, got)
		}
	}
	startup := "1234567890abcdef1234567890abcdef"
	for _, test := range []struct {
		app     string
		build   int64
		version string
	}{
		{"hyjs/2026110101/" + startup, 2026110101, "1.1.0"},
		{"hyjs/2026110102/" + startup, 2026110102, ""},
		{"hyjs/2026110102/" + startup + "/1.10.0", 2026110102, "1.10.0"},
		{"hyjs/2026110101/" + startup + "/01.1.0", 0, ""},
	} {
		build, _, version := parseClientUpdateVersion(test.app)
		if build != test.build || version != test.version {
			t.Errorf("parse(%q)=(%d,%q)", test.app, build, version)
		}
	}
}

// 复现旧库三张表并验证新增列可随 AutoMigrate 安全默认。
type legacyClientUpdatePolicy struct {
	ID                                                                        int64 `gorm:"primaryKey;autoIncrement:false"`
	ReleaseID, LatestBuild, MinLoginBuild, MinRunBuild, Revision, NoticeBatch int64
	NoticeEnabled, Maintenance                                                bool
	MaintenanceStartAt, MaintenanceEndAt, MaintenanceEpoch, LoginGateAt       int64
	VersionName, Title, Content, Mode                                         string
}

func (legacyClientUpdatePolicy) TableName() string { return "client_update_policy" }

type legacyClientUpdateRelease struct {
	ID                                                               int64 `gorm:"primaryKey;autoIncrement"`
	BuildCode                                                        int64
	VersionName, Title, Content, Mode                                string
	MinLoginBuild, MinRunBuild, MaintenanceStartAt, MaintenanceEndAt int64
	State                                                            string
	PublishedAt, UpdatedAt                                           int64
	PublishedBy                                                      string
}

func (legacyClientUpdateRelease) TableName() string { return "client_update_releases" }

type legacyClientUpdateSession struct {
	PlayerID                                              int64 `gorm:"primaryKey;autoIncrement:false"`
	SessionHash                                           string
	BuildCode                                             int64
	StartupID                                             string
	LoginAt, LastSeenAt, DisconnectedAt, MaintenanceEpoch int64
	Connected                                             bool
}

func (legacyClientUpdateSession) TableName() string { return "client_update_sessions" }

func TestClientUpdateSemanticSQLite(t *testing.T) {
	ctx := context.Background()
	if err := db.InitGame(ctx, filepath.Join(t.TempDir(), "semantic.sqlite")); err != nil {
		t.Fatal(err)
	}
	d := db.GetIns()
	sqlDB, err := d.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { clientUpdateReady.Delete(clientUpdateDBKey(d)); _ = sqlDB.Close() })
	if err = d.AutoMigrate(&legacyClientUpdatePolicy{}, &legacyClientUpdateRelease{}, &legacyClientUpdateSession{}, &db.AdminUser{}, &db.AdminAudit{}); err != nil {
		t.Fatal(err)
	}
	oldPolicy := legacyClientUpdatePolicy{ID: 1, LatestBuild: 2026110101, MinLoginBuild: 100, MinRunBuild: 50, VersionName: "1.1.0"}
	oldRelease := legacyClientUpdateRelease{BuildCode: 2026110101, VersionName: "1.1.0", State: "published", PublishedAt: 1}
	oldSession := legacyClientUpdateSession{PlayerID: 9999, BuildCode: 2026110101}
	for _, value := range []any{&oldPolicy, &oldRelease, &oldSession} {
		if err = d.Create(value).Error; err != nil {
			t.Fatal(err)
		}
	}
	s := &GameService{}
	if err = s.InitializeClientUpdates(ctx); err != nil {
		t.Fatal("migrate old rows:", err)
	}
	var migrated db.ClientUpdatePolicy
	var migratedRelease db.ClientUpdateRelease
	var migratedSession db.ClientUpdateSession
	if err = d.First(&migrated, 1).Error; err != nil {
		t.Fatal(err)
	}
	if err = d.First(&migratedRelease, oldRelease.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err = d.First(&migratedSession, 9999).Error; err != nil {
		t.Fatal(err)
	}
	if migrated.VersionPolicy || migrated.MinLoginVersion != "" || migrated.MinRunVersion != "" || migrated.MinLoginBuild != 100 || migrated.MinRunBuild != 50 || migratedRelease.VersionPolicy || migratedSession.ClientVersion != "" {
		t.Fatalf("migration changed legacy semantics: %+v %+v %+v", migrated, migratedRelease, migratedSession)
	}
	user := db.AdminUser{Username: "semantic-test", Role: constUtil.AdminRoleAdmin, Enabled: true}
	if err = d.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	actor := &model.AdminPrincipal{UserID: user.ID, Username: user.Username, Role: user.Role}
	overview := func() *model.AdminClientUpdatesResponse {
		t.Helper()
		out, err := s.AdminClientUpdates(ctx, actor)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	draft := func(version, mode string, start, end int64) int64 {
		t.Helper()
		out, err := s.AdminClientUpdateDraft(ctx, actor, model.AdminClientUpdateDraftRequest{Version: overview().Version, VersionPolicy: true, VersionName: version, Mode: mode, Title: "更新", Content: "更新说明", Reason: "test", MaintenanceStartAt: start, MaintenanceEndAt: end, BuildCode: 999, MinLoginBuild: 999, MinRunBuild: 999})
		if err != nil {
			t.Fatal(err)
		}
		return out.Items[0].ID
	}
	action := func(id int64, name, requestID string) *model.AdminClientUpdatesResponse {
		t.Helper()
		out, err := s.AdminClientUpdateAction(ctx, actor, model.AdminClientUpdateActionRequest{ID: id, Version: overview().Version, Action: name, RequestID: requestID, Reason: "test"})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return out
	}
	type client struct {
		profile          *db.Profile
		build            int64
		version, startup string
	}
	newClient := func(i int, build int64, version string, connected bool) client {
		t.Helper()
		profile := &db.Profile{Uid: fmt.Sprintf("semantic-%d", i), Session: []byte(fmt.Sprintf("session-%08d", i))}
		if err := d.Create(profile).Error; err != nil {
			t.Fatal(err)
		}
		startup := fmt.Sprintf("%032x", i)
		app := fmt.Sprintf("hyjs/%d/%s", build, startup)
		if version != "" {
			app += "/" + version
		}
		if err := s.BindClientLogin(ctx, profile, app); err != nil {
			t.Fatal(err)
		}
		if connected {
			if err := s.ClientUpdateConnected(ctx, profile.Id, profile.Session); err != nil {
				t.Fatal(err)
			}
			key := p10ClientKey{s.p10ClientOwner(), profile.Id}
			p10ClientSinks.Store(key, &p10ClientSink{session: profile.Session, send: func(proto.Message) error { return nil }, close: func() {}})
			t.Cleanup(func() { p10ClientSinks.Delete(key) })
		}
		return client{profile, build, version, startup}
	}
	old := newClient(1, 2026110101, "", true)
	unknown := newClient(2, 2026110105, "", true)
	same := newClient(3, 2026110102, "1.1.1", true)
	higher := newClient(4, 2026110103, "1.10.0", true)
	behind := newClient(5, 2026110104, "1.0.9", true)
	offline := newClient(6, 2026110101, "", false)
	_ = behind
	if err = d.Model(&db.ClientUpdateSession{}).Where("player_id=?", old.profile.Id).Update("client_version", "").Error; err != nil {
		t.Fatal(err)
	}
	if err = d.Model(&db.ClientUpdateSession{}).Where("player_id=?", offline.profile.Id).Update("disconnected_at", 1).Error; err != nil {
		t.Fatal(err)
	}
	status := func(c client, authenticated bool) *model.ClientUpdateStatusResponse {
		t.Helper()
		req := model.ClientUpdateStatusRequest{BuildCode: c.build, ClientVersion: c.version, StartupID: c.startup}
		if authenticated {
			req.PlayerID, req.SessionHex = c.profile.Id, hex.EncodeToString(c.profile.Session)
		}
		out, err := s.ClientUpdateStatus(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	if err = d.Model(&db.ClientUpdateSession{}).Where("player_id=?", unknown.profile.Id).Update("build_code", 0).Error; err != nil {
		t.Fatal(err)
	}
	if st := overview().Stats; st.Unknown != 1 || st.Older != 0 {
		t.Fatalf("legacy unknown counted as older: %+v", st)
	}
	if err = d.Model(&db.ClientUpdateSession{}).Where("player_id=?", unknown.profile.Id).Update("build_code", unknown.build).Error; err != nil {
		t.Fatal(err)
	}
	baseline := draft("1.1.0", "notice", 0, 0)
	action(baseline, "publish", "semantic-baseline-publish")
	if out := status(old, true); out.NoticeEnabled || out.LatestBuild != 0 {
		t.Fatalf("matching old package received notice: %+v", out)
	}
	if out := status(unknown, false); !out.NoticeEnabled || out.LatestBuild <= 2100000000 {
		t.Fatalf("unknown baseline version lost notice: %+v", out)
	}
	notice := draft("1.1.1", "notice", 0, 0)
	noticeRequest := model.AdminClientUpdateActionRequest{ID: notice, Version: overview().Version, Action: "publish", RequestID: "semantic-notice-publish", Reason: "test"}
	published, err := s.AdminClientUpdateAction(ctx, actor, noticeRequest)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := s.AdminClientUpdateAction(ctx, actor, noticeRequest)
	if err != nil || retry.Version != published.Version || retry.Policy.NoticeBatch != published.Policy.NoticeBatch {
		t.Fatalf("semantic publish retry: %+v %v", retry, err)
	}
	if !published.Policy.VersionPolicy || published.Policy.MinLoginVersion != "" || published.Policy.MinRunVersion != "" || published.Policy.MinLoginBuild != 100 || published.Policy.MinRunBuild != 50 || published.Policy.VersionName != "1.1.1" {
		t.Fatalf("notice policy: %+v", published.Policy)
	}
	if st := published.Stats; st.Online != 5 || st.Older != 2 || st.Unknown != 1 {
		t.Fatalf("semantic stats: %+v", st)
	}
	if out := status(old, true); !out.NoticeEnabled || out.Blocked || out.LatestBuild <= 2100000000 {
		t.Fatalf("old package notice: %+v", out)
	}
	if out := status(unknown, false); !out.NoticeEnabled || out.Blocked || out.LatestBuild <= 2100000000 {
		t.Fatalf("unknown notice: %+v", out)
	}
	if out := status(same, true); out.NoticeEnabled {
		t.Fatalf("same version notice: %+v", out)
	}
	if out := status(higher, true); out.NoticeEnabled {
		t.Fatalf("higher version notice: %+v", out)
	}
	ack := model.ClientUpdateAckRequest{BuildCode: old.build, StartupID: old.startup, PlayerID: old.profile.Id, SessionHex: hex.EncodeToString(old.profile.Session), BatchID: published.Policy.NoticeBatch, Kind: "shown"}
	if _, err = s.ClientUpdateAck(ctx, ack); err != nil {
		t.Fatal("old ack:", err)
	}
	ack.ClientVersion = "1.1.1"
	if _, err = s.ClientUpdateAck(ctx, ack); !errors.Is(err, constUtil.ErrClientUpdateIdentity) {
		t.Fatalf("mismatched ack: %v", err)
	}
	if _, err = s.ClientUpdateStatus(ctx, model.ClientUpdateStatusRequest{BuildCode: old.build, ClientVersion: "1.1.1", StartupID: old.startup, PlayerID: old.profile.Id, SessionHex: hex.EncodeToString(old.profile.Session)}); !errors.Is(err, constUtil.ErrClientUpdateIdentity) {
		t.Fatalf("mismatched status: %v", err)
	}
	if err = s.CheckClientLogin(ctx, "hyjs/2026110101/"+old.startup); err != nil {
		t.Fatal("notice blocked old login:", err)
	}
	next := draft("1.1.2", "next_login", 0, 0)
	nextPolicy := action(next, "publish", "semantic-next-publish")
	if nextPolicy.Policy.MinLoginVersion != "1.1.2" || nextPolicy.Policy.MinRunVersion != "" {
		t.Fatalf("next login policy: %+v", nextPolicy.Policy)
	}
	if err = s.CheckClientLogin(ctx, "hyjs/2026110101/"+old.startup); !errors.Is(err, constUtil.ErrClientUpdateRequired) {
		t.Fatalf("old login: %v", err)
	}
	if err = s.CheckClientLogin(ctx, fmt.Sprintf("hyjs/%d/%s", unknown.build, unknown.startup)); !errors.Is(err, constUtil.ErrClientUpdateRequired) {
		t.Fatalf("unknown login: %v", err)
	}
	if err = s.CheckClientLogin(ctx, fmt.Sprintf("hyjs/%d/%s/%s", higher.build, higher.startup, higher.version)); err != nil {
		t.Fatalf("higher login: %v", err)
	}
	if err = CheckClientUpdateSession(ctx, old.profile); err != nil {
		t.Fatalf("online session lost: %v", err)
	}
	if err = s.CheckClientReconnect(ctx, offline.profile); !errors.Is(err, constUtil.ErrClientUpdateRequired) {
		t.Fatalf("offline session resumed: %v", err)
	}
	if err = s.ClientUpdateDisconnected(ctx, old.profile.Id, old.profile.Session); err != nil {
		t.Fatal(err)
	}
	if err = s.CheckClientReconnect(ctx, old.profile); err != nil {
		t.Fatalf("short reconnect: %v", err)
	}
	if err = d.Model(&db.ClientUpdateSession{}).Where("player_id=?", old.profile.Id).Update("disconnected_at", time.Now().UnixMilli()-constUtil.ClientUpdateReconnectMS-1).Error; err != nil {
		t.Fatal(err)
	}
	if err = s.CheckClientReconnect(ctx, old.profile); !errors.Is(err, constUtil.ErrClientUpdateRequired) {
		t.Fatalf("expired reconnect: %v", err)
	}
	withdrawn := action(next, "withdraw", "semantic-withdraw")
	if withdrawn.Policy.NoticeEnabled || withdrawn.Policy.MinLoginVersion != "1.1.2" {
		t.Fatalf("withdraw changed gate: %+v", withdrawn.Policy)
	}
	if out := status(unknown, false); !out.Blocked || out.NoticeEnabled || out.LatestBuild != 0 {
		t.Fatalf("unknown gate: %+v", out)
	}
	start := time.Now().UnixMilli() + 100000
	maintenance := draft("1.10.0", "maintenance", start, start+100000)
	plan := action(maintenance, "publish", "semantic-maintenance-plan")
	if plan.Policy.Maintenance || plan.Policy.MinLoginVersion != "1.1.2" || plan.Policy.MinRunVersion != "" {
		t.Fatalf("plan raised gate early: %+v", plan.Policy)
	}
	if out := status(higher, true); !out.NoticeEnabled {
		t.Fatalf("maintenance preview missing for target version: %+v", out)
	}
	if out := status(client{build: higher.build, version: "1.10.1", startup: higher.startup}, false); !out.NoticeEnabled {
		t.Fatalf("maintenance preview missing for higher version: %+v", out)
	}
	if out := status(old, false); !out.NoticeEnabled || out.LatestBuild <= 2100000000 {
		t.Fatalf("legacy maintenance preview: %+v", out)
	}
	if err = d.Model(&db.ClientUpdatePolicy{}).Where("id=1").Update("min_login_version", "2.0.0").Error; err != nil {
		t.Fatal(err)
	}
	_, err = s.AdminClientUpdateAction(ctx, actor, model.AdminClientUpdateActionRequest{ID: maintenance, Version: overview().Version, Action: "end_maintenance", RequestID: "semantic-end-no-downgrade", Reason: "test"})
	if !errors.Is(err, constUtil.ErrAdminConfigConflict) || overview().Policy.MinLoginVersion != "2.0.0" {
		t.Fatalf("maintenance end lowered gate: %v", err)
	}
	if err = d.Model(&db.ClientUpdatePolicy{}).Where("id=1").Update("min_login_version", "1.1.2").Error; err != nil {
		t.Fatal(err)
	}
	if err = d.Model(&db.ClientUpdatePolicy{}).Where("id=1").Update("maintenance_start_at", time.Now().UnixMilli()-1).Error; err != nil {
		t.Fatal(err)
	}
	if err = s.CheckClientLogin(ctx, fmt.Sprintf("hyjs/%d/%s/%s", higher.build, higher.startup, higher.version)); !errors.Is(err, constUtil.ErrClientUpdateMaintenance) {
		t.Fatalf("maintenance bypass: %v", err)
	}
	ended := action(maintenance, "end_maintenance", "semantic-maintenance-end")
	if ended.Policy.Maintenance || ended.Policy.MinLoginVersion != "1.10.0" || ended.Policy.MinRunVersion != "1.10.0" || ended.Policy.MinLoginBuild != 100 || ended.Policy.MinRunBuild != 50 {
		t.Fatalf("maintenance end: %+v", ended.Policy)
	}
	if err = CheckClientUpdateSession(ctx, higher.profile); !errors.Is(err, constUtil.ErrClientUpdateRequired) {
		t.Fatalf("old epoch resumed: %v", err)
	}
	if err = s.BindClientLogin(ctx, higher.profile, fmt.Sprintf("hyjs/%d/%s/%s", higher.build, higher.startup, higher.version)); err != nil {
		t.Fatalf("new login failed: %v", err)
	}
	if err = CheckClientUpdateSession(ctx, higher.profile); err != nil {
		t.Fatalf("new session blocked: %v", err)
	}
	rollback := draft("1.9.999", "notice", 0, 0)
	before := overview()
	_, err = s.AdminClientUpdateAction(ctx, actor, model.AdminClientUpdateActionRequest{ID: rollback, Version: before.Version, Action: "publish", RequestID: "semantic-reject-rollback", Reason: "test"})
	if !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatalf("rollback accepted: %v", err)
	}
	after := overview()
	if before.Version != after.Version || before.Policy != after.Policy {
		t.Fatalf("rejected publish changed policy: before=%+v after=%+v", before.Policy, after.Policy)
	}
	legacy, err := s.AdminClientUpdateDraft(ctx, actor, model.AdminClientUpdateDraftRequest{Version: after.Version, BuildCode: 2026110105, VersionName: "legacy", Mode: "notice", MinLoginBuild: 100, MinRunBuild: 50, Title: "更新", Content: "更新说明", Reason: "test"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.AdminClientUpdateAction(ctx, actor, model.AdminClientUpdateActionRequest{ID: legacy.Items[0].ID, Version: legacy.Version, Action: "publish", RequestID: "semantic-reject-legacy", Reason: "test"})
	if !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatalf("legacy downgrade accepted: %v", err)
	}
	start = time.Now().UnixMilli() + 100000
	manual := draft("1.10.1", "maintenance", start, start+100000)
	action(manual, "publish", "semantic-manual-plan")
	action(manual, "server_stop", "semantic-manual-stop")
	if err = d.Model(&db.ClientUpdatePolicy{}).Where("id=1").Update("min_run_version", "2.0.0").Error; err != nil {
		t.Fatal(err)
	}
	_, err = s.AdminClientUpdateAction(ctx, actor, model.AdminClientUpdateActionRequest{ID: manual, Version: overview().Version, Action: "server_start", RequestID: "semantic-start-no-downgrade", Reason: "test"})
	if !errors.Is(err, constUtil.ErrAdminConfigConflict) || overview().Policy.MinRunVersion != "2.0.0" || !overview().Policy.Maintenance {
		t.Fatalf("server start lowered gate: %v", err)
	}
	if err = d.Model(&db.ClientUpdatePolicy{}).Where("id=1").Update("min_run_version", "1.10.0").Error; err != nil {
		t.Fatal(err)
	}
	manualEnd := action(manual, "server_start", "semantic-manual-start")
	if manualEnd.Policy.MinLoginVersion != "1.10.1" || manualEnd.Policy.MinRunVersion != "1.10.1" {
		t.Fatalf("server_start bypassed version gates: %+v", manualEnd.Policy)
	}
}
