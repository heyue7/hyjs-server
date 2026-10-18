package service

import "math"

// FRelease reuses one random enemy for all hits in a skill until LAST or source weak death.
// Candidates exclude the current hit target and weak/dead, keeping invisible/invincible actors.
func (e *combatRoundEngine) originalSameDamage(skill *combatRoundSkill, hitTarget *combatRoundActor, damage float64, typ int32, exportOrigin ...combatRoundHPExportOrigin) error {
	source := skill.actor
	percent := source.attribute("sameDamForRandom") * .01
	if percent <= 0 || damage <= 0 {
		return nil
	}
	if len(skill.sameDamTargets) == 0 {
		var candidates []*combatRoundActor
		for _, actor := range e.players[hitTarget.camp].actors {
			if actor != hitTarget && actor.state < 3 {
				candidates = append(candidates, actor)
			}
		}
		selected := hitTarget
		if len(candidates) > 0 {
			selected = candidates[int(e.rng.Value()*float64(len(candidates)))]
		}
		skill.sameDamTargets = append(skill.sameDamTargets, selected)
		e.trace("same_damage_target", source, selected, skill.id, "", percent)
	}
	for _, target := range skill.sameDamTargets {
		if target.state == 4 {
			continue
		}
		// Original visual kind8 bypasses repeated release formulas; exportType still spends typed shields.
		killer := (*combatRoundActor)(nil)
		if target == skill.target {
			killer = source
		}
		nominal, visible, shielded := e.damageHPKind(source, target, math.Max(1, damage*percent), typ, "same_damage", killer)
		e.originalExtraHPExport(target, nominal, visible, shielded, 8, exportOrigin)
	}
	return e.failure
}
