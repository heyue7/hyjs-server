package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// The first Rush settlement family preserves the original typed callback and
// explicit automatic-player choice. Later waves, manual records and nonzero
// private-server win criteria remain outside this independent capability.
func validateP6RushStarterScope(series *model.FrozenP6RushSeries) error {
	reject := func() error { return errors.New(constUtil.CombatUnsupported) }
	if err := validateP6RushSeries(series); err != nil {
		return err
	}
	if !series.PlayerAutomatic || len(series.Start.BattleMonsterList) != 1 {
		return reject()
	}
	input := series.Template
	if input.BattleType != 9 && input.BattleType != 10 {
		return reject()
	}
	monster := input.Monster
	if monster == nil || monster.Inst == nil || monster.Inst.GetAnger() != 0 || len(monster.Formation) != 1 || p6ScopedUnknown(monster) || p6ScopedUnknown(monster.Inst) {
		return reject()
	}
	actor := monster.Formation[0]
	if actor == nil || actor.GetPose() != 1 || actor.GetBreachedLevel() != 0 || actor.GetLevel() != 1 {
		return reject()
	}
	if input.Attacker.Inst.GetFirstOrder() != 0 || len(input.Attacker.Inst.ExtBattleAdds) != 0 {
		return reject()
	}
	// These views share only the sealed field validator. Actual source actor
	// pose/breach, copy-enemy driver and initiative are retained for computation.
	view, err := cloneP6CalculationInput(input)
	if err != nil {
		return err
	}
	view.BattleType = 11
	source := proto.Clone(actor).(*pb.BattleHero)
	source.Pose = proto.Int32(3)
	source.BreachedLevel = proto.Int32(1)
	peerID := int64(1)
	if view.Attacker.GetPlayerId() == peerID {
		peerID = 2
	}
	view.Defender = &pb.BattlePlayer{PlayerId: proto.Int64(peerID), ServerId: view.Attacker.ServerId, Formation: []*pb.BattleHero{source}, Inst: &pb.BattlePlayerInst{Anger: proto.Float64(0), FirstOrder: proto.Int32(0)}}
	view.Monster = nil
	view.Attacker.Inst.FirstOrder = proto.Int32(1)
	return validateP6Starter047Scope(view)
}

// ComputeP6RushStarterSettlement returns a result only for an independently
// original-certified automatic single-wave9/10 source. A consumer must bind its
// real client driver/record before applying business rewards; manual gameplay
// cannot be relabeled automatic merely by calling this API.
func (s *GameService) ComputeP6RushStarterSettlement(ctx context.Context, series *model.FrozenP6RushSeries) (*model.CombatFinalHPOutcome, error) {
	if ctx == nil || series == nil {
		return nil, errors.New(constUtil.CombatInvalid)
	}
	if err := validateP6RushStarterScope(series); err != nil {
		return nil, err
	}
	input, err := cloneP6CalculationInput(series.Template)
	if err != nil {
		return nil, err
	}
	copy := *series
	copy.Template = input
	copy.Start = proto.Clone(series.Start).(*pb.SCRushFightStart)
	if err = validateP6RushStarterScope(&copy); err != nil {
		return nil, err
	}
	if constUtil.P6RushStarterNativeProof == "" {
		return nil, errors.New(constUtil.CombatUnsupported)
	}
	out, err := s.ComputeOriginalP6ProtocolFinalHP(ctx, input, nil)
	if err != nil {
		return nil, err
	}
	if err = validateP6RushStarterTerminal(input, out); err != nil {
		return nil, err
	}
	out.Outcome.Authoritative = true
	out.Outcome.CapabilityHash = constUtil.P6RushStarterNativeProof
	return out, nil
}

func validateP6RushStarterTerminal(input *model.FrozenBattleInput, result *model.CombatFinalHPOutcome) error {
	reject := func() error { return errors.New(constUtil.CombatUnsupported) }
	if result == nil || result.Outcome == nil || result.OriginalOperates == nil || len(result.FinalActors) != 2 || result.OriginalRecordSHA256 != "" || result.OriginalOperates.AttackerTransOpt != nil || result.OriginalOperates.DefenderTransOpt != nil {
		return reject()
	}
	sources := []*pb.BattleHero{input.Attacker.Formation[0], input.Monster.Formation[0]}
	seen := [2]bool{}
	current := [2]float64{}
	for _, actor := range result.FinalActors {
		if actor.Camp < 0 || actor.Camp > 1 || seen[actor.Camp] {
			return reject()
		}
		seen[actor.Camp] = true
		source := sources[actor.Camp]
		body, err := proto.MarshalOptions{Deterministic: true}.Marshal(source)
		if err != nil || actor.InitialActorSHA256 != p6Digest(body) || actor.GUID != source.GetGuid() || actor.HeroID != source.GetId() || actor.UID != fmt.Sprintf("%d_%s", actor.Camp, source.GetGuid()) || actor.Pose != source.GetPose() || actor.RuntimePose != 0 || actor.BloodNumber != 1 || actor.MaxHP != source.Prop.GetHp() || !combatFinite(actor.CurrentHP) || actor.CurrentHP < 0 || actor.CurrentHP > actor.MaxHP {
			return reject()
		}
		current[actor.Camp] = actor.CurrentHP
	}
	if !seen[0] || !seen[1] || result.Outcome.FinalRound < 1 || result.Outcome.FinalRound > 41 || result.OriginalOperates.GetFinalRoundNum() != result.Outcome.FinalRound || result.OriginalOperates.GetAttackerTotalLeftHp() != current[0] || result.OriginalOperates.GetDefenderTotalLeftHp() != current[1] || result.Outcome.Operates.GetAttackerTotalLeftHp() != current[0] || result.Outcome.Operates.GetDefenderTotalLeftHp() != current[1] {
		return reject()
	}
	winner := int32(1)
	if current[1] == 0 && current[0] > 0 {
		winner = 0
	} else if !(current[0] == 0 && current[1] > 0 || current[0] > 0 && current[1] > 0 && result.Outcome.FinalRound == 41) {
		return reject()
	}
	if result.Outcome.Winner != winner {
		return reject()
	}
	for _, opt := range result.OriginalOperates.SkillOpts {
		if opt == nil || opt.GetCamp() < 0 || opt.GetCamp() > 1 || opt.GetActorUid() != sources[opt.GetCamp()].GetGuid() || opt.GetSkillIndex() < 0 || opt.GetSkillIndex() > 2 || opt.GetRoundNum() < 1 || opt.GetRoundNum() > result.Outcome.FinalRound || !combatFinite(opt.GetRoundTime()) || opt.GetRoundTime() < 0 {
			return reject()
		}
	}
	for _, event := range result.Outcome.Events {
		if event.Buff != "" && event.Buff != "buff_380" {
			return reject()
		}
		if event.Skill != "" && event.Skill != "047_1" && event.Skill != "047_2" && event.Skill != "047_3" {
			return reject()
		}
	}
	return nil
}

