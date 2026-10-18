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

// Only actor identity selects this independent family. It never grants proof
// or falls back to the existing001003 certificate.
func p7NaturalRushIdentity(series *model.FrozenP6RushSeries) bool {
	return series != nil && series.Template != nil && series.Template.Attacker != nil &&
		len(series.Template.Attacker.Formation) == 2 &&
		series.Template.Attacker.Formation[0].GetId() == "hero_047" &&
		series.Template.Attacker.Formation[1].GetId() == "hero_025"
}

func validateP7NaturalRushScope(series *model.FrozenP6RushSeries) error {
	reject := func() error { return errors.New(constUtil.CombatUnsupported) }
	if err := validateP6RushSeries(series); err != nil {
		return err
	}
	input := series.Template
	if !p7NaturalRushIdentity(series) || !series.Trial || series.PlayerAutomatic || input.BattleType != 9 ||
		series.Start.GetRushType() != constUtil.P7RushTrial || series.Start.GetFightScene() != "map_grass_001" ||
		series.Start.GetMaxRound() != 40 || input.MaxRound != 40 || len(series.Start.BattleMonsterList) != 1 ||
		input.Seed < 0 || input.Seed > math.MaxInt32 || input.EngineVersion != constUtil.P6LinearPhysicalEngine ||
		input.ClientVersion != constUtil.P6OriginalClientHash || input.ConfigurationHash != constUtil.P6LinearPhysicalTables ||
		input.DiagnosticAIDriverPolicy != "" || input.DiagnosticManualAutoStartRoundTime != nil ||
		input.PVECompatibilityRule != "" || input.KFReplayViewer != nil || p6ScopedUnknown(series.Start) {
		return reject()
	}
	party := input.Attacker
	if party.GetPlayerId() <= 0 || party.GetServerId() <= 0 {
		return reject()
	}
	if err := validateP7NaturalRushNumericActors(party); err != nil {
		return err
	}
	raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(input.Monster)
	if err != nil || p6Digest(raw) != constUtil.P7NaturalRushNPCSHA256 {
		return reject()
	}
	tables, err := json.Marshal(input.Tables)
	if err != nil || p6Digest(tables) != constUtil.P6LinearPhysicalTables {
		return reject()
	}
	return nil
}

func p7NaturalRushProof(series *model.FrozenP6RushSeries) (string, error) {
	if err := validateP7NaturalRushScope(series); err != nil {
		return "", err
	}
	if constUtil.P7NaturalRushNativeProof == "" {
		return "", errors.New(constUtil.CombatUnsupported)
	}
	return constUtil.P7NaturalRushNativeProof, nil
}

func p7NaturalRushSourceAvailable(series *model.FrozenP6RushSeries) bool {
	if series == nil || constUtil.P7NaturalRushNativeProof == "" {
		return false
	}
	view := *series
	view.PlayerAutomatic = false
	_, err := p7NaturalRushProof(&view)
	return err == nil
}

func p7NaturalRushInputAt(series *model.FrozenP6RushSeries, at float64) (*model.FrozenBattleInput, error) {
	if err := validateP7NaturalRushScope(series); err != nil {
		return nil, err
	}
	// A legal first Auto clock selects only the private driver input. Every
	// resulting operation and terminal field is still recomputed and compared.
	if !combatFinite(at) || at < 0 || at > constUtil.P7NaturalRushAutoRoundTimeLimit {
		return nil, errors.New(constUtil.P6ReplayConflict)
	}
	input, err := cloneP6CalculationInput(series.Template)
	if err != nil {
		return nil, err
	}
	input.DiagnosticAIDriverPolicy = constUtil.P6DiagnosticManualAutoStart
	input.DiagnosticManualAutoStartRoundTime = proto.Float64(at)
	if err = ValidateP6AIDriverPolicy(input); err != nil {
		return nil, err
	}
	return input, nil
}

