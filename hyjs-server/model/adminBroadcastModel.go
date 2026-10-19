package model

// AdminBroadcastPreviewRequest freezes the selected realm's current characters and attachment.
type AdminBroadcastPreviewRequest struct {
	RequestID string                `json:"requestId" binding:"required"` // RequestID is the preview UUID; retries must keep the same content.
	Title     string                `json:"title" binding:"required"`     // Title is the original system mail title.
	Body      string                `json:"body" binding:"required"`      // Body is the original mail text.
	Rewards   []AdminResourceAmount `json:"rewards" binding:"required"`   // Rewards contains 1..100 distinct original resources.
	Reason    string                `json:"reason" binding:"required"`    // Reason is retained in operation audit.
}

// AdminBroadcastSendRequest explicitly confirms one immutable audience preview.
type AdminBroadcastSendRequest struct {
	RequestID string `json:"requestId" binding:"required"` // RequestID is the send UUID reused for all retries.
	Version   string `json:"version" binding:"required"`   // Version binds the reviewed content and frozen audience.
}

// AdminBroadcastDTO describes a preview or committed delivery batch without credentials.
type AdminBroadcastDTO struct {
	ID             string                `json:"id"`             // ID is the preview/batch UUID.
	ServerID       int64                 `json:"serverId"`       // ServerID is the actual serving realm.
	ServerName     string                `json:"serverName"`     // ServerName is the current realm label.
	ActorName      string                `json:"actorName"`      // ActorName identifies the operator.
	Title          string                `json:"title"`          // Title is the exact frozen mail title.
	Body           string                `json:"body"`           // Body is the frozen original mail body.
	Rewards        []AdminResourceAmount `json:"rewards"`        // Rewards is the same attachment for every recipient.
	Reason         string                `json:"reason"`         // Reason records the operator's purpose.
	RecipientCount int64                 `json:"recipientCount"` // RecipientCount is the frozen number of created, unprotected characters.
	ExcludedCount  int64                 `json:"excludedCount"`  // ExcludedCount counts protected characters omitted from the preview.
	SuccessCount   int64                 `json:"successCount"`   // SuccessCount counts committed original reward notices.
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
