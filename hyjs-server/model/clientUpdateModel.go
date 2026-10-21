package model

// ClientUpdatePolicy 定义客户端更新接口数据。
type ClientUpdatePolicy struct {
	// ReleaseID 是当前策略所引用的发布记录ID。
	ReleaseID int64 `json:"releaseId"`
	// LatestBuild 是已发布的最新构建号。
	LatestBuild int64 `json:"latestBuild"`
	// MinLoginBuild 是新登录最低构建号，0表示无限制。
	MinLoginBuild int64 `json:"minLoginBuild"`
	// MinRunBuild 是当前会话最低构建号，0表示无限制。
	MinRunBuild int64 `json:"minRunBuild"`
	// Revision 是单行策略递增修订号。
	Revision int64 `json:"revision"`
	// NoticeBatch 是每次发布或再次提醒递增的批次号。
	NoticeBatch int64 `json:"noticeBatch"`
	// NoticeEnabled 为 false=关闭提醒，true=允许展示当前批次。
	NoticeEnabled bool `json:"noticeEnabled"`
	// Maintenance 为 false=正常服务，true=维护拦截中。
	Maintenance bool `json:"maintenance"`
	// MaintenanceStartAt 是计划维护开始的毫秒时间，0表示无计划。
	MaintenanceStartAt int64 `json:"maintenanceStartAt"`
	// MaintenanceEndAt 是预计维护结束的毫秒时间，不会自动开服。
	MaintenanceEndAt int64 `json:"maintenanceEndAt"`
	// VersionName 是展示给玩家的版本名称。
	VersionName string `json:"versionName"`
	// Title 是玩家更新提示的标题。
	Title string `json:"title"`
	// Content 是玩家更新提示的纯文本说明。
	Content string `json:"content"`
	// Mode 取 notice=提醒、next_login=下次登录限制、maintenance=维护更新。
	Mode string `json:"mode"`
}

// ClientUpdateRelease 定义客户端更新接口数据。
type ClientUpdateRelease struct {
	// ID 是草稿或发布记录ID，创建草稿传0。
	ID int64 `json:"id"`
	// BuildCode 是APK独立构建号，0表示无法识别。
	BuildCode int64 `json:"buildCode"`
	// VersionName 是展示给玩家的版本名称。
	VersionName string `json:"versionName"`
	// Title 是玩家更新提示的标题。
	Title string `json:"title"`
	// Content 是玩家更新提示的纯文本说明。
	Content string `json:"content"`
	// Mode 取 notice=提醒、next_login=下次登录限制、maintenance=维护更新。
	Mode string `json:"mode"`
	// MinLoginBuild 是新登录最低构建号，0表示无限制。
	MinLoginBuild int64 `json:"minLoginBuild"`
	// MinRunBuild 是当前会话最低构建号，0表示无限制。
	MinRunBuild int64 `json:"minRunBuild"`
	// MaintenanceStartAt 是计划维护开始的毫秒时间，0表示无计划。
	MaintenanceStartAt int64 `json:"maintenanceStartAt"`
	// MaintenanceEndAt 是预计维护结束的毫秒时间，不会自动开服。
	MaintenanceEndAt int64 `json:"maintenanceEndAt"`
	// State 取 draft=草稿、published=已发布、withdrawn=已撤回。
	State string `json:"state"`
	// PublishedAt 是首次发布的毫秒时间。
	PublishedAt int64 `json:"publishedAt"`
	// UpdatedAt 是最后修改的毫秒时间。
	UpdatedAt int64 `json:"updatedAt"`
	// PublishedBy 是执行发布的后台用户名。
	PublishedBy string `json:"publishedBy"`
}

// ClientUpdateStats 定义客户端更新接口数据。
type ClientUpdateStats struct {
	// Online 是当前服务经会话和账号校验的连接数量。
	Online int64 `json:"online"`
	// Older 是当前连接中低于最新build的数量。
	Older int64 `json:"older"`
	// Unknown 是当前连接中无法识别build的数量。
	Unknown int64 `json:"unknown"`
	// Shown 是当前提醒批次已实际展示的认证在线玩家数。
	Shown int64 `json:"shown"`
	// Acknowledged 是当前提醒批次已确认的认证在线玩家数。
	Acknowledged int64 `json:"acknowledged"`
	// Pending 是当前连接中尚无当前批次展示回执的数量。
	Pending int64 `json:"pending"`
	// ObservedAt 是当前服务连接快照的毫秒时间。
	ObservedAt int64 `json:"observedAt"`
}

// AdminClientUpdatesResponse 定义客户端更新接口数据。
type AdminClientUpdatesResponse struct {
	// Version 是全局策略修订字符串，写入时用于乐观锁。
	Version string `json:"version"`
	// Policy 是当前权威更新策略。
	Policy ClientUpdatePolicy `json:"policy"`
	// Items 是当前页面所需的有限发布记录。
	Items []ClientUpdateRelease `json:"items"`
	// Stats 是当前运行服务的连接快照统计。
	Stats ClientUpdateStats `json:"stats"`
	// CanEdit 为 false=只读，true=可保存草稿。
	CanEdit bool `json:"canEdit"`
	// CanPublish 为 false=禁止发布，true=管理员可提交操作。
	CanPublish bool `json:"canPublish"`
}

