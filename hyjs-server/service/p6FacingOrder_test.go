package service

import (
	"reflect"
	"testing"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func TestP6OriginalFacingControlsSameTickPlayerOrderIndependently(t *testing.T) {
	s := p6OriginalTables(t)
	for _, facing := range []bool{false, true} {
		for _, first := range []bool{false, true} {
			input := p6Input(t, s, p6Actor("hero_047", "attacker", "047_1", 1, 1000000), p6Actor("hero_047", "defender", "047_1", 1, 1000000), 4)
			input.IsAttToDef, input.DefenderFirst = facing, first
			if err := SetP6FirstOrder(input.Attacker, input.Defender, first); err != nil {
				t.Fatal(err)
			}
			e := &combatRoundEngine{service: s, input: input, rng: NewCombatRandom(0), operate: &pb.BattleOperate{}, supplied: &pb.BattleOperate{}, round: 1}
			if err := e.initialize(); err != nil {
				t.Fatal(err)
			}
			// Isolate original FPlayer.Update with two pending releases in the same
			// frame. Initiative governs AI, while the facing flag governs both updates.
			for _, p := range e.players {
				e.beginSkill(p.actors[0].skill(0))
			}
			e.events = nil
			e.time = 4
			if err := e.step(); err != nil {
				t.Fatal(err)
			}
			var camps []int32
			for _, event := range e.events {
				if event.Kind == "release" && (len(camps) == 0 || camps[len(camps)-1] != event.Camp) {
					camps = append(camps, event.Camp)
				}
			}
			expected := []int32{0, 1}
			if facing {
				expected = []int32{1, 0}
			}
			if !reflect.DeepEqual(camps, expected) {
				t.Fatal("original same-frame facing update order drift", facing, first, camps)
			}
		}
	}
}

func TestP6OriginalFacingFreezeAndWholeReplay(t *testing.T) {
	s := p6OriginalTables(t)
	for _, facing := range []bool{false, true} {
		input := p6Input(t, s, p6Actor("hero_047", "attacker", "047_1", 1, 12000), p6Actor("hero_047", "defender", "047_1", 1, 12000), 6)
		input.IsAttToDef = facing
		frozen, err := s.FreezeBattleInput(*input)
		if err != nil || frozen.IsAttToDef != facing {
			t.Fatal("original facing not frozen", err)
		}
		out, err := s.ComputeFrozenBattle(t.Context(), frozen, nil)
		if err != nil {
			t.Fatal(err)
		}
		replay, err := s.ComputeFrozenBattle(t.Context(), frozen, out.Operates)
		if err != nil || !reflect.DeepEqual(out.Events, replay.Events) || out.RandomDraws != replay.RandomDraws || !proto.Equal(out.Operates, replay.Operates) || out.Winner != replay.Winner {
			t.Fatal("original facing whole replay drift", facing, err)
		}
		if len(out.Events) == 0 {
			t.Fatal("original facing no events")
		}
	}
}

func TestP6FacingIdentityRetryCannotReplaceFrozenDirection(t *testing.T) {
	s, ctx, id, input := p6Recorder(t)
	var recordID string
	if err := p6Tx(ctx, id, func(tx *gorm.DB) error {
		var e error
		recordID, e = s.CaptureBattleReplay(ctx, tx, id, "facing-identity", "original", *input)
		return e
	}); err != nil {
		t.Fatal(err)
	}
	var before db.P6BattleRecord
	if err := db.GetIns().First(&before, "id=?", recordID).Error; err != nil {
		t.Fatal(err)
	}
	input.IsAttToDef = true
	if err := p6Tx(ctx, id, func(tx *gorm.DB) error {
		_, e := s.CaptureBattleReplay(ctx, tx, id, "facing-identity", "original", *input)
		return e
	}); err == nil {
		t.Fatal("frozen direction changed by capture retry")
	}
	var after db.P6BattleRecord
	if err := db.GetIns().First(&after, "id=?", recordID).Error; err != nil || before.InputHash != after.InputHash || !reflect.DeepEqual(before.Input, after.Input) {
		t.Fatal("rejected facing retry mutated original snapshot", err)
	}
}
