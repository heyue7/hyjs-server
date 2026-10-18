package service

import (
	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"reflect"
	"testing"
)

func p6MaxHPPercentInput(t *testing.T, s *GameService) *model.FrozenBattleInput {
	input := p6Input(t, s, p6Actor("hero_017", "maxhp-percent-source", "017_1,017_2,017_3", 1, 80000), p6Actor("hero_018", "maxhp-percent-foe", "018_1,018_2,018_3", 1, 80000), 18)
	input.Attacker.Inst.Anger = proto.Float64(100)
	input.Defender.Inst.Anger = proto.Float64(100)
	// This explicit mechanism fixture seed covers both native 25%/10% HP-percent casts.
	// The original probabilities and character/skill/Buff rows remain unchanged.
	input.Seed = 42
	return input
}

func TestP6OriginalMaxHPPercentCurrentSkillCapAndPhysicalShieldOrder(t *testing.T) {
	s := p6OriginalTables(t)
	for _, tc := range []struct {
		attackType int32
		damage     float64
	}{{1, 1200}, {2, 1400}, {3, 1000}} {
		input := p6MaxHPPercentInput(t, s)
		e := &combatRoundEngine{service: &GameService{Tables: input.Tables}, input: input, rng: NewCombatRandom(0), operate: &pb.BattleOperate{}}
		if err := e.initialize(); err != nil {
			t.Fatal(err)
		}
		a, d := e.players[0].actors[0], e.players[1].actors[0]
		a.base["att"], a.base["patt"], a.base["matt"] = 500, 100, 200
		a.current = &combatRoundSkill{attackType: tc.attackType}
		for _, row := range []map[string]string{
			{"Id": "percent-physical-probe", "Factor": "pshield", "Args1": "200", "TotalRound": "2"},
			{"Id": "percent-common-probe", "Factor": "shield", "Args1": "100", "TotalRound": "2"},
			{"Id": "percent-magic-probe", "Factor": "mshield", "Args1": "400", "TotalRound": "2"},
		} {
			if err := e.loadBuff(a, d, row, 4); err != nil {
				t.Fatal(err)
			}
		}
		before := d.hp
		if err := e.loadBuff(a, d, e.service.table("Buff", "Id", "buff_156"), 12); err != nil {
			t.Fatal(err)
		}
		closeCombat(t, d.hp, before-tc.damage+300)
		closeCombat(t, d.attribute("maxHp"), 80000)
		closeCombat(t, d.shield("pshield"), 0)
		closeCombat(t, d.shield("shield"), 0)
		closeCombat(t, d.shield("mshield"), 400)
		closeCombat(t, d.buffs[len(d.buffs)-1].value, -tc.damage)
		if e.rng.Draws() != 0 {
			t.Fatal("native maxHP-percent consumed randomness")
		}
	}
}

func TestP6OriginalMaxHPPercentSmallTargetCapHealAndWeakGuard(t *testing.T) {
	s := p6OriginalTables(t)
	input := p6MaxHPPercentInput(t, s)
	e := &combatRoundEngine{service: &GameService{Tables: input.Tables}, input: input, rng: NewCombatRandom(0), operate: &pb.BattleOperate{}}
	if err := e.initialize(); err != nil {
		t.Fatal(err)
	}
	a, d := e.players[0].actors[0], e.players[1].actors[0]
	a.base["att"], a.base["patt"] = 500, 100
	d.base["maxHp"], d.hp = 1000, 500
	if err := e.loadBuff(a, d, e.service.table("Buff", "Id", "buff_156"), 12); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, d.hp, 350)
	// The original positive branch is capped as well and retains AddHp recovery rules.
	row := map[string]string{"Id": "positive-native-percent-probe", "BuffClass": "0", "Factor": "minusMaxHpPercent", "Args1": "50", "Args2": "10"}
	if err := e.loadBuff(a, d, row, 12); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, d.hp, 410)
	if err := e.loadBuff(a, d, map[string]string{"Id": "native-invincible-percent-probe", "Factor": "invincible", "TotalRound": "2"}, 4); err != nil {
		t.Fatal(err)
	}
	if err := e.loadBuff(a, d, e.service.table("Buff", "Id", "buff_156"), 12); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, d.hp, 260)
	d.state = 3
	if err := e.loadBuff(a, d, e.service.table("Buff", "Id", "buff_156"), 12); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, d.hp, 260)
	closeCombat(t, d.attribute("maxHp"), 1000)
}

func TestP6OriginalHero017And018FullMaxHPPercentWholeBattleAndReplay(t *testing.T) {
	s := p6OriginalTables(t)
	input := p6MaxHPPercentInput(t, s)
	out, err := s.ComputeFrozenBattle(t.Context(), input, nil)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	changes := 0
	changingActors := map[string]bool{}
	for _, event := range out.Events {
		if event.Kind == "release" {
			seen[event.Actor+":"+event.Skill] = true
		}
		if event.Kind == "maxhp_percent_change" {
			changes++
			changingActors[event.Actor] = true
			if event.Value >= 0 {
				t.Fatal("original negative percent changed sign", event)
			}
		}
	}
	if len(seen) != 6 || changes < 2 || len(changingActors) != 2 || out.Authoritative {
		t.Fatal("complete original headshot roles missing", seen, changes, changingActors)
	}
	replayed, err := s.ComputeFrozenBattle(t.Context(), input, out.Operates)
	if err != nil || !proto.Equal(out.Operates, replayed.Operates) || !proto.Equal(out.Attacker, replayed.Attacker) || !proto.Equal(out.Defender, replayed.Defender) || out.RandomDraws != replayed.RandomDraws || !reflect.DeepEqual(out.Events, replayed.Events) {
		t.Fatal("maxHP-percent whole original replay diverged", err)
	}
}

func TestP6MaxHPPercentRejectsFactoryAndNegativeCap(t *testing.T) {
	for _, tc := range []struct{ field, value string }{{"BuffClass", "1"}, {"Args2", "-1"}} {
		s := p6OriginalTables(t)
		s.table("Buff", "Id", "buff_156")[tc.field] = tc.value
		if _, err := s.ComputeFrozenBattle(t.Context(), p6MaxHPPercentInput(t, s), nil); err == nil {
			t.Fatal("unsupported native HP-percent accepted", tc)
		}
	}
}