// ValidateP6RushStarterEnd binds the complete original client end record to the
// same independently computed automatic source. It never replays or accepts a
// merely equal winner, subset of actions, shifted clock or unverified diagnostic.
func ValidateP6RushStarterEnd(series *model.FrozenP6RushSeries, terminal *model.CombatFinalHPOutcome, supplied *pb.BattleOperate, reportedResult int32) error {
	reject := func() error { return errors.New(constUtil.P6ReplayConflict) }
	if err := validateP6RushStarterScope(series); err != nil {
		return err
	}
	if constUtil.P6RushStarterNativeProof == "" {
		return errors.New(constUtil.CombatUnsupported)
	}
	if terminal == nil || terminal.Outcome == nil || !terminal.Outcome.Authoritative || terminal.Outcome.CapabilityHash != constUtil.P6RushStarterNativeProof || terminal.Schema != constUtil.P6FinalActorHPSchema || terminal.EngineVersion != series.Template.EngineVersion || terminal.ClientVersion != series.Template.ClientVersion || terminal.ConfigurationHash != series.Template.ConfigurationHash {
		return reject()
	}
	if err := validateP6RushStarterTerminal(series.Template, terminal); err != nil {
		return err
	}
	closure := *series.Template
	closure.Tables = nil
	raw, err := json.Marshal(&closure)
	if err != nil || p6Digest(raw) != terminal.CalculationSourceSHA256 {
		return reject()
	}
	expected := int32(0)
	if terminal.Outcome.Winner == 0 {
		expected = 1
	}
	if reportedResult != expected || !p6RushStarterStrictRecordEqual(supplied, terminal.OriginalOperates) {
		return reject()
	}
	return nil
}

func p6RushStarterStrictRecordEqual(left, right *pb.BattleOperate) bool {
	if left == nil || right == nil || left.AttackerTransOpt != nil || left.DefenderTransOpt != nil || right.AttackerTransOpt != nil || right.DefenderTransOpt != nil {
		return false
	}
	a := proto.Clone(left).(*pb.BattleOperate)
	b := proto.Clone(right).(*pb.BattleOperate)
	if !p6RushStarterNormalizeDefaults(a.ProtoReflect()) || !p6RushStarterNormalizeDefaults(b.ProtoReflect()) {
		return false
	}
	return proto.Equal(a, b)
}

// Original protobuf-net omits explicit scalar defaults. Normalize only those
// fields in private clones; preserve message/list order and every nonzero bit.
func p6RushStarterNormalizeDefaults(message protoreflect.Message) bool {
	if !message.IsValid() || len(message.GetUnknown()) != 0 {
		return false
	}
	valid := true
	message.Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
		if field.IsList() {
			if field.Kind() != protoreflect.MessageKind {
				valid = false
				return false
			}
			list := value.List()
			for index := 0; index < list.Len(); index++ {
				if !p6RushStarterNormalizeDefaults(list.Get(index).Message()) {
					valid = false
					return false
				}
			}
			return true
		}
		zero := false
		switch field.Kind() {
		case protoreflect.MessageKind:
			valid = p6RushStarterNormalizeDefaults(value.Message())
		case protoreflect.Int32Kind, protoreflect.Int64Kind, protoreflect.Sint32Kind, protoreflect.Sint64Kind, protoreflect.Sfixed32Kind, protoreflect.Sfixed64Kind:
			zero = value.Int() == 0
		case protoreflect.DoubleKind, protoreflect.FloatKind:
			valid = combatFinite(value.Float())
			zero = value.Float() == 0
		case protoreflect.StringKind:
			zero = value.String() == ""
		default:
			valid = false
		}
		if zero {
			message.Clear(field)
		}
		return valid
	})
	return valid
}

func p6KnownRushStarterSettlementProof(input *model.FrozenBattleInput, proof string) bool {
	return input != nil && (input.BattleType == 9 || input.BattleType == 10) && input.EngineVersion == constUtil.P6RushStarterSettlementEngine && input.ClientVersion == constUtil.P6OriginalClientHash && input.ConfigurationHash == constUtil.P6LinearPhysicalTables && proof != "" && proof == constUtil.P6RushStarterNativeProof
}
