package model

import protocol "hyjs-server/model/protocol"

// GGOFieldAuth identifies the logged-in player and the current 16-byte session.
type GGOFieldAuth struct {
	PlayerID   int64  `json:"playerId" binding:"required"`
	SessionHex string `json:"sessionHex" binding:"required"`
}

type GGOFieldSnapshotRequest struct{ GGOFieldAuth }
type GGOFieldMutation struct {
	GGOFieldAuth
	RequestID string `json:"requestId" binding:"required"`
}
type GGOFieldGatherStartRequest struct {
	GGOFieldMutation
	PointID    string `json:"pointId" binding:"required"`
	Generation int64  `json:"generation" binding:"required"`
}
type GGOFieldGatherCancelRequest struct {
	GGOFieldMutation
	StartRequestID string `json:"startRequestId" binding:"required"`
	AttemptToken   string `json:"attemptToken,omitempty"`
}
type GGOFieldGatherClaimRequest struct {
	GGOFieldMutation
	StartRequestID string `json:"startRequestId" binding:"required"`
	AttemptToken   string `json:"attemptToken" binding:"required"`
}
type GGOFieldGatherStatusRequest struct {
	GGOFieldAuth
	StartRequestID string `json:"startRequestId" binding:"required"`
}
type GGOFieldProtectionReleaseRequest struct{ GGOFieldMutation }
type GGOFieldEntryRequest struct{ GGOFieldMutation }
type GGOFieldSettlementRequest struct{ GGOFieldMutation }
type GGOFieldExtractionStartRequest struct {
	GGOFieldMutation
	PointID string `json:"pointId" binding:"required"`
}
type GGOFieldExtractionCancelRequest struct {
	GGOFieldMutation
	StartRequestID string `json:"startRequestId" binding:"required"`
}
type GGOFieldExtractionStatusRequest struct {
	GGOFieldAuth
	StartRequestID string `json:"startRequestId" binding:"required"`
}

type GGOFieldLeavePrepareRequest struct {
	GGOFieldMutation
	Mode        string `json:"mode" binding:"required"`
	TargetScene string `json:"targetScene,omitempty"`
	TargetLine  int32  `json:"targetLine,omitempty"`
}
type GGOFieldLeaveStatusRequest struct {
	GGOFieldAuth
	LeaveRequestID string `json:"leaveRequestId" binding:"required"`
}

type GGOFieldTerminalView struct {
	Chips          [6]int64 `json:"chips"`
	Protected      [6]int64 `json:"protected"`
	ExchangeValues [6]int64 `json:"exchangeValues"`
	Revision       int64    `json:"revision"`
	CreditBalance  int64    `json:"creditBalance"`
}

// GGOFieldChipCountView is the public carried total for a visible player.
type GGOFieldChipCountView struct {
	PlayerID int64 `json:"playerId"`
	Count    int64 `json:"count"`
}

type GGOFieldResourceView struct {
	PointID       string                 `json:"pointId"`
	Kind          string                 `json:"kind"`
	Level         int64                  `json:"level"`
	Remaining     int64                  `json:"remaining"`
	Generation    int64                  `json:"generation"`
	Position      *AdminFieldGGOPosition `json:"position"`
	StandPosition *AdminFieldGGOPosition `json:"standPosition"`
	Radius        float64                `json:"radius"`
	RefreshAtMS   int64                  `json:"refreshAtMs"`
}

