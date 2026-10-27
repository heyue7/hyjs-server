package service

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// The shape is shared with the raw stored-row canonical encoding. NULL is
// distinct from an empty blob; SQL SHA2 always hashes the actual stored bytes.
type p7RushBlobDigest struct {
	Length int
	Nil    bool
	SHA256 string
}

func p7RushDurableRowFingerprint(row any, omitted map[string]p7RushBlobDigest) (string, error) {
	v := reflect.Indirect(reflect.ValueOf(row))
	fields := map[string]any{"__type": v.Type().String()}
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		if !f.CanInterface() {
			continue
		}
		name := v.Type().Field(i).Name
		value := f.Interface()
		if raw, ok := value.([]byte); ok {
			if digest, exists := omitted[name]; exists {
				if err := p7RushValidateBlobDigest(digest); err != nil {
					return "", err
				}
				value = digest
			} else {
				value = p7RushBlobDigest{len(raw), raw == nil, p6Digest(raw)}
			}
		}
		fields[name] = value
	}
	raw, err := json.Marshal(fields)
	if err != nil {
		return "", err
	}
	return p6Digest(raw), nil
}

// Every Gorm-stored byte slice is explicitly represented by actual-byte
// SHA256, length and SQL NULL state. Scalar/new model fields remain included.
func p7RushDurableProjectedRows(tx *gorm.DB, ids []string, key string, output any, digests *map[string]map[string]p7RushBlobDigest) error {
	if tx.Dialector.Name() != "mysql" {
		return tx.Where(key+" IN ?", ids).Find(output).Error
	}
	slice := reflect.ValueOf(output).Elem()
	typ := slice.Type().Elem()
	stmt := &gorm.Statement{DB: tx}
	if err := stmt.Parse(reflect.New(typ).Interface()); err != nil {
		return err
	}
	var columns []string
	for _, name := range stmt.Schema.DBNames {
		field := stmt.Schema.FieldsByDBName[name]
		if field.FieldType == reflect.TypeOf([]byte(nil)) {
			columns = append(columns, "LOWER(SHA2(COALESCE(`"+name+"`, ''),256))", "OCTET_LENGTH(COALESCE(`"+name+"`, ''))", "(`"+name+"` IS NULL)")
		} else {
			columns = append(columns, "`"+name+"`")
		}
	}
	rows, err := tx.Table(stmt.Schema.Table).Select(strings.Join(columns, ",")).Where(key+" IN ?", ids).Rows()
	if err != nil {
		return err
	}
	defer rows.Close()
	if *digests == nil {
		*digests = map[string]map[string]p7RushBlobDigest{}
	}
	for rows.Next() {
		value := reflect.New(typ).Elem()
		dest := []any{}
		blobs := map[string]*p7RushBlobDigest{}
		for _, name := range stmt.Schema.DBNames {
			field := stmt.Schema.FieldsByDBName[name]
			if field.FieldType == reflect.TypeOf([]byte(nil)) {
				digest := &p7RushBlobDigest{}
				blobs[field.Name] = digest
				dest = append(dest, &digest.SHA256, &digest.Length, &digest.Nil)
			} else {
				dest = append(dest, field.ReflectValueOf(tx.Statement.Context, value).Addr().Interface())
			}
		}
		if err := rows.Scan(dest...); err != nil {
			return err
		}
		rowDigests := map[string]p7RushBlobDigest{}
		for name, digest := range blobs {
			if err := p7RushValidateBlobDigest(*digest); err != nil {
				return err
			}
			rowDigests[name] = *digest
		}
		id, _ := stmt.Schema.FieldsByDBName[key].ValueOf(tx.Statement.Context, value)
		(*digests)[id.(string)] = rowDigests
		slice.Set(reflect.Append(slice, value))
	}
	return rows.Err()
}
func (q *p7EliteQueryProjection) loadManualRows(ids []string, rows *[]db.P7RushManualCalculation) error {
	return p7RushDurableProjectedRows(q.tx, ids, "fight_id", rows, &q.manualBlobs)
}
func (q *p7EliteQueryProjection) loadRecordRows(ids []string, rows *[]db.P6BattleRecord) error {
	return p7RushDurableProjectedRows(q.tx, ids, "id", rows, &q.recordBlobs)
}
func (q *p7EliteQueryProjection) loadScopedRows(ids []string, rows *[]db.P7RushScopedCalculation) error {
	return p7RushDurableProjectedRows(q.tx, ids, "fight_id", rows, &q.scopedBlobs)
}

