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

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

// AdminInitialize 只创建独立后台结构；首次启动可从0600文件建立管理员，永不覆盖既有账号。
func AdminInitialize(ctx context.Context, bootstrapFilePath string) error {
	d := db.GetIns().WithContext(ctx)
	if err := d.AutoMigrate(&db.AdminUser{}, &db.AdminSession{}, &db.AdminLoginThrottle{}, &db.AdminAudit{}); err != nil {
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
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(bootstrap.Password), bcrypt.DefaultCost)
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

// AdminLogin 原子保存失败冷却或创建随机12小时会话；失败回包与账户是否存在无关。
func AdminLogin(ctx context.Context, username, password string) (*model.AdminLoginResult, error) {
	// Handler负责清洗；内部认证仍拒绝非规范账号，防止同账号冷却被大小写或空格绕过。
	username = strings.ToLower(strings.TrimSpace(username))
	if !adminUsernameValid(username) || len(password) > constUtil.AdminPasswordMaxBytes {
		return nil, constUtil.ErrAdminCredentials
	}
	var result *model.AdminLoginResult
	var authErr error
	now := time.Now().Unix()
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		state := db.AdminLoginThrottle{Username: username}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&state).Error; err != nil {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&state, "username = ?", username).Error; err != nil {
			return err
		}
		if state.LockedUntilUnix > now {
			authErr = constUtil.ErrAdminLoginCooldown
			return nil
		}
		if state.WindowStartedAtUnix == 0 || now-state.WindowStartedAtUnix >= constUtil.AdminLoginCooldownSeconds {
			state.Failures = 0
			state.WindowStartedAtUnix = now
			state.LockedUntilUnix = 0
		}
		var user db.AdminUser
		lookup := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("username = ?", username).Take(&user).Error
		if lookup != nil && !errors.Is(lookup, gorm.ErrRecordNotFound) {
			return lookup
		}
		valid := lookup == nil && user.Enabled && AdminRoleValid(user.Role)
		passwordHash := []byte(user.PasswordHash)
		if !valid {
			// 不存在的账号同样执行bcrypt，避免廉价的账号枚举与无成本猜测。
			var err error
			passwordHash, err = bcrypt.GenerateFromPassword([]byte("unavailable-admin-login"), bcrypt.DefaultCost)
			if err != nil {
				return err
			}
		}
		if bcrypt.CompareHashAndPassword(passwordHash, []byte(password)) != nil || !valid {
			state.Failures++
			authErr = constUtil.ErrAdminCredentials
			if state.Failures >= constUtil.AdminLoginFailureLimit {
				state.LockedUntilUnix = now + constUtil.AdminLoginCooldownSeconds
				authErr = constUtil.ErrAdminLoginCooldown
			}
			return tx.Save(&state).Error
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
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
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
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
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
			if !user.Enabled || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(currentPassword)) != nil {
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
