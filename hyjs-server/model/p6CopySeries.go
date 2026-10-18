package model

import pb "hyjs-server/model/protocol"

// FrozenP6CopySeries preserves the complete original chapter start and one frozen
// table/parameter closure. Each original monster remains in its original wave order.
type FrozenP6CopySeries struct {
	Schema          string               `json:"schema"`
	Start           *pb.SCCopyFightStart `json:"start"`
	StartSHA256     string               `json:"startSHA256"`
	PlayerAutomatic bool                 `json:"playerAutomatic"`
	Template        *FrozenBattleInput   `json:"template"`
}

// P6CopyFightOutcome is one freshly initialized original GetFighter pair.
type P6CopyFightOutcome struct {
	AttackerWave int64          `json:"attackerWave"`
	DefenderWave int64          `json:"defenderWave"`
	InputSHA256  string         `json:"inputSHA256"`
	Outcome      *CombatOutcome `json:"outcome"`
}

// P6CopySeriesOutcome retains every fight separately and the original last-fight
// operation record. Wave indices never alter original round numbers or protobufs.
type P6CopySeriesOutcome struct {
	Schema        string               `json:"schema"`
	StartSHA256   string               `json:"startSHA256"`
	Winner        int32                `json:"winner"`
	AttackerWave  int64                `json:"attackerWave"`
	DefenderWave  int64                `json:"defenderWave"`
	Fights        []P6CopyFightOutcome `json:"fights"`
	FinalOperates *pb.BattleOperate    `json:"finalOperates"`
	Authoritative bool                 `json:"authoritative"`
}
