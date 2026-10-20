package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"encoding/base64"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	"hyjs-server/utils"
)

// AdminInitialize 只创建独立后台结构；首次启动可从0600文件建立管理员，永不覆盖既有账号。
func AdminInitialize(ctx context.Context, bootstrapFilePath string) error {
	d := db.GetIns().WithContext(ctx)
	if err := d.AutoMigrate(&db.AdminUser{}, &db.AdminSession{}, &db.AdminLoginThrottle{}, &db.AdminAudit{}); err != nil {
		return err
	}
	if err := InitializeSecurityEvents(ctx); err != nil {
		return err
	}
	if err := InitializeLoginSecurity(ctx); err != nil {
		return err
	}
	if bootstrapFilePath == "" {
		return nil
	}
	var count int64
	if err := d.Model(&db.AdminUser{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	info, err := os.Lstat(bootstrapFilePath)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || info.Size() > 4096 {
		return constUtil.ErrAdminConfiguration
	}
	file, err := os.Open(bootstrapFilePath)
	if err != nil {
		return err
	}
	defer file.Close()
	var bootstrap struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	decoder := json.NewDecoder(io.LimitReader(file, 4097))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&bootstrap) != nil {
		return constUtil.ErrAdminConfiguration
	}
	var extra any
	if err = decoder.Decode(&extra); err != io.EOF {
		return constUtil.ErrAdminConfiguration
	}
	if !adminUsernameValid(bootstrap.Username) || !adminPasswordValid(bootstrap.Password) {
		return constUtil.ErrAdminConfiguration
	}
	_, passwordHash, err := utils.NewPasswordCredential(ctx, bootstrap.Password)
	if err != nil {
		return err
	}
	return d.Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		if err := tx.Model(&db.AdminUser{}).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return nil
		}
		user := db.AdminUser{Username: bootstrap.Username, PasswordHash: string(passwordHash), Role: constUtil.AdminRoleAdmin, Enabled: true, CreatedAtUnix: time.Now().Unix()}
		if err := tx.Create(&user).Error; err != nil {
			return err
		}
		principal := &model.AdminPrincipal{UserID: user.ID, Username: user.Username, Role: user.Role}
		return adminAudit(tx, principal, "user.bootstrap", strconv.FormatInt(user.ID, 10), "username="+user.Username+" role="+user.Role)
	})
}

