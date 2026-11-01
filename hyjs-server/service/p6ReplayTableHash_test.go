package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p6ReplayTableHashFixture(tb testing.TB) (*model.FrozenBattleInput, []byte, []byte) {
	tb.Helper()
	tables, err := readP6OriginalTestTables()
	if err != nil {
		tb.Fatal(err)
	}
	s := &GameService{Tables: cloneP6OriginalTestTables(tables)}
	frozen, err := s.FreezeBattleInput(model.FrozenBattleInput{
		Attacker:   &pb.BattlePlayer{PlayerId: proto.Int64(42), Formation: []*pb.BattleHero{p6Actor("hero_047", "player", "047_1", 1, 1000)}, Inst: &pb.BattlePlayerInst{Anger: proto.Float64(0)}},
		Defender:   &pb.BattlePlayer{PlayerId: proto.Int64(43), Formation: []*pb.BattleHero{p6Actor("hero_047", "enemy", "047_1", 1, 1000)}, Inst: &pb.BattlePlayerInst{Anger: proto.Float64(0)}},
		Seed:       0,
		MaxRound:   12,
		BattleType: 2,
	})
	if err != nil {
		tb.Fatal(err)
	}
	raw, err := json.Marshal(frozen)
	if err != nil {
		tb.Fatal(err)
	}
	stored, err := encodeP6StoredInput(raw)
	if err != nil || len(stored) < 2 || stored[0] != 0x1f || stored[1] != 0x8b {
		tb.Fatalf("original table input was not gzip encoded: %v", err)
	}
	return frozen, raw, stored
}

func p6ReplayTableHashRecord(frozen *model.FrozenBattleInput, raw, stored []byte) db.P6BattleRecord {
	const source, fight = "p6-table-hash-test", "fixture"
	return db.P6BattleRecord{
		ID:       p6Digest([]byte(fmt.Sprintf("%d/%s/%s", frozen.Attacker.GetPlayerId(), source, fight))),
		PlayerID: frozen.Attacker.GetPlayerId(), Source: source, SourceFight: fight, Status: constUtil.P6BattleCaptured,
		Seed: frozen.Seed, ConfigurationHash: frozen.ConfigurationHash, EngineVersion: frozen.EngineVersion,
		ClientVersion: frozen.ClientVersion, Input: stored, InputHash: p6Digest(raw),
	}
}

// Keep the former decoder's verification order as an independent benchmark baseline.
func p6ReplayTableHashBaseline(record *db.P6BattleRecord) (*model.FrozenBattleInput, error) {
	var input model.FrozenBattleInput
	if record == nil {
		return nil, errors.New(constUtil.P6ReplayConflict)
	}
	raw, err := decodeP6StoredInput(record.Input)
	if err != nil || p6Digest(raw) != record.InputHash || json.Unmarshal(raw, &input) != nil || input.Attacker.GetPlayerId() != record.PlayerID || input.Seed != record.Seed || input.ConfigurationHash != record.ConfigurationHash || input.EngineVersion != record.EngineVersion || input.ClientVersion != record.ClientVersion {
		return nil, errors.New(constUtil.P6ReplayConflict)
	}
	tables, err := json.Marshal(input.Tables)
	if err != nil || p6Digest(tables) != input.ConfigurationHash || validateFrozenP6(&input) != nil {
		return nil, errors.New(constUtil.P6ReplayConflict)
	}
	return &input, nil
}

func p6ReplayTableHashEnvelope(tb testing.TB, raw []byte) map[string]json.RawMessage {
	tb.Helper()
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(raw, &envelope); err != nil {
		tb.Fatal(err)
	}
	if len(envelope["tables"]) == 0 {
		tb.Fatal("frozen input has no tables")
	}
	return envelope
}

