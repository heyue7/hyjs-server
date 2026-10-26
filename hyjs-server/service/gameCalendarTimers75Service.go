package service

import (
	"context"
	"encoding/json"
	"fmt"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	"time"
)

// A published policy or irreversible guild upgrade can belong to a date the
// administrator has already visited. Keep its original source/hash intact and
// bound chronological checks by the authenticated calendar's historical offset.
// This horizon is never used for request stamps, eligibility windows or quotas.
func calendar75SourceHorizon(tx *gorm.DB, now time.Time) time.Time {
	if !calendar75Adjusted(tx) {
		return now
	}
	ctx := tx.Statement.Context
	high, _ := ctx.Value(constUtil.ServerGameClockHighDaysKey).(int64)
	offset, _ := ctx.Value(constUtil.ServerGameClockOffsetKey).(int64)
	high = max(int64(0), high, offset)
	if high > constUtil.ServerGameClockMaxDays {
		return now
	}
	real := time.Now().AddDate(0, 0, int(high))
	if real.After(now) {
		return real
	}
	return now
}

// Seal only presentation leases already expired in the day being left. A
// rewind must not resurrect them; accepted settlement/reward rows stay intact.
func expireCalendarWanted75(tx *gorm.DB, serverID, oldGameMS int64) error {
	if !gameSchemaHasTable(tx, &db.P12WantedFight{}) || !gameSchemaHasTable(tx, &db.P12WantedInstance{}) {
		return nil
	}
	instances := tx.Model(&db.P12WantedInstance{}).Select("id").Where("server_id=? AND created_at<=? AND expires_at<=?", serverID, oldGameMS, oldGameMS)
	owners := tx.Model(&db.Profile{}).Select("id").Where("server_id=? OR server_id=0", serverID)
	return tx.Model(&db.P12WantedFight{}).Where("committed=? AND abandoned_at=0 AND instance_id IN (?) AND player_id IN (?)", false, instances, owners).Update("abandoned_at", oldGameMS).Error
}

// These are mutable duration anchors expressed in game-calendar milliseconds.
// Shifting them preserves remaining cooldowns. Request/financial/battle receipts
// and the wall-clock gathering, PK and shared-boss leases are never touched.
func calendar75ProfileTimers(tx *gorm.DB, rows []db.Profile, shift int64, repair bool, now int64) (int, error) {
	changed := 0
	for _, r := range rows {
		var p, g map[string]json.RawMessage
		if r.Player == nil || r.Game == nil {
			continue
		}
		if e := json.Unmarshal([]byte(*r.Player), &p); e != nil {
			return changed, e
		}
		if e := json.Unmarshal([]byte(*r.Game), &g); e != nil {
			return changed, e
		}
		dirtyP, dirtyG := false, false
		var anchor int64
		if b := p["wantedAttTime"]; len(b) > 0 {
			if e := json.Unmarshal(b, &anchor); e != nil {
				return changed, e
			}
		}
		if anchor > 0 && (!repair || anchor > now) {
			next := anchor + shift
			if repair && next > now {
				// Old anchors may already include a previous positive test offset.
				// Rebase this recovery watermark without granting any attempts.
				next = now
			}
			if next <= 0 {
				return changed, fmt.Errorf("悬赏计时超出有效范围")
			}
			p["wantedAttTime"], _ = json.Marshal(next)
			dirtyP = true
		}
		var recruits map[string]map[string]json.RawMessage
		if b := g["recruit"]; len(b) > 0 {
			if e := json.Unmarshal(b, &recruits); e != nil {
				return changed, e
			}
			for _, st := range recruits {
				var free int64
				if b := st["next_free"]; len(b) > 0 {
					if e := json.Unmarshal(b, &free); e != nil {
						return changed, e
					}
				}
				// Native three recruit pools have at most a one-day free interval.
				// Initial repair distinguishes the old wall-time namespace from a valid next-day cooldown.
				if free > 0 && (!repair || free > now+2*86400000) {
					next := free + shift
					if next <= 0 {
						return changed, fmt.Errorf("招募计时超出有效范围")
					}
					st["next_free"], _ = json.Marshal(next)
					dirtyG = true
				}
			}
			if dirtyG {
				g["recruit"], _ = json.Marshal(recruits)
			}
		}
		values := map[string]any{}
		if dirtyP {
			raw, e := json.Marshal(p)
			if e != nil {
				return changed, e
			}
			values["player"] = string(raw)
		}
		if dirtyG {
			raw, e := json.Marshal(g)
			if e != nil {
				return changed, e
			}
			values["game"] = string(raw)
		}
		if len(values) > 0 {
			if e := tx.Model(&db.Profile{}).Where("id=?", r.Id).Updates(values).Error; e != nil {
				return changed, e
			}
			changed++
		}
	}
	return changed, nil
}

