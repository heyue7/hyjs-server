package model

// WarriorBreachPity stores the accumulated probability phase in 0.0001% units.
// Presence marks an initialized phase, including a legitimate zero value.
type WarriorBreachPity struct {
	Phase int64 `json:"phase"`
}
