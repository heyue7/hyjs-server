package service

import (
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestP7RushHistoricalCacheBoundsAndDatabaseIsolation(t *testing.T) {
	c := &p7RushHistoricalCache{rows: make(map[p7RushHistoricalKey]p7RushHistoricalEntry)}
	a, b := new(sql.DB), new(sql.DB)
	now := time.Unix(100, 0)
	key := p7RushHistoricalKey{a, "same"}
	c.put(key, []byte("ok"), now)
	if c.get(p7RushHistoricalKey{b, "same"}, now) != nil {
		t.Fatal("cross database hit")
	}
	if c.get(key, now) == nil || c.get(key, now.Add(time.Duration(constUtil.P7RushHistoricalReadCacheTTLSeconds)*time.Second)) != nil {
		t.Fatal("TTL boundary")
	}
	for i := 0; i < 40; i++ {
		c.put(p7RushHistoricalKey{a, fmt.Sprint(i)}, []byte("x"), now)
	}
	if len(c.rows) != 32 {
		t.Fatal("entry bound", len(c.rows))
	}
	if c.get(p7RushHistoricalKey{a, "0"}, now) != nil {
		t.Fatal("oldest not evicted")
	}
	large := make([]byte, constUtil.P7RushHistoricalReadCacheEntryBytes)
	for i := 0; i < 20; i++ {
		c.put(p7RushHistoricalKey{a, fmt.Sprint("large", i)}, large, now)
	}
	if c.bytes > constUtil.P7RushHistoricalReadCacheTotalBytes {
		t.Fatal("byte bound", c.bytes)
	}
	c.put(p7RushHistoricalKey{a, "oversized"}, make([]byte, len(large)+1), now)
	if c.get(p7RushHistoricalKey{a, "oversized"}, now) != nil {
		t.Fatal("oversized cached")
	}
}

func TestP7RushHistoricalAcceptedQueryCloneAndTamper(t *testing.T) {
	f, _, startRequest := p7ScopedFixture(t, constUtil.P7RushTrial)
	old := p7RushHistoryCache
	p7RushHistoryCache = &p7RushHistoricalCache{rows: make(map[p7RushHistoricalKey]p7RushHistoricalEntry)}
	t.Cleanup(func() { p7RushHistoryCache = old })
	t.Cleanup(func() { db.GetIns().Model(&db.P7RushManualPolicy{}).Where("id=?", 1).Update("enabled", false) })
	for _, row := range []any{&db.P7RushScopedPolicy{}, &db.P7RushManualPolicy{}} {
		if err := db.GetIns().Model(row).Where("id=?", 1).Update("enabled", true).Error; err != nil {
			t.Fatal(err)
		}
	}
	start, err := f.s.p7Start(f.ctx, f.id, startRequest, 76501, false)
	if err != nil {
		t.Fatal(err)
	}
	var manual db.P7RushManualCalculation
	if err = db.GetIns().First(&manual, "fight_id=?", start.GetFightId()).Error; err != nil {
		t.Fatal(err)
	}
	body, err := decodeP6StoredInput(manual.Series)
	if err != nil {
		t.Fatal(err)
	}
	var series model.FrozenP6RushSeries
	if err = json.Unmarshal(body, &series); err != nil {
		t.Fatal(err)
	}
	input, err := cloneP6CalculationInput(series.Template)
	if err != nil {
		t.Fatal(err)
	}
	input.DiagnosticAIDriverPolicy = constUtil.P6DiagnosticManualAutoStart
	input.DiagnosticManualAutoStartRoundTime = proto.Float64(30.100000000000158)
	calculated, err := f.s.ComputeOriginalP6ProtocolFinalHP(f.ctx, input, nil)
	if err != nil {
		t.Fatal(err)
	}
	result := int32(0)
	if calculated.Outcome.Winner == 0 {
		result = 1
	}
	var pendingFight db.P7RushFight
	if err = db.GetIns().First(&pendingFight, "id=?", start.GetFightId()).Error; err != nil {
		t.Fatal(err)
	}
	err = db.GetIns().WithContext(p7WithRushHistoricalQuery(p7WithRushReadRequestCache(f.ctx))).Transaction(func(tx *gorm.DB) error {
		_, handled, e := p7ReadScopedRush(tx, &pendingFight, nil)
		if e != nil {
			return e
		}
		if !handled {
			return fmt.Errorf("pending source missing")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(p7RushHistoryCache.rows) != 0 {
		t.Fatal("pending preview cached")
	}
	end := &pb.CSRushFightEnd{FightId: start.FightId, Result: proto.Int32(result), OptRecord: calculated.OriginalOperates}
	if _, err = f.s.p7End(f.ctx, f.id, end); err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().First(&manual, "fight_id=?", start.GetFightId()).Error; err != nil {
		t.Fatal(err)
	}
	var fight db.P7RushFight
	if err = db.GetIns().First(&fight, "id=?", start.GetFightId()).Error; err != nil {
		t.Fatal(err)
	}

	query := func() context.Context { return p7WithRushHistoricalQuery(p7WithRushReadRequestCache(f.ctx)) }
	read := func(ctx context.Context, fn func(*gorm.DB, *model.CombatOutcome) error) error {
		return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			out, handled, e := p7ReadScopedRush(tx, &fight, end)
			if e != nil {
				return e
			}
			if !handled || out == nil {
				return fmt.Errorf("missing authoritative result")
			}
			return fn(tx, out)
		})
	}
	if err = read(query(), func(_ *gorm.DB, out *model.CombatOutcome) error { out.Winner = 999; return nil }); err != nil {
		t.Fatal(err)
	}
	if len(p7RushHistoryCache.rows) != 1 {
		t.Fatal("accepted query not cached")
	}
	var originalExpiry time.Time
	for _, entry := range p7RushHistoryCache.rows {
		originalExpiry = entry.expires
		t.Logf("verified fixture compressed bytes=%d", len(entry.raw))
	}
	seq := p7RushHistoryCache.sequence
	if err = read(query(), func(_ *gorm.DB, out *model.CombatOutcome) error {
		if out.Winner == 999 {
			return fmt.Errorf("clone polluted")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if p7RushHistoryCache.sequence != seq+1 {
		t.Fatal("new transaction did not hit")
	}
	for _, entry := range p7RushHistoryCache.rows {
		if !entry.expires.Equal(originalExpiry) {
			t.Fatal("cross-request read reverified and republished instead of hitting")
		}
	}
	// Corrupt cache payloads must fall back to the complete authoritative verifier.
	for key, entry := range p7RushHistoryCache.rows {
		p7RushHistoryCache.bytes -= len(entry.raw)
		entry.raw = []byte("bad gzip")
		p7RushHistoryCache.bytes += len(entry.raw)
		p7RushHistoryCache.rows[key] = entry
	}
	if err = read(query(), func(_ *gorm.DB, out *model.CombatOutcome) error {
		if !out.Authoritative {
			return fmt.Errorf("non-authoritative fallback")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// An End-style request context must never access the cross-request cache.
	seq = p7RushHistoryCache.sequence
	if err = read(p7WithRushReadRequestCache(f.ctx), func(_ *gorm.DB, _ *model.CombatOutcome) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if p7RushHistoryCache.sequence != seq {
		t.Fatal("End context accessed cache")
	}
	err = db.GetIns().WithContext(query()).Transaction(func(tx *gorm.DB) error {
		var record db.P6BattleRecord
		if e := tx.First(&record, "id=?", manual.P6RecordID).Error; e != nil {
			return e
		}
		// Each compressed/raw evidence blob changes while every declared digest stays intact.
		for _, mutation := range []struct {
			row    any
			column string
			raw    []byte
		}{
			{&db.P6BattleRecord{}, "input", record.Input}, {&db.P6BattleRecord{}, "operates", record.Operates}, {&db.P6BattleRecord{}, "outcome", record.Outcome},
			{&db.P7RushManualCalculation{}, "series", manual.Series}, {&db.P7RushManualCalculation{}, "terminal", manual.Terminal},
		} {
			original := mutation.raw
			if len(original) == 0 {
				return fmt.Errorf("empty fixture blob %s", mutation.column)
			}
			changed := append([]byte(nil), original...)
			changed[0] ^= 1
			condition, value := "id=?", record.ID
			if _, ok := mutation.row.(*db.P7RushManualCalculation); ok {
				condition, value = "fight_id=?", fight.ID
			}
			if e := tx.Model(mutation.row).Where(condition, value).Update(mutation.column, changed).Error; e != nil {
				return e
			}
			if _, _, e := p7ReadScopedRush(tx, &fight, end); e == nil {
				return fmt.Errorf("tamper accepted: %s", mutation.column)
			}
			if e := tx.Model(mutation.row).Where(condition, value).Update(mutation.column, original).Error; e != nil {
				return e
			}
		}
		for _, column := range []string{"start", "rule"} {
			altered := fight
			original := fight.Start
			if column == "rule" {
				original = fight.Rule
			}
			changed := append([]byte(nil), original...)
			changed[0] ^= 1
			if column == "start" {
				altered.Start = changed
			} else {
				altered.Rule = changed
			}
			if e := tx.Model(&db.P7RushFight{}).Where("id=?", fight.ID).Update(column, changed).Error; e != nil {
				return e
			}
			if _, _, e := p7ReadScopedRush(tx, &altered, end); e == nil {
				return fmt.Errorf("fight blob tamper accepted: %s", column)
			}
			if e := tx.Model(&db.P7RushFight{}).Where("id=?", fight.ID).Update(column, original).Error; e != nil {
				return e
			}
		}
		if len(p7RushHistoryCache.rows) != 1 {
			return fmt.Errorf("failed verification cached")
		}
		pending := fight
		pending.Status = constUtil.P7RushComputed
		var key p7RushHistoricalKey
		p7RushReadFingerprint(tx, &pending, end, &key)
		if key.database != nil {
			return fmt.Errorf("pending eligible")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestP7Feedback65RePrerequisiteReusesAcceptedHistory(t *testing.T) {
	if !strings.HasPrefix(os.Getenv("HYJS_P7_MYSQL_DATABASE"), "hyjs_test_p7_") {
		t.Skip("owned isolated MySQL P7 database required")
	}
	f := p7PartyBusinessFixture(t)
	if err := f.s.PublishP7ReRush(f.ctx); err != nil {
		t.Fatal(err)
	}
	old := p7RushHistoryCache
	p7RushHistoryCache = &p7RushHistoricalCache{rows: make(map[p7RushHistoricalKey]p7RushHistoricalEntry)}
	t.Cleanup(func() { p7RushHistoryCache = old })
	start, err := f.s.p7Start(f.ctx, f.id, &pb.CSRushFightStart{RushType: proto.Int32(1), TaskId: proto.String("local_rush_1"), Star: proto.Int32(1)}, 65101, false)
	if err != nil {
		t.Fatal(err)
	}
	series, err := f.s.FreezeP6RushSeriesInput(start, true, false, nil, constUtil.P6OriginalClientHash)
	if err != nil {
		t.Fatal(err)
	}
	input, err := p7GenericMechanismRushInputAt(series, 0)
	if err != nil {
		t.Fatal(err)
	}
	client, err := f.s.ComputeOriginalP6ProtocolFinalHP(f.ctx, input, nil)
	if err != nil {
		t.Fatal(err)
	}
	if client.Outcome.Winner != 0 || client.Outcome.Authoritative {
		t.Fatal("finite original roster lost; retain actual outcome")
	}
	end := &pb.CSRushFightEnd{FightId: start.FightId, Result: proto.Int32(1), OptRecord: client.OriginalOperates}
	before, beforeGame := f.profile(t)
	lateFailure := errors.New("feedback65 profile save after accepted fight")
	callback := "p7feedback65:reject-final-profile-save"
	lateReached := false
	if err = db.GetIns().Callback().Update().After("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table != "profiles" {
			return
		}
		var fight db.P7RushFight
		if e := tx.Session(&gorm.Session{NewDB: true}).First(&fight, "id=?", start.GetFightId()).Error; e != nil {
			tx.AddError(e)
			return
		}
		if fight.Status == constUtil.P7RushSettled {
			lateReached = true
			if len(p7RushHistoryCache.rows) != 0 {
				tx.AddError(errors.New("uncommitted accepted history published"))
				return
			}
			tx.AddError(lateFailure)
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.GetIns().Callback().Update().Remove(callback) })
	_, failed := f.s.p7End(f.ctx, f.id, end)
	if err = db.GetIns().Callback().Update().Remove(callback); err != nil {
		t.Fatal(err)
	}
	if !lateReached || !errors.Is(failed, lateFailure) || len(p7RushHistoryCache.rows) != 0 {
		t.Fatal("late rollback did not keep history unpublished", lateReached, failed)
	}
	var rolledFight db.P7RushFight
	var rolledManual db.P7RushManualCalculation
	var rolledRecord db.P6BattleRecord
	if err = db.GetIns().First(&rolledFight, "id=?", start.GetFightId()).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().First(&rolledManual, "fight_id=?", start.GetFightId()).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().First(&rolledRecord, "id=?", rolledFight.ReplayID).Error; err != nil {
		t.Fatal(err)
	}
	afterRollback, afterRollbackGame := f.profile(t)
	if rolledFight.Status != constUtil.P7RushComputed || len(rolledFight.Result) != 0 || len(rolledFight.EndRequest) != 0 || rolledManual.Status != constUtil.P7RushManualPending || len(rolledManual.Terminal) != 0 || rolledRecord.Status != constUtil.P6BattleCaptured || rolledRecord.Authoritative || !proto.Equal(before, afterRollback) || !p6CopySeriesJSONEqual(t, beforeGame, afterRollbackGame) {
		t.Fatal("late profile failure left accepted source, reward or profile state")
	}
	if _, err = f.s.p7End(f.ctx, f.id, end); err != nil {
		t.Fatal(err)
	}
	if len(p7RushHistoryCache.rows) != 1 {
		t.Fatal("successful commit did not immediately warm accepted history")
	}
	var expiry time.Time
	for _, entry := range p7RushHistoryCache.rows {
		expiry = entry.expires
	}
	if _, err = f.s.p7End(f.ctx, f.id, end); err != nil {
		t.Fatal(err)
	}
	for _, entry := range p7RushHistoryCache.rows {
		if !entry.expires.Equal(expiry) {
			t.Fatal("settled End replay renewed history")
		}
	}
	read := func() error {
		return db.GetIns().WithContext(p7WithRushReadRequestCache(f.ctx)).Transaction(func(tx *gorm.DB) error {
			wins, e := p7ReRushAcceptedWins(tx, f.id, 1, 1, 1)
			if e == nil && (len(wins) != 1 || wins[1].Fight != start.GetFightId() || wins[1].Star != 1) {
				return fmt.Errorf("accepted prerequisite source changed")
			}
			return e
		})
	}
	if err = read(); err != nil {
		t.Fatal(err)
	}
	if len(p7RushHistoryCache.rows) != 1 {
		t.Fatal("accepted prerequisite was not cached")
	}
	sequence := p7RushHistoryCache.sequence
	if err = read(); err != nil {
		t.Fatal(err)
	}
	if p7RushHistoryCache.sequence != sequence+1 {
		t.Fatal("new prerequisite transaction did not reuse history")
	}
	for _, entry := range p7RushHistoryCache.rows {
		if !entry.expires.Equal(expiry) {
			t.Fatal("history recomputed on repeated prerequisite")
		}
	}
	var manual db.P7RushManualCalculation
	if err = db.GetIns().First(&manual, "fight_id=?", start.GetFightId()).Error; err != nil {
		t.Fatal(err)
	}
	changed := append([]byte(nil), manual.Terminal...)
	changed[0] ^= 1
	if err = db.GetIns().Model(&manual).Update("terminal", changed).Error; err != nil {
		t.Fatal(err)
	}
	if err = read(); err == nil {
		t.Fatal("changed actual history blob bypassed prerequisite verification")
	}
}

func TestP7RushHistoricalFingerprintBindsEveryStoredField(t *testing.T) {
	for _, row := range []any{db.P7RushFight{}, db.P7RushManualCalculation{}, db.P6BattleRecord{}} {
		original := reflect.ValueOf(row)
		before, err := p7RushStoredRowFingerprint(row)
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < original.NumField(); i++ {
			changed := reflect.New(original.Type()).Elem()
			changed.Set(original)
			field := changed.Field(i)
			if !field.CanSet() {
				continue
			}
			switch field.Kind() {
			case reflect.String:
				field.SetString("changed")
			case reflect.Int, reflect.Int32, reflect.Int64:
				field.SetInt(1)
			case reflect.Bool:
				field.SetBool(true)
			case reflect.Slice:
				if field.Type().Elem().Kind() != reflect.Uint8 {
					t.Fatalf("uncovered field %s", original.Type().Field(i).Name)
				}
				field.SetBytes([]byte{1})
			default:
				t.Fatalf("uncovered kind %v", field.Kind())
			}
			after, e := p7RushStoredRowFingerprint(changed.Interface())
			if e != nil || before == after {
				t.Fatalf("unbound %s.%s: %v", original.Type(), original.Type().Field(i).Name, e)
			}
		}
	}
}

func TestP7RushHistoricalBoundedDecoder(t *testing.T) {
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write(make([]byte, constUtil.P7RushHistoricalReadCacheDecodedBytes+1)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	for _, raw := range [][]byte{compressed.Bytes(), []byte("invalid"), compressed.Bytes()[:len(compressed.Bytes())/2]} {
		if _, err := p7DecodeHistoricalOutcome(raw); err == nil {
			t.Fatal("unsafe compressed entry accepted")
		}
	}
	if _, err := p7EncodeHistoricalOutcome(&model.CombatOutcome{Authoritative: true, CapabilityHash: string(make([]byte, constUtil.P7RushHistoricalReadCacheDecodedBytes))}); err == nil {
		t.Fatal("oversized JSON encoded")
	}
}
