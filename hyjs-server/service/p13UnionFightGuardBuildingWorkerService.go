package service

import (
	"context"
	"errors"
	"fmt"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/db"
	"sync"
	"time"
)

// One bounded in-memory scan cursor avoids sixteen damaged matches starving a
// later healthy match. Recovery facts remain durable; restart merely restarts
// the scan, and no receipt/hash or eligibility is rewritten by this scheduler.
var p13UFGuardBuildingScan struct {
	sync.Mutex
	Database *gorm.DB
	After    string
}

func p13UFGuardBuildingDueMatches(tx *gorm.DB, now time.Time) ([]string, error) {
	p13UFGuardBuildingScan.Lock()
	defer p13UFGuardBuildingScan.Unlock()
	database := db.GetIns()
	if p13UFGuardBuildingScan.Database != database {
		p13UFGuardBuildingScan.Database = database
		p13UFGuardBuildingScan.After = ""
	}
	query := func(after string, limit int) ([]string, error) {
		var ids []string
		q := tx.Model(&db.P13UnionFightGuardState{}).Distinct("p13_union_fight_guard_states.match_id").Joins("JOIN p13_union_fight_matches AS m ON m.id=p13_union_fight_guard_states.match_id").Where("recover_at>0 AND recover_at<=? AND m.start_at<=? AND m.end_at>?", now.UnixMilli(), now.UnixMilli(), now.UnixMilli())
		if after != "" {
			q = q.Where("p13_union_fight_guard_states.match_id>?", after)
		}
		err := q.Order("p13_union_fight_guard_states.match_id").Limit(limit).Pluck("p13_union_fight_guard_states.match_id", &ids).Error
		return ids, err
	}
	ids, err := query(p13UFGuardBuildingScan.After, 16)
	if err != nil {
		return nil, err
	}
	if len(ids) < 16 && p13UFGuardBuildingScan.After != "" {
		wrapped, e := query("", 16-len(ids))
		if e != nil {
			return nil, e
		}
		seen := map[string]bool{}
		for _, id := range ids {
			seen[id] = true
		}
		for _, id := range wrapped {
			if !seen[id] {
				ids = append(ids, id)
				seen[id] = true
			}
		}
	}
	if len(ids) > 0 {
		p13UFGuardBuildingScan.After = ids[len(ids)-1]
	} else {
		p13UFGuardBuildingScan.After = ""
	}
	return ids, nil
}

// TickP13UnionFightGuardBuildings uses independent match transactions. A bad
// receipt cannot roll back another match's recovery; publication follows commit.
func (s *GameService) TickP13UnionFightGuardBuildings(ctx context.Context, now time.Time) error {
	ids, err := p13UFGuardBuildingDueMatches(db.GetIns().WithContext(ctx), now)
	if err != nil {
		return err
	}
	clocks, clockErr := p13UFWorldClockBatch(ctx, ids, now)
	if clockErr != nil {
		return clockErr
	}
	var failures []error
	for _, id := range ids {
		if clocks.paused(id) {
			continue
		}
		if err := ctx.Err(); err != nil {
			return errors.Join(append(failures, err)...)
		}
		var frames []proto.Message
		err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := ServerGameClockGuardTransaction(tx); err != nil {
				return err
			}
			var gate db.P13GuildPolicy
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&gate, 1).Error; err != nil {
				return err
			}
			if closed, err := p13UFWorldSealed(tx, id); err != nil {
				return err
			} else if closed {
				return nil
			}
			var policy db.P13UnionFightPolicy
			var field db.P13UnionFightFieldPolicy
			if err := tx.First(&policy, 1).Error; err != nil {
				return err
			}
			if err := tx.First(&field, 1).Error; err != nil {
				return err
			}
			if !gate.ManagementEnabled || !policy.EnrollmentEnabled || !policy.MatchEnabled || !field.Enabled {
				return nil
			}
			var match db.P13UnionFightMatch
			if err := tx.First(&match, "id=?", id).Error; err != nil {
				return err
			}
			if now.UnixMilli() < match.StartAt || now.UnixMilli() >= clocks.end(&match) {
				return nil
			}
			var err error
			frames, err = s.p13UFGuardBuildingAdvance(tx, &match, now)
			return err
		})
		if err != nil {
			failures = append(failures, fmt.Errorf("guard building recovery %s: %w", id, err))
			continue
		}
		if len(frames) == 0 {
			continue
		}
		// Resolve authenticated peers once for all committed frames, retaining the
		// batched membership/session/scene contract of FieldBroadcast.
		peers, _, err := s.p13UFFieldPeers(db.GetIns().WithContext(ctx), id, 0)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		if err := p13UFPublishBeforeAbort(ctx, id, func() {
			for _, peer := range peers {
				for _, frame := range frames {
					s.publishP10Private(ctx, peer.PlayerID, frame)
				}
			}
		}); err != nil {
			failures = append(failures, err)
		}

	}
	return errors.Join(failures...)
}
