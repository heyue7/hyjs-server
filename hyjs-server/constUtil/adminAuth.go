package constUtil

import "errors"

const (
	AdminRoleViewer                   = "viewer"
	AdminRoleOperator                 = "operator"
	AdminRoleAdmin                    = "admin"
	AdminSessionLifetimeSeconds int64 = 12 * 60 * 60
	AdminLoginFailureLimit            = 5
	AdminLoginCooldownSeconds   int64 = 15 * 60
	AdminPasswordMinBytes             = 12
	AdminPasswordMaxBytes             = 72
	AdminSessionCookieName            = "hyjs_admin_session"
	AdminCSRFCookieName               = "hyjs_admin_csrf"
)

var (
	ErrAdminCredentials       = errors.New("后台账号或密码错误")
	ErrAdminLoginCooldown     = errors.New("登录尝试较多，请稍等片刻后重试（最长30秒）")
	ErrAdminSession           = errors.New("后台会话无效或已过期")
	ErrAdminPermission        = errors.New("后台操作权限不足")
	ErrAdminConfiguration     = errors.New("后台认证配置无效")
	ErrAdminLastAdministrator = errors.New("需保留至少一位后台管理员")
)
