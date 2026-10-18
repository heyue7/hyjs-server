package service

import "strings"

// Original Dispel(bool,0) collects every clearable debuff before unloading it.
// Dispel(string) iterates the mutating collection and increments after removal;
// adjacent equal identities are skipped unless Param repeats that exact ID.
func (e *combatRoundEngine) originalDispelEffect(target *combatRoundActor, row map[string]string) {
	if p6OriginalFactoryNoEffect(row) {
		return
	}
	if row["Factor"] != "clearSomeBuffs" {
		var selected []*combatRoundBuff
		for _, b := range target.buffs {
			if b.row["CanBeCleared"] == "1" && (b.row["IsDebuff"] == "1") == strings.Contains(row["Factor"], "Debuff") {
				selected = append(selected, b)
			}
		}
		if strings.HasPrefix(row["Factor"], "clearOne") {
			// Native Dispel(bool,1) consumes Range(0,count), even with one
			// eligible identity. An empty candidate collection consumes no RNG.
			if len(selected) > 0 {
				e.unloadBuff(target, selected[int(e.rng.Value()*float64(len(selected)))])
			}
			return
		}
		for _, b := range selected {
			e.unloadBuff(target, b)
		}
		return
	}
	for _, id := range strings.Split(row["Param"], ",") {
		if id == "" {
			continue
		}
		for i := 0; i < len(target.buffs); i++ {
			b := target.buffs[i]
			if b.row["CanBeCleared"] == "1" && b.row["Id"] == id {
				e.unloadBuff(target, b)
			}
		}
	}
}
