package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	"hyjs-server/utils"
	"strings"
	"time"
)

func accountTokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// LoginGlobalGameAccount opens only an account session; no realm or profile is materialized.
func LoginGlobalGameAccount(ctx context.Context, username, password, peer string) (*model.GameAccountLoginResponse, error) {
	event := model.SecurityEventInput{Actor: username, SourceIP: peer, Action: "game.login", Target: "account", Outcome: "failure", ReasonCode: "invalid_credentials"}
	if !LoginIngressAllowed(peer, time.Now()) {
		event.Outcome = "throttled"
		event.ReasonCode = "ingress_limit"
		ObserveSecurityEvent(ctx, event)
		return nil, errors.New("登录请求较多，请稍后重试")
	}
	if delay, err := GameLoginRetryAfter(ctx, username, time.Now()); err != nil {
		return nil, err
	} else if delay > 0 {
		event.Outcome = "throttled"
		event.ReasonCode = "account_retry"
		ObserveSecurityEvent(ctx, event)
		return nil, fmt.Errorf("登录尝试较多，请%d秒后重试", max(1, int64(delay.Seconds())+1))
	}
	out, err := issueGlobalAccountSession(ctx, username, password)
	if err != nil {
		if strings.HasPrefix(err.Error(), "账号或密码错误") {
			delay, e := RecordGameLoginFailure(ctx, username, time.Now())
			if e != nil {
				return nil, e
			}
			ObserveSecurityEvent(ctx, event)
			if delay > 0 {
				return nil, fmt.Errorf("登录尝试较多，请%d秒后重试", max(1, int64(delay.Seconds())))
			}
			return nil, errors.New("账号或密码错误")
		}
		event.ReasonCode = "service_unavailable"
		ObserveSecurityEvent(ctx, event)
		return nil, err
	}
	_ = ClearGameLoginFailures(ctx, username)
	event.Outcome = "success"
	event.ReasonCode = "authenticated"
	ObserveSecurityEvent(ctx, event)
	return out, nil
}

func issueGlobalAccountSession(ctx context.Context, username, password string) (*model.GameAccountLoginResponse, error) {
	var snapshot db.GameAccount
	err := db.AccountsTable(db.GetIns().WithContext(ctx), "game_accounts").Where("username=?", username).Take(&snapshot).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if e := utils.VerifyUnavailablePassword(ctx, password); e != nil {
			return nil, e
		}
		return nil, errors.New("账号或密码错误")
	}
	if err != nil {
		return nil, err
	}
	valid, legacy, err := globalAccountPasswordMatches(ctx, &snapshot, password)
	if err != nil {
		if passwordCredentialRetryable(err) {
			return nil, err
		}
		return nil, errors.New("账号或密码错误")
	}
	if !valid || !snapshot.Enabled {
		return nil, errors.New("账号或密码错误")
	}
	var salt, hash []byte
	if legacy {
		salt, hash, err = utils.NewPasswordCredential(ctx, password)
		if err != nil {
			return nil, err
		}
	}
	raw := make([]byte, 32)
	if _, err = rand.Read(raw); err != nil {
		return nil, err
	}
	token := hex.EncodeToString(raw)
	out := &model.GameAccountLoginResponse{AccountSession: token}
	err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var account db.GameAccount
		if e := db.AccountsTable(tx, "game_accounts").Clauses(clause.Locking{Strength: "UPDATE"}).First(&account, snapshot.ID).Error; e != nil {
			return e
		}
		if !account.Enabled || account.AuthRevision != snapshot.AuthRevision || !bytes.Equal(account.PasswordSalt, snapshot.PasswordSalt) || !bytes.Equal(account.PasswordHash, snapshot.PasswordHash) {
			return errors.New("账号状态已变化，请重新登录")
		}
		if legacy {
			if e := db.AccountsTable(tx, "game_accounts").Where("id=?", account.ID).Updates(map[string]any{"password_salt": salt, "password_hash": hash}).Error; e != nil {
				return e
			}
		}
		now := time.Now()
		if e := db.AccountsTable(tx, "game_account_sessions").Where("account_id=? AND expires_at<=?", account.ID, now.UnixMilli()).Delete(&db.GameAccountSession{}).Error; e != nil {
			return e
		}
		row := db.GameAccountSession{TokenHash: accountTokenHash(token), AccountID: account.ID, AuthRevision: account.AuthRevision, CreatedAt: now.UnixMilli(), ExpiresAt: now.Add(constUtil.GameAccountSessionLifetime).UnixMilli()}
		if e := db.AccountsTable(tx, "game_account_sessions").Create(&row).Error; e != nil {
			return e
		}
		var e error
		out.Account, e = gameAccountProjection(tx, account)
		return e
	})
	return out, err
}

