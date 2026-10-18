package service

import (
	"google.golang.org/protobuf/proto"
	pb "hyjs-server/model/protocol"
	"testing"
)

func TestP6OriginalCounterstrikeFormulaCapShieldAndDeadGuard(t *testing.T) {
	s := p6OriginalTables(t)
	input := p6Input(t, s, p6Actor("hero_047", "attacker", "047_1", 1, 2000), p6Actor("hero_047", "retaliator", "047_1", 1, 2000), 6)
	e := &combatRoundEngine{service: &GameService{Tables: input.Tables}, input: input, rng: NewCombatRandom(0), operate: &pb.BattleOperate{}}
	if err := e.initialize(); err != nil {
		t.Fatal(err)
	}
	a, d := e.players[0].actors[0], e.players[1].actors[0]
	a.base["pdef"] = 100
	a.base["damageReduce"] = 10
	d.base["damageAdd"] = 50
	if err := e.loadBuff(d, d, e.service.table("Buff", "Id", "buff_259"), 7); err != nil {
		t.Fatal(err)
	}
	if err := e.counterStrike(d, a, 1000); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, a.hp, 2000-112)
	if err := e.counterStrike(d, a, 100); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, a.hp, 2000-112-20)
	if e.rng.Draws() != 0 {
		t.Fatal("retaliation created a random roll")
	}
	if err := e.loadBuff(a, a, map[string]string{"Id": "counter-shield", "Factor": "pshield", "Args1": "200", "TotalRound": "1"}, 4); err != nil {
		t.Fatal(err)
	}
	if err := e.counterStrike(d, a, 1000); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, a.hp, 2000-112-20)
	closeCombat(t, a.shield("pshield"), 88)
	d.hp, d.state = 0, 3
	before := len(e.events)
	if err := e.counterStrike(d, a, 1000); err != nil || len(e.events) != before {
		t.Fatal("dead retaliator acted", err)
	}
}

func TestP6RealCounterstrikeBypassesDefenseAndUnsupportedSelection(t *testing.T) {
	s := p6OriginalTables(t)
	input := p6Input(t, s, p6Actor("hero_047", "a", "047_1", 1, 2000), p6Actor("hero_047", "d", "047_1", 1, 2000), 6)
	e := &combatRoundEngine{service: &GameService{Tables: input.Tables}, input: input, rng: NewCombatRandom(0), operate: &pb.BattleOperate{}}
	if err := e.initialize(); err != nil {
		t.Fatal(err)
	}
	a, d := e.players[0].actors[0], e.players[1].actors[0]
	a.base["pdef"], a.base["damageReduce"], a.base["pdamageReduce"] = 10000, 50, 50
	if err := e.loadBuff(d, d, map[string]string{"Id": "real-counter", "Factor": "counterStrikeRealDam", "Args1": "20", "Args2": "2", "TotalRound": "1"}, 7); err != nil {
		t.Fatal(err)
	}
	if err := e.counterStrike(d, a, 1000); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, a.hp, 1900)
	e.service.Tables["Buff"] = append(e.service.Tables["Buff"], map[string]string{"Id": "unsupported-counter", "Factor": "counterStrikeDam", "Args2": "6"})
	if err := e.validateDynamicTrigger(combatRoundTrigger{buff: "unsupported-counter", timer: 7, chance: 1}); err == nil {
		t.Fatal("uncompared random counter target admitted")
	}
}

func TestP6OriginalCounterstrikeTalentWholeFightAndReplay(t *testing.T) {
	s := p6OriginalTables(t)
	a := p6Actor("hero_047", "a", "047_1,,047_3", 1, 12000)
	d := p6Actor("hero_047", "d", "047_1", 1, 12000)
	d.TalentList = []string{"tal_353"}
	input := p6Input(t, s, a, d, 12)
	input.Attacker.Inst.Anger = proto.Float64(300)
	out, err := s.ComputeFrozenBattle(t.Context(), input, nil)
	if err != nil {
		t.Fatal(err)
	}
	counters, ultimate := 0, 0
	lastCounterTime := -1.0
	lastCounterRound := int32(-1)
	for _, event := range out.Events {
		if event.Kind == "counterstrike" {
			counters++
			lastCounterTime, lastCounterRound = event.Time, event.Round
		}
		if event.Kind == "release" && event.Skill == "047_3" {
			ultimate++
			if lastCounterTime == event.Time && lastCounterRound == event.Round {
				t.Fatal("ultimate damage countered")
			}
		}
	}
	if counters < 1 || ultimate < 1 || out.Authoritative {
		t.Fatal("original retaliation/ultimate path missing", counters, ultimate)
	}
	replayed, err := s.ComputeFrozenBattle(t.Context(), input, out.Operates)
	if err != nil || !proto.Equal(replayed.Operates, out.Operates) || out.RandomDraws != replayed.RandomDraws || !proto.Equal(replayed.Attacker, out.Attacker) {
		t.Fatal("counterstrike replay diverged", err)
	}
}

func TestP6PeriodicPowerResistanceOriginalFraction(t *testing.T) {
	s := p6OriginalTables(t)
	input := p6Input(t, s, p6Actor("hero_047", "a", "047_1", 1, 2000), p6Actor("hero_047", "d", "047_1", 1, 2000), 6)
	e := &combatRoundEngine{service: &GameService{Tables: input.Tables}, input: input, rng: NewCombatRandom(0), operate: &pb.BattleOperate{}}
	if err := e.initialize(); err != nil {
		t.Fatal(err)
	}
	a, d := e.players[0].actors[0], e.players[1].actors[0]
	if err := e.loadBuff(a, a, map[string]string{"Id": "poison-power", "Factor": "poison", "Args1": "100", "TotalRound": "1"}, 1); err != nil {
		t.Fatal(err)
	}
	if err := e.loadBuff(d, d, map[string]string{"Id": "poison-resist", "Factor": "antiPoison", "Args1": "200", "TotalRound": "1"}, 1); err != nil {
		t.Fatal(err)
	}
	if err := e.loadBuff(a, d, e.service.table("Buff", "Id", "buff_111"), 12); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, d.buffs[len(d.buffs)-1].value, 299)
}
