package service

import (
	"context"
	"encoding/hex"
	"errors"
	driver "github.com/go-sql-driver/mysql"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestClientUpdateSQLite(t *testing.T) { clientUpdateBusinessSuite(t, false) }
func TestClientUpdateMySQL(t *testing.T) {
	if os.Getenv("HYJS_CLIENT_UPDATE_MYSQL_DSN") == "" {
		t.Skip("isolated MySQL DSN not supplied")
	}
	clientUpdateBusinessSuite(t, true)
}
func clientUpdateBusinessSuite(t *testing.T, mysql bool) {
	ctx := context.Background()
	if mysql {
		cfg, err := driver.ParseDSN(os.Getenv("HYJS_CLIENT_UPDATE_MYSQL_DSN"))
		if err != nil {
			t.Fatal("invalid isolated MySQL DSN")
		}
		if !strings.HasPrefix(cfg.DBName, "hyjs_test_") {
			t.Fatal("requires hyjs_test_ database")
		}
		if err = db.InitGameMySQL(ctx, db.MySQLConfig{Host: cfg.Addr, User: cfg.User, Password: cfg.Passwd, Database: cfg.DBName}); err != nil {
			t.Fatal(err)
		}
	} else {
		if err := db.InitGame(ctx, filepath.Join(t.TempDir(), "client-update.sqlite")); err != nil {
			t.Fatal(err)
		}
	}
	d := db.GetIns()
	sqlDB, _ := d.DB()
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { clientUpdateReady.Delete(clientUpdateDBKey(d)); sqlDB.Close() })
	s := &GameService{}
	if err := s.InitializeClientUpdates(ctx); err != nil {
		t.Fatal(err)
	}
	// 清理仅限此suite所属的独立表，MySQL已先验证测试库前缀。
	for _, table := range []any{&db.ClientUpdateAction{}, &db.ClientUpdateReceipt{}, &db.ClientUpdateSession{}, &db.ClientUpdateRelease{}} {
		if err := d.Where("1=1").Delete(table).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := d.Model(&db.ClientUpdatePolicy{}).Where("id=1").Select("*").Updates(&db.ClientUpdatePolicy{ID: 1}).Error; err != nil {
		t.Fatal(err)
	}
	if err := d.AutoMigrate(&db.AdminUser{}, &db.AdminAudit{}); err != nil {
		t.Fatal(err)
	}
	user := db.AdminUser{Username: "client-update-test-admin", Role: constUtil.AdminRoleAdmin, Enabled: true}
	if err := d.Where("username=?", user.Username).Delete(&db.AdminUser{}).Error; err != nil {
		t.Fatal(err)
	}
	if err := d.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	actor := &model.AdminPrincipal{UserID: user.ID, Username: user.Username, Role: user.Role}
	profile := &db.Profile{Uid: "client-update-test-profile", Session: []byte("1234567890123456")}
	_ = d.Where("uid=?", profile.Uid).Delete(&db.Profile{})
	if err := d.Create(profile).Error; err != nil {
		t.Fatal(err)
	}
	startup := "1234567890abcdef1234567890abcdef"
	app := "hyjs/100/" + startup
	if err := s.BindClientLogin(ctx, profile, app); err != nil {
		t.Fatal(err)
	}
	s.ClientUpdateConnected(ctx, profile.Id, profile.Session)
	overview := func() *model.AdminClientUpdatesResponse {
		t.Helper()
		out, err := s.AdminClientUpdates(ctx, actor)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	draft := func(mode string, build, login, run, start, end int64) int64 {
		t.Helper()
		o := overview()
		out, err := s.AdminClientUpdateDraft(ctx, actor, model.AdminClientUpdateDraftRequest{Version: o.Version, BuildCode: build, VersionName: "test", Title: "更新", Content: constUtil.ClientUpdateDownloadText, Mode: mode, MinLoginBuild: login, MinRunBuild: run, MaintenanceStartAt: start, MaintenanceEndAt: end, Reason: "isolated test"})
		if err != nil {
			t.Fatal(err)
		}
		return out.Items[0].ID
	}
	action := func(id int64, name, key string) *model.AdminClientUpdatesResponse {
		t.Helper()
		out, err := s.AdminClientUpdateAction(ctx, actor, model.AdminClientUpdateActionRequest{ID: id, Version: overview().Version, RequestID: key, Action: name, Reason: "isolated test"})
		if err != nil {
			t.Fatal(name, err)
		}
		return out
	}
	invalidID := draft("notice", 200, 100, 0, 0, 0)
	if _, err := s.AdminClientUpdateAction(ctx, actor, model.AdminClientUpdateActionRequest{ID: invalidID, Version: overview().Version, RequestID: "invalid-notice-gates-test", Action: "publish", Reason: "isolated test"}); !errors.Is(err, constUtil.ErrAdminInput) {
		t.Fatal("notice raised login gate", err)
	}
	id := draft("next_login", 200, 200, 0, 0, 0)
	if err := s.CheckClientLogin(ctx, app); err != nil {
		t.Fatal("draft became effective", err)
	}
	req := model.AdminClientUpdateActionRequest{ID: id, Version: overview().Version, RequestID: "publish-idempotent-test", Action: "publish", Reason: "isolated test"}
	published, err := s.AdminClientUpdateAction(ctx, actor, req)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := s.AdminClientUpdateAction(ctx, actor, req)
	if err != nil || retry.Policy.NoticeBatch != published.Policy.NoticeBatch || retry.Version != published.Version {
		t.Fatal("publish retry changed policy", err)
	}
	req.Reason = "different"
	if _, err = s.AdminClientUpdateAction(ctx, actor, req); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatal("request reuse accepted", err)
	}
	if err = s.CheckClientLogin(ctx, app); !errors.Is(err, constUtil.ErrClientUpdateRequired) {
		t.Fatal("old login allowed", err)
	}
	if err = s.CheckClientLogin(ctx, "hyjs/200/"+startup); err != nil {
		t.Fatal("new login blocked", err)
	}
	if err = s.CheckClientLogin(ctx, "invalid"); err == nil {
		t.Fatal("unknown login allowed")
	}
	if err = CheckClientUpdateSession(ctx, profile); err != nil {
		t.Fatal("next_login interrupted online session", err)
	}
	ack := model.ClientUpdateAckRequest{BuildCode: 100, StartupID: startup, BatchID: published.Policy.NoticeBatch, Kind: "shown"}
	if _, err = s.ClientUpdateAck(ctx, ack); err != nil {
		t.Fatal(err)
	}
	ack.PlayerID = profile.Id
	ack.SessionHex = hex.EncodeToString(profile.Session)
	if _, err = s.ClientUpdateAck(ctx, ack); err != nil {
		t.Fatal(err)
	}
	var receipt db.ClientUpdateReceipt
	if err = d.First(&receipt, "batch_id=? AND startup_id=? AND kind=?", ack.BatchID, startup, "shown").Error; err != nil || receipt.PlayerID != profile.Id {
		t.Fatal("authenticated receipt not attributed", err)
	}
	key := p10ClientKey{s.p10ClientOwner(), profile.Id}
	pushes := 0
	closed := 0
	p10ClientSinks.Store(key, &p10ClientSink{session: profile.Session, send: func(proto.Message) error { pushes++; return nil }, close: func() { closed++ }})
	t.Cleanup(func() { p10ClientSinks.Delete(key) })
	if st := overview().Stats; st.Online != 1 || st.Shown != 1 {
		t.Fatal("online receipt stats", st)
	}
	forged, err := s.ClientUpdateStatus(ctx, model.ClientUpdateStatusRequest{BuildCode: 200, StartupID: startup, PlayerID: profile.Id, SessionHex: hex.EncodeToString(profile.Session)})
	if err != nil || !forged.Blocked {
		t.Fatal("session build forged", err)
	}
	remind := action(id, "remind", "remind-idempotent-test")
	if remind.Policy.NoticeBatch != published.Policy.NoticeBatch+1 {
		t.Fatal("remind batch unchanged")
	}
	action(id, "withdraw", "withdraw-policy-test")
	again := action(id, "remind", "remind-after-withdraw")
	if !again.Policy.NoticeEnabled {
		t.Fatal("withdrawn release cannot be reminded")
	}
	s.ClientUpdateDisconnected(ctx, profile.Id, profile.Session)
	if err = s.CheckClientReconnect(ctx, profile); err != nil {
		t.Fatal("short reconnect rejected", err)
	}
	if err = d.Model(&db.ClientUpdateSession{}).Where("player_id=?", profile.Id).Update("disconnected_at", time.Now().UnixMilli()-constUtil.ClientUpdateReconnectMS-1).Error; err != nil {
		t.Fatal(err)
	}
	if err = s.CheckClientReconnect(ctx, profile); err == nil {
		t.Fatal("expired reconnect allowed")
	}
	if err = CheckClientUpdateSession(ctx, profile); err == nil {
		t.Fatal("HTTP extended expired qualification")
	}
	// 普通重启沿持久化最后活跃时间保留短宽限，停机时间自然计入。
	s.ClientUpdateConnected(ctx, profile.Id, profile.Session)
	clientUpdateReady.Delete(clientUpdateDBKey(d))
	if err = s.InitializeClientUpdates(ctx); err != nil {
		t.Fatal(err)
	}
	if err = s.CheckClientReconnect(ctx, profile); err != nil {
		t.Fatal("short process restart lost reconnect", err)
	}
	// 发布前已离线的会话不能取得下一次门槛保留资格。
	if err = d.Model(&db.ClientUpdateSession{}).Where("player_id=?", profile.Id).Update("disconnected_at", int64(1)).Error; err != nil {
		t.Fatal(err)
	}
	if err = s.CheckClientReconnect(ctx, profile); err == nil {
		t.Fatal("prepublication offline allowed")
	}
	// 维护发布先保存计划，到期才拦截；预计结束不自动开服。
	now := time.Now().UnixMilli()
	maintenanceID := draft("maintenance", 300, 300, 300, now+100000, now+200000)
	plan := action(maintenanceID, "publish", "maintenance-plan-test")
	if plan.Policy.Maintenance || plan.Policy.MinRunBuild != 0 {
		t.Fatal("plan activated early")
	}
	if err = d.Model(&db.ClientUpdatePolicy{}).Where("id=1").Updates(map[string]any{"maintenance_start_at": now - 100, "maintenance_end_at": now - 1}).Error; err != nil {
		t.Fatal(err)
	}
	if err = s.CheckClientLogin(ctx, "hyjs/300/"+startup); !errors.Is(err, constUtil.ErrClientUpdateMaintenance) {
		t.Fatal("scheduled maintenance bypass", err)
	}
	if closed != 1 {
		t.Fatal("scheduled maintenance did not close connection once", closed)
	}
	clientUpdateReady.Delete(clientUpdateDBKey(d))
	if err = s.InitializeClientUpdates(ctx); err != nil {
		t.Fatal(err)
	}
	if err = s.CheckClientLogin(ctx, "hyjs/300/"+startup); !errors.Is(err, constUtil.ErrClientUpdateMaintenance) {
		t.Fatal("cold maintenance bypass", err)
	}
	ended := action(maintenanceID, "end_maintenance", "maintenance-end-test")
	if ended.Policy.Maintenance || ended.Policy.MinRunBuild != 300 {
		t.Fatal("maintenance end gates wrong")
	}
	if err = CheckClientUpdateSession(ctx, profile); err == nil {
		t.Fatal("old maintenance epoch resumed")
	}
	if err = s.BindClientLogin(ctx, profile, "hyjs/300/"+startup); err != nil {
		t.Fatal(err)
	}
	if err = CheckClientUpdateSession(ctx, profile); err != nil {
		t.Fatal("fresh login cannot resume", err)
	}
	// 手动维护向当前运行服务连接推送原错误提示。
	nextID := draft("maintenance", 400, 400, 400, time.Now().UnixMilli()+100000, time.Now().UnixMilli()+200000)
	action(nextID, "publish", "manual-maintenance-plan")
	action(nextID, "start_maintenance", "manual-maintenance-start")
	if pushes == 0 || closed != 2 {
		t.Fatal("manual maintenance did not push original error")
	}
	action(nextID, "end_maintenance", "manual-maintenance-end")
	// 权限读取数据库实时角色，陈旧admin principal不能继续发布。
	if err = d.Model(&db.AdminUser{}).Where("id=?", user.ID).Update("role", constUtil.AdminRoleOperator).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = s.AdminClientUpdateAction(ctx, actor, model.AdminClientUpdateActionRequest{ID: maintenanceID, Version: overview().Version, RequestID: "operator-rejected-test", Action: "remind", Reason: "test"}); !errors.Is(err, constUtil.ErrAdminPermission) {
		t.Fatal("operator published", err)
	}
}
