package service

import (
	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestP6OriginalAliveCountWeakFinalDeathBothActualHighTierOwners(t *testing.T) {
	for _, tc := range []struct{ hero, talent string }{{"hero_017", "tal_286"}, {"hero_081", "tal_3235"}} {
		for _, camp := range []int{0, 1} {
			s := p6OriginalTables(t)
			row := s.table("Hero", "id", tc.hero)
			owner := p6Actor(tc.hero, "alive-owner", row["SkillId1"]+","+row["SkillId2"]+","+row["UltSkillId"], 1, 80000)
			owner.TalentList = []string{tc.talent}
			f := p6Input(t, s, owner, p6Actor("hero_047", "alive-enemy", "047_1,047_2,047_3", 1, 80000), 12)
			for pose := int32(2); pose <= 5; pose++ {
				f.Attacker.Formation = append(f.Attacker.Formation, p6Actor("hero_047", "alive-mate-"+strconv.Itoa(int(pose)), "047_1,047_2,047_3", pose, 80000))
			}
			if camp == 1 {
				f.Attacker, f.Defender = f.Defender, f.Attacker
			}
			e := &combatRoundEngine{service: &GameService{Tables: f.Tables}, input: f, rng: NewCombatRandom(0), operate: &pb.BattleOperate{}}
			if err := e.initialize(); err != nil {
				t.Fatal(err)
			}
			a, mate := e.players[camp].actors[0], e.players[camp].actors[4]
			before := a.attribute("att")
			// Original aliveNum follows final-death state, independent of weak HP zero.
			mate.state, mate.hp = 3, 0
			closeCombat(t, e.conditionProperty(a, "aliveNum"), 5)
			if err := e.talentTimer(a, e.players[1-camp].actors[0], 4); err != nil {
				t.Fatal(err)
			}
			closeCombat(t, a.attribute("att"), before)
			if e.rng.Draws() != 0 {
				t.Fatal("false alive condition consumed original chance RNG")
			}
			mate.state = 4
			closeCombat(t, e.conditionProperty(a, "aliveNum"), 4)
			if err := e.talentTimer(a, e.players[1-camp].actors[0], 4); err != nil {
				t.Fatal(err)
			}
			closeCombat(t, a.attribute("att"), before*1.3)
			if e.rng.Draws() != 1 || len(a.buffs) != 1 || a.buffs[0].row["Id"] != "buff_282" {
				t.Fatal("actual 10-1 alive talent failed its native chance/buff", tc, camp)
			}
		}
	}
}

func TestP6OriginalPairedRoundConditionAndUnmappedOddEvenTalent(t *testing.T) {
	e := p6SameDamageEngine(t)
	a, d := e.players[0].actors[0], e.players[1].actors[0]
	a.selfRounds, d.selfRounds = 30, 0
	row := e.service.table("Talent", "Id", "tal_189")
	for round := int32(0); round <= 6; round++ {
		e.round = round
		for _, actor := range []*combatRoundActor{a, d} {
			want := float64((round + 1) / 2)
			closeCombat(t, e.conditionProperty(actor, "selfRoundNum"), want)
			for slot := 0; slot < 2; slot++ {
				key := "Buff" + strconv.Itoa(slot)
				trigger := combatRoundTrigger{condition: 1, args: row[key+"ConditionArgs"]}
				if e.conditionTrue(trigger, actor, actor) != (int(want)%2 == 1-slot) {
					t.Fatal("original odd/even paired round differs across camps", round, actor.camp, slot)
				}
			}
		}
	}
}

func TestP6OriginalAttributeSelectorsReadLiveBuffsAndRetainTieOrder(t *testing.T) {
	for _, tc := range []struct {
		selector    int32
		key, debuff string
	}{{26, "att", "buff_729"}, {27, "pdef", "buff_730"}, {28, "mdef", "buff_730"}} {
		e := p6SameDamageEngine(t)
		a := e.players[0].actors[0]
		x, y, z := e.players[1].actors[0], e.players[1].actors[1], e.players[1].actors[2]
		x.base[tc.key], y.base[tc.key], z.base[tc.key] = 100, 200, 300
		assertTarget := func(want *combatRoundActor) {
			t.Helper()
			result, err := e.selectTargets(a, tc.selector, nil, nil)
			if err != nil || len(result) != 1 || result[0] != want || e.rng.Draws() != 0 {
				t.Fatal("live deterministic selection", tc.selector, err, result)
			}
		}
		if tc.selector == 26 {
			assertTarget(z)
		} else {
			assertTarget(x)
		}
		if err := e.loadBuff(a, z, e.service.table("Buff", "Id", tc.debuff), 16); err != nil {
			t.Fatal(err)
		}
		if tc.selector == 26 {
			assertTarget(y)
		} else {
			assertTarget(z)
		}
		// Suppression and reactivation must immediately change the selection ranking.
		z.buffs[0].suppressed = true
		if tc.selector == 26 {
			assertTarget(z)
		} else {
			assertTarget(x)
		}
		z.buffs[0].suppressed = false
		y.base[tc.key] = z.attribute(tc.key)
		if tc.selector == 26 {
			x.base[tc.key] = 0
		}
		assertTarget(y)
	}
}

func TestP6OriginalMaxMissingHPSelectionAndHigherThanSelfAllTargets(t *testing.T) {
	e := p6SameDamageEngine(t)
	a := e.players[0].actors[0]
	x, y, z := e.players[1].actors[0], e.players[1].actors[1], e.players[1].actors[2]
	a.hp, x.hp, y.hp, z.hp = 15000, 15000, 10000, 20000
	selectOne := func(want *combatRoundActor) {
		t.Helper()
		result, err := e.selectTargets(a, 31, nil, nil)
		if err != nil || len(result) != 1 || result[0] != want {
			t.Fatal("native current maximum missing HP selection", err)
		}
	}
	selectOne(y)
	// Exercise the native current attribute boundary without fabricating a max-HP buff.
	// Original minusMaxHpPercent is HP damage and does not change maximum HP.
	y.base["maxHp"] = 72000
	selectOne(x)
	result, err := e.selectTargets(a, 25, nil, nil)
	if err != nil || len(result) != 1 || result[0] != z {
		t.Fatal("native more-HP-than-self strict threshold", err)
	}
	y.hp = 16000
	result, err = e.selectTargets(a, 25, nil, nil)
	if err != nil || len(result) != 2 || result[0] != y || result[1] != z || e.rng.Draws() != 0 {
		t.Fatal("native more-HP-than-self includes all eligible targets without RNG", err)
	}
	a.hp = 80000
	result, err = e.selectTargets(a, 25, nil, nil)
	if err != nil || len(result) != 0 {
		t.Fatal("native more-HP-than-self has no fallback", err)
	}
}

func p6LiveSelectionWholeInputs(t *testing.T, s *GameService) []*model.FrozenBattleInput {
	var inputs []*model.FrozenBattleInput
	for _, tc := range []struct{ hero, talents string }{{"hero_017", "tal_286"}, {"hero_081", "tal_3235"}, {"hero_011", "tal_339"}, {"hero_062", "tal_381,tal_383"}, {"hero_069", "tal_381,tal_383"}, {"hero_070", "tal_381,tal_383"}, {"hero_073", "tal_381,tal_383"}} {
		for _, camp := range []int{0, 1} {
			row := s.table("Hero", "id", tc.hero)
			a := p6Actor(tc.hero, "live-owner", row["SkillId1"]+","+row["SkillId2"]+","+row["UltSkillId"], 1, 80000)
			a.TalentList = strings.Split(tc.talents, ",")
			f := p6Input(t, s, a, p6Actor("hero_047", "live-enemy", "047_1,047_2,047_3", 1, 80000), 10)
			for pose := int32(2); pose <= 4; pose++ {
				mate := p6Actor("hero_047", "live-mate-"+strconv.Itoa(int(pose)), "047_1,047_2,047_3", pose, 80000)
				mate.Inst.CurHp = proto.Float64(80000 - float64(pose)*2000)
				f.Attacker.Formation = append(f.Attacker.Formation, mate)
			}
			if camp == 1 {
				f.Attacker, f.Defender = f.Defender, f.Attacker
			}
			f.Attacker.Inst.Anger, f.Defender.Inst.Anger = proto.Float64(100), proto.Float64(100)
			inputs = append(inputs, f)
		}
	}
	return inputs
}

func TestP6OriginalLiveSelectionSevenMappedHeroesBothCampsWholeReplay(t *testing.T) {
	for _, input := range p6LiveSelectionWholeInputs(t, p6OriginalTables(t)) {
		s := &GameService{Tables: input.Tables}
		out, err := s.ComputeFrozenBattle(t.Context(), input, nil)
		if err != nil {
			t.Fatal(err)
		}
		active := false
		for _, event := range out.Events {
			if event.Kind == "buff_load" && strings.Contains(event.Actor, "live-owner") && (event.Buff == "buff_282" || event.Buff == "buff_520" || event.Buff == "buff_545" || event.Buff == "buff_547" || event.Buff == "buff_548") {
				active = true
			}
		}
		if !active {
			t.Fatal("mapped actual talent never applied its original content")
		}
		replay, err := s.ComputeFrozenBattle(t.Context(), input, out.Operates)
		if err != nil || out.Authoritative || !proto.Equal(out.Operates, replay.Operates) || !proto.Equal(out.Attacker, replay.Attacker) || !proto.Equal(out.Defender, replay.Defender) || out.RandomDraws != replay.RandomDraws || !reflect.DeepEqual(out.Events, replay.Events) {
			t.Fatal("mapped original current selection replay", err)
		}
	}
}
