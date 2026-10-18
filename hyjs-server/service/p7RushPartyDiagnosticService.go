package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// validateP7PartyRushDiagnosticScope 只复用已证整数面板/字段校验；9型双角色驱动独立待原机验证。
func validateP7PartyRushDiagnosticScope(series *model.FrozenP6RushSeries) error {
	if err := validateP6RushSeries(series); err != nil {
		return err
	}
	reject := func() error { return errors.New(constUtil.CombatUnsupported) }
	if !series.Trial || series.PlayerAutomatic || series.Template.BattleType != 9 || len(series.Start.BattleMonsterList) != 1 || len(series.Template.Attacker.Formation) != 2 || series.Template.DiagnosticAIDriverPolicy != "" || series.Template.DiagnosticManualAutoStartRoundTime != nil {
		return reject()
	}
	actors := series.Template.Attacker.Formation
	if actors[0] == nil || actors[1] == nil || actors[0].GetId() != "hero_001" || actors[1].GetId() != "hero_003" || actors[0].GetPose() != 3 || actors[1].GetPose() != 7 || actors[0].GetGuid() == actors[1].GetGuid() {
		return reject()
	}
	for i, actor := range actors {
		prefix := []string{"001", "003"}[i]
		if actor.GetSkillOrder() != prefix+"_1,"+prefix+"_2,"+prefix+"_3" || len(actor.TalentList) != 0 {
			return reject()
		}
		view := *series
		input, err := cloneP6CalculationInput(series.Template)
		if err != nil {
			return err
		}
		hero := proto.Clone(actor).(*pb.BattleHero)
		hero.Id = proto.String("hero_047")
		hero.SkillOrder = proto.String("047_1,047_2,047_3")
		hero.Pose = proto.Int32(3)
		input.Attacker.Formation = []*pb.BattleHero{hero}
		view.Template = input
		view.PlayerAutomatic = true
		view.Start = proto.Clone(series.Start).(*pb.SCRushFightStart)
		view.Start.BattlePlayer = proto.Clone(input.Attacker).(*pb.BattlePlayer)
		raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(view.Start)
		if err != nil {
			return err
		}
		view.StartSHA256 = p6Digest(raw)
		// 私有身份视图只检查已有字段边界；计算始终使用原001/003及全部技能。
		if err = validateP6RushStarterScope(&view); err != nil {
			return err
		}
	}
	return nil
}

// ComputeP7PartyRushDiagnostic 供有界原机差分；本函数永远不授予付费、星数或结果权威。
func (s *GameService) ComputeP7PartyRushDiagnostic(ctx context.Context, series *model.FrozenP6RushSeries, autoAt float64) (*model.CombatFinalHPOutcome, error) {
	if ctx == nil {
		return nil, errors.New(constUtil.CombatInvalid)
	}
	if err := validateP7PartyRushDiagnosticScope(series); err != nil {
		return nil, err
	}
	input, err := cloneP6CalculationInput(series.Template)
	if err != nil {
		return nil, err
	}
	input.DiagnosticAIDriverPolicy = constUtil.P6DiagnosticManualAutoStart
	input.DiagnosticManualAutoStartRoundTime = proto.Float64(autoAt)
	if err = ValidateP6AIDriverPolicy(input); err != nil {
		return nil, err
	}
	out, err := s.ComputeOriginalP6ProtocolFinalHP(ctx, input, nil)
	if err != nil {
		return nil, err
	}
	if err = validateP7PartyRushDiagnosticTerminal(input, out); err != nil {
		return nil, err
	}
	if out.Outcome.Authoritative {
		return nil, errors.New("试炼队伍诊断意外获得业务权威")
	}
	return out, nil
}

func validateP7PartyRushDiagnosticTerminal(input *model.FrozenBattleInput, out *model.CombatFinalHPOutcome) error {
	reject := func() error { return errors.New(constUtil.CombatUnsupported) }
	if out == nil || out.Outcome == nil || out.OriginalOperates == nil || len(out.FinalActors) != 3 || out.OriginalRecordSHA256 != "" || out.OriginalOperates.AttackerTransOpt != nil || out.OriginalOperates.DefenderTransOpt != nil {
		return reject()
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
			return reject()
		}
		seen[key] = true
		if actor.UID != fmt.Sprintf("%d_%s", actor.Camp, actor.GUID) || actor.HeroID != source.GetId() || actor.Pose != source.GetPose() || actor.RuntimePose != 0 || actor.BloodNumber != 1 || actor.MaxHP != source.Prop.GetHp() || !combatFinite(actor.CurrentHP) || actor.CurrentHP < 0 || actor.CurrentHP > actor.MaxHP {
			return reject()
		}
		raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(source)
		if err != nil || p6Digest(raw) != actor.InitialActorSHA256 {
			return reject()
		}
		current[actor.Camp] += actor.CurrentHP
	}
	if len(seen) != len(sources) || out.Outcome.FinalRound < 1 || out.Outcome.FinalRound > 41 || out.OriginalOperates.GetFinalRoundNum() != out.Outcome.FinalRound || out.OriginalOperates.GetAttackerTotalLeftHp() != current[0] || out.OriginalOperates.GetDefenderTotalLeftHp() != current[1] {
		return reject()
	}
	winner := int32(1)
	if current[1] == 0 && current[0] > 0 {
		winner = 0
	} else if !(current[0] == 0 && current[1] > 0 || current[0] > 0 && current[1] > 0 && out.Outcome.FinalRound == 41) {
		return reject()
	}
	if out.Outcome.Winner != winner {
		return reject()
	}
	for _, opt := range out.OriginalOperates.SkillOpts {
		if opt == nil || opt.GetCamp() < 0 || opt.GetCamp() > 1 || sources[fmt.Sprintf("%d/%s", opt.GetCamp(), opt.GetActorUid())] == nil || opt.GetSkillIndex() < 0 || opt.GetSkillIndex() > 2 || opt.GetRoundNum() < 1 || opt.GetRoundNum() > out.Outcome.FinalRound || !combatFinite(opt.GetRoundTime()) || opt.GetRoundTime() < 0 {
			return reject()
		}
	}
	for _, event := range out.Outcome.Events {
		if event.Skill != "" {
			switch event.Skill {
			case "001_1", "001_2", "001_3", "003_1", "003_2", "003_3", "047_1", "047_2", "047_3":
			default:
				return reject()
			}
		}
		if event.Buff != "" {
			switch event.Buff {
			case "buff_119", "buff_120", "buff_122", "buff_135", "buff_152", "buff_197", "buff_270", "buff_318", "buff_355", "buff_365", "buff_380", "buff_488", "buff_489", "buff_mstone", "buff_mstone1":
			default:
				return reject()
			}
		}
	}
	// 冻结计算来源含具体自动切换时点，防止对照时偷换driver或演员。
	view := *input
	view.Tables = nil
	raw, err := json.Marshal(&view)
	if err != nil || p6Digest(raw) != out.CalculationSourceSHA256 {
		return reject()
	}
	return nil
}