type GGOFieldMonsterView struct {
	UUID        string                 `json:"uuid"`
	MonID       string                 `json:"monId"`
	PointID     string                 `json:"pointId"`
	Level       int64                  `json:"level"`
	HeroID      string                 `json:"heroId"`
	Generation  int64                  `json:"generation"`
	Position    *AdminFieldGGOPosition `json:"position"`
	HPRatio     float64                `json:"hpRatio"`
	Available   bool                   `json:"available"`
	RefreshAtMS int64                  `json:"refreshAtMs"`
	Patrol      *GGOFieldPatrolView    `json:"patrol,omitempty"`
}
type GGOFieldPatrolView struct {
	Waypoints  []*AdminFieldGGOPosition `json:"waypoints"`
	Speed      float64                  `json:"speed"`
	EpochMS    int64                    `json:"epochMs"`
	PausedAtMS int64                    `json:"pausedAtMs"`
}
type GGOFieldBossView struct {
	HeroID      string                 `json:"heroId"`
	BossID      string                 `json:"bossId"`
	Nature      int64                  `json:"nature"`
	Level       int64                  `json:"level"`
	WarningAtMS int64                  `json:"warningAtMs"`
	Radius      float64                `json:"radius"`
	PointID     string                 `json:"pointId"`
	EventID     string                 `json:"eventId"`
	Position    *AdminFieldGGOPosition `json:"position"`
	HP          int64                  `json:"hp"`
	MaxHP       int64                  `json:"maxHp"`
	StartsAtMS  int64                  `json:"startsAtMs"`
	EndsAtMS    int64                  `json:"endsAtMs"`
	State       string                 `json:"state"`
}
type GGOFieldNPCView struct {
	ID                  string                 `json:"id"`
	Name                string                 `json:"name"`
	HeroID              string                 `json:"heroId"`
	Role                string                 `json:"role"`
	Position            *AdminFieldGGOPosition `json:"position"`
	InteractionPosition *AdminFieldGGOPosition `json:"interactionPosition"`
	Radius              float64                `json:"radius"`
	PointID             string                 `json:"pointId,omitempty"`
}
type GGOFieldExitView struct {
	PointID  string                 `json:"pointId"`
	Position *AdminFieldGGOPosition `json:"position"`
	Radius   float64                `json:"radius"`
}
type GGOFieldSnapshotResponse struct {
	ServerTimeMS      int64                    `json:"serverTimeMs"`
	Scene             string                   `json:"scene"`
	Line              int64                    `json:"line"`
	EconomyVersion    string                   `json:"economyVersion"`
	Revision          int64                    `json:"revision"`
	ResourceMineCount int64                    `json:"resourceMineCount"`
	ResourceFishCount int64                    `json:"resourceFishCount"`
	MonsterCount      int64                    `json:"monsterCount"`
	Safe              bool                     `json:"safe"`
	ProtectionUntilMS int64                    `json:"protectionUntilMs"`
	Fatigue           int64                    `json:"fatigue"`
	Materials         []GGOFieldResourceAmount `json:"materials"`
	Terminal          GGOFieldTerminalView     `json:"terminal"`
	PlayerChipCounts  []GGOFieldChipCountView  `json:"playerChipCounts"`
	NPCs              []GGOFieldNPCView        `json:"npcs"`
	SafeArea          *AdminFieldGGOSafeArea   `json:"safeArea"`
	Resources         []GGOFieldResourceView   `json:"resources"`
	Monsters          []GGOFieldMonsterView    `json:"monsters"`
	Boss              *GGOFieldBossView        `json:"boss"`
	Exits             []GGOFieldExitView       `json:"exits"`
}
type GGOFieldGatherStartResponse struct {
	RequestID    string `json:"requestId"`
	AttemptToken string `json:"attemptToken"`
	PointID      string `json:"pointId"`
	Generation   int64  `json:"generation"`
	AcceptedAtMS int64  `json:"acceptedAtMs"`
	ReadyAtMS    int64  `json:"readyAtMs"`
	FatigueCost  int64  `json:"fatigueCost"`
}
type GGOFieldResourceAmount struct {
	ID     string `json:"id"`
	Amount int64  `json:"amount"`
}
type GGOFieldGatherClaimResponse struct {
	RequestID        string                   `json:"requestId"`
	StartRequestID   string                   `json:"startRequestId"`
	AttemptToken     string                   `json:"attemptToken"`
	Resources        []GGOFieldResourceAmount `json:"resources"`
	Chips            [6]int64                 `json:"chips"`
	TerminalRevision int64                    `json:"terminalRevision"`
}
type GGOFieldGatherCancelResponse struct {
	RequestID      string                       `json:"requestId"`
	StartRequestID string                       `json:"startRequestId"`
	Status         string                       `json:"status"`
	AttemptToken   string                       `json:"attemptToken,omitempty"`
	ClaimReceipt   *GGOFieldGatherClaimResponse `json:"claimReceipt,omitempty"`
}
type GGOFieldGatherStatusResponse struct {
	StartRequestID string                       `json:"startRequestId"`
	Status         string                       `json:"status"`
	AttemptToken   string                       `json:"attemptToken,omitempty"`
	AcceptedAtMS   int64                        `json:"acceptedAtMs,omitempty"`
	ReadyAtMS      int64                        `json:"readyAtMs,omitempty"`
	FatigueCost    int64                        `json:"fatigueCost,omitempty"`
	ClaimReceipt   *GGOFieldGatherClaimResponse `json:"claimReceipt,omitempty"`
}
type GGOFieldProtectionReleaseResponse struct {
	RequestID         string `json:"requestId"`
	ServerTimeMS      int64  `json:"serverTimeMs"`
	ProtectionUntilMS int64  `json:"protectionUntilMs"`
	Released          bool   `json:"released"`
}

