package service

import (
	"errors"
	"testing"
	"time"

	"hyjs-server/constUtil"
	"hyjs-server/db"
)

// Push authorization must apply the same version gates as ordinary requests.
func TestP10PushAuthorizationSemanticVersion(t *testing.T) {
	f := p10PushAuthSetup(t)
	now := time.Now().UnixMilli()
	tests := []struct {
		name         string
		version      string
		build        int64
		minLogin     string
		minRun       string
		connected    bool
		disconnected int64
		want         error
	}{
		{name: "numeric version greater than gate", version: "1.10.0", minRun: "1.9.999", connected: true},
		{name: "runtime gate rejects old version", version: "1.1.0", minRun: "1.1.1", connected: true, want: constUtil.ErrClientUpdateRequired},
		{name: "unknown runtime version cannot bypass gate", minRun: "1.1.1", connected: true, want: constUtil.ErrClientUpdateRequired},
		{name: "installed 1.1.0 maps from build", build: 2026110101, minRun: "1.1.0", connected: true},
		{name: "older online session survives login gate", version: "1.1.0", minLogin: "1.1.1", connected: true},
		{name: "older recent reconnect survives login gate", version: "1.1.0", minLogin: "1.1.1", disconnected: now - 1000},
		{name: "older disconnected before login gate rejected", version: "1.1.0", minLogin: "1.1.1", disconnected: now - 60000, want: constUtil.ErrClientUpdateRequired},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f.reset(t)
			if err := f.d.Model(&db.ClientUpdatePolicy{}).Where("id=1").Updates(map[string]any{
				"min_login_version": tc.minLogin, "min_run_version": tc.minRun, "login_gate_at": now - 10000,
			}).Error; err != nil {
				t.Fatal(err)
			}
			if err := f.d.Model(&db.ClientUpdateSession{}).Where("player_id=?", f.profile.Id).Updates(map[string]any{
				"client_version": tc.version, "build_code": tc.build, "connected": tc.connected, "disconnected_at": tc.disconnected,
			}).Error; err != nil {
				t.Fatal(err)
			}
			_, fullErr := AuthorizeAccount(f.ctx, f.profile.Id, f.profile.Session)
			pushErr := authorizeP10Push(f.ctx, f.profile.Id, f.profile.Session)
			if tc.want == nil {
				if fullErr != nil || pushErr != nil {
					t.Fatalf("valid version rejected: full=%v push=%v", fullErr, pushErr)
				}
			} else if !errors.Is(fullErr, tc.want) || !errors.Is(pushErr, tc.want) {
				t.Fatalf("expected %v, got full=%v push=%v", tc.want, fullErr, pushErr)
			}
		})
	}
}
