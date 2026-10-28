package model

// GGOFieldTradeLink is a small pointer to the current bilateral quote. Durable
// quotes, mutations and settlement evidence remain in the existing receipt table.
type GGOFieldTradeLink struct {
	TradeID     string `json:"trade_id"`
	InitiatorID int64  `json:"initiator_id"`
	ReceiverID  int64  `json:"receiver_id"`
	ExpiresAtMS int64  `json:"expires_at_ms"`
}

type GGOFieldTradeInviteRequest struct {
	GGOFieldMutation
	TargetPlayerID int64 `json:"targetPlayerId" binding:"required"`
}

type GGOFieldTradeCurrentRequest struct{ GGOFieldAuth }

type GGOFieldTradeStatusRequest struct {
	GGOFieldAuth
	TradeID     string `json:"tradeId" binding:"required"`
	InitiatorID int64  `json:"initiatorId" binding:"required"`
	ReceiverID  int64  `json:"receiverId" binding:"required"`
}

type GGOFieldTradeActionRequest struct {
	GGOFieldMutation
	TradeID     string  `json:"tradeId" binding:"required"`
	InitiatorID int64   `json:"initiatorId" binding:"required"`
	ReceiverID  int64   `json:"receiverId" binding:"required"`
	Version     int64   `json:"version"`
	Offer       []int64 `json:"offer,omitempty"`
}

// GGOFieldTradeView contains both offers but only the caller's terminal balance.
// Credit estimates are decimal strings so multiplication cannot truncate int64 chips.
type GGOFieldTradeView struct {
	Active            bool                 `json:"active"`
	TradeID           string               `json:"tradeId,omitempty"`
	InitiatorID       int64                `json:"initiatorId,omitempty"`
	ReceiverID        int64                `json:"receiverId,omitempty"`
	Status            string               `json:"status"`
	Reason            string               `json:"reason,omitempty"`
	Version           int64                `json:"version,omitempty"`
	Offers            [2][6]int64          `json:"offers"`
	Confirmed         [2]bool              `json:"confirmed"`
	OfferCreditValues [2]string            `json:"offerCreditValues"`
	ExpiresAtMS       int64                `json:"expiresAtMs,omitempty"`
	ServerTimeMS      int64                `json:"serverTimeMs"`
	Terminal          GGOFieldTerminalView `json:"terminal"`
	AfterChips        [6]int64             `json:"afterChips"`
	AfterProtected    [6]int64             `json:"afterProtected"`
	PreviewValid      bool                 `json:"previewValid"`
}
