package service

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

type p10PushAuthFixture struct {
	ctx     context.Context
	s       *GameService
	d       *gorm.DB
	profile db.Profile
	account db.GameAccount
	session db.ClientUpdateSession
	policy  db.ClientUpdatePolicy
}

func p10PushAuthSetup(t *testing.T) *p10PushAuthFixture {
	t.Helper()
	ctx := context.Background()
	if err := db.InitGame(ctx, filepath.Join(t.TempDir(), "push-auth.sqlite")); err != nil {
		t.Fatal(err)
	}
	d := db.GetIns()
	if err := d.AutoMigrate(&db.GameAccount{}); err != nil {
		t.Fatal(err)
	}
	s := &GameService{}
	if err := s.InitializeClientUpdates(ctx); err != nil {
		t.Fatal(err)
	}
	sqlDB, err := d.DB()
	if err != nil {
		t.Fatal(err)
	}
	key := clientUpdateDBKey(d)
	t.Cleanup(func() { clientUpdateReady.Delete(key); _ = sqlDB.Close() })
	account := db.GameAccount{ID: 771, Username: "push-auth-isolated", PasswordSalt: []byte{1}, PasswordHash: []byte{1}, Enabled: true, AuthRevision: 7, Revision: 1}
	if err := d.Create(&account).Error; err != nil {
		t.Fatal(err)
	}
	playerJSON, gameJSON := `{"playerId":1}`, `{"position":{"scene":"town_main"}}`
	profile := db.Profile{Uid: "push-auth-role", Session: []byte("1234567890abcdef"), AccountID: account.ID, AccountAuthRevision: account.AuthRevision, Player: &playerJSON, Game: &gameJSON}
	if err := d.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	session := db.ClientUpdateSession{PlayerID: profile.Id, SessionHash: clientUpdateSessionHash(profile.Session), BuildCode: 100, LoginAt: now - 200000, LastSeenAt: now, Connected: true}
	if err := d.Create(&session).Error; err != nil {
		t.Fatal(err)
	}
	return &p10PushAuthFixture{ctx: ctx, s: s, d: d, profile: profile, account: account, session: session, policy: db.ClientUpdatePolicy{ID: 1}}
}

func (f *p10PushAuthFixture) reset(t *testing.T) {
	t.Helper()
	if err := f.d.Save(&f.account).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.d.Model(&db.Profile{}).Where("id=?", f.profile.Id).Updates(map[string]any{"session": f.profile.Session, "account_id": f.account.ID, "account_auth_revision": f.account.AuthRevision}).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.d.Model(&db.ClientUpdatePolicy{}).Where("id=1").Select("*").Updates(&f.policy).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.d.Save(&f.session).Error; err != nil {
		t.Fatal(err)
	}
}

