package service

import "testing"

func TestCopyFragmentTargetPolicy73(t *testing.T) {
	s := &GameService{Tables: map[string][]map[string]string{
		"Equip":    {{"id": "equip_001", "quality": "20"}, {"id": "equip_002", "quality": "23"}},
		"Treasure": {{"id": "treasure_001", "quality": "6"}, {"id": "treasure_002", "quality": "5"}},
	}}
	for _, tc := range []struct {
		id   string
		want bool
	}{
		{"equip_001,1", false}, {"frag_equip_001,1", true}, {"frag_equip_002,1", false},
		{"frag_treasure_001,1", false}, {"frag_treasure_002,1", true}, {"frag_equip_missing,1", false},
		{"frag_hero_032,1", true}, {"item_001,1", false},
	} {
		if got := s.copyFragmentAllowed(tc.id); got != tc.want {
			t.Fatalf("%s allowed=%v", tc.id, got)
		}
	}
}
