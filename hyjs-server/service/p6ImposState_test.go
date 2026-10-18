package service

import (
	"reflect"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p6OriginalImposBossInput(t *testing.T, s *GameService) *model.FrozenBattleInput {
	t.Helper()
	input := p6OriginalMultiBarInput(t, s)
	actor := p6Actor("hero_001", "immune-boss-player", "001_1,001_2,001_3", 1, 40000)
	actor.Prop.Cri = proto.Float64(100)
	input.Attacker.Formation = []*pb.BattleHero{actor}
	frozen, err := s.FreezeBattleInput(*input)
	if err != nil {
		t.Fatal(err)
	}
	return frozen
}

func TestP6OriginalImposStateBossCriticalWholeBattleAndOperationReplay(t *testing.T) {
	s := p6OriginalTables(t)
	input := p6OriginalImposBossInput(t, s)
	if s.table("Hero", "id", "monster_035")["ImposState"] != "2" {
		t.Fatal("original boss immune-state row changed")
	}
	out, err := s.ComputeFrozenBattle(t.Context(), input, nil)
	if err != nil {
		t.Fatal(err)
	}
	criticalFloatCast := false
	for _, event := range out.Events {
		if event.Kind == "special_begin" && event.Target == "1_original-boss" && event.Value == 2 {
			t.Fatal("original ImposState2 boss became floating", event)
		}
		if event.Kind == "release" && event.Actor == "0_immune-boss-player" && event.Cast == "3" && event.Critical {
			criticalFloatCast = true
		}
	}
	if !criticalFloatCast || out.Winner != 0 || out.Authoritative || out.Monster.Formation[0].GetBloodNumber() != 2 {
		t.Fatal("original immune multi-bar boss whole fight incomplete", criticalFloatCast, out.Winner)
	}
	replayed, err := s.ComputeFrozenBattle(t.Context(), input, out.Operates)
	if err != nil || !proto.Equal(out.Operates, replayed.Operates) || !proto.Equal(out.Attacker, replayed.Attacker) || !proto.Equal(out.Monster, replayed.Monster) || out.RandomDraws != replayed.RandomDraws || !reflect.DeepEqual(out.Events, replayed.Events) {
		t.Fatal("original immune-state whole replay changed", err)
	}
	// Removing only the original immunity must expose the real critical float path.
	// Frozen original tables stay intact while a separate diagnostic variant is computed.
	s.table("Hero", "id", "monster_035")["ImposState"] = "0"
	variant, err := s.FreezeBattleInput(*input)
	if err != nil {
		t.Fatal(err)
	}
	other, err := s.ComputeFrozenBattle(t.Context(), variant, nil)
	if err != nil {
		t.Fatal(err)
	}
	floated := false
	for _, event := range other.Events {
		if event.Kind == "special_begin" && event.Target == "1_original-boss" && event.Value == 2 {
			floated = true
		}
	}
	if !floated || input.ConfigurationHash == variant.ConfigurationHash || (&GameService{Tables: input.Tables}).table("Hero", "id", "monster_035")["ImposState"] != "2" {
		t.Fatal("immune-state delta did not preserve the original frozen boss", floated)
	}
}

func TestP6OriginalCommaImposStatesFloatBreakAndRejectedUnknowns(t *testing.T) {
	for _, blocked := range []string{"0", "2", "3", "2,3"} {
		s := p6OriginalTables(t)
		s.table("Hero", "id", "hero_047")["ImposState"] = blocked
		input := p6Input(t, s, p6Actor("hero_001", "source", "001_1", 1, 1000), p6Actor("hero_047", "target", "047_1", 1, 1000), 1)
		e := &combatRoundEngine{service: &GameService{Tables: input.Tables}, input: input, rng: NewCombatRandom(0), operate: &pb.BattleOperate{}}
		if err := e.initialize(); err != nil {
			t.Fatal(err)
		}
		a, d := e.players[0].actors[0], e.players[1].actors[0]
		e.specialHit(a, d, 2)
		if d.unableStates[2] {
			if d.speState != 1 || d.speUntil != 0 {
				t.Fatal("forbidden Float changed state or timer", blocked)
			}
		} else if d.speState != 2 {
			t.Fatal("allowed original Float was suppressed", blocked)
		}
		beforeState, beforeEnd := d.speState, d.speUntil
		e.specialHit(a, d, 3)
		if d.unableStates[3] {
			if d.speState != beforeState || d.speUntil != beforeEnd {
				t.Fatal("forbidden Break altered the running Float", blocked)
			}
		} else if d.speState != 3 {
			t.Fatal("Float immunity also suppressed allowed Break", blocked)
		}
	}
	for _, unknown := range []string{"", "4", "-1", "2,,3", "floating"} {
		s := p6OriginalTables(t)
		s.table("Hero", "id", "hero_047")["ImposState"] = unknown
		input := p6Input(t, s, p6Actor("hero_001", "source", "001_1", 1, 1000), p6Actor("hero_047", "target", "047_1", 1, 1000), 1)
		if _, err := s.ComputeFrozenBattle(t.Context(), input, nil); err == nil {
			t.Fatal("unknown original forbidden-state input accepted", unknown)
		}
	}
}
