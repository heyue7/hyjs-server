package service

import (
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	"sort"
	"time"
)

func adminActivityRepeatPolicy(form model.AdminActivityForm) string {
	if form.RepeatPolicy != "" {
		return form.RepeatPolicy
	}
	if form.Mode == "opening-14" {
		return "once"
	}
	return "repeat"
}
func adminActivityOccurrenceRepeatPolicy(row db.AdminActivityOccurrence) (string, error) {
	var form model.AdminActivityForm
	if json.Unmarshal(row.FormJSON, &form) != nil {
		return "", constUtil.ErrAdminConfiguration
	}
	policy := adminActivityRepeatPolicy(form)
	if policy != "once" && policy != "repeat" {
		return "", constUtil.ErrAdminConfiguration
	}
	return policy, nil
}

// The primary key serializes even metadata entrances racing to accept two
// once-only occurrences. Repeated periods preserve the first owner for any
// later once-only snapshot; changing a template never rewrites issued policy.
func adminAcceptActivityTemplate(tx *gorm.DB, playerID int64, row db.AdminActivityOccurrence) (bool, error) {
	policy, err := adminActivityOccurrenceRepeatPolicy(row)
	if err != nil {
		return false, err
	}
	if playerID < 1 {
		return false, constUtil.ErrAdminInput
	}
	if !adminActivitySchemaHasTable(tx, &db.AdminActivityTemplateOwner{}) {
		if policy == "repeat" {
			return true, nil
		}
		return false, constUtil.ErrAdminConfiguration
	}
	var first db.AdminActivityTemplateOwner
	lookup := tx.Where("server_id=? AND player_id=? AND template_id=?", row.ServerID, playerID, row.TemplateID).Take(&first).Error
	if lookup != nil && !errors.Is(lookup, gorm.ErrRecordNotFound) {
		return false, lookup
	}
	if errors.Is(lookup, gorm.ErrRecordNotFound) {
		original, err := adminRecoverActivityFirstOwner(tx, playerID, row)
		if err != nil {
			return false, err
		}
		first = db.AdminActivityTemplateOwner{ServerID: row.ServerID, PlayerID: playerID, TemplateID: row.TemplateID, OccurrenceID: original, CreatedAt: time.Now().UnixMilli()}
		if err = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&first).Error; err != nil {
			return false, err
		}
	}
	if err = tx.Clauses(clause.Locking{Strength: "SHARE"}).First(&first, "server_id=? AND player_id=? AND template_id=?", row.ServerID, playerID, row.TemplateID).Error; err != nil {
		return false, err
	}
	return policy == "repeat" || first.OccurrenceID == row.ID, nil
}

// Recover only persisted participation, including stopped/expired sources. A
// current template's form or mere visibility is never historical evidence.
func adminRecoverActivityFirstOwner(tx *gorm.DB, playerID int64, current db.AdminActivityOccurrence) (string, error) {
	var sources []db.AdminActivityOccurrence
	if err := tx.Where("server_id=? AND template_id=?", current.ServerID, current.TemplateID).Order("id").Find(&sources).Error; err != nil {
		return "", err
	}
	type evidence struct {
		ID      string
		StartAt int64
	}
	candidates := []evidence{}
	for _, source := range sources {
		if adminActivitySchemaHasTable(tx, &db.AdminCharacterActivityInstance{}) {
			var issued db.AdminCharacterActivityInstance
			err := tx.Where("player_id=? AND server_id=? AND occurrence_id=?", playerID, source.ServerID, source.ID).Take(&issued).Error
			if err == nil {
				var frozen model.AdminActivityForm
				if issued.Mode != source.Mode || json.Unmarshal(issued.FormJSON, &frozen) != nil || frozen.Mode != source.Mode || issued.StartAt <= 0 {
					return "", constUtil.ErrAdminConfiguration
				}
				candidates = append(candidates, evidence{source.ID, issued.StartAt})
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return "", err
			}
		}
		if adminActivityTiming(source) == "calendar" && source.Mode != "opening-14" {
			proved, err := adminActivityNativeParticipation(tx, playerID, source)
			if err != nil {
				return "", err
			}
			if proved {
				candidates = append(candidates, evidence{source.ID, source.StartAt})
			}
		}
	}
	if len(candidates) == 0 {
		return current.ID, nil
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].StartAt == candidates[j].StartAt {
			return candidates[i].ID < candidates[j].ID
		}
		return candidates[i].StartAt < candidates[j].StartAt
	})
	return candidates[0].ID, nil
}

func adminActivityNativeParticipation(tx *gorm.DB, playerID int64, source db.AdminActivityOccurrence) (bool, error) {
	definition, ok := adminActivityDefinitionFor(source.Mode)
	if !ok {
		return false, nil
	}
	type proof struct {
		table any
		where string
		args  []any
	}
	proofs := []proof{}
	key := source.ConsumerKey
	switch definition.domain {
	case "commerce":
		proofs = append(proofs, proof{&db.P4CommerceState{}, "player_id=? AND rule_id=?", []any{playerID, key}}, proof{&db.P4Receipt{}, "player_id=? AND domain=?", []any{playerID, key}})
	case "welfare":
		// Welfare receipts have a kind but no immutable instance ID. Only the
		// instance-keyed frozen state proves which historical period was accepted.
		proofs = append(proofs, proof{&db.P4WelfareState{}, "player_id=? AND rule_id=?", []any{playerID, key}})
	case "period":
		proofs = append(proofs, proof{&db.PeriodProgress{}, "player_id=? AND instance_id=?", []any{playerID, key}}, proof{&db.RewardDelivery{}, "player_id=? AND source_key=?", []any{playerID, "activity:" + key}})
	case "pool":
		proofs = append(proofs, proof{&db.TurntablePersonal{}, "player_id=? AND pool_id=?", []any{playerID, key}}, proof{&db.TurntableState{}, "player_id=? AND pool_id=?", []any{playerID, key}}, proof{&db.TurntableDrawReceipt{}, "player_id=? AND pool_id=?", []any{playerID, key}})
	}
	for _, p := range proofs {
		if !tx.Migrator().HasTable(p.table) {
			continue
		}
		var n int64
		if err := tx.Model(p.table).Where(p.where, p.args...).Limit(1).Count(&n).Error; err != nil {
			return false, err
		}
		if n > 0 {
			return true, nil
		}
	}
	return false, nil
}
