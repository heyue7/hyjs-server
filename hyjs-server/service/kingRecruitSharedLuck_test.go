package service

import (
	"encoding/json"
	"hyjs-server/model"
	"testing"
)

func TestKingSharedLuckMigration(t *testing.T) {
	cases := []struct {
		name    string
		old     map[int32]int32
		legacy  int32
		want    int32
		invalid bool
	}{
		{"sum", map[int32]int32{1: 490, 2: 730, 3: 0}, 99, 1220, false},
		{"legacy", nil, 80, 80, false},
		{"negative", map[int32]int32{1: -1}, 0, 0, true},
		{"key", map[int32]int32{4: 1}, 0, 0, true},
		{"overflow", map[int32]int32{1: 2147483647, 2: 1}, 0, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := &model.GameState{KingRecruitLuck: tc.old}
			got, err := kingWorldLuck(g, 1, tc.legacy)
			if tc.invalid {
				if err == nil || g.KingRecruitSharedLuck != nil {
					t.Fatal("invalid migration accepted")
				}
				return
			}
			if err != nil || got != tc.want || g.KingRecruitLuck != nil {
				t.Fatal(got, err)
			}
			raw, err := json.Marshal(g)
			if err != nil {
				t.Fatal(err)
			}
			var recovered model.GameState
			if err = json.Unmarshal(raw, &recovered); err != nil {
				t.Fatal(err)
			}
			for world := int32(1); world <= 3; world++ {
				got, err = kingWorldLuck(&recovered, world, 999)
				if err != nil || got != tc.want {
					t.Fatal("cold retry", got, err)
				}
			}
		})
	}
}
