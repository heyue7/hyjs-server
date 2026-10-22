package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"testing"

	"hyjs-server/constUtil"
	"hyjs-server/db"
)

func p6CompactionRecord(id string, playerID int64, input []byte, inputHash string) db.P6BattleRecord {
	return db.P6BattleRecord{
		ID:                id,
		PlayerID:          playerID,
		Source:            "compact",
		SourceFight:       id,
		Status:            constUtil.P6BattleObserved,
		Seed:              42,
		ConfigurationHash: "configuration",
		EngineVersion:     "engine",
		ClientVersion:     "client",
		Input:             input,
		InputHash:         inputHash,
		Operates:          []byte("operates"),
		Outcome:           []byte("outcome"),
		ReplayHash:        "replay",
		OutcomeHash:       "outcome",
		ClientResult:      1,
		ServerResult:      1,
		Authoritative:     true,
		CreatedAt:         100,
		ExpiresAt:         200,
		FinishedAt:        150,
	}
}

func p6CompactionRecords(t *testing.T) map[string]db.P6BattleRecord {
	t.Helper()
	var rows []db.P6BattleRecord
	if err := db.GetIns().Order("id ASC").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	out := make(map[string]db.P6BattleRecord, len(rows))
	for _, row := range rows {
		out[row.ID] = row
	}
	return out
}

func p6CompactionWithoutInput(row db.P6BattleRecord) db.P6BattleRecord {
	row.Input = nil
	return row
}

func TestCompactLegacyP6BattleInputsMixedAndIdempotent(t *testing.T) {
	s, ctx, id, frozen := p6Recorder(t)
	if os.Getenv("HYJS_P4_MYSQL_DATABASE") != "" {
		f, _ := adminModesFixture(t)
		s, ctx, id = f.s, f.ctx, f.id
		if err := s.InitializeP6(ctx); err != nil {
			t.Fatal(err)
		}
		frozen = p6Input(t, s, p6Actor("hero_047", "player", "047_1", 1, 1000), p6Actor("hero_047", "enemy", "047_1", 1, 1000), 12)
	}
	raw, err := json.Marshal(frozen)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) <= constUtil.P6StoredInputCompressionThreshold {
		t.Fatalf("real frozen input too small for legacy compaction: %d", len(raw))
	}
	compressed, err := encodeP6StoredInput(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(compressed) >= len(raw) || !bytes.HasPrefix(compressed, []byte{0x1f, 0x8b}) {
		t.Fatal("fixture compressed input was not smaller gzip")
	}
	small := []byte(`{"small":true}`)
	unknown := bytes.Repeat([]byte{0x00, 0x01, 0xff}, constUtil.P6StoredInputCompressionThreshold)
	records := []db.P6BattleRecord{
		p6CompactionRecord("a-legacy", id, raw, p6Digest(raw)),
		p6CompactionRecord("b-compressed", id, compressed, p6Digest(raw)),
		p6CompactionRecord("c-small", id, small, p6Digest(small)),
		p6CompactionRecord("d-bad-hash", id, append([]byte(nil), raw...), p6Digest([]byte("different-content"))),
		p6CompactionRecord("e-unknown", id, unknown, p6Digest(unknown)),
	}
	if err = db.GetIns().Create(&records).Error; err != nil {
		t.Fatal(err)
	}
	before := p6CompactionRecords(t)

	result, err := s.CompactLegacyP6BattleInputs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.Scanned != 5 || result.Compacted != 1 || result.SkippedInvalid != 2 || result.BytesBefore != int64(len(raw)) || result.BytesAfter <= 0 || result.BytesAfter >= result.BytesBefore {
		t.Fatalf("unexpected first compaction stats: %+v", result)
	}
	after := p6CompactionRecords(t)
	legacy := after["a-legacy"]
	if !bytes.HasPrefix(legacy.Input, []byte{0x1f, 0x8b}) || len(legacy.Input) >= len(raw) {
		t.Fatal("legacy input was not compacted")
	}
	restored, err := decodeP6StoredInput(legacy.Input)
	if err != nil || !bytes.Equal(restored, raw) || p6Digest(restored) != legacy.InputHash {
		t.Fatal("legacy canonical input or hash changed", err)
	}
	if !reflect.DeepEqual(p6CompactionWithoutInput(before["a-legacy"]), p6CompactionWithoutInput(legacy)) {
		t.Fatal("columns other than input changed")
	}
	for _, unchanged := range []string{"b-compressed", "c-small", "d-bad-hash", "e-unknown"} {
		if !reflect.DeepEqual(before[unchanged], after[unchanged]) {
			t.Fatalf("preserved record changed: %s", unchanged)
		}
	}

	second, err := s.CompactLegacyP6BattleInputs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if second.Scanned != 5 || second.Compacted != 0 || second.SkippedInvalid != 2 || second.BytesBefore != 0 || second.BytesAfter != 0 {
		t.Fatalf("second compaction was not idempotent: %+v", second)
	}
	if !reflect.DeepEqual(after, p6CompactionRecords(t)) {
		t.Fatal("idempotent run changed stored records")
	}
}

