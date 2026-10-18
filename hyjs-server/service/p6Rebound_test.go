package service

import (
	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"reflect"
	"testing"
)

func p6ReboundEngine(t *testing.T) *combatRoundEngine {
	e := p6PropertyTransferEngine(t)
	for i := int32(2); i <= 3; i++ {
		e.input.Attacker.Formation = append(e.input.Attacker.Formation, p6Actor("hero_047", string(rune('a'+i)), "047_1,047_2,047_3", i, 80000))
	}
	e = &combatRoundEngine{service: e.service, input: e.input, rng: NewCombatRandom(0), operate: &pb.BattleOperate{}}
	if err := e.initialize(); err != nil {
		t.Fatal(err)
	}
	return e
}
func TestP6OriginalReboundDeathOrderSourceOnceTargetOnceAndCooldown(t *testing.T) {
	e := p6ReboundEngine(t)
	source, first, second := e.players[0].actors[0], e.players[0].actors[2], e.players[0].actors[1]
	source.talents = []combatRoundTrigger{{buff: "buff_457", timer: 2, selectStrategy: 130, chance: 1}}
	first.skills[0].cooldown = 3
	if err := e.completeActorDeath(first); err != nil {
		t.Fatal(err)
	}
	if err := e.completeActorDeath(second); err != nil {
		t.Fatal(err)
	}
	targets, err := e.selectTargets(source, 130, nil, nil)
	if err != nil || len(targets) != 1 || targets[0] != first || e.rng.Draws() != 0 {
		t.Fatal("native death order selector/zero RNG", err)
	}
	if err = e.talentTimer(source, nil, 2); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, first.hp, 800)
	closeCombat(t, first.attribute("damageAdd"), first.base["damageAdd"]+40)
	if first.state != 0 || !first.rebounded || !source.reboundUsed || len(first.buffs) != 1 || first.buffs[0].row["Id"] != "buff_456" || first.buffs[0].source != source || first.buffs[0].timer != 2 || first.skills[0].cooldown != 3 {
		t.Fatal("native rebound lifecycle/cooldown/source flags")
	}
	for _, skill := range first.skills {
		if skill.state != 0 {
			t.Fatal("native teammate revival failed ResetRound")
		}
	}
	if len(e.players[0].deadActors) != 1 || e.players[0].deadActors[0] != second {
		t.Fatal("revived actor stayed in death list")
	}
	if err = e.talentTimer(source, nil, 2); err != nil {
		t.Fatal(err)
	}
	if second.state != 4 || second.hp != 0 || e.rng.Draws() != 2 {
		t.Fatal("native source used gate or trigger RNG", second.state, e.rng.Draws())
	}
	if err = e.completeActorDeath(first); err != nil {
		t.Fatal(err)
	}
	other := e.players[1].actors[0]
	// Probe selection in its original camp with a fresh eligible source.
	second.state = 0
	second.hp = 80000
	e.players[0].deadActors = e.players[0].deadActors[1:]
	targets, err = e.selectTargets(second, 130, nil, nil)
	if err != nil || len(targets) != 0 || !first.rebounded || other.reboundUsed {
		t.Fatal("native target was rebound twice", err)
	}
}
func TestP6OriginalReboundClearsOldMarkerShieldsRetainsOverridesAndNoDeadSelfReviveQueue(t *testing.T) {
	e := p6ReboundEngine(t)
	source, target := e.players[0].actors[0], e.players[0].actors[1]
	if err := e.loadBuff(source, target, e.service.table("Buff", "Id", "buff_157"), 4); err != nil {
		t.Fatal(err)
	}
	target.skills[0].buffOverrides = map[string]string{"known": "buff_135"}
	if err := e.completeActorDeath(target); err != nil {
		t.Fatal(err)
	}
	if err := e.loadBuff(source, target, e.service.table("Buff", "Id", "buff_457"), 2); err != nil {
		t.Fatal(err)
	}
	if target.shield("shield") != 0 || len(target.buffs) != 1 || target.skills[0].buffOverrides["known"] != "buff_135" {
		t.Fatal("native rebound reset retained stale effect or reverted skill mutation")
	}
	// Original own Die revival never reaches deadActors.Add.
	r := e.players[0].actors[2]
	r.talents = []combatRoundTrigger{{buff: "buff_277", timer: 16, chance: 1}}
	if err := e.completeActorDeath(r); err != nil {
		t.Fatal(err)
	}
	if !r.revived || r.state != 0 || len(e.players[0].deadActors) != 0 {
		t.Fatal("self revived actor added to teammate death queue")
	}
	e.camp = 0
	if err := e.endRound(); err != nil {
		t.Fatal(err)
	}
	if target.attribute("damageAdd") != target.base["damageAdd"] || len(target.buffs) != 0 {
		t.Fatal("rebound child456 did not expire at original one round")
	}
}
func p6ReboundWholeInputs(t *testing.T, s *GameService) []*model.FrozenBattleInput {
	var out []*model.FrozenBattleInput
	for _, camp := range []int{0, 1} {
		reviver := p6Actor("hero_024", "team-reviver", "024_1,024_2,024_3", 2, 80000)
		reviver.TalentList = []string{"tal_345"}
		ally := p6Actor("hero_047", "fallen-ally", "047_1,047_2,047_3", 1, 80000)
		ally.Inst.CurHp = proto.Float64(20)
		enemy := p6Actor("hero_047", "rebound-enemy", "047_1,047_2,047_3", 1, 80000)
		var f *model.FrozenBattleInput
		if camp == 0 {
			f = p6Input(t, s, reviver, enemy, 14)
			f.Attacker.Formation = append(f.Attacker.Formation, ally)
		} else {
			f = p6Input(t, s, enemy, reviver, 14)
			f.Defender.Formation = append(f.Defender.Formation, ally)
		}
		f.Attacker.Inst.Anger = proto.Float64(100)
		f.Defender.Inst.Anger = proto.Float64(100)
		frozen, err := s.FreezeBattleInput(*f)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, frozen)
	}
	return out
}
func TestP6OriginalReboundHero024FullSkillsBothCampsAndReplay(t *testing.T) {
	s := p6OriginalTables(t)
	for i, f := range p6ReboundWholeInputs(t, s) {
		out, err := s.ComputeFrozenBattle(t.Context(), f, nil)
		if err != nil {
			t.Fatal(err)
		}
		seen := map[string]bool{}
		revives, afterRevive := 0, 0
		reviveRound := int32(0)
		for _, v := range out.Events {
			if v.Kind == "release" && (v.Skill == "024_1" || v.Skill == "024_2" || v.Skill == "024_3") {
				seen[v.Skill] = true
			}
			if v.Kind == "teammate_revive" {
				revives++
				reviveRound = v.Round
				closeCombat(t, v.TargetHP, 800)
			}
			if v.Kind == "release" && (v.Actor == "0_fallen-ally" || v.Actor == "1_fallen-ally") && reviveRound > 0 && v.Round >= reviveRound {
				afterRevive++
			}
		}
		if revives != 1 || afterRevive == 0 || len(seen) != 3 || out.Authoritative {
			t.Fatal("actual hero024 full teammate revival", i, revives, reviveRound, afterRevive, seen)
		}
		replay, err := s.ComputeFrozenBattle(t.Context(), f, out.Operates)
		if err != nil || !proto.Equal(out.Operates, replay.Operates) || !proto.Equal(out.Attacker, replay.Attacker) || !proto.Equal(out.Defender, replay.Defender) || out.RandomDraws != replay.RandomDraws || !reflect.DeepEqual(out.Events, replay.Events) {
			t.Fatal("native teammate rebound replay", i, err)
		}
	}
}
func TestP6OriginalReboundRejectsBadOriginFactoryAndEmptyNativeParam(t *testing.T) {
	e := p6PropertyTransferEngine(t)
	for _, tr := range []combatRoundTrigger{{buff: "buff_457", timer: 16, selectStrategy: 130}, {buff: "buff_457", timer: 2}, {buff: "buff_207", timer: 2, selectStrategy: 130}} {
		if err := e.validateDynamicTrigger(tr); err == nil {
			t.Fatal("unverified rebound origin/empty native template accepted")
		}
	}
	e.service.table("Buff", "Id", "buff_457")["BuffClass"] = "1"
	if err := e.validateDynamicTrigger(combatRoundTrigger{buff: "buff_457", timer: 2, selectStrategy: 130}); err == nil {
		t.Fatal("wrong rebound factory accepted")
	}
}
