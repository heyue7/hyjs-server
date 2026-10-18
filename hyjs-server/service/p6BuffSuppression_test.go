package service

import (
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p6CompleteBowInput(t *testing.T, s *GameService) *model.FrozenBattleInput {
	input := p6Input(t, s, p6Actor("hero_052", "complete-bow", "052_1,052_2,052_3", 1, 80000), p6Actor("hero_047", "bow-first", "047_1,047_2,047_3", 1, 80000), 18)
	input.Attacker.Inst.Anger = proto.Float64(100)
	input.Defender.Formation = append(input.Defender.Formation, p6Actor("hero_047", "bow-second", "047_1", 2, 80000), p6Actor("hero_047", "bow-third", "047_1", 3, 80000))
	return input
}

func TestP6OriginalCompleteBowAllThreeSkillsMarkerAndWholeReplay(t *testing.T) {
	s := p6OriginalTables(t)
	input := p6CompleteBowInput(t, s)
	out, err := s.ComputeFrozenBattle(t.Context(), input, nil)
	if err != nil {
		t.Fatal(err)
	}
	seen, marker := map[string]bool{}, 0
	for _, event := range out.Events {
		if event.Actor == "0_complete-bow" && event.Kind == "release" {
			seen[event.Skill] = true
		}
		if event.Buff == "buff_204" && event.Kind == "buff_load" {
			marker++
			// Original 052_3 rows have Buff0SelStr=0 (SELF), never rewrite them.
			if event.Target != "0_complete-bow" {
				t.Fatal("original bow marker changed recipient", event)
			}
		}
	}
	if len(seen) != 3 || marker < 11 || out.Authoritative {
		t.Fatal("complete original bow skill dependency remained open", seen, marker)
	}
	replayed, err := s.ComputeFrozenBattle(t.Context(), input, out.Operates)
	if err != nil || !proto.Equal(out.Operates, replayed.Operates) || !proto.Equal(out.Attacker, replayed.Attacker) || !proto.Equal(out.Defender, replayed.Defender) || out.RandomDraws != replayed.RandomDraws {
		t.Fatal("complete bow original operation replay diverged", err)
	}
}

func TestP6OriginalTargetSuppressionUsesUnbuffedDamageThenRestoresEffects(t *testing.T) {
	s := p6OriginalTables(t)
	makeEngine := func() *combatRoundEngine {
		input := p6Input(t, s, p6Actor("hero_001", "source", "001_1", 1, 2000), p6Actor("hero_047", "target", "047_1", 1, 2000), 1)
		e := &combatRoundEngine{service: &GameService{Tables: input.Tables}, input: input, rng: NewCombatRandom(0), operate: &pb.BattleOperate{}}
		if err := e.initialize(); err != nil {
			t.Fatal(err)
		}
		e.players[0].actors[0].base["hit"] = 100
		e.players[0].actors[0].base["cri"] = 0
		e.players[1].actors[0].base["pdef"] = 100
		e.players[1].actors[0].base["dodge"] = 0
		return e
	}
	actual, baseline := makeEngine(), makeEngine()
	for _, e := range []*combatRoundEngine{actual, baseline} {
		a, d := e.players[0].actors[0], e.players[1].actors[0]
		if err := e.loadBuff(a, d, map[string]string{"Id": "retained-negative-defense", "Factor": "perdef", "Args1": "-20", "IsDebuff": "1", "CanBeCleared": "1", "TotalRound": "2"}, 4); err != nil {
			t.Fatal(err)
		}
		a.skills[0].target = d
		// Original FSkill.Do supplies the first cast collection before FRelease.
		a.skills[0].targets = []*combatRoundActor{d}
	}
	a, d := actual.players[0].actors[0], actual.players[1].actors[0]
	for _, row := range []map[string]string{
		actual.service.table("Buff", "Id", "buff_204"),
		{"Id": "clearable-defense", "Factor": "perdef", "Args1": "100", "CanBeCleared": "1", "TotalRound": "2"},
		{"Id": "clearable-shield", "Factor": "shield", "Args1": "200", "CanBeCleared": "1", "TotalRound": "2"},
		{"Id": "retained-shield", "Factor": "shield", "Args1": "50", "TotalRound": "2"},
	} {
		if err := actual.loadBuff(a, d, row, 4); err != nil {
			t.Fatal(err)
		}
	}
	d.buffs[3].value = 80 // Original reactivation restores Args1, not remaining shield.
	closeCombat(t, d.shield("shield"), 130)
	closeCombat(t, d.attribute("pdef"), 180)
	for _, e := range []*combatRoundEngine{actual, baseline} {
		if err := e.dynamicRelease(e.players[0].actors[0].skills[0], e.service.table("SkillCast", "SkillCastId", "1")); err != nil {
			t.Fatal(err)
		}
	}
	closeCombat(t, d.hp, baseline.players[1].actors[0].hp)
	closeCombat(t, d.attribute("pdef"), 180)
	closeCombat(t, d.shield("shield"), 200)
	if len(d.buffs) != 4 || d.buffs[2].rounds != 2 || actual.rng.Draws() != baseline.rng.Draws() {
		t.Fatal("suppression changed collection, lifetime or RNG")
	}
	for _, b := range d.buffs {
		if b.suppressed {
			t.Fatal("temporary effect remained disabled after original anger stage")
		}
	}
}

func TestP6OriginalSuppressionReeffectsInstantHealAndRejectsUnknownReactivation(t *testing.T) {
	s := p6OriginalTables(t)
	input := p6Input(t, s, p6Actor("hero_047", "source", "047_1", 1, 2000), p6Actor("hero_047", "target", "047_1", 1, 2000), 1)
	e := &combatRoundEngine{service: &GameService{Tables: input.Tables}, input: input, rng: NewCombatRandom(0), operate: &pb.BattleOperate{}}
	if err := e.initialize(); err != nil {
		t.Fatal(err)
	}
	a, d := e.players[0].actors[0], e.players[1].actors[0]
	d.hp = 1000
	if err := e.loadBuff(a, d, map[string]string{"Id": "repeat-hp-effect", "Factor": "hp", "Args1": "10", "CanBeCleared": "1", "TotalRound": "2"}, 4); err != nil {
		t.Fatal(err)
	}
	suppressed, err := e.suppressBuffsOnce(d)
	if err != nil || len(suppressed) != 1 {
		t.Fatal(err)
	}
	e.changeHP(a, d, -100, "diagnostic-hit")
	if err = e.restoreBuffsOnce(d, suppressed); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, d.hp, 920)
	if e.rng.Draws() != 0 || len(d.buffs) != 1 || d.buffs[0].rounds != 2 {
		t.Fatal("Effect replay changed original lifetime/collection/RNG")
	}
	if err = e.loadBuff(a, d, map[string]string{"Id": "unknown-delay-reactivation", "Factor": "delayCreateBuffs", "Args1": "20", "CanBeCleared": "1", "TotalRound": "2"}, 4); err != nil {
		t.Fatal(err)
	}
	if _, err = e.suppressBuffsOnce(d); err == nil || d.buffs[0].suppressed {
		t.Fatal("unsupported suppression partially mutated the collection", err)
	}
	// ClearAllBuff precedes Die/relive. Old temporary effects cannot heal the
	// revived instance after its original collection has already been cleared.
	e.unloadBuff(d, d.buffs[1])
	suppressed, err = e.suppressBuffsOnce(d)
	if err != nil {
		t.Fatal(err)
	}
	d.state = 4
	e.unloadBuff(d, d.buffs[0])
	d.state, d.hp = 0, 200
	if err = e.restoreBuffsOnce(d, suppressed); err != nil || d.hp != 200 {
		t.Fatal("cleared pre-death Buff reactivated after revival", d.hp, err)
	}
}
