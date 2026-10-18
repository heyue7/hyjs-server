package service

import (
	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	"reflect"
	"testing"
)

func TestP6OriginalRoundMaxHPSixRowsSnapshotAndTiming(t *testing.T) {
	for _, tc := range []struct {
		id   string
		want float64
	}{
		{"buff_267", -4000}, {"buff_281", 8000}, {"buff_425", -1600}, {"buff_496", -1000}, {"buff_719", -16000}, {"buff_801", -8000},
	} {
		t.Run(tc.id, func(t *testing.T) {
			e := p6PropertyTransferEngine(t)
			a, d := e.players[0].actors[0], e.players[1].actors[0]
			d.hp = 40000
			row := e.service.table("Buff", "Id", tc.id)
			if err := e.validateDynamicTrigger(combatRoundTrigger{buff: tc.id, timer: 4}); err != nil {
				t.Fatal(err)
			}
			if err := e.loadBuff(a, d, row, 4); err != nil {
				t.Fatal(err)
			}
			closeCombat(t, d.buffs[0].value, tc.want)
			closeCombat(t, d.hp, 40000)
			a.base["att"] = 1000
			d.base["maxHp"] = 100000
			e.round = 1
			e.camp = 0
			if err := e.beginRound(); err != nil {
				t.Fatal(err)
			}
			closeCombat(t, d.hp, 40000)
			e.round = 2
			e.camp = 1
			if err := e.beginRound(); err != nil {
				t.Fatal(err)
			}
			closeCombat(t, d.hp, 40000+tc.want)
			if e.rng.Draws() != 0 {
				t.Fatal("periodic HP consumes RNG")
			}
			if err := e.endRound(); err != nil {
				t.Fatal(err)
			}
			if len(d.buffs) != 1 || d.buffs[0].rounds != int32(tableNumber(row, "TotalRound"))-1 {
				t.Fatal("round lifetime changed")
			}
		})
	}
}
func TestP6RoundMaxHPCurrentTypedCapPhysicalShieldAndRestore(t *testing.T) {
	e := p6PropertyTransferEngine(t)
	a, d := e.players[0].actors[0], e.players[1].actors[0]
	a.base["patt"] = 100
	a.base["matt"] = 300
	row := e.service.table("Buff", "Id", "buff_496")
	for _, tc := range []struct {
		typ  int32
		want float64
	}{{1, -1200}, {2, -1600}, {3, -1000}} {
		a.current = a.skills[0]
		a.current.attackType = tc.typ
		closeCombat(t, e.roundMaxHPValue(a, d, row), tc.want)
	}
	a.current = nil
	a.attackType = 2
	if err := e.loadBuff(a, d, row, 4); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, d.buffs[0].value, -1600)
	for _, r := range []map[string]string{
		{"Id": "periodic-p", "Factor": "pshield", "Args1": "1000", "TotalRound": "2"},
		{"Id": "periodic-m", "Factor": "mshield", "Args1": "2000", "TotalRound": "2"},
		{"Id": "periodic-g", "Factor": "shield", "Args1": "200", "TotalRound": "2"},
	} {
		if err := e.loadBuff(d, d, r, 1); err != nil {
			t.Fatal(err)
		}
	}
	before := d.hp
	e.applyRoundMaxHP(d.buffs[0])
	closeCombat(t, d.hp, before-400)
	closeCombat(t, d.shield("mshield"), 2000)
	// Source type controls the cap; application is always physical1 in original AddHp.
	d.buffs[0].row["CanBeCleared"] = "1"
	d.buffs[0].row["IsDebuff"] = ""
	suppressed, err := e.suppressBuffsOnce(d)
	if err != nil {
		t.Fatal(err)
	}
	a.base["att"] = 1000
	if err = e.restoreBuffsOnce(d, suppressed); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, d.buffs[0].value, -2600)
	// Args2=0 means uncapped; nonzero original cap bounds both signs.
	probe := map[string]string{"Factor": "permaxHpBySelfRound", "Args1": "10", "Args2": "200"}
	closeCombat(t, e.roundMaxHPValue(a, d, probe), 2600)
	probe["Args2"] = "0"
	closeCombat(t, e.roundMaxHPValue(a, d, probe), 8000)
}
func TestP6RoundMaxHPHealPenaltyForbiddenDeathAndClear(t *testing.T) {
	e := p6PropertyTransferEngine(t)
	a, d := e.players[0].actors[0], e.players[1].actors[0]
	d.hp = 40000
	if err := e.loadBuff(a, d, e.service.table("Buff", "Id", "buff_281"), 1); err != nil {
		t.Fatal(err)
	}
	if err := e.loadBuff(a, d, e.service.table("Buff", "Id", "buff_111"), 1); err != nil {
		t.Fatal(err)
	}
	e.applyRoundMaxHP(d.buffs[0])
	closeCombat(t, d.hp, 47600)
	if err := e.loadBuff(d, d, map[string]string{"Id": "forbid-periodic-hp", "Factor": "forbidAddHp", "TotalRound": "2"}, 1); err != nil {
		t.Fatal(err)
	}
	e.applyRoundMaxHP(d.buffs[0])
	closeCombat(t, d.hp, 47600)
	// Clearing the HP buff prevents further ticks and does not undo previously changed HP.
	e.unloadBuff(d, d.buffs[0])
	closeCombat(t, d.hp, 47600)
	for _, b := range append([]*combatRoundBuff(nil), d.buffs...) {
		e.unloadBuff(d, b)
	}
	d.hp = 500
	if err := e.loadBuff(a, d, e.service.table("Buff", "Id", "buff_267"), 1); err != nil {
		t.Fatal(err)
	}
	e.round = 2
	e.camp = 1
	if err := e.beginRound(); err != nil {
		t.Fatal(err)
	}
	if d.hp != 0 || d.state != 4 || len(d.buffs) != 0 || len(e.commands) != 0 {
		t.Fatal("periodic death retained actor action", d.hp, d.state, len(e.commands))
	}
}
func p6RoundMaxHPWholeInputs(t *testing.T, s *GameService) []*model.FrozenBattleInput {
	var result []*model.FrozenBattleInput
	for _, tc := range []struct{ id, skills, talent string }{
		{"hero_051", "051_1,051_2,051_3", ""},
		{"hero_006", "006_1,006_2,006_3", "tal_361"},
		{"hero_047", "047_1,047_2,047_3", "tal_1021"},
		{"monster_041", "worldboss1041_1,,worldboss1041_3", ""},
	} {
		a := p6Actor(tc.id, "roundhp-"+tc.id, tc.skills, 1, 80000)
		if tc.talent != "" {
			a.TalentList = []string{tc.talent}
		}
		f := p6Input(t, s, a, p6Actor("hero_047", "roundhp-enemy", "047_1,047_2,047_3", 1, 80000), 18)
		f.Attacker.Inst.Anger = proto.Float64(100)
		result = append(result, f)
	}
	return result
}
func TestP6OriginalRoundMaxHPWholeInputsReplayAndFactoryGuards(t *testing.T) {
	s := p6OriginalTables(t)
	for _, f := range p6RoundMaxHPWholeInputs(t, s) {
		t.Run(f.Attacker.Formation[0].GetId(), func(t *testing.T) {
			out, err := s.ComputeFrozenBattle(t.Context(), f, nil)
			if err != nil {
				t.Fatal(err)
			}
			ticks := 0
			seen := map[string]bool{}
			for _, v := range out.Events {
				if v.Kind == "round_maxhp_change" {
					ticks++
				}
				if v.Kind == "release" && v.Actor == "0_"+f.Attacker.Formation[0].GetGuid() {
					seen[v.Skill] = true
				}
			}
			wantSkills := 3
			if f.Attacker.Formation[0].GetId() == "monster_041" {
				wantSkills = 2
			}
			if ticks == 0 || len(seen) != wantSkills || out.Authoritative {
				t.Fatal("full original periodic HP sequence missing", ticks, seen)
			}
			replay, err := s.ComputeFrozenBattle(t.Context(), f, out.Operates)
			if err != nil || !proto.Equal(out.Operates, replay.Operates) || !proto.Equal(out.Attacker, replay.Attacker) || !proto.Equal(out.Defender, replay.Defender) || out.RandomDraws != replay.RandomDraws || !reflect.DeepEqual(out.Events, replay.Events) {
				t.Fatal("native periodic HP full replay diverged", err)
			}
		})
	}
	for _, tc := range []struct{ field, value string }{{"BuffClass", "1"}, {"Args2", "-1"}} {
		e := p6PropertyTransferEngine(t)
		e.service.table("Buff", "Id", "buff_496")[tc.field] = tc.value
		if err := e.validateDynamicTrigger(combatRoundTrigger{buff: "buff_496", timer: 4}); err == nil {
			t.Fatal("invalid periodic HP factory/cap accepted", tc)
		}
	}
}
