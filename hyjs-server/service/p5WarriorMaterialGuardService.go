package service

import (
	"gorm.io/gorm"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// p5WarriorMaterialHolds reads the existing expedition ownership inside the
// profile transaction. A held card stays protected until its real mission
// releases the row, including the finished-but-unclaimed state.
func p5WarriorMaterialHolds(tx *gorm.DB, id int64) (map[string]bool, error) {
	held := map[string]bool{}
	if !gameSchemaHasTable(tx, &db.P7ExpeditionHero{}) {
		return held, nil
	}
	var rows []db.P7ExpeditionHero
	if err := tx.Where("player_id = ?", id).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		held[row.GUID] = true
	}
	return held, nil
}

// p5WarriorMaterialAvailable applies the same ownership and investment rules
// used by the final save before automatic donor selection. Skipping a protected
// early GUID allows other genuinely idle cards to satisfy the original cost.
func p5WarriorMaterialAvailable(p *pb.Player, g *model.GameState, w *pb.Warrior, held map[string]bool) bool {
	if p == nil || g == nil || w == nil || w.GetGuid() == "" || held[w.GetGuid()] || !talentMaterialUnused(w) || p5WarriorProgressed(w) {
		return false
	}
	if err := P5GuardInstance(p, g, w.GetGuid()); err != nil {
		return false
	}
	// Match the owned instance rather than trusting a caller-created descriptor.
	current, err := p5Warrior(g, w.GetGuid())
	return err == nil && current == w
}

// guardP5TalentMaterials rejects an explicit protected selection as a whole.
// It never silently substitutes a different card than the original request.
func guardP5TalentMaterials(tx *gorm.DB, id int64, p *pb.Player, g *model.GameState, b *pb.CSWarriorTalent) error {
	held, err := p5WarriorMaterialHolds(tx, id)
	if err != nil {
		return err
	}
	for _, cost := range b.GetCost() {
		if cost.GetGuid() == "" {
			continue
		}
		w, err := p5Warrior(g, cost.GetGuid())
		if err != nil || !p5WarriorMaterialAvailable(p, g, w, held) {
			return p5Error()
		}
	}
	return nil
}
