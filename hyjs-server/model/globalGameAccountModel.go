package model

// GameAccountDTO exposes the global identity without credential or session bytes.
type GameAccountDTO struct {
	CurrentServerID int64                `json:"currentServerId,omitempty"` // 自助账号页当前会话所属区服；后台列表为0省略。
	CurrentPlayerID int64                `json:"currentPlayerId,omitempty"` // 自助账号页当前区服角色编号，与区服共同标识终端。
	PasswordChanged bool                 `json:"passwordChanged,omitempty"` // PasswordChanged confirms a successful self-service credential update.
	ID              int64                `json:"id"`                        // ID is the global account ID.
	Username        string               `json:"username"`                  // Username is the case-sensitive login name.
	Nickname        string               `json:"nickname"`                  // Nickname is separate from all realm character names.
	Protected       bool                 `json:"protected"`                 // Protected identifies the inherited protected historical account.
	Enabled         bool                 `json:"enabled"`                   // Enabled=false rejects authentication and existing sessions.
	CreatedAt       int64                `json:"createdAt"`                 // CreatedAt is real Unix milliseconds.
	Version         string               `json:"version"`                   // Version protects admin concurrent changes.
	Players         []GameAccountRoleDTO `json:"players"`                   // Players indexes actual roles in their owning runtimes.
}

// GameAccountRoleDTO indexes a real server role; details remain owned by that server.
type GameAccountRoleDTO struct {
	ServerID             int64  `json:"serverId"`             // 所属真实运行区服编号。
	PlayerID             int64  `json:"playerId"`             // 当前区服的角色编号。
	Name                 string `json:"name"`                 // 当前区服的角色名。
	Level                int64  `json:"level"`                // 关联角色的当前等级。
	TerminalBalanceCents *int64 `json:"terminalBalanceCents"` // 本账号该区服角色的实际终端余额；null表示未读取或钱包尚未建立。
	TerminalUnreadCount  *int64 `json:"terminalUnreadCount"`  // 本账号该区服角色未读投放条数；null表示未读取，0表示已读取且没有未读。
}

// AdminGameAccountsResponse is a global identity page.
type AdminGameAccountsResponse struct {
	Items []GameAccountDTO `json:"items"` // 本页真实账号列表。
	Total int64            `json:"total"` // 符合筛选的账号总数。
}

// AdminGameAccountCreateRequest creates a global identity without inventing a realm role.
type AdminGameAccountCreateRequest struct {
	Username        string `json:"username" binding:"required"`        // 区分大小写的登录账号，新增时3至32位字母数字下划线或短横线。
	Password        string `json:"password" binding:"required"`        // 新密码，至少6字符且最多72字节，绝不返回。
	ConfirmPassword string `json:"confirmPassword" binding:"required"` // 与新密码完全相同的确认值。
	Nickname        string `json:"nickname" binding:"required"`        // 全局账号昵称，1至24个可见字符，与角色名独立。
	Reason          string `json:"reason" binding:"required"`          // 本次后台操作的审计原因。
}

// AdminGameAccountUpdateRequest edits global display and login state.
type AdminGameAccountUpdateRequest struct {
	Nickname string `json:"nickname" binding:"required"` // 全局账号昵称，1至24个可见字符，与角色名独立。
	Enabled  bool   `json:"enabled"`                     // false停用登录和旧会话，true允许后续重新登录。
	Version  string `json:"version" binding:"required"`  // 读取账号时的乐观并发版本。
	Reason   string `json:"reason" binding:"required"`   // 本次后台操作的审计原因。
}

// AdminGameAccountPasswordRequest resets a global account password with an audit reason.
type AdminGameAccountPasswordRequest struct {
	Password        string `json:"password" binding:"required"`        // 新密码，至少6字符且最多72字节，绝不返回。
	ConfirmPassword string `json:"confirmPassword" binding:"required"` // 与新密码完全相同的确认值。
	Version         string `json:"version" binding:"required"`         // 读取账号时的乐观并发版本。
	Reason          string `json:"reason" binding:"required"`          // 本次后台操作的审计原因。
}

// GameAccountProfileRequest authorizes a global identity through the current real role session.
type GameAccountProfileRequest struct {
	AccountSession string `json:"accountSession"` // 64位全局账号会话，可独立于角色使用。
	PlayerID       int64  `json:"playerId"`       // 当前区服的角色编号。
	SessionHex     string `json:"sessionHex"`     // 当前角色的16字节会话，以32位十六进制传入。
}

// GameAccountNicknameRequest changes only the global nickname.
type GameAccountNicknameRequest struct {
	AccountSession string `json:"accountSession"`              // 64位全局账号会话，可独立于角色使用。
	PlayerID       int64  `json:"playerId"`                    // 当前区服的角色编号。
	SessionHex     string `json:"sessionHex"`                  // 当前角色的16字节会话，以32位十六进制传入。
	Nickname       string `json:"nickname" binding:"required"` // 全局账号昵称，1至24个可见字符，与角色名独立。
}

// GameAccountPasswordRequest verifies the old password before changing the global credential.
type GameAccountPasswordRequest struct {
	AccountSession  string `json:"accountSession"`                     // 64位全局账号会话，可独立于角色使用。
	PlayerID        int64  `json:"playerId"`                           // 当前区服的角色编号。
	SessionHex      string `json:"sessionHex"`                         // 当前角色的16字节会话，以32位十六进制传入。
	OldPassword     string `json:"oldPassword" binding:"required"`     // 用户当前原密码，只用于验证，不保存明文。
	Password        string `json:"password" binding:"required"`        // 新密码，至少6字符且最多72字节，绝不返回。
	ConfirmPassword string `json:"confirmPassword" binding:"required"` // 与新密码完全相同的确认值。
}

// GameAccountLogoutRequest releases the current role session of a Brain Burst 2039 account.
type GameAccountLogoutRequest struct {
	AccountSession string `json:"accountSession"` // 64位全局账号会话，可独立于角色使用。
	PlayerID       int64  `json:"playerId"`       // 当前区服的角色编号。
	SessionHex     string `json:"sessionHex"`     // 当前会话的32位十六进制值。
}

// GameAccountLogoutResponse also acknowledges a session already released by a retry.
type GameAccountLogoutResponse struct {
	LoggedOut bool `json:"loggedOut"` // true表示客户端可以清除当前登录状态。
}

// GameAccountLoginRequest authenticates the account before a realm is selected.
type GameAccountLoginRequest struct {
	Username string `json:"username" binding:"required"` // 全局登录账号。
	Password string `json:"password" binding:"required"` // 登录密码。
}

// GameAccountLoginResponse returns the one-time plain token and public account projection.
type GameAccountLoginResponse struct {
	AccountSession string         `json:"accountSession"` // 仅客户端内存保存的全局会话。
	Account        GameAccountDTO `json:"account"`        // 全局账号及完整角色索引。
}
