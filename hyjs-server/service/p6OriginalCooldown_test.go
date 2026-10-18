package service

import (
	pb "hyjs-server/model/protocol"
	"testing"
)

func TestP6OriginalCooldownCurrentNormalPendingUltimateIdleAndNegative(t *testing.T) {
	for _, tc := range []struct {
		name     string
		current  int32
		pending  bool
		expected int32
	}{{"idle", -1, false, 0}, {"normal", 1, false, 1}, {"ultimate", 2, false, 2}, {"pending-ultimate-before-normal", 1, true, 2}} {
		t.Run(tc.name, func(t *testing.T) {
			s := p6OriginalTables(t)
			f := p6Input(t, s, p6Actor("hero_003", "owner", "003_1,003_2,003_3", 1, 40000), p6Actor("hero_047", "enemy", "047_1,047_2,047_3", 1, 40000), 12)
			e := &combatRoundEngine{service: &GameService{Tables: f.Tables}, input: f, rng: NewCombatRandom(42), operate: &pb.BattleOperate{}}
			if err := e.initialize(); err != nil {
				t.Fatal(err)
			}
			a := e.players[0].actors[0]
			for _, skill := range a.skills {
				skill.cooldown = 1
			}
			if tc.current >= 0 {
				a.current = a.skill(tc.current)
			}
			if tc.pending {
				e.pendingUltimate = a.skill(2)
			}
			if err := e.loadBuff(a, a, e.table("Buff", "Id", "buff_193"), 4); err != nil {
				t.Fatal(err)
			}
			for _, skill := range a.skills {
				want := int32(1)
				if skill.index == tc.expected {
					want = -1
				}
				if skill.cooldown != want {
					t.Fatal("original single active skill cooldown", skill.id, skill.cooldown, want)
				}
			}
			a.current = nil
			e.pendingUltimate = nil
			a.skill(tc.expected).state = 0
			if !a.ready(a.skill(tc.expected)) {
				t.Fatal("original cooldownCur<=0 readiness")
			}
		})
	}
}
