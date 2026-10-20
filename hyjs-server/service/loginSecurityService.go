package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
)

type loginBucket struct {
	Tokens float64
	Last   time.Time
}
type loginSourceBuckets struct {
	sync.Mutex
	Global  loginBucket
	Sources map[string]loginBucket
	Sweep   time.Time
}

var loginSources = loginSourceBuckets{Sources: make(map[string]loginBucket)}

// LoginIngressAllowed protects only unauthenticated credential work. Its burst
// permits a shared network's normal launch traffic; established play is excluded.
func LoginIngressAllowed(source string, now time.Time) bool {
	ip := net.ParseIP(source)
	if ip == nil {
		source = "unknown"
	} else {
		source = ip.String()
	}
	loginSources.Lock()
	defer loginSources.Unlock()
	if now.Sub(loginSources.Sweep) >= time.Minute {
		for key, value := range loginSources.Sources {
			if now.Sub(value.Last) > constUtil.LoginSourceLifetime {
				delete(loginSources.Sources, key)
			}
		}
		loginSources.Sweep = now
	}
	refill := func(b loginBucket, rate float64) loginBucket {
		if b.Last.IsZero() {
			return loginBucket{Tokens: constUtil.LoginBurst, Last: now}
		}
		elapsed := now.Sub(b.Last).Seconds()
		if elapsed > 0 {
			b.Tokens = min(float64(constUtil.LoginBurst), b.Tokens+elapsed*rate)
		}
		b.Last = now
		return b
	}
	global := refill(loginSources.Global, constUtil.LoginGlobalRate)
	value, exists := loginSources.Sources[source]
	if !exists && len(loginSources.Sources) >= constUtil.LoginSourceCapacity {
		return false
	}
	value = refill(value, constUtil.LoginSourceRate)
	allowed := global.Tokens >= 1 && value.Tokens >= 1
	if allowed {
		global.Tokens--
		value.Tokens--
	}
	loginSources.Global = global
	loginSources.Sources[source] = value
	return allowed
}

// LoginFailureDelay leaves four ordinary mistakes unaffected, then backs off
// briefly. Attempts made during an existing delay do not extend that delay.
func LoginFailureDelay(failures int64) time.Duration {
	if failures < 5 {
		return 0
	}
	if failures >= 10 {
		return constUtil.LoginMaximumDelay
	}
	return time.Second << uint(failures-5)
}

func gameLoginKey(username string) string {
	sum := sha256.Sum256([]byte(username))
	return hex.EncodeToString(sum[:])
}

// InitializeLoginSecurity uses the optional shared identity schema so the same
// account's retry delay is respected by every realm on that MySQL instance.
func InitializeLoginSecurity(ctx context.Context) error {
	d := db.GetIns().WithContext(ctx)
	if d.Dialector.Name() == "mysql" {
		return d.Exec("CREATE TABLE IF NOT EXISTS " + db.AccountsTableName("game_login_throttles") + " (account_key VARCHAR(64) PRIMARY KEY, failures BIGINT NOT NULL, window_started_at BIGINT NOT NULL, retry_at BIGINT NOT NULL, updated_at BIGINT NOT NULL, INDEX game_login_updated(updated_at)) ENGINE=InnoDB").Error
	}
	return d.AutoMigrate(&db.GameLoginThrottle{})
}

func GameLoginRetryAfter(ctx context.Context, username string, now time.Time) (time.Duration, error) {
	var row db.GameLoginThrottle
	err := db.AccountsTable(db.GetIns().WithContext(ctx), "game_login_throttles").First(&row, "account_key=?", gameLoginKey(username)).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return max(time.Duration(0), time.UnixMilli(row.RetryAt).Sub(now)), nil
}

// RecordGameLoginFailure persists counters only for existing identities. Random
// attacker-controlled account names cannot create an unbounded throttle table.
func RecordGameLoginFailure(ctx context.Context, username string, now time.Time) (time.Duration, error) {
	d := db.GetIns().WithContext(ctx)
	var count int64
	if globalGameAccountsAvailable(d) {
		if err := db.AccountsTable(d, "game_accounts").Where("username=?", username).Count(&count).Error; err != nil {
			return 0, err
		}
	} else {
		if err := d.Model(&db.Profile{}).Where("uid=?", username).Count(&count).Error; err != nil {
			return 0, err
		}
	}
	if count == 0 {
		return 0, nil
	}
	var delay time.Duration
	err := d.Transaction(func(tx *gorm.DB) error {
		row := db.GameLoginThrottle{AccountKey: gameLoginKey(username)}
		if err := db.AccountsTable(tx, "game_login_throttles").Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			return err
		}
		if err := db.AccountsTable(tx, "game_login_throttles").Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "account_key=?", row.AccountKey).Error; err != nil {
			return err
		}
		if row.RetryAt > now.UnixMilli() {
			delay = time.UnixMilli(row.RetryAt).Sub(now)
			return nil
		}
		if row.WindowStartedAt == 0 || now.UnixMilli()-row.WindowStartedAt >= constUtil.LoginFailureWindow.Milliseconds() {
			row.Failures = 0
			row.WindowStartedAt = now.UnixMilli()
		}
		row.Failures = min(row.Failures+1, int64(10))
		delay = LoginFailureDelay(row.Failures)
		row.RetryAt = now.Add(delay).UnixMilli()
		row.UpdatedAt = now.UnixMilli()
		return db.AccountsTable(tx, "game_login_throttles").Where("account_key=?", row.AccountKey).Updates(map[string]any{"failures": row.Failures, "window_started_at": row.WindowStartedAt, "retry_at": row.RetryAt, "updated_at": row.UpdatedAt}).Error
	})
	return delay, err
}

func ClearGameLoginFailures(ctx context.Context, username string) error {
	return db.AccountsTable(db.GetIns().WithContext(ctx), "game_login_throttles").Where("account_key=?", gameLoginKey(username)).Delete(&db.GameLoginThrottle{}).Error
}
