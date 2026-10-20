package service

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

func adminAuthPerformanceFixture(t testing.TB) (context.Context, db.AdminUser, string) {
	t.Helper()
	ctx := context.Background()
	if err := db.InitGame(ctx, filepath.Join(t.TempDir(), "auth-performance.sqlite")); err != nil {
		t.Fatal(err)
	}
	d := db.GetIns()
	connection, err := d.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { connection.Close() })
	if err := d.AutoMigrate(&db.AdminUser{}, &db.AdminSession{}); err != nil {
		t.Fatal(err)
	}
	user := db.AdminUser{Username: "performance-reader", PasswordHash: "not-used-by-session-auth", Role: constUtil.AdminRoleViewer, Enabled: true}
	if err := d.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	token := strings.Repeat("a", 64)
	if err := d.Create(&db.AdminSession{TokenHash: adminDigest(token), UserID: user.ID, CSRFHash: adminDigest(strings.Repeat("b", 64)), ExpiresAtUnix: time.Now().Unix() + 3600}).Error; err != nil {
		t.Fatal(err)
	}
	return ctx, user, token
}

// The previous two-query path remains test-only as an equivalent-input baseline.
func adminAuthenticateTwoQueries(ctx context.Context, token string) (*model.AdminPrincipal, error) {
	if len(token) != 64 {
		return nil, constUtil.ErrAdminSession
	}
	var session db.AdminSession
	d := db.GetIns().WithContext(ctx)
	if err := d.Where("token_hash = ? AND expires_at_unix > ?", adminDigest(token), time.Now().Unix()).Take(&session).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, constUtil.ErrAdminSession
		}
		return nil, err
	}
	var user db.AdminUser
	if err := d.First(&user, session.UserID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, constUtil.ErrAdminSession
		}
		return nil, err
	}
	if !user.Enabled || !AdminRoleValid(user.Role) {
		return nil, constUtil.ErrAdminSession
	}
	return &model.AdminPrincipal{UserID: user.ID, Username: user.Username, Role: user.Role, CSRFHash: session.CSRFHash}, nil
}

func TestAdminAuthenticateSingleQueryAndImmediateRevocation(t *testing.T) {
	ctx, user, token := adminAuthPerformanceFixture(t)
	d := db.GetIns()
	var statements []string
	callback := "admin_auth_performance_queries"
	if err := d.Callback().Query().After("gorm:query").Register(callback, func(q *gorm.DB) {
		statements = append(statements, q.Statement.SQL.String())
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Callback().Query().Remove(callback) })
	previous, err := adminAuthenticateTwoQueries(ctx, token)
	if err != nil || len(statements) != 2 {
		t.Fatal("two-query baseline", len(statements), err)
	}
	statements = nil
	current, err := AdminAuthenticate(ctx, token)
	if err != nil || !reflect.DeepEqual(previous, current) || len(statements) != 1 {
		t.Fatal("single-query identity or query count", len(statements), err)
	}
	if strings.Contains(strings.ToLower(statements[0]), "password_hash") || strings.Contains(statements[0], "*") {
		t.Fatal("authentication loaded unused credential columns")
	}
	if !AdminCheckCSRF(current, strings.Repeat("b", 64)) {
		t.Fatal("session-bound CSRF changed")
	}
	// Direct committed changes are observed on the next request without a cache.
	for _, role := range []string{constUtil.AdminRoleOperator, constUtil.AdminRoleAdmin} {
		if err := d.Model(&user).Update("role", role).Error; err != nil {
			t.Fatal(err)
		}
		p, err := AdminAuthenticate(ctx, token)
		if err != nil || p.Role != role {
			t.Fatal("stale role", err)
		}
	}
	for _, change := range []struct {
		column string
		value  any
	}{{"enabled", false}, {"role", "invalid-role"}} {
		values := map[string]any{"enabled": true, "role": constUtil.AdminRoleViewer}
		values[change.column] = change.value
		if err := d.Model(&user).Updates(values).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := AdminAuthenticate(ctx, token); !errors.Is(err, constUtil.ErrAdminSession) {
			t.Fatal("revoked account authenticated", err)
		}
	}
	if err := d.Model(&user).Updates(map[string]any{"enabled": true, "role": constUtil.AdminRoleViewer}).Error; err != nil {
		t.Fatal(err)
	}
	if err := AdminLogout(ctx, token); err != nil {
		t.Fatal(err)
	}
	if _, err := AdminAuthenticate(ctx, token); !errors.Is(err, constUtil.ErrAdminSession) {
		t.Fatal("deleted session authenticated", err)
	}
	if err := d.Create(&db.AdminSession{TokenHash: adminDigest(token), UserID: user.ID, CSRFHash: "", ExpiresAtUnix: time.Now().Unix() - 1}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := AdminAuthenticate(ctx, token); !errors.Is(err, constUtil.ErrAdminSession) {
		t.Fatal("expired session authenticated", err)
	}
	if err := d.Model(&db.AdminSession{}).Where("token_hash=?", adminDigest(token)).Update("expires_at_unix", time.Now().Unix()+3600).Error; err != nil {
		t.Fatal(err)
	}
	if err := d.Delete(&user).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := AdminAuthenticate(ctx, token); !errors.Is(err, constUtil.ErrAdminSession) {
		t.Fatal("orphaned session authenticated", err)
	}
	t.Log("SQL per successful authentication: 2 -> 1; immediate role/disable/logout/expiry/orphan checks passed")
}

func BenchmarkAdminAuthenticateQueries(b *testing.B) {
	ctx, _, token := adminAuthPerformanceFixture(b)
	for _, item := range []struct {
		name string
		fn   func(context.Context, string) (*model.AdminPrincipal, error)
	}{{"previous-two-queries", adminAuthenticateTwoQueries}, {"current-one-query", AdminAuthenticate}} {
		b.Run(item.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := item.fn(ctx, token); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
