package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/config"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p7DurableHistoryFixture(t *testing.T, ctx context.Context) {
	t.Helper()
	old := config.AppConfigIns
	var current config.AppConfig
	if old != nil {
		current = *old
	}
	current.Security.RushHistoryCertificateKey = "isolated-durable-history-test-key-32-bytes"
	config.AppConfigIns = &current
	t.Cleanup(func() { config.AppConfigIns = old })
	p7InitializeDurableHistoryTest(t, ctx)
}

func p7InitializeDurableHistoryTest(t *testing.T, ctx context.Context) {
	t.Helper()
	if err := InitializeP7RushHistoryCertificates(ctx); err != nil {
		t.Fatal(err)
	}
	pool, err := db.GetIns().DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { p7RushCertificateDatabases.Delete(pool) })
}

func p7HistoryCertificateCount(t *testing.T, fightID string) int64 {
	t.Helper()
	var count int64
	if err := db.GetIns().Model(&db.P7RushHistoryCertificate{}).Where("fight_id=?", fightID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	return count
}

func TestP7RushHistoricalAcceptedQueryCloneAndTamper(t *testing.T) {
	f, _, startRequest := p7ScopedFixture(t, constUtil.P7RushTrial)
	p7DurableHistoryFixture(t, f.ctx)
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
	err = db.GetIns().WithContext(p7WithRushReadRequestCache(f.ctx)).Transaction(func(tx *gorm.DB) error {
		_, handled, e := p7ReadScopedRush(tx, &pendingFight, nil)
		if e != nil {
			return e
		}
		if !handled {
			return fmt.Errorf("pending source missing")
		}
		var state db.P7RushState
		if e := tx.First(&state, "player_id=?", f.id).Error; e != nil {
			return e
		}
		bound, e := f.s.p7BindSettledQueryProjection(tx, &state)
		if e != nil {
			return e
		}
		q := p7EliteProjectionFor(bound)
		if e = q.load(); e != nil {
			return e
		}
		for _, row := range q.fights {
			if row.ID == pendingFight.ID {
				return fmt.Errorf("pending source entered historical projection")
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if p7HistoryCertificateCount(t, start.GetFightId()) != 0 {
		t.Fatal("pending preview acquired settled proof")
	}
	end := &pb.CSRushFightEnd{FightId: start.FightId, Result: proto.Int32(result), OptRecord: calculated.OriginalOperates}
	beforeRollback, beforeRollbackGame := f.profile(t)
	lateFailure := errors.New("durable proof final profile save rollback")
	callback := "p7durablehistory:reject-profile-save"
	reached := false
	if err = db.GetIns().Callback().Update().After("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table != "profiles" {
			return
		}
		var settled db.P7RushFight
		if e := tx.Session(&gorm.Session{NewDB: true}).First(&settled, "id=?", start.GetFightId()).Error; e != nil {
			tx.AddError(e)
			return
		}
		if settled.Status != constUtil.P7RushSettled {
			return
		}
		reached = true
		var count int64
		if e := tx.Session(&gorm.Session{NewDB: true}).Model(&db.P7RushHistoryCertificate{}).Where("fight_id=?", settled.ID).Count(&count).Error; e != nil {
			tx.AddError(e)
			return
		}
		if count != 1 {
			tx.AddError(fmt.Errorf("missing transactional proof: %d", count))
			return
		}
		tx.AddError(lateFailure)
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.GetIns().Callback().Update().Remove(callback) })
	_, failed := f.s.p7End(f.ctx, f.id, end)
	if err = db.GetIns().Callback().Update().Remove(callback); err != nil {
		t.Fatal(err)
	}
	if !reached || !errors.Is(failed, lateFailure) || p7HistoryCertificateCount(t, start.GetFightId()) != 0 {
		t.Fatal("late settlement failure left proof", reached, failed)
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
	if rolledFight.Status != pendingFight.Status || len(rolledFight.Result) != 0 || len(rolledFight.EndRequest) != 0 || rolledManual.Status != constUtil.P7RushManualPending || len(rolledManual.Terminal) != 0 || rolledRecord.Authoritative || !proto.Equal(beforeRollback, afterRollback) || !p6CopySeriesJSONEqual(t, beforeRollbackGame, afterRollbackGame) {
		t.Fatal("rollback left accepted source, rewards or profile")
	}
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

	var rawRecord db.P6BattleRecord
	if err = db.GetIns().First(&rawRecord, "id=?", manual.P6RecordID).Error; err != nil {
		t.Fatal(err)
	}
	var rawScoped []db.P7RushScopedCalculation
	if err = db.GetIns().Where("fight_id=?", fight.ID).Find(&rawScoped).Error; err != nil {
		t.Fatal(err)
	}
	expectProofHit := true
	projectedReads := 0
	checkProjection := func(q *p7EliteQueryProjection) error {
		bytes, digests := 0, 0
		check := func(projected, raw any, omitted map[string]p7RushBlobDigest) error {
			pv, rv := reflect.ValueOf(projected), reflect.ValueOf(raw)
			for i := 0; i < pv.NumField(); i++ {
				actual, ok := pv.Field(i).Interface().([]byte)
				if !ok {
					continue
				}
				name := pv.Type().Field(i).Name
				source := rv.Field(i).Interface().([]byte)
				if actual != nil {
					return fmt.Errorf("proof hit loaded source blob %s.%s", pv.Type(), name)
				}
				digest, ok := omitted[name]
				if !ok || digest.Length != len(source) || digest.Nil != (source == nil) || (source != nil && digest.SHA256 != p6Digest(source)) {
					return fmt.Errorf("missing actual source digest %s.%s", pv.Type(), name)
				}
				bytes += len(source)
				digests++
			}
			return nil
		}
		if err := check(q.manuals[fight.ID], manual, q.manualBlobs[fight.ID]); err != nil {
			return err
		}
		if err := check(q.records[rawRecord.ID], rawRecord, q.recordBlobs[rawRecord.ID]); err != nil {
			return err
		}
		for _, raw := range rawScoped {
			if err := check(q.scoped[raw.FightID], raw, q.scopedBlobs[raw.FightID]); err != nil {
				return err
			}
		}
		projectedReads++
		t.Logf("proof-hit actual source blob bytes=%d digest fields=%d projected reads=%d", bytes, digests, projectedReads)
		return nil
	}

	query := func() context.Context { return p7WithRushReadRequestCache(f.ctx) }
	read := func(ctx context.Context, fn func(*gorm.DB, *model.CombatOutcome) error) error {
		return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var state db.P7RushState
			if e := tx.First(&state, "player_id=?", f.id).Error; e != nil {
				return e
			}
			bound, e := f.s.p7BindSettledQueryProjection(tx, &state)
			if e != nil {
				return e
			}
			q := p7EliteProjectionFor(bound)
			if e = q.load(); e != nil {
				return e
			}
			if expectProofHit && tx.Dialector.Name() == "mysql" {
				if e = checkProjection(q); e != nil {
					return e
				}
			}
			out, handled, e := q.read(&fight, end)
			if e != nil {
				return e
			}
			if !handled || out == nil || !out.Authoritative || out.Winner != calculated.Outcome.Winner {
				return fmt.Errorf("missing authoritative result")
			}
			return fn(tx, out)
		})
	}
	if p7HistoryCertificateCount(t, fight.ID) != 1 {
		t.Fatal("settled End did not atomically persist proof")
	}
	var certificate db.P7RushHistoryCertificate
	if err = db.GetIns().First(&certificate, "fight_id=?", fight.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err = read(query(), func(_ *gorm.DB, out *model.CombatOutcome) error { out.Winner = 999; return nil }); err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	p7InitializeDurableHistoryTest(t, f.ctx)
	if err = read(query(), func(_ *gorm.DB, out *model.CombatOutcome) error {
		if out.Winner == 999 {
			return fmt.Errorf("previous transaction polluted durable outcome")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var reused db.P7RushHistoryCertificate
	if err = db.GetIns().First(&reused, "fight_id=?", fight.ID).Error; err != nil || !reflect.DeepEqual(certificate, reused) {
		t.Fatal("cold proof changed", err)
	}
	// Missing or unauthenticated proofs must run the strict verifier and repair durable evidence.
	for _, variant := range []string{"missing", "signature", "legacy-format", "legacy-validator"} {
		if variant == "missing" {
			err = db.GetIns().Where("fight_id=?", fight.ID).Delete(&db.P7RushHistoryCertificate{}).Error
		} else if variant == "signature" {
			err = db.GetIns().Model(&db.P7RushHistoryCertificate{}).Where("fight_id=?", fight.ID).Update("mac", p6Digest([]byte("forged proof"))).Error
		} else {
			// An obsolete signed contract must use strict stored-receipt validation
			// before issuing the current proof, even when its MAC is authentic.
			var obsolete db.P7RushHistoryCertificate
			err = db.GetIns().First(&obsolete, "fight_id=?", fight.ID).Error
			if err == nil {
				if variant == "legacy-format" {
					obsolete.Format = 1
				} else {
					obsolete.Validator = "obsolete-engine-contract"
				}
				runtime := p7RushCertificateRuntimeFor(db.GetIns())
				obsolete.MAC = runtime.sign(obsolete)
				err = db.GetIns().Save(&obsolete).Error
			}
		}
		if err != nil {
			t.Fatal(err)
		}
		expectProofHit = false
		if err = read(query(), func(_ *gorm.DB, out *model.CombatOutcome) error { return nil }); err != nil {
			t.Fatalf("%s strict fallback: %v", variant, err)
		}
		expectProofHit = true
		if p7HistoryCertificateCount(t, fight.ID) != 1 {
			t.Fatalf("%s fallback did not persist proof", variant)
		}
		if err = read(query(), func(_ *gorm.DB, out *model.CombatOutcome) error { return nil }); err != nil {
			t.Fatalf("%s repaired proof: %v", variant, err)
		}
	}
	err = db.GetIns().WithContext(query()).Transaction(func(tx *gorm.DB) error {
		check := func(value *db.P7RushFight) error {
			var state db.P7RushState
			if e := tx.First(&state, "player_id=?", f.id).Error; e != nil {
				return e
			}
			bound, e := f.s.p7BindSettledQueryProjection(tx, &state)
			if e != nil {
				return e
			}
			q := p7EliteProjectionFor(bound)
			if e = q.load(); e != nil {
				return e
			}
			_, _, e = q.read(value, end)
			return e
		}
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
			if e := check(&fight); e == nil {
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
			if e := check(&altered); e == nil {
				return fmt.Errorf("fight blob tamper accepted: %s", column)
			}
			if e := tx.Model(&db.P7RushFight{}).Where("id=?", fight.ID).Update(column, original).Error; e != nil {
				return e
			}
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
	p7DurableHistoryFixture(t, f.ctx)
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
			var count int64
			if e := tx.Session(&gorm.Session{NewDB: true}).Model(&db.P7RushHistoryCertificate{}).Where("fight_id=?", start.GetFightId()).Count(&count).Error; e != nil {
				tx.AddError(e)
				return
			}
			if count != 1 {
				tx.AddError(errors.New("settlement proof missing inside owning transaction"))
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
	if !lateReached || !errors.Is(failed, lateFailure) || p7HistoryCertificateCount(t, start.GetFightId()) != 0 {
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
	if p7HistoryCertificateCount(t, start.GetFightId()) != 1 {
		t.Fatal("commit missing durable proof")
	}
	var certificate db.P7RushHistoryCertificate
	if err = db.GetIns().First(&certificate, "fight_id=?", start.GetFightId()).Error; err != nil {
		t.Fatal(err)
	}
	beforeReplay, beforeReplayGame := f.profile(t)
	f.restart(t)
	p7InitializeDurableHistoryTest(t, f.ctx)
	if _, err = f.s.p7End(f.ctx, f.id, end); err != nil {
		t.Fatal(err)
	}
	afterReplay, afterReplayGame := f.profile(t)
	if !proto.Equal(beforeReplay, afterReplay) || !p6CopySeriesJSONEqual(t, beforeReplayGame, afterReplayGame) {
		t.Fatal("cold duplicate End changed rewards/profile")
	}
	read := func() error {
		return db.GetIns().WithContext(p7WithRushReadRequestCache(f.ctx)).Transaction(func(tx *gorm.DB) error {
			var state db.P7RushState
			if e := tx.First(&state, "player_id=?", f.id).Error; e != nil {
				return e
			}
			bound, e := f.s.p7BindSettledQueryProjection(tx, &state)
			if e != nil {
				return e
			}
			wins, e := p7ReRushAcceptedWins(bound, f.id, 1, 1, 1)
			if e == nil && (len(wins) != 1 || wins[1].Fight != start.GetFightId() || wins[1].Star != 1) {
				return fmt.Errorf("accepted prerequisite source changed")
			}
			return e
		})
	}
	if err = read(); err != nil {
		t.Fatal(err)
	}
	if err = read(); err != nil {
		t.Fatal(err)
	}
	var reused db.P7RushHistoryCertificate
	if err = db.GetIns().First(&reused, "fight_id=?", start.GetFightId()).Error; err != nil || !reflect.DeepEqual(certificate, reused) {
		t.Fatal("fresh prerequisite transaction rewrote proof", err)
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
