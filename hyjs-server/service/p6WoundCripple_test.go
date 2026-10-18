package service

import (
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	"reflect"
	"testing"
)

func TestP6OriginalWeapon184Class0NoEffectIdentityAndCastLifetime(t *testing.T) {
	e := p6PropertyTransferEngine(t)
	a := e.players[0].actors[0]
	d := e.players[1].actors[0]
	before := map[string]float64{}
	for _, k := range []string{"att", "maxHp", "pdef", "mdef", "hit", "cri", "damageAdd", "WeaponPerAtt"} {
		before[k] = a.attribute(k)
	}
	row := e.service.table("Buff", "Id", "buff_184")
	if err := e.loadBuff(a, a, row, 5); err != nil {
		t.Fatal(err)
	}
	if !p6OriginalFactoryNoEffect(row) || len(a.buffs) != 1 || a.buffs[0].value != 0 || e.rng.Draws() != 0 {
		t.Fatal("native class0 WeaponPerAtt must stay inert")
	}
	for k, v := range before {
		closeCombat(t, a.attribute(k), v)
	}
	a.skills[0].target = d
	a.skills[0].targets = []*combatRoundActor{d}
	if err := e.dynamicRelease(a.skills[0], a.skills[0].casts[0]); err != nil {
		t.Fatal(err)
	}
	if len(a.buffs) != 0 {
		t.Fatal("native untimed weapon marker retained after Cast")
	}
	row["BuffClass"] = "1"
	if err := e.validateDynamicTrigger(combatRoundTrigger{buff: "buff_184", timer: 5}); err == nil {
		t.Fatal("unknown weapon factory accepted")
	}
}
func TestP6OriginalWoundRawHealBonusIrreversibleHPAndReEffect(t *testing.T) {
	e := p6PropertyTransferEngine(t)
	a, d := e.players[0].actors[0], e.players[1].actors[0]
	row := e.service.table("Buff", "Id", "buff_205")
	row["CanBeCleared"] = "1"
	d.hp = 72000
	if err := e.loadBuff(a, d, row, 1); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, d.hp, 73200)
	closeCombat(t, d.attribute("att"), d.base["att"]+120)
	b := d.buffs[0]
	suppressed, err := e.suppressBuffsOnce(d)
	if err != nil {
		t.Fatal(err)
	}
	closeCombat(t, d.hp, 73200)
	closeCombat(t, d.attribute("att"), d.base["att"])
	if err = e.restoreBuffsOnce(d, suppressed); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, d.hp, 74220)
	closeCombat(t, b.value, 102)
	closeCombat(t, d.attribute("att"), d.base["att"]+102)
	e.unloadBuff(d, b)
	closeCombat(t, d.hp, 74220)
	closeCombat(t, d.attribute("att"), d.base["att"])
	if e.rng.Draws() != 0 {
		t.Fatal("native wound Effect draws random")
	}
	// The bonus uses raw healing even when AddHp is penalized or forbidden.
	e = p6PropertyTransferEngine(t)
	a, d = e.players[0].actors[0], e.players[1].actors[0]
	d.hp = 72000
	if err := e.loadBuff(a, d, e.service.table("Buff", "Id", "buff_111"), 12); err != nil {
		t.Fatal(err)
	}
	if err := e.loadBuff(a, d, e.service.table("Buff", "Id", "buff_205"), 1); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, d.hp, 73140)
	closeCombat(t, d.buffs[1].value, 120)
	e = p6PropertyTransferEngine(t)
	a, d = e.players[0].actors[0], e.players[1].actors[0]
	d.hp = 72000
	if err := e.loadBuff(a, d, map[string]string{"Id": "forbid", "Factor": "forbidAddHp", "TotalRound": "4"}, 1); err != nil {
		t.Fatal(err)
	}
	if err := e.loadBuff(a, d, e.service.table("Buff", "Id", "buff_205"), 1); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, d.hp, 72000)
	closeCombat(t, d.attribute("att"), d.base["att"]+120)
	e.service.table("Buff", "Id", "buff_205")["BuffClass"] = "2"
	if err := e.validateDynamicTrigger(combatRoundTrigger{buff: "buff_205", timer: 1}); err == nil {
		t.Fatal("wrong wound factory accepted")
	}
}
func TestP6OriginalCrippleCastEndPhysicalShieldExpiryProtectionAndNoSelfKill(t *testing.T) {
	e := p6PropertyTransferEngine(t)
	a, d := e.players[0].actors[0], e.players[1].actors[0]
	if err := e.loadBuff(a, a, e.service.table("Buff", "Id", "buff_792"), 2); err != nil {
		t.Fatal(err)
	}
	if err := e.loadBuff(a, a, map[string]string{"Id": "typed-shield", "Factor": "pshield", "Args1": "1000", "TotalRound": "2"}, 2); err != nil {
		t.Fatal(err)
	}
	if err := e.loadBuff(a, a, map[string]string{"Id": "instant-att", "Factor": "att", "Args1": "1000"}, 2); err != nil {
		t.Fatal(err)
	}
	skill := a.skills[0]
	skill.target = d
	skill.targets = []*combatRoundActor{d}
	for i := 0; i < 2; i++ {
		if err := e.dynamicRelease(skill, skill.casts[0]); err != nil {
			t.Fatal(err)
		}
	}
	closeCombat(t, a.hp, 79400)
	closeCombat(t, a.shield("pshield"), 0)
	closeCombat(t, a.attribute("att"), a.base["att"])
	for i := 0; i < 2; i++ {
		if err := e.endRound(); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.originalCrippleCastEnd(a); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, a.hp, 79400)
	if err := e.loadBuff(a, a, e.service.table("Buff", "Id", "buff_792"), 2); err != nil {
		t.Fatal(err)
	}
	if err := e.loadBuff(a, a, e.service.table("Buff", "Id", "buff_196"), 24); err != nil {
		t.Fatal(err)
	}
	a.hp = .01
	if err := e.originalCrippleCastEnd(a); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, a.hp, .01)
	for _, b := range append([]*combatRoundBuff(nil), a.buffs...) {
		if b.row["Factor"] == "dieAfterLastAttack" {
			e.unloadBuff(a, b)
		}
	}
	a.hp = 100
	a.talents = []combatRoundTrigger{{buff: "buff_196", timer: 18, chance: 1}}
	if err := e.originalCrippleCastEnd(a); err != nil {
		t.Fatal(err)
	}
	if a.state != 4 || a.hp != 0 || a.hasState("dieAfterLastAttack") {
		t.Fatal("self cripple damage triggered self-killer or failed death")
	}
}
func p6UnmappedMechanismProbeInputs(t *testing.T, s *GameService) []*model.FrozenBattleInput {
	var out []*model.FrozenBattleInput
	// Explicit mechanism fixtures. These generated talent IDs have no original content mapping.
	for _, id := range []string{"buff_184", "buff_205", "buff_792"} {
		encoded, _ := json.Marshal(s.Tables)
		var tables map[string][]map[string]string
		if err := json.Unmarshal(encoded, &tables); err != nil {
			t.Fatal(err)
		}
		timer := "1"
		if id == "buff_792" {
			timer = "2"
		}
		talent := "p6_unmapped_probe_" + id
		tables["Talent"] = append(tables["Talent"], map[string]string{"Id": talent, "Buff0": id, "Buff0TriggerTimer": timer, "Buff0Select": "0", "Buff0Percent": "100"})
		probe := &GameService{Tables: tables}
		actor := p6Actor("hero_047", "probe-source", "047_1,047_2,047_3", 1, 80000)
		actor.Inst.CurHp = proto.Float64(60000)
		actor.TalentList = []string{talent}
		f := p6Input(t, probe, actor, p6Actor("hero_047", "probe-enemy", "047_1,047_2,047_3", 1, 80000), 10)
		f.Attacker.Inst.Anger = proto.Float64(100)
		f.Defender.Inst.Anger = proto.Float64(100)
		out = append(out, f)
	}
	return out
}
func TestP6OriginalUnmappedWoundWeaponCrippleMechanismProbeCompleteReplay(t *testing.T) {
	s := p6OriginalTables(t)
	for i, f := range p6UnmappedMechanismProbeInputs(t, s) {
		probe := &GameService{Tables: f.Tables}
		out, err := probe.ComputeFrozenBattle(t.Context(), f, nil)
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"native_buff_no_effect", "wound_treat", "cripple_tick"}[i]
		seen := false
		for _, v := range out.Events {
			if v.Kind == want {
				seen = true
			}
		}
		if !seen || out.Authoritative || f.Attacker.Formation[0].TalentList[0][:18] != "p6_unmapped_probe_" {
			t.Fatal("unmapped mechanism fixture was not explicit", i)
		}
		replay, err := probe.ComputeFrozenBattle(t.Context(), f, out.Operates)
		if err != nil || !proto.Equal(out.Operates, replay.Operates) || !proto.Equal(out.Attacker, replay.Attacker) || !proto.Equal(out.Defender, replay.Defender) || out.RandomDraws != replay.RandomDraws || !reflect.DeepEqual(out.Events, replay.Events) {
			t.Fatal("unmapped mechanism whole replay", i, err)
		}
	}
}
