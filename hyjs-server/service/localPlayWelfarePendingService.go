package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
)

type localPlayRetiredWelfare struct {
	RuleID    string  `json:"ruleId"`
	Kind      string  `json:"kind"`
	LoginDays int64   `json:"loginDays"`
	LoginAt   []int64 `json:"loginAt,omitempty"`
}

func localPlayWelfareRetiredKey(ruleID string) string {
	digest := sha256.Sum256([]byte(ruleID))
	return "welfare-retired:" + hex.EncodeToString(digest[:16])
}

func localPlayWelfareRetired(tx *gorm.DB, ruleID string) (*localPlayRetiredWelfare, error) {
	owner := localPlayActivityOwner(tx)
	if owner <= 1 {
		return nil, nil
	}
	var row db.LocalPlayActivityOperation
	err := tx.First(&row, "player_id=? AND request_id=?", owner, localPlayWelfareRetiredKey(ruleID)).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var saved localPlayRetiredWelfare
	if row.Action != "welfare-retired" || row.ActivityID != ruleID || json.Unmarshal(row.Response, &saved) != nil || saved.RuleID != ruleID || saved.LoginDays < 0 || saved.LoginDays != int64(len(saved.LoginAt)) || (saved.Kind != constUtil.P4WelfareLogin && saved.Kind != constUtil.P4WelfareCat) {
		return nil, errLocalPlayActivitiesInvalid
	}
	return &saved, nil
}

func localPlayWelfareRetiredDays(saved *localPlayRetiredWelfare, now time.Time) int64 {
	var days int64
	for _, at := range saved.LoginAt {
		if at <= now.UnixMilli() {
			days++
		}
	}
	return days
}

func localPlayWelfarePendingKeys(tx *gorm.DB, id int64, kind, current string) ([]string, error) {
	keys := []string{}
	var rows []db.LocalPlayActivityOperation
	if err := tx.Where("player_id=? AND action=?", id, "welfare-retired").Order("created_at,request_id").Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		var saved localPlayRetiredWelfare
		if json.Unmarshal(row.Response, &saved) != nil || saved.RuleID != row.ActivityID || saved.LoginDays < 0 {
			return nil, errLocalPlayActivitiesInvalid
		}
		if saved.Kind == kind {
			keys = append(keys, saved.RuleID)
		}
	}
	return append(keys, current), nil
}

func localPlayWelfarePending(saved *localPlayRetiredWelfare, rules p4WelfareRules, state *p4WelfareProgress) bool {
	if saved.Kind == constUtil.P4WelfareCat {
		return state.CatTotal >= rules.TotalLimit && !state.CatFinal
	}
	for i := range rules.Days {
		if int64(i) < saved.LoginDays && !state.Claimed[i] {
			return true
		}
	}
	return false
}

// Freeze only already earned rights before replacing a binding. Old rules and
// original receipts remain immutable; no new login, exploration or award occurs.
func (s *GameService) localPlayRetireWelfare(tx *gorm.DB, id int64, binding db.LocalPlayActivityBinding, next *db.LocalPlayActivityCycle, now time.Time) error {
	if next.ActivityID != "LoginAward" && next.ActivityID != "fortuneCat" {
		return nil
	}
	kind := constUtil.P4WelfareLogin
	if next.ActivityID == "fortuneCat" {
		kind = constUtil.P4WelfareCat
	}
	var originals []db.P4WelfareRule
	var previous *db.LocalPlayActivityCycle
	if binding.CycleID != "" {
		var cycle db.LocalPlayActivityCycle
		if err := tx.First(&cycle, "id=?", binding.CycleID).Error; err != nil {
			return err
		}
		if cycle.PlayerID != id || cycle.Source != constUtil.LocalPlayActivitiesVersion {
			return errLocalPlayActivitiesInvalid
		}
		previous = &cycle
		var snapshot localPlayScopedSnapshot
		if json.Unmarshal(cycle.Snapshot, &snapshot) != nil {
			return errLocalPlayActivitiesInvalid
		}
		originals = snapshot.Welfare
	} else {
		// The first explicit activation preserves earned legacy calendar/cat rights
		// for this owner too; other players and their original rows are untouched.
		if err := tx.Where("kind=? AND enabled=? AND id NOT LIKE ?", kind, true, "%lp-%").Order("start_at,id").Find(&originals).Error; err != nil {
			return err
		}
	}
	for _, cfg := range originals {
		if cfg.Kind != kind || now.UnixMilli() >= cfg.ClaimEndAt {
			continue
		}
		if prior, e := localPlayWelfareRetired(tx, cfg.ID); e != nil {
			return e
		} else if prior != nil {
			continue
		}
		if len(cfg.Frozen) > 0 {
			if err := json.Unmarshal(cfg.Frozen, &cfg); err != nil {
				return err
			}
		}
		rules, err := s.validateP4Welfare(cfg)
		if err != nil {
			return err
		}
		saved := localPlayRetiredWelfare{RuleID: cfg.ID, Kind: kind}
		state := p4WelfareProgress{Claimed: map[int]bool{}}
		var row db.P4WelfareState
		err = tx.First(&row, "player_id=? AND rule_id=?", id, cfg.ID).Error
		if err == nil {
			if json.Unmarshal(row.Payload, &state) != nil {
				return errLocalPlayActivitiesInvalid
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if kind == constUtil.P4WelfareLogin {
			query := tx.Model(&db.EventFact{}).Where("player_id=? AND kind=? AND occurred_at>=? AND occurred_at<? AND id<=?", id, constUtil.P1EventLoginDay, cfg.StartAt, cfg.EndAt, next.EventAfterID)
			if previous != nil {
				query = query.Where("id>?", previous.EventAfterID)
			}
			var days []struct{ At int64 }
			if err := query.Select("MIN(occurred_at) AS at").Group("day").Scan(&days).Error; err != nil {
				return err
			}
			for _, day := range days {
				saved.LoginAt = append(saved.LoginAt, day.At)
			}
			saved.LoginDays = int64(len(saved.LoginAt))
		}
		if !localPlayWelfarePending(&saved, rules, &state) {
			continue
		}
		raw, err := json.Marshal(saved)
		if err != nil {
			return err
		}
		operation := db.LocalPlayActivityOperation{PlayerID: id, RequestID: localPlayWelfareRetiredKey(cfg.ID), Action: "welfare-retired", ActivityID: cfg.ID, Response: raw, CreatedAt: time.Now().UnixMilli()}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&operation).Error; err != nil {
			return err
		}
	}
	return nil
}