func p7NaturalRushCalculationInput(series *model.FrozenP6RushSeries, supplied *pb.BattleOperate) (*model.FrozenBattleInput, error) {
	if err := validateP7NaturalRushScope(series); err != nil {
		return nil, err
	}
	if supplied == nil || supplied.AttackerTransOpt != nil || supplied.DefenderTransOpt != nil || len(supplied.SkillOpts) == 0 || p6ScopedUnknown(supplied) {
		return nil, errors.New(constUtil.P6ReplayConflict)
	}
	first := supplied.SkillOpts[0]
	if first == nil || first.GetCamp() != 0 || first.GetRoundNum() != 1 || first.GetSkillIndex() < 0 || first.GetSkillIndex() > 2 || p6ScopedUnknown(first) {
		return nil, errors.New(constUtil.P6ReplayConflict)
	}
	known := false
	for _, actor := range series.Template.Attacker.Formation {
		if actor.GetGuid() == first.GetActorUid() {
			known = true
		}
	}
	if !known {
		return nil, errors.New(constUtil.P6ReplayConflict)
	}
	return p7NaturalRushInputAt(series, first.GetRoundTime())
}

// Diagnostic computation remains usable while proof is empty, with no paid
// entry or result authority. Only frozen input controls the original engine.
func (s *GameService) computeP7NaturalRushDiagnostic(ctx context.Context, series *model.FrozenP6RushSeries, at float64) (*model.CombatFinalHPOutcome, error) {
	if ctx == nil {
		return nil, errors.New(constUtil.CombatInvalid)
	}
	input, err := p7NaturalRushInputAt(series, at)
	if err != nil {
		return nil, err
	}
	out, err := s.ComputeOriginalP6ProtocolFinalHP(ctx, input, nil)
	if err != nil {
		return nil, fmt.Errorf("natural Trial original computation: %w", err)
	}
	if err = validateP7NaturalRushTerminal(input, out); err != nil {
		return nil, fmt.Errorf("natural Trial terminal: %w", err)
	}
	if out.Outcome.Authoritative {
		return nil, errors.New(constUtil.P6ReplayConflict)
	}
	return out, nil
}

func p7NaturalRushPreview(ctx context.Context, s *GameService, series *model.FrozenP6RushSeries) (*model.CombatFinalHPOutcome, error) {
	proof, err := p7NaturalRushProof(series)
	if err != nil {
		return nil, err
	}
	out, err := s.computeP7NaturalRushDiagnostic(ctx, series, 0)
	if err != nil {
		return nil, err
	}
	out.Outcome.Authoritative = true
	out.Outcome.CapabilityHash = proof
	if err = p7NaturalRushValidatePreview(series, out); err != nil {
		return nil, err
	}
	return out, nil
}

func p7NaturalRushValidatePreview(series *model.FrozenP6RushSeries, out *model.CombatFinalHPOutcome) error {
	proof, err := p7NaturalRushProof(series)
	if err != nil {
		return err
	}
	input, err := p7NaturalRushInputAt(series, 0)
	if err != nil {
		return err
	}
	return validateP7NaturalRushCertifiedTerminal(input, out, proof)
}

func p7NaturalRushComputeEnd(ctx context.Context, s *GameService, series *model.FrozenP6RushSeries, supplied *pb.BattleOperate, reported int32) (*model.CombatFinalHPOutcome, error) {
	proof, err := p7NaturalRushProof(series)
	if err != nil {
		return nil, err
	}
	input, err := p7NaturalRushCalculationInput(series, supplied)
	if err != nil {
		return nil, err
	}
	// Supplied operations determine only the already observed auto boundary.
	// Winner, all operations and terminal HP are independently recomputed.
	out, err := s.ComputeOriginalP6ProtocolFinalHP(ctx, input, nil)
	if err != nil {
		return nil, fmt.Errorf("natural Trial original computation: %w", err)
	}
	if err = validateP7NaturalRushTerminal(input, out); err != nil {
		return nil, fmt.Errorf("natural Trial terminal: %w", err)
	}
	out.Outcome.Authoritative = true
	out.Outcome.CapabilityHash = proof
	if err = p7NaturalRushValidateEnd(series, out, supplied, reported); err != nil {
		return nil, err
	}
	return out, nil
}

func p7NaturalRushValidateEnd(series *model.FrozenP6RushSeries, out *model.CombatFinalHPOutcome, supplied *pb.BattleOperate, reported int32) error {
	proof, err := p7NaturalRushProof(series)
	if err != nil {
		return err
	}
	input, err := p7NaturalRushCalculationInput(series, supplied)
	if err != nil {
		return err
	}
	if err = validateP7NaturalRushCertifiedTerminal(input, out, proof); err != nil {
		return err
	}
	expected := int32(0)
	if out.Outcome.Winner == 0 {
		expected = 1
	}
	if reported != expected || !p6RushStarterStrictRecordEqual(supplied, out.OriginalOperates) {
		return errors.New(constUtil.P6ReplayConflict)
	}
	return nil
}

