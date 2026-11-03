package service

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

type formationActiveTalent struct {
	node, talent map[string]string
}

type formationTalentSelection struct {
	current map[string]string
	hasTree bool
	active  []formationActiveTalent
}

type formationSharedBuff struct {
	row       map[string]string
	condition string
}

type formationSharedMember struct {
	slot      *pb.FormationItem
	warrior   *pb.Warrior
	selection formationTalentSelection
}

// formationTalents selects the same active trunk and branch rows used for an
// actor's own growth. The immutable row references are reused within this call.
func (s *GameService) formationTalents(warrior *pb.Warrior) (formationTalentSelection, error) {
	var selection formationTalentSelection
	nodes, indexed := s.indexedTableGroup("TalentAdvenced", "warId", warrior.GetId())
	if !indexed {
		nodes = s.Tables["TalentAdvenced"]
	}
	for _, node := range nodes {
		if node["warId"] != warrior.GetId() {
			continue
		}
		selection.hasTree = true
		if node["trunck"] == "1" {
			tier, err := strconv.ParseInt(node["talentLevel"], 10, 32)
			if err != nil {
				return selection, err
			}
			if int32(tier) == warrior.GetTalent() {
				selection.current = node
			}
			if int32(tier) <= warrior.GetTalent() {
				selection.active = append(selection.active, formationActiveTalent{node: node})
			}
		} else if slices.Contains(warrior.GetTalentBranch(), node["talentLevel"]) {
			selection.active = append(selection.active, formationActiveTalent{node: node})
		}
	}
	for i := range selection.active {
		active := &selection.active[i]
		if id := active.node["talentId"]; id != "" {
			active.talent = s.table("Talent", "Id", id)
			if active.talent == nil {
				return selection, errors.New("Missing warrior talent")
			}
		}
	}
	return selection, nil
}

// formationSharedTalent retains each permanent team effect in source order;
// the original client adds repeated effects and includes their owner.
func (s *GameService) formationSharedTalent(shared []formationSharedBuff, talent map[string]string, missingError string) ([]formationSharedBuff, error) {
	for i := 0; i < 3; i++ {
		key := fmt.Sprintf("Buff%d", i)
		if talent[key] == "" || tableNumber(talent, key+"TriggerTimer") != 0 || tableNumber(talent, key+"Select") != 110 {
			continue
		}
		buff := s.table("Buff", "Id", talent[key])
		if buff == nil {
			return nil, errors.New(missingError)
		}
		shared = append(shared, formationSharedBuff{row: buff, condition: strings.TrimSpace(talent[key+"ConditionArgs"])})
	}
	return shared, nil
}

// formationSharedSources collects only mounted heroes and selects each source's
// active nodes once. Actor projections reuse those selections and the buffs.
func (s *GameService) formationSharedSources(game *model.GameState) ([]formationSharedMember, []formationSharedBuff, error) {
	var members []formationSharedMember
	var shared []formationSharedBuff
	for _, slot := range game.Formation {
		for _, thing := range slot.GetThings() {
			if thing.GetPos() != 0 {
				continue
			}
			for _, warrior := range game.Warriors {
				if warrior.GetGuid() != thing.GetGuid() {
					continue
				}
				selection, err := s.formationTalents(warrior)
				if err != nil {
					return nil, nil, err
				}
				for i := range selection.active {
					active := &selection.active[i]
					if active.talent != nil {
						shared, err = s.formationSharedTalent(shared, active.talent, "Missing warrior talent buff")
						if err != nil {
							return nil, nil, err
						}
					}
				}
				if warrior.GetAwake().GetStage() >= 1 {
					awake := s.awakeTable("HeroAwakenAdd", warrior.GetId(), 1)
					if awake == nil {
						return nil, nil, errors.New(constUtil.WarriorAwakeConfig)
					}
					if id := awake["talentId"]; id != "" {
						talent := s.table("Talent", "Id", id)
						if talent == nil {
							return nil, nil, errors.New(constUtil.WarriorAwakeConfig)
						}
						shared, err = s.formationSharedTalent(shared, talent, constUtil.WarriorAwakeConfig)
						if err != nil {
							return nil, nil, err
						}
					}
				}
				members = append(members, formationSharedMember{slot: slot, warrior: warrior, selection: selection})
			}
		}
	}
	return members, shared, nil
}

// formationSharedBuffFits follows UIHeroFormation.BuffFit's space predicate.
func formationSharedBuffFits(condition string, hero map[string]string) bool {
	if condition == "" {
		return true
	}
	const prefix = "type:space,src1:2,op:"
	if !strings.HasPrefix(condition, prefix) || len(condition) <= len(prefix) {
		return false
	}
	operator, target := condition[len(prefix)], condition[len(condition)-1]
	if target < '0' || target > '9' {
		return false
	}
	space, value := tableNumber(hero, "space"), float64(target-'0')
	switch operator {
	case '1':
		return space > value
	case '2':
		return space >= value
	case '3':
		return space == value
	case '4':
		return space <= value
	case '5':
		return space < value
	default:
		return false
	}
}
