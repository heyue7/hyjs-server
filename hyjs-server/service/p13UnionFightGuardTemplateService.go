package service

import (
	"errors"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func p13UFGuardTemplateRuleKnown(rule string) bool {
	return rule == "" || rule == constUtil.P13UFGuardMirrorRule
}

func p13UFGuardTemplatePolicyValid(policy *db.P13UnionFightFieldPolicy) bool {
	if policy == nil || !p13UFGuardTemplateRuleKnown(policy.GuardRule) {
		return false
	}
	if policy.GuardRule == constUtil.P13UFGuardMirrorRule {
		return policy.GuardHPSource == constUtil.P13UnionFightGuardHPSourceMonster && policy.InitialGuardHP == 0
	}
	return policy.GuardHPSource == "" && policy.InitialGuardHP > 0 || policy.GuardHPSource == constUtil.P13UnionFightGuardHPSourceMonster && policy.InitialGuardHP == 0
}

// The original guard IDs remain protocol identities. Only the named new rule
// changes the entire combat hero; old snapshots keep their original mapping.
func p13UFGuardTemplateHeroID(guardID, originalHeroID, rule string) (string, error) {
	if !p13UFGuardTemplateRuleKnown(rule) {
		return "", errors.New(constUtil.P13UFGuardConflict)
	}
	if rule == "" {
		if originalHeroID == "" {
			return "", errors.New(constUtil.P13UFGuardConflict)
		}
		return originalHeroID, nil
	}
	switch guardID {
	case "baseCamp_guard_001", "baseCamp_guard_002":
		return constUtil.P13UFGuardMirrorBaseHero, nil
	case "tower_guard_001", "tower_guard_002", "tower_guard_003", "tower_guard_004", "tower_guard_005", "tower_guard_006", "factory_guard_001", "factory_guard_002":
		return constUtil.P13UFGuardMirrorOrdinaryHero, nil
	default:
		return "", errors.New(constUtil.P13UFGuardConflict)
	}
}

// P13UnionFightMirrorGuardTemplate constructs the full prepublication source
// from original Hero/skills. Level1/blood1/pose3 are the explicit local entity
// contract; awakening/gear/talent upgrades are not fabricated from potential rows.
func (s *GameService) P13UnionFightMirrorGuardTemplate(guardID string) (*pb.BattleMonster, error) {
	definition := s.table("UnionFightGuard", "UFGuardId", guardID)
	if definition == nil {
		return nil, errors.New(constUtil.P13UFGuardConflict)
	}
	heroID, err := p13UFGuardTemplateHeroID(guardID, definition["HeroId"], constUtil.P13UFGuardMirrorRule)
	if err != nil {
		return nil, err
	}
	hero, err := s.actor(s.table("Hero", "id", heroID), "mirror-guard/"+guardID, 3, false)
	if err != nil {
		return nil, err
	}
	hero.Level, hero.BloodNumber = proto.Int32(1), proto.Int32(1)
	hp, att, pdef, mdef := float64(constUtil.P13UFGuardMirrorOrdinaryHP), float64(constUtil.P13UFGuardMirrorOrdinaryATT), float64(constUtil.P13UFGuardMirrorOrdinaryDEF), float64(constUtil.P13UFGuardMirrorOrdinaryDEF)
	if heroID == constUtil.P13UFGuardMirrorBaseHero {
		hp, att, pdef, mdef = constUtil.P13UFGuardMirrorBaseHP, constUtil.P13UFGuardMirrorBaseATT, constUtil.P13UFGuardMirrorBasePDEF, constUtil.P13UFGuardMirrorBaseMDEF
	}
	if hero.Prop == nil || hero.Inst == nil || hero.Prop.GetHp() != hp || hero.Prop.GetAtt() != att || hero.Prop.GetPdef() != pdef || hero.Prop.GetMdef() != mdef || hero.Inst.GetCurHp() != hp {
		return nil, errors.New(constUtil.P13UFGuardConflict)
	}
	return &pb.BattleMonster{MonsterId: proto.Int64(1), ServerId: proto.Int32(int32(s.OriginalServerID())), TaskId: proto.String(guardID), GroupIndex: proto.Int32(1), Formation: []*pb.BattleHero{hero}, Inst: &pb.BattleMonsterInst{Anger: proto.Float64(0)}}, nil
}

func p13UFGuardMirrorSource(source *db.P13UnionFightGuardAuditSource, original *GameService, match *db.P13UnionFightMatch, now time.Time) (*pb.BattleMonster, error) {
	if source == nil || original == nil || match == nil || source.GuardRule != constUtil.P13UFGuardMirrorRule || source.Day != match.Day || !source.Enabled || source.PublishedAt <= 0 || source.PublishedAt > now.UnixMilli() || source.PublishedAt >= match.StartAt || strings.TrimSpace(source.Source) == "" {
		return nil, errors.New(constUtil.P13UFGuardConflict)
	}
	actual := &pb.BattleMonster{}
	if proto.Unmarshal(source.Monster, actual) != nil || len(actual.Formation) != 1 || actual.Inst == nil || actual.Inst.GetAnger() != 0 || actual.GetTaskId() != source.GuardID || actual.GetGroupIndex() != 1 || actual.GetMonsterId() <= 0 || actual.GetServerId() != int32(original.OriginalServerID()) {
		return nil, errors.New(constUtil.P13UFGuardConflict)
	}
	expected, err := original.P13UnionFightMirrorGuardTemplate(source.GuardID)
	if err != nil {
		return nil, err
	}
	hero := actual.Formation[0]
	if hero == nil || hero.GetGuid() == "" {
		return nil, errors.New(constUtil.P13UFGuardConflict)
	}
	expected.Formation[0].Guid = hero.Guid
	if !proto.Equal(expected.Formation[0], hero) || !proto.Equal(expected.Inst, actual.Inst) {
		return nil, errors.New(constUtil.P13UFGuardConflict)
	}
	return actual, nil
}

// Existing map snapshots never reread this policy. New mirror maps validate
// all full sources before freezing any building/offer, not just their HP.
func p13UFGuardTemplateHP(tx *gorm.DB, match *db.P13UnionFightMatch, policy *db.P13UnionFightFieldPolicy, original *GameService, now time.Time) (map[string]float64, error) {
	result := map[string]float64{}
	reject := func() (map[string]float64, error) {
		return nil, errors.New("守卫独立生命模板来源不完整或未在开场前发布")
	}
	if policy == nil || !p13UFGuardTemplateRuleKnown(policy.GuardRule) {
		return reject()
	}
	if policy.GuardHPSource == "" && policy.GuardRule == "" {
		return result, nil
	}
	if policy.GuardHPSource != constUtil.P13UnionFightGuardHPSourceMonster || policy.InitialGuardHP != 0 {
		return reject()
	}
	var sources []db.P13UnionFightGuardAuditSource
	if err := tx.Where("day=? AND enabled=1", match.Day).Find(&sources).Error; err != nil {
		return nil, err
	}
	if len(sources) != 10 {
		return reject()
	}
	for _, source := range sources {
		definition := original.table("UnionFightGuard", "UFGuardId", source.GuardID)
		monster := &pb.BattleMonster{}
		if policy.GuardRule == constUtil.P13UFGuardMirrorRule {
			var err error
			monster, err = p13UFGuardMirrorSource(&source, original, match, now)
			if err != nil {
				return nil, err
			}
		} else {
			if source.GuardRule != "" || definition == nil || source.PublishedAt <= 0 || source.PublishedAt > now.UnixMilli() || source.PublishedAt >= match.StartAt || strings.TrimSpace(source.Source) == "" || proto.Unmarshal(source.Monster, monster) != nil || len(monster.Formation) != 1 || monster.Inst == nil || monster.Inst.GetAnger() != 0 {
				return reject()
			}
			hero := monster.Formation[0]
			if hero == nil || hero.Prop == nil || hero.Inst == nil || hero.GetId() != definition["HeroId"] || hero.GetLevel() <= 0 || hero.GetBloodNumber() != 1 || !combatFinite(hero.Prop.GetHp()) || hero.Prop.GetHp() <= 0 || hero.Prop.GetHp() > 1e15 || hero.Inst.GetCurHp() != hero.Prop.GetHp() {
				return reject()
			}
		}
		if _, exists := result[source.GuardID]; exists {
			return reject()
		}
		result[source.GuardID] = monster.Formation[0].Prop.GetHp()
	}
	return result, nil
}
