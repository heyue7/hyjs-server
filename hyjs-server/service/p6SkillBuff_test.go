package service

import (
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"reflect"
	"strings"
	"testing"
)

func p6SkillReplacementInput(t *testing.T, s *GameService, hero, skills, talent string) *model.FrozenBattleInput {
	a := p6Actor(hero, "replace-source", skills, 1, 80000)
	a.TalentList = []string{talent}
	f := p6Input(t, s, a, p6Actor("hero_047", "replace-enemy", "047_1,047_2,047_3", 1, 80000), 18)
	f.Attacker.Inst.Anger = proto.Float64(100)
	return f
}
func p6SkillReplacementWholeInputs(t *testing.T, s *GameService) []*model.FrozenBattleInput {
	var out []*model.FrozenBattleInput
	for _, x := range []struct{ hero, skills, talent string }{{"hero_065", "065_1,065_2,065_3", "tal_394"}, {"hero_066", "066_1,066_2,066_3", "tal_399"}, {"hero_068", "068_1,068_2,068_3", "tal_404"}, {"hero_071", "071_1,071_2,071_3", "tal_410"}} {
		out = append(out, p6SkillReplacementInput(t, s, x.hero, x.skills, x.talent))
	}
	return out
}
func TestP6OriginalSkillReplacementFiveRowsActorLocalPersistentAndImmutableTables(t *testing.T) {
	s := p6OriginalTables(t)
	for _, id := range []string{"buff_568", "buff_579", "buff_746", "buff_758", "buff_761"} {
		row := s.table("Buff", "Id", id)
		parts := strings.Split(row["Param"], ",")
		hero := "hero_" + parts[0][:3]
		skills := parts[0][:3] + "_1," + parts[0][:3] + "_2," + parts[0][:3] + "_3"
		f := p6Input(t, s, p6Actor(hero, "owner", skills, 1, 80000), p6Actor(hero, "other", skills, 1, 80000), 8)
		e := &combatRoundEngine{service: &GameService{Tables: f.Tables}, input: f, rng: NewCombatRandom(0), operate: &pb.BattleOperate{}}
		if err := e.initialize(); err != nil {
			t.Fatal(err)
		}
		before, _ := json.Marshal(f.Tables)
		a, d := e.players[0].actors[0], e.players[1].actors[0]
		row = e.service.table("Buff", "Id", id)
		row["CanBeCleared"] = "1"
		before, _ = json.Marshal(f.Tables)
		if err := e.loadBuff(a, a, row, 1); err != nil {
			t.Fatal(err)
		}
		changed := 0
		for _, skill := range a.skills {
			if skill.id != parts[0] {
				continue
			}
			for _, cast := range skill.casts {
				for _, slot := range []string{"0", "1", "2"} {
					if cast["BufferId"+slot] == parts[1] {
						changed++
						if skill.originalCastBuffID(cast, slot) != parts[2] {
							t.Fatal("native skill trigger not replaced")
						}
					}
				}
			}
		}
		if changed == 0 {
			t.Fatal("original replacement matches no cast", id)
		}
		for _, skill := range d.skills {
			if len(skill.buffOverrides) > 0 {
				t.Fatal("another actor's skill was mutated")
			}
		}
		suppressed, err := e.suppressBuffsOnce(a)
		if err != nil {
			t.Fatal(err)
		}
		if err = e.restoreBuffsOnce(a, suppressed); err != nil {
			t.Fatal(err)
		}
		e.unloadBuff(a, a.buffs[0])
		e.round, e.camp = 1, 0
		if err = e.beginRound(); err != nil {
			t.Fatal(err)
		}
		if err = e.completeActorDeath(a); err != nil {
			t.Fatal(err)
		}
		preserved := 0
		for _, skill := range a.skills {
			preserved += len(skill.buffOverrides)
		}
		after, _ := json.Marshal(f.Tables)
		if preserved != changed || string(before) != string(after) || e.rng.Draws() != 0 {
			t.Fatal("native irreversible replacement or immutable input violated", id, preserved, changed)
		}
	}
}
func TestP6OriginalIceYNTRatesBothCountersNoStunAndFourRoundLifecycle(t *testing.T) {
	e := p6PropertyTransferEngine(t)
	a, d := e.players[0].actors[0], e.players[1].actors[0]
	row := e.service.table("Buff", "Id", "buff_573")
	for _, id := range []string{"buff_573", "buff_583"} {
		if err := e.loadBuff(a, d, e.service.table("Buff", "Id", id), 12); err != nil {
			t.Fatal(err)
		}
	}
	closeCombat(t, d.attribute("hit"), d.base["hit"]-40)
	closeCombat(t, d.attribute("dodge"), d.base["dodge"]-40)
	closeCombat(t, d.attribute("cri"), d.base["cri"]-20)
	closeCombat(t, d.attribute("damageAdd"), d.base["damageAdd"]-50)
	closeCombat(t, d.attribute("treatAdd"), -50)
	if e.conditionProperty(d, "ice") != 2 || e.conditionProperty(d, "iceYNT") != 1 || d.hasState("dizzy") || d.hp != 80000 || e.rng.Draws() != 0 {
		t.Fatal("native iceYNT rates/count or action state mismatch")
	}
	for i := 0; i < 4; i++ {
		if err := e.endRound(); err != nil {
			t.Fatal(err)
		}
	}
	if d.hasState("iceYNT") || d.hasState("ice") || d.attribute("damageAdd") != d.base["damageAdd"] || d.attribute("treatAdd") != 0 {
		t.Fatal("iceYNT lifetime did not revert rates")
	}
	row["BuffClass"] = "0"
	if err := e.validateDynamicTrigger(combatRoundTrigger{buff: "buff_573", timer: 12}); err == nil {
		t.Fatal("wrong iceYNT state factory accepted")
	}
}
func TestP6OriginalIceYNTNativeNoDebuffGroupAndClearable(t *testing.T) {
	e := p6PropertyTransferEngine(t)
	a, d := e.players[0].actors[0], e.players[1].actors[0]
	d.immuneBuff["group_ice"] = true
	if err := e.loadBuff(d, d, map[string]string{"Id": "immune", "Factor": "immuneDebuff", "TotalRound": "4"}, 4); err != nil {
		t.Fatal(err)
	}
	if err := e.loadBuff(a, d, e.service.table("Buff", "Id", "buff_583"), 12); err != nil {
		t.Fatal(err)
	}
	if d.hasState("ice") {
		t.Fatal("original grouped ice bypassed immunity")
	}
	if err := e.loadBuff(a, d, e.service.table("Buff", "Id", "buff_573"), 12); err != nil {
		t.Fatal(err)
	}
	if !d.hasState("iceYNT") {
		t.Fatal("573 has no debuff/group flags and must load")
	}
	suppressed, err := e.suppressBuffsOnce(d)
	if err != nil {
		t.Fatal(err)
	}
	if d.hasState("iceYNT") || d.attribute("damageAdd") != d.base["damageAdd"] {
		t.Fatal("clearable positive573 did not suppress")
	}
	if err = e.restoreBuffsOnce(d, suppressed); err != nil {
		t.Fatal(err)
	}
	if !d.hasState("iceYNT") || d.attribute("damageAdd") != d.base["damageAdd"]-50 {
		t.Fatal("iceYNT restore lost property/state")
	}
	for _, b := range append([]*combatRoundBuff(nil), d.buffs...) {
		if b.row["CanBeCleared"] == "1" && b.row["IsDebuff"] != "1" {
			e.unloadBuff(d, b)
		}
	}
	if d.hasState("iceYNT") || d.attribute("treatAdd") != 0 {
		t.Fatal("positive clear left native iceYNT effects")
	}
}
func TestP6OriginalSkillReplacementFourActualHeroesFullSkillsAndReplay(t *testing.T) {
	s := p6OriginalTables(t)
	for i, f := range p6SkillReplacementWholeInputs(t, s) {
		before, _ := json.Marshal(f)
		out, err := s.ComputeFrozenBattle(t.Context(), f, nil)
		if err != nil {
			t.Fatal(i, err)
		}
		seen := map[string]bool{}
		replaced, loaded := map[string]bool{}, map[string]bool{}
		for _, v := range out.Events {
			if v.Kind == "release" && v.Actor == "0_replace-source" {
				seen[v.Skill] = true
			}
			if v.Kind == "skill_buff_replace" {
				replaced[v.Buff] = true
			}
			if v.Kind == "buff_load" {
				loaded[v.Buff] = true
			}
		}
		want := []string{"buff_135"}
		if i == 1 {
			want = []string{"buff_573"}
		}
		if i == 2 {
			want = []string{"buff_742"}
		}
		if i == 3 {
			want = []string{"buff_759", "buff_760"}
		}
		for _, id := range want {
			if !replaced[id] || !loaded[id] {
				t.Fatal("replacement template did not actually trigger", i, id, replaced, loaded)
			}
		}
		if len(seen) != 3 || out.Authoritative {
			t.Fatal("native full replacement skills", i, seen)
		}
		replay, err := s.ComputeFrozenBattle(t.Context(), f, out.Operates)
		after, _ := json.Marshal(f)
		if err != nil || !proto.Equal(out.Operates, replay.Operates) || !proto.Equal(out.Attacker, replay.Attacker) || !proto.Equal(out.Defender, replay.Defender) || out.RandomDraws != replay.RandomDraws || !reflect.DeepEqual(out.Events, replay.Events) || string(before) != string(after) {
			t.Fatal("native replacement replay/input diverged", i, err)
		}
	}
}
func TestP6OriginalSkillReplacementRejectsBadParamsFactoryAndUnknownTemplate(t *testing.T) {
	for _, param := range []string{"", "065_2,buff_403", "065_2,buff_403,unknown", "unknown,buff_403,buff_135", "065_2,buff_403,buff_207"} {
		e := p6PropertyTransferEngine(t)
		e.service.table("Buff", "Id", "buff_568")["Param"] = param
		if err := e.validateDynamicTrigger(combatRoundTrigger{buff: "buff_568", timer: 1}); err == nil {
			t.Fatal("unverified replacement accepted", param)
		}
	}
	e := p6PropertyTransferEngine(t)
	e.service.table("Buff", "Id", "buff_568")["BuffClass"] = "1"
	if err := e.validateDynamicTrigger(combatRoundTrigger{buff: "buff_568", timer: 1}); err == nil {
		t.Fatal("wrong replacement factory accepted")
	}
}
