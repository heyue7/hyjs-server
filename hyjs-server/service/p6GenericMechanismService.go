package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// ValidateP6GenericMechanisms uses original source rows and implemented branch
// handlers. Actor IDs, party size, fractional grown panels and pet talents are
// accepted by their original engine constraints, independent of old demo scopes.
func (s *GameService) ValidateP6GenericMechanisms(ctx context.Context, input *model.FrozenBattleInput) (*model.P6MechanismQualification, error) {
	if ctx == nil || input == nil {
		return nil, errors.New(constUtil.CombatInvalid)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateFrozenP6(input); err != nil {
		return nil, err
	}
	if input.EngineVersion != constUtil.P6EngineVersion || input.ClientVersion != constUtil.P6OriginalClientHash {
		return nil, errors.New(constUtil.P6ReplayConflict)
	}
	// Compare actual rows with the server's original source collection, so a
	// caller cannot make fabricated skills valid merely by changing their digest.
	canonical, err := s.FreezeBattleInput(*input)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(input.Tables)
	if err != nil || p6Digest(raw) != input.ConfigurationHash || canonical.ConfigurationHash != input.ConfigurationHash {
		return nil, errors.New(constUtil.P6ReplayConflict)
	}
	return qualifyP6GenericMechanisms(s, input, canonical)
}

// freezeAndQualifyP6GenericMechanisms admits a new server-built fight before its
// frozen input escapes. The source tables come directly from FreezeBattleInput;
// persisted or caller-provided snapshots still use the full public verifier.
func (s *GameService) freezeAndQualifyP6GenericMechanisms(ctx context.Context, source model.FrozenBattleInput) (*model.FrozenBattleInput, error) {
	if ctx == nil {
		return nil, errors.New(constUtil.CombatInvalid)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	frozen, err := s.FreezeBattleInput(source)
	if err != nil {
		return nil, err
	}
	if frozen.EngineVersion != constUtil.P6EngineVersion || frozen.ClientVersion != constUtil.P6OriginalClientHash {
		return nil, errors.New(constUtil.P6ReplayConflict)
	}
	if _, err := qualifyP6GenericMechanisms(s, frozen, frozen); err != nil {
		return nil, err
	}
	return frozen, nil
}

func qualifyP6GenericMechanisms(s *GameService, input, canonical *model.FrozenBattleInput) (*model.P6MechanismQualification, error) {
	engine := &combatRoundEngine{service: &GameService{Tables: canonical.Tables}, input: canonical, rng: NewCombatRandom(canonical.Seed), operate: &pb.BattleOperate{}, round: 1, originalRecordMode: true}
	if canonical.DefenderFirst {
		engine.camp = 1
	}
	formations := [][]*pb.BattleHero{canonical.Attacker.GetFormation()}
	if canonical.Defender != nil {
		formations = append(formations, canonical.Defender.GetFormation())
	} else {
		formations = append(formations, canonical.Monster.GetFormation())
	}
	for _, formation := range formations {
		for _, hero := range formation {
			if err := p6ValidateGenericActorSources(engine, hero); err != nil {
				return nil, err
			}
		}
	}
	// The existing constructor validates every original slot, cast, recursive
	// buff template and talent branch before any fight or economic side effect.
	if err := engine.initialize(); err != nil {
		return nil, fmt.Errorf("%s: original actor/skill/talent admission: %w", constUtil.CombatUnsupported, err)
	}
	qualification := &model.P6MechanismQualification{EngineVersion: input.EngineVersion, ConfigurationHash: input.ConfigurationHash}
	skills, talents, buffs, mechanisms := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, player := range engine.players {
		for _, actor := range player.actors {
			qualification.ActorCount++
			for _, id := range actor.hero.GetTalentList() {
				talents[id] = true
			}
			for _, skill := range actor.skills {
				skills[skill.id] = true
				for _, cast := range skill.casts {
					mechanisms[fmt.Sprintf("cast/export=%s/selector=%s/state=%s/advance=%s", cast["ExportType"], cast["SelectStrategy"], cast["CauseSpeState"], cast["AdvSpeState"])] = true
					for slot := 0; slot < 3; slot++ {
						key := fmt.Sprintf("BufferId%d", slot)
						if id := cast[key]; id != "" {
							row := engine.table("Buff", "Id", id)
							p6CollectGenericBuffSources(engine, id, buffs, mechanisms)
							mechanisms[fmt.Sprintf("buff/%s/class=%s/cast=%s/selector=%s/condition=%s:%s", row["Factor"], row["BuffClass"], cast[fmt.Sprintf("BuffTriggerClass%d", slot)], cast[fmt.Sprintf("Buff%dSelStr", slot)], cast[fmt.Sprintf("Buff%dTriggerCondition", slot)], cast[fmt.Sprintf("Buff%dConditionArgs", slot)])] = true
						}
					}
				}
			}
			for _, trigger := range actor.talents {
				row := engine.table("Buff", "Id", trigger.buff)
				p6CollectGenericBuffSources(engine, trigger.buff, buffs, mechanisms)
				mechanisms[fmt.Sprintf("buff/%s/class=%s/timer=%d/selector=%d/condition=%d:%s", row["Factor"], row["BuffClass"], trigger.timer, trigger.selectStrategy, trigger.condition, trigger.args)] = true
			}
		}
	}
	qualification.SkillIDs = p6SortedMechanismKeys(skills)
	qualification.TalentIDs = p6SortedMechanismKeys(talents)
	qualification.BuffIDs = p6SortedMechanismKeys(buffs)
	qualification.Mechanisms = p6SortedMechanismKeys(mechanisms)
	encoded, err := json.Marshal(qualification.Mechanisms)
	if err != nil {
		return nil, err
	}
	qualification.MechanismHash = p6Digest(encoded)
	return qualification, nil
}

func p6ValidateGenericActorSources(engine *combatRoundEngine, hero *pb.BattleHero) error {
	row := engine.table("Hero", "id", hero.GetId())
	if row == nil {
		return fmt.Errorf("%s: Hero %s", constUtil.CombatUnsupported, hero.GetId())
	}
	for _, id := range hero.GetTalentList() {
		talent := engine.table("Talent", "Id", id)
		if talent == nil {
			return fmt.Errorf("%s: actor %s Talent %s", constUtil.CombatUnsupported, hero.GetId(), id)
		}
		for slot := 0; slot < 4; slot++ {
			key := fmt.Sprintf("Buff%d", slot)
			if talent[key] == "" {
				continue
			}
			if engine.table("Buff", "Id", talent[key]) == nil {
				return fmt.Errorf("%s: actor %s Talent %s Buff %s", constUtil.CombatUnsupported, hero.GetId(), id, talent[key])
			}
			trigger := combatRoundTrigger{buff: talent[key], timer: int32(tableNumber(talent, key+"TriggerTimer")), selectStrategy: int32(tableNumber(talent, key+"Select")), condition: int32(tableNumber(talent, key+"TriggerCondition")), args: talent[key+"ConditionArgs"], chance: tableNumber(talent, key+"Percent") * .01}
			if err := engine.validateDynamicTrigger(trigger); err != nil {
				return fmt.Errorf("actor %s Talent %s Buff %s: %w", hero.GetId(), id, talent[key], err)
			}
		}
	}
	slots, _, err := engine.p6AwakenSkillSlots(hero, row)
	if err != nil {
		return fmt.Errorf("actor %s original skill slots: %w", hero.GetId(), err)
	}
	for _, skill := range slots {
		if skill == "" {
			continue
		}
		if engine.table("Skill", "SkillId", skill) == nil {
			return fmt.Errorf("%s: actor %s Skill %s", constUtil.CombatUnsupported, hero.GetId(), skill)
		}
		for _, cast := range engine.service.Tables["SkillCast"] {
			if cast["SkillId"] != skill {
				continue
			}
			if err := engine.validateCastBuffs(cast); err != nil {
				return fmt.Errorf("actor %s Skill %s cast %s: %w", hero.GetId(), skill, cast["SkillCastId"], err)
			}
		}
	}
	return nil
}

// p6CollectGenericBuffSources records the already validated template closure.
// The actual admission remains validateDynamicTriggerPath, which checks cycles,
// trigger shapes and factory arguments before this source inventory is built.
func p6CollectGenericBuffSources(engine *combatRoundEngine, id string, buffs, mechanisms map[string]bool) {
	if buffs[id] {
		return
	}
	row := engine.table("Buff", "Id", id)
	if row == nil {
		return
	}
	buffs[id] = true
	mechanisms[fmt.Sprintf("buff/source=%s/class=%s", row["Factor"], row["BuffClass"])] = true
	var children []string
	switch row["Factor"] {
	case "randomBuff", "reboundPerMaxhp":
		children = strings.Split(row["Param"], ",")
	case "changeSkillBuff":
		parts := strings.Split(row["Param"], ",")
		if len(parts) == 3 {
			children = parts[2:]
		}
	}
	for _, child := range children {
		if child != "" {
			p6CollectGenericBuffSources(engine, child, buffs, mechanisms)
		}
	}
}

func p6SortedMechanismKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// ComputeP6GenericMechanismBattle runs the unchanged complete original driver.
// Native authority stays with reviewed mode certificates; this result can be
// compared with a representative native trace without demo actor restrictions.
func (s *GameService) ComputeP6GenericMechanismBattle(ctx context.Context, input *model.FrozenBattleInput, supplied *pb.BattleOperate) (*model.CombatFinalHPOutcome, *model.P6MechanismQualification, error) {
	qualification, err := s.ValidateP6GenericMechanisms(ctx, input)
	if err != nil {
		return nil, nil, err
	}
	result, err := s.ComputeOriginalP6ProtocolFinalHP(ctx, input, supplied)
	if err != nil {
		return nil, qualification, err
	}
	if err = validateP6GenericMechanismTerminal(input, result); err != nil {
		return nil, qualification, err
	}
	// Do not relax full original record equality to hide a clock or operation mismatch.
	if supplied != nil && !proto.Equal(supplied, result.OriginalOperates) {
		return nil, qualification, errors.New(constUtil.P6ReplayConflict)
	}
	return result, qualification, nil
}

func validateP6GenericMechanismTerminal(input *model.FrozenBattleInput, result *model.CombatFinalHPOutcome) error {
	if input == nil || result == nil || result.Outcome == nil || result.Outcome.Operates == nil || result.OriginalOperates == nil {
		return errors.New(constUtil.P6ReplayConflict)
	}
	formations := [][]*pb.BattleHero{input.Attacker.GetFormation()}
	if input.Defender != nil {
		formations = append(formations, input.Defender.GetFormation())
	} else {
		formations = append(formations, input.Monster.GetFormation())
	}
	sources := map[string]*pb.BattleHero{}
	for camp, formation := range formations {
		for _, hero := range formation {
			sources[fmt.Sprintf("%d_%s", camp, hero.GetGuid())] = hero
		}
	}
	if len(result.FinalActors) != len(sources) {
		return errors.New(constUtil.P6ReplayConflict)
	}
	seen, totals := map[string]bool{}, [2]float64{}
	for _, actor := range result.FinalActors {
		source := sources[actor.UID]
		if source == nil || seen[actor.UID] || actor.Camp < 0 || actor.Camp > 1 || actor.UID != fmt.Sprintf("%d_%s", actor.Camp, actor.GUID) || actor.HeroID != source.GetId() || actor.GUID != source.GetGuid() || actor.Pose != source.GetPose() || !combatFinite(actor.CurrentHP) || !combatFinite(actor.MaxHP) || actor.CurrentHP < 0 || actor.MaxHP <= 0 || actor.CurrentHP > actor.MaxHP {
			return errors.New(constUtil.P6ReplayConflict)
		}
		encoded, err := proto.MarshalOptions{Deterministic: true}.Marshal(source)
		if err != nil || p6Digest(encoded) != actor.InitialActorSHA256 {
			return errors.New(constUtil.P6ReplayConflict)
		}
		seen[actor.UID] = true
		totals[actor.Camp] += actor.CurrentHP
	}
	end := result.OriginalOperates
	if end.GetFinalRoundNum() < 1 || end.GetFinalRoundNum() > input.MaxRound+1 || end.GetFinalRoundNum() != result.Outcome.FinalRound || end.GetAttackerTotalLeftHp() != totals[0] || end.GetDefenderTotalLeftHp() != totals[1] || result.Outcome.Operates.GetAttackerTotalLeftHp() != totals[0] || result.Outcome.Operates.GetDefenderTotalLeftHp() != totals[1] {
		return errors.New(constUtil.P6ReplayConflict)
	}
	winner := int32(1)
	if totals[1] == 0 {
		winner = 0
	} else if totals[0] > 0 && end.GetFinalRoundNum() != input.MaxRound+1 {
		return errors.New(constUtil.P6ReplayConflict)
	}
	if result.Outcome.Winner != winner {
		return errors.New(constUtil.P6ReplayConflict)
	}
	return nil
}
