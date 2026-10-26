package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

type p7RushVerifiedRead struct {
	pool    gorm.ConnPool
	outcome []byte
}

type p7RushReadRequestCache struct {
	sync.Mutex
	rows         map[string]p7RushVerifiedRead
	hits, misses int
}

func p7WithRushReadRequestCache(ctx context.Context) context.Context {
	return context.WithValue(ctx, constUtil.P7RushReadRequestCacheKey{}, &p7RushReadRequestCache{rows: map[string]p7RushVerifiedRead{}})
}

// Hash every exported stored field, including blobs deliberately excluded from JSON DTOs.
// A changed blob with an unchanged declared digest must miss and run the original verifier.
func p7RushStoredRowFingerprint(row any) (string, error) {
	value := reflect.Indirect(reflect.ValueOf(row))
	fields := map[string]any{"__type": value.Type().String()}
	for i := 0; i < value.NumField(); i++ {
		field := value.Field(i)
		if !field.CanInterface() {
			continue
		}
		data := field.Interface()
		if raw, ok := data.([]byte); ok {
			data = struct {
				Length int
				Nil    bool
				SHA256 string
			}{len(raw), raw == nil, p6Digest(raw)}
		}
		fields[value.Type().Field(i).Name] = data
	}
	raw, err := json.Marshal(fields)
	if err != nil {
		return "", err
	}
	return p6Digest(raw), nil
}

// Query original source rows again on every lookup. Only their expensive decoding,
// canonical admission and accepted-End verification may be reused after all bound rows match.
func p7RushSettledReadFingerprint(tx *gorm.DB, fight *db.P7RushFight, request *pb.CSRushFightEnd) (*p7RushReadRequestCache, string) {
	return p7RushReadFingerprint(tx, fight, request)
}

func p7RushReadFingerprint(tx *gorm.DB, fight *db.P7RushFight, request *pb.CSRushFightEnd) (*p7RushReadRequestCache, string) {
	if tx == nil || tx.Statement == nil || fight == nil || fight.Diagnostic || fight.Status != constUtil.P7RushSettled {
		return nil, ""
	}
	cache, _ := tx.Statement.Context.Value(constUtil.P7RushReadRequestCacheKey{}).(*p7RushReadRequestCache)
	if cache == nil {
		return nil, ""
	}
	pool := tx.Statement.ConnPool
	if _, ok := pool.(gorm.TxCommitter); !ok || !reflect.TypeOf(pool).Comparable() {
		return nil, ""
	}
	parts := []string{fmt.Sprintf("%T:%p", pool, pool)}
	add := func(row any) bool {
		hash, err := p7RushStoredRowFingerprint(row)
		if err != nil {
			return false
		}
		parts = append(parts, hash)
		return true
	}
	if !add(fight) {
		return nil, ""
	}
	if request == nil {
		parts = append(parts, "no-end-request")
	} else {
		raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(request)
		if err != nil {
			return nil, ""
		}
		parts = append(parts, p6Digest(raw))
	}
	var manual db.P7RushManualCalculation
	err := tx.First(&manual, "fight_id=?", fight.ID).Error
	recordID := ""
	if err == nil {
		if !add(&manual) {
			return nil, ""
		}
		recordID = manual.P6RecordID
	} else if errors.Is(err, gorm.ErrRecordNotFound) {
		var scoped db.P7RushScopedCalculation
		if tx.First(&scoped, "fight_id=?", fight.ID).Error != nil || !add(&scoped) {
			return nil, ""
		}
		recordID = scoped.P6RecordID
	} else {
		return nil, ""
	}
	var record db.P6BattleRecord
	if tx.First(&record, "id=?", recordID).Error != nil || !add(&record) {
		return nil, ""
	}
	// Same-request memoization still rereads actual rows; no result survives a request.
	var current db.P7RushFight
	if tx.First(&current, "id=?", fight.ID).Error != nil {
		return nil, ""
	}
	currentHash, err := p7RushStoredRowFingerprint(&current)
	if err != nil || currentHash != parts[1] {
		return nil, ""
	}
	raw, err := json.Marshal(parts)
	if err != nil {
		return nil, ""
	}
	return cache, p6Digest(raw)
}

func p7ReadScopedRush(tx *gorm.DB, fight *db.P7RushFight, request *pb.CSRushFightEnd) (*model.CombatOutcome, bool, error) {
	if q := p7EliteProjectionFor(tx); q != nil && fight.PlayerID == q.state.PlayerID && fight.Kind == constUtil.P7RushTrial && fight.Status == constUtil.P7RushSettled {
		if err := q.load(); err != nil {
			return nil, true, err
		}
		for i := range q.fights {
			stored := &q.fights[i]
			if stored.ID != fight.ID {
				continue
			}
			var end pb.CSRushFightEnd
			a, ae := p7RushStoredRowFingerprint(stored)
			b, be := p7RushStoredRowFingerprint(fight)
			if ae == nil && be == nil && a == b && proto.Unmarshal(stored.EndRequest, &end) == nil && (request == nil || proto.Equal(request, &end)) {
				return q.read(stored, request)
			}
			break
		}
	}

	cache, key := p7RushReadFingerprint(tx, fight, request)
	if cache != nil {
		cache.Lock()
		entry, found := cache.rows[key]
		if found && entry.pool == tx.Statement.ConnPool {
			cache.hits++
		} else {
			cache.misses++
		}
		cache.Unlock()
		if found && entry.pool == tx.Statement.ConnPool {
			var outcome model.CombatOutcome
			if json.Unmarshal(entry.outcome, &outcome) == nil {
				return &outcome, true, nil
			}
		}
	}
	outcome, scoped, err := p7ReadScopedRushUncached(tx, fight, request)
	if cache != nil && err == nil && scoped && outcome != nil && outcome.Authoritative && tx.Statement.Context.Err() == nil {
		if raw, e := json.Marshal(outcome); e == nil {
			cache.Lock()
			if len(cache.rows) < constUtil.P7RushReadRequestCacheLimit {
				cache.rows[key] = p7RushVerifiedRead{pool: tx.Statement.ConnPool, outcome: raw}
			}
			cache.Unlock()
		}
	}
	return outcome, scoped, err
}

func p7WithRushHistoricalQuery(ctx context.Context) context.Context {
	return context.WithValue(ctx, constUtil.P7RushHistoricalQueryKey{}, true)
}
