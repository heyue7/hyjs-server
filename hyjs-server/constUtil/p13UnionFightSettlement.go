package constUtil

const P13UnionFightSettlementSource = "local-published-zero-fight-settlement-v1: original timed zero-kill/simultaneous tie lower frozen total power; no combat intent/audit/P6/current HP change; explicit archived/local reward publication; original day2330 inbox; equal power/withdrawal/absent side unresolved"

const (
	P13UnionFightTransportSource  = "local-transport-mvp-v1: actual deposited ore since new field creation, independent of spent tokens/camp ore; unique positive maximum only, tied/zero or legacy partial histories receive no transport MVP; original prefab bestPerson2, one prepublished local MVP award, no combat authority"
	P13UnionFightTransportMVPSlot = int32(2)
)

// Personal mission kinds are the named local catalog, never recovered private IDs.
const (
	P13UnionFightMissionGather    = int32(1)
	P13UnionFightMissionTransport = int32(2)
	P13UnionFightMissionSource    = "local-published-natural-missions-v1: original task lists/score push/award slot3; original text keys, local kind/threshold/points; actual completed ore tasks and cumulative deposits only; frozen before match; no combat authority"
)

// DueAt orders committed five-second combat effects regardless of worker delay.
const P13UnionFightCombatSettlementSource = "local-certified-player-combat-settlement-v1: original kills/first-reaching/lower-frozen-power; local persisted DueAt effect order; complete certified ordinary14 ledger only; frozen published awards; unresolved exact tie/audit/pending/unknown changes"

const P13UFCombatProofSchema = "p13-certified-combat-terminal-proof-v1"