// GGOFieldEntryResponse records one NPC teleport; current position is still
// confirmed by the original CSCurrentPosition scene handshake.
type GGOFieldEntryResponse struct {
	RequestID         string                 `json:"requestId"`
	AcceptedAtMS      int64                  `json:"acceptedAtMs"`
	ProtectionUntilMS int64                  `json:"protectionUntilMs"`
	Position          *AdminFieldGGOPosition `json:"position"`
	PositionRevision  int64                  `json:"positionRevision"`
}

type GGOFieldSettlementResponse struct {
	RequestID        string   `json:"requestId"`
	EconomyVersion   string   `json:"economyVersion"`
	Chips            [6]int64 `json:"chips"`
	CreditAdded      int64    `json:"creditAdded"`
	CreditBalance    int64    `json:"creditBalance"`
	TerminalRevision int64    `json:"terminalRevision"`
}

type GGOFieldLeavePrepareResponse struct {
	RequestID   string `json:"requestId"`
	Status      string `json:"status"`
	Mode        string `json:"mode"`
	TargetScene string `json:"targetScene,omitempty"`
	TargetLine  int32  `json:"targetLine,omitempty"`
	ExpiresAtMS int64  `json:"expiresAtMs"`
}
type GGOFieldLeaveStatusResponse struct {
	RequestID         string                      `json:"requestId"`
	Status            string                      `json:"status"`
	Mode              string                      `json:"mode,omitempty"`
	TargetScene       string                      `json:"targetScene,omitempty"`
	TargetLine        int32                       `json:"targetLine,omitempty"`
	ExpiresAtMS       int64                       `json:"expiresAtMs,omitempty"`
	Settlement        *GGOFieldSettlementResponse `json:"settlement,omitempty"`
	NativeRequestHash string                      `json:"nativeRequestHash,omitempty"`
	NativeResponse    []byte                      `json:"nativeResponse"`
	PositionRevision  int64                       `json:"positionRevision,omitempty"`
	TargetCurrent     bool                        `json:"targetCurrent"`
}

type GGOFieldExtractionStartResponse struct {
	RequestID    string `json:"requestId"`
	PointID      string `json:"pointId"`
	AcceptedAtMS int64  `json:"acceptedAtMs"`
	ReadyAtMS    int64  `json:"readyAtMs"`
}
type GGOFieldExtractionStatusResponse struct {
	RequestID        string                 `json:"requestId"`
	StartRequestID   string                 `json:"startRequestId"`
	Status           string                 `json:"status"`
	Reason           string                 `json:"reason,omitempty"`
	AcceptedAtMS     int64                  `json:"acceptedAtMs,omitempty"`
	ReadyAtMS        int64                  `json:"readyAtMs,omitempty"`
	CompletedAtMS    int64                  `json:"completedAtMs,omitempty"`
	PositionRevision int64                  `json:"positionRevision,omitempty"`
	Position         *AdminFieldGGOPosition `json:"position,omitempty"`
}

