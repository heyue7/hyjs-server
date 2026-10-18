package model

// P6MechanismQualification identifies admitted original mechanism branches.
// It records executable code coverage independently from a mode's native certificate.
type P6MechanismQualification struct {
	// EngineVersion identifies the unchanged calculation kernel.
	EngineVersion string `json:"engineVersion"`
	// ConfigurationHash binds every frozen original table row.
	ConfigurationHash string `json:"configurationHash"`
	// MechanismHash binds sorted mechanism families and their trigger shapes.
	MechanismHash string `json:"mechanismHash"`
	// ActorCount includes every source actor without an actor ID allowlist.
	ActorCount int `json:"actorCount"`
	// SkillIDs retains all actual ordinary, ultimate and awakening slots.
	SkillIDs []string `json:"skillIds"`
	// TalentIDs retains actual hero and guarding-pet talent sources.
	TalentIDs []string `json:"talentIds"`
	// BuffIDs retains direct and recursively referenced original factory rows.
	BuffIDs []string `json:"buffIds"`
	// Mechanisms lists handlers and original timer/selector/condition branches.
	Mechanisms []string `json:"mechanisms"`
	// NativeCertified remains false until a reviewed mode certificate consumes this scope.
	NativeCertified bool `json:"nativeCertified"`
}
