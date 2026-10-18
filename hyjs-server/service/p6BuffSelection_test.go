package service

import (
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p6AreaBuffInput(t *testing.T, s *GameService) *model.FrozenBattleInput {
	input := p6Input(t, s, p6Actor("hero_008", "area-fire", "008_1,008_2,008_3", 1, 80000), p6Actor("hero_047", "area-first", "047_1", 1, 80000), 18)
	input.Attacker.Inst.Anger = proto.Float64(100)
	input.Defender.Formation = append(input.Defender.Formation, p6Actor("hero_047", "area-second", "047_1", 2, 80000), p6Actor("hero_047", "area-third", "047_1", 3, 80000))
	return input
}

func TestP6OriginalAreaHitBuffAppliesOncePerActualTargetAndWholeReplay(t *testing.T) {
	s := p6OriginalTables(t)
	input := p6AreaBuffInput(t, s)
	out, err := s.ComputeFrozenBattle(t.Context(), input, nil)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, event := range out.Events {
		if event.Round == 1 && event.Kind == "buff_load" && event.Buff == "buff_360" {
			counts[event.Target]++
		}
	}
	if len(counts) != 3 {
		t.Fatal("original area cast did not reach all original targets", counts)
	}
	for target, count := range counts {
		if count != 1 {
			t.Fatal("one actual hit expanded its Buff into the full area again", target, count)
		}
	}
	replayed, err := s.ComputeFrozenBattle(t.Context(), input, out.Operates)
	if err != nil || !proto.Equal(out.Operates, replayed.Operates) || !proto.Equal(out.Defender, replayed.Defender) || !proto.Equal(out.Attacker, replayed.Attacker) || out.RandomDraws != replayed.RandomDraws || out.Authoritative {
		t.Fatal("area hit Buff whole replay changed", err)
	}
}

func TestP6OriginalBuffSelectionWeakInvisibleInvincibleAndCastClasses(t *testing.T) {
	s := p6OriginalTables(t)
	input := p6AreaBuffInput(t, s)
	e := &combatRoundEngine{service: &GameService{Tables: input.Tables}, input: input, rng: NewCombatRandom(input.Seed), operate: &pb.BattleOperate{}}
	if err := e.initialize(); err != nil {
		t.Fatal(err)
	}
	a, d, other := e.players[0].actors[0], e.players[1].actors[0], e.players[1].actors[1]
	d.hp, d.state = 0, 3
	if err := e.loadBuff(d, other, map[string]string{"Id": "buff-selection-invisible", "Factor": "invisible", "TotalRound": "1"}, 4); err != nil {
		t.Fatal(err)
	}
	selected, err := e.selectTargetsWithConditions(a, 10, d, nil, true, true, false)
	if err != nil || len(selected) != 3 {
		t.Fatal("Buff scope excluded weak-dead or invisible targets", err)
	}
	if err = e.loadBuff(d, other, map[string]string{"Id": "buff-selection-invincible", "Factor": "invincible", "TotalRound": "1"}, 4); err != nil {
		t.Fatal(err)
	}
	selected, err = e.selectTargetsWithConditions(a, 10, d, nil, true, true, false)
	if err != nil || len(selected) != 2 {
		t.Fatal("Debuff scope included invincible target", err)
	}
	selected, err = e.selectTargetsWithConditions(a, 10, d, nil, true, true, true)
	if err != nil || len(selected) != 3 {
		t.Fatal("beneficial Buff scope excluded invincible target", err)
	}
	skill := a.skill(1)
	skill.target, skill.targets = d, []*combatRoundActor{d, other}
	// Original 008_3 cast152 hits its area, then debuffs each current reference once.
	cast := e.service.table("SkillCast", "SkillCastId", "152")
	before := len(e.events)
	if err = e.castBuffs(skill, cast, 1, 12, d); err != nil {
		t.Fatal(err)
	}
	weakDebuff := 0
	for _, event := range e.events[before:] {
		if event.Kind == "buff_load" && event.Buff == "buff_360" {
			if event.Target != d.uid {
				t.Fatal("hit Buff escaped its current reference", event)
			}
			weakDebuff++
		}
	}
	if weakDebuff != 1 || d.hp != 0 {
		t.Fatal("weak target lost original non-HP Buff trigger", weakDebuff, d.hp)
	}
	// Original class0 cast19 keeps the prior cast collection; invincible debuff
	// receivers are filtered before loading, while weak and invisible are allowed.
	skill.targets = append(skill.targets, e.players[1].actors[2])
	before = len(e.events)
	if err = e.castBuffs(skill, e.service.table("SkillCast", "SkillCastId", "19"), 0, 4, d); err != nil {
		t.Fatal(err)
	}
	preCast := map[string]int{}
	for _, event := range e.events[before:] {
		if event.Kind == "buff_load" && event.Buff == "buff_120" {
			preCast[event.Target]++
		}
	}
	if preCast[d.uid] != 1 || preCast[e.players[1].actors[2].uid] != 1 || len(preCast) != 2 {
		t.Fatal("before-cast collection lost its distinct original filtering", preCast)
	}
}
