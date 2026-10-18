package service

import (
	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"testing"
)

func p6SourceShieldInput(t *testing.T, s *GameService) *model.FrozenBattleInput {
	input := p6Input(t, s, p6Actor("hero_071", "source-shield-caster", "071_1,071_2,071_3", 1, 80000), p6Actor("hero_052", "source-shield-foe", "052_1,052_2,052_3", 1, 80000), 18)
	input.Attacker.Inst.Anger = proto.Float64(100)
	input.Defender.Inst.Anger = proto.Float64(100)
	return input
}

func TestP6OriginalSourceShieldReeffectRereadsOriginAttackAndKeepsLifetime(t *testing.T) {
	for _, tc := range []struct{ factor, kind string }{{"sourceperattToshield", "shield"}, {"sourceperattTopshield", "pshield"}, {"sourceperattTomshield", "mshield"}} {
		s := p6OriginalTables(t)
		input := p6SourceShieldInput(t, s)
		e := &combatRoundEngine{service: &GameService{Tables: input.Tables}, input: input, rng: NewCombatRandom(0), operate: &pb.BattleOperate{}}
		if err := e.initialize(); err != nil {
			t.Fatal(err)
		}
		a, d := e.players[0].actors[0], e.players[1].actors[0]
		row := map[string]string{}
		for k, v := range e.service.table("Buff", "Id", "buff_157") {
			row[k] = v
		}
		row["Factor"] = tc.factor // Typed variants retain original Transfer branches; native buff157 supplies lifecycle.
		if err := e.loadBuff(a, d, row, 4); err != nil {
			t.Fatal(err)
		}
		original := d.buffs[0]
		original.value = 40
		if err := e.loadBuff(a, d, map[string]string{"Id": "retained-same-kind-probe", "Factor": tc.kind, "Args1": "25", "TotalRound": "2"}, 4); err != nil {
			t.Fatal(err)
		}
		closeCombat(t, d.shield(tc.kind), 65)
		e.time = 2
		suppressed, err := e.suppressBuffsOnce(d)
		if err != nil || len(suppressed) != 1 {
			t.Fatal(err)
		}
		closeCombat(t, d.shield(tc.kind), 0)
		a.base["att"] = 750
		if err = e.restoreBuffsOnce(d, suppressed); err != nil {
			t.Fatal(err)
		}
		closeCombat(t, d.shield(tc.kind), 600)
		closeCombat(t, original.initialValue, 600)
		if original.loaded != 0 || original.rounds != 10 || len(d.buffs) != 2 || e.rng.Draws() != 0 {
			t.Fatal("shield reEffect replaced identity/lifetime/draws", tc)
		}
		e.unloadBuff(d, original)
		closeCombat(t, d.shield(tc.kind), 0)
		if len(d.buffs) != 1 {
			t.Fatal("native typed reset removed unrelated Buff identity")
		}
	}
}

func TestP6OriginalShieldUneffectResetsOnlyMatchingWholeKind(t *testing.T) {
	s := p6OriginalTables(t)
	input := p6SourceShieldInput(t, s)
	e := &combatRoundEngine{service: &GameService{Tables: input.Tables}, input: input, rng: NewCombatRandom(0), operate: &pb.BattleOperate{}}
	if err := e.initialize(); err != nil {
		t.Fatal(err)
	}
	a, d := e.players[0].actors[0], e.players[1].actors[0]
	for _, row := range []map[string]string{
		e.service.table("Buff", "Id", "buff_157"),
		{"Id": "retained-flat-probe", "Factor": "shield", "Args1": "25", "TotalRound": "2"},
		{"Id": "other-kind-probe", "Factor": "pshield", "Args1": "35", "TotalRound": "2"},
	} {
		if err := e.loadBuff(a, d, row, 4); err != nil {
			t.Fatal(err)
		}
	}
	e.unloadBuff(d, d.buffs[0])
	closeCombat(t, d.shield("shield"), 0)
	closeCombat(t, d.shield("pshield"), 35)
	if len(d.buffs) != 2 {
		t.Fatal("typed UnEffect changed other Buff identities")
	}
}