// lockedGlobalAccountSession follows account -> session -> optional profile lock order.
func lockedGlobalAccountSession(tx *gorm.DB, token string, id int64, session []byte) (*db.GameAccount, *db.Profile, error) {
	var found db.GameAccountSession
	if err := db.AccountsTable(tx, "game_account_sessions").Where("token_hash=?", accountTokenHash(token)).Take(&found).Error; err != nil {
		return nil, nil, errors.New("账号登录已失效")
	}
	var account db.GameAccount
	if err := db.AccountsTable(tx, "game_accounts").Clauses(clause.Locking{Strength: "UPDATE"}).First(&account, found.AccountID).Error; err != nil {
		return nil, nil, err
	}
	var current db.GameAccountSession
	if err := db.AccountsTable(tx, "game_account_sessions").Clauses(clause.Locking{Strength: "UPDATE"}).Where("token_hash=?", found.TokenHash).Take(&current).Error; err != nil {
		return nil, nil, errors.New("账号登录已失效")
	}
	if !account.Enabled || current.AuthRevision != account.AuthRevision || current.ExpiresAt <= time.Now().UnixMilli() {
		return nil, nil, errors.New("账号登录已失效")
	}
	if id == 0 && len(session) == 0 {
		return &account, nil, nil
	}
	var role db.Profile
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&role, id).Error; err != nil {
		return nil, nil, err
	}
	if role.AccountID != account.ID || role.AccountAuthRevision != account.AuthRevision || len(session) != 16 || subtle.ConstantTimeCompare(role.Session, session) != 1 {
		return nil, nil, errors.New("角色登录已失效或账号不匹配")
	}
	if game, ok := tx.Statement.Context.Value(constUtil.GameServiceContextKey).(*GameService); ok && AdminProfileServerID(role) != game.OriginalServerID() {
		return nil, nil, errors.New("角色不属于当前区服账号")
	}
	return &account, &role, nil
}

// CurrentGlobalGameAccountSession authorizes an account token independently from role sessions.
func CurrentGlobalGameAccountSession(ctx context.Context, token string, id int64, session []byte) (*model.GameAccountDTO, error) {
	if token == "" {
		return CurrentGlobalGameAccount(ctx, id, session)
	}
	var out model.GameAccountDTO
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		account, role, e := lockedGlobalAccountSession(tx, token, id, session)
		if e != nil {
			return e
		}
		out, e = gameAccountProjection(tx, *account)
		if role != nil {
			out.CurrentServerID, out.CurrentPlayerID = AdminProfileServerID(*role), role.Id
		}
		return e
	})
	return &out, err
}

