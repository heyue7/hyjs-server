package model

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	pb "hyjs-server/model/protocol"
)

// profileRound2OldGameState is the previous complete decoder, retained only
// for equal-input compatibility checks and benchmarks.
type profileRound2OldGameState GameState

func (game *profileRound2OldGameState) UnmarshalJSON(raw []byte) error {
	type plainGameState GameState
	if bytes.Contains(raw, []byte(constUtil.ProfileWarriorsCodec86)) {
		plain, err := ExpandProfileGame86(raw)
		if err != nil {
			return err
		}
		raw = plain
	}
	return json.Unmarshal(raw, (*plainGameState)(game))
}

func profileRound2PartialGame() *GameState {
	return &GameState{
		Warriors: []*pb.Warrior{
			{Id: proto.String("hero_001"), Guid: proto.String("kept"), Level: proto.Int32(8)},
			{Id: proto.String("hero_002"), Guid: proto.String("second"), Level: proto.Int32(5)},
		},
		Position: &pb.Position{Scene: proto.String("town")},
	}
}

func profileRound2CompressedPartial(t *testing.T) []byte {
	t.Helper()
	plain := []byte(`[{"level":9,"future":"` + strings.Repeat("x", constUtil.ProfileWarriorsCompressMin86) + `"}]`)
	roster, err := compressWarriorsJSON86(plain)
	if err != nil || len(roster) == 0 || roster[0] != '{' {
		t.Fatalf("partial fixture must use compressed roster: %v", err)
	}
	raw, err := json.Marshal(struct {
		Warriors        json.RawMessage `json:"warriors"`
		PositionLoading bool            `json:"position_loading"`
	}{Warriors: roster, PositionLoading: true})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestProfileStorageRound2DecodeCompatibility(t *testing.T) {
	stored, err := MarshalProfileGame86(profileStorageRoster86())
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		raw  []byte
	}{
		{"legacy-partial", []byte(`{"warriors":[{"level":9}],"position_loading":true}`)},
		{"compressed-partial", profileRound2CompressedPartial(t)},
		{"compressed-complete", stored},
		{"missing-with-codec-elsewhere", []byte(`{"position_loading":true,"future":"hyjs-warriors-zlib-v1"}`)},
		{"null-with-codec-elsewhere", []byte(`{"warriors":null,"future":"hyjs-warriors-zlib-v1"}`)},
		{"empty-with-codec-elsewhere", []byte(`{"warriors":[],"future":"hyjs-warriors-zlib-v1"}`)},
		{"whole-null", []byte(`null`)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			expected := profileRound2PartialGame()
			actual := profileRound2PartialGame()
			oldErr := json.Unmarshal(tc.raw, (*profileRound2OldGameState)(expected))
			newErr := json.Unmarshal(tc.raw, actual)
			if (oldErr == nil) != (newErr == nil) || !reflect.DeepEqual(expected, actual) {
				t.Fatalf("decode changed legacy state: old error=%v new error=%v", oldErr, newErr)
			}
			if tc.name == "compressed-partial" && (len(actual.Warriors) != 1 || actual.Warriors[0].GetGuid() != "kept" || actual.Warriors[0].GetId() != "hero_001" || actual.Warriors[0].GetLevel() != 9 || !actual.PositionLoading || actual.Position.GetScene() != "town") {
				t.Fatal("compressed partial decode lost existing warrior or unrelated state")
			}
		})
	}
}

