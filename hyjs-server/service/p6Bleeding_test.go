package service

import (
	"google.golang.org/protobuf/proto"
	pb "hyjs-server/model/protocol"
	"testing"
)

func TestP6OriginalBleedingPhysicalFormulaShieldsCoverAndCondition(t *testing.T) {
	s := p6OriginalTables(t)
	input := p6Input(t, s, p6Actor("hero_047", "a", "047_1", 1, 2000), p6Actor("hero_047", "d", "047_1", 1, 2000), 6)
	e := &combatRoundEngine{service: &GameService{Tables: input.Tables}, input: input, rng: NewCombatRandom(0), operate: &pb.BattleOperate{}}
	if err := e.initialize(); err != nil {
		t.Fatal(err)
	}
	a, d := e.players[0].actors[0], e.players[1].actors[0]
	a.attackType = 2
	a.base["damageAdd"], d.base["damageReduce"], d.base["pdamageReduce"], d.base["pdef"] = 50, 5, 5, 100
	d.base["mdef"] = 10000
	if err := e.loadBuff(a, d, e.service.table("Buff", "Id", "buff_127"), 12); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, e.conditionProperty(d, "bleeding"), .04)
	if !supportedP6Condition(1, "type:bleeding,src1:3,op:1,src2:1,src2_val:0") {
		t.Fatal("native bleeding condition rejected")
	}
	if err := e.bleedingHit(a, d); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, d.hp, 2000-22.4)
	if e.rng.Draws() != 0 {
		t.Fatal("bleeding consumed random draw")
	}
	if err := e.loadBuff(d, d, map[string]string{"Id": "bleeding-physical-shield", "Factor": "pshield", "Args1": "50", "TotalRound": "1"}, 4); err != nil {
		t.Fatal(err)
	}
	if err := e.loadBuff(d, d, map[string]string{"Id": "bleeding-magic-shield", "Factor": "mshield", "Args1": "50", "TotalRound": "1"}, 4); err != nil {
		t.Fatal(err)
	}
	if err := e.bleedingHit(a, d); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, d.hp, 2000-22.4)
	closeCombat(t, d.shield("pshield"), 27.6)
	closeCombat(t, d.shield("mshield"), 50)
	if err := e.loadBuff(a, d, e.service.table("Buff", "Id", "buff_339"), 12); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, e.conditionProperty(d, "bleeding"), .05)
	for _, buff := range append([]*combatRoundBuff(nil), d.buffs...) {
		if buff.shieldKind() != "" {
			e.unloadBuff(d, buff)
		}
	}
	d.base["pdef"] = 10000
	beforeHP := d.hp
	if err := e.bleedingHit(a, d); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, d.hp, beforeHP-1)
	for count := 0; count < 4; count++ {
		if err := e.endRound(); err != nil {
			t.Fatal(err)
		}
	}
	closeCombat(t, e.conditionProperty(d, "bleeding"), 0)
	before := len(e.events)
	if err := e.bleedingHit(a, d); err != nil || len(e.events) != before {
		t.Fatal("expired bleeding remained", err)
	}
}

func p6BleedingActor() *pb.BattleHero {
	a := p6Actor("hero_047", "bleed", "047_1,047_2,047_3", 1, 40000)
	a.TalentList = []string{"tal_384"}
	return a
}

func TestP6OriginalBleedingTalentWholeBattleAndReplay(t *testing.T) {
	s := p6OriginalTables(t)
	input := p6Input(t, s, p6BleedingActor(), p6Actor("hero_047", "target", "047_1", 1, 40000), 12)
	input.Attacker.Inst.Anger = proto.Float64(300)
	out, err := s.ComputeFrozenBattle(t.Context(), input, nil)
	if err != nil {
		t.Fatal(err)
	}
	bleeding, ultimate := 0, 0
	lastBleedTime, lastBleedRound := -1.0, int32(-1)
	for _, event := range out.Events {
		if event.Kind == "bleeding" {
			bleeding++
			lastBleedTime, lastBleedRound = event.Time, event.Round
		}
		if event.Kind == "release" && event.Skill == "047_3" {
			ultimate++
			if lastBleedTime == event.Time && lastBleedRound == event.Round {
				t.Fatal("ultimate triggered bleeding")
			}
		}
	}
	if bleeding == 0 || ultimate == 0 || out.Authoritative {
		t.Fatal("missing original bleed/ultimate path", bleeding, ultimate)
	}
	replay, err := s.ComputeFrozenBattle(t.Context(), input, out.Operates)
	if err != nil || !proto.Equal(out.Operates, replay.Operates) || !proto.Equal(out.Attacker, replay.Attacker) || !proto.Equal(out.Defender, replay.Defender) || out.RandomDraws != replay.RandomDraws {
		t.Fatal("bleeding replay diverged", err)
	}
}
