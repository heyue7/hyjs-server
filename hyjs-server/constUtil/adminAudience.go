package constUtil

import "fmt"

const (
	// AdminAudienceModeAll 冻结预览时当前区服已创建的全部有效角色，跨分页生效。
	AdminAudienceModeAll = "all"
	// AdminAudienceModePlayers 只冻结后台明确勾选的角色ID，可为本页或跨页选择的子集。
	AdminAudienceModePlayers = "players"
	// AdminAudienceMaximumPlayers bounds one explicit cross-page selection.
	AdminAudienceMaximumPlayers = 5000
)

var ErrAdminAudience = fmt.Errorf("%w：勾选的角色已不存在或不属于当前区服，请重新选择", ErrAdminConfigConflict)
