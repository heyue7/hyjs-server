package model

// LocalWalletStateRequest reads the authenticated player's local wallet.
type LocalWalletStateRequest struct {
	// PlayerID is the currently logged-in original game character.
	PlayerID int64 `json:"playerId" binding:"required"`
	// SessionHex contains the current 16-byte game session encoded as 32 hex digits.
	SessionHex string `json:"sessionHex" binding:"required"`
}

// LocalWalletAddRequest adds explicitly requested local test funds.
type LocalWalletAddRequest struct {
	// PlayerID is the currently logged-in original game character.
	PlayerID int64 `json:"playerId" binding:"required"`
	// SessionHex is the current original game session; it is never persisted by the overlay.
	SessionHex string `json:"sessionHex" binding:"required"`
	// RequestID identifies one addition and is reused after an uncertain network result.
	RequestID string `json:"requestId" binding:"required"`
	// AmountCents is a positive integer CNY amount, up to 100000000000000 cents per addition.
	AmountCents int64 `json:"amountCents" binding:"required"`
}

// LocalWalletPayRequest pays one owned original game order using its frozen price.
type LocalWalletPayRequest struct {
	// PlayerID is the currently logged-in original game character.
	PlayerID int64 `json:"playerId" binding:"required"`
	// SessionHex is the current original game session.
	SessionHex string `json:"sessionHex" binding:"required"`
	// OrderID is the original server-generated order identifier; price is never supplied here.
	OrderID string `json:"orderId" binding:"required"`
}
