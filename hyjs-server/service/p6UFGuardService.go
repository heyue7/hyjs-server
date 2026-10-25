package service

import (
	"context"
	"encoding/json"
	"errors"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	"math"
	"strings"
)

// validateP6UFGuardCandidate validates the current exported input family only.
// It grants no authority and deliberately excludes future wounded/AP continuations.
func validateP6UFGuardCandidate(input *model.FrozenBattleInput) error {
	reject := func() error { return errors.New(constUtil.CombatUnsupported) }
	if input == nil || input.Attacker == nil || input.Monster == nil || input.Defender != nil || input.BattleType != 14 || input.IsAttToDef || input.DefenderFirst || input.MaxRound != 40 || input.Seed < 0 || input.EngineVersion != constUtil.P6EngineVersion || input.ClientVersion != constUtil.P6OriginalClientHash || input.KFReplay != nil || len(input.WinConditions) != 0 || input.DiagnosticAIDriverPolicy != "" || input.PVECompatibilityRule != "" || input.DiagnosticManualAutoStartRoundTime != nil {
		return reject()
	}
	if input.AttackerDamageFactor == nil || input.DefenderDamageFactor == nil || *input.AttackerDamageFactor != 1 || *input.DefenderDamageFactor != 1 || input.Attacker.Inst == nil || input.Monster.Inst == nil || input.Attacker.Inst.GetAnger() != 0 || input.Monster.Inst.GetAnger() != 0 || len(input.Attacker.Formation) != 1 || len(input.Monster.Formation) != 1 || len(input.Attacker.Inst.ExtBattleAdds) != 0 {
		return reject()
	}
	guardIDs := map[string]string{"tower_guard_001": "guard01_npc", "tower_guard_002": "guard01_npc", "tower_guard_003": "monster_016", "tower_guard_004": "monster_016", "tower_guard_005": "guard01_npc", "tower_guard_006": "monster_016", "factory_guard_001": "monster_016", "factory_guard_002": "guard01_npc", "baseCamp_guard_001": "hero_002", "baseCamp_guard_002": "hero_013"}
	a, g := input.Attacker.Formation[0], input.Monster.Formation[0]
	if a == nil || g == nil || a.Prop == nil || g.Prop == nil || a.Inst == nil || g.Inst == nil || a.GetId() != "hero_047" || g.GetId() != guardIDs[input.Monster.GetTaskId()] || !strings.HasPrefix(g.GetGuid(), "guard/") || a.GetPose() != 3 || g.GetPose() != 3 || a.GetBloodNumber() != 1 || g.GetBloodNumber() != 1 || a.GetInst().GetCurHp() != a.GetProp().GetHp() || a.GetProp().GetHp() != 2054 || g.GetInst().GetCurHp() != 1000 || g.GetProp().GetHp() != 1000 {
		return reject()
	}
	return validateFrozenP6(input)
}

// ComputeP6UFGuardSettlement is closed until native proof and its exact scope
// are separately approved. No audit output acquires an authority bit here.
func (s *GameService) ComputeP6UFGuardSettlement(ctx context.Context, input *model.FrozenBattleInput) (*model.CombatFinalHPOutcome, error) {
	if err := validateP6UFGuardCandidate(input); err != nil {
		return nil, err
	}
	if constUtil.P6UFGuardNativeProof == "" {
		return nil, errors.New(constUtil.P13UFGuardProofMissing)
	}
	return nil, errors.New(constUtil.P13UFGuardProofMissing)
}

// validateP6UFGuardTerminal checks a future independently certified terminal.
// It never upgrades a diagnostic result or borrows a player-only proof.
func validateP6UFGuardTerminal(input *model.FrozenBattleInput, out *model.CombatFinalHPOutcome) error {
	if err := validateP6UFGuardCandidate(input); err != nil {
		return err
	}
	if constUtil.P6UFGuardNativeProof == "" || out == nil || out.Outcome == nil || !out.Outcome.Authoritative || out.Outcome.CapabilityHash != constUtil.P6UFGuardNativeProof {
		return errors.New(constUtil.P13UFGuardProofMissing)
	}
	if out.Schema != constUtil.P6FinalActorHPSchema || out.EngineVersion != input.EngineVersion || out.ClientVersion != input.ClientVersion || out.ConfigurationHash != input.ConfigurationHash || out.Outcome.Winner < 0 || out.Outcome.Winner > 1 || out.Outcome.Attacker == nil || out.Outcome.Monster == nil || out.Outcome.Defender != nil {
		return errors.New(constUtil.P13UFGuardConflict)
	}
	closure := *input
	closure.Tables = nil
	raw, err := json.Marshal(&closure)
	if err != nil || p6Digest(raw) != out.CalculationSourceSHA256 {
		return errors.New(constUtil.P13UFGuardConflict)
	}
	if len(out.FinalActors) != len(input.Attacker.Formation)+len(input.Monster.Formation) {
		return errors.New(constUtil.P13UFGuardConflict)
	}
	seen := map[string]bool{}
	for _, actor := range out.FinalActors {
		camp := actor.Camp
		if camp != 0 && camp != 1 {
			return errors.New(constUtil.P13UFGuardConflict)
		}
		initial := input.Attacker.Formation[0]
		if camp == 1 {
			initial = input.Monster.Formation[0]
		}
		encoded, _ := proto.MarshalOptions{Deterministic: true}.Marshal(initial)
		key := string(rune(camp)) + actor.GUID
		if seen[key] || actor.GUID != initial.GetGuid() || actor.HeroID != initial.GetId() || actor.Pose != initial.GetPose() || actor.BloodNumber != 1 || actor.InitialActorSHA256 != p6Digest(encoded) || !combatFinite(actor.CurrentHP) || !combatFinite(actor.MaxHP) || actor.MaxHP <= 0 || actor.CurrentHP < 0 || actor.CurrentHP > actor.MaxHP || actor.MaxHP != initial.GetProp().GetHp() {
			return errors.New(constUtil.P13UFGuardConflict)
		}
		seen[key] = true
	}
	for _, ap := range []float64{out.Outcome.Attacker.GetInst().GetAnger(), out.Outcome.Monster.GetInst().GetAnger()} {
		if math.IsNaN(ap) || math.IsInf(ap, 0) || ap < 0 || ap > 300 {
			return errors.New(constUtil.P13UFGuardConflict)
		}
	}
	return nil
}
