package model

import pb "hyjs-server/model/protocol"

// CombatOriginalOperateOutcome retains the complete calculation audit and the
// exact original BattleNetOver record with original record-container boundaries.
type CombatOriginalOperateOutcome struct {
	Schema           string            `json:"schema"`
	RecordAttacker   bool              `json:"recordAttacker"`
	RecordDefender   bool              `json:"recordDefender"`
	OriginalOperates *pb.BattleOperate `json:"originalOperates"`
	// SuppliedOriginalOperates remains byte-semantic source. Original PlaybackAI
	// executes at most one queued message per Tick, so a rerecord can shift a
	// simultaneous recorded press by one frame without changing this source.
	SuppliedOriginalOperates *pb.BattleOperate `json:"suppliedOriginalOperates,omitempty"`
	Outcome                  *CombatOutcome    `json:"outcome"`
}