func TestP6ReplayTableHashCompatibility(t *testing.T) {
	frozen, canonical, compressed := p6ReplayTableHashFixture(t)
	canonicalEnvelope := p6ReplayTableHashEnvelope(t, canonical)
	if p6Digest(canonicalEnvelope["tables"]) != frozen.ConfigurationHash {
		t.Fatal("canonical fixture tables do not match their configuration hash")
	}

	var indented bytes.Buffer
	if err := json.Indent(&indented, canonical, "", "  "); err != nil {
		t.Fatal(err)
	}
	var tableValues map[string]json.RawMessage
	if err := json.Unmarshal(canonicalEnvelope["tables"], &tableValues); err != nil {
		t.Fatal(err)
	}
	keys := make([]string, 0, len(tableValues))
	for key := range tableValues {
		keys = append(keys, key)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(keys)))
	var reversed bytes.Buffer
	reversed.WriteByte('{')
	for i, key := range keys {
		if i > 0 {
			reversed.WriteByte(',')
		}
		quoted, err := json.Marshal(key)
		if err != nil {
			t.Fatal(err)
		}
		reversed.Write(quoted)
		reversed.WriteByte(':')
		reversed.Write(tableValues[key])
	}
	reversed.WriteByte('}')
	reorderedEnvelope := p6ReplayTableHashEnvelope(t, canonical)
	reorderedEnvelope["tables"] = reversed.Bytes()
	reordered, err := json.Marshal(reorderedEnvelope)
	if err != nil {
		t.Fatal(err)
	}
	unicodeEnvelope := p6ReplayTableHashEnvelope(t, canonical)
	unicodeTables := bytes.Replace(unicodeEnvelope["tables"], []byte("浮"), []byte(`\u6d6e`), 1)
	if bytes.Equal(unicodeTables, unicodeEnvelope["tables"]) {
		t.Fatal("Unicode compatibility fixture did not escape the original table value")
	}
	unicodeEnvelope["tables"] = unicodeTables
	unicodeRaw, err := json.Marshal(unicodeEnvelope)
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name      string
		raw       []byte
		stored    []byte
		canonical bool
	}{
		{"canonical", canonical, canonical, true},
		{"compressed", canonical, compressed, true},
		{"indented", indented.Bytes(), indented.Bytes(), false},
		{"reordered", reordered, reordered, false},
		{"unicode-escaped", unicodeRaw, unicodeRaw, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			envelope := p6ReplayTableHashEnvelope(t, tc.raw)
			if matches := p6Digest(envelope["tables"]) == frozen.ConfigurationHash; matches != tc.canonical {
				t.Fatalf("table byte digest match=%t, want %t", matches, tc.canonical)
			}
			record := p6ReplayTableHashRecord(frozen, tc.raw, tc.stored)
			for _, decode := range []struct {
				name string
				fn   func(*db.P6BattleRecord) (*model.FrozenBattleInput, error)
			}{{"baseline", p6ReplayTableHashBaseline}, {"current", decodeP6Record}} {
				got, err := decode.fn(&record)
				if err != nil {
					t.Fatalf("%s: %v", decode.name, err)
				}
				encoded, err := json.Marshal(got)
				if err != nil || !bytes.Equal(encoded, canonical) {
					t.Fatalf("%s changed frozen input semantics: %v", decode.name, err)
				}
			}
		})
	}

	tampered := p6ReplayTableHashEnvelope(t, canonical)
	var changedTables map[string][]map[string]string
	if err := json.Unmarshal(tampered["tables"], &changedTables); err != nil {
		t.Fatal(err)
	}
	if len(changedTables["Hero"]) == 0 {
		t.Fatal("missing original Hero table")
	}
	changedTables["Hero"][0]["id"] += "-tampered"
	tampered["tables"], err = json.Marshal(changedTables)
	if err != nil {
		t.Fatal(err)
	}
	tamperedRaw, err := json.Marshal(tampered)
	if err != nil {
		t.Fatal(err)
	}
	badTable := p6ReplayTableHashRecord(frozen, tamperedRaw, tamperedRaw)
	if _, err := decodeP6Record(&badTable); err == nil {
		t.Fatal("changed table accepted after recomputing the outer input hash")
	}
	if _, err := p6ReplayTableHashBaseline(&badTable); err == nil {
		t.Fatal("baseline accepted changed table")
	}
	badIdentity := p6ReplayTableHashRecord(frozen, canonical, compressed)
	badIdentity.PlayerID = 43
	if _, err := decodeP6Record(&badIdentity); err == nil {
		t.Fatal("record with another player identity was accepted")
	}
	if _, err := p6ReplayTableHashBaseline(&badIdentity); err == nil {
		t.Fatal("baseline accepted another player identity")
	}

	record := p6ReplayTableHashRecord(frozen, canonical, compressed)
	first, err := decodeP6Record(&record)
	if err != nil {
		t.Fatal(err)
	}
	second, err := decodeP6Record(&record)
	if err != nil {
		t.Fatal(err)
	}
	first.Tables["Hero"][0]["id"] = "changed-only-in-first-decode"
	first.Attacker.Formation[0].Prop.Hp = proto.Float64(1)
	for _, value := range []*model.FrozenBattleInput{second, frozen} {
		encoded, err := json.Marshal(value)
		if err != nil || !bytes.Equal(encoded, canonical) {
			t.Fatalf("decoded inputs share mutable data: %v", err)
		}
	}
	third, err := decodeP6Record(&record)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(third)
	if err != nil || !bytes.Equal(encoded, canonical) {
		t.Fatalf("later decode was contaminated: %v", err)
	}
}

func BenchmarkP6ReplayTableHash(b *testing.B) {
	frozen, raw, stored := p6ReplayTableHashFixture(b)
	record := p6ReplayTableHashRecord(frozen, raw, stored)
	for _, tc := range []struct {
		name string
		fn   func(*db.P6BattleRecord) (*model.FrozenBattleInput, error)
	}{{"baseline", p6ReplayTableHashBaseline}, {"current", decodeP6Record}} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(stored)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := tc.fn(&record); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
