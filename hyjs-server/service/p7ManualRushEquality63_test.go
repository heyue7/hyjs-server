package service

import (
	"bytes"
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"math"
	"testing"
)

func TestP7ManualRushEquality63CanonicalBoundaries(t *testing.T) {
	table := func(rows ...map[string]string) map[string][]map[string]string {
		return map[string][]map[string]string{"Hero": rows}
	}
	input := func(tables map[string][]map[string]string) *model.FrozenBattleInput {
		return &model.FrozenBattleInput{Tables: tables}
	}
	tests := []struct {
		name  string
		a, b  *model.FrozenBattleInput
		equal bool
	}{
		{"both nil", nil, nil, true},
		{"nil input", nil, input(nil), false},
		{"nil versus empty tables", input(nil), input(map[string][]map[string]string{}), false},
		{"nil versus empty rows", input(map[string][]map[string]string{"Hero": nil}), input(map[string][]map[string]string{"Hero": {}}), false},
		{"nil versus empty row", input(table(nil)), input(table(map[string]string{})), false},
		{"independent maps different insertion order", input(table(map[string]string{"id": "hero_047", "skill": "047_1"})), input(table(map[string]string{"skill": "047_1", "id": "hero_047"})), true},
		{"row order", input(table(map[string]string{"id": "a"}, map[string]string{"id": "b"})), input(table(map[string]string{"id": "b"}, map[string]string{"id": "a"})), false},
		{"column changed", input(table(map[string]string{"id": "a"})), input(table(map[string]string{"id": "b"})), false},
		{"extra column", input(table(map[string]string{"id": "a"})), input(table(map[string]string{"id": "a", "other": ""})), false},
		{"invalid UTF8 table fallback", input(table(map[string]string{"id": string([]byte{255})})), input(table(map[string]string{"id": "�"})), true},
		{"invalid UTF8 table keys fallback", input(table(map[string]string{string([]byte{255}): "x"})), input(table(map[string]string{"�": "x"})), true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a, _ := json.Marshal(tc.a)
			b, _ := json.Marshal(tc.b)
			legacy := bytes.Equal(a, b)
			if legacy != tc.equal {
				t.Fatal("legacy contract differs from fixture", string(a), string(b))
			}
			if p7ManualRushInputsEqual(tc.a, tc.b) != legacy {
				t.Fatal("comparison changed legacy JSON equality")
			}
			afterA, _ := json.Marshal(tc.a)
			afterB, _ := json.Marshal(tc.b)
			if !bytes.Equal(a, afterA) || !bytes.Equal(b, afterB) {
				t.Fatal("comparison mutated source")
			}
		})
	}
}

func TestP7ManualRushEquality63RetainsMetadataAndMarshalSemantics(t *testing.T) {
	for _, variant := range []string{"seed", "protobuf-presence", "omitempty", "metadata-invalid-utf8", "marshal-failure"} {
		t.Run(variant, func(t *testing.T) {
			a := &model.FrozenBattleInput{Tables: map[string][]map[string]string{"Hero": {{"id": "hero_047"}}}, Attacker: &pb.BattlePlayer{PlayerId: proto.Int64(3)}}
			b := *a
			b.Tables = map[string][]map[string]string{"Hero": {{"id": "hero_047"}}}
			b.Attacker = proto.Clone(a.Attacker).(*pb.BattlePlayer)
			switch variant {
			case "seed":
				b.Seed = 7
			case "protobuf-presence":
				a.Attacker.PlayerId = nil
				b.Attacker.PlayerId = proto.Int64(0)
			case "omitempty":
				b.AppliedProperties = map[string][]string{}
				b.WinConditions = []*pb.BattleWinCondition{}
			case "metadata-invalid-utf8":
				a.ClientVersion = string([]byte{255})
				b.ClientVersion = "�"
			case "marshal-failure":
				a.AttackerDamageFactor = proto.Float64(math.NaN())
				b.DefenderDamageFactor = proto.Float64(math.Inf(1))
			}
			ra, _ := json.Marshal(a)
			rb, _ := json.Marshal(&b)
			if p7ManualRushInputsEqual(a, &b) != bytes.Equal(ra, rb) {
				t.Fatal("original metadata JSON/error behavior changed")
			}
		})
	}
	// This equality helper does not replace authoritative source admission.
	series, _, _ := p7Admission63Fixture(t)
	series.Template.Attacker.Formation[0].Prop.Hp = proto.Float64(math.NaN())
	if _, err := newP7GenericMechanismRushAdmission(series); err == nil {
		t.Fatal("invalid source bypassed strict proof")
	}
}
