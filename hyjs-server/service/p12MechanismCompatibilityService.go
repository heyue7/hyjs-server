package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p12MechanismIdentity(mode int32) (proof, driver string) {
	switch mode {
	case 18:
		return constUtil.P12WorldBossMechanismProof, "local-original-worldboss18-auto-mechanism-v1"
	case 8:
		return constUtil.P12WantedMechanismProof, "local-original-wanted8-auto-mechanism-v1"
	case 5:
		return constUtil.P12FieldPVPMechanismProof, "local-original-field5-two-player-ai-mechanism-v1"
	}
	return "", ""
}

// Mode semantics stay typed and independently bounded. No eligibility view is
// sent through type11 and no actor panel, current HP, AP or first-order is reset.
func p12MechanismScope(input *model.FrozenBattleInput, mode int32) error {
	reject := func() error { return errors.New(constUtil.CombatUnsupported) }
	proof, _ := p12MechanismIdentityForInput(input)
	maximum, configuration := int32(40), constUtil.P6LinearPhysicalTables
	if mode == 8 {
		maximum, configuration = 20, constUtil.P12WantedConfigurationHash
	}
	if input == nil || len(proof) != 64 || input.BattleType != mode || input.MaxRound != maximum || input.EngineVersion != constUtil.P6EngineVersion || input.ClientVersion != constUtil.P6OriginalClientHash || input.ConfigurationHash != configuration || input.Seed < 0 || input.Seed > math.MaxInt32 || input.KFReplay != nil || input.KFReplayViewer != nil || input.IsAttToDef || len(input.WinConditions) != 0 || input.PVECompatibilityRule != "" || input.DiagnosticAIDriverPolicy != "" || input.DiagnosticManualAutoStartRoundTime != nil {
		return reject()
	}
	if input.AttackerDamageFactor == nil || input.DefenderDamageFactor == nil || *input.DefenderDamageFactor != 1 || (*input.AttackerDamageFactor != 1 && !(mode == 8 && *input.AttackerDamageFactor == 2.5)) {
		return reject()
	}
	players := []*pb.BattlePlayer{input.Attacker}
	if mode == 5 {
		if input.Defender == nil || input.Monster != nil {
			return reject()
		}
		players = append(players, input.Defender)
		if input.Attacker.GetPlayerId() == input.Defender.GetPlayerId() {
			return reject()
		}
	} else {
		if input.Defender != nil || input.Monster == nil || input.Monster.Inst == nil || input.DefenderFirst || input.Monster.Inst.GetAnger() != 0 || len(input.Monster.Formation) != 4 || p6ScopedUnknown(input.Monster) || p6ScopedUnknown(input.Monster.Inst) {
			return reject()
		}
		if mode == 18 {
			if _, err := p12WorldBossPartyBoostMask(input); err != nil {
				return err
			}
			level := input.Monster.Formation[0].GetLevel()
			pins, ok := p12MechanismWorldBossNPCPins[level]
			if !ok {
				return fmt.Errorf("%s: original WorldBoss level %d", constUtil.CombatUnsupported, level)
			}
			for index, actor := range input.Monster.Formation {
				if actor.GetLevel() != level {
					return reject()
				}
				raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(actor)
				if err != nil || p6Digest(raw) != pins[index] {
					return reject()
				}
			}
		} else if mode == 8 && p12WantedCatalogMarker(input) {
			if err := validateP12WantedCatalogNPCs(input); err != nil {
				return err
			}
		} else if mode == 8 {
			pins := []string{"b08a58e43afd108996b4bfd1aa964742f712eb54cb7c402c363d04dc529d47de", "fe4b615600ea0dc06baccd0b7168f39e50751a13b93f1dab24c0242c9b4f62e3", "6f0a21e225e89a0fce49f9b951eeb4bbd44815e26883f9a1f540225968dca963", "fbc37eb5b74a85d84e043644049ef87494fa4fcf68b6c075a32e6a94f889ecc0"}
			living := false
			for index, actor := range input.Monster.Formation {
				if actor == nil || actor.Prop == nil || actor.Inst == nil || !combatFinite(actor.Inst.GetCurHp()) || actor.Inst.GetCurHp() < 0 || actor.Inst.GetCurHp() > actor.Prop.GetHp() {
					return reject()
				}
				living = living || actor.Inst.GetCurHp() > 0
				copy := proto.Clone(actor).(*pb.BattleHero)
				copy.Inst.CurHp = proto.Float64(copy.Prop.GetHp())
				raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(copy)
				if err != nil || p6Digest(raw) != pins[index] {
					return reject()
				}
			}
			if !living {
				return reject()
			}
		}
	}
	maxHeroes := 4
	if mode == 8 {
		maxHeroes = constUtil.P12WantedMaxHeroes
	}
	if mode == 18 {
		maxHeroes = constUtil.P12WorldBossMaxHeroes
	}
	if mode == 5 {
		maxHeroes = constUtil.P12FieldPVPMaxHeroes
	}
	for _, party := range players {
		if party == nil || party.Inst == nil || party.GetPlayerId() <= 0 || party.GetServerId() <= 0 || len(party.Formation) < 1 || len(party.Formation) > maxHeroes || len(party.Inst.ExtBattleAdds) != 0 || !combatFinite(party.Inst.GetAnger()) || party.Inst.GetAnger() < 0 || party.Inst.GetAnger() > 300 || p6ScopedUnknown(party) || p6ScopedUnknown(party.Inst) {
			return reject()
		}
		if mode != 5 && party.Inst.GetFirstOrder() != 0 {
			return reject()
		}
		seen, poses, living := map[string]bool{}, map[int32]bool{}, false
		for _, actor := range party.Formation {
			if actor == nil || actor.Prop == nil || actor.Inst == nil || actor.GetGuid() == "" || len(actor.GetGuid()) > 128 || strings.Contains(actor.GetGuid(), "_") || seen[actor.GetGuid()] || actor.GetPose() < 1 || actor.GetPose() > 15 || poses[actor.GetPose()] || !combatFinite(actor.Inst.GetCurHp()) || actor.Inst.GetCurHp() < 0 || actor.Inst.GetCurHp() > actor.Prop.GetHp() || actor.Prop.GetHp() <= 0 || p6ScopedUnknown(actor) || p6ScopedUnknown(actor.Prop) || p6ScopedUnknown(actor.Inst) {
				return reject()
			}
			var row map[string]string
			for _, candidate := range input.Tables["Hero"] {
				if candidate["id"] == actor.GetId() {
					row = candidate
					break
				}
			}
			if row == nil || !strings.HasPrefix(actor.GetId(), "hero_") || row["heroClass"] == "99" {
				return reject()
			}
			normal := strings.Join([]string{row["SkillId1"], row["SkillId2"], row["UltSkillId"]}, ",")
			swapped := strings.Join([]string{row["SkillId2"], row["SkillId1"], row["UltSkillId"]}, ",")
			if actor.GetSkillOrder() != normal && actor.GetSkillOrder() != swapped {
				return fmt.Errorf("%s: actor %s original skill ownership", constUtil.CombatUnsupported, actor.GetId())
			}
			seen[actor.GetGuid()], poses[actor.GetPose()] = true, true
			living = living || actor.Inst.GetCurHp() > 0
		}
		if !living {
			return reject()
		}
	}
	if err := ValidateP6FirstOrder(input); err != nil {
		return err
	}
	_, err := (&GameService{Tables: input.Tables}).ValidateP6GenericMechanisms(context.Background(), input)
	return err
}

