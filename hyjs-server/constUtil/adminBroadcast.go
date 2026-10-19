package constUtil

import (
	"fmt"
	"time"
)

// AdminBroadcastPreviewTTL bounds the time between audience review and explicit sending.
const AdminBroadcastPreviewTTL = 15 * time.Minute

var (
	ErrAdminBroadcastExpired  = fmt.Errorf("%w：预览已过期，请重新预览", ErrAdminConfigConflict)
	ErrAdminBroadcastEmpty    = fmt.Errorf("%w：本服没有可发放的已建角色", ErrAdminInput)
	ErrAdminBroadcastAudience = fmt.Errorf("%w：预览中的角色归属或保护状态已变化，请重新预览", ErrAdminConfigConflict)
)
