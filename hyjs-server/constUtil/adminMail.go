package constUtil

import (
	"fmt"
	"time"
)

// AdminMailPreviewTTL bounds the time between operator review and explicit mail sending.
const AdminMailPreviewTTL = 15 * time.Minute

var (
	ErrAdminMailExpired  = fmt.Errorf("%w：邮件预览已过期，请重新预览", ErrAdminConfigConflict)
	ErrAdminMailEmpty    = fmt.Errorf("%w：当前范围没有可发送邮件的已建角色", ErrAdminInput)
	ErrAdminMailAudience = fmt.Errorf("%w：预览中的角色归属、冻结或保护状态已变化，请重新预览", ErrAdminConfigConflict)
)
