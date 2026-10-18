package service

import (
	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"reflect"
	"testing"
)

func TestP6OriginalAngerStealTwoRowsInsufficientRateCapAndNoRollback(t *testing.T) {
	for _, tc := range []struct {
		id                                    string
		target, start, wantTarget, wantSource float64
	}{
		{"buff_319", 0, 100, 0, 100}, {"buff_319", 2.5, 100, 0, 103.75}, {"buff_319", 3, 100, 0, 104.5}, {"buff_319", 50, 100, 47, 104.5},
		{"buff_542", 40, 100, 0, 160}, {"buff_542", 50, 100, 0, 175}, {"buff_542", 70, 100, 20, 175}, {"buff_542", 50, 299, 0, 300},
	} {
		e := p6PropertyTransferEngine(t)
		a, d := e.players[0].actors[0], e.players[1].actors[0]
		e.players[0].anger = tc.start
		e.players[1].anger = tc.target
		if err := e.loadBuff(a, a, map[string]string{"Id": "anger-steal-rate", "Factor": "angerRate", "Args1": "50", "TotalRound": "2"}, 1); err != nil {
			t.Fatal(err)
		}
		row := e.service.table("Buff", "Id", tc.id)
		if err := e.loadBuff(a, d, row, 4); err != nil {
			t.Fatal(err)
		}
		closeCombat(t, e.players[1].anger, tc.wantTarget)
		closeCombat(t, e.players[0].anger, tc.wantSource)
		e.unloadBuff(d, d.buffs[0])
		closeCombat(t, e.players[0].anger, tc.wantSource)
		closeCombat(t, e.players[1].anger, tc.wantTarget)
		if e.rng.Draws() != 0 {
			t.Fatal("native steal consumed RNG")
		}
	}
}
func TestP6OriginalAngerStealHealRawAmountLowestHPAndBlockedRecovery(t *testing.T) {
	e := p6PropertyTransferEngine(t)
	mate := p6Actor("hero_047", "lowest-hp-mate", "047_1,047_2,047_3", 2, 80000)
	mate.Inst.CurHp = proto.Float64(10000)
	e.input.Attacker.Formation = append(e.input.Attacker.Formation, mate)
	e = &combatRoundEngine{service: e.service, input: e.input, rng: NewCombatRandom(0), operate: &pb.BattleOperate{}}
	if err := e.initialize(); err != nil {
		t.Fatal(err)
	}
	a, m, d := e.players[0].actors[0], e.players[0].actors[1], e.players[1].actors[0]
	a.hp = 40000
	a.base["patt"] = 100
	a.base["treatAdd"] = 20
	a.base["cure"] = 2
	m.base["heal"] = 3
	e.players[0].anger = 299
	e.players[1].anger = 2
	if err := e.loadBuff(a, a, map[string]string{"Id": "anger-heal-rate", "Factor": "angerRate", "Args1": "50", "TotalRound": "2"}, 1); err != nil {
		t.Fatal(err)
	}
	row := e.service.table("Buff", "Id", "buff_323")
	if err := e.loadBuff(a, d, row, 12); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, e.players[0].anger, 300)
	closeCombat(t, e.players[1].anger, 0)
	closeCombat(t, m.hp, 10033.8)
	closeCombat(t, a.hp, 40000)
	anger, heal := int64(0), int64(0)
	for _, v := range e.events {
		if v.Kind == "anger" {
			anger = v.Sequence
		}
		if v.Kind == "anger_steal_heal" {
			heal = v.Sequence
		}
	}
	if heal <= anger {
		t.Fatal("healing preceded native AP mutations")
	}
	if err := e.loadBuff(m, m, map[string]string{"Id": "forbid-anger-heal", "Factor": "forbidAddHp", "TotalRound": "2"}, 1); err != nil {
		t.Fatal(err)
	}
	e.players[1].anger = 3
	if err := e.loadBuff(a, d, row, 12); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, m.hp, 10033.8)
	closeCombat(t, e.players[1].anger, 0)
	// No enemy AP means no cure/heal term, no recipient selection and no healing.
	e.unloadBuff(m, m.buffs[0])
	before := m.hp
	if err := e.loadBuff(a, d, row, 12); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, m.hp, before)
}
func TestP6OriginalManaBurnSignedCapAngerBoundaryShieldsAndDeath(t *testing.T) {
	for _, tc := range []struct{ anger, wantLoss float64 }{{0, 0}, {1, 100}, {2, 200}, {10, 200}} {
		e := p6PropertyTransferEngine(t)
		a, d := e.players[0].actors[0], e.players[1].actors[0]
		e.players[1].anger = tc.anger
		before := d.hp
		if err := e.loadBuff(a, d, e.service.table("Buff", "Id", "buff_455"), 5); err != nil {
			t.Fatal(err)
		}
		closeCombat(t, d.hp, before-tc.wantLoss)
		closeCombat(t, e.players[1].anger, tc.anger-min(tc.anger, 2))
		if e.rng.Draws() != 0 {
			t.Fatal("mana burn consumed RNG")
		}
	}
	e := p6PropertyTransferEngine(t)
	a, d := e.players[0].actors[0], e.players[1].actors[0]
	if err := e.loadBuff(d, d, map[string]string{"Id": "burn-p-shield", "Factor": "pshield", "Args1": "150", "TotalRound": "2"}, 1); err != nil {
		t.Fatal(err)
	}
	if err := e.loadBuff(d, d, map[string]string{"Id": "burn-m-shield", "Factor": "mshield", "Args1": "500", "TotalRound": "2"}, 1); err != nil {
		t.Fatal(err)
	}
	e.players[1].anger = 2
	before := d.hp
	if err := e.loadBuff(a, d, e.service.table("Buff", "Id", "buff_455"), 5); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, d.hp, before-50)
	closeCombat(t, d.shield("mshield"), 500)
	for _, b := range append([]*combatRoundBuff(nil), d.buffs...) {
		e.unloadBuff(d, b)
	}
	d.hp = 100
	e.players[1].anger = 2
	if _, err := e.originalAngerCompoundEffect(a, d, e.service.table("Buff", "Id", "buff_455")); err != nil {
		t.Fatal(err)
	}
	if d.state != 4 || d.hp != 0 || e.players[1].anger != 0 {
		t.Fatal("mana burn AP/death order failed")
	}
}
func TestP6ManaHPPositiveNeedsExistingAngerAndReeffectConsumesAgain(t *testing.T) {
	e := p6PropertyTransferEngine(t)
	a, d := e.players[0].actors[0], e.players[1].actors[0]
	d.hp = 40000
	row := map[string]string{"Id": "native-positive-AP-HP-branch", "Factor": "angerAndhp", "BuffClass": "0", "Args1": "2", "Args2": "1", "Args3": "20", "CanBeCleared": "1", "TotalRound": "2"}
	if err := e.loadBuff(a, d, row, 4); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, d.hp, 40000)
	closeCombat(t, e.players[1].anger, 0)
	e.players[1].anger = 1
	suppressed, err := e.suppressBuffsOnce(d)
	if err != nil {
		t.Fatal(err)
	}
	if err = e.restoreBuffsOnce(d, suppressed); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, e.players[1].anger, 2)
	closeCombat(t, d.hp, 40100)
	suppressed, err = e.suppressBuffsOnce(d)
	if err != nil {
		t.Fatal(err)
	}
	if err = e.restoreBuffsOnce(d, suppressed); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, e.players[1].anger, 4)
	closeCombat(t, d.hp, 40300)
}
func p6AngerCompoundWholeInputs(t *testing.T, s *GameService) []*model.FrozenBattleInput {
	var result []*model.FrozenBattleInput
	for _, tc := range []struct{ id, skills, talent string }{
		{"hero_014", "014_1,014_2,014_3", "tal_273"},
		{"hero_018", "018_1,018_2,018_3", "tal_289"},
		{"hero_023", "023_1,023_2,023_3", "tal_305"},
		{"hero_047", "047_1,047_2,047_3", "tal_378"},
	} {
		a := p6Actor(tc.id, "compound-"+tc.id, tc.skills, 1, 80000)
		a.TalentList = []string{tc.talent}
		f := p6Input(t, s, a, p6Actor("hero_047", "compound-enemy", "047_1,047_2,047_3", 1, 80000), 18)
		f.Attacker.Inst.Anger = proto.Float64(100)
		f.Defender.Inst.Anger = proto.Float64(80)
		result = append(result, f)
	}
	return result
}
func TestP6OriginalAngerCompoundFourFullInputsReplayFactory(t *testing.T) {
	s := p6OriginalTables(t)
	for i, f := range p6AngerCompoundWholeInputs(t, s) {
		out, err := s.ComputeFrozenBattle(t.Context(), f, nil)
		if err != nil {
			t.Fatal(err)
		}
		seen := map[string]bool{}
		changes := 0
		for _, v := range out.Events {
			if v.Kind == "release" && v.Actor == "0_"+f.Attacker.Formation[0].GetGuid() {
				seen[v.Skill] = true
			}
			if v.Kind == "anger_steal" || v.Kind == "anger_hp_change" {
				changes++
			}
		}
		if len(seen) != 3 || changes == 0 || out.Authoritative {
			t.Fatal("native AP compound whole flow missing", i, seen, changes)
		}
		replay, err := s.ComputeFrozenBattle(t.Context(), f, out.Operates)
		if err != nil || !proto.Equal(out.Operates, replay.Operates) || !proto.Equal(out.Attacker, replay.Attacker) || !proto.Equal(out.Defender, replay.Defender) || out.RandomDraws != replay.RandomDraws || !reflect.DeepEqual(out.Events, replay.Events) {
			t.Fatal("original AP compound replay diverged", err)
		}
	}
	for _, id := range []string{"buff_319", "buff_542", "buff_323", "buff_455"} {
		e := p6PropertyTransferEngine(t)
		e.service.table("Buff", "Id", id)["BuffClass"] = "1"
		if err := e.validateDynamicTrigger(combatRoundTrigger{buff: id, timer: 5}); err == nil {
			t.Fatal("wrong AP compound factory accepted", id)
		}
	}
}

func TestP6OriginalAngerRateAndIceLoadOnlyTheirNativeAttributes(t *testing.T) {
	for _, id := range []string{"buff_197", "buff_385", "buff_464", "buff_152"} {
		e := p6PropertyTransferEngine(t)
		a, d := e.players[0].actors[0], e.players[1].actors[0]
		if err := e.loadBuff(d, d, map[string]string{"Id": "rate-ice-shield", "Factor": "shield", "Args1": "500", "TotalRound": "2"}, 1); err != nil {
			t.Fatal(err)
		}
		before := d.hp
		if err := e.loadBuff(a, d, e.service.table("Buff", "Id", id), 12); err != nil {
			t.Fatal(err)
		}
		closeCombat(t, d.hp, before)
		closeCombat(t, d.shield("shield"), 500)
		for _, v := range e.events {
			if v.Kind == "additional_attack_damage" {
				t.Fatal("attribute-only original buff inflicted additional damage", id)
			}
		}
		if e.rng.Draws() != 0 {
			t.Fatal("attribute-only buff consumed RNG", id)
		}
	}
}
