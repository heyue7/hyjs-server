package service

import (
	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestP6OriginalRemainingConditionLivePropertiesAndClassPersistence(t *testing.T) {
	e := p6SameDamageEngine(t)
	owner, enemy := e.players[0].actors[0], e.players[1].actors[0]
	closeCombat(t, e.conditionProperty(owner, "doskillNum"), 0)
	owner.current = owner.skills[0]
	closeCombat(t, e.conditionProperty(owner, "doskillNum"), 1)
	e.players[0].actors = append(e.players[0].actors, enemy)
	enemy.current, enemy.state, enemy.hp = enemy.skills[2], 3, 0
	closeCombat(t, e.conditionProperty(owner, "doskillNum"), 2)
	enemy.state = 4
	closeCombat(t, e.conditionProperty(owner, "doskillNum"), 2)
	e.players[0].actors = e.players[0].actors[:1]
	closeCombat(t, e.conditionProperty(owner, "aliveEnemyNum"), 2)
	enemy.state = 3
	closeCombat(t, e.conditionProperty(owner, "aliveEnemyNum"), 3)
	e.players[0].anger = 50.125
	closeCombat(t, e.conditionProperty(owner, "anger"), 50.125)
	owner.base["damageReduce"] = 20
	closeCombat(t, e.conditionProperty(owner, "damageReduce"), .2)
	closeCombat(t, e.conditionProperty(owner, "skillClass"), -1)
	enemy.state, enemy.hp, enemy.current = 0, 80000, nil
	owner.current = nil
	owner.skills[0].target = enemy
	owner.skills[0].targets = []*combatRoundActor{enemy}
	if err := e.dynamicRelease(owner.skills[0], owner.skills[0].casts[0]); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, e.conditionProperty(owner, "skillClass"), tableNumber(owner.skills[0].row, "SkillClass"))
	if err := e.beginRound(); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, e.conditionProperty(owner, "skillClass"), tableNumber(owner.skills[0].row, "SkillClass"))
}

func TestP6OriginalAllTwentySixRemainingConditionSlotsBothBranches(t *testing.T) {
	count := 0
	for _, id := range []string{"tal_276", "tal_284", "tal_315", "tal_321", "tal_322", "tal_341", "tal_342", "tal_344", "tal_407", "tal_412", "tal_415", "tal_3239"} {
		e := p6SameDamageEngine(t)
		owner, target := e.players[0].actors[0], e.players[1].actors[0]
		row := e.service.table("Talent", "Id", id)
		for i := 0; i < 4; i++ {
			key := "Buff" + strconv.Itoa(i)
			if row[key+"TriggerCondition"] == "" || row[key] == "" {
				continue
			}
			trigger := combatRoundTrigger{buff: row[key], timer: int32(tableNumber(row, key+"TriggerTimer")), selectStrategy: int32(tableNumber(row, key+"Select")), condition: int32(tableNumber(row, key+"TriggerCondition")), args: row[key+"ConditionArgs"], chance: tableNumber(row, key+"Percent") * .01}
			if err := e.validateDynamicTrigger(trigger); err != nil {
				t.Fatal(id, key, err)
			}
			parsed := parseP6Condition(trigger.args)
			values := []float64{}
			for _, value := range []float64{-1, 0, 1, 2, 3, 49.999, 50, 50.001} {
				for _, actor := range e.players[1].actors {
					actor.state = 4
				}
				e.players[0].actors = e.players[0].actors[:1]
				owner.current = nil
				switch parsed["type"] {
				case "doskillNum":
					if value != 0 && value != 1 && value != 2 {
						continue
					}
					if value >= 1 {
						owner.current = owner.skills[0]
					}
					if value == 2 {
						target.current = target.skills[2]
						e.players[0].actors = append(e.players[0].actors, target)
					}
				case "anger":
					e.players[0].anger = value
				case "skillClass":
					target.curSkillClass = int32(value)
				case "aliveEnemyNum":
					if value < 0 || value > 3 {
						continue
					}
					for index := 0; index < int(value); index++ {
						e.players[1].actors[index].state = 3
					}
				case "damageReduce":
					owner.base["damageReduce"], target.base["damageReduce"] = value, 1
				}
				if e.conditionTrue(trigger, owner, target) {
					values = append(values, 1)
				} else {
					values = append(values, 0)
				}
			}
			seenTrue, seenFalse := false, false
			for _, value := range values {
				seenTrue, seenFalse = seenTrue || value == 1, seenFalse || value == 0
			}
			if !seenTrue || !seenFalse {
				t.Fatal("original condition lacks trigger/nontrigger boundary", id, key, values)
			}
			count++
		}
	}
	if count != 26 {
		t.Fatal("original missing condition slot inventory drifted", count)
	}
}

func p6RemainingConditionWholeInputs(t *testing.T, s *GameService) []*model.FrozenBattleInput {
	var inputs []*model.FrozenBattleInput
	for _, tc := range []struct{ hero, talents string }{
		{"hero_015", "tal_276"}, {"hero_017", "tal_284"}, {"hero_026", "tal_315"},
		{"hero_028", "tal_321"}, {"hero_054", "tal_321"}, {"hero_029", "tal_322"},
		{"hero_005", "tal_341,tal_342,tal_344"}, {"hero_068", "tal_407"},
		{"hero_072", "tal_412,tal_415"}, {"hero_082", "tal_3239"},
	} {
		row := s.table("Hero", "id", tc.hero)
		for _, enemyCount := range []int{1, 3} {
			owner := p6Actor(tc.hero, "original-condition", row["SkillId1"]+","+row["SkillId2"]+","+row["UltSkillId"], 1, 80000)
			owner.TalentList = strings.Split(tc.talents, ",")
			owner.Prop.DamageReduce = proto.Float64(10)
			f := p6Input(t, s, owner, p6Actor("hero_005", "original-enemy-0", "005_1,005_2,005_3", 1, 80000), 12)
			f.Attacker.Formation = append(f.Attacker.Formation, p6Actor("hero_047", "original-concurrent-mate", "047_1,047_2,047_3", 2, 80000))
			for index := 1; index < enemyCount; index++ {
				f.Defender.Formation = append(f.Defender.Formation, p6Actor("hero_047", "original-enemy-"+strconv.Itoa(index), "047_1,047_2,047_3", int32(index+1), 80000))
			}
			f.Attacker.Inst.Anger, f.Defender.Inst.Anger = proto.Float64(100), proto.Float64(100)
			inputs = append(inputs, f)
		}
	}
	return inputs
}

func TestP6OriginalRemainingConditionsTenMappedHeroesWholeReplay(t *testing.T) {
	for _, f := range p6RemainingConditionWholeInputs(t, p6OriginalTables(t)) {
		s := &GameService{Tables: f.Tables}
		out, err := s.ComputeFrozenBattle(t.Context(), f, nil)
		if err != nil {
			t.Fatal(f.Attacker.Formation[0].GetId(), err)
		}
		if out.Authoritative {
			t.Fatal("restoring native conditions cannot grant authority")
		}
		replay, err := s.ComputeFrozenBattle(t.Context(), f, out.Operates)
		if err != nil || !proto.Equal(out.Operates, replay.Operates) || !proto.Equal(out.Attacker, replay.Attacker) || !proto.Equal(out.Defender, replay.Defender) || out.RandomDraws != replay.RandomDraws || !reflect.DeepEqual(out.Events, replay.Events) {
			t.Fatal("actual high-tier original condition replay", err)
		}
	}
}