func adminUsernameValid(username string) bool {
	return len(username) >= 3 && len(username) <= 64 && regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]*$`).MatchString(username)
}
func adminPasswordValid(password string) bool {
	return len(password) >= constUtil.AdminPasswordMinBytes && len(password) <= constUtil.AdminPasswordMaxBytes
}

// AdminRoleValid 给Handler和内部账号管理复用固定的最小角色集合。
func AdminRoleValid(role string) bool {
	return role == constUtil.AdminRoleViewer || role == constUtil.AdminRoleOperator || role == constUtil.AdminRoleAdmin
}

func adminDigest(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
func adminRandomToken() (string, error) {
	var token [32]byte
	if _, err := rand.Read(token[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(token[:]), nil
}
func adminUserDTO(user *db.AdminUser) model.AdminUserDTO {
	return model.AdminUserDTO{ID: user.ID, Username: user.Username, Role: user.Role}
}

// adminPasswordMatches accepts existing bcrypt and the current PHC credential.
func adminPasswordMatches(ctx context.Context, password, encoded string) (bool, bool, error) {
	var salt []byte
	if strings.HasPrefix(encoded, "$argon2id$") {
		parts := strings.Split(encoded, "$")
		if len(parts) != 6 {
			return false, false, constUtil.ErrAdminCredentials
		}
		var err error
		salt, err = base64.RawStdEncoding.DecodeString(parts[4])
		if err != nil {
			return false, false, constUtil.ErrAdminCredentials
		}
	}
	return utils.VerifyPasswordCredential(ctx, password, salt, []byte(encoded))
}

// AdminLogin computes credentials outside the database transaction, then locks
// and rechecks the exact credential/state before issuing a session or upgrading.
func AdminLogin(ctx context.Context, username, password string) (*model.AdminLoginResult, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	if !adminUsernameValid(username) || len(password) > constUtil.AdminPasswordMaxBytes {
		return nil, constUtil.ErrAdminCredentials
	}
	d := db.GetIns().WithContext(ctx)
	now := time.Now().Unix()
	var throttle db.AdminLoginThrottle
	lookupThrottle := d.First(&throttle, "username=?", username).Error
	if lookupThrottle != nil && !errors.Is(lookupThrottle, gorm.ErrRecordNotFound) {
		return nil, lookupThrottle
	}
	if throttle.LockedUntilUnix > now {
		return nil, constUtil.ErrAdminLoginCooldown
	}
	var snapshot db.AdminUser
	lookup := d.Where("username=?", username).Take(&snapshot).Error
	if lookup != nil && !errors.Is(lookup, gorm.ErrRecordNotFound) {
		return nil, lookup
	}
	valid := lookup == nil && snapshot.Enabled && AdminRoleValid(snapshot.Role)
	var matched, legacy bool
	var err error
	if valid {
		matched, legacy, err = adminPasswordMatches(ctx, password, snapshot.PasswordHash)
	} else {
		err = utils.VerifyUnavailablePassword(ctx, password)
	}
	if err != nil {
		return nil, err
	}
	if errors.Is(lookup, gorm.ErrRecordNotFound) {
		return nil, constUtil.ErrAdminCredentials
	}
	var upgraded []byte
	if matched && legacy {
		_, upgraded, err = utils.NewPasswordCredential(ctx, password)
		if err != nil {
			return nil, err
		}
	}
	var result *model.AdminLoginResult
	var authErr error
	err = d.Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}
		state := db.AdminLoginThrottle{Username: username}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&state).Error; err != nil {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&state, "username=?", username).Error; err != nil {
			return err
		}
		now = time.Now().Unix()
		if state.LockedUntilUnix > now {
			authErr = constUtil.ErrAdminLoginCooldown
			return nil
		}
		if state.WindowStartedAtUnix == 0 || now-state.WindowStartedAtUnix >= int64(constUtil.LoginFailureWindow.Seconds()) {
			state.Failures = 0
			state.WindowStartedAtUnix = now
			state.LockedUntilUnix = 0
		}
		var user db.AdminUser
		found := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("username=?", username).Take(&user).Error
		if found != nil && !errors.Is(found, gorm.ErrRecordNotFound) {
			return found
		}
		// A concurrent password/role/enable change invalidates the computed snapshot.
		if found == nil && valid && (user.ID != snapshot.ID || user.PasswordHash != snapshot.PasswordHash || user.Enabled != snapshot.Enabled || user.Role != snapshot.Role) {
			authErr = constUtil.ErrAdminCredentials
			return nil
		}
		if !valid || !matched || found != nil {
			state.Failures = min(state.Failures+1, int64(10))
			authErr = constUtil.ErrAdminCredentials
			if delay := LoginFailureDelay(state.Failures); delay > 0 {
				state.LockedUntilUnix = now + int64(delay.Seconds())
				authErr = constUtil.ErrAdminLoginCooldown
			}
			return tx.Save(&state).Error
		}
		if len(upgraded) > 0 {
			if err := tx.Model(&user).Update("password_hash", string(upgraded)).Error; err != nil {
				return err
			}
		}
		token, err := adminRandomToken()
		if err != nil {
			return err
		}
		csrf, err := adminRandomToken()
		if err != nil {
			return err
		}
		expires := now + constUtil.AdminSessionLifetimeSeconds
		if err := tx.Create(&db.AdminSession{TokenHash: adminDigest(token), UserID: user.ID, CSRFHash: adminDigest(csrf), ExpiresAtUnix: expires}).Error; err != nil {
			return err
		}
		state.Failures = 0
		state.WindowStartedAtUnix = now
		state.LockedUntilUnix = 0
		if err := tx.Save(&state).Error; err != nil {
			return err
		}
		result = &model.AdminLoginResult{User: adminUserDTO(&user), SessionToken: token, CsrfToken: csrf, ExpiresAt: expires}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if authErr != nil {
		return nil, authErr
	}
	return result, nil
}

// AdminAuthenticate 每次读取当前角色，角色改变或密码更新后已撤销会话立即失效。
func AdminAuthenticate(ctx context.Context, token string) (*model.AdminPrincipal, error) {
	if len(token) != 64 {
		return nil, constUtil.ErrAdminSession
	}
	var sessionUser struct {
		UserID   int64
		Username string
		Role     string
		Enabled  bool
		CSRFHash string
	}
	d := db.GetIns().WithContext(ctx)
	// 同一条查询读取有效会话及当前用户状态，减少数据库往返并保持撤权即时可见。
	err := d.Table("admin_sessions AS s").
		Select("u.id AS user_id, u.username, u.role, u.enabled, s.csrf_hash").
		Joins("INNER JOIN admin_users AS u ON u.id = s.user_id").
		Where("s.token_hash = ? AND s.expires_at_unix > ?", adminDigest(token), time.Now().Unix()).
		Take(&sessionUser).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, constUtil.ErrAdminSession
		}
		return nil, err
	}
	if !sessionUser.Enabled || !AdminRoleValid(sessionUser.Role) {
		return nil, constUtil.ErrAdminSession
	}
	return &model.AdminPrincipal{UserID: sessionUser.UserID, Username: sessionUser.Username, Role: sessionUser.Role, CSRFHash: sessionUser.CSRFHash}, nil
}

// AdminCheckCSRF 校验请求头与认证会话绑定的随机CSRF摘要，不能由另一个会话的Cookie代替。
func AdminCheckCSRF(principal *model.AdminPrincipal, header string) bool {
	if principal == nil || len(header) != 64 || len(principal.CSRFHash) != 64 {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(adminDigest(header)), []byte(principal.CSRFHash)) == 1
}

// AdminLogout 仅注销当前会话；已注销口令重复调用保持幂等。
func AdminLogout(ctx context.Context, token string) error {
	if len(token) != 64 {
		return nil
	}
	return db.GetIns().WithContext(ctx).Where("token_hash = ?", adminDigest(token)).Delete(&db.AdminSession{}).Error
}

// adminRequireAdministrator 重新读取角色以拒绝先前认证后已被降权的调用者。
func adminRequireAdministrator(tx *gorm.DB, principal *model.AdminPrincipal) error {
	if principal == nil || principal.UserID < 1 {
		return constUtil.ErrAdminPermission
	}
	var user db.AdminUser
	if err := tx.First(&user, principal.UserID).Error; err != nil {
		return constUtil.ErrAdminPermission
	}
	if !user.Enabled || user.Role != constUtil.AdminRoleAdmin {
		return constUtil.ErrAdminPermission
	}
	return nil
}

// AdminListUsers 仅管理员可查看最小账号目录。
func AdminListUsers(ctx context.Context, principal *model.AdminPrincipal) ([]model.AdminUserDTO, error) {
	d := db.GetIns().WithContext(ctx)
	if err := adminRequireAdministrator(d, principal); err != nil {
		return nil, err
	}
	var rows []db.AdminUser
	if err := d.Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]model.AdminUserDTO, 0, len(rows))
	for _, row := range rows {
		result = append(result, adminUserDTO(&row))
	}
	return result, nil
}

// AdminCreateUser 由管理员建立独立账号，不允许写入客户端提供的密码摘要。
func AdminCreateUser(ctx context.Context, principal *model.AdminPrincipal, username, password, role string) (*model.AdminUserDTO, error) {
	if !adminUsernameValid(username) || !adminPasswordValid(password) || !AdminRoleValid(role) {
		return nil, constUtil.ErrAdminConfiguration
	}
	_, hash, err := utils.NewPasswordCredential(ctx, password)
	if err != nil {
		return nil, err
	}
	user := db.AdminUser{Username: username, PasswordHash: string(hash), Role: role, Enabled: true, CreatedAtUnix: time.Now().Unix()}
	err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		if err := adminRequireAdministrator(tx, principal); err != nil {
			return err
		}
		if err := tx.Create(&user).Error; err != nil {
			return err
		}
		return adminAudit(tx, principal, "user.create", strconv.FormatInt(user.ID, 10), "username="+user.Username+" role="+user.Role)
	})
	if err != nil {
		return nil, err
	}
	result := adminUserDTO(&user)
	return &result, nil
}

// AdminUpdateRole 锁定有序账号目录，保护最后管理员并撤销目标的全部旧权限会话。
func AdminUpdateRole(ctx context.Context, principal *model.AdminPrincipal, userID int64, role string) error {
	if !AdminRoleValid(role) || userID < 1 {
		return constUtil.ErrAdminConfiguration
	}
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var users []db.AdminUser
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Order("id ASC").Find(&users).Error; err != nil {
			return err
		}
		if err := adminRequireAdministrator(tx, principal); err != nil {
			return err
		}
		var target *db.AdminUser
		admins := 0
		for i := range users {
			if users[i].ID == userID {
				target = &users[i]
			}
			if users[i].Enabled && users[i].Role == constUtil.AdminRoleAdmin {
				admins++
			}
		}
		if target == nil {
			return gorm.ErrRecordNotFound
		}
		if target.Role == role {
			return nil
		}
		if target.Enabled && target.Role == constUtil.AdminRoleAdmin && role != constUtil.AdminRoleAdmin && admins <= 1 {
			return constUtil.ErrAdminLastAdministrator
		}
		oldRole := target.Role
		if err := tx.Model(target).Update("role", role).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", userID).Delete(&db.AdminSession{}).Error; err != nil {
			return err
		}
		return adminAudit(tx, principal, "user.role.update", strconv.FormatInt(userID, 10), fmt.Sprintf("username=%s role=%s->%s", target.Username, oldRole, role))
	})
}

// AdminChangePassword 本人需核对旧密码；管理员可重置其他账号，成功后撤销该账号全部会话。
func AdminChangePassword(ctx context.Context, principal *model.AdminPrincipal, userID int64, currentPassword, newPassword string) error {
	if principal == nil || userID < 1 || !adminPasswordValid(newPassword) {
		return constUtil.ErrAdminConfiguration
	}
	var snapshot db.AdminUser
	if err := db.GetIns().WithContext(ctx).First(&snapshot, userID).Error; err != nil {
		return err
	}
	if principal.UserID == userID {
		matched, _, err := adminPasswordMatches(ctx, currentPassword, snapshot.PasswordHash)
		if err != nil {
			return err
		}
		if !snapshot.Enabled || !matched {
			return constUtil.ErrAdminCredentials
		}
	} else if err := adminRequireAdministrator(db.GetIns().WithContext(ctx), principal); err != nil {
		return err
	}
	_, hash, err := utils.NewPasswordCredential(ctx, newPassword)
	if err != nil {
		return err
	}
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var user db.AdminUser
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, userID).Error; err != nil {
			return err
		}
		if principal.UserID == userID {
			if !user.Enabled || user.PasswordHash != snapshot.PasswordHash {
				return constUtil.ErrAdminCredentials
			}
		} else if err := adminRequireAdministrator(tx, principal); err != nil {
			return err
		}
		if err := tx.Model(&user).Update("password_hash", string(hash)).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", userID).Delete(&db.AdminSession{}).Error; err != nil {
			return err
		}
		return adminAudit(tx, principal, "user.password.change", strconv.FormatInt(userID, 10), "username="+user.Username+" role="+user.Role)
	})
}
