package service

import (
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	"reflect"
	"testing"
)

func TestP6OriginalAngerAbsorbAfterShieldCeilingWholeStepsAndSubstep(t *testing.T) {
	for _, x := range []struct{ anger, damage, hploss, left float64 }{{5, 801, 0, 1}, {5, 2000, 400, 1}, {1, 2000, 2000, 1}, {0, 2000, 2000, 0}} {
		e := p6PropertyTransferEngine(t)
		a, d := e.players[0].actors[0], e.players[1].actors[0]
		if err := e.loadBuff(a, d, e.service.table("Buff", "Id", "buff_202"), 7); err != nil {
			t.Fatal(err)
		}
		e.players[1].anger = x.anger
		e.damageHP(a, d, x.damage, 1)
		closeCombat(t, d.hp, 80000-x.hploss)
		closeCombat(t, e.players[1].anger, x.left)
	}
	e := p6PropertyTransferEngine(t)
	a, d := e.players[0].actors[0], e.players[1].actors[0]
	if err := e.loadBuff(a, d, e.service.table("Buff", "Id", "buff_202"), 7); err != nil {
		t.Fatal(err)
	}
	if err := e.loadBuff(a, d, map[string]string{"Id": "physical-shield", "Factor": "pshield", "Args1": "1000", "TotalRound": "4"}, 2); err != nil {
		t.Fatal(err)
	}
	e.players[1].anger = 5
	e.damageHP(a, d, 1000, 1)
	closeCombat(t, e.players[1].anger, 5)
	closeCombat(t, d.hp, 80000)
	e.damageHP(a, d, 100, 1)
	closeCombat(t, e.players[1].anger, 3)
	closeCombat(t, d.hp, 80000)
	if e.rng.Draws() != 0 {
		t.Fatal("native AP/HP conversion consumes random")
	}
}
func TestP6OriginalLossToAngerStrictThresholdRemainderLethalAndRoundReset(t *testing.T) {
	e := p6PropertyTransferEngine(t)
	a, d := e.players[0].actors[0], e.players[1].actors[0]
	if err := e.loadBuff(a, d, e.service.table("Buff", "Id", "buff_203"), 7); err != nil {
		t.Fatal(err)
	}
	e.damageHP(a, d, 8000, 1)
	closeCombat(t, e.players[1].anger, 0)
	closeCombat(t, d.hpLossAccumulator, -8000)
	e.damageHP(a, d, 1, 1)
	closeCombat(t, e.players[1].anger, 10)
	closeCombat(t, d.hpLossAccumulator, -1)
	e.damageHP(a, d, 16000, 1)
	closeCombat(t, e.players[1].anger, 30)
	closeCombat(t, d.hpLossAccumulator, -1)
	e.changeHP(a, d, 1000, "heal")
	closeCombat(t, d.hpLossAccumulator, -1)
	e.round, e.camp = 2, 1
	if err := e.beginRound(); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, d.hpLossAccumulator, 0)
	e.damageHP(a, d, d.hp+1, 1)
	closeCombat(t, e.players[1].anger, 30)
	closeCombat(t, d.hpLossAccumulator, 0)
	if d.state != 4 {
		t.Fatal("lethal loss wrongly produced AP")
	}
	// Native accumulation starts before this Buff exists; it is not cleared by Effect/UnEffect.
	e = p6PropertyTransferEngine(t)
	a, d = e.players[0].actors[0], e.players[1].actors[0]
	e.damageHP(a, d, 7000, 1)
	if err := e.loadBuff(a, d, e.service.table("Buff", "Id", "buff_203"), 7); err != nil {
		t.Fatal(err)
	}
	e.damageHP(a, d, 1001, 1)
	closeCombat(t, e.players[1].anger, 10)
	closeCombat(t, d.hpLossAccumulator, -1)
}
func TestP6OriginalHPAPCompoundAndReEffectCurrentMaxHPResidual(t *testing.T) {
	e := p6PropertyTransferEngine(t)
	a, d := e.players[0].actors[0], e.players[1].actors[0]
	d.base["maxHp"], d.hp = 1000, 1000
	for _, id := range []string{"buff_202", "buff_203"} {
		if err := e.loadBuff(a, d, e.service.table("Buff", "Id", id), 7); err != nil {
			t.Fatal(err)
		}
	}
	e.players[1].anger = 3
	e.damageHP(a, d, 110, 1)
	closeCombat(t, d.hp, 900)
	closeCombat(t, e.players[1].anger, 1)
	closeCombat(t, d.hpLossAccumulator, -100)
	e.damageHP(a, d, 1, 1)
	closeCombat(t, d.hp, 899)
	closeCombat(t, e.players[1].anger, 11)
	closeCombat(t, d.hpLossAccumulator, -1)
	for _, id := range []string{"buff_202", "buff_203"} {
		e = p6PropertyTransferEngine(t)
		a, d = e.players[0].actors[0], e.players[1].actors[0]
		row := e.service.table("Buff", "Id", id)
		row["CanBeCleared"] = "1"
		if err := e.loadBuff(a, d, row, 7); err != nil {
			t.Fatal(err)
		}
		suppressed, err := e.suppressBuffsOnce(d)
		if err != nil {
			t.Fatal(err)
		}
		if d.angerToHPAP != 0 || d.lossToAPAP != 0 || d.angerToHPAmount != 0 || d.lossToAPHP != 0 {
			t.Fatal("native transfer suppression did not subtract fields", id, len(suppressed), d.angerToHPAP, d.lossToAPAP, d.angerToHPAmount, d.lossToAPHP)
		}
		if err = e.restoreBuffsOnce(d, suppressed); err != nil {
			t.Fatal(err)
		}
		d.base["maxHp"] = 100000
		e.unloadBuff(d, d.buffs[0])
		if id == "buff_202" {
			closeCombat(t, d.angerToHPAmount, -200)
			if d.angerToHPAP != 0 {
				t.Fatal("AF not undone")
			}
		} else {
			closeCombat(t, d.lossToAPHP, 2000)
			if d.lossToAPAP != 0 {
				t.Fatal("loss AF not undone")
			}
		}
		if err = e.loadBuff(a, d, row, 7); err != nil {
			t.Fatal(err)
		}
		if id == "buff_202" {
			closeCombat(t, d.angerToHPAmount, 800)
		} else {
			closeCombat(t, d.lossToAPHP, -8000)
		}
		row["BuffClass"] = "0"
		if err = e.validateDynamicTrigger(combatRoundTrigger{buff: id, timer: 7}); err == nil {
			t.Fatal("unknown HP/AP factory accepted")
		}
	}
}
func p6HPAPProbeInputs(t *testing.T, s *GameService) []*model.FrozenBattleInput {
	var out []*model.FrozenBattleInput
	for _, id := range []string{"buff_202", "buff_203"} {
		encoded, _ := json.Marshal(s.Tables)
		var tables map[string][]map[string]string
		if err := json.Unmarshal(encoded, &tables); err != nil {
			t.Fatal(err)
		}
		talent := "p6_unmapped_probe_" + id
		tables["Talent"] = append(tables["Talent"], map[string]string{"Id": talent, "Buff0": id, "Buff0TriggerTimer": map[string]string{"buff_202": "7", "buff_203": "1"}[id], "Buff0Select": "0", "Buff0Percent": "100"})
		probe := &GameService{Tables: tables}
		actor := p6Actor("hero_047", "transfer-probe", "047_1,047_2,047_3", 1, 80000)
		actor.Inst.CurHp = proto.Float64(60000)
		actor.TalentList = []string{talent}
		enemy := p6Actor("hero_047", "transfer-enemy", "047_1,047_2,047_3", 1, 80000)
		enemy.Prop.Att = proto.Float64(10000)
		f := p6Input(t, probe, actor, enemy, 10)
		f.Attacker.Inst.Anger = proto.Float64(100)
		f.Defender.Inst.Anger = proto.Float64(100)
		out = append(out, f)
	}
	return out
}
func TestP6OriginalHPAPUnmappedFullSkillProbeReplayAndNoContentClaim(t *testing.T) {
	s := p6OriginalTables(t)
	for i, f := range p6HPAPProbeInputs(t, s) {
		probe := &GameService{Tables: f.Tables}
		out, err := probe.ComputeFrozenBattle(t.Context(), f, nil)
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"anger_absorb_hp", "hp_loss_to_anger"}[i]
		seen := false
		for _, v := range out.Events {
			if v.Kind == want {
				seen = true
			}
		}
		if !seen || out.Authoritative {
			t.Fatal("explicit unmapped conversion probe missing", i)
		}
		replay, err := probe.ComputeFrozenBattle(t.Context(), f, out.Operates)
		if err != nil || !proto.Equal(out.Operates, replay.Operates) || !proto.Equal(out.Attacker, replay.Attacker) || !proto.Equal(out.Defender, replay.Defender) || out.RandomDraws != replay.RandomDraws || !reflect.DeepEqual(out.Events, replay.Events) {
			t.Fatal("native conversion probe replay", i, err)
		}
	}
}
