package model

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	pb "hyjs-server/model/protocol"
)

func profileStorageRoster86() *GameState {
	g := &GameState{Position: &pb.Position{Scene: proto.String("field_003"), Current: &pb.PointNew{X: proto.Float32(1), Z: proto.Float32(2)}}, GGOField: &GGOFieldState{}}
	for i := 0; i < 400; i++ {
		g.Warriors = append(g.Warriors, &pb.Warrior{Id: proto.String("hero_001"), Guid: proto.String(fmt.Sprintf("hero-%04d", i)), Level: proto.Int32(40), Exp: proto.Int64(100)})
	}
	return g
}

func TestProfileStorage86RoundTripLegacyExportAndJSONPaths(t *testing.T) {
	g := profileStorageRoster86()
	legacy, err := json.Marshal(g)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := MarshalProfileGame86(g)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(stored, []byte(constUtil.ProfileWarriorsCodec86)) || len(stored)*3 >= len(legacy) {
		t.Fatalf("compression not effective: %d -> %d", len(legacy), len(stored))
	}
	for _, raw := range [][]byte{legacy, stored} {
		var cold GameState
		if err := json.Unmarshal(raw, &cold); err != nil {
			t.Fatal(err)
		}
		if len(cold.Warriors) != len(g.Warriors) || !proto.Equal(cold.Position, g.Position) {
			t.Fatal("cold roster or position changed")
		}
		for i := range g.Warriors {
			if !proto.Equal(cold.Warriors[i], g.Warriors[i]) {
				t.Fatal("hero changed")
			}
		}
		exported, err := json.Marshal(&cold)
		if err != nil || !bytes.Equal(exported, legacy) {
			t.Fatal("ordinary JSON export shape changed")
		}
		again, err := MarshalProfileGame86(&cold)
		if err != nil || !bytes.Equal(again, stored) {
			t.Fatal("unchanged save is not byte stable")
		}
	}
	var before, after map[string]json.RawMessage
	json.Unmarshal(legacy, &before)
	json.Unmarshal(stored, &after)
	delete(before, "warriors")
	delete(after, "warriors")
	if !reflect.DeepEqual(before, after) {
		t.Fatal("non-roster query fields changed")
	}
	t.Logf("roster representative game bytes %d -> %d", len(legacy), len(stored))
}

func TestProfileStorage86RawUnknownFieldsAndRollback(t *testing.T) {
	legacy, _ := json.Marshal(profileStorageRoster86())
	var fields map[string]json.RawMessage
	json.Unmarshal(legacy, &fields)
	var roster []map[string]json.RawMessage
	json.Unmarshal(fields["warriors"], &roster)
	roster[0]["futureHeroData"] = json.RawMessage(`{"value":9007199254740993}`)
	fields["warriors"], _ = json.Marshal(roster)
	fields["futureTopLevel"] = json.RawMessage(`{"unchanged":true}`)
	legacy, _ = json.Marshal(fields)
	stored, err := CompactProfileGame86(legacy)
	if err != nil {
		t.Fatal(err)
	}
	back, err := ExpandProfileGame86(stored)
	if err != nil || !bytes.Equal(back, legacy) {
		t.Fatal("legacy export lost unknown fields or exact integer")
	}
	again, err := CompactProfileGame86(stored)
	if err != nil || !bytes.Equal(again, stored) {
		t.Fatal("already compressed input changed")
	}
	// A pre-codec reader can read the exported legacy format after maintenance.
	type legacyGameState GameState
	var old legacyGameState
	if err := json.Unmarshal(back, &old); err != nil || len(old.Warriors) != 400 {
		t.Fatal("legacy rollback export is unreadable")
	}
}

func TestProfileStorage86CorruptionAndBoundsRejected(t *testing.T) {
	stored, _ := MarshalProfileGame86(profileStorageRoster86())
	var fields map[string]json.RawMessage
	json.Unmarshal(stored, &fields)
	var original storedWarriors86
	json.Unmarshal(fields["warriors"], &original)
	for _, name := range []string{"digest", "length", "oversized", "codec", "truncated", "trailing"} {
		t.Run(name, func(t *testing.T) {
			e := original
			e.Data = append([]byte(nil), original.Data...)
			switch name {
			case "digest":
				e.SHA256 = string(bytes.Repeat([]byte{'0'}, 64))
			case "length":
				e.Bytes--
			case "oversized":
				e.Bytes = constUtil.ProfileWarriorsMaxBytes86 + 1
			case "codec":
				e.Codec = "unknown"
			case "truncated":
				e.Data = e.Data[:len(e.Data)-1]
			case "trailing":
				e.Data = append(e.Data, 1)
			}
			raw, _ := json.Marshal(e)
			fields["warriors"] = raw
			bad, _ := json.Marshal(fields)
			if _, err := ExpandProfileGame86(bad); err == nil {
				t.Fatal("damaged roster accepted")
			}
			var g GameState
			if err := json.Unmarshal(bad, &g); err == nil {
				t.Fatal("game accepted damaged roster")
			}
		})
	}
}

func TestProfileStorage86OriginalPartialAndNullDecode(t *testing.T) {
	old := &GameState{Warriors: []*pb.Warrior{{Id: proto.String("hero_001"), Guid: proto.String("kept"), Level: proto.Int32(8)}}, Position: &pb.Position{Scene: proto.String("town")}}
	for _, raw := range []string{`{"warriors":[{"level":9}]}`, `{"position_loading":true}`, `null`, `{"warriors":null}`} {
		type plainGameState GameState
		base, _ := json.Marshal(old)
		var expected plainGameState
		var actual GameState
		json.Unmarshal(base, &expected)
		json.Unmarshal(base, &actual)
		err1 := json.Unmarshal([]byte(raw), &expected)
		err2 := json.Unmarshal([]byte(raw), &actual)
		if (err1 == nil) != (err2 == nil) || !reflect.DeepEqual(GameState(expected), actual) {
			t.Fatalf("standard decode changed for %s", raw)
		}
	}
}
