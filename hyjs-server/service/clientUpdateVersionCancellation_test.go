package service

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

func TestClientUpdateMaintenanceCancellationSQLite(t *testing.T) {
	ctx := context.Background()
	if err := db.InitGame(ctx, filepath.Join(t.TempDir(), "cancel.sqlite")); err != nil {
		t.Fatal(err)
	}
	d := db.GetIns()
	sqlDB, err := d.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { clientUpdateReady.Delete(clientUpdateDBKey(d)); _ = sqlDB.Close() })
	if err = d.AutoMigrate(&db.AdminUser{}, &db.AdminAudit{}); err != nil {
		t.Fatal(err)
	}
	s := &GameService{}
	if err = s.InitializeClientUpdates(ctx); err != nil {
		t.Fatal(err)
	}
	if err = d.Model(&db.ClientUpdatePolicy{}).Where("id=1").Updates(map[string]any{
		"version_policy": true, "version_name": "1.1.0", "min_login_version": "1.0.0",
		"min_run_version": "1.0.0", "min_login_build": 100, "min_run_build": 50, "login_gate_at": int64(12345),
	}).Error; err != nil {
		t.Fatal(err)
	}
	user := db.AdminUser{Username: "cancel-admin", Role: constUtil.AdminRoleAdmin, Enabled: true}
	operator := db.AdminUser{Username: "cancel-operator", Role: constUtil.AdminRoleOperator, Enabled: true}
	if err = d.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	if err = d.Create(&operator).Error; err != nil {
		t.Fatal(err)
	}
	actor := &model.AdminPrincipal{UserID: user.ID, Username: user.Username, Role: user.Role}
	operatorActor := &model.AdminPrincipal{UserID: operator.ID, Username: operator.Username, Role: operator.Role}
	profile := &db.Profile{Uid: "cancel-player", Session: []byte("1234567890123456")}
	if err = d.Create(profile).Error; err != nil {
		t.Fatal(err)
	}
	startup := "1234567890abcdef1234567890abcdef"
	if err = s.BindClientLogin(ctx, profile, "hyjs/2026110101/"+startup); err != nil {
		t.Fatal(err)
	}
	if err = s.ClientUpdateConnected(ctx, profile.Id, profile.Session); err != nil {
		t.Fatal(err)
	}
	closed := 0
	key := p10ClientKey{s.p10ClientOwner(), profile.Id}
	p10ClientSinks.Store(key, &p10ClientSink{session: profile.Session, send: func(proto.Message) error { return nil }, close: func() { closed++ }})
	t.Cleanup(func() { p10ClientSinks.Delete(key) })
	overview := func() *model.AdminClientUpdatesResponse {
		t.Helper()
		out, err := s.AdminClientUpdates(ctx, actor)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	draft := func(version string, start int64) int64 {
		t.Helper()
		out, err := s.AdminClientUpdateDraft(ctx, actor, model.AdminClientUpdateDraftRequest{Version: overview().Version, VersionPolicy: true, VersionName: version, Mode: "maintenance", Title: "维护", Content: "维护计划", Reason: "test", MaintenanceStartAt: start, MaintenanceEndAt: start + 100000})
		if err != nil {
			t.Fatal(err)
		}
		return out.Items[0].ID
	}
	request := func(id int64, action, key string) model.AdminClientUpdateActionRequest {
		return model.AdminClientUpdateActionRequest{ID: id, Version: overview().Version, Action: action, RequestID: key, Reason: "test"}
	}
	action := func(id int64, name, key string) *model.AdminClientUpdatesResponse {
		t.Helper()
		out, err := s.AdminClientUpdateAction(ctx, actor, request(id, name, key))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return out
	}
	readPolicy := func() db.ClientUpdatePolicy {
		t.Helper()
		var p db.ClientUpdatePolicy
		if err := d.First(&p, 1).Error; err != nil {
			t.Fatal(err)
		}
		return p
	}
	readBinding := func() db.ClientUpdateSession {
		t.Helper()
		var binding db.ClientUpdateSession
		if err := d.First(&binding, "player_id=?", profile.Id).Error; err != nil {
			t.Fatal(err)
		}
		return binding
	}
	assertRetained := func(before, after db.ClientUpdatePolicy) {
		t.Helper()
		if after.MinLoginBuild != before.MinLoginBuild || after.MinRunBuild != before.MinRunBuild || after.MinLoginVersion != before.MinLoginVersion || after.MinRunVersion != before.MinRunVersion || after.MaintenanceEpoch != before.MaintenanceEpoch || after.LoginGateAt != before.LoginGateAt || after.NoticeBatch != before.NoticeBatch {
			t.Fatalf("cancellation changed gates or epoch: before=%+v after=%+v", before, after)
		}
	}
	start := time.Now().UnixMilli() + 120000
	id := draft("1.2.0", start)
	action(id, "publish", "cancel-plan-publish")
	action(id, "withdraw", "cancel-plan-withdraw")
	before, binding := readPolicy(), readBinding()
	if before.MaintenanceStartAt != start || before.NoticeEnabled {
		t.Fatalf("withdraw removed plan: %+v", before)
	}
	if _, err = s.AdminClientUpdateAction(ctx, actor, request(id, "end_maintenance", "cancel-plan-end-early")); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatalf("early end accepted: %v", err)
	}
	stale := request(id, "cancel_maintenance", "cancel-plan-stale")
	stale.Version = "-1"
	if _, err = s.AdminClientUpdateAction(ctx, actor, stale); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatalf("stale cancel accepted: %v", err)
	}
	if _, err = s.AdminClientUpdateAction(ctx, operatorActor, request(id, "cancel_maintenance", "cancel-plan-operator")); !errors.Is(err, constUtil.ErrAdminPermission) {
		t.Fatalf("operator cancelled: %v", err)
	}
	cancelRequest := request(id, "cancel_maintenance", "cancel-plan-idempotent")
	cancelled, err := s.AdminClientUpdateAction(ctx, actor, cancelRequest)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := s.AdminClientUpdateAction(ctx, actor, cancelRequest)
	if err != nil || retry.Version != cancelled.Version {
		t.Fatalf("cancel retry: %+v %v", retry, err)
	}
	cancelRequest.Reason = "different"
	if _, err = s.AdminClientUpdateAction(ctx, actor, cancelRequest); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatalf("changed retry accepted: %v", err)
	}
	after := readPolicy()
	assertRetained(before, after)
	if after.Maintenance || after.MaintenanceStartAt != 0 || after.MaintenanceEndAt != 0 || after.NoticeEnabled {
		t.Fatalf("plan remains active: %+v", after)
	}
	var release db.ClientUpdateRelease
	if err = d.First(&release, id).Error; err != nil || release.State != "cancelled" {
		t.Fatalf("cancelled release: %+v %v", release, err)
	}
	if got := readBinding(); !reflect.DeepEqual(binding, got) {
		t.Fatalf("cancel changed session: before=%+v after=%+v", binding, got)
	}
	if closed != 0 {
		t.Fatalf("cancel closed %d connections", closed)
	}
	if err = CheckClientUpdateSession(ctx, profile); err != nil {
		t.Fatalf("cancel blocked session: %v", err)
	}
	var count int64
	if err = d.Model(&db.AdminAudit{}).Where("action=? AND target=?", "client_update.cancel_maintenance", strconv.FormatInt(id, 10)).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("audit count=%d err=%v", count, err)
	}
	if err = d.Model(&db.ClientUpdateAction{}).Where("request_id=?", "cancel-plan-idempotent").Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("idempotency count=%d err=%v", count, err)
	}
	for _, name := range []string{"remind", "start_maintenance", "cancel_maintenance"} {
		if _, err = s.AdminClientUpdateAction(ctx, actor, request(id, name, "cancelled-reject-"+name)); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
			t.Fatalf("cancelled %s accepted: %v", name, err)
		}
	}
	// 通用开服对未来计划执行同一取消动作，之后普通停服/开服不启用被取消的门槛。
	id = draft("1.2.0", time.Now().UnixMilli()+120000)
	action(id, "publish", "cancel-same-version-publish")
	before, binding = readPolicy(), readBinding()
	serverStartRequest := request(0, "server_start", "cancel-server-start")
	serverStart, err := s.AdminClientUpdateAction(ctx, actor, serverStartRequest)
	if err != nil {
		t.Fatal(err)
	}
	serverRetry, err := s.AdminClientUpdateAction(ctx, actor, serverStartRequest)
	if err != nil || serverStart.Version != serverRetry.Version {
		t.Fatalf("server start cancellation retry: %+v %v", serverRetry, err)
	}
	after = readPolicy()
	assertRetained(before, after)
	release = db.ClientUpdateRelease{}
	if err = d.First(&release, id).Error; err != nil || release.State != "cancelled" || after.NoticeEnabled || after.MaintenanceStartAt != 0 || after.MaintenanceEndAt != 0 {
		t.Fatalf("server start cancellation: %+v %+v %v", release, after, err)
	}
	if got := readBinding(); !reflect.DeepEqual(binding, got) || closed != 0 {
		t.Fatalf("server start cancellation changed session: %+v closed=%d", got, closed)
	}
	if err = d.Model(&db.AdminAudit{}).Where("action=? AND target=?", "client_update.cancel_maintenance", strconv.FormatInt(id, 10)).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("server start cancellation audit count=%d err=%v", count, err)
	}
	action(0, "server_stop", "cancel-later-stop")
	manualEnd := action(0, "server_start", "cancel-later-start")
	if manualEnd.Policy.MinLoginVersion != before.MinLoginVersion || manualEnd.Policy.MinRunVersion != before.MinRunVersion {
		t.Fatalf("cancelled plan applied on later start: %+v", manualEnd.Policy)
	}
	// 将事务中的策略读取暂停到开始时间之后，验证锁内时钟拒绝竞态取消。
	for _, test := range []struct{ operation, version string }{{"cancel_maintenance", "1.2.1"}, {"server_start", "1.2.2"}} {
		start = time.Now().UnixMilli() + 1200
		id = draft(test.version, start)
		action(id, "publish", "cancel-boundary-publish-"+test.operation)
		boundaryRequest := request(id, test.operation, "cancel-boundary-race-"+test.operation)
		arrived := make(chan struct{})
		resume := make(chan struct{})
		done := make(chan error, 1)
		var policyReads atomic.Int32
		if err = d.Callback().Query().After("gorm:query").Register("client_update_cancel_boundary_test", func(tx *gorm.DB) {
			if tx.Statement.Table == "client_update_policy" && policyReads.Add(1) == 2 {
				close(arrived)
				<-resume
			}
		}); err != nil {
			t.Fatal(err)
		}
		go func() { _, e := s.AdminClientUpdateAction(ctx, actor, boundaryRequest); done <- e }()
		select {
		case <-arrived:
		case <-time.After(5 * time.Second):
			close(resume)
			<-done
			t.Fatal("cancel transaction did not reach locked policy read")
		}
		if delay := time.Until(time.UnixMilli(start)) + 5*time.Millisecond; delay > 0 {
			<-time.After(delay)
		}
		close(resume)
		err = <-done
		if removeErr := d.Callback().Query().Remove("client_update_cancel_boundary_test"); removeErr != nil {
			t.Fatal(removeErr)
		}
		if !errors.Is(err, constUtil.ErrAdminConfigConflict) {
			t.Fatalf("%s crossed start time: %v", test.operation, err)
		}
		after = readPolicy()
		release = db.ClientUpdateRelease{}
		if err = d.First(&release, id).Error; err != nil || release.State != "published" || after.Maintenance || after.MaintenanceStartAt != start {
			t.Fatalf("boundary cancellation mutated plan: %+v %+v %v", release, after, err)
		}
		if err = s.CheckClientLogin(ctx, "hyjs/2026110101/"+startup); !errors.Is(err, constUtil.ErrClientUpdateMaintenance) {
			t.Fatalf("due maintenance not activated: %v", err)
		}
		if _, err = s.AdminClientUpdateAction(ctx, actor, request(id, "cancel_maintenance", "cancel-after-start-"+test.operation)); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
			t.Fatalf("active maintenance cancelled: %v", err)
		}
		ended := action(id, "end_maintenance", "cancel-boundary-end-"+test.operation)
		if ended.Policy.Maintenance || ended.Policy.MinLoginVersion != test.version || ended.Policy.MinRunVersion != test.version {
			t.Fatalf("maintenance end missed gates: %+v", ended.Policy)
		}
	}
}

