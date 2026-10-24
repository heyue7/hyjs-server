package service

import (
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"sync"
	"time"

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
	return p7RushReadFingerprint(tx, fight, request, nil)
}

func p7RushReadFingerprint(tx *gorm.DB, fight *db.P7RushFight, request *pb.CSRushFightEnd, historical *p7RushHistoricalKey) (*p7RushReadRequestCache, string) {
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
	// A process-wide hit requires current transaction rows, including the fight.
	// Preserve rejection of stale/forged arguments instead of silently substituting them.
	if historical != nil && tx.Statement.Context.Value(constUtil.P7RushHistoricalQueryKey{}) == true &&
		manual.Status == constUtil.P7RushManualAccepted && manual.AcceptedAt > 0 &&
		record.Status == constUtil.P6BattleComputed && record.Authoritative {
		var current db.P7RushFight
		currentErr := tx.First(&current, "id=?", fight.ID).Error
		currentHash, hashErr := p7RushStoredRowFingerprint(&current)
		var stored pb.CSRushFightEnd
		storedErr := proto.Unmarshal(fight.EndRequest, &stored)
		if currentErr != nil || hashErr != nil || currentHash != parts[1] {
			return nil, ""
		}
		if currentErr == nil && hashErr == nil && currentHash == parts[1] && len(fight.EndRequest) > 0 &&
			storedErr == nil && (request == nil || proto.Equal(request, &stored)) {
			if pool, poolErr := tx.DB(); poolErr == nil {
				// Exclude only SQL transaction identity; retain every row and request digest.
				if body, e := json.Marshal(parts[1:]); e == nil {
					*historical = p7RushHistoricalKey{database: pool, fingerprint: p6Digest(body)}
				}
			}
		}
	}
	raw, err := json.Marshal(parts)
	if err != nil {
		return nil, ""
	}
	return cache, p6Digest(raw)
}

func p7ReadScopedRush(tx *gorm.DB, fight *db.P7RushFight, request *pb.CSRushFightEnd) (*model.CombatOutcome, bool, error) {
	var historical p7RushHistoricalKey
	cache, key := p7RushReadFingerprint(tx, fight, request, &historical)
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
	if historical.database != nil && tx.Statement.Context.Err() == nil {
		if raw := p7RushHistoryCache.get(historical, time.Now()); raw != nil {
			if outcome, e := p7DecodeHistoricalOutcome(raw); e == nil {
				// Keep the existing same-request memo after a cross-request hit.
				if cache != nil {
					if body, marshalErr := json.Marshal(outcome); marshalErr == nil {
						cache.Lock()
						if len(cache.rows) < constUtil.P7RushReadRequestCacheLimit {
							cache.rows[key] = p7RushVerifiedRead{pool: tx.Statement.ConnPool, outcome: body}
						}
						cache.Unlock()
					}
				}
				return outcome, true, nil
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
	if historical.database != nil && err == nil && scoped && outcome != nil && outcome.Authoritative && tx.Statement.Context.Err() == nil {
		if raw, e := p7EncodeHistoricalOutcome(outcome); e == nil {
			p7RushHistoryCache.put(historical, raw, time.Now())
		}
	}
	return outcome, scoped, err
}

func p7WithRushHistoricalQuery(ctx context.Context) context.Context {
	return context.WithValue(ctx, constUtil.P7RushHistoricalQueryKey{}, true)
}

type p7RushHistoricalKey struct {
	database    *sql.DB
	fingerprint string
}
type p7RushHistoricalEntry struct {
	raw      []byte
	expires  time.Time
	sequence uint64
}
type p7RushHistoricalCache struct {
	sync.Mutex
	rows     map[p7RushHistoricalKey]p7RushHistoricalEntry
	bytes    int
	sequence uint64
}

var p7RushHistoryCache = &p7RushHistoricalCache{rows: make(map[p7RushHistoricalKey]p7RushHistoricalEntry)}

func (c *p7RushHistoricalCache) remove(key p7RushHistoricalKey) {
	c.bytes -= len(c.rows[key].raw)
	delete(c.rows, key)
}
func (c *p7RushHistoricalCache) expire(now time.Time) {
	for key, entry := range c.rows {
		if !now.Before(entry.expires) {
			c.remove(key)
		}
	}
}

// Published byte slices are immutable; decoding and hashing happen without the mutex.
func (c *p7RushHistoricalCache) get(key p7RushHistoricalKey, now time.Time) []byte {
	c.Lock()
	defer c.Unlock()
	c.expire(now)
	entry, ok := c.rows[key]
	if !ok {
		return nil
	}
	c.sequence++
	entry.sequence = c.sequence
	c.rows[key] = entry
	return entry.raw
}
func (c *p7RushHistoricalCache) put(key p7RushHistoricalKey, raw []byte, now time.Time) {
	if len(raw) > constUtil.P7RushHistoricalReadCacheEntryBytes || len(raw) == 0 {
		return
	}
	c.Lock()
	defer c.Unlock()
	c.expire(now)
	if _, ok := c.rows[key]; ok {
		c.remove(key)
	}
	for len(c.rows) >= constUtil.P7RushHistoricalReadCacheLimit || c.bytes+len(raw) > constUtil.P7RushHistoricalReadCacheTotalBytes {
		var oldest p7RushHistoricalKey
		sequence := ^uint64(0)
		for candidate, entry := range c.rows {
			if entry.sequence < sequence {
				oldest, sequence = candidate, entry.sequence
			}
		}
		c.remove(oldest)
	}
	c.sequence++
	c.rows[key] = p7RushHistoricalEntry{raw: raw, expires: now.Add(time.Duration(constUtil.P7RushHistoricalReadCacheTTLSeconds) * time.Second), sequence: c.sequence}
	c.bytes += len(raw)
}

func p7EncodeHistoricalOutcome(outcome *model.CombatOutcome) ([]byte, error) {
	raw, err := json.Marshal(outcome)
	if err != nil {
		return nil, err
	}
	if len(raw) > constUtil.P7RushHistoricalReadCacheDecodedBytes {
		return nil, errors.New("historical outcome exceeds decoded cache bound")
	}
	var body bytes.Buffer
	writer := gzip.NewWriter(&body)
	if _, err = writer.Write(raw); err != nil {
		return nil, err
	}
	if err = writer.Close(); err != nil {
		return nil, err
	}
	if body.Len() > constUtil.P7RushHistoricalReadCacheEntryBytes {
		return nil, errors.New("historical outcome exceeds compressed cache bound")
	}
	return body.Bytes(), nil
}
func p7DecodeHistoricalOutcome(raw []byte) (*model.CombatOutcome, error) {
	if len(raw) > constUtil.P7RushHistoricalReadCacheEntryBytes {
		return nil, errors.New("historical outcome exceeds compressed cache bound")
	}
	reader, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	body, err := io.ReadAll(io.LimitReader(reader, int64(constUtil.P7RushHistoricalReadCacheDecodedBytes)+1))
	if err != nil {
		return nil, err
	}
	if len(body) > constUtil.P7RushHistoricalReadCacheDecodedBytes {
		return nil, errors.New("historical outcome exceeds decoded cache bound")
	}
	var outcome model.CombatOutcome
	if err = json.Unmarshal(body, &outcome); err != nil {
		return nil, err
	}
	if !outcome.Authoritative {
		return nil, errors.New("historical outcome is not authoritative")
	}
	return &outcome, nil
}