// The local compatibility proof and exact typed driver are closed separately
// from the unchanged original calculation input, preventing cross-family stamps.
func p12MechanismSourceDigest(input *model.FrozenBattleInput) (string, error) {
	proof, driver := p12MechanismIdentityForInput(input)
	view := *input
	view.Tables = nil
	raw, err := json.Marshal(&view)
	if err != nil {
		return "", err
	}
	bound, err := json.Marshal(struct {
		Input  string `json:"frozenInput"`
		Proof  string `json:"localCompatibilityProof"`
		Driver string `json:"originalTypedDriver"`
	}{p6Digest(raw), proof, driver})
	if err != nil {
		return "", err
	}
	return p6Digest(bound), nil
}

func (s *GameService) computeP12MechanismFinalHP(ctx context.Context, input *model.FrozenBattleInput, mode int32) (*model.CombatFinalHPOutcome, error) {
	if err := p12MechanismScope(input, mode); err != nil {
		return nil, err
	}
	// Existing original typed18/type8/type5 wrapper, not the generic type11 gate.
	out, err := s.computeFrozenBattleFinalHP(ctx, input, nil, true)
	if err != nil {
		return nil, err
	}
	out.CalculationSourceSHA256, err = p12MechanismSourceDigest(input)
	if err != nil {
		return nil, err
	}
	proof, _ := p12MechanismIdentityForInput(input)
	out.Outcome.Authoritative = true
	out.Outcome.CapabilityHash = proof
	if err = p12MechanismTerminal(input, out, mode); err != nil {
		return nil, err
	}
	return out, nil
}

