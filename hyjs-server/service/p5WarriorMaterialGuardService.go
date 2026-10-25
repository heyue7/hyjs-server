package service

import (
	"errors"
	"fmt"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
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
	if err := tx.Select("guid").Where("player_id = ?", id).Find(&rows).Error; err != nil {
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
	return p5WarriorMaterialReason(p, g, w, held) == ""
}

// The read projection and final consuming transaction share these exact rules.
func p5WarriorMaterialReason(p *pb.Player, g *model.GameState, w *pb.Warrior, held map[string]bool) string {
	if p == nil || g == nil || w == nil || w.GetGuid() == "" {
		return constUtil.TalentMaterialMissing
	}
	guid := w.GetGuid()
	if guid == p.GetHeroWarId() {
		return constUtil.TalentMaterialMain
	}
	if held[guid] {
		return constUtil.TalentMaterialExpedition
	}
	for _, slot := range g.Formation {
		for _, thing := range slot.GetThings() {
			if thing.GetGuid() == guid {
				return constUtil.TalentMaterialFormation
			}
		}
	}
	for i := 1; i <= 6; i++ {
		if p5KV(g, fmt.Sprintf("p5.helper:%d", i)) == guid {
			return constUtil.TalentMaterialHelper
		}
	}
	if p5KV(g, "p5.lock:"+guid) == "1" {
		return constUtil.TalentMaterialLocked
	}
	if p5KV(g, "p5.dispatch:"+guid) != "" {
		return constUtil.TalentMaterialDispatch
	}
	if !talentMaterialUnused(w) || p5WarriorProgressed(w) {
		return constUtil.TalentMaterialProgressed
	}
	current, err := p5Warrior(g, guid)
	if err != nil || current != w {
		return constUtil.TalentMaterialMissing
	}
	return ""
}

func instanceTalentMaterialSnapshot(tx *gorm.DB, p *pb.Player, g *model.GameState, out *model.InstanceLockResponse) error {
	held, err := p5WarriorMaterialHolds(tx, p.GetPlayerId())
	if err != nil {
		return err
	}
	out.MaterialUnavailable = map[string]string{}
	for _, w := range g.Warriors {
		if reason := p5WarriorMaterialReason(p, g, w, held); reason != "" {
			out.MaterialUnavailable[w.GetGuid()] = reason
		}
	}
	return nil
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
		if err != nil {
			return errors.New(constUtil.TalentMaterialMissing)
		}
		if reason := p5WarriorMaterialReason(p, g, w, held); reason != "" {
			return errors.New(reason)
		}
	}
	return nil
}
