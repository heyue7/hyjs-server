package model

import pb "hyjs-server/model/protocol"

// FrozenP6RushSeries retains the exact SCRushFightStart and original trial/elite
// callback choice. Automatic is an explicit caller choice after the original
// callback initializes the player as manual; it never comes from actor shape.
type FrozenP6RushSeries struct {
	Schema          string               `json:"schema"`
	Start           *pb.SCRushFightStart `json:"start"`
	StartSHA256     string               `json:"startSHA256"`
	Trial           bool                 `json:"trial"`
	PlayerAutomatic bool                 `json:"playerAutomatic"`
	Template        *FrozenBattleInput   `json:"template"`
}

// P6RushSeriesOutcome retains each original fresh pair and the final-only
// original record. Winner remains a computation audit without authority.
type P6RushSeriesOutcome struct {
	Schema        string               `json:"schema"`
	StartSHA256   string               `json:"startSHA256"`
	Winner        int32                `json:"winner"`
	AttackerWave  int64                `json:"attackerWave"`
	DefenderWave  int64                `json:"defenderWave"`
	Fights        []P6CopyFightOutcome `json:"fights"`
	FinalOperates *pb.BattleOperate    `json:"finalOperates"`
	Authoritative bool                 `json:"authoritative"`
}