func TestProfileStorageRound2CompressedNonCanonicalCompatibility(t *testing.T) {
	var fixture map[string]json.RawMessage
	if err := json.Unmarshal(profileRound2CompressedPartial(t), &fixture); err != nil {
		t.Fatal(err)
	}
	roster := fixture["warriors"]
	var envelope storedWarriors86
	if err := json.Unmarshal(roster, &envelope); err != nil {
		t.Fatal(err)
	}
	envelope.SHA256 = strings.Repeat("0", 64)
	badRoster, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name    string
		raw     []byte
		wantErr bool
	}{
		{"duplicate-non-roster-key", []byte(`{"position":{"scene":"discarded"},"warriors":` + string(roster) + `,"position":{"current":{"x":3}}}`), false},
		{"uppercase-only-compressed-roster", []byte(`{"Warriors":` + string(roster) + `,"position":{"scene":"other"}}`), true},
		{"uppercase-and-lowercase-roster", []byte(`{"warriors":` + string(roster) + `,"Warriors":[{"guid":"alias"}]}`), false},
		{"damaged-roster-before-outer-mutation", []byte(`{"position":{"scene":"bad"},"warriors":` + string(badRoster) + `}`), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			expected := profileRound2PartialGame()
			actual := profileRound2PartialGame()
			oldErr := json.Unmarshal(tc.raw, (*profileRound2OldGameState)(expected))
			newErr := json.Unmarshal(tc.raw, actual)
			if (oldErr != nil) != tc.wantErr || (newErr != nil) != tc.wantErr || !reflect.DeepEqual(expected, actual) {
				t.Fatalf("noncanonical decode changed: old error=%v new error=%v", oldErr, newErr)
			}
			if tc.name == "duplicate-non-roster-key" && (actual.Position.GetScene() != "town" || actual.Position.GetCurrent().GetX() != 3) {
				t.Fatal("duplicate keys did not retain the final object and prior absent fields")
			}
			if tc.name == "damaged-roster-before-outer-mutation" && actual.Position.GetScene() != "town" {
				t.Fatal("damaged roster changed destination before rejection")
			}
		})
	}
}

func TestProfileStorageRound2IndependentChangesAndPreviousValidation(t *testing.T) {
	previous, err := MarshalProfileGame86(profileStorageRoster86())
	if err != nil {
		t.Fatal(err)
	}
	var first, second GameState
	if err := json.Unmarshal(previous, &first); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(previous, &second); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("independent decodes differ")
	}
	first.Warriors[0].Level = proto.Int32(41)
	second.Position.Scene = proto.String("town_main")
	for _, state := range []*GameState{&first, &second} {
		got, err := MarshalProfileGame86Reusing(state, previous)
		if err != nil {
			t.Fatal(err)
		}
		want, err := MarshalProfileGame86(state)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("reused save differs from complete save: %v", err)
		}
		var cold GameState
		if err := json.Unmarshal(got, &cold); err != nil || !reflect.DeepEqual(cold, *state) {
			t.Fatalf("cold save differs: %v", err)
		}
	}
	if first.Warriors[0].GetLevel() != 41 || second.Warriors[0].GetLevel() != 40 || first.Position.GetScene() != "field_003" || second.Position.GetScene() != "town_main" {
		t.Fatal("independent game objects affected each other")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(previous, &fields); err != nil {
		t.Fatal(err)
	}
	var envelope storedWarriors86
	if err := json.Unmarshal(fields["warriors"], &envelope); err != nil {
		t.Fatal(err)
	}
	envelope.SHA256 = strings.Repeat("0", 64)
	fields["warriors"], _ = json.Marshal(envelope)
	corrupt, _ := json.Marshal(fields)
	if _, err := MarshalProfileGame86Reusing(&second, corrupt); err == nil {
		t.Fatal("damaged previous roster was accepted")
	}
}

var profileRound2BenchmarkSink []byte

func BenchmarkProfileStorageRound2(b *testing.B) {
	stored, err := MarshalProfileGame86(profileStorageRoster86())
	if err != nil {
		b.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		old  bool
		save bool
	}{
		{"decode/old", true, false},
		{"decode/new", false, false},
		{"decode-and-position-save/old", true, true},
		{"decode-and-position-save/new", false, true},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(stored)))
			for i := 0; i < b.N; i++ {
				var game GameState
				if tc.old {
					err = json.Unmarshal(stored, (*profileRound2OldGameState)(&game))
				} else {
					err = json.Unmarshal(stored, &game)
				}
				if err != nil {
					b.Fatal(err)
				}
				if tc.save {
					game.Position.Scene = proto.String("town_main")
					profileRound2BenchmarkSink, err = MarshalProfileGame86Reusing(&game, stored)
					if err != nil {
						b.Fatal(err)
					}
				}
			}
		})
	}
}