func TestP10PushAuthorizationMatchesFullAccountGates(t *testing.T) {
	f := p10PushAuthSetup(t)
	now := time.Now().UnixMilli()
	oldGate := now - constUtil.ClientUpdateReconnectMS - 120000
	update := func(t *testing.T, model any, where string, args []any, values map[string]any) {
		t.Helper()
		if err := f.d.Model(model).Where(where, args...).Updates(values).Error; err != nil {
			t.Fatal(err)
		}
	}
	tests := []struct {
		name    string
		prepare func(*testing.T)
		session []byte
		want    error
		message string
	}{
		{name: "valid"},
		{name: "short session", session: []byte("short"), message: "本地会话无效，请重新登录"},
		{name: "rotated session", prepare: func(t *testing.T) {
			update(t, &db.Profile{}, "id=?", []any{f.profile.Id}, map[string]any{"session": []byte("fedcba0987654321")})
		}, message: "本地会话无效，请重新登录"},
		{name: "disabled account", prepare: func(t *testing.T) {
			update(t, &db.GameAccount{}, "id=?", []any{f.account.ID}, map[string]any{"enabled": false})
		}, message: "账号登录已失效，请重新登录"},
		{name: "revoked revision", prepare: func(t *testing.T) {
			update(t, &db.GameAccount{}, "id=?", []any{f.account.ID}, map[string]any{"auth_revision": f.account.AuthRevision + 1})
		}, message: "账号登录已失效，请重新登录"},
		{name: "missing account", prepare: func(t *testing.T) {
			update(t, &db.Profile{}, "id=?", []any{f.profile.Id}, map[string]any{"account_id": f.account.ID + 999})
		}, message: "账号登录已失效，请重新登录"},
		{name: "legacy profile", prepare: func(t *testing.T) {
			update(t, &db.Profile{}, "id=?", []any{f.profile.Id}, map[string]any{"account_id": 0})
		}},
		{name: "maintenance", prepare: func(t *testing.T) {
			update(t, &db.ClientUpdatePolicy{}, "id=1", nil, map[string]any{"maintenance": true})
		}, want: constUtil.ErrClientUpdateMaintenance},
		{name: "scheduled maintenance", prepare: func(t *testing.T) {
			update(t, &db.ClientUpdatePolicy{}, "id=1", nil, map[string]any{"maintenance_start_at": now - 1000})
		}, want: constUtil.ErrClientUpdateMaintenance},
		{name: "minimum run build", prepare: func(t *testing.T) {
			update(t, &db.ClientUpdatePolicy{}, "id=1", nil, map[string]any{"min_run_build": 101})
		}, want: constUtil.ErrClientUpdateRequired},
		{name: "maintenance epoch", prepare: func(t *testing.T) {
			update(t, &db.ClientUpdatePolicy{}, "id=1", nil, map[string]any{"maintenance_epoch": 1})
		}, want: constUtil.ErrClientUpdateRequired},
		{name: "old build already connected", prepare: func(t *testing.T) {
			update(t, &db.ClientUpdatePolicy{}, "id=1", nil, map[string]any{"min_login_build": 101, "login_gate_at": now - 60000})
		}},
		{name: "old build reconnect in grace", prepare: func(t *testing.T) {
			update(t, &db.ClientUpdatePolicy{}, "id=1", nil, map[string]any{"min_login_build": 101, "login_gate_at": now - 60000})
			update(t, &db.ClientUpdateSession{}, "player_id=?", []any{f.profile.Id}, map[string]any{"connected": false, "disconnected_at": now - 10000})
		}},
		{name: "old build reconnect expired", prepare: func(t *testing.T) {
			update(t, &db.ClientUpdatePolicy{}, "id=1", nil, map[string]any{"min_login_build": 101, "login_gate_at": oldGate})
			update(t, &db.ClientUpdateSession{}, "player_id=?", []any{f.profile.Id}, map[string]any{"connected": false, "disconnected_at": now - constUtil.ClientUpdateReconnectMS - 2000, "login_at": oldGate - 10000})
		}, want: constUtil.ErrClientUpdateRequired},
		{name: "disconnected before login gate", prepare: func(t *testing.T) {
			update(t, &db.ClientUpdatePolicy{}, "id=1", nil, map[string]any{"min_login_build": 101, "login_gate_at": now - 10000})
			update(t, &db.ClientUpdateSession{}, "player_id=?", []any{f.profile.Id}, map[string]any{"connected": false, "disconnected_at": now - 20000})
		}, want: constUtil.ErrClientUpdateRequired},
		{name: "logged in after gate", prepare: func(t *testing.T) {
			update(t, &db.ClientUpdatePolicy{}, "id=1", nil, map[string]any{"min_login_build": 101, "login_gate_at": now - 10000})
			update(t, &db.ClientUpdateSession{}, "player_id=?", []any{f.profile.Id}, map[string]any{"login_at": now - 5000})
		}, want: constUtil.ErrClientUpdateRequired},
		{name: "unbound update session", prepare: func(t *testing.T) {
			update(t, &db.ClientUpdatePolicy{}, "id=1", nil, map[string]any{"min_login_build": 101})
			update(t, &db.ClientUpdateSession{}, "player_id=?", []any{f.profile.Id}, map[string]any{"session_hash": "different"})
		}, want: constUtil.ErrClientUpdateRequired},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f.reset(t)
			if tc.prepare != nil {
				tc.prepare(t)
			}
			session := f.profile.Session
			if tc.session != nil {
				session = tc.session
			}
			full, fullErr := AuthorizeAccount(f.ctx, f.profile.Id, session)
			pushErr := authorizeP10Push(f.ctx, f.profile.Id, session)
			if tc.want != nil {
				if !errors.Is(fullErr, tc.want) || !errors.Is(pushErr, tc.want) {
					t.Fatalf("expected gate %v; full=%v push=%v", tc.want, fullErr, pushErr)
				}
			} else if tc.message != "" {
				if fullErr == nil || pushErr == nil || fullErr.Error() != tc.message || pushErr.Error() != tc.message {
					t.Fatalf("expected authorization %q; full=%v push=%v", tc.message, fullErr, pushErr)
				}
			} else if fullErr != nil || pushErr != nil || full == nil || full.Player == nil || full.Game == nil {
				t.Fatalf("valid session or full profile lost; full=%v push=%v", fullErr, pushErr)
			}
		})
	}
	t.Run("connected activity touch", func(t *testing.T) {
		f.reset(t)
		before := time.Now().UnixMilli() - constUtil.ClientUpdateSeenIntervalMS - 1000
		update(t, &db.ClientUpdateSession{}, "player_id=?", []any{f.profile.Id}, map[string]any{"last_seen_at": before})
		if err := authorizeP10Push(f.ctx, f.profile.Id, f.profile.Session); err != nil {
			t.Fatal(err)
		}
		var current db.ClientUpdateSession
		if err := f.d.First(&current, "player_id=?", f.profile.Id).Error; err != nil || current.LastSeenAt <= before {
			t.Fatalf("push authorization lost connected activity: before=%d after=%d err=%v", before, current.LastSeenAt, err)
		}
	})
	t.Run("missing profile", func(t *testing.T) {
		_, fullErr := AuthorizeAccount(f.ctx, f.profile.Id+10000, f.profile.Session)
		pushErr := authorizeP10Push(f.ctx, f.profile.Id+10000, f.profile.Session)
		if !errors.Is(fullErr, gorm.ErrRecordNotFound) || !errors.Is(pushErr, gorm.ErrRecordNotFound) {
			t.Fatalf("missing profile errors changed: full=%v push=%v", fullErr, pushErr)
		}
	})
}

