package model

// GGOFieldBattleResumeRequest identifies one previously accepted desert battle.
type GGOFieldBattleResumeRequest struct {
	GGOFieldAuth
	// BattleType is 6 for a monster or 12 for a boss.
	BattleType int64 `json:"battleType" binding:"required"`
	// ExpectedFightID is the original 24-character lowercase hexadecimal ID.
	ExpectedFightID string `json:"expectedFightId" binding:"required"`
	// ExpectedInputHash is the 64-character lowercase SHA-256 of frozen input.
	ExpectedInputHash string `json:"expectedInputHash" binding:"required"`
}

// GGOFieldBattleResumeResponse binds an original protobuf frame to its fight.
type GGOFieldBattleResumeResponse struct {
	// Status is active or settled.
	Status string `json:"status"`
	// BattleType is 6 for a monster or 12 for a boss.
	BattleType int64 `json:"battleType"`
	// FightID is the requested original fight ID.
	FightID string `json:"fightId"`
	// InputHash is the verified frozen-input SHA-256.
	InputHash string `json:"inputHash"`
	// MonsterUUID identifies an active type-6 target.
	MonsterUUID string `json:"monsterUuid,omitempty"`
	// EventID identifies an active or settled type-12 event.
	EventID string `json:"eventId,omitempty"`
	// NativeStart is the original start protobuf, encoded as base64 in JSON.
	NativeStart []byte `json:"nativeStart,omitempty"`
	// NativeEnd is the immutable original end protobuf, encoded as base64 in JSON.
	NativeEnd []byte `json:"nativeEnd,omitempty"`
	// EndRequestHash binds a settled response to the submitted original end.
	EndRequestHash string `json:"endRequestHash,omitempty"`
}