// Batch download only missing/untrusted proofs. No replay takes place here:
// unrelated later stages retain their original lazy validation/error boundary.
func (q *p7EliteQueryProjection) loadProofMissBlobs() error {
	if q.tx.Dialector.Name() != "mysql" {
		return nil
	}
	var manualIDs, scopedIDs, recordIDs []string
	records := map[string]bool{}
	for _, fight := range q.fights {
		manual, ok := q.manuals[fight.ID]
		if !ok {
			if scoped, exists := q.scoped[fight.ID]; exists {
				record, exists := q.records[scoped.P6RecordID]
				var end pb.CSRushFightEnd
				accepted := false
				if exists && proto.Unmarshal(fight.EndRequest, &end) == nil && q.certificateRuntime != nil {
					fingerprint, err := p7RushDurableFingerprint(&fight, &scoped, &record, &end, q.scopedBlobs[fight.ID], q.recordBlobs[record.ID])
					accepted = err == nil && q.certificateRuntime.acceptedProjection(q.certificates[fight.ID], q.serverID, q.state.PlayerID, fight.ID, fingerprint, &record) != nil
				}
				if !accepted {
					scopedIDs = append(scopedIDs, fight.ID)
					records[scoped.P6RecordID] = true
				}
			} else {
				records[fight.ReplayID] = true
			}
			continue
		}
		record, ok := q.records[manual.P6RecordID]
		var end pb.CSRushFightEnd
		accepted := false
		if ok && proto.Unmarshal(fight.EndRequest, &end) == nil && q.certificateRuntime != nil {
			fingerprint, err := p7RushDurableFingerprint(&fight, &manual, &record, &end, q.manualBlobs[fight.ID], q.recordBlobs[record.ID])
			accepted = err == nil && q.certificateRuntime.acceptedProjection(q.certificates[fight.ID], q.serverID, q.state.PlayerID, fight.ID, fingerprint, &record) != nil
		}
		if !accepted {
			manualIDs = append(manualIDs, fight.ID)
			records[manual.P6RecordID] = true
		}
	}
	for id := range records {
		recordIDs = append(recordIDs, id)
	}
	for from := 0; from < len(manualIDs); from += 128 {
		var rows []db.P7RushManualCalculation
		if err := q.tx.Where("fight_id IN ?", manualIDs[from:min(from+128, len(manualIDs))]).Find(&rows).Error; err != nil {
			return err
		}
		for _, r := range rows {
			q.manuals[r.FightID] = r
			delete(q.manualBlobs, r.FightID)
		}
	}
	for from := 0; from < len(scopedIDs); from += 128 {
		var rows []db.P7RushScopedCalculation
		if err := q.tx.Where("fight_id IN ?", scopedIDs[from:min(from+128, len(scopedIDs))]).Find(&rows).Error; err != nil {
			return err
		}
		for _, r := range rows {
			q.scoped[r.FightID] = r
			delete(q.scopedBlobs, r.FightID)
		}
	}

	for from := 0; from < len(recordIDs); from += 128 {
		var rows []db.P6BattleRecord
		if err := q.tx.Where("id IN ?", recordIDs[from:min(from+128, len(recordIDs))]).Find(&rows).Error; err != nil {
			return err
		}
		for _, r := range rows {
			q.records[r.ID] = r
			delete(q.recordBlobs, r.ID)
		}
	}
	return nil
}

// Called only after the current End's original full replay has succeeded and
// its accepted rows have been persisted in this same transaction. Never replay
// an already verified battle merely to publish its durable settlement proof.
func p7PersistRushSettlementProof(tx *gorm.DB, fight *db.P7RushFight, end *pb.CSRushFightEnd, outcome *model.CombatOutcome, serverID int64) error {
	runtime := p7RushCertificateRuntimeFor(tx)
	if runtime == nil {
		return nil
	}
	if fight == nil || end == nil || fight.Status != constUtil.P7RushSettled || fight.Diagnostic || fight.PlayerID < 1 || end.GetFightId() != fight.ID {
		return errors.New(constUtil.P7ScopedRushConflict)
	}
	var source any
	recordID := ""
	var manual db.P7RushManualCalculation
	err := tx.First(&manual, "fight_id=?", fight.ID).Error
	if err == nil {
		if manual.PlayerID != fight.PlayerID || manual.Status != constUtil.P7RushManualAccepted || manual.AcceptedAt <= 0 {
			return errors.New(constUtil.P7ScopedRushConflict)
		}
		source = &manual
		recordID = manual.P6RecordID
	} else if errors.Is(err, gorm.ErrRecordNotFound) {
		var scoped db.P7RushScopedCalculation
		if err = tx.First(&scoped, "fight_id=?", fight.ID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		if scoped.PlayerID != fight.PlayerID {
			return errors.New(constUtil.P7ScopedRushConflict)
		}
		source = &scoped
		recordID = scoped.P6RecordID
	} else {
		return err
	}
	var record db.P6BattleRecord
	if err = tx.First(&record, "id=?", recordID).Error; err != nil {
		return err
	}
	fingerprint, err := p7RushDurableFingerprint(fight, source, &record, end, nil, nil)
	if err != nil {
		return err
	}
	q := &p7EliteQueryProjection{tx: tx, state: &db.P7RushState{PlayerID: fight.PlayerID}, serverID: serverID, certificateRuntime: runtime, certificates: map[string]db.P7RushHistoryCertificate{}}
	return q.certify(fight, &record, fingerprint, outcome)
}

func p7RushDurableCloneOutcome(outcome *model.CombatOutcome) *model.CombatOutcome {
	if outcome == nil {
		return nil
	}
	raw, err := json.Marshal(outcome)
	if err != nil {
		return nil
	}
	var clone model.CombatOutcome
	if json.Unmarshal(raw, &clone) != nil {
		return nil
	}
	return &clone
}

func p7RushValidateBlobDigest(digest p7RushBlobDigest) error {
	decoded, err := hex.DecodeString(digest.SHA256)
	if err != nil || len(decoded) != 32 || len(digest.SHA256) != 64 || digest.Length < 0 || (digest.Nil && (digest.Length != 0 || digest.SHA256 != p6Digest(nil))) {
		return fmt.Errorf("invalid actual trial source blob digest")
	}
	return nil
}
