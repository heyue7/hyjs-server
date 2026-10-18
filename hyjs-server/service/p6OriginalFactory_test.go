package service

import (
	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	"reflect"
	"testing"
)

func TestP6OriginalBuff722FactoryNoEffectIdentityCountAndLife(t *testing.T) {
	e := p6PropertyTransferEngine(t)
	a, d := e.players[0].actors[0], e.players[1].actors[0]
	row := e.service.table("Buff", "Id", "buff_722")
	if err := e.validateDynamicTrigger(combatRoundTrigger{buff: "buff_722", timer: 9, selectStrategy: 0, chance: 1}); err != nil {
		t.Fatal(err)
	}
	if err := e.loadBuff(a, d, map[string]string{"Id": "real-shield-alongside722", "Factor": "shield", "Args1": "100", "TotalRound": "3"}, 1); err != nil {
		t.Fatal(err)
	}
	beforeHP := d.hp
	for i := 0; i < 2; i++ {
		if err := e.loadBuff(a, d, row, 9); err != nil {
			t.Fatal(err)
		}
	}
	if len(d.buffs) != 2 || d.buffs[1].row["Id"] != "buff_722" || d.buffs[1].value != 0 || d.buffs[1].shieldKind() != "" {
		t.Fatal("original no-effect identity/count changed")
	}
	closeCombat(t, d.shield("shield"), 100)
	closeCombat(t, d.attribute("shield"), 100)
	closeCombat(t, d.hp, beforeHP)
	e.damageHP(a, d, 20, 1)
	closeCombat(t, d.shield("shield"), 80)
	e.unloadBuff(d, d.buffs[1])
	closeCombat(t, d.shield("shield"), 80)
	if err := e.loadBuff(a, d, row, 9); err != nil {
		t.Fatal(err)
	}
	e.round = 1
	if err := e.endRound(); err != nil {
		t.Fatal(err)
	}
	if len(d.buffs) != 1 || d.buffs[0].row["Id"] != "real-shield-alongside722" {
		t.Fatal("original one-round no-effect lifetime failed")
	}
	if e.rng.Draws() != 0 {
		t.Fatal("direct inert factory consumes RNG")
	}
}
func TestP6OriginalBuff722NoEffectReactivationAndDeath(t *testing.T) {
	e := p6PropertyTransferEngine(t)
	a, d := e.players[0].actors[0], e.players[1].actors[0]
	row := e.service.table("Buff", "Id", "buff_722")
	row["CanBeCleared"] = "1"
	if err := e.loadBuff(a, d, row, 9); err != nil {
		t.Fatal(err)
	}
	inert := d.buffs[0]
	suppressed, err := e.suppressBuffsOnce(d)
	if err != nil {
		t.Fatal(err)
	}
	a.base["att"] = 1000
	if err = e.restoreBuffsOnce(d, suppressed); err != nil {
		t.Fatal(err)
	}
	if d.buffs[0] != inert || inert.value != 0 || d.shield("shield") != 0 || inert.rounds != 1 {
		t.Fatal("inert original buff became numerical during reEffect")
	}
	e.damageHP(a, d, d.hp+1, 1)
	if d.state != 4 || len(d.buffs) != 0 {
		t.Fatal("dead actor retained inert buff")
	}
}
func p6Native722Input(t *testing.T, s *GameService) *model.FrozenBattleInput {
	f := p6SourceShieldInput(t, s)
	f.Attacker.Formation[0].TalentList = []string{"tal_2007"}
	return f
}
func TestP6OriginalBuff722UltimateHurtCompleteHeroesReplay(t *testing.T) {
	s := p6OriginalTables(t)
	f := p6Native722Input(t, s)
	out, err := s.ComputeFrozenBattle(t.Context(), f, nil)
	if err != nil {
		t.Fatal(err)
	}
	loads, inertEvents := 0, 0
	skills := map[string]bool{}
	for _, v := range out.Events {
		if v.Kind == "buff_load" && v.Buff == "buff_722" {
			loads++
		}
		if v.Kind == "native_buff_no_effect" && v.Buff == "buff_722" {
			inertEvents++
		}
		if v.Kind == "release" && v.Actor == "0_"+f.Attacker.Formation[0].GetGuid() {
			skills[v.Skill] = true
		}
	}
	if loads == 0 || loads != inertEvents || len(skills) != 3 || out.Authoritative {
		t.Fatal("original class0 no-effect hurt flow missing", loads, inertEvents, skills)
	}
	replay, err := s.ComputeFrozenBattle(t.Context(), f, out.Operates)
	if err != nil || !proto.Equal(out.Operates, replay.Operates) || !proto.Equal(out.Attacker, replay.Attacker) || !proto.Equal(out.Defender, replay.Defender) || out.RandomDraws != replay.RandomDraws || !reflect.DeepEqual(out.Events, replay.Events) {
		t.Fatal("original722 replay diverged", err)
	}
}
