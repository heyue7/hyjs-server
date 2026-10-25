package constUtil

const (
	PlayerRenameStateKey       = "player.rename.v1"
	PlayerRenameInvalid        = "昵称需要1至12个半角宽度，最多6个汉字，不能包含控制字符或首尾空白"
	PlayerRenameUnchanged      = "新昵称与当前昵称相同"
	PlayerRenameReceiptInvalid = "改名次数或回执来源无效"
	PlayerRenameRequestExpired = "改名请求已过期或重试内容不一致"
	PlayerRenameUnavailable    = "改名事务尚未初始化"
	PlayerRenameCostSource     = "local-player-rename-v1：原20017/20018、20115/20116；首次免费、后续EditNameCost原300钻、UTF16宽度12；原服务同名细则未知，沿当前Create允许不同ID使用重复显示名，不新增全局唯一约束；拒绝原地同名改名及首尾空白/控制字符。原客户端字库HasCharacter仍由原UI执行；服务端按本地固定版本色情/政治词库统一拦截，词库与规范化策略见utils/contentfilter/README.md；不声称已恢复原服SDK审核。次数/最后请求回执与名字在同角色Profile行锁存档事务提交。"
)
