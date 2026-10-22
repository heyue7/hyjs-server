package service

import (
	"encoding/json"
	"hyjs-server/model"
	"testing"
)

func TestGGOSeedPreservesExistingConfiguration(t *testing.T) {
	original := `{"scene":"field_003","minLevel":75,"custom":{"keep":true}}`
	raw, changed, err := addGGOFieldConfiguration(original)
	if err != nil || !changed {
		t.Fatalf("seed: %v %v", changed, err)
	}
	var cfg model.AdminFieldConfiguration
	if err = json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.MinLevel != 75 || cfg.GGO == nil || len(cfg.GGO.NPCs) != 4 || len(cfg.GGO.Monsters) != 5 || len(cfg.GGO.Bosses) != 3 {
		t.Fatal("seed or existing fields lost")
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal([]byte(raw), &fields)
	if string(fields["custom"]) != `{"keep":true}` {
		t.Fatal("unknown existing field lost")
	}
	cfg.GGO.NPCs[1].Dialogue = "用户编辑"
	fields["ggo"], _ = json.Marshal(cfg.GGO)
	edited, _ := json.Marshal(fields)
	repeated, changed, err := addGGOFieldConfiguration(string(edited))
	if err != nil || changed || repeated != string(edited) {
		t.Fatal("existing user configuration overwritten")
	}
	for _, npc := range cfg.GGO.NPCs {
		if npc.Position != nil {
			t.Fatal("fabricated coordinate")
		}
	}
	for _, chip := range cfg.GGO.Chips {
		if chip.ExchangeValue != nil {
			t.Fatal("fabricated exchange value")
		}
	}
	for _, npc := range cfg.GGO.NPCs[1:] {
		if npc.Role != "settlement" || !npc.SettlesChips {
			t.Fatal("settlement relationship lost")
		}
	}
}
