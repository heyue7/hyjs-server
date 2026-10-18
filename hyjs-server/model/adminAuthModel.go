package model

// AdminUserDTO 为后台列表和当前登录身份提供最小账号信息。
type AdminUserDTO struct {
	// ID 是后台账号ID，与游戏玩家ID独立。
	ID int64 `json:"id"`
	// Username 是后台登录名。
	Username string `json:"username"`
	// Role 取viewer只读、operator运营、admin管理员。
	Role string `json:"role"`
}

// AdminLoginResult 的原始口令仅交给HTTP层设置Cookie，禁止直接序列化到JSON。
type AdminLoginResult struct {
	// User 是当前登录身份。
	User AdminUserDTO `json:"user"`
	// SessionToken 是仅本次返回的随机会话口令。
	SessionToken string `json:"-"`
	// CsrfToken 用于独立非HttpOnly Cookie及CSRF请求头。
	CsrfToken string `json:"-"`
	// ExpiresAt 是会话到期Unix秒。
	ExpiresAt int64 `json:"expiresAt"`
}

// AdminPrincipal 是认证后的服务端身份，不允许通过客户端输入构造。
type AdminPrincipal struct {
	// UserID 是已认证的后台账号ID。
	UserID int64 `json:"-"`
	// Username 是已认证的登录名。
	Username string `json:"username"`
	// Role 取viewer只读、operator运营、admin管理员。
	Role string `json:"role"`
	// CSRFHash 是会话CSRF摘要，只在服务端校验使用。
	CSRFHash string `json:"-"`
}
