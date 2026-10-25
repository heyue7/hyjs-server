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
