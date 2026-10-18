package service

import (
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	"math"
	"reflect"
	"strconv"
	"testing"
)

func TestP6OriginalRoundExtraOwnerCurrentTypedAttackAndNoReleaseProcs(t *testing.T) {
	for _, typ := range []int32{1, 2} {
		e := p6PropertyTransferEngine(t)
		owner, target := e.players[0].actors[0], e.players[1].actors[0]
		owner.attackType = typ
		owner.base["att"], owner.base["patt"], owner.base["matt"] = 1000, 100, 200
		owner.base["damageAdd"], owner.base["cri"] = 20, 100
		target.base["pdef"], target.base["mdef"] = 100, 200
		target.base["damageReduce"], target.base["pdamageReduce"], target.base["mdamageReduce"] = 10, 20, 30
		target.base["block"], target.base["dodge"] = 100, 100
		target.talents = []combatRoundTrigger{{buff: "buff_421", timer: 7, chance: 1}}
		for _, x := range []struct{ factor, amount string }{{"pshield", "10"}, {"mshield", "10"}, {"shield", "20"}, {"invincible", "1"}, {"invisible", "1"}} {
			if err := e.loadBuff(target, target, map[string]string{"Id": x.factor, "Factor": x.factor, "Args1": x.amount, "TotalRound": "4"}, 4); err != nil {
				t.Fatal(err)
			}
		}
		if err := e.loadBuff(owner, owner, map[string]string{"Id": "ignored-immunity", "Factor": "immuneDef", "TotalRound": "4"}, 4); err != nil {
			t.Fatal(err)
		}
		// Different-camp origin makes any accidental use of Buff.source observable.
		if err := e.loadBuff(target, owner, e.service.table("Buff", "Id", "buff_208"), 1); err != nil {
			t.Fatal(err)
		}
		b := owner.buffs[len(owner.buffs)-1]
		closeCombat(t, owner.attribute("att"), 1000)
		ap0, ap1, hp := e.players[0].anger, e.players[1].anger, owner.hp
		if err := e.originalRoundExtraAttacks(b); err != nil {
			t.Fatal(err)
		}
		damage := 900.0
		other := "mshield"
		if typ == 2 {
			damage, other = 800, "pshield"
		}
		closeCombat(t, target.hp, 80000-2*damage+30)
		closeCombat(t, target.shield(other), 10)
		closeCombat(t, target.shield("shield"), 0)
		closeCombat(t, owner.hp, hp)
		closeCombat(t, e.players[0].anger, ap0)
		closeCombat(t, e.players[1].anger, ap1)
		if e.rng.Draws() != 2 {
			t.Fatal("only native per-attempt selection may draw RNG", e.rng.Draws())
		}
		owner.base["att"] = 2000
		before := target.hp
		if err := e.originalRoundExtraAttacks(b); err != nil {
			t.Fatal(err)
		}
		closeCombat(t, target.hp, before-4*damage)
		e.unloadBuff(owner, b)
		closeCombat(t, owner.attribute("att"), 2000)
	}
}

func TestP6OriginalRoundExtraNumericLoopWeakAndEmptySelection(t *testing.T) {
	for _, count := range []float64{-1, 0, .5, 2, 2.2} {
		e := p6PropertyTransferEngine(t)
		owner, target := e.players[0].actors[0], e.players[1].actors[0]
		row := e.service.table("Buff", "Id", "buff_208")
		row["Args3"] = strconv.FormatFloat(count, 'f', -1, 64)
		if err := e.loadBuff(owner, owner, row, 1); err != nil {
			t.Fatal(err)
		}
		b := owner.buffs[len(owner.buffs)-1]
		target.state, target.hp = 3, 0
		if err := e.loadBuff(target, target, map[string]string{"Id": "weak-shield", "Factor": "pshield", "Args1": "100", "TotalRound": "4"}, 4); err != nil {
			t.Fatal(err)
		}
		if err := e.originalRoundExtraAttacks(b); err != nil {
			t.Fatal(err)
		}
		want := int64(math.Max(0, math.Ceil(count)))
		if e.rng.Draws() != want || target.hp != 0 || target.shield("pshield") != 100 {
			t.Fatal("native numeric loop or weak AddHp guard", count, e.rng.Draws())
		}
		target.state = 4
		if err := e.originalRoundExtraAttacks(b); err != nil || e.rng.Draws() != want {
			t.Fatal("empty candidate set consumed selection RNG", err)
		}
		attempts := int64(0)
		for _, ev := range e.events {
			if ev.Kind == "round_extra_attempt" {
				attempts++
			}
		}
		if attempts != want {
			t.Fatal("empty native export iterations missing", attempts, want)
		}
	}
}

