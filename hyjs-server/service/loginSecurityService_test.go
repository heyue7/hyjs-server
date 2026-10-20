package service

import (
	"bytes"
	"context"
	"hyjs-server/db"
	"path/filepath"
	"testing"
	"time"
)

func TestLoginSecuritySharedIPBurstAndResourceBounds(t *testing.T) {
	loginSources.Lock()
	loginSources.Sources = make(map[string]loginBucket)
	loginSources.Global = loginBucket{}
	loginSources.Sweep = time.Time{}
	loginSources.Unlock()
	now := time.Now().Truncate(time.Millisecond)
	for i := 0; i < 100; i++ {
		if !LoginIngressAllowed("198.51.100.44", now) {
			t.Fatalf("normal shared-IP launch rejected at %d", i)
		}
	}
	for i := 100; i < 200; i++ {
		if !LoginIngressAllowed("198.51.100.44", now) {
			t.Fatal("burst ended early")
		}
	}
	if LoginIngressAllowed("198.51.100.44", now) {
		t.Fatal("unbounded burst")
	}
	if !LoginIngressAllowed("198.51.100.44", now.Add(time.Second)) {
		t.Fatal("bucket did not refill")
	}
}

func TestLoginSecurityPersistentShortBackoff(t *testing.T) {
	ctx := context.Background()
	if err := db.InitGame(ctx, filepath.Join(t.TempDir(), "auth.db")); err != nil {
		t.Fatal(err)
	}
	d, _ := db.GetIns().DB()
	t.Cleanup(func() { d.Close() })
	if err := InitializeLoginSecurity(ctx); err != nil {
		t.Fatal(err)
	}
	row := db.Profile{Uid: "existing-account", PasswordSalt: bytes.Repeat([]byte{1}, 16), PasswordHash: bytes.Repeat([]byte{2}, 32), Session: bytes.Repeat([]byte{3}, 16)}
	if err := db.GetIns().Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().Truncate(time.Millisecond)
	for i := 1; i <= 10; i++ {
		delay, err := RecordGameLoginFailure(ctx, row.Uid, now)
		if err != nil {
			t.Fatal(err)
		}
		want := LoginFailureDelay(int64(i))
		if delay != want {
			t.Fatal("backoff", i, delay, want)
		}
		if delay > 0 {
			again, err := RecordGameLoginFailure(ctx, row.Uid, now)
			if err != nil || again != delay {
				t.Fatal("retry extended delay", again, err)
			}
			now = now.Add(delay + time.Millisecond)
		}
	}
	var persisted db.GameLoginThrottle
	if err := db.GetIns().First(&persisted).Error; err != nil || persisted.Failures != 10 {
		t.Fatal("not persisted", err)
	}
	if _, err := RecordGameLoginFailure(ctx, "unknown-account", now); err != nil {
		t.Fatal(err)
	}
	var count int64
	db.GetIns().Model(&db.GameLoginThrottle{}).Count(&count)
	if count != 1 {
		t.Fatal("unknown names retained")
	}
	if err := ClearGameLoginFailures(ctx, row.Uid); err != nil {
		t.Fatal(err)
	}
	if delay, err := GameLoginRetryAfter(ctx, row.Uid, now); err != nil || delay != 0 {
		t.Fatal("success did not reset", delay, err)
	}
}
