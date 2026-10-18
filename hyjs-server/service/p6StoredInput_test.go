package service

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"testing"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func TestP6StoredInputLegacyCompressedAndCorruptionBounds(t *testing.T) {
	for _, raw := range [][]byte{[]byte(`{"historical":true}`), bytes.Repeat([]byte(`{"frozen":"original"}`), 10000)} {
		stored, err := encodeP6StoredInput(raw)
		if err != nil {
			t.Fatal(err)
		}
		restored, err := decodeP6StoredInput(stored)
		if err != nil || !bytes.Equal(restored, raw) {
			t.Fatal("canonical input changed", err)
		}
		if len(raw) > constUtil.P6StoredInputCompressionThreshold {
			if len(stored) >= len(raw) || !bytes.HasPrefix(stored, []byte{0x1f, 0x8b}) {
				t.Fatal("large frozen input was not compressed")
			}
			corrupt := append([]byte(nil), stored...)
			corrupt[len(corrupt)-8] ^= 1
			for _, malformed := range [][]byte{corrupt, stored[:len(stored)-1], append(append([]byte(nil), stored...), 0), append(append([]byte(nil), stored...), stored...)} {
				if _, err := decodeP6StoredInput(malformed); err == nil {
					t.Fatal("corrupt, truncated or trailing member accepted")
				}
			}
		}
	}
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	block := bytes.Repeat([]byte("x"), 1024)
	for written := 0; written <= constUtil.P6StoredInputMaxBytes; written += len(block) {
		if _, err := writer.Write(block); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := decodeP6StoredInput(compressed.Bytes()); err == nil {
		t.Fatal("expanded input beyond the bound accepted")
	}
}

func TestP6CompressedStoredReplayColdAndHistoricalWireIdentity(t *testing.T) {
	s, ctx, path := newDailyCopyTest(t)
	if err := s.InitializeP6(ctx); err != nil {
		t.Fatal(err)
	}
	id := dailyCopyTestPlayer(t, ctx, "p6-storage-cold", 10)
	input := p6Input(t, s, p6Actor("hero_047", "player", "047_1", 1, 1000), p6Actor("hero_047", "enemy", "047_1", 1, 1000), 12)
	input.Attacker.PlayerId = proto.Int64(id)
	out, err := s.ComputeFrozenBattle(ctx, input, nil)
	if err != nil {
		t.Fatal(err)
	}
	var recordID string
	if err = p6Tx(ctx, id, func(tx *gorm.DB) error {
		var captureErr error
		recordID, captureErr = s.CaptureBattleReplay(ctx, tx, id, "storage", "cold", *input)
		if captureErr != nil {
			return captureErr
		}
		return s.FinishObservedBattleReplay(ctx, tx, id, "storage", "cold", out.Operates, out.Winner == 0)
	}); err != nil {
		t.Fatal(err)
	}
	var record db.P6BattleRecord
	if err = db.GetIns().First(&record, "id=?", recordID).Error; err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(record.Input, []byte{0x1f, 0x8b}) {
		t.Fatal("real frozen original tables were not compressed")
	}
	raw, err := decodeP6StoredInput(record.Input)
	if err != nil || p6Digest(raw) != record.InputHash {
		t.Fatal("canonical hash changed", err)
	}
	frozen, err := decodeP6Record(&record)
	if err != nil || !proto.Equal(frozen.Attacker, input.Attacker) {
		t.Fatal("record identity changed", err)
	}
	query := &pb.CSFightPlayback{FightId: proto.String(recordID), ServerId: proto.Int32(1)}
	before, err := s.fightPlayback(ctx, id, query)
	if err != nil {
		t.Fatal(err)
	}
	wire, err := proto.Marshal(before[0])
	if err != nil {
		t.Fatal(err)
	}
	connection, _ := db.GetIns().DB()
	if err = connection.Close(); err != nil {
		t.Fatal(err)
	}
	if err = db.InitGame(ctx, path); err != nil {
		t.Fatal(err)
	}
	coldConnection, _ := db.GetIns().DB()
	t.Cleanup(func() { coldConnection.Close() })
	after, err := s.fightPlayback(ctx, id, query)
	if err != nil {
		t.Fatal(err)
	}
	coldWire, err := proto.Marshal(after[0])
	if err != nil || !bytes.Equal(wire, coldWire) {
		t.Fatal("cold original replay bytes changed", err)
	}
	// Replace only the storage encoding to exercise historical plaintext without changing content.
	record.Input = raw
	if err = db.GetIns().Save(&record).Error; err != nil {
		t.Fatal(err)
	}
	legacy, err := s.fightPlayback(ctx, id, query)
	if err != nil || !proto.Equal(before[0], legacy[0]) {
		t.Fatal("historical plaintext replay changed", err)
	}
	var altered map[string]any
	if err = json.Unmarshal(raw, &altered); err != nil {
		t.Fatal(err)
	}
	altered["seed"] = float64(input.Seed + 1)
	tampered, _ := json.Marshal(altered)
	record.Input, err = encodeP6StoredInput(tampered)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = decodeP6Record(&record); err == nil {
		t.Fatal("compressed tampered content bypassed canonical hash")
	}
	t.Logf("frozen storage bytes=%d canonical bytes=%d", len(record.Input), len(raw))
}