func TestCompactLegacyP6BattleInputsBatchBoundary(t *testing.T) {
	s, ctx, id, frozen := p6Recorder(t)
	raw, err := json.Marshal(frozen)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) <= constUtil.P6StoredInputCompressionThreshold {
		t.Fatalf("real frozen input too small for batch compaction: %d", len(raw))
	}
	records := make([]db.P6BattleRecord, 0, 9)
	for i := 0; i < 9; i++ {
		records = append(records, p6CompactionRecord("batch-"+string(rune('a'+i)), id, append([]byte(nil), raw...), p6Digest(raw)))
	}
	if err = db.GetIns().Create(&records).Error; err != nil {
		t.Fatal(err)
	}
	result, err := s.CompactLegacyP6BattleInputs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.Scanned != 9 || result.Compacted != 9 || result.SkippedInvalid != 0 || result.BytesBefore != int64(9*len(raw)) {
		t.Fatalf("unexpected boundary batch stats: %+v", result)
	}
	stored := p6CompactionRecords(t)
	for _, row := range stored {
		if !bytes.HasPrefix(row.Input, []byte{0x1f, 0x8b}) {
			t.Fatal("boundary record was not compacted")
		}
		restored, decodeErr := decodeP6StoredInput(row.Input)
		if decodeErr != nil || !bytes.Equal(restored, raw) || p6Digest(restored) != row.InputHash {
			t.Fatal("boundary record canonical input changed", decodeErr)
		}
	}
	second, err := s.CompactLegacyP6BattleInputs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if second.Scanned != 9 || second.Compacted != 0 || second.SkippedInvalid != 0 {
		t.Fatalf("boundary second run was not idempotent: %+v", second)
	}
}

func TestCompactLegacyP6BattleInputsCancellation(t *testing.T) {
	s, ctx, id, frozen := p6Recorder(t)
	raw, err := json.Marshal(frozen)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) <= constUtil.P6StoredInputCompressionThreshold {
		t.Fatalf("real frozen input too small for cancellation test: %d", len(raw))
	}
	if err = db.GetIns().Create(&db.P6BattleRecord{
		ID: "cancel-legacy", PlayerID: id, Source: "compact", SourceFight: "cancel-legacy",
		Status: constUtil.P6BattleObserved, Seed: 42, ConfigurationHash: "configuration",
		EngineVersion: "engine", ClientVersion: "client", Input: raw, InputHash: p6Digest(raw),
		CreatedAt: 100, ExpiresAt: 200,
	}).Error; err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	result, err := s.CompactLegacyP6BattleInputs(canceled)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation was not returned: %v", err)
	}
	if result == nil || result.Scanned != 0 || result.Compacted != 0 || result.BytesBefore != 0 || result.BytesAfter != 0 {
		t.Fatalf("cancellation processed rows: %+v", result)
	}
	var stored db.P6BattleRecord
	if err = db.GetIns().First(&stored, "id=?", "cancel-legacy").Error; err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stored.Input, raw) || stored.InputHash != p6Digest(raw) {
		t.Fatal("cancellation changed the stored input")
	}
}
