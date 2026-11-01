package service

import (
	"errors"
	"math"
	"strings"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

type copyContinuityIdentity struct {
	guid string
	id   string
}

// copyContinuityFormationGuard records only identities. Equipment, positions,
// accepted HP and anger remain governed by their existing independent paths.
type copyContinuityFormationGuard struct {
	target map[copyContinuityIdentity]int
	before map[copyContinuityIdentity]int
}

// copyContinuityFormationIdentities follows battleFormationActors' slot and
// owned-warrior traversal, including duplicate counts in an old malformed save.
func copyContinuityFormationIdentities(game *model.GameState) map[copyContinuityIdentity]int {
	result := map[copyContinuityIdentity]int{}
	for _, slot := range game.Formation {
		for _, thing := range slot.GetThings() {
			if thing.GetPos() != 0 {
				continue
			}
			for _, warrior := range game.Warriors {
				if warrior.GetGuid() == thing.GetGuid() {
					result[copyContinuityIdentity{guid: warrior.GetGuid(), id: warrior.GetId()}]++
				}
			}
		}
	}
	return result
}

// copyContinuityGuardFormation captures the established roster under the same
// profile lock as the requested formation mutation. Pre-upgrade pending fights
// use the same non-demo roster as copyContinuityAccepted. An ambiguous legacy
// snapshot keeps its existing behavior rather than guessing an identity.
func copyContinuityGuardFormation(player *pb.Player, game *model.GameState) *copyContinuityFormationGuard {
	if player == nil || game == nil || game.Run == nil || game.Run.Complete {
		return nil
	}
	party := game.Run.Party
	legacyFight := false
	if party == nil {
		fight := game.Run.Fight
		if fight == nil || fight.Settled || fight.Start == nil {
			return nil
		}
		party = fight.Start.BattlePlayer
		legacyFight = true
	}
	if party == nil || player.GetPlayerId() <= 0 || party.GetPlayerId() != player.GetPlayerId() {
		return nil
	}
	target := map[copyContinuityIdentity]int{}
	seen := map[string]bool{}
	for _, hero := range party.GetFormation() {
		if hero == nil || hero.GetGuid() == "" {
			return nil
		}
		if legacyFight && strings.HasPrefix(hero.GetGuid(), "demo-") {
			continue
		}
		if hero.GetId() == "" || seen[hero.GetGuid()] {
			return nil
		}
		seen[hero.GetGuid()] = true
		target[copyContinuityIdentity{guid: hero.GetGuid(), id: hero.GetId()}]++
	}
	if len(target) == 0 {
		return nil
	}
	return &copyContinuityFormationGuard{target: target, before: copyContinuityFormationIdentities(game)}
}

// check permits an unchanged roster or a step toward the already frozen roster.
// It never changes an existing run; a previously inconsistent save can be
// repaired one original hero at a time while new divergence is rejected.
func (guard *copyContinuityFormationGuard) check(game *model.GameState) error {
	if guard == nil {
		return nil
	}
	after := copyContinuityFormationIdentities(game)
	keys := map[copyContinuityIdentity]bool{}
	for identity := range guard.target {
		keys[identity] = true
	}
	for identity := range guard.before {
		keys[identity] = true
	}
	for identity := range after {
		keys[identity] = true
	}
	for identity := range keys {
		before, current, target := guard.before[identity], after[identity], guard.target[identity]
		if before <= target && (current < before || current > target) || before > target && (current > before || current < target) {
			return errors.New(constUtil.CopyContinuityLineupChanged)
		}
	}
	return nil
}

// copyContinuityParty rebuilds legal attributes while carrying this run's accepted
// HP and anger. An old run without Party receives its historical fresh start once.
func (s *GameService) copyContinuityParty(player *pb.Player, game *model.GameState) (*pb.BattlePlayer, error) {
	if game.Run == nil {
		return nil, errors.New(constUtil.CopyContinuityInvalid)
	}
	fresh, err := s.battlePlayer(player, game)
	if err != nil {
		return nil, err
	}
	if game.Run.Party == nil {
		game.Run.Party = proto.Clone(fresh).(*pb.BattlePlayer)
		return fresh, nil
	}
	return s.copyContinuityRestore(fresh, game.Run.Party)
}

func (s *GameService) copyContinuityAnger(party *pb.BattlePlayer) error {
	maximum := tableNumber(s.table("GameConfigure", "StaticName", "UTIL_MAXANGER"), "NameValue")
	anger := party.GetInst().GetAnger()
	if party == nil || party.Inst == nil || !combatFinite(maximum) || maximum <= 0 || !combatFinite(anger) || anger < 0 || anger > maximum {
		return errors.New(constUtil.CopyContinuityInvalid)
	}
	return nil
}

func (s *GameService) copyContinuityRestore(fresh, saved *pb.BattlePlayer) (*pb.BattlePlayer, error) {
	if fresh == nil || saved == nil || fresh.GetPlayerId() != saved.GetPlayerId() || s.copyContinuityAnger(saved) != nil {
		return nil, errors.New(constUtil.CopyContinuityInvalid)
	}
	previous := map[string]*pb.BattleHero{}
	for _, hero := range saved.Formation {
		if hero == nil || hero.GetGuid() == "" || previous[hero.GetGuid()] != nil {
			return nil, errors.New(constUtil.CopyContinuityInvalid)
		}
		previous[hero.GetGuid()] = hero
	}
	if len(fresh.Formation) != len(previous) {
		return nil, errors.New(constUtil.CopyContinuityLineupChanged)
	}
	result := proto.Clone(fresh).(*pb.BattlePlayer)
	seen := map[string]bool{}
	for _, hero := range result.Formation {
		old := previous[hero.GetGuid()]
		if old == nil || seen[hero.GetGuid()] || old.GetId() != hero.GetId() {
			return nil, errors.New(constUtil.CopyContinuityLineupChanged)
		}
		seen[hero.GetGuid()] = true
		hp := old.GetInst().GetCurHp()
		if hero.Inst == nil || !combatFinite(hp) || hp < 0 || !combatFinite(hero.GetProp().GetHp()) || hero.GetProp().GetHp() <= 0 {
			return nil, errors.New(constUtil.CopyContinuityInvalid)
		}
		hero.Inst.CurHp = proto.Float64(math.Min(hp, hero.GetProp().GetHp()))
	}
	result.Inst.Anger = proto.Float64(saved.GetInst().GetAnger())
	return result, nil
}

// copyContinuityAccepted retains the existing observed settlement contract. Only
// accepted runtime HP/anger are copied onto the frozen server identity/attributes;
// this function does not certify a server-computed battle result.
func (s *GameService) copyContinuityAccepted(run *model.CopyRun, reported *pb.BattlePlayer) (*pb.BattlePlayer, error) {
	if run == nil || run.Fight == nil || run.Fight.Start == nil || s.copyContinuityAnger(reported) != nil {
		return nil, errors.New(constUtil.CopyContinuityInvalid)
	}
	source := run.Fight.Start.BattlePlayer
	if source == nil || reported.GetPlayerId() != source.GetPlayerId() {
		return nil, errors.New(constUtil.CopyContinuityInvalid)
	}
	allowed := map[string]bool{}
	if run.Party != nil {
		for _, hero := range run.Party.Formation {
			allowed[hero.GetGuid()] = true
		}
	} else {
		// Pre-upgrade in-flight sources retain their original player roster;
		// task-local demo actors have always used this server-owned GUID prefix.
		for _, hero := range source.Formation {
			if !strings.HasPrefix(hero.GetGuid(), "demo-") {
				allowed[hero.GetGuid()] = true
			}
		}
	}
	observed := map[string]*pb.BattleHero{}
	for _, hero := range reported.Formation {
		if hero == nil || observed[hero.GetGuid()] != nil {
			return nil, errors.New(constUtil.CopyContinuityInvalid)
		}
		observed[hero.GetGuid()] = hero
	}
	result := proto.Clone(source).(*pb.BattlePlayer)
	result.Formation = nil
	for _, hero := range source.Formation {
		if !allowed[hero.GetGuid()] {
			continue
		}
		value := observed[hero.GetGuid()]
		hp := value.GetInst().GetCurHp()
		if value == nil || value.GetId() != hero.GetId() || !combatFinite(hp) || hp < 0 || hp > hero.GetProp().GetHp() {
			return nil, errors.New(constUtil.CopyContinuityInvalid)
		}
		actor := proto.Clone(hero).(*pb.BattleHero)
		actor.Inst.CurHp = proto.Float64(hp)
		result.Formation = append(result.Formation, actor)
	}
	if len(result.Formation) != len(allowed) {
		return nil, errors.New(constUtil.CopyContinuityInvalid)
	}
	result.Inst.Anger = proto.Float64(reported.GetInst().GetAnger())
	return result, nil
}