func TestP6OriginalRoundExtraIndependentRandomOrderAndDeathMarkerNoRevive(t *testing.T) {
	e := p6SameDamageEngine(t)
	owner := e.players[0].actors[0]
	if err := e.loadBuff(owner, owner, e.service.table("Buff", "Id", "buff_208"), 1); err != nil {
		t.Fatal(err)
	}
	b := owner.buffs[len(owner.buffs)-1]
	rng := NewCombatRandom(0)
	want := []string{}
	for i := 0; i < 2; i++ {
		want = append(want, e.players[1].actors[int(rng.Value()*3)].uid)
	}
	if err := e.originalRoundExtraAttacks(b); err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, event := range e.events {
		if event.Kind == "round_extra_attack" {
			got = append(got, event.Target)
		}
	}
	if !reflect.DeepEqual(got, want) || e.rng.Draws() != 2 {
		t.Fatal("periodic numeric iterations reused a skill target cache", got, want)
	}
	// The sole original tal332 binds timer16 SELF. It has no resurrection effect.
	one := p6PropertyTransferEngine(t)
	a, d := one.players[0].actors[0], one.players[1].actors[0]
	d.talents = []combatRoundTrigger{{buff: "buff_208", timer: 16, chance: 1}}
	d.hp = 1
	one.damageHP(a, d, 2, 1)
	if d.state != 4 || d.hp != 0 || d.revived {
		t.Fatal("original death marker fabricated resurrection")
	}
	before := len(one.events)
	one.camp = 1
	if err := one.beginRound(); err != nil {
		t.Fatal(err)
	}
	for _, event := range one.events[before:] {
		if event.Kind == "round_extra_attack" {
			t.Fatal("dead original marker attacked on next round")
		}
	}
}

func TestP6OriginalRoundExtraRecomputesAfterEnemyDeathAndGuardsIndexMutation(t *testing.T) {
	for _, mutateIndex := range []bool{false, true} {
		e := p6SameDamageEngine(t)
		owner := e.players[0].actors[0]
		owner.base["att"], owner.base["patt"], owner.base["damageAdd"] = 100, 0, 0
		for _, actor := range e.players[1].actors {
			actor.base["pdef"], actor.base["damageReduce"], actor.base["pdamageReduce"] = 0, 0, 0
		}
		row := e.service.table("Buff", "Id", "buff_208")
		row["CanBeCleared"] = "1"
		if err := e.loadBuff(owner, owner, row, 1); err != nil {
			t.Fatal(err)
		}
		b := owner.buffs[len(owner.buffs)-1]
		first := e.players[1].actors[int(NewCombatRandom(0).Value()*3)]
		first.hp = 1
		if mutateIndex {
			// Explicit synthetic mutation probe; no original content binding is claimed.
			e.service.Tables["Buff"] = append(e.service.Tables["Buff"], map[string]string{"Id": "test-round-remove", "Factor": "clearSomeBuffs", "BuffClass": "3", "Param": "buff_208"})
			first.talents = []combatRoundTrigger{{buff: "test-round-remove", timer: 16, chance: 1, selectStrategy: 3}}
			if err := e.originalRoundExtraAttacks(b); err == nil {
				t.Fatal("unsupported native mutable-index continuation silently recovered")
			}
		} else {
			e.service.Tables["Buff"] = append(e.service.Tables["Buff"], map[string]string{"Id": "test-round-stat", "Factor": "att", "BuffClass": "0", "Args1": "500", "TotalRound": "2"})
			owner.talents = []combatRoundTrigger{{buff: "test-round-stat", timer: 28, chance: 1}}
			if err := e.originalRoundExtraAttacks(b); err != nil {
				t.Fatal(err)
			}
			values := []float64{}
			for _, event := range e.events {
				if event.Kind == "round_extra_attack" {
					values = append(values, event.Value)
				}
			}
			if !reflect.DeepEqual(values, []float64{100, 600}) {
				t.Fatal("periodic subsequent iteration cached stale attack", values)
			}
		}
	}
}

