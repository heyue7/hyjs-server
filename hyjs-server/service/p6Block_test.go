package service

import (
	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	"reflect"
	"testing"
)

func TestP6OriginalBlockTwelveRowsAndNativeUnload(t *testing.T) {
	for _, id := range []string{"buff_138", "buff_169", "buff_342", "buff_354", "buff_382", "buff_422", "buff_428", "buff_432", "buff_441", "buff_462", "buff_493", "buff_798"} {
		t.Run(id, func(t *testing.T) {
			e := p6PropertyTransferEngine(t)
			a := e.players[0].actors[0]
			row := e.service.table("Buff", "Id", id)
			if err := e.validateDynamicTrigger(combatRoundTrigger{buff: id, timer: 7}); err != nil {
				t.Fatal(err)
			}
			if err := e.loadBuff(a, a, row, 7); err != nil {
				t.Fatal(err)
			}
			closeCombat(t, a.attribute("block"), tableNumber(row, "Args1"))
			closeCombat(t, a.attribute("blockDam"), -50)
			e.unloadBuff(a, a.buffs[0])
			closeCombat(t, a.attribute("block"), 0)
			// Native UnEffect removes chance alone, retaining the last assigned damage modifier.
			closeCombat(t, a.attribute("blockDam"), -50)
			row["BuffClass"] = "1"
			if err := e.validateDynamicTrigger(combatRoundTrigger{buff: id, timer: 7}); err == nil {
				t.Fatal("wrong block factory accepted")
			}
		})
	}
}
func TestP6BlockStackOverwriteSuppressionRoundAndDeath(t *testing.T) {
	e := p6PropertyTransferEngine(t)
	a, d := e.players[0].actors[0], e.players[1].actors[0]
	for _, row := range []map[string]string{
		{"Id": "block-a", "Factor": "blockDam", "BuffClass": "0", "Args1": "20", "Args2": "-25", "TotalRound": "2", "CanBeCleared": "1"},
		{"Id": "block-b", "Factor": "blockDam", "BuffClass": "0", "Args1": "30", "Args2": "-60", "TotalRound": "3", "CanBeCleared": "1"},
	} {
		if err := e.loadBuff(a, a, row, 1); err != nil {
			t.Fatal(err)
		}
	}
	closeCombat(t, a.attribute("block"), 50)
	closeCombat(t, a.attribute("blockDam"), -60)
	suppressed, err := e.suppressBuffsOnce(a)
	if err != nil {
		t.Fatal(err)
	}
	closeCombat(t, a.attribute("block"), 0)
	closeCombat(t, a.attribute("blockDam"), -60)
	if err = e.restoreBuffsOnce(a, suppressed); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, a.attribute("block"), 50)
	closeCombat(t, a.attribute("blockDam"), -60)
	e.unloadBuff(a, a.buffs[1])
	closeCombat(t, a.attribute("block"), 20)
	closeCombat(t, a.attribute("blockDam"), -60)
	e.round = 1
	if err = e.endRound(); err != nil {
		t.Fatal(err)
	}
	if len(a.buffs) != 1 {
		t.Fatal("block expired before round2")
	}
	e.round = 2
	if err = e.endRound(); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, a.attribute("block"), 0)
	closeCombat(t, a.attribute("blockDam"), -60)
	if err = e.loadBuff(a, a, e.service.table("Buff", "Id", "buff_798"), 1); err != nil {
		t.Fatal(err)
	}
	e.damageHP(d, a, a.hp+1, 1)
	if a.state != 4 || len(a.buffs) != 0 || a.attribute("block") != 0 {
		t.Fatal("dead actor retained block")
	}
}
func p6BlockReleaseProbe(t *testing.T, chance float64, typ int32, talent bool, shields bool) (*combatRoundEngine, float64) {
	e := p6PropertyTransferEngine(t)
	a, d := e.players[0].actors[0], e.players[1].actors[0]
	a.base["cri"] = 0
	row := map[string]string{"Id": "block-guaranteed-probe", "BuffClass": "0", "Factor": "blockDam", "Args1": "100", "Args2": "-50", "TotalRound": "2"}
	if err := e.loadBuff(d, d, row, 1); err != nil {
		t.Fatal(err)
	}
	row["Args1"] = "0"
	if chance == 100 {
		row["Args1"] = "100"
	}
	if talent {
		d.talents = []combatRoundTrigger{{buff: "buff_664", timer: 23, selectStrategy: 0, chance: 1}}
	}
	skill := a.skills[0]
	skill.target = d
	skill.first = true
	cast := map[string]string{}
	for k, v := range skill.casts[0] {
		cast[k] = v
	}
	cast["ExportType"] = "1"
	if typ == 2 {
		cast["ExportType"] = "2"
	}
	if shields {
		for _, kind := range []string{"pshield", "mshield"} {
			if err := e.loadBuff(d, d, map[string]string{"Id": "block-shield-" + kind, "Factor": kind, "Args1": "50", "TotalRound": "2"}, 1); err != nil {
				t.Fatal(err)
			}
		}
	}
	before := d.hp
	if err := e.dynamicRelease(skill, cast); err != nil {
		t.Fatal(err)
	}
	return e, before - d.hp
}
func TestP6BlockRollDamageTypedShieldsAndBlockTalentOrder(t *testing.T) {
	for _, typ := range []int32{1, 2} {
		plain, base := p6BlockReleaseProbe(t, 0, typ, false, false)
		blocked, damage := p6BlockReleaseProbe(t, 100, typ, false, false)
		closeCombat(t, damage, base*.5)
		if plain.rng.Draws() != 4 || blocked.rng.Draws() != 4 {
			t.Fatal("block RNG draw order changed")
		}
		withTalent, withDamage := p6BlockReleaseProbe(t, 100, typ, true, false)
		closeCombat(t, withDamage, damage)
		block, load, hurt, counter := int64(0), int64(0), int64(0), int64(0)
		for _, v := range withTalent.events {
			if v.Kind == "block" {
				block = v.Sequence
			}
			if v.Kind == "buff_load" && v.Buff == "buff_664" {
				load = v.Sequence
			}
			if v.Kind == "damage" && v.Target == withTalent.players[1].actors[0].uid {
				hurt = v.Sequence
			}
			if v.Kind == "counterstrike" {
				counter = v.Sequence
			}
		}
		if block == 0 || load <= block || hurt <= load || counter <= hurt || withTalent.rng.Draws() != 5 {
			t.Fatal("block→timer23→damage→normal counter order failed", block, load, hurt, counter, withTalent.rng.Draws())
		}
		shielded, hpLoss := p6BlockReleaseProbe(t, 100, typ, false, true)
		closeCombat(t, hpLoss, 0)
		matched, opposite := "pshield", "mshield"
		if typ == 2 {
			matched, opposite = opposite, matched
		}
		closeCombat(t, shielded.players[1].actors[0].shield(matched), 50-damage)
		closeCombat(t, shielded.players[1].actors[0].shield(opposite), 50)
		if withTalent.players[1].actors[0].attribute("counterStrikeDam") != 0 {
			t.Fatal("block's one-hit counter survived")
		}
	}
}
func p6BlockWholeInputs(t *testing.T, s *GameService) []*model.FrozenBattleInput {
	var result []*model.FrozenBattleInput
	for _, tc := range []struct{ id, skills, talent string }{
		{"hero_015", "015_1,015_2,015_3", ""},
		{"hero_021", "021_1,021_2,021_3", ""},
		{"hero_047", "047_1,047_2,047_3", "tal_329"},
		{"hero_002", "002_1,002_2,002_3", "tal_348"},
		{"hero_067", "067_1,067_2,067_3", "tal_400"},
	} {
		a := p6Actor(tc.id, "block-"+tc.id, tc.skills, 1, 80000)
		if tc.talent != "" {
			a.TalentList = []string{tc.talent}
		}
		f := p6Input(t, s, a, p6Actor("hero_047", "block-enemy", "047_1,047_2,047_3", 1, 80000), 18)
		f.Attacker.Inst.Anger = proto.Float64(100)
		result = append(result, f)
	}
	return result
}
func TestP6OriginalBlockFullHeroesAndReplay(t *testing.T) {
	s := p6OriginalTables(t)
	for _, f := range p6BlockWholeInputs(t, s) {
		t.Run(f.Attacker.Formation[0].GetId(), func(t *testing.T) {
			out, err := s.ComputeFrozenBattle(t.Context(), f, nil)
			if err != nil {
				t.Fatal(err)
			}
			blocks := 0
			seen := map[string]bool{}
			for _, v := range out.Events {
				if v.Kind == "block" {
					blocks++
				}
				if v.Kind == "release" && v.Actor == "0_"+f.Attacker.Formation[0].GetGuid() {
					seen[v.Skill] = true
				}
			}
			if blocks == 0 || len(seen) != 3 || out.Authoritative {
				t.Fatal("full original block sequence missing", blocks, seen)
			}
			replay, err := s.ComputeFrozenBattle(t.Context(), f, out.Operates)
			if err != nil || !proto.Equal(out.Operates, replay.Operates) || !proto.Equal(out.Attacker, replay.Attacker) || !proto.Equal(out.Defender, replay.Defender) || out.RandomDraws != replay.RandomDraws || !reflect.DeepEqual(out.Events, replay.Events) {
				t.Fatal("native block full replay diverged", err)
			}
		})
	}
}

