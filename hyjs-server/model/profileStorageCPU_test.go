package model

import (
	"bytes"
	"encoding/json"
	"testing"

	"google.golang.org/protobuf/proto"
)

// BenchmarkProfileStorage86 measures the same large roster used by the
// persistence compatibility tests, without measuring fixture construction.
func BenchmarkProfileStorage86(b *testing.B) {
	game := profileStorageRoster86()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		raw, err := MarshalProfileGame86(game)
		if err != nil || len(raw) == 0 {
			b.Fatal(err)
		}
	}
}

func BenchmarkProfileStorage86Reusing(b *testing.B) {
	game := profileStorageRoster86()
	previous, err := MarshalProfileGame86(game)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		raw, err := MarshalProfileGame86Reusing(game, previous)
		if err != nil || len(raw) == 0 {
			b.Fatal(err)
		}
	}
}

func TestProfileStorage86ReusePreservesAndValidatesRoster(t *testing.T) {
	game := profileStorageRoster86()
	previous, err := MarshalProfileGame86(game)
	if err != nil {
		t.Fatal(err)
	}
	game.Position.Scene = proto.String("town_main")
	got, err := MarshalProfileGame86Reusing(game, previous)
	if err != nil {
		t.Fatal(err)
	}
	want, err := MarshalProfileGame86(game)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("unchanged roster or changed position differs: %v", err)
	}

	game.Warriors[0].Level = proto.Int32(41)
	got, err = MarshalProfileGame86Reusing(game, previous)
	if err != nil {
		t.Fatal(err)
	}
	want, err = MarshalProfileGame86(game)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("changed roster differs: %v", err)
	}
	var restored GameState
	if err := json.Unmarshal(got, &restored); err != nil || restored.Warriors[0].GetLevel() != 41 {
		t.Fatalf("changed roster failed cold recovery: %v", err)
	}

	legacy, err := json.Marshal(game)
	if err != nil {
		t.Fatal(err)
	}
	got, err = MarshalProfileGame86Reusing(game, legacy)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("legacy roster changed: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(previous, &fields); err != nil {
		t.Fatal(err)
	}
	var envelope storedWarriors86
	if err := json.Unmarshal(fields["warriors"], &envelope); err != nil {
		t.Fatal(err)
	}
	envelope.SHA256 = string(bytes.Repeat([]byte{'0'}, 64))
	fields["warriors"], _ = json.Marshal(envelope)
	corrupt, _ := json.Marshal(fields)
	if _, err := MarshalProfileGame86Reusing(game, corrupt); err == nil {
		t.Fatal("damaged prior roster was accepted")
	}
}