// UpdateGlobalGameAccountSession keeps only this token and optional current role valid after a password change.
func UpdateGlobalGameAccountSession(ctx context.Context, token string, id int64, session []byte, nickname, oldPassword, newPassword string) (*model.GameAccountDTO, error) {
	if token == "" {
		return UpdateCurrentGlobalGameAccount(ctx, id, session, nickname, oldPassword, newPassword)
	}
	var snapshot *db.GameAccount
	var salt, hash []byte
	if newPassword != "" {
		err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			a, _, e := lockedGlobalAccountSession(tx, token, id, session)
			snapshot = a
			return e
		})
		if err != nil {
			return nil, err
		}
		valid, _, e := globalAccountPasswordMatches(ctx, snapshot, oldPassword)
		if e != nil {
			if passwordCredentialRetryable(e) {
				return nil, e
			}
			return nil, errors.New("原密码不正确")
		}
		if !valid {
			return nil, errors.New("原密码不正确")
		}
		salt, hash, e = utils.NewPasswordCredential(ctx, newPassword)
		if e != nil {
			return nil, e
		}
	}
	var out model.GameAccountDTO
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		account, role, e := lockedGlobalAccountSession(tx, token, id, session)
		if e != nil {
			return e
		}
		if account.Protected {
			return errors.New("账号登录已失效或受保护")
		}
		if newPassword != "" {
			if snapshot == nil || snapshot.AuthRevision != account.AuthRevision || !bytes.Equal(snapshot.PasswordSalt, account.PasswordSalt) || !bytes.Equal(snapshot.PasswordHash, account.PasswordHash) {
				return errors.New("账号凭据已更新，请重新登录")
			}
			account.PasswordSalt, account.PasswordHash = salt, hash
			account.AuthRevision++
			if e = db.AccountsTable(tx, "game_account_sessions").Where("token_hash=?", accountTokenHash(token)).Update("auth_revision", account.AuthRevision).Error; e != nil {
				return e
			}
			if role != nil {
				if e = tx.Model(&db.Profile{}).Where("id=?", role.Id).Update("account_auth_revision", account.AuthRevision).Error; e != nil {
					return e
				}
			}
		} else {
			account.Nickname = nickname
		}
		account.Revision++
		if e = db.AccountsTable(tx, "game_accounts").Save(account).Error; e != nil {
			return e
		}
		out, e = gameAccountProjection(tx, *account)
		if role != nil {
			out.CurrentServerID, out.CurrentPlayerID = AdminProfileServerID(*role), role.Id
		}
		out.PasswordChanged = newPassword != "" && e == nil
		return e
	})
	return &out, err
}

// LogoutGlobalGameAccountSession revokes an exact token and exact optional role, tolerating retries.
func LogoutGlobalGameAccountSession(ctx context.Context, token string, id int64, session []byte) (*model.GameAccountLogoutResponse, error) {
	if token == "" {
		return LogoutCurrentGlobalGameAccount(ctx, id, session)
	}
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row db.GameAccountSession
		e := db.AccountsTable(tx, "game_account_sessions").Where("token_hash=?", accountTokenHash(token)).Take(&row).Error
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return nil
		}
		if e != nil {
			return e
		}
		// Lock the account before the token, matching account mutation order.
		var account db.GameAccount
		if e = db.AccountsTable(tx, "game_accounts").Clauses(clause.Locking{Strength: "UPDATE"}).First(&account, row.AccountID).Error; e != nil {
			return e
		}
		var current db.GameAccountSession
		e = db.AccountsTable(tx, "game_account_sessions").Clauses(clause.Locking{Strength: "UPDATE"}).Where("token_hash=?", row.TokenHash).Take(&current).Error
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return nil
		}
		if e != nil {
			return e
		}
		// Revoked credentials may acknowledge logout, but cannot authorize any role mutation.
		if !account.Enabled || current.AuthRevision != account.AuthRevision || current.ExpiresAt <= time.Now().UnixMilli() {
			return db.AccountsTable(tx, "game_account_sessions").Where("token_hash=?", current.TokenHash).Delete(&db.GameAccountSession{}).Error
		}
		if id != 0 || len(session) != 0 {
			var role db.Profile
			if e = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&role, id).Error; e != nil {
				return e
			}
			if role.AccountID != account.ID || role.AccountAuthRevision != account.AuthRevision || len(session) != 16 || subtle.ConstantTimeCompare(role.Session, session) != 1 {
				return errors.New("角色登录已失效或账号不匹配")
			}
			if game, ok := ctx.Value(constUtil.GameServiceContextKey).(*GameService); ok && AdminProfileServerID(role) != game.OriginalServerID() {
				return errors.New("角色不属于当前区服账号")
			}

			replacement := make([]byte, 16)
			if _, e = rand.Read(replacement); e != nil {
				return e
			}
			if e = tx.Model(&db.Profile{}).Where("id=? AND session=?", id, session).UpdateColumn("session", replacement).Error; e != nil {
				return e
			}
		}
		return db.AccountsTable(tx, "game_account_sessions").Where("token_hash=?", row.TokenHash).Delete(&db.GameAccountSession{}).Error
	})
	return &model.GameAccountLogoutResponse{LoggedOut: true}, err
}