// PVPStatus recovers the original 30046 by the positive HTTP stamp chosen
// before sending. Unknown never proves the request cannot still arrive.
type GGOFieldPVPStatusRequest struct {
	GGOFieldAuth
	Stamp         int64  `json:"stamp" binding:"required"`
	NativeRequest []byte `json:"nativeRequest" binding:"required"`
}
type GGOFieldPVPStatusResponse struct {
	Status              string `json:"status"`
	Stamp               int64  `json:"stamp"`
	OpponentID          int64  `json:"opponentId"`
	RequestHash         string `json:"requestHash"`
	FightID             string `json:"fightId,omitempty"`
	WinnerID            int64  `json:"winnerId,omitempty"`
	NativeResponse      []byte `json:"nativeResponse,omitempty"`
	NativeState         []byte `json:"nativeState,omitempty"`
	ServerTimeMS        int64  `json:"serverTimeMs"`
	PresentationUntilMS int64  `json:"presentationUntilMs,omitempty"`
	PresentationCurrent bool   `json:"presentationCurrent"`
	PositionRevision    int64  `json:"positionRevision,omitempty"`
}

type GGOFieldPVPReceiptRequest struct {
	GGOFieldAuth
	FightID string `json:"fightId" binding:"required"`
}
type GGOFieldPVPReceipt struct {
	FightID          string   `json:"fightId"`
	WinnerID         int64    `json:"winnerId"`
	LoserID          int64    `json:"loserId"`
	Lost             [6]int64 `json:"lost"`
	TerminalRevision int64    `json:"terminalRevision"`
	CreditBalance    int64    `json:"creditBalance"`
}

type GGOFieldMonsterReceiptRequest struct {
	GGOFieldAuth
	FightID string `json:"fightId" binding:"required"`
}
type GGOFieldMonsterClaimRequest struct {
	GGOFieldMutation
	FightID string `json:"fightId" binding:"required"`
}
type GGOFieldMonsterReceipt struct {
	RequestID        string                   `json:"requestId,omitempty"`
	FightID          string                   `json:"fightId"`
	Status           string                   `json:"status"`
	DeliveryStatus   string                   `json:"deliveryStatus,omitempty"`
	InputHash        string                   `json:"inputHash,omitempty"`
	EndRequestHash   string                   `json:"endRequestHash,omitempty"`
	NativeEnd        []byte                   `json:"nativeEnd,omitempty"`
	Resources        []GGOFieldResourceAmount `json:"resources"`
	Warriors         []*protocol.Warrior      `json:"warriors"`
	Lost             [6]int64                 `json:"lost"`
	Chips            [6]int64                 `json:"chips"`
	TerminalRevision int64                    `json:"terminalRevision"`
	CreditBalance    int64                    `json:"creditBalance"`
}

type GGOFieldMonsterActiveResponse struct {
	Active          bool   `json:"active"`
	FightID         string `json:"fightId,omitempty"`
	InputHash       string `json:"inputHash,omitempty"`
	MonsterUUID     string `json:"monsterUuid,omitempty"`
	PointID         string `json:"pointId,omitempty"`
	Generation      int64  `json:"generation,omitempty"`
	StartedAtMS     int64  `json:"startedAtMs,omitempty"`
	ReservedUntilMS int64  `json:"reservedUntilMs,omitempty"`
}

// GGOFieldPositionStatusResponse ties the original position handshake to the
// authenticated session and the latest server-side forced position change.
type GGOFieldPositionStatusResponse struct {
	Scene             string                 `json:"scene"`
	Position          *AdminFieldGGOPosition `json:"position"`
	PositionRevision  int64                  `json:"positionRevision"`
	ConfirmedRevision int64                  `json:"confirmedRevision"`
	Loading           bool                   `json:"loading"`
}
