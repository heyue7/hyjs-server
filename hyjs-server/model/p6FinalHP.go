package model

import pb "hyjs-server/model/protocol"

// CombatFinalActorHP binds one terminal original runtime actor to its unchanged
// frozen initial actor. Initial zero-HP heroes remain original runtime DEAD actors
// and are included with their exact currentHP0/maxHP and source identity.
// BloodNumber is the original visual-bar field, not a multiplier.
type CombatFinalActorHP struct {
	Camp   int32  `json:"camp"`
	UID    string `json:"uid"`
	GUID   string `json:"guid"`
	HeroID string `json:"heroId"`
	// Pose remains the original frozen participant slot for source HP mapping.
	// RuntimePose is present only when original DoTrans changed MainPosition.
	RuntimePose        int32   `json:"runtimePose,omitempty"`
	Pose               int32   `json:"pose"`
	BloodNumber        int32   `json:"bloodNumber"`
	CurrentHP          float64 `json:"currentHp"`
	MaxHP              float64 `json:"maxHp"`
	State              int32   `json:"state"`
	InitialActorSHA256 string  `json:"initialActorSHA256"`
}

// CombatFinalHPOutcome is opt-in calculation audit. The base original outcome
// and authoritative gate keep their existing contract; this does not apply war HP.
type CombatFinalHPOutcome struct {
	Schema string `json:"schema"`
	// CalculationSourceSHA256 hashes all original input fields, using its checked
	// ConfigurationHash instead of duplicate table rows. It is not the DB InputHash.
	CalculationSourceSHA256 string `json:"calculationSourceSHA256"`
	// OriginalRecordSHA256 binds the actual source command record for the
	// explicitly requested original record driver; empty for the legacy audit.
	OriginalRecordSHA256 string               `json:"originalRecordSHA256,omitempty"`
	ConfigurationHash    string               `json:"configurationHash"`
	EngineVersion        string               `json:"engineVersion"`
	ClientVersion        string               `json:"clientVersion"`
	FinalActors          []CombatFinalActorHP `json:"finalActors"`
	// OriginalOperates is the original final protocol from this same terminal
	// calculation, present only for ComputeOriginalP6ProtocolFinalHP. Outcome.Operates
	// retains the diagnostic complete command stream and has a different contract.
	OriginalOperates *pb.BattleOperate `json:"originalOperates,omitempty"`
	Outcome          *CombatOutcome    `json:"outcome"`
}