func TestClientUpdateCancelBoundary(t *testing.T) {
	for _, test := range []struct {
		name        string
		start, now  int64
		maintenance bool
		want        error
	}{
		{"before", 101, 100, false, nil},
		{"at", 100, 100, false, constUtil.ErrAdminConfigConflict},
		{"after", 99, 100, false, constUtil.ErrAdminConfigConflict},
		{"started", 101, 100, true, constUtil.ErrAdminConfigConflict},
	} {
		t.Run(test.name, func(t *testing.T) {
			p := db.ClientUpdatePolicy{ReleaseID: 1, Mode: "maintenance", Maintenance: test.maintenance, MaintenanceStartAt: test.start, MaintenanceEndAt: 200, NoticeEnabled: true, MinLoginVersion: "1.0.0", MinRunVersion: "1.0.0", LoginGateAt: 12, MaintenanceEpoch: 3}
			r := db.ClientUpdateRelease{ID: 1, Mode: "maintenance", State: "published"}
			err := clientUpdateCancelMaintenance(&p, &r, test.now)
			if !errors.Is(err, test.want) {
				t.Fatalf("got %v want %v", err, test.want)
			}
			if err == nil && (r.State != "cancelled" || p.MaintenanceStartAt != 0 || p.MaintenanceEndAt != 0 || p.NoticeEnabled || p.MinLoginVersion != "1.0.0" || p.MinRunVersion != "1.0.0" || p.LoginGateAt != 12 || p.MaintenanceEpoch != 3) {
				t.Fatalf("bad cancellation: %+v %+v", p, r)
			}
		})
	}
}
