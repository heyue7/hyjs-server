package model

import pb "hyjs-server/model/protocol"

// GGOFieldBossFight keeps only the active battle's event and death policy.
// The canonical combat input stays in FieldFight's existing compressed payload.
type GGOFieldBossFight struct {
	InputHash string       `json:"input_hash"`
	Boss      *pb.SideBoss `json:"boss"`
	EventID   string       `json:"event_id"`
	// Line freezes the server-selected spawn line; zero denotes an old fight on line one.
	Line             int64                  `json:"line,omitempty"`
	EndsAtMS         int64                  `json:"ends_at_ms"`
	EconomyVersion   string                 `json:"economy_version"`
	ExchangeValues   [6]int64               `json:"exchange_values"`
	ProtectedChips   int64                  `json:"protected_chips"`
	DeathLossPercent *int64                 `json:"death_loss_percent,omitempty"`
	SafeReturn       *AdminFieldGGOPosition `json:"safe_return"`
	HealRatio        float64                `json:"heal_ratio"`
}

// GGOFieldBossActiveRequest authenticates a read of the current accepted fight.
type GGOFieldBossActiveRequest struct{ GGOFieldAuth }

// GGOFieldBossReceiptRequest looks up an immutable result by original fight ID.
type GGOFieldBossReceiptRequest struct {
	GGOFieldAuth
	FightID string `json:"fightId"`
}

// GGOFieldBossActiveResponse identifies the live engine input without restarting it.
type GGOFieldBossActiveResponse struct {
	Active      bool   `json:"active"`
	FightID     string `json:"fightId,omitempty"`
	EventID     string `json:"eventId,omitempty"`
	InputHash   string `json:"inputHash,omitempty"`
	StartedAtMS int64  `json:"startedAtMs,omitempty"`
	EndsAtMS    int64  `json:"endsAtMs,omitempty"`
}

// GGOFieldBossBattleReceipt preserves the exact native end reply and input binding.
type GGOFieldBossBattleReceipt struct {
	State            string   `json:"status"` // unknown, active or settled.
	FightID          string   `json:"fightId"`
	EventID          string   `json:"eventId,omitempty"`
	InputHash        string   `json:"inputHash,omitempty"`
	EndRequestHash   string   `json:"endRequestHash,omitempty"`
	NativeEnd        []byte   `json:"nativeEnd,omitempty"` // Original 30086 Protobuf bytes, base64 in JSON.
	Damage           float64  `json:"damage"`
	Lost             [6]int64 `json:"lost"`
	TerminalRevision int64    `json:"terminalRevision"`
}