func validateP7NaturalRushCertifiedTerminal(input *model.FrozenBattleInput, out *model.CombatFinalHPOutcome, proof string) error {
	if proof == "" || proof != constUtil.P7NaturalRushNativeProof || out == nil || out.Outcome == nil || !out.Outcome.Authoritative || out.Outcome.CapabilityHash != proof {
		return errors.New(constUtil.P6ReplayConflict)
	}
	return validateP7NaturalRushTerminal(input, out)
}

func validateP7NaturalRushTerminal(input *model.FrozenBattleInput, out *model.CombatFinalHPOutcome) error {
	reject := func() error { return errors.New(constUtil.CombatUnsupported) }
	if input == nil || out == nil || out.Outcome == nil || out.OriginalOperates == nil || out.Outcome.Operates == nil || out.Schema != constUtil.P6FinalActorHPSchema || out.EngineVersion != input.EngineVersion || out.ClientVersion != input.ClientVersion || out.ConfigurationHash != input.ConfigurationHash || len(out.FinalActors) != 3 || out.OriginalRecordSHA256 != "" || out.OriginalOperates.AttackerTransOpt != nil || out.OriginalOperates.DefenderTransOpt != nil {
		return fmt.Errorf("outcome headers: %w", reject())
	}
	sources := map[string]*pb.BattleHero{}
	for camp, heroes := range [][]*pb.BattleHero{input.Attacker.Formation, input.Monster.Formation} {
		for _, hero := range heroes {
			sources[fmt.Sprintf("%d/%s", camp, hero.GetGuid())] = hero
		}
	}
	seen := map[string]bool{}
	current := [2]float64{}
	for _, actor := range out.FinalActors {
		key := fmt.Sprintf("%d/%s", actor.Camp, actor.GUID)
		source := sources[key]
		if source == nil || seen[key] || actor.Camp < 0 || actor.Camp > 1 {
			return fmt.Errorf("source actor: %w", reject())
		}
		seen[key] = true
		if actor.UID != fmt.Sprintf("%d_%s", actor.Camp, actor.GUID) || actor.HeroID != source.GetId() || actor.Pose != source.GetPose() || actor.RuntimePose != 0 || actor.BloodNumber != 1 || actor.MaxHP != source.Prop.GetHp() || !combatFinite(actor.CurrentHP) || actor.CurrentHP < 0 || actor.CurrentHP > actor.MaxHP {
			return fmt.Errorf("actor identity or maxHP: %w", reject())
		}
		raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(source)
		if err != nil || p6Digest(raw) != actor.InitialActorSHA256 {
			return fmt.Errorf("initial actor digest: %w", reject())
		}
		current[actor.Camp] += actor.CurrentHP
	}
	if out.Outcome.Operates.GetFinalRoundNum() != out.Outcome.FinalRound || out.Outcome.Operates.GetAttackerTotalLeftHp() != current[0] || out.Outcome.Operates.GetDefenderTotalLeftHp() != current[1] || len(seen) != len(sources) || out.Outcome.FinalRound < 1 || out.Outcome.FinalRound > 41 || out.OriginalOperates.GetFinalRoundNum() != out.Outcome.FinalRound || out.OriginalOperates.GetAttackerTotalLeftHp() != current[0] || out.OriginalOperates.GetDefenderTotalLeftHp() != current[1] {
		return fmt.Errorf("final clock or totalHP: %w", reject())
	}
	winner := int32(1)
	if current[1] == 0 && current[0] > 0 {
		winner = 0
	} else if !(current[0] == 0 && current[1] > 0 || current[0] > 0 && current[1] > 0 && out.Outcome.FinalRound == 41) {
		return fmt.Errorf("survivor convention: %w", reject())
	}
	if out.Outcome.Winner != winner {
		return fmt.Errorf("reported winner: %w", reject())
	}
	for _, opt := range out.OriginalOperates.SkillOpts {
		if opt == nil || opt.GetCamp() < 0 || opt.GetCamp() > 1 || sources[fmt.Sprintf("%d/%s", opt.GetCamp(), opt.GetActorUid())] == nil || opt.GetSkillIndex() < 0 || opt.GetSkillIndex() > 2 || opt.GetRoundNum() < 1 || opt.GetRoundNum() > out.Outcome.FinalRound || !combatFinite(opt.GetRoundTime()) || opt.GetRoundTime() < 0 {
			return fmt.Errorf("original operations: %w", reject())
		}
	}
	for _, event := range out.Outcome.Events {
		if event.Skill != "" {
			switch event.Skill {
			case "025_1", "025_2", "025_3", "047_1", "047_2", "047_3":
			default:
				return fmt.Errorf("skill domain: %w", reject())
			}
		}
		if event.Buff != "" {
			switch event.Buff {
			case "buff_120", "buff_270", "buff_355", "buff_365", "buff_380":
			default:
				return fmt.Errorf("buff domain: %w", reject())
			}
		}
	}
	// 冻结计算来源含具体自动切换时点，防止对照时偷换driver或演员。
	view := *input
	view.Tables = nil
	raw, err := json.Marshal(&view)
	if err != nil || p6Digest(raw) != out.CalculationSourceSHA256 {
		return fmt.Errorf("calculation source digest: %w", reject())
	}
	return nil
}