func TestP6OriginalShieldPartialContributionRetainsGlobalBalanceAndBuffs(t *testing.T) {
	s := p6OriginalTables(t)
	input := p6SourceShieldInput(t, s)
	e := &combatRoundEngine{service: &GameService{Tables: input.Tables}, input: input, rng: NewCombatRandom(0), operate: &pb.BattleOperate{}}
	if err := e.initialize(); err != nil {
		t.Fatal(err)
	}
	a, d := e.players[0].actors[0], e.players[1].actors[0]
	// Original hero073's distinct groups share one scalar shield attribute.
	// This isolated AddHp probe does not claim an original actor mapping for d.
	for _, id := range []string{"buff_546", "buff_547"} {
		if err := e.loadBuff(a, d, s.table("Buff", "Id", id), 7); err != nil {
			t.Fatal(err)
		}
	}
	initialHP := d.hp
	firstBalance := d.buffs[0].value
	secondBalance := d.buffs[1].value
	e.damageHP(a, d, firstBalance+25, 1)
	closeCombat(t, d.shield("shield"), secondBalance-25)
	closeCombat(t, d.hp, initialHP)
	if len(d.buffs) != 2 || d.buffs[0].value != 0 {
		t.Fatal("partial contribution incorrectly removed shared shield Buffs")
	}
	// Refreshing the first group clears the scalar but keeps the second identity.
	if err := e.loadBuff(a, d, s.table("Buff", "Id", "buff_546"), 7); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, d.shield("shield"), firstBalance)
	if len(d.buffs) != 2 || d.buffs[0].row["Id"] != "buff_547" || d.buffs[0].value != 0 {
		t.Fatal("native cover changed unrelated shield identity")
	}
	// The existing second Buff compares the global scalar to its original val.
	if err := e.loadBuff(a, d, s.table("Buff", "Id", "buff_547"), 7); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, d.shield("shield"), firstBalance)
	e.damageHP(a, d, firstBalance+20, 1)
	closeCombat(t, d.hp, initialHP-20)
	if d.shield("shield") != 0 || len(d.buffs) != 0 {
		t.Fatal("exhausted global scalar retained shield identities")
	}
}

func TestP6OriginalHero071FullSourceShieldSkillsWholeBattleAndReplay(t *testing.T) {
	s := p6OriginalTables(t)
	input := p6SourceShieldInput(t, s)
	out, err := s.ComputeFrozenBattle(t.Context(), input, nil)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	loads := 0
	unloads := 0
	for _, event := range out.Events {
		if event.Kind == "release" && event.Actor == "0_source-shield-caster" {
			seen[event.Skill] = true
		}
		if event.Buff == "buff_754" && event.Kind == "buff_load" {
			loads++
		}
		if event.Buff == "buff_754" && event.Kind == "buff_unload" {
			unloads++
		}
	}
	if len(seen) != 3 || loads < 2 || unloads < 1 || out.Authoritative {
		t.Fatal("complete original shield actor mechanism missing", seen, loads, unloads)
	}
	replayed, err := s.ComputeFrozenBattle(t.Context(), input, out.Operates)
	if err != nil || !proto.Equal(out.Operates, replayed.Operates) || !proto.Equal(out.Attacker, replayed.Attacker) || !proto.Equal(out.Defender, replayed.Defender) || out.RandomDraws != replayed.RandomDraws {
		t.Fatal("source shield whole original replay diverged", err)
	}
}

func TestP6OriginalSourceShieldRejectsUnknownNonTransferFactory(t *testing.T) {
	e := p6PropertyTransferEngine(t)
	row := e.service.table("Buff", "Id", "buff_157")
	if row == nil {
		t.Fatal("original shield157 missing")
	}
	row["BuffClass"] = "0"
	if err := e.validateDynamicTrigger(combatRoundTrigger{buff: "buff_157", timer: 4}); err == nil {
		t.Fatal("unknown source shield class0 silently executed class1 formula")
	}
}
