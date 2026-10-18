package service

import (
	"google.golang.org/protobuf/proto"
	pb "hyjs-server/model/protocol"
	"testing"
)

func TestP6OriginalOneUseReviveClearsEffectsResetsSkillsAndExpiresInvisibility(t *testing.T) {
	s := p6OriginalTables(t)
	actor := p6Actor("hero_047", "reviver", "047_1", 1, 1000)
	actor.TalentList = []string{"tal_282"}
	input := p6Input(t, s, p6Actor("hero_047", "attacker", "047_1", 1, 1000), actor, 6)
	e := &combatRoundEngine{service: &GameService{Tables: input.Tables}, input: input, rng: NewCombatRandom(0), operate: &pb.BattleOperate{}}
	if err := e.initialize(); err != nil {
		t.Fatal(err)
	}
	a, d := e.players[0].actors[0], e.players[1].actors[0]
	if err := e.loadBuff(a, d, e.service.table("Buff", "Id", "buff_111"), 12); err != nil {
		t.Fatal(err)
	}
	d.hp = 20
	a.current = a.skills[0]
	a.current.first, a.current.state, a.current.target = true, 2, d
	e.damageHP(a, d, 100, 1)
	if d.hp != 0 || d.state != 3 || d.revived {
		t.Fatal("death was finalized before pending check")
	}
	a.current.state = 3
	if err := e.finishPendingDeaths(); err != nil {
		t.Fatal(err)
	}
	if d.hp != 100 || d.state != 0 || !d.revived || !d.hasState("invisible") || len(d.buffs) != 1 || d.skills[0].state != 4 {
		t.Fatal("native die talent revive/reset mismatch", d.hp, d.state, len(d.buffs))
	}
	if targets, err := e.selectTargets(a, 3, nil, nil); err != nil || len(targets) != 1 || targets[0] != d {
		t.Fatal("original FIRST fallback failed when the only living enemy is invisible", err)
	}
	if err := e.endRound(); err != nil {
		t.Fatal(err)
	}
	if d.hasState("invisible") {
		t.Fatal("revival invisibility did not expire")
	}
	e.damageHP(a, d, 200, 1)
	if err := e.finishPendingDeaths(); err != nil {
		t.Fatal(err)
	}
	if d.hp != 0 || d.state != 4 || len(d.buffs) != 0 {
		t.Fatal("native revival repeated in one fight", d.hp, d.state)
	}
}

func TestP6PeriodicDeathRevivesBeforeSameRoundAIAndClearsRemainingDots(t *testing.T) {
	s := p6OriginalTables(t)
	actor := p6Actor("hero_047", "reviver", "047_1", 1, 1000)
	actor.TalentList = []string{"tal_282"}
	input := p6Input(t, s, p6Actor("hero_047", "attacker", "047_1", 1, 1000), actor, 6)
	e := &combatRoundEngine{service: &GameService{Tables: input.Tables}, input: input, rng: NewCombatRandom(0), operate: &pb.BattleOperate{}}
	if err := e.initialize(); err != nil {
		t.Fatal(err)
	}
	a, d := e.players[0].actors[0], e.players[1].actors[0]
	d.hp = 20
	for _, id := range []string{"buff_111", "buff_149"} {
		if err := e.loadBuff(a, d, e.service.table("Buff", "Id", id), 12); err != nil {
			t.Fatal(err)
		}
	}
	e.round, e.camp = 2, 1
	if err := e.beginRound(); err != nil {
		t.Fatal(err)
	}
	if !d.revived || d.hp != 100 || len(d.buffs) != 1 {
		t.Fatal("cleared second DOT damaged revived actor", d.hp, len(d.buffs))
	}
	// Native DoDead forces FINISH for this round, even though AI can inspect the revived actor.
	if len(e.commands) != 0 || d.skills[0].state != 4 {
		t.Fatal("revival incorrectly re-enabled skill state", len(e.commands), d.skills[0].state)
	}
}

func TestP6OriginalReviveWaitsForLastReleaseAndWholeOperationReplay(t *testing.T) {
	s := p6OriginalTables(t)
	actor := p6Actor("hero_047", "reviver", "047_1", 1, 100)
	actor.Inst.CurHp = proto.Float64(20)
	actor.TalentList = []string{"tal_282"}
	input := p6Input(t, s, p6Actor("hero_047", "attacker", "047_1", 1, 1000), actor, 8)
	out, err := s.ComputeFrozenBattle(t.Context(), input, nil)
	if err != nil {
		t.Fatal(err)
	}
	revives, lastRound := 0, int32(0)
	for _, event := range out.Events {
		if event.Kind == "revive" {
			revives++
			lastRound = event.Round
			closeCombat(t, event.TargetHP, 10)
		}
	}
	if revives != 1 || lastRound != 1 || out.FinalRound < 3 || out.Winner != 0 || out.Authoritative {
		t.Fatal("one-use revival whole fight", revives, lastRound, out.FinalRound, out.Winner)
	}
	replayed, err := s.ComputeFrozenBattle(t.Context(), input, out.Operates)
	if err != nil || !proto.Equal(out.Operates, replayed.Operates) || !proto.Equal(out.Defender, replayed.Defender) || out.RandomDraws != replayed.RandomDraws {
		t.Fatal("revival replay diverged", err)
	}
	e := &combatRoundEngine{service: &GameService{Tables: input.Tables}, input: input, rng: NewCombatRandom(0), operate: &pb.BattleOperate{}}
	if err := e.initialize(); err != nil {
		t.Fatal(err)
	}
	a, d := e.players[0].actors[0], e.players[1].actors[0]
	a.current = a.skills[0]
	a.current.first, a.current.state, a.current.target = true, 2, d
	e.damageHP(a, d, 200, 1)
	if err := e.finishPendingDeaths(); err != nil || d.revived {
		t.Fatal("revival happened while attacks were running", err)
	}
	a.current.state = 3
	if err := e.finishPendingDeaths(); err != nil || !d.revived {
		t.Fatal("LAST release did not finalize revival", err)
	}
}