func calendar75MemoryTimers(tx *gorm.DB, serverID int64, shift int64, repair bool, now int64) error {
	if !gameSchemaHasTable(tx, &db.P4CommerceState{}) || !gameSchemaHasTable(tx, &db.P4CommerceRule{}) {
		return nil
	}
	var states []db.P4CommerceState
	if e := tx.Table("p4_commerce_states AS state").Select("state.*").Joins("JOIN p4_commerce_rules AS rule ON rule.id=state.rule_id").Where("rule.kind=? AND state.player_id IN (SELECT id FROM profiles WHERE server_id=? OR server_id=0)", "memory", serverID).Find(&states).Error; e != nil {
		return e
	}
	for _, st := range states {
		var payload map[string]json.RawMessage
		if e := json.Unmarshal(st.Payload, &payload); e != nil {
			return e
		}
		var at int64
		if b := payload["freeAt"]; len(b) > 0 {
			if e := json.Unmarshal(b, &at); e != nil {
				return e
			}
		}
		if at <= 0 || repair && at <= now {
			continue
		}
		next := at + shift
		if next <= 0 {
			return fmt.Errorf("圣坛计时超出有效范围")
		}
		payload["freeAt"], _ = json.Marshal(next)
		raw, e := json.Marshal(payload)
		if e != nil {
			return e
		}
		if e = tx.Model(&db.P4CommerceState{}).Where("player_id=? AND rule_id=?", st.PlayerID, st.RuleID).Update("payload", raw).Error; e != nil {
			return e
		}
	}
	return nil
}

// Protection is a mutable game-calendar duration. Translate its three anchors
// together, including expired states, so neither direction revives or skips it.
// Paid receipts and their original admission times retain their exact bytes.
func calendar75PlunderTimers(tx *gorm.DB, serverID, shift int64) error {
	if !gameSchemaHasTable(tx, &db.P11PlunderState{}) {
		return nil
	}
	q := tx.Model(&db.P11PlunderState{}).Where("player_id IN (SELECT id FROM profiles WHERE server_id=? OR server_id=0)", serverID)
	if shift < 0 {
		var invalid int64
		if err := q.Session(&gorm.Session{}).Where("(avoid_until>0 AND avoid_until<=?) OR (last_use_at>0 AND last_use_at<=?) OR (updated_at>0 AND updated_at<=?)", -shift, -shift, -shift).Count(&invalid).Error; err != nil {
			return err
		}
		if invalid > 0 {
			return constUtil.ErrAdminConfiguration
		}
	}
	return q.Where("avoid_until>0 OR last_use_at>0 OR updated_at>0").Updates(map[string]any{
		"avoid_until": gorm.Expr("CASE WHEN avoid_until>0 THEN avoid_until + ? ELSE avoid_until END", shift),
		"last_use_at": gorm.Expr("CASE WHEN last_use_at>0 THEN last_use_at + ? ELSE last_use_at END", shift),
		"updated_at":  gorm.Expr("CASE WHEN updated_at>0 THEN updated_at + ? ELSE updated_at END", shift),
	}).Error
}

func (s *GameService) shiftCalendarDurationTimers75(tx *gorm.DB, rows []db.Profile, delta int64) error {
	if delta == 0 {
		return nil
	}
	shift := delta * 86400000
	if _, e := calendar75ProfileTimers(tx, rows, shift, false, 0); e != nil {
		return e
	}
	if e := calendar75MemoryTimers(tx, s.OriginalServerID(), shift, false, 0); e != nil {
		return e
	}
	if e := calendar75PlunderTimers(tx, s.OriginalServerID(), shift); e != nil {
		return e
	}
	if gameSchemaHasTable(tx, &db.TurntableState{}) {
		q := tx.Model(&db.TurntableState{}).Where("next_free>0 AND player_id IN (SELECT id FROM profiles WHERE server_id=? OR server_id=0)", s.OriginalServerID())
		if shift < 0 {
			var invalid int64
			if e := q.Session(&gorm.Session{}).Where("next_free<=?", -shift).Count(&invalid).Error; e != nil {
				return e
			}
			if invalid > 0 {
				return constUtil.ErrAdminConfiguration
			}
		}
		if e := q.UpdateColumn("next_free", gorm.Expr("next_free + ?", shift)).Error; e != nil {
			return e
		}
	}
	return nil
}

// RepairGameCalendar75 upgrades only stale mutable anchors left by the original
// test rewind. It is explicit, audited and idempotent; it grants no resources.
func (s *GameService) RepairGameCalendar75(ctx context.Context, a *model.AdminPrincipal) error {
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var done int64
		if e := tx.Model(&db.AdminAudit{}).Where("action=? AND target=?", "calendar75.anchors", fmt.Sprint(s.OriginalServerID())).Count(&done).Error; e != nil {
			return e
		}
		if done > 0 {
			return nil
		}
		var clock db.ServerGameClock
		if e := tx.First(&clock, "server_id=?", s.OriginalServerID()).Error; e != nil {
			return e
		}
		if clock.OffsetDays < 0 {
			var rows []db.Profile
			if e := tx.Where("player IS NOT NULL AND game IS NOT NULL AND (server_id=? OR server_id=0)", s.OriginalServerID()).Order("id").Find(&rows).Error; e != nil {
				return e
			}
			shift := clock.OffsetDays * 86400000
			now := time.Now().UnixMilli() + shift
			if _, e := calendar75ProfileTimers(tx, rows, shift, true, now); e != nil {
				return e
			}
			if e := calendar75MemoryTimers(tx, s.OriginalServerID(), shift, true, now); e != nil {
				return e
			}
		}
		return adminAudit(tx, a, "calendar75.anchors", fmt.Sprint(s.OriginalServerID()), "修复测试回调遗留的可变冷却锚点；保留资源、限购和全部回执")
	})
}