func p12MechanismTerminal(input *model.FrozenBattleInput, out *model.CombatFinalHPOutcome, mode int32) error {
	if err := p12MechanismScope(input, mode); err != nil {
		return err
	}
	proof, _ := p12MechanismIdentityForInput(input)
	if out == nil || out.Outcome == nil || !out.Outcome.Authoritative || out.Outcome.CapabilityHash != proof || out.Schema != constUtil.P6FinalActorHPSchema || out.EngineVersion != input.EngineVersion || out.ClientVersion != input.ClientVersion || out.ConfigurationHash != input.ConfigurationHash || out.OriginalRecordSHA256 != "" || out.OriginalOperates == nil || out.OriginalOperates.AttackerTransOpt != nil || out.OriginalOperates.DefenderTransOpt != nil || out.Outcome.DiagnosticAIDriverPolicy != "" {
		return errors.New(constUtil.P6ReplayConflict)
	}
	if mode == 5 && len(out.OriginalOperates.SkillOpts) != 0 {
		return errors.New(constUtil.P6ReplayConflict)
	}
	if err := validateP6GenericMechanismTerminal(input, out); err != nil {
		return err
	}
	source, err := p12MechanismSourceDigest(input)
	if err != nil || source != out.CalculationSourceSHA256 {
		return errors.New(constUtil.P6ReplayConflict)
	}
	finals := map[string]model.CombatFinalActorHP{}
	for _, actor := range out.FinalActors {
		if actor.RuntimePose != 0 {
			return errors.New(constUtil.P6ReplayConflict)
		}
		finals[actor.UID] = actor
	}
	// All terminal HP slots and the AP subsequently persisted to field state must
	// belong to the same original runtime. Original properties/identity stay frozen.
	for camp := 0; camp < 2; camp++ {
		var sourcePlayer, targetPlayer *pb.BattlePlayer
		if camp == 0 {
			sourcePlayer, targetPlayer = input.Attacker, out.Outcome.Attacker
		} else if mode == 5 {
			sourcePlayer, targetPlayer = input.Defender, out.Outcome.Defender
		}
		if sourcePlayer != nil {
			if targetPlayer == nil || targetPlayer.Inst == nil || !combatFinite(targetPlayer.Inst.GetAnger()) || targetPlayer.Inst.GetAnger() < 0 || targetPlayer.Inst.GetAnger() > 300 {
				return errors.New(constUtil.P6ReplayConflict)
			}
			copy := proto.Clone(sourcePlayer).(*pb.BattlePlayer)
			copy.Inst.Anger = proto.Float64(targetPlayer.Inst.GetAnger())
			for _, actor := range copy.Formation {
				end, ok := finals[fmt.Sprintf("%d_%s", camp, actor.GetGuid())]
				if !ok || end.BloodNumber != actor.GetBloodNumber() {
					return errors.New(constUtil.P6ReplayConflict)
				}
				actor.Inst.CurHp = proto.Float64(end.CurrentHP)
			}
			if !proto.Equal(copy, targetPlayer) {
				return errors.New(constUtil.P6ReplayConflict)
			}
		} else {
			if out.Outcome.Monster == nil || out.Outcome.Monster.Inst == nil || !combatFinite(out.Outcome.Monster.Inst.GetAnger()) || out.Outcome.Monster.Inst.GetAnger() < 0 || out.Outcome.Monster.Inst.GetAnger() > 300 {
				return errors.New(constUtil.P6ReplayConflict)
			}
			copy := proto.Clone(input.Monster).(*pb.BattleMonster)
			copy.Inst.Anger = proto.Float64(out.Outcome.Monster.Inst.GetAnger())
			for _, actor := range copy.Formation {
				end, ok := finals[fmt.Sprintf("1_%s", actor.GetGuid())]
				if !ok || end.BloodNumber != actor.GetBloodNumber() {
					return errors.New(constUtil.P6ReplayConflict)
				}
				actor.Inst.CurHp = proto.Float64(end.CurrentHP)
			}
			if !proto.Equal(copy, out.Outcome.Monster) {
				return errors.New(constUtil.P6ReplayConflict)
			}
		}
	}
	return nil
}
