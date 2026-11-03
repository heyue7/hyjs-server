package service

import (
	"encoding/base64"
	"encoding/json"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func nativePowerTitle(t *testing.T, effects string, deadline int64) string {
	t.Helper()
	raw, err := json.Marshal(p5TitleState{Effects: effects, DeadTime: deadline, Source: "isolated frozen title fixture"})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestNativeTitlePowerOwnedSpaceClockAndPrecision(t *testing.T) {
	s := &GameService{Tables: map[string][]map[string]string{
		"Title": {
			{"titleId": "general", "type": "1", "effects": "att,9999"},
			{"titleId": "sao", "type": "2"},
			{"titleId": "alo", "type": "3"},
			{"titleId": "ggo", "type": "4"},
			{"titleId": "timed", "type": "5"},
			{"titleId": "expired", "type": "6"},
		},
		"Hero": {
			{"id": "hero1", "space": "1"},
			{"id": "hero2", "space": "2"},
			{"id": "hero3", "space": "3"},
			{"id": "hero4", "space": "4"},
		},
	}}
	now := time.Now()
	g := &model.GameState{}
	p5SetKV(g, "p5.current-title", "alo")
	p5SetKV(g, "p5.title:general", nativePowerTitle(t, "att,1;damageAdd,0.1", 0))
	p5SetKV(g, "p5.title:sao", nativePowerTitle(t, "att,10", 0))
	p5SetKV(g, "p5.title:alo", nativePowerTitle(t, "att,20", 0))
	p5SetKV(g, "p5.title:ggo", nativePowerTitle(t, "att,40", 0))
	p5SetKV(g, "p5.title:timed", nativePowerTitle(t, "hp,1.25", now.Add(12*time.Hour).UnixMilli()))
	p5SetKV(g, "p5.title:expired", nativePowerTitle(t, "hp,1000", now.Add(-time.Hour).UnixMilli()))
	// Unknown IDs are ignored and duplicate keys retain p5KV's first-record rule.
	p5SetKV(g, "p5.title:unknown", nativePowerTitle(t, "att,10000", 0))
	g.KV = append(g.KV, &pb.KeyValueEntry{Key: proto.String("p5.title:general"), Value: proto.String(nativePowerTitle(t, "att,10000", 0))})

	for _, tt := range []struct {
		hero string
		att  float64
	}{{"hero1", 11}, {"hero2", 21}, {"hero3", 41}, {"hero4", 1}} {
		t.Run(tt.hero, func(t *testing.T) {
			before, err := json.Marshal(g)
			if err != nil {
				t.Fatal(err)
			}
			values := map[string]float64{}
			if err := s.p5TitleBonus(g, values, tt.hero); err != nil {
				t.Fatal(err)
			}
			if values["att"] != tt.att || values["hp"] != 1.25 || values["damageAdd"] != float64(float32(0.1)) {
				t.Fatalf("owned and space bonuses: %+v", values)
			}
			after, err := json.Marshal(g)
			if err != nil || string(after) != string(before) {
				t.Fatalf("title calculation changed player state: %v", err)
			}
		})
	}
	// Changing the worn caption does not change the attributes.
	p5SetKV(g, "p5.current-title", "general")
	values := map[string]float64{}
	if err := s.p5TitleBonus(g, values, "hero2"); err != nil || values["att"] != 21 || values["hp"] != 1.25 {
		t.Fatalf("worn title affected attributes: %v %+v", err, values)
	}
	s.ServerClockReady = true
	s.ServerClockState.Store(&model.ServerGameClockView{OffsetDays: 1})
	values = map[string]float64{}
	if err := s.p5TitleBonus(g, values, "hero2"); err != nil || values["att"] != 21 || values["hp"] != 0 {
		t.Fatalf("business clock did not expire title: %v %+v", err, values)
	}
}

func TestNativeTitlePowerRejectsInvalidOwnedEffect(t *testing.T) {
	s := &GameService{Tables: map[string][]map[string]string{"Title": {{"titleId": "valid", "type": "1"}}}}
	blank := &model.GameState{KV: []*pb.KeyValueEntry{
		{Key: proto.String("p5.title:valid"), Value: proto.String("")},
		{Key: proto.String("p5.title:valid"), Value: proto.String(nativePowerTitle(t, "att,100", 0))},
	}}
	values := map[string]float64{}
	if err := s.p5TitleBonus(blank, values, ""); err != nil || values["att"] != 0 {
		t.Fatalf("empty first record should be unowned: %v %+v", err, values)
	}
	for _, effect := range []string{"att,-1", "att,NaN", "att,+Inf", "att,1e100", "unknown,1", "att", "att,1;"} {
		t.Run(effect, func(t *testing.T) {
			g := &model.GameState{}
			p5SetKV(g, "p5.title:valid", nativePowerTitle(t, effect, 0))
			if err := s.p5TitleBonus(g, map[string]float64{}, ""); err == nil {
				t.Fatalf("accepted invalid owned effect %q", effect)
			}
		})
	}
	for _, raw := range []string{"{", `{"effects":"att,1","source":""}`} {
		g := &model.GameState{}
		p5SetKV(g, "p5.title:valid", raw)
		if err := s.p5TitleBonus(g, map[string]float64{}, ""); err == nil {
			t.Fatalf("accepted invalid title snapshot %q", raw)
		}
	}
}

func nativePowerTreasureFixture(strength1, strength2, refine1, refine2 int32) (*GameService, *model.GameState, *pb.FormationItem) {
	s := &GameService{Tables: map[string][]map[string]string{
		"Treasure": {
			{"id": "ring", "type": "1", "maxStrengthen": "100", "maxRefine": "30", "hp": "10", "att": "1"},
			{"id": "necklace", "type": "0", "maxStrengthen": "100", "maxRefine": "30", "hp": "20", "att": "2"},
		},
		"TreasureStrengthen": {{"treasureId": "ring"}, {"treasureId": "necklace"}},
		"TreasureRefine":     {{"treasureId": "ring"}, {"treasureId": "necklace"}},
		// Deliberately unordered, including other equipment master types.
		"StrengthenMaster": {
			{"type": "3", "level": "2", "strengthenLevel": "20", "hp": "200"},
			{"type": "4", "level": "3", "strengthenLevel": "3", "att": "30"},
			{"type": "3", "level": "1", "strengthenLevel": "10", "hp": "100"},
			{"type": "4", "level": "2", "strengthenLevel": "2", "att": "20"},
			{"type": "3", "level": "3", "strengthenLevel": "30", "hp": "300"},
			{"type": "4", "level": "1", "strengthenLevel": "1", "att": "10"},
			{"type": "1", "level": "100", "strengthenLevel": "1", "hp": "10000"},
			{"type": "2", "level": "100", "strengthenLevel": "0", "att": "10000"},
		},
	}}
	slot := &pb.FormationItem{Things: []*pb.FormationThing{
		{Guid: proto.String("ring-guid"), Id: proto.String("ring"), Pos: proto.Int32(5)},
		{Guid: proto.String("necklace-guid"), Id: proto.String("necklace"), Pos: proto.Int32(6)},
	}}
	g := &model.GameState{Formation: []*pb.FormationItem{slot}, Treasures: []*pb.Treasure{
		{Guid: proto.String("ring-guid"), TreasureId: proto.String("ring"), StrengthLevel: proto.Int32(strength1), RefineLevel: proto.Int32(refine1)},
		{Guid: proto.String("necklace-guid"), TreasureId: proto.String("necklace"), StrengthLevel: proto.Int32(strength2), RefineLevel: proto.Int32(refine2)},
	}}
	return s, g, slot
}

func TestNativeTreasureMasterTwoSlotsAndMinimumLevels(t *testing.T) {
	for _, tt := range []struct {
		name                 string
		strength1, strength2 int32
		refine1, refine2     int32
		slots                int
		hp, att              float64
	}{
		{"highest reached from each minimum", 30, 20, 4, 2, 2, 230, 23},
		{"below next thresholds", 30, 19, 4, 1, 2, 130, 13},
		{"below first thresholds", 30, 9, 4, 0, 2, 30, 3},
		{"only ring", 30, 30, 4, 4, 1, 10, 1},
		{"no slots", 30, 30, 4, 4, 0, 0, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s, g, slot := nativePowerTreasureFixture(tt.strength1, tt.strength2, tt.refine1, tt.refine2)
			slot.Things = slot.Things[:tt.slots]
			before, err := json.Marshal(g)
			if err != nil {
				t.Fatal(err)
			}
			values, percent := map[string]float64{}, map[string]float64{}
			if err := s.treasureFormationProperties(g, slot, values, percent); err != nil {
				t.Fatal(err)
			}
			if values["hp"] != tt.hp || values["att"] != tt.att {
				t.Fatalf("master values: got %+v, want hp=%v att=%v", values, tt.hp, tt.att)
			}
			after, err := json.Marshal(g)
			if err != nil || string(after) != string(before) {
				t.Fatalf("treasure calculation changed player state: %v", err)
			}
		})
	}
	for _, kind := range []string{"duplicate-guid", "wrong-slot", "wrong-id", "missing-record", "invalid-level"} {
		t.Run(kind, func(t *testing.T) {
			s, g, slot := nativePowerTreasureFixture(30, 20, 4, 2)
			switch kind {
			case "duplicate-guid":
				slot.Things[1].Guid = proto.String("ring-guid")
			case "wrong-slot":
				slot.Things[1].Pos = proto.Int32(5)
			case "wrong-id":
				slot.Things[1].Id = proto.String("ring")
			case "missing-record":
				g.Treasures = g.Treasures[:1]
			case "invalid-level":
				g.Treasures[1].StrengthLevel = proto.Int32(math.MaxInt32)
			}
			if err := s.treasureFormationProperties(g, slot, map[string]float64{}, map[string]float64{}); err == nil {
				t.Fatalf("accepted %s treasure state", kind)
			}
		})
	}
}

// TestLeaderboardPowerNativeFixture is an opt-in, database-free comparison probe.
// Its input and output stay private and the normal test suite does not invoke it.
func TestLeaderboardPowerNativeFixture(t *testing.T) {
	path := os.Getenv("HYJS_POWER_PROBE_FILE")
	if path == "" {
		t.Skip("private finite growth input not requested")
	}
	var input struct {
		Profile struct {
			Player string          `json:"player"`
			Game   json.RawMessage `json:"game"`
		} `json:"profile"`
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("private input unavailable")
	}
	if err = json.Unmarshal(raw, &input); err != nil {
		t.Fatal("private input is invalid")
	}
	var player pb.Player
	var game model.GameState
	if err = json.Unmarshal([]byte(input.Profile.Player), &player); err != nil {
		t.Fatal("private player is invalid")
	}
	if err = json.Unmarshal(input.Profile.Game, &game); err != nil {
		t.Fatal("private game is invalid")
	}
	PrepareWarriors(&game)
	s := &GameService{}
	for _, load := range []func(string) error{s.LoadTables, s.LoadP5Tables, s.LoadP8Tables, s.LoadP9Tables} {
		if err = load("../../analysis/tables"); err != nil {
			t.Fatal(err)
		}
	}
	if err = s.ApplyP15GreenHeroCompatibility(); err != nil {
		t.Fatal(err)
	}
	s.InitializeGameTableLookup()
	game.GuildPrayerProjection = &model.P5GuildPrayerProjection{PlayerID: player.GetPlayerId(), CapturedAt: time.Now().UnixMilli(), Properties: &pb.BaseProp{}, TechProperties: &pb.BaseProp{}}
	actors, err := s.battleFormationActors(&player, &game)
	if err != nil {
		t.Fatal(err)
	}
	power, err := s.formationFightPowerFromParty(&pb.BattlePlayer{Formation: actors})
	if err != nil {
		t.Fatal(err)
	}
	var pets []*pb.Pet
	seenPets := map[string]bool{}
	for _, slot := range game.Formation {
		for _, thing := range slot.GetThings() {
			if thing.GetPos() == 7 && !seenPets[thing.GetGuid()] {
				seenPets[thing.GetGuid()] = true
				encoded := p5KV(&game, "p9.pet:"+thing.GetGuid())
				petBytes, err := base64.StdEncoding.DecodeString(encoded)
				if err != nil {
					t.Fatal("private pet source is invalid")
				}
				pet := &pb.Pet{}
				if err = proto.Unmarshal(petBytes, pet); err != nil {
					t.Fatal("private pet source is invalid")
				}
				pets = append(pets, pet)
			}
		}
	}
	type titleSource struct {
		ID       string `json:"id"`
		Effects  string `json:"effects"`
		DeadTime int64  `json:"deadTime"`
		Active   bool   `json:"active"`
	}
	var titles []titleSource
	seenTitles := map[string]bool{}
	now := time.Now().UnixMilli()
	for _, entry := range game.KV {
		if !strings.HasPrefix(entry.GetKey(), "p5.title:") {
			continue
		}
		id := strings.TrimPrefix(entry.GetKey(), "p5.title:")
		if seenTitles[id] {
			continue
		}
		seenTitles[id] = true
		if s.table("Title", "titleId", id) == nil {
			continue
		}
		if entry.GetValue() == "" {
			continue
		}
		var title p5TitleState
		if json.Unmarshal([]byte(entry.GetValue()), &title) != nil || title.Source == "" {
			t.Fatal("private title source is invalid")
		}
		titles = append(titles, titleSource{ID: id, Effects: title.Effects, DeadTime: title.DeadTime, Active: title.DeadTime <= 0 || title.DeadTime > now})
	}
	native := map[string]any{"equipment": game.Equipment, "treasures": game.Treasures, "warriors": game.Warriors, "formation": game.Formation, "helpers": P5HelperList(&game), "pets": pets, "tacticalSkill": game.TacticalSkillId, "titles": titles}
	output := struct {
		WithoutGuildPower int32            `json:"withoutGuildPower"`
		Actors            []*pb.BattleHero `json:"actors"`
		Native            map[string]any   `json:"native"`
	}{power, actors, native}
	encoded, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		t.Fatal("private output encoding failed")
	}
	file, err := os.OpenFile(path+".actors.json", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		t.Fatal("private output unavailable")
	}
	if err = file.Chmod(0600); err != nil {
		file.Close()
		t.Fatal("private output permissions failed")
	}
	if _, err = file.Write(encoded); err != nil {
		file.Close()
		t.Fatal("private output write failed")
	}
	if err = file.Close(); err != nil {
		t.Fatal("private output close failed")
	}
}
