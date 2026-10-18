package service

import (
	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"reflect"
	"testing"
)

func TestP6OriginalDamageDualFiveStacksNoDoubleApplyLifecycle(t *testing.T) {
	e := p6PropertyTransferEngine(t)
	a := e.players[0].actors[0]
	row := e.service.table("Buff", "Id", "buff_570")
	for i := 0; i < 7; i++ {
		if err := e.loadBuff(a, a, row, 4); err != nil {
			t.Fatal(err)
		}
	}
	closeCombat(t, a.attribute("damageAdd"), a.base["damageAdd"]+75)
	closeCombat(t, a.attribute("damageReduce"), a.base["damageReduce"]+75)
	if len(a.buffs) != 5 {
		t.Fatal("native five-stack cap failed")
	}
	row["CanBeCleared"] = "1"
	suppressed, err := e.suppressBuffsOnce(a)
	if err != nil {
		t.Fatal(err)
	}
	closeCombat(t, a.attribute("damageAdd"), a.base["damageAdd"])
	closeCombat(t, a.attribute("damageReduce"), a.base["damageReduce"])
	if err = e.restoreBuffsOnce(a, suppressed); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, a.attribute("damageAdd"), a.base["damageAdd"]+75)
	closeCombat(t, a.attribute("damageReduce"), a.base["damageReduce"]+75)
	e.unloadBuff(a, a.buffs[0])
	closeCombat(t, a.attribute("damageAdd"), a.base["damageAdd"]+60)
	closeCombat(t, a.attribute("damageReduce"), a.base["damageReduce"]+60)
	if e.rng.Draws() != 0 {
		t.Fatal("native compound damage properties consume RNG")
	}
}
func TestP6OriginalReductionTransferThreeRowsBothEndsSuppressDeathAndSelf(t *testing.T) {
	for _, id := range []string{"buff_714", "buff_741", "buff_742"} {
		e := p6PropertyTransferEngine(t)
		a, d := e.players[0].actors[0], e.players[1].actors[0]
		a.base["damageReduce"] = 5
		d.base["damageReduce"] = 10
		row := e.service.table("Buff", "Id", id)
		x := tableNumber(row, "Args1")
		row["CanBeCleared"] = "1"
		if err := e.loadBuff(a, d, row, 12); err != nil {
			t.Fatal(err)
		}
		closeCombat(t, a.attribute("damageReduce"), 5+x)
		closeCombat(t, d.attribute("damageReduce"), 10-x)
		original := d.buffs[0]
		for i := 0; i < 3; i++ {
			suppressed, err := e.suppressBuffsOnce(d)
			if err != nil {
				t.Fatal(err)
			}
			closeCombat(t, a.attribute("damageReduce"), 5)
			closeCombat(t, d.attribute("damageReduce"), 10)
			if err = e.restoreBuffsOnce(d, suppressed); err != nil {
				t.Fatal(err)
			}
			closeCombat(t, a.attribute("damageReduce"), 5+x)
			closeCombat(t, d.attribute("damageReduce"), 10-x)
			if len(a.outgoingReduction) != 1 || d.buffs[0] != original {
				t.Fatal("native source endpoint duplicated or identity changed")
			}
		}
		e.damageHP(a, d, d.hp+1, 1)
		closeCombat(t, a.attribute("damageReduce"), 5)
		if len(a.outgoingReduction) != 0 || len(d.buffs) != 0 {
			t.Fatal("target death left source reduction behind")
		}
		// Actual cast1546 uses SELF: both native contributions cancel but identity lasts4 rounds.
		if err := e.loadBuff(a, a, row, 12); err != nil {
			t.Fatal(err)
		}
		closeCombat(t, a.attribute("damageReduce"), 5)
		e.unloadBuff(a, a.buffs[0])
		closeCombat(t, a.attribute("damageReduce"), 5)
	}
}
func TestP6BothCampsOnceCastClearSourceFirstAndRepeatedManaBurn(t *testing.T) {
	e := p6PropertyTransferEngine(t)
	e.input.Attacker.Formation = append(e.input.Attacker.Formation, p6Actor("hero_047", "source-mate", "047_1,047_2,047_3", 2, 80000))
	e.input.Defender.Formation = append(e.input.Defender.Formation, p6Actor("hero_047", "enemy-mate", "047_1,047_2,047_3", 2, 80000))
	e = &combatRoundEngine{service: e.service, input: e.input, rng: NewCombatRandom(0), operate: &pb.BattleOperate{}}
	if err := e.initialize(); err != nil {
		t.Fatal(err)
	}
	a, d := e.players[0].actors[0], e.players[1].actors[0]
	for _, p := range e.players {
		for _, actor := range p.actors {
			if err := e.loadBuff(a, actor, map[string]string{"Id": "scope-" + actor.uid, "Factor": "att", "Args1": "10"}, 4); err != nil {
				t.Fatal(err)
			}
		}
	}
	e.camp = 1 // Cleanup order follows the caster, not a surrounding camp scheduler.
	a.talents = []combatRoundTrigger{{buff: "buff_455", timer: 5, selectStrategy: 2, chance: 1}}
	e.players[1].anger = 20
	skill := a.skills[0]
	skill.target = d
	// Preserve FSkill.Do's first held target collection for this release probe.
	skill.targets = []*combatRoundActor{d}
	if err := e.dynamicRelease(skill, skill.casts[0]); err != nil {
		t.Fatal(err)
	}
	var order []int32
	for _, v := range e.events {
		if v.Kind == "buff_unload" && len(v.Buff) > 6 && v.Buff[:6] == "scope-" {
			order = append(order, v.Camp)
		}
	}
	// Buff-unload trace Camp is its original source camp; verify target UIDs instead.
	var targets []string
	for _, v := range e.events {
		if v.Kind == "buff_unload" && len(v.Buff) > 6 && v.Buff[:6] == "scope-" {
			targets = append(targets, v.Target)
		}
	}
	want := []string{a.uid, e.players[0].actors[1].uid, d.uid, e.players[1].actors[1].uid}
	if !reflect.DeepEqual(targets, want) {
		t.Fatal("native caster then opposite camp cleanup order", order, targets, want)
	}
	for _, p := range e.players {
		for _, actor := range p.actors {
			if len(actor.buffs) != 0 {
				t.Fatal("onceCast buff escaped end of cast", actor.uid)
			}
		}
	}
	if err := e.dynamicRelease(skill, skill.casts[0]); err != nil {
		t.Fatal(err)
	}
	loads, burns := 0, 0
	for _, v := range e.events {
		if v.Kind == "buff_load" && v.Buff == "buff_455" {
			loads++
		}
		if v.Kind == "anger_hp_change" {
			burns++
		}
	}
	if loads != 2 || burns != 2 {
		t.Fatal("old target cast marker blocked next original mana-burn effect", loads, burns)
	}
}
func p6DamageTransferWholeInputs(t *testing.T, s *GameService) []*model.FrozenBattleInput {
	var result []*model.FrozenBattleInput
	dual := p6Actor("hero_067", "dual-bonus", "067_1,067_2,067_3", 1, 80000)
	dual.TalentList = []string{"tal_403"}
	f := p6Input(t, s, dual, p6Actor("hero_047", "dual-enemy", "047_1,047_2,047_3", 1, 80000), 18)
	f.Attacker.Inst.Anger = proto.Float64(100)
	result = append(result, f)
	for _, slot := range []string{"0", "1", "2"} {
		awake := p6Actor("hero_015", "steal-awake", "015_1,015_2,015_3", 1, 80000)
		awake.AwakeStage = proto.Int32(2)
		awake.AwakenSkill = proto.String("0015_1," + slot)
		awake.TalentList = []string{"tal_1094"}
		f = p6Input(t, s, awake, p6Actor("hero_047", "steal-enemy", "047_1,047_2,047_3", 1, 80000), 18)
		f.Attacker.Inst.Anger = proto.Float64(100)
		result = append(result, f)
	}
	self := p6Actor("hero_068", "self-transfer", "068_1,068_2,068_3", 1, 80000)
	f = p6Input(t, s, self, p6Actor("hero_047", "self-transfer-enemy", "047_1,047_2,047_3", 1, 80000), 18)
	f.Attacker.Inst.Anger = proto.Float64(100)
	result = append(result, f)
	return result
}
func TestP6OriginalDamageTransferFullSkillsThreeAwakeSlotsReplayAndFactory(t *testing.T) {
	s := p6OriginalTables(t)
	for i, f := range p6DamageTransferWholeInputs(t, s) {
		out, err := s.ComputeFrozenBattle(t.Context(), f, nil)
		if err != nil {
			t.Fatal(err)
		}
		seen := map[string]bool{}
		loads := 0
		wantBuff := "buff_714"
		wantSkills := 4
		if i == 0 {
			wantBuff = "buff_570"
			wantSkills = 3
		}
		if i == 4 {
			wantBuff = "buff_741"
			wantSkills = 3
		}
		for _, v := range out.Events {
			if v.Kind == "release" && v.Actor == "0_"+f.Attacker.Formation[0].GetGuid() {
				seen[v.Skill] = true
			}
			if v.Kind == "buff_load" && v.Buff == wantBuff {
				loads++
			}
		}
		if len(seen) != wantSkills || loads == 0 || out.Authoritative {
			t.Fatal("native complete damage-transfer sequence missing", i, seen, loads)
		}
		replay, err := s.ComputeFrozenBattle(t.Context(), f, out.Operates)
		if err != nil || !proto.Equal(out.Operates, replay.Operates) || !proto.Equal(out.Attacker, replay.Attacker) || !proto.Equal(out.Defender, replay.Defender) || out.RandomDraws != replay.RandomDraws || !reflect.DeepEqual(out.Events, replay.Events) {
			t.Fatal("original damage transfer replay diverged", err)
		}
	}
	for _, id := range []string{"buff_570", "buff_714", "buff_741", "buff_742"} {
		e := p6PropertyTransferEngine(t)
		e.service.table("Buff", "Id", id)["BuffClass"] = "1"
		if err := e.validateDynamicTrigger(combatRoundTrigger{buff: id, timer: 5}); err == nil {
			t.Fatal("wrong damage-compound factory accepted", id)
		}
	}
}
