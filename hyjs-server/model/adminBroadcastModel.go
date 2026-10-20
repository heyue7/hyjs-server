package model

// AdminBroadcastPreviewRequest freezes the selected realm's audience and reward-center attachment.
type AdminBroadcastPreviewRequest struct {
	RequestID string                `json:"requestId" binding:"required"` // RequestID is the preview UUID; retries must keep the same content.
	Audience  AdminAudience         `json:"audience" binding:"required"`  // Audience selects all players across pages or an explicit player subset.
	Title     string                `json:"title" binding:"required"`     // Title is the exact reward-center title.
	Body      string                `json:"body" binding:"required"`      // Body is the exact reward-center description.
	Rewards   []AdminResourceAmount `json:"rewards" binding:"required"`   // Rewards contains 1..100 distinct original resources.
	Reason    string                `json:"reason" binding:"required"`    // Reason is retained in operation audit.
}

// AdminBroadcastSendRequest explicitly confirms one immutable audience preview.
type AdminBroadcastSendRequest struct {
	RequestID string `json:"requestId" binding:"required"` // RequestID is the send UUID reused for all retries.
	Version   string `json:"version" binding:"required"`   // Version binds the reviewed content and frozen audience.
}

// AdminBroadcastDTO describes a preview or committed reward-center delivery batch without credentials.
type AdminBroadcastDTO struct {
	ID             string                `json:"id"`             // ID is the preview/batch UUID.
	ServerID       int64                 `json:"serverId"`       // ServerID is the actual serving realm.
	ServerName     string                `json:"serverName"`     // ServerName is the current realm label.
	ActorName      string                `json:"actorName"`      // ActorName identifies the operator.
	AudienceMode   string                `json:"audienceMode"`   // AudienceMode is all=全部玩家，players=指定玩家；历史批次为空时按all展示。
	Title          string                `json:"title"`          // Title is the exact frozen reward-center title.
	Body           string                `json:"body"`           // Body is the frozen reward-center description.
	Rewards        []AdminResourceAmount `json:"rewards"`        // Rewards is the same attachment for every recipient.
	Reason         string                `json:"reason"`         // Reason records the operator's purpose.
	RecipientCount int64                 `json:"recipientCount"` // RecipientCount is the frozen number of created, unprotected, enabled characters.
	ExcludedCount  int64                 `json:"excludedCount"`  // ExcludedCount counts frozen or protected characters omitted from the preview.
	SuccessCount   int64                 `json:"successCount"`   // SuccessCount counts committed original reward-center deliveries.
	CreatedAt      int64                 `json:"createdAt"`      // CreatedAt is real preview time in milliseconds.
	ExpiresAt      int64                 `json:"expiresAt"`      // ExpiresAt is the send-preview deadline, not an attachment expiry.
	SentAt         int64                 `json:"sentAt"`         // SentAt=0 means not sent; positive means the entire batch committed.
	Version        string                `json:"version"`        // Version binds the preview for confirmation.
}

// AdminBroadcastListResponse returns committed batches for the selected realm.
type AdminBroadcastListResponse struct {
	Items []AdminBroadcastDTO `json:"items"` // Items is one page of committed batches.
	Total int64               `json:"total"` // Total is the committed batch count.
}

// AdminBroadcastPreviewResponse contains the immutable pre-send summary.
type AdminBroadcastPreviewResponse struct {
	Batch AdminBroadcastDTO `json:"batch"` // Batch must be reviewed before confirmation.
}

// AdminBroadcastSendResponse is an idempotent receipt for one committed batch.
type AdminBroadcastSendResponse struct {
	Batch AdminBroadcastDTO `json:"batch"` // Batch reports the committed success count.
}
