package service

import (
	"hyjs-server/db"
	"reflect"
	"testing"
)

func TestP7RushDurableRawProjectionCanonical(t *testing.T) {
	for _, raw := range [][]byte{nil, {}, []byte("actual immutable source")} {
		manual := db.P7RushManualCalculation{FightID: "owner", Series: raw, SeriesHash: "untrusted declaration"}
		actual, err := p7RushStoredRowFingerprint(&manual)
		if err != nil {
			t.Fatal(err)
		}
		projection := manual
		projection.Series = nil
		digest := map[string]p7RushBlobDigest{"Series": {len(raw), raw == nil, p6Digest(raw)}}
		projected, err := p7RushDurableRowFingerprint(&projection, digest)
		if err != nil || actual != projected {
			t.Fatalf("raw/projected mismatch: %v", err)
		}
		projection.DriverPolicy = "tampered driver"
		changed, _ := p7RushDurableRowFingerprint(&projection, digest)
		if changed == actual {
			t.Fatal("driver excluded")
		}
		digest["Series"] = p7RushBlobDigest{len(raw) + 1, false, p6Digest(append(raw, '!'))}
		changed, _ = p7RushDurableRowFingerprint(&manual, digest)
		if changed == actual {
			t.Fatal("actual blob excluded")
		}
	}
	record := db.P6BattleRecord{Input: []byte("large input"), Outcome: []byte("terminal"), Source: "rush"}
	expected, _ := p7RushStoredRowFingerprint(&record)
	digest := map[string]p7RushBlobDigest{"Input": {len(record.Input), false, p6Digest(record.Input)}}
	record.Input = nil
	actual, _ := p7RushDurableRowFingerprint(&record, digest)
	if actual != expected {
		t.Fatal("P6 canonical mismatch")
	}
}

// This iterates the actual models, so newly added stored blobs join the same
// raw/projected equivalence and tamper checks without an allow-list update.
func TestP7RushDurableEveryModelBlob(t *testing.T) {
	for _, row := range []any{&db.P7RushManualCalculation{}, &db.P7RushScopedCalculation{}, &db.P6BattleRecord{}} {
		value := reflect.ValueOf(row).Elem()
		digests := map[string]p7RushBlobDigest{}
		for i := 0; i < value.NumField(); i++ {
			field := value.Field(i)
			if field.Type() != reflect.TypeOf([]byte(nil)) {
				continue
			}
			raw := []byte(value.Type().Field(i).Name + ":actual-source")
			field.SetBytes(raw)
			digests[value.Type().Field(i).Name] = p7RushBlobDigest{len(raw), false, p6Digest(raw)}
		}
		expected, err := p7RushStoredRowFingerprint(row)
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < value.NumField(); i++ {
			field := value.Field(i)
			if field.Type() == reflect.TypeOf([]byte(nil)) {
				field.SetBytes(nil)
			}
		}
		actual, err := p7RushDurableRowFingerprint(row, digests)
		if err != nil || actual != expected {
			t.Fatalf("%T canonical mismatch: %v", row, err)
		}
		for name, original := range digests {
			bad := original
			bad.SHA256 = p6Digest([]byte("changed with stale declared hash"))
			digests[name] = bad
			changed, _ := p7RushDurableRowFingerprint(row, digests)
			if changed == expected {
				t.Fatalf("%T %s actual bytes excluded", row, name)
			}
			digests[name] = original
		}
	}
}

func TestP7RushDurableInvalidDigestRejected(t *testing.T) {
	for _, bad := range []p7RushBlobDigest{{Length: -1, SHA256: p6Digest(nil)}, {Length: 0, SHA256: "wrong"}, {Length: 1, Nil: true, SHA256: p6Digest(nil)}, {Length: 0, Nil: true, SHA256: p6Digest([]byte("tampered"))}} {
		if _, err := p7RushDurableRowFingerprint(&db.P6BattleRecord{}, map[string]p7RushBlobDigest{"Input": bad}); err == nil {
			t.Fatal("invalid SQL digest accepted")
		}
	}
}