func TestP10PushAuthorizationProjectsColumnsAndPinsBatch(t *testing.T) {
	f := p10PushAuthSetup(t)
	var profileQueries []string
	callback := f.d.Callback().Query()
	if err := callback.After("gorm:query").Register("p10_push_projection_check", func(tx *gorm.DB) {
		if tx.Statement.Table == "profiles" {
			profileQueries = append(profileQueries, strings.ToLower(tx.Statement.SQL.String()))
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer callback.Remove("p10_push_projection_check")
	var sent []proto.Message
	key := p10ClientKey{f.s.p10ClientOwner(), f.profile.Id}
	sink := &p10ClientSink{session: append([]byte(nil), f.profile.Session...), send: func(m proto.Message) error { sent = append(sent, m); return nil }}
	p10ClientSinks.Store(key, sink)
	t.Cleanup(func() { p10ClientSinks.Delete(key) })
	batch := []proto.Message{&pb.SCPlayerDisappear{PlayerId: []int64{1}}, &pb.SCRefreshScene{}}
	f.s.publishP10PrivateBatchPinned(f.ctx, f.profile.Id, sink, batch)
	if len(sent) != len(batch) || !proto.Equal(sent[0], batch[0]) || !proto.Equal(sent[1], batch[1]) {
		t.Fatalf("authorized ordered batch failed: %v", sent)
	}
	if len(profileQueries) != 1 {
		t.Fatalf("expected one profile read per batch: %v", profileQueries)
	}
	query := profileQueries[0]
	for _, field := range []string{"`id`", "`session`", "`account_id`", "`account_auth_revision`"} {
		if !strings.Contains(query, field) {
			t.Fatalf("missing identity column %s in %s", field, query)
		}
	}
	for _, extra := range []string{"`player`", "`game`", "select *", " join "} {
		if strings.Contains(query, extra) {
			t.Fatalf("unneeded profile data or join in %s", query)
		}
	}
	t.Logf("push authorization profile read uses four identity columns, 0 JOIN")
	replacement := &p10ClientSink{session: append([]byte(nil), f.profile.Session...), send: func(m proto.Message) error { sent = append(sent, m); return nil }}
	p10ClientSinks.Store(key, replacement)
	f.s.publishP10PrivateBatchPinned(f.ctx, f.profile.Id, sink, batch)
	if len(sent) != len(batch) || len(profileQueries) != 1 {
		t.Fatal("replaced socket inherited the pinned batch")
	}
	f.s.publishP10PrivateBatchPinned(f.ctx, f.profile.Id, replacement, batch[:1])
	if len(sent) != len(batch)+1 || len(profileQueries) != 2 {
		t.Fatal("replacement socket did not receive its own authenticated batch")
	}
}

func p10WaitRetired(t *testing.T, key p10ClientKey, sink *p10ClientSink) {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		if current, ok := p10ClientSinks.Load(key); !ok || current != sink {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("connection cleanup did not complete")
}

func TestP10PushFailureRetiresAndDisconnects(t *testing.T) {
	for _, kind := range []string{"authorization", "send"} {
		t.Run(kind, func(t *testing.T) {
			f := p10PushAuthSetup(t)
			var closed, sent atomic.Int32
			sink := &p10ClientSink{session: append([]byte(nil), f.profile.Session...), close: func() { closed.Add(1) }}
			sink.send = func(proto.Message) error { sent.Add(1); return errors.New("test send failure") }
			key := p10ClientKey{f.s.p10ClientOwner(), f.profile.Id}
			p10ClientSinks.Store(key, sink)
			t.Cleanup(func() { p10ClientSinks.CompareAndDelete(key, sink) })
			if kind == "authorization" {
				if err := f.d.Model(&db.GameAccount{}).Where("id=?", f.account.ID).Update("enabled", false).Error; err != nil {
					t.Fatal(err)
				}
			}
			f.s.publishP10PrivateBatchPinned(f.ctx, f.profile.Id, sink, []proto.Message{&pb.SCRefreshScene{}, &pb.SCRefreshScene{}})
			if sink.disconnectedAt.Load() == 0 || closed.Load() != 1 {
				t.Fatal("failed socket was not immediately marked and closed")
			}
			if kind == "authorization" && sent.Load() != 0 || kind == "send" && sent.Load() != 1 {
				t.Fatalf("unexpected failed batch sends: %d", sent.Load())
			}
			if _, ok := p10LiveClient(key); ok {
				t.Fatal("failed socket remained live")
			}
			p10WaitRetired(t, key, sink)
			var state db.ClientUpdateSession
			if err := f.d.First(&state, "player_id=?", f.profile.Id).Error; err != nil {
				t.Fatal(err)
			}
			if state.Connected || state.DisconnectedAt != sink.disconnectedAt.Load() {
				t.Fatalf("failed push left stale connected state: %+v", state)
			}
		})
	}
}

func TestP10PushFailureReconnectDoesNotRetireReplacement(t *testing.T) {
	for _, rotate := range []bool{false, true} {
		name := "same_session"
		if rotate {
			name = "new_session"
		}
		t.Run(name, func(t *testing.T) {
			f := p10PushAuthSetup(t)
			key := p10ClientKey{f.s.p10ClientOwner(), f.profile.Id}
			var oldClosed, newClosed atomic.Int32
			old := &p10ClientSink{session: append([]byte(nil), f.profile.Session...), close: func() { oldClosed.Add(1) }, send: func(proto.Message) error { return errors.New("test failure") }}
			p10ClientSinks.Store(key, old)
			t.Cleanup(func() { p10ClientSinks.Delete(key) })
			p10ClientLifecycle.Lock()
			f.s.publishP10PrivateBatchPinned(f.ctx, f.profile.Id, old, []proto.Message{&pb.SCRefreshScene{}})
			if old.disconnectedAt.Load() == 0 || oldClosed.Load() != 1 {
				p10ClientLifecycle.Unlock()
				t.Fatal("push failure did not immediately close old socket")
			}
			newSession := append([]byte(nil), old.session...)
			if rotate {
				newSession[0] ^= 0x80
				if err := f.d.Model(&db.Profile{}).Where("id=?", f.profile.Id).Update("session", newSession).Error; err != nil {
					p10ClientLifecycle.Unlock()
					t.Fatal(err)
				}
				if err := f.d.Model(&db.ClientUpdateSession{}).Where("player_id=?", f.profile.Id).Updates(map[string]any{"session_hash": clientUpdateSessionHash(newSession), "connected": false}).Error; err != nil {
					p10ClientLifecycle.Unlock()
					t.Fatal(err)
				}
			}
			registered := make(chan func(), 1)
			go func() {
				registered <- f.s.RegisterP10Client(f.ctx, f.profile.Id, newSession, func(proto.Message) error { return nil }, func() { newClosed.Add(1) })
			}()
			p10ClientLifecycle.Unlock()
			var unregister func()
			select {
			case unregister = <-registered:
			case <-time.After(4 * time.Second):
				t.Fatal("replacement registration deadlocked")
			}
			defer unregister()
			current, ok := p10LiveClient(key)
			if !ok || current == old || !bytes.Equal(current.(*p10ClientSink).session, newSession) || newClosed.Load() != 0 {
				t.Fatal("old failure affected replacement socket")
			}
			var state db.ClientUpdateSession
			if err := f.d.First(&state, "player_id=?", f.profile.Id).Error; err != nil || !state.Connected || state.SessionHash != clientUpdateSessionHash(newSession) {
				t.Fatalf("replacement connected flag lost: %+v, %v", state, err)
			}
			// A delayed old cleanup must not touch the now-live replacement.
			p10ClientLifecycle.Lock()
			if err := f.s.p10CleanupClient(f.ctx, key, old); err != nil {
				p10ClientLifecycle.Unlock()
				t.Fatal(err)
			}
			p10ClientLifecycle.Unlock()
			if err := f.d.First(&state, "player_id=?", f.profile.Id).Error; err != nil || !state.Connected {
				t.Fatalf("old cleanup cleared new connection: %+v, %v", state, err)
			}
		})
	}
}

func TestP10ReconnectRetriesFailedOldCleanup(t *testing.T) {
	f := p10PushAuthSetup(t)
	key := p10ClientKey{f.s.p10ClientOwner(), f.profile.Id}
	var oldClosed, newClosed atomic.Int32
	old := &p10ClientSink{session: append([]byte(nil), f.profile.Session...), send: func(proto.Message) error { return nil }, close: func() { oldClosed.Add(1) }}
	p10ClientSinks.Store(key, old)
	t.Cleanup(func() { p10ClientSinks.Delete(key) })
	callback := f.d.Callback().Update()
	if err := callback.Before("gorm:update").Register("p10_test_fail_disconnect", func(tx *gorm.DB) {
		if tx.Statement.Table == "client_update_sessions" {
			tx.AddError(errors.New("injected disconnect update failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	f.s.RegisterP10Client(f.ctx, f.profile.Id, f.profile.Session, func(proto.Message) error { return nil }, func() { newClosed.Add(1) })
	if oldClosed.Load() != 1 || newClosed.Load() != 1 {
		t.Fatal("failed cleanup did not close both old and rejected new sockets")
	}
	if current, ok := p10ClientSinks.Load(key); !ok || current != old || old.disconnectedAt.Load() == 0 {
		t.Fatal("failed cleanup was not retained for retry")
	}
	if _, ok := p10LiveClient(key); ok {
		t.Fatal("failed cleanup was advertised as live")
	}
	if err := callback.Remove("p10_test_fail_disconnect"); err != nil {
		t.Fatal(err)
	}
	unregister := f.s.RegisterP10Client(f.ctx, f.profile.Id, f.profile.Session, func(proto.Message) error { return nil })
	defer unregister()
	if current, ok := p10LiveClient(key); !ok || current == old {
		t.Fatal("retry did not install new connection")
	}
	var state db.ClientUpdateSession
	if err := f.d.First(&state, "player_id=?", f.profile.Id).Error; err != nil || !state.Connected {
		t.Fatalf("retry did not restore connected flag: %+v, %v", state, err)
	}
}

func TestP10OldDisconnectCannotClearLivePresence(t *testing.T) {
	f := p10PushAuthSetup(t)
	if err := f.d.AutoMigrate(&db.P10ScenePresence{}, &db.P10ScenePolicy{}); err != nil {
		t.Fatal(err)
	}
	if err := f.d.Create(&db.P10ScenePolicy{ID: constUtil.P10ScenePolicyID, Enabled: true, LineCount: 1, MaxPlayers: 20, Source: "test"}).Error; err != nil {
		t.Fatal(err)
	}
	presence := db.P10ScenePresence{PlayerID: f.profile.Id, Scene: "town_main", Line: 1, SessionHash: p6Digest(f.profile.Session), Frame: []byte{}}
	if err := f.d.Create(&presence).Error; err != nil {
		t.Fatal(err)
	}
	key := p10ClientKey{f.s.p10ClientOwner(), f.profile.Id}
	replacement := &p10ClientSink{session: append([]byte(nil), f.profile.Session...), send: func(proto.Message) error { return nil }}
	p10ClientSinks.Store(key, replacement)
	t.Cleanup(func() { p10ClientSinks.CompareAndDelete(key, replacement) })
	if err := f.s.p10DisconnectScene(f.ctx, f.profile.Id, f.profile.Session, time.Now().UnixMilli()); err != nil {
		t.Fatal(err)
	}
	if err := f.d.First(&presence, "player_id=?", f.profile.Id).Error; err != nil || presence.SessionHash != p6Digest(f.profile.Session) {
		t.Fatalf("old disconnect cleared live presence: %+v, %v", presence, err)
	}
}

func TestGGOExtractionDisconnectOriginalTimeAndSession(t *testing.T) {
	for _, tc := range []struct {
		name                                          string
		readyOffset, disconnectOffset, acceptedOffset int64
		rotate                                        bool
		want                                          string
	}{
		{name: "before deadline", readyOffset: -1000, disconnectOffset: -2000, acceptedOffset: -5000, want: "cancelled"},
		{name: "after deadline", readyOffset: -2000, disconnectOffset: -1000, acceptedOffset: -5000, want: "completed"},
		{name: "rotated session after deadline", readyOffset: -2000, disconnectOffset: -1000, acceptedOffset: -5000, rotate: true, want: "cancelled"},
		{name: "newer action", readyOffset: 1000, disconnectOffset: -2000, acceptedOffset: -1000, want: "pending"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HYJS_P1_MYSQL_DATABASE", "")
			f := ggoTradeSetup(t)
			id, session := f.ids[0], f.sessions[0]
			base := time.Now().UnixMilli()
			accepted, ready, disconnected := base+tc.acceptedOffset, base+tc.readyOffset, base+tc.disconnectOffset
			requestID := "00000000-0000-4000-8000-000000000019"
			safe := &model.AdminFieldGGOPosition{X: ggoFloat64(-7), Y: ggoFloat64(0), Z: ggoFloat64(-6)}
			if err := updateProfileWithTx(f.ctx, id, func(tx *gorm.DB, player *pb.Player, game *model.GameState) error {
				game.GGOField.Extraction = &model.GGOFieldExtraction{RequestID: requestID, PointID: "exit", Line: 1, AcceptedAt: accepted, ReadyAt: ready, Origin: safe, SafeReturn: safe}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			start := model.GGOFieldExtractionStartResponse{RequestID: requestID, PointID: "exit", AcceptedAtMS: accepted, ReadyAtMS: ready}
			if err := ggoSaveReceipt(db.GetIns(), id, requestID, constUtil.GGOFieldOpExtractStart, ggoExtractionStartHash("exit"), f.config.Runtime.EconomyVersion, nil, [6]int64{}, 0, accepted, start); err != nil {
				t.Fatal(err)
			}
			if err := db.GetIns().Model(&db.GGOFieldReceipt{}).Where("player_id=? AND request_id=?", id, requestID).Updates(map[string]any{"active": true, "ready_at": ready}).Error; err != nil {
				t.Fatal(err)
			}
			key := p10ClientKey{f.s.p10ClientOwner(), id}
			value, ok := p10ClientSinks.Load(key)
			if !ok {
				t.Fatal("fixture socket missing")
			}
			value.(*p10ClientSink).disconnectedAt.Store(disconnected)
			if tc.rotate {
				rotated := append([]byte(nil), session...)
				rotated[0] ^= 0x80
				if err := db.GetIns().Model(&db.Profile{}).Where("id=?", id).Update("session", rotated).Error; err != nil {
					t.Fatal(err)
				}
			}
			if tc.name == "before deadline" {
				if err := f.s.ggoFinishExtraction(f.ctx, id, requestID); err == nil {
					t.Fatal("deadline worker processed a disconnect awaiting cleanup")
				}
				callback := db.GetIns().Callback().Create()
				if err := callback.Before("gorm:create").Register("p10_test_fail_extraction_receipt", func(tx *gorm.DB) {
					if tx.Statement.Table == "ggo_field_receipts" {
						tx.AddError(errors.New("injected extraction receipt failure"))
					}
				}); err != nil {
					t.Fatal(err)
				}
				if _, err := f.s.ggoExtractionDisconnect(f.ctx, id, session, disconnected); err == nil {
					t.Fatal("receipt error was swallowed")
				}
				if err := callback.Remove("p10_test_fail_extraction_receipt"); err != nil {
					t.Fatal(err)
				}
				var active db.GGOFieldReceipt
				if err := db.GetIns().First(&active, "player_id=? AND request_id=?", id, requestID).Error; err != nil || !active.Active {
					t.Fatalf("failed cleanup did not roll back receipt: %+v, %v", active, err)
				}
			}
			completed, err := f.s.ggoExtractionDisconnect(f.ctx, id, session, disconnected)
			if err != nil {
				t.Fatal(err)
			}
			if completed != (tc.want == "completed") {
				t.Fatalf("wrong completion: %v", completed)
			}
			var profile db.Profile
			if err := db.GetIns().First(&profile, id).Error; err != nil {
				t.Fatal(err)
			}
			_, game, err := decodeProfile(&profile)
			if err != nil {
				t.Fatal(err)
			}
			if game.GGOField.Chips != [6]int64{10} {
				t.Fatalf("cleanup changed chips: %v", game.GGOField.Chips)
			}
			if (game.GGOField.Extraction != nil) != (tc.want == "pending") {
				t.Fatal("wrong extraction state")
			}
			for _, result := range []string{"completed", "cancelled"} {
				operation := constUtil.GGOFieldOpExtractCancel
				if result == "completed" {
					operation = constUtil.GGOFieldOpExtractComplete
				}
				var count int64
				if err := db.GetIns().Model(&db.GGOFieldReceipt{}).Where("player_id=? AND operation=? AND business_key=?", id, operation, requestID).Count(&count).Error; err != nil {
					t.Fatal(err)
				}
				want := int64(0)
				if tc.want == result {
					want = 1
				}
				if count != want {
					t.Fatalf("%s receipt count=%d want=%d", result, count, want)
				}
			}
			if _, err := f.s.ggoExtractionDisconnect(f.ctx, id, session, disconnected); err != nil {
				t.Fatalf("retry failed: %v", err)
			}
		})
	}
}

func TestGGONoticeSendFailureRetiresConnection(t *testing.T) {
	f := p10PushAuthSetup(t)
	if err := f.d.AutoMigrate(&db.P10ScenePresence{}); err != nil {
		t.Fatal(err)
	}
	presence := db.P10ScenePresence{PlayerID: f.profile.Id, Scene: constUtil.GGOFieldScene, Line: 1, SessionHash: p6Digest(f.profile.Session), Frame: []byte{}}
	if err := f.d.Create(&presence).Error; err != nil {
		t.Fatal(err)
	}
	var closed, sent atomic.Int32
	sink := &p10ClientSink{session: append([]byte(nil), f.profile.Session...), close: func() { closed.Add(1) }, send: func(proto.Message) error { sent.Add(1); return errors.New("notice send failed") }}
	key := p10ClientKey{f.s.p10ClientOwner(), f.profile.Id}
	p10ClientSinks.Store(key, sink)
	t.Cleanup(func() { p10ClientSinks.CompareAndDelete(key, sink) })
	if err := f.s.ggoBroadcastFieldChange(f.ctx, 1); err != nil {
		t.Fatal(err)
	}
	if sent.Load() != 1 || closed.Load() != 1 || sink.disconnectedAt.Load() == 0 {
		t.Fatalf("notice failure did not close socket: sent=%d closed=%d", sent.Load(), closed.Load())
	}
	p10WaitRetired(t, key, sink)
	var state db.ClientUpdateSession
	if err := f.d.First(&state, "player_id=?", f.profile.Id).Error; err != nil || state.Connected {
		t.Fatalf("notice left connected flag: %+v, %v", state, err)
	}
}

func TestGGODisconnectedSinkRejectedByActionsAndDirectory(t *testing.T) {
	t.Setenv("HYJS_P1_MYSQL_DATABASE", "")
	f := ggoTradeSetup(t)
	var actors [2]*ggoFieldActor
	for i, id := range f.ids {
		var row db.Profile
		if err := db.GetIns().First(&row, id).Error; err != nil {
			t.Fatal(err)
		}
		player, game, err := decodeProfile(&row)
		if err != nil {
			t.Fatal(err)
		}
		runtime := f.s.withRuntimeTables(f.s.Tables)
		runtime.GGOFieldConfig = f.config
		actors[i] = &ggoFieldActor{Service: runtime, Row: &row, Player: player, Game: game, Now: time.Now().UnixMilli()}
	}
	pair := &ggoTradePair{Actors: actors, Now: time.Now().UnixMilli()}
	live, err := ggoTradeLive(db.GetIns(), pair)
	if err != nil || !live {
		t.Fatalf("fixture trade not live: %v, %v", live, err)
	}
	key := p10ClientKey{f.s.p10ClientOwner(), f.ids[0]}
	value, ok := p10ClientSinks.Load(key)
	if !ok {
		t.Fatal("fixture socket missing")
	}
	value.(*p10ClientSink).disconnectedAt.Store(time.Now().UnixMilli())
	live, err = ggoTradeLive(db.GetIns(), pair)
	if err != nil || live {
		t.Fatalf("disconnected player remained trade eligible: %v, %v", live, err)
	}
	called := false
	err = f.s.ggoWithProfile(f.ctx, f.auth(0), f.sessions[0], true, true, func(*gorm.DB, *ggoFieldActor) error { called = true; return nil })
	if err == nil || called {
		t.Fatal("disconnected socket authorized GGO actor action")
	}
	var row db.P10ScenePresence
	if err := db.GetIns().First(&row, "player_id=?", f.ids[0]).Error; err != nil {
		t.Fatal(err)
	}
	peers, err := f.s.p10SceneLiveRows(db.GetIns(), []db.P10ScenePresence{row})
	if err != nil || len(peers) != 0 {
		t.Fatalf("disconnected player remained in live directory: %v, %v", peers, err)
	}
}

func TestGGOReconnectSerializesOriginalExtractionCleanup(t *testing.T) {
	for _, tc := range []struct {
		name                            string
		readyOffset, disconnectedOffset int64
		operation                       string
	}{
		{name: "before deadline", readyOffset: -1000, disconnectedOffset: -2000, operation: constUtil.GGOFieldOpExtractCancel},
		{name: "after deadline", readyOffset: -2000, disconnectedOffset: -1000, operation: constUtil.GGOFieldOpExtractComplete},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HYJS_P1_MYSQL_DATABASE", "")
			f := ggoTradeSetup(t)
			if err := f.s.InitializeP10Scene(f.ctx); err != nil {
				t.Fatal(err)
			}
			if err := db.GetIns().Model(&db.P10ScenePolicy{}).Where("id=?", constUtil.P10ScenePolicyID).Update("enabled", true).Error; err != nil {
				t.Fatal(err)
			}
			id, session := f.ids[0], f.sessions[0]
			base := time.Now().UnixMilli()
			accepted, ready, disconnected := base-5000, base+tc.readyOffset, base+tc.disconnectedOffset
			requestID := "00000000-0000-4000-8000-000000000029"
			safe := &model.AdminFieldGGOPosition{X: ggoFloat64(-7), Y: ggoFloat64(0), Z: ggoFloat64(-6)}
			if err := updateProfileWithTx(f.ctx, id, func(tx *gorm.DB, player *pb.Player, game *model.GameState) error {
				game.GGOField.Extraction = &model.GGOFieldExtraction{RequestID: requestID, PointID: "exit", Line: 1, AcceptedAt: accepted, ReadyAt: ready, Origin: safe, SafeReturn: safe}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			start := model.GGOFieldExtractionStartResponse{RequestID: requestID, PointID: "exit", AcceptedAtMS: accepted, ReadyAtMS: ready}
			if err := ggoSaveReceipt(db.GetIns(), id, requestID, constUtil.GGOFieldOpExtractStart, ggoExtractionStartHash("exit"), f.config.Runtime.EconomyVersion, nil, [6]int64{}, 0, accepted, start); err != nil {
				t.Fatal(err)
			}
			if err := db.GetIns().Model(&db.GGOFieldReceipt{}).Where("player_id=? AND request_id=?", id, requestID).Updates(map[string]any{"active": true, "ready_at": ready}).Error; err != nil {
				t.Fatal(err)
			}
			key := p10ClientKey{f.s.p10ClientOwner(), id}
			old, ok := p10ClientSinks.Load(key)
			if !ok {
				t.Fatal("fixture old connection missing")
			}
			old.(*p10ClientSink).disconnectedAt.Store(disconnected)
			var closed atomic.Int32
			unregister := f.s.RegisterP10Client(f.ctx, id, session, func(proto.Message) error { return nil }, func() { closed.Add(1) })
			defer func() { unregister(); f.s.ggoCancelOfflineTimer(id); f.s.ggoStopNoticeTimers() }()
			if value, ok := p10LiveClient(key); !ok || value == old || closed.Load() != 0 {
				t.Fatal("replacement was not installed after old cleanup")
			}
			var receipt db.GGOFieldReceipt
			if err := db.GetIns().First(&receipt, "player_id=? AND operation=? AND business_key=?", id, tc.operation, requestID).Error; err != nil {
				t.Fatalf("old action was not resolved before replacement: %v", err)
			}
			var profile db.Profile
			if err := db.GetIns().First(&profile, id).Error; err != nil {
				t.Fatal(err)
			}
			_, game, err := decodeProfile(&profile)
			if err != nil || game.GGOField.Extraction != nil {
				t.Fatalf("old extraction remained active: %v", err)
			}
			if game.GGOField.Chips != [6]int64{10} {
				t.Fatal("reconnect altered chips")
			}
			var presence db.P10ScenePresence
			if err := db.GetIns().First(&presence, "player_id=?", id).Error; err != nil {
				t.Fatal(err)
			}
			if tc.operation == constUtil.GGOFieldOpExtractCancel && presence.SessionHash != p6Digest(session) {
				t.Fatal("replacement scene presence was not restored")
			}
			if tc.operation == constUtil.GGOFieldOpExtractComplete && (!game.PositionLoading || presence.SessionHash != "") {
				t.Fatal("completed extraction did not preserve safe return")
			}
		})
	}
}