// AdminClientUpdateDraftRequest 定义客户端更新接口数据。
type AdminClientUpdateDraftRequest struct {
	// ID 是草稿或发布记录ID，创建草稿传0。
	ID int64 `json:"id"`
	// Version 是全局策略修订字符串，写入时用于乐观锁。
	Version string `json:"version" binding:"required"`
	// BuildCode 是APK独立构建号，0表示无法识别。
	BuildCode int64 `json:"buildCode"`
	// VersionName 是展示给玩家的版本名称。
	VersionName string `json:"versionName"`
	// Title 是玩家更新提示的标题。
	Title string `json:"title"`
	// Content 是玩家更新提示的纯文本说明。
	Content string `json:"content"`
	// Mode 取 notice=提醒、next_login=下次登录限制、maintenance=维护更新。
	Mode string `json:"mode"`
	// MinLoginBuild 是新登录最低构建号，0表示无限制。
	MinLoginBuild int64 `json:"minLoginBuild"`
	// MinRunBuild 是当前会话最低构建号，0表示无限制。
	MinRunBuild int64 `json:"minRunBuild"`
	// MaintenanceStartAt 是计划维护开始的毫秒时间，0表示无计划。
	MaintenanceStartAt int64 `json:"maintenanceStartAt"`
	// MaintenanceEndAt 是预计维护结束的毫秒时间，不会自动开服。
	MaintenanceEndAt int64 `json:"maintenanceEndAt"`
	// Reason 是管理员操作原因或客户端受限提示。
	Reason string `json:"reason"`
}

// AdminClientUpdateActionRequest 定义客户端更新接口数据。
type AdminClientUpdateActionRequest struct {
	// ID 是草稿或发布记录ID，创建草稿传0。
	ID int64 `json:"id"`
	// Version 是全局策略修订字符串，写入时用于乐观锁。
	Version string `json:"version" binding:"required"`
	// RequestID 是本次操作的唯一幂等键。
	RequestID string `json:"requestId" binding:"required"`
	// Action 取 publish、remind、withdraw、start_maintenance、end_maintenance。
	Action string `json:"action" binding:"required"`
	// Reason 是管理员操作原因或客户端受限提示。
	Reason string `json:"reason"`
}

// AdminClientUpdateHistoryResponse 定义客户端更新接口数据。
type AdminClientUpdateHistoryResponse struct {
	// Items 是当前页面所需的有限发布记录。
	Items []ClientUpdateRelease `json:"items"`
	// Total 是历史记录总条数。
	Total int64 `json:"total"`
}

// ClientUpdateStatusRequest 定义客户端更新接口数据。
type ClientUpdateStatusRequest struct {
	// BuildCode 是APK独立构建号，0表示无法识别。
	BuildCode int64 `json:"buildCode"`
	// StartupID 是当前客户端进程的32位小写十六进制启动标识。
	StartupID string `json:"startupId" binding:"required"`
	// PlayerID 是当前真实会话绑定的玩家ID，匿名查询可省略。
	PlayerID int64 `json:"playerId"`
	// SessionHex 是真实16字节会话的十六进制编码，仅用于服务端验证。
	SessionHex string `json:"sessionHex"`
}

// ClientUpdateAckRequest 定义客户端更新接口数据。
type ClientUpdateAckRequest struct {
	// BuildCode 是APK独立构建号，0表示无法识别。
	BuildCode int64 `json:"buildCode"`
	// StartupID 是当前客户端进程的32位小写十六进制启动标识。
	StartupID string `json:"startupId" binding:"required"`
	// PlayerID 是当前真实会话绑定的玩家ID，匿名查询可省略。
	PlayerID int64 `json:"playerId"`
	// SessionHex 是真实16字节会话的十六进制编码，仅用于服务端验证。
	SessionHex string `json:"sessionHex"`
	// BatchID 是回执所对应的当前提醒批次号。
	BatchID int64 `json:"batchId"`
	// Kind 取 shown=实际展示、acknowledged=玩家确认。
	Kind string `json:"kind" binding:"required"`
}

// ClientUpdateStatusResponse 定义客户端更新接口数据。
type ClientUpdateStatusResponse struct {
	// LatestBuild 是已发布的最新构建号。
	LatestBuild int64 `json:"latestBuild"`
	// MinLoginBuild 是新登录最低构建号，0表示无限制。
	MinLoginBuild int64 `json:"minLoginBuild"`
	// MinRunBuild 是当前会话最低构建号，0表示无限制。
	MinRunBuild int64 `json:"minRunBuild"`
	// NoticeEnabled 为 false=关闭提醒，true=允许展示当前批次。
	NoticeEnabled bool `json:"noticeEnabled"`
	// Maintenance 为 false=正常服务，true=维护拦截中。
	Maintenance bool `json:"maintenance"`
	// MaintenanceStartAt 是计划维护开始的毫秒时间，0表示无计划。
	MaintenanceStartAt int64 `json:"maintenanceStartAt"`
	// MaintenanceEndAt 是预计维护结束的毫秒时间，不会自动开服。
	MaintenanceEndAt int64 `json:"maintenanceEndAt"`
	// VersionName 是展示给玩家的版本名称。
	VersionName string `json:"versionName"`
	// Title 是玩家更新提示的标题。
	Title string `json:"title"`
	// Content 是玩家更新提示的纯文本说明。
	Content string `json:"content"`
	// Mode 取 notice=提醒、next_login=下次登录限制、maintenance=维护更新。
	Mode string `json:"mode"`
	// BatchID 是回执所对应的当前提醒批次号。
	BatchID int64 `json:"batchId"`
	// Blocked 为 false=当前允许使用，true=停留登录页并重新检查。
	Blocked bool `json:"blocked"`
	// Reason 是管理员操作原因或客户端受限提示。
	Reason string `json:"reason"`
	// PollSeconds 是建议轮询间隔秒数，固定60。
	PollSeconds int64 `json:"pollSeconds"`
}

// ClientUpdateAckResponse 定义客户端更新接口数据。
type ClientUpdateAckResponse struct {
	// OK 为 false=未保存，true=回执已保存。
	OK bool `json:"ok"`
}
