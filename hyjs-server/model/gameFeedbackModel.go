package model

// GameFeedbackRequest is the internal support submission command; HTTP endpoints use independent request types.
type GameFeedbackRequest struct {
	PlayerID   int64  `json:"playerId" binding:"required"`   // Original player ID.
	SessionHex string `json:"sessionHex" binding:"required"` // Current original session, never persisted.
	ID         string `json:"id"`                            // Conversation ID for detail/reply.
	RequestID  string `json:"requestId"`                     // UUID idempotency key for create/reply.
	Category   string `json:"category"`                      // bug, suggestion, account, other.
	Subject    string `json:"subject"`                       // Conversation subject.
	Content    string `json:"content"`                       // Message text.
	Page       int    `json:"page"`                          // One-based page.
	PageSize   int    `json:"pageSize"`                      // 1..100.
}

// FeedbackMessageDTO is a credential-free conversation message.
type FeedbackMessageDTO struct {
	ID        string `json:"id"`        // Message ID.
	Sender    string `json:"sender"`    // player or support.
	Content   string `json:"content"`   // Frozen message text.
	CreatedAt int64  `json:"createdAt"` // Real Unix milliseconds.
}

// FeedbackDTO is the shared support conversation projection.
type FeedbackDTO struct {
	ID         string               `json:"id"`                 // Conversation ID.
	PlayerID   int64                `json:"playerId"`           // Owning game character.
	PlayerName string               `json:"playerName"`         // Character display name.
	Subject    string               `json:"subject"`            // Subject.
	Category   string               `json:"category"`           // bug, suggestion, account, other.
	State      string               `json:"state"`              // open, processing, replied, closed.
	CreatedAt  int64                `json:"createdAt"`          // Real Unix milliseconds.
	UpdatedAt  int64                `json:"updatedAt"`          // Real Unix milliseconds.
	Version    string               `json:"version"`            // Optimistic concurrency token.
	Messages   []FeedbackMessageDTO `json:"messages,omitempty"` // Ordered immutable history in detail responses.
}

// FeedbackListResponse pages conversation summaries.
type FeedbackListResponse struct {
	Items    []FeedbackDTO `json:"items"`    // Summaries.
	Total    int64         `json:"total"`    // Matching conversation count.
	Page     int           `json:"page"`     // One-based page.
	PageSize int           `json:"pageSize"` // Page size.
}

// AdminFeedbackReplyRequest adds a support reply with optimistic locking.
type AdminFeedbackReplyRequest struct {
	Version   string `json:"version" binding:"required"`   // Current conversation version.
	Content   string `json:"content" binding:"required"`   // Support reply.
	RequestID string `json:"requestId" binding:"required"` // UUID idempotency key.
}

// AdminFeedbackStateRequest changes the handling state without removing history.
type AdminFeedbackStateRequest struct {
	Version string `json:"version" binding:"required"` // Current conversation version.
	State   string `json:"state" binding:"required"`   // open, processing, replied, closed.
}

// GameFeedbackListRequest defines the list endpoint input.
type GameFeedbackListRequest struct {
	// PlayerID Existing player ID.
	PlayerID int64 `json:"playerId" binding:"required"`
	// SessionHex Current original session.
	SessionHex string `json:"sessionHex" binding:"required"`
	// Page One-based page.
	Page int `json:"page"`
	// PageSize Page size, 1..100.
	PageSize int `json:"pageSize"`
}

// GameFeedbackCreateRequest defines the create endpoint input.
type GameFeedbackCreateRequest struct {
	// PlayerID Existing player ID.
	PlayerID int64 `json:"playerId" binding:"required"`
	// SessionHex Current original session.
	SessionHex string `json:"sessionHex" binding:"required"`
	// RequestID Canonical request UUID.
	RequestID string `json:"requestId" binding:"required"`
	// Category bug, suggestion, account, other.
	Category string `json:"category" binding:"required"`
	// Subject One to eighty characters.
	Subject string `json:"subject" binding:"required"`
	// Content One to two thousand characters.
	Content string `json:"content" binding:"required"`
}

// GameFeedbackDetailRequest defines the detail endpoint input.
type GameFeedbackDetailRequest struct {
	// PlayerID Existing player ID.
	PlayerID int64 `json:"playerId" binding:"required"`
	// SessionHex Current original session.
	SessionHex string `json:"sessionHex" binding:"required"`
	// ID Conversation UUID.
	ID string `json:"id" binding:"required"`
}

// GameFeedbackReplyRequest defines the reply endpoint input.
type GameFeedbackReplyRequest struct {
	// PlayerID Existing player ID.
	PlayerID int64 `json:"playerId" binding:"required"`
	// SessionHex Current original session.
	SessionHex string `json:"sessionHex" binding:"required"`
	// ID Conversation UUID.
	ID string `json:"id" binding:"required"`
	// RequestID Canonical request UUID.
	RequestID string `json:"requestId" binding:"required"`
	// Content One to two thousand characters.
	Content string `json:"content" binding:"required"`
}