func TestP6OriginalRoundExtraOwnRoundLifecycleMinimumAndNoKiller(t *testing.T) {
	e := p6PropertyTransferEngine(t)
	owner, target := e.players[0].actors[0], e.players[1].actors[0]
	row := e.service.table("Buff", "Id", "buff_208")
	row["CanBeCleared"] = "1"
	if err := e.loadBuff(owner, owner, row, 1); err != nil {
		t.Fatal(err)
	}
	owner.base["att"] = 0
	target.base["damageReduce"] = 200
	owner.talents = []combatRoundTrigger{{buff: "buff_196", timer: 18, chance: 1}}
	b := owner.buffs[len(owner.buffs)-1]
	suppressed, err := e.suppressBuffsOnce(owner)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.originalRoundExtraAttacks(b); err != nil || e.rng.Draws() != 0 {
		t.Fatal("suppressed original effect still attacks", err)
	}
	if err := e.restoreBuffsOnce(owner, suppressed); err != nil {
		t.Fatal(err)
	}
	e.camp = 1
	if err := e.beginRound(); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, target.hp, 80000)
	e.camp = 0
	if err := e.beginRound(); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, target.hp, 79998)
	if b.rounds != 100 || owner.attribute("att") != 0 {
		t.Fatal("periodic factory changed attack or counted a round before end")
	}
	for remaining := int32(99); remaining >= 0; remaining-- {
		if err := e.endRound(); err != nil || b.rounds != remaining {
			t.Fatal("original 100-round marker lifetime", err, b.rounds, remaining)
		}
	}
	draws := e.rng.Draws()
	if err := e.beginRound(); err != nil || e.rng.Draws() != draws {
		t.Fatal("expired marker still selected periodic targets", err)
	}
	closeCombat(t, target.hp, 79998)
	if err := e.loadBuff(owner, owner, row, 1); err != nil {
		t.Fatal(err)
	}
	b = owner.buffs[len(owner.buffs)-1]
	target.hp = 1
	if err := e.originalRoundExtraAttacks(b); err != nil || target.state != 4 || owner.hasState("dieAfterLastAttack") {
		t.Fatal("periodic extra death fabricated primary kill18", err, target.state)
	}
	row["BuffClass"] = "1"
	if err := e.validateDynamicTrigger(combatRoundTrigger{buff: "buff_208", timer: 1}); err == nil {
		t.Fatal("wrong original round-attack factory accepted")
	}
	row["BuffClass"], row["Args2"] = "0", "3"
	if err := e.validateDynamicTrigger(combatRoundTrigger{buff: "buff_208", timer: 1}); err == nil {
		t.Fatal("unproved round-attack strategy accepted")
	}
}

func p6RoundExtraProbeInputs(t *testing.T, s *GameService) []*model.FrozenBattleInput {
	raw, err := json.Marshal(s.Tables)
	if err != nil {
		t.Fatal(err)
	}
	var tables map[string][]map[string]string
	if err := json.Unmarshal(raw, &tables); err != nil {
		t.Fatal(err)
	}
	talent := "p6_unmapped_probe_buff_208"
	tables["Talent"] = append(tables["Talent"], map[string]string{"Id": talent, "Buff0": "buff_208", "Buff0TriggerTimer": "1", "Buff0Select": "0", "Buff0Percent": "100"})
	probe := &GameService{Tables: tables}
	var out []*model.FrozenBattleInput
	for _, x := range []struct{ hero, skills string }{{"hero_047", "047_1,047_2,047_3"}, {"hero_018", "018_1,018_2,018_3"}} {
		owner := p6Actor(x.hero, "extra-probe", x.skills, 1, 80000)
		owner.TalentList = []string{talent}
		f := p6Input(t, probe, owner, p6Actor("hero_047", "extra-enemy", "047_1,047_2,047_3", 1, 80000), 12)
		f.Attacker.Inst.Anger, f.Defender.Inst.Anger = proto.Float64(100), proto.Float64(100)
		out = append(out, f)
	}
	return out
}

func TestP6OriginalRoundExtraUnmappedFullSkillProbeReplay(t *testing.T) {
	for _, f := range p6RoundExtraProbeInputs(t, p6OriginalTables(t)) {
		probe := &GameService{Tables: f.Tables}
		out, err := probe.ComputeFrozenBattle(t.Context(), f, nil)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, ev := range out.Events {
			found = found || ev.Kind == "round_extra_attack"
		}
		if !found || out.Authoritative {
			t.Fatal("explicit unmapped periodic probe missing")
		}
		replay, err := probe.ComputeFrozenBattle(t.Context(), f, out.Operates)
		if err != nil || !proto.Equal(out.Operates, replay.Operates) || !proto.Equal(out.Attacker, replay.Attacker) || !proto.Equal(out.Defender, replay.Defender) || out.RandomDraws != replay.RandomDraws || !reflect.DeepEqual(out.Events, replay.Events) {
			t.Fatal("unmapped periodic original-parameter probe replay", err)
		}
	}
}
