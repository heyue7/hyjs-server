package service

import (
	"context"
	"crypto/subtle"
	"errors"
	"math"
	"sort"
	"strconv"
	"strings"

	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func instanceLockOwned(g *model.GameState, guid string) bool {
	for _, w := range g.Warriors {
		if w.GetGuid() == guid {
			return true
		}
	}
	for _, e := range g.Equipment {
		if e.GetGuid() == guid {
			return true
		}
	}
	for _, e := range g.Treasures {
		if e.GetGuid() == guid {
			return true
		}
	}
	return false
}

func instanceLockSnapshot(g *model.GameState) (*model.InstanceLockResponse, error) {
	out := &model.InstanceLockResponse{LockedGUIDs: []string{}}
	value := p5KV(g, constUtil.InstanceLockRevisionKey)
	if value != "" {
		var err error
		out.Revision, err = strconv.ParseInt(value, 10, 64)
		if err != nil || out.Revision < 0 {
			return nil, errors.New(constUtil.InstanceLockConflict)
		}
	}
	owned := map[string]bool{}
	for _, w := range g.Warriors {
		owned[w.GetGuid()] = true
	}
	for _, e := range g.Equipment {
		owned[e.GetGuid()] = true
	}
	for _, e := range g.Treasures {
		owned[e.GetGuid()] = true
	}
	for _, entry := range g.KV {
		if entry.GetValue() == "1" && strings.HasPrefix(entry.GetKey(), constUtil.InstanceLockPrefix) {
			guid := strings.TrimPrefix(entry.GetKey(), constUtil.InstanceLockPrefix)
			if owned[guid] {
				out.LockedGUIDs = append(out.LockedGUIDs, guid)
			}
		}
	}
	sort.Strings(out.LockedGUIDs)
	return out, nil
}

// InstanceLockState reads the same original profile KV consumed by all P5 guards.
func InstanceLockState(ctx context.Context, id int64, session []byte) (*model.InstanceLockResponse, error) {
	row, err := AuthorizeAccount(ctx, id, session)
	if err != nil {
		return nil, err
	}
	_, game, err := decodeProfile(row)
	if err != nil {
		return nil, err
	}
	return instanceLockSnapshot(game)
}

// SetInstanceLock shares the consuming transaction's profile lock, and rechecks live ownership/auth.
func SetInstanceLock(ctx context.Context, id int64, session []byte, guid string, locked bool, revision int64) (*model.InstanceLockResponse, error) {
	var out *model.InstanceLockResponse
	err := updateProfileWithTx(ctx, id, func(tx *gorm.DB, _ *pb.Player, game *model.GameState) error {
		var row db.Profile
		if err := tx.First(&row, id).Error; err != nil {
			return err
		}
		if len(session) != 16 || subtle.ConstantTimeCompare(row.Session, session) != 1 {
			return errors.New(constUtil.NoviceGuideSessionInvalid)
		}
		if row.AccountID > 0 {
			account, err := AccountForProfile(tx, &row)
			if err != nil {
				return err
			}
			if !account.Enabled || row.AccountAuthRevision != account.AuthRevision {
				return errors.New(constUtil.NoviceGuideSessionInvalid)
			}
		}
		if !instanceLockOwned(game, guid) {
			return errors.New(constUtil.InstanceLockInvalid)
		}
		var err error
		out, err = instanceLockSnapshot(game)
		if err != nil {
			return err
		}
		current := p5KV(game, constUtil.InstanceLockPrefix+guid) == "1"
		// Explicit desired state makes retry idempotent without undoing another lock.
		if current == locked {
			return nil
		}
		if out.Revision != revision || out.Revision == math.MaxInt64 {
			return errors.New(constUtil.InstanceLockConflict)
		}
		value := "0"
		if locked {
			value = "1"
		}
		p5SetKV(game, constUtil.InstanceLockPrefix+guid, value)
		p5SetKV(game, constUtil.InstanceLockRevisionKey, strconv.FormatInt(out.Revision+1, 10))
		out, err = instanceLockSnapshot(game)
		return err
	})
	return out, err
}