// The mechanism header fixes skills/talents/pose while production P5 may vary
// bounded numerical panels. The caller retains typed9/driver/NPC/table/owner
// gates and the independent certificate; missing proof cannot admit a source.
// Only the production battlePlayer producer may supply these actor panels;
// this helper is not an admission API for client-supplied BattlePlayers.
func validateP7NaturalRushNumericActors(party *pb.BattlePlayer) error {
	reject := func() error { return errors.New(constUtil.CombatUnsupported) }
	if party == nil || party.GetLevel() < 22 || party.GetLevel() > 100 || len(party.Formation) != 2 ||
		party.Inst == nil || party.Inst.GetAnger() != 0 || party.Inst.GetFirstOrder() != 0 ||
		len(party.Inst.ExtBattleAdds) != 0 || p6ScopedUnknown(party) || p6ScopedUnknown(party.Inst) {
		return reject()
	}
	seen := map[string]bool{}
	for index, actor := range party.Formation {
		if actor == nil || actor.Prop == nil || actor.Inst == nil || p6ScopedUnknown(actor.Prop) || p6ScopedUnknown(actor.Inst) ||
			actor.GetGuid() == "" || len(actor.GetGuid()) > 128 || strings.Contains(actor.GetGuid(), "_") || seen[actor.GetGuid()] {
			return reject()
		}
		seen[actor.GetGuid()] = true
		if index == 0 && actor.GetLevel() != party.GetLevel() || index == 1 && (actor.GetLevel() < 3 || actor.GetLevel() > party.GetLevel()) {
			return reject()
		}
		view := proto.Clone(actor).(*pb.BattleHero)
		view.Guid = proto.String([]string{"parameter-main", "parameter-companion"}[index])
		view.Level = proto.Int32(0)
		view.Prop = nil
		view.Inst = nil
		raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(view)
		if err != nil || p6Digest(raw) != []string{constUtil.P7NaturalRushMainHeaderSHA256, constUtil.P7NaturalRushCompanionHeaderSHA256}[index] {
			return reject()
		}
		// ATT/defense are original double inputs. Never truncate, snap to half
		// steps or normalize IEEE residues produced by legal P5 equipment math.
		p := actor.Prop
		for _, value := range []float64{p.GetAtt(), p.GetHp(), p.GetPdef(), p.GetMdef()} {
			if !combatFinite(value) || value < 0 || value > constUtil.P6LinearPhysicalPanelLimit {
				return reject()
			}
		}
		// Current narrow extension keeps the existing integer-maxHP class.
		// Fractional HP remains pending an explicit original-Engine/phone vector;
		// HP itself is a parameter, never an exact snapshot whitelist value.
		if p.GetAtt() < 1 || p.GetHp() < 1 || math.Trunc(p.GetHp()) != p.GetHp() || actor.Inst.GetCurHp() != p.GetHp() {
			return reject()
		}
		if p.GetHit() != 95 || p.GetDodge() != 5 || p.GetCri() != 10 || p.GetAntiCri() != 0 || p.GetCriFactor() != 50 ||
			p.GetDamageAdd() != 0 || p.GetDamageReduce() != 0 || p.GetPeratt() != 0 || p.GetPerhp() != 0 || p.GetPerpdef() != 0 || p.GetPermdef() != 0 {
			return reject()
		}
	}
	return nil
}
