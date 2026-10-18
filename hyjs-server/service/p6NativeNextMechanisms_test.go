package service

import (
	"testing"
)

func TestP6NativeOriginalNextRealTickGolden(t *testing.T) {
	p6NativeAutomaticPVPOriginalTickGolden(t, "native-next-real-golden-v54.json", "9046936d6c5691e792305abd5554a02cc23f652da3a10db5dc1ae319c7f59fc1", 14)
}

func TestP6NativeOriginalNextRealActorDetailAndBuffMetadataGoldens(t *testing.T) {
	const (
		file = "native-next-real-golden-v54.json"
		sha  = "9046936d6c5691e792305abd5554a02cc23f652da3a10db5dc1ae319c7f59fc1"
	)
	t.Run("actor-properties", func(t *testing.T) { p6NativeActorPropertiesFromFixture(t, "", true, file, sha) })
	t.Run("skill-CD-Buff-lifetime-shields", func(t *testing.T) { p6NativeDetailsFromFixture(t, "", true, file, sha) })
	t.Run("Buff-source-values-flags", func(t *testing.T) { p6NativeBuffSourceAndValuesFromFixture(t, "", true, file, sha) })
}

func TestP6OriginalFirstInvisibleFallbackAndRowPreference(t *testing.T) {
	e := p6SameDamageEngine(t)
	a := e.players[0].actors[0]
	actors := e.players[1].actors
	for index, actor := range actors {
		actor.runtimePose = int32(index*5 + 1)
	}
	for index, strategy := range []int32{11, 12, 13} {
		selected, err := e.selectTargets(a, strategy, nil, nil)
		if err != nil || len(selected) != 1 || selected[0] != actors[index] {
			t.Fatal("original preferred row", strategy, err)
		}
	}
	actors[1].state = 4
	selected, err := e.selectTargets(a, 12, nil, nil)
	if err != nil || len(selected) != 1 || selected[0] != actors[0] {
		t.Fatal("middle row did not fall back to first row", err)
	}
	actors[2].state = 4
	selected, err = e.selectTargets(a, 13, nil, nil)
	if err != nil || len(selected) != 1 || selected[0] != actors[0] {
		t.Fatal("back row did not fall back through middle to first row", err)
	}
	if err = e.loadBuff(a, actors[0], map[string]string{"Id": "original-first-hidden-probe", "Factor": "invisible", "TotalRound": "1"}, 4); err != nil {
		t.Fatal(err)
	}
	selected, err = e.selectTargets(a, 3, nil, nil)
	if err != nil || len(selected) != 1 || selected[0] != actors[0] {
		t.Fatal("FIRST failed original all-invisible fallback", err)
	}
	selected, err = e.selectTargets(a, 10, actors[0], nil)
	if err != nil || len(selected) != 1 || selected[0] != actors[0] {
		t.Fatal("original invisible reference exception lost", err)
	}
	actors[0].hp, actors[0].state = 0, 3
	selected, err = e.selectTargets(a, 3, nil, nil)
	if err != nil || len(selected) != 0 {
		t.Fatal("invisibility fallback admitted original weak death", err)
	}
}

func TestP6OriginalSingleDispelRangeAndUngroupedFriendlyTalentGuard(t *testing.T) {
	e := p6PropertyTransferEngine(t)
	owner, target := e.players[0].actors[0], e.players[1].actors[0]
	for _, id := range []string{"buff_111", "buff_149"} {
		if err := e.loadBuff(owner, target, e.table("Buff", "Id", id), 12); err != nil {
			t.Fatal(err)
		}
	}
	e.originalDispelEffect(target, e.table("Buff", "Id", "buff_131"))
	if len(target.buffs) != 1 || target.buffs[0].row["Id"] != "buff_111" || e.rng.Draws() != 1 {
		t.Fatal("native one-debuff Range selected the first or skipped RNG")
	}
	e.originalDispelEffect(target, e.table("Buff", "Id", "buff_131"))
	e.originalDispelEffect(target, e.table("Buff", "Id", "buff_131"))
	if len(target.buffs) != 0 || e.rng.Draws() != 2 {
		t.Fatal("single candidate or empty collection RNG boundary")
	}
	trigger := combatRoundTrigger{origin: combatRoundBuffOrigin{talent: "tal_257"}, buff: "buff_149", timer: 5, selectStrategy: 0, chance: 1}
	if err := e.applyTrigger(owner, owner, trigger, nil); err != nil || len(owner.buffs) != 0 || e.rng.Draws() != 2 {
		t.Fatal("empty talent Group bypassed friendly-debuff guard", err)
	}
	trigger.origin = combatRoundBuffOrigin{skill: "original-explicit-skill-probe", cast: "cast-probe"}
	if err := e.applyTrigger(owner, owner, trigger, nil); err != nil || len(owner.buffs) != 1 || e.rng.Draws() != 3 {
		t.Fatal("talent guard incorrectly blocked original SkillBuff contract", err)
	}
}

func TestP6OriginalTimedAndRoundDelayedBuffBoundaries(t *testing.T) {
	for _, id := range []string{"buff_mstone", "buff_delay"} {
		t.Run(id, func(t *testing.T) {
			e := p6PropertyTransferEngine(t)
			owner, target := e.players[0].actors[0], e.players[1].actors[0]
			e.round, e.time = 2, 32.3
			origin := combatRoundBuffOrigin{skill: "0005_1", cast: "2363"}
			if err := e.loadBuffWithOrigin(owner, target, e.table("Buff", "Id", id), 4, origin); err != nil {
				t.Fatal(err)
			}
			e.round = 3 // Original FinishRound advances this before minusRound0.
			if err := e.originalBuffRoundEnd(target); err != nil {
				t.Fatal(err)
			}
			if id == "buff_mstone" {
				if len(target.buffs) != 0 {
					t.Fatal("timed helper or its exhausted one-round child survived RoundEnd")
				}
			} else if len(target.buffs) != 2 || target.buffs[0].rounds != 1 || target.buffs[1].rounds != 1 || target.buffs[0].origin != origin || target.buffs[1].origin != origin {
				t.Fatal("round-delay child counts, native live cursor or source origin changed")
			}
			if e.rng.Draws() != 0 || e.table("Buff", "Id", "buff_mstone1")["TotalRound"] != "1" || e.table("Buff", "Id", "buff_mstone2")["TotalRound"] != "2" {
				t.Fatal("delay changed RNG or original table rows")
			}
		})
	}
}

func TestP6OriginalPermaxHPRecoveryKeepsMaximumAndDoesNotRollBack(t *testing.T) {
	e := p6PropertyTransferEngine(t)
	owner, target := e.players[0].actors[0], e.players[1].actors[0]
	maximum := target.attribute("maxHp")
	target.hp = maximum * .5
	row := e.table("Buff", "Id", "buff_424")
	if err := e.loadBuff(owner, target, row, 4); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, target.attribute("maxHp"), maximum)
	closeCombat(t, target.hp, maximum*.55)
	b := target.buffs[len(target.buffs)-1]
	closeCombat(t, b.initialValue, maximum*.05)
	e.unloadBuff(target, b)
	closeCombat(t, target.hp, maximum*.55)
	closeCombat(t, target.attribute("maxHp"), maximum)
}