func TestP6BlockSkippedForMissInvincibilityHealingAndUltimateCounter(t *testing.T) {
	for _, kind := range []string{"miss", "invincible", "heal", "ultimate"} {
		e := p6PropertyTransferEngine(t)
		a, d := e.players[0].actors[0], e.players[1].actors[0]
		row := map[string]string{"Id": "block-skip-probe", "BuffClass": "0", "Factor": "blockDam", "Args1": "100", "Args2": "-50", "TotalRound": "2"}
		if err := e.loadBuff(d, d, row, 1); err != nil {
			t.Fatal(err)
		}
		d.talents = []combatRoundTrigger{{buff: "buff_664", timer: 23, selectStrategy: 0, chance: 1}}
		skill := a.skills[0]
		skill.target = d
		skill.first = true
		cast := map[string]string{}
		for k, v := range skill.casts[0] {
			cast[k] = v
		}
		switch kind {
		case "miss":
			a.base["hit"] = 0
		case "invincible":
			if err := e.loadBuff(d, d, map[string]string{"Id": "block-invincible", "Factor": "invincible", "TotalRound": "2"}, 1); err != nil {
				t.Fatal(err)
			}
		case "heal":
			cast["ExportType"] = "3"
		case "ultimate":
			skill.row = map[string]string{"IsUltimate": "1"}
		}
		if err := e.dynamicRelease(skill, cast); err != nil {
			t.Fatal(err)
		}
		blocks, counters := 0, 0
		for _, v := range e.events {
			if v.Kind == "block" {
				blocks++
			}
			if v.Kind == "counterstrike" {
				counters++
			}
		}
		if kind == "ultimate" {
			if blocks != 1 || counters != 0 {
				t.Fatal("ultimate block/counter gate", blocks, counters)
			}
		} else if blocks != 0 || counters != 0 {
			t.Fatal("non-damage branch blocked/countered", kind)
		}
	}
}

func TestP6BlockPartialSuppressionKeepsThenReeffectsNativeLastAssignment(t *testing.T) {
	e := p6PropertyTransferEngine(t)
	a := e.players[0].actors[0]
	for _, row := range []map[string]string{
		{"Id": "partial-clearable", "Factor": "blockDam", "Args1": "20", "Args2": "-25", "TotalRound": "2", "CanBeCleared": "1"},
		{"Id": "partial-unclearable", "Factor": "blockDam", "Args1": "30", "Args2": "-60", "TotalRound": "2"},
	} {
		if err := e.loadBuff(a, a, row, 1); err != nil {
			t.Fatal(err)
		}
	}
	suppressed, err := e.suppressBuffsOnce(a)
	if err != nil {
		t.Fatal(err)
	}
	closeCombat(t, a.attribute("block"), 30)
	closeCombat(t, a.attribute("blockDam"), -60)
	if err = e.restoreBuffsOnce(a, suppressed); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, a.attribute("block"), 50)
	closeCombat(t, a.attribute("blockDam"), -25)
}
