package service

import (
	"google.golang.org/protobuf/proto"
	"testing"
)

func TestP6OriginalProjectileLaunchFlyDelayAndWholeReplay(t *testing.T) {
	s := p6OriginalTables(t)
	input := p6Input(t, s, p6Actor("hero_052", "bow", "052_1", 1, 40000), p6Actor("hero_047", "target", "047_1", 1, 40000), 12)
	// Original second cast uses the cross around its main target, requiring a neighbour.
	input.Defender.Formation = append(input.Defender.Formation, p6Actor("hero_047", "adjacent-target", "047_1", 2, 40000))
	out, err := s.ComputeFrozenBattle(t.Context(), input, nil)
	if err != nil {
		t.Fatal(err)
	}
	launches, hits := 0, 0
	launchTime := map[string]float64{}
	launchDraws := map[string]int64{}
	for _, event := range out.Events {
		if event.Round != 1 || event.Actor != "0_bow" {
			continue
		}
		if event.Kind == "projectile_launch" {
			launches++
			launchTime[event.Cast] = event.Time
			launchDraws[event.Cast] = event.RandomDraws
			closeCombat(t, event.TargetHP, 40000)
			closeCombat(t, event.Value, .2)
		}
		if event.Kind == "release" {
			hits++
			if event.Time-launchTime[event.Cast] < .199 || event.Time-launchTime[event.Cast] > .301 || event.RandomDraws <= launchDraws[event.Cast] {
				t.Fatal("projectile impact did not preserve FlyTime/RNG", event)
			}
		}
	}
	if launches != 2 || hits != 2 || launchDraws["1004"] != launchDraws["1005"] || out.Authoritative {
		t.Fatal("original two-projectile cast incomplete", launches, hits)
	}
	replayed, err := s.ComputeFrozenBattle(t.Context(), input, out.Operates)
	if err != nil || !proto.Equal(out.Operates, replayed.Operates) || !proto.Equal(out.Defender, replayed.Defender) || out.RandomDraws != replayed.RandomDraws {
		t.Fatal("projectile replay changed", err)
	}
}

func TestP6OriginalProjectileZeroFlyAndDelayedCastOrder(t *testing.T) {
	s := p6OriginalTables(t)
	input := p6Input(t, s, p6Actor("hero_029", "poison-caster", ",029_2", 1, 40000), p6Actor("hero_047", "target", "047_1", 1, 40000), 12)
	out, err := s.ComputeFrozenBattle(t.Context(), input, nil)
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	launchTime := map[string]float64{}
	for _, event := range out.Events {
		if event.Round != 1 || event.Actor != "0_poison-caster" {
			continue
		}
		if event.Kind == "projectile_launch" {
			launchTime[event.Cast] = event.Time
		}
		if event.Kind == "release" {
			order = append(order, event.Cast)
			if event.Cast == "689" && event.Time-launchTime[event.Cast] < .399 {
				t.Fatal("original poison projectile ignored .4 FlyTime", event)
			}
			if event.Cast == "690" && event.Time != launchTime[event.Cast] {
				t.Fatal("zero-fly cast did not land at original launch frame", event)
			}
		}
	}
	if len(order) != 4 || order[0] != "690" || order[1] != "689" {
		t.Fatal("actual release chronology was sorted by cast alone", order)
	}
	replayed, err := s.ComputeFrozenBattle(t.Context(), input, out.Operates)
	if err != nil || !proto.Equal(replayed.Operates, out.Operates) {
		t.Fatal("zero-fly chronology replay", err)
	}
}

func TestP6ProjectileRejectsNegativeFlightAndUnknownClass(t *testing.T) {
	for _, mutation := range []struct{ table, key, id, field, value string }{
		{"SkillCast", "SkillCastId", "1004", "FlyTime", "-1"},
		{"Skill", "SkillId", "052_1", "SkillClass", "2"},
	} {
		s := p6OriginalTables(t)
		s.table(mutation.table, mutation.key, mutation.id)[mutation.field] = mutation.value
		input := p6Input(t, s, p6Actor("hero_052", "bow", "052_1", 1, 40000), p6Actor("hero_047", "target", "047_1", 1, 40000), 12)
		if _, err := s.ComputeFrozenBattle(t.Context(), input, nil); err == nil {
			t.Fatal("invalid projectile configuration admitted", mutation.field)
		}
	}
}
