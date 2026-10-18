package service

import (
	"math"
	"testing"
)

// FRelease pushes visual effectKind=8 followed by cast.exportType=1/2 to AddHp.
// AddHp checks its fourth argument for shields; kind8 alone cannot bypass them.
func TestP6OriginalExtraDamageExportTypeShieldSpillAndHPAPOrder(t *testing.T) {
	for _, mechanism := range []string{"same", "share"} {
		for _, typ := range []int32{1, 2} {
			t.Run(mechanism+string(rune('0'+typ)), func(t *testing.T) {
				e := p6SameDamageEngine(t)
				source, hit, recipient := e.players[0].actors[0], e.players[1].actors[0], e.players[1].actors[1]
				for _, x := range []struct {
					factor, amount string
				}{{"pshield", "5"}, {"mshield", "11"}, {"shield", "7"}} {
					if err := e.loadBuff(recipient, recipient, map[string]string{"Id": x.factor, "Factor": x.factor, "Args1": x.amount, "TotalRound": "4"}, 4); err != nil {
						t.Fatal(err)
					}
				}
				skill := source.skills[0]
				skill.target = hit
				damage := 75.0
				if mechanism == "same" {
					if err := e.loadBuff(source, source, e.service.table("Buff", "Id", "buff_311"), 5); err != nil {
						t.Fatal(err)
					}
					skill.sameDamTargets = []*combatRoundActor{recipient}
					if err := e.originalSameDamage(skill, hit, 100, typ); err != nil {
						t.Fatal(err)
					}
					if e.rng.Draws() != 0 {
						t.Fatal("cached extra damage consumed formula RNG")
					}
				} else {
					recipient.base["pdef"], recipient.base["mdef"] = 0, 0
					recipient.base["damageReduce"], recipient.base["pdamageReduce"], recipient.base["mdamageReduce"] = 0, 0, 0
					recipient.base["dodge"] = 0
					hit.shareFactor, hit.shareActors = .5, []*combatRoundActor{recipient}
					rng := NewCombatRandom(0)
					damage = 100 * .5 * (1 + rng.Value()*.2 - .1)
					if err := e.originalSharedDamage(skill, hit, typ, 1, 100, 0); err != nil {
						t.Fatal(err)
					}
					if e.rng.Draws() != 1 {
						t.Fatal("shared recipient formula draw count")
					}
				}
				absorbed, other := 12.0, "mshield"
				if typ == 2 {
					absorbed, other = 18, "pshield"
				}
				closeCombat(t, recipient.hp, 80000-damage+absorbed)
				closeCombat(t, recipient.shield("shield"), 0)
				otherAmount := 11.0
				if typ == 2 {
					otherAmount = 5
				}
				closeCombat(t, recipient.shield(other), otherAmount)
				// Conversion sees only actual HP loss after the matching + shared shields.
				closeCombat(t, recipient.hpLossAccumulator, -math.Max(1, damage-absorbed))
				found := false
				for _, event := range e.events {
					if event.Kind == mechanism+"_damage" {
						found = true
						closeCombat(t, event.Value, -damage+absorbed)
					}
				}
				if !found {
					t.Fatal("visual kind did not retain its mechanism event")
				}
			})
		}
	}
}
