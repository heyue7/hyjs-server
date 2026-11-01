package service

import (
	"bytes"
	"encoding/json"
	"math"
	"reflect"
	"slices"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// The reference retains the production single-row indexes but disables the
// new multi-row group. This exercises the previous scan and JSON power path.
func powerProjectionReference(s *GameService) *GameService {
	ref := s.withRuntimeTables(s.Tables)
	if s.tableLookup == nil {
		return ref
	}
	tables := make(map[string]gameTableLookupTable, len(s.tableLookup.tables))
	for name, entry := range s.tableLookup.tables {
		if name == "TalentAdvenced" {
			entry.groups = nil
		}
		tables[name] = entry
	}
	ref.tableLookup = &gameTableLookup{tables: tables}
	return ref
}

func powerProjectionFixture(t testing.TB) (*GameService, *pb.Player, *model.GameState) {
	t.Helper()
	s := &GameService{}
	for _, load := range []func(string) error{s.LoadTables, s.LoadP5Tables, s.LoadP8Tables, s.LoadP9Tables} {
		if err := load("../../analysis/tables"); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.ApplyP15GreenHeroCompatibility(); err != nil {
		t.Fatal(err)
	}
	s.InitializeGameTableLookup()
	p := &pb.Player{PlayerId: proto.Int64(3), Level: proto.Int32(5), HeroWarId: proto.String("power-main"), CurrentWorldSeedId: proto.Int32(1), Name: proto.String("power-projection-fixture")}
	g := &model.GameState{}
	for i, hero := range []struct {
		id, guid                string
		level, talent, position int32
	}{{"hero_047", "power-main", 1, 1, 3}, {"hero_025", "power-partner", 3, 0, 7}, {"hero_001", "power-third", 1, 0, 9}, {"hero_003", "power-fourth", 1, 0, 13}} {
		w := &pb.Warrior{Id: proto.String(hero.id), Guid: proto.String(hero.guid), Level: proto.Int32(hero.level), Talent: proto.Int32(hero.talent), Training: &pb.Training{}, Awake: &pb.Awake{}, Breached: &pb.Breached{Level: proto.Int32(1)}}
		g.Warriors = append(g.Warriors, w)
		g.Formation = append(g.Formation, &pb.FormationItem{FormatPos: proto.Int32(int32(i + 1)), BattlePos: proto.Int32(hero.position), Things: []*pb.FormationThing{{Pos: proto.Int32(0), Id: w.Id, Guid: w.Guid}}})
	}
	g.Equipment = []*pb.Equipment{{Guid: proto.String("power-weapon"), EquipId: proto.String("equip_001"), Level: proto.Int32(7)}}
	g.Formation[0].Things = append(g.Formation[0].Things, &pb.FormationThing{Pos: proto.Int32(1), Id: proto.String("equip_001"), Guid: proto.String("power-weapon")})
	return s, p, g
}

func powerProjectionClone(t testing.TB, source *model.GameState) *model.GameState {
	t.Helper()
	raw, err := json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	var out model.GameState
	if err = json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	out.GuildPrayerProjection = source.GuildPrayerProjection.Clone()
	return &out
}

func powerProjectionLegacy(s *GameService, player *pb.Player, game *model.GameState) (int32, *pb.BattlePlayer, error) {
	// This is the pre-change formation traversal and BattlePlayer conversion.
	if err := s.PrepareFormation(game); err != nil {
		return 0, nil, err
	}
	var actors []*pb.BattleHero
	for _, slot := range game.Formation {
		for _, thing := range slot.GetThings() {
			if thing.GetPos() != 0 {
				continue
			}
			for _, warrior := range game.Warriors {
				if warrior.GetGuid() == thing.GetGuid() {
					a, err := s.battleWarrior(player, warrior, game, slot)
					if err != nil {
						return 0, nil, err
					}
					actors = append(actors, a)
				}
			}
		}
	}
	message, err := model.GameMessage("BattlePlayer", map[string]any{"playerId": player.GetPlayerId(), "serverId": s.OriginalServerID(), "level": player.GetLevel(), "nickName": player.GetName(), "formation": actors, "inst": map[string]any{"anger": 0}})
	if err != nil {
		return 0, nil, err
	}
	party := message.(*pb.BattlePlayer)
	power, err := s.formationFightPowerFromParty(party)
	return power, party, err
}

func powerProjectionCompare(t *testing.T, s *GameService, player *pb.Player, source *model.GameState, requireSuccess bool) {
	t.Helper()
	legacy := powerProjectionReference(s)
	oldGame, newGame := powerProjectionClone(t, source), powerProjectionClone(t, source)
	oldPlayer, newPlayer := proto.Clone(player).(*pb.Player), proto.Clone(player).(*pb.Player)
	want, party, oldErr := powerProjectionLegacy(legacy, oldPlayer, oldGame)
	got, newErr := s.formationFightPower(newPlayer, newGame)
	if (oldErr == nil) != (newErr == nil) || oldErr != nil && (reflect.TypeOf(oldErr) != reflect.TypeOf(newErr) || oldErr.Error() != newErr.Error()) {
		t.Fatalf("legacy error %v differs from optimized error %v", oldErr, newErr)
	}
	oldJSON, err := json.Marshal(oldGame)
	if err != nil {
		t.Fatal(err)
	}
	newJSON, err := json.Marshal(newGame)
	if err != nil || !bytes.Equal(oldJSON, newJSON) || !proto.Equal(oldPlayer, newPlayer) {
		t.Fatalf("formation preparation changed saved state: %v", err)
	}
	if oldErr != nil {
		if requireSuccess {
			t.Fatalf("fixture unexpectedly failed: %v", oldErr)
		}
		return
	}
	if !requireSuccess {
		t.Fatal("fixture unexpectedly succeeded on both paths")
	}
	if got != want {
		t.Fatalf("power changed: got %d, want %d", got, want)
	}
	newParty, err := s.battlePlayer(proto.Clone(player).(*pb.Player), powerProjectionClone(t, source))
	if err != nil || !proto.Equal(party, newParty) {
		t.Fatalf("full BattlePlayer snapshot changed: %v", err)
	}
	oldWire, oldWireErr := proto.MarshalOptions{Deterministic: true}.Marshal(party)
	newWire, newWireErr := proto.MarshalOptions{Deterministic: true}.Marshal(newParty)
	if oldWireErr != nil || newWireErr != nil || !bytes.Equal(oldWire, newWire) {
		t.Fatalf("full BattlePlayer wire changed: %v / %v", oldWireErr, newWireErr)
	}
	actorGame := powerProjectionClone(t, source)
	actors, err := s.battleFormationActors(proto.Clone(player).(*pb.Player), actorGame)
	if err != nil || len(actors) != len(party.GetFormation()) {
		t.Fatalf("hero projection failed or changed order/length: %v", err)
	}
	for i, actor := range actors {
		wantHero := party.GetFormation()[i]
		oldWire, oldWireErr := proto.MarshalOptions{Deterministic: true}.Marshal(wantHero)
		newWire, newWireErr := proto.MarshalOptions{Deterministic: true}.Marshal(actor)
		if oldWireErr != nil || newWireErr != nil || !proto.Equal(wantHero, actor) || !bytes.Equal(oldWire, newWire) {
			t.Fatalf("hero %d differs after prior JSON projection: %v / %v", i, oldWireErr, newWireErr)
		}
	}
}

func TestPowerProjectionIndexOriginalAndOverlay(t *testing.T) {
	s, p, g := powerProjectionFixture(t)
	powerProjectionCompare(t, s, p, g, true)

	ids := map[string]bool{}
	for _, row := range s.Tables["TalentAdvenced"] {
		ids[row["warId"]] = true
	}
	if len(ids) != 76 || len(s.Tables["TalentAdvenced"]) != 1003 {
		t.Fatal("original talent catalogue changed")
	}
	ordered := make([]string, 0, len(ids))
	for id := range ids {
		ordered = append(ordered, id)
	}
	slices.Sort(ordered)
	for _, id := range ordered {
		t.Run(id, func(t *testing.T) {
			guid := "power-" + id
			w := &pb.Warrior{Id: proto.String(id), Guid: proto.String(guid), Level: proto.Int32(1), Talent: proto.Int32(0), Training: &pb.Training{}, Awake: &pb.Awake{}, Breached: &pb.Breached{Level: proto.Int32(1)}}
			one := &model.GameState{Warriors: []*pb.Warrior{w}, Formation: []*pb.FormationItem{{FormatPos: proto.Int32(1), BattlePos: proto.Int32(3), Things: []*pb.FormationThing{{Pos: proto.Int32(0), Id: w.Id, Guid: w.Guid}}}}}
			player := &pb.Player{PlayerId: proto.Int64(3), Level: proto.Int32(1), HeroWarId: proto.String(guid)}
			powerProjectionCompare(t, s, player, one, true)
		})
	}
	for _, id := range []string{"hero_056", "hero_057", "hero_058", "hero_059", "hero_060", "hero_061"} {
		t.Run("green-"+id, func(t *testing.T) {
			guid := "power-" + id
			w := &pb.Warrior{Id: proto.String(id), Guid: proto.String(guid), Level: proto.Int32(1), Training: &pb.Training{}, Awake: &pb.Awake{}, Breached: &pb.Breached{Level: proto.Int32(1)}}
			one := &model.GameState{Warriors: []*pb.Warrior{w}, Formation: []*pb.FormationItem{{FormatPos: proto.Int32(1), BattlePos: proto.Int32(3), Things: []*pb.FormationThing{{Pos: proto.Int32(0), Id: w.Id, Guid: w.Guid}}}}}
			player := &pb.Player{PlayerId: proto.Int64(3), Level: proto.Int32(1), HeroWarId: proto.String(guid)}
			powerProjectionCompare(t, s, player, one, true)
		})
	}

	t.Run("guild-prayer-tech-and-pet", func(t *testing.T) {
		current := powerProjectionClone(t, g)
		current.GuildPrayerProjection = &model.P5GuildPrayerProjection{PlayerID: p.GetPlayerId(), Properties: &pb.BaseProp{Peratt: proto.Float64(10), Perhp: proto.Float64(5)}, SourceHash: strings.Repeat("a", 64), CapturedAt: 1, ExpiresAt: 2, TechProperties: &pb.BaseProp{Hit: proto.Float64(2)}, TechSourceHash: strings.Repeat("b", 64)}
		pet := &pb.Pet{Id: proto.String("pet_002"), Guid: proto.String("power-pet"), Level: proto.Int32(1), Exp: proto.Int64(0), InitProp: proto.String("PetTalent_11"), Eat: &pb.PetEat{Level: proto.Int32(0), Exp: proto.Int32(0), HdTimeList: []int32{0, 0, 0}}, Breached: &pb.PetBreached{Level: proto.Int32(0)}}
		if err := p9ProjectPet(current, pet); err != nil {
			t.Fatal(err)
		}
		current.Formation[0].Things = append(current.Formation[0].Things, &pb.FormationThing{Id: pet.Id, Guid: pet.Guid, Pos: proto.Int32(7)})
		powerProjectionCompare(t, s, p, current, true)
	})
	t.Run("invalid-guild-source", func(t *testing.T) {
		current := powerProjectionClone(t, g)
		current.GuildPrayerProjection = &model.P5GuildPrayerProjection{PlayerID: p.GetPlayerId() + 1, Properties: &pb.BaseProp{}, CapturedAt: 1}
		powerProjectionCompare(t, s, p, current, false)
	})
	t.Run("empty-formation", func(t *testing.T) {
		powerProjectionCompare(t, s, p, &model.GameState{}, true)
	})
	t.Run("duplicate-guid-preserves-ordered-traversal", func(t *testing.T) {
		current := powerProjectionClone(t, g)
		duplicate := proto.Clone(current.Warriors[2]).(*pb.Warrior)
		duplicate.Guid = proto.String(current.Warriors[0].GetGuid())
		current.Warriors = append(current.Warriors, duplicate)
		powerProjectionCompare(t, s, p, current, true)
	})
	t.Run("missing-hero", func(t *testing.T) {
		current := powerProjectionClone(t, g)
		current.Warriors[0].Id = proto.String("missing-hero")
		powerProjectionCompare(t, s, p, current, false)
	})
	t.Run("missing-talent", func(t *testing.T) {
		current := powerProjectionClone(t, g)
		current.Warriors[0].Talent = proto.Int32(999)
		powerProjectionCompare(t, s, p, current, false)
	})
	t.Run("invalid-formation", func(t *testing.T) {
		current := powerProjectionClone(t, g)
		current.Formation[0].FormatPos = proto.Int32(999)
		powerProjectionCompare(t, s, p, current, false)
	})
	t.Run("invalid-power-factor", func(t *testing.T) {
		row := s.table("GameConfigure", "StaticName", "fightpower_att_factor")
		previous := row["NameValue"]
		row["NameValue"] = "invalid"
		defer func() { row["NameValue"] = previous }()
		powerProjectionCompare(t, s, p, g, false)
	})
	t.Run("invalid-actor-number", func(t *testing.T) {
		row := s.table("Hero", "id", "hero_047")
		previous := row["SkillCastChance"]
		row["SkillCastChance"] = "NaN"
		defer func() { row["SkillCastChance"] = previous }()
		powerProjectionCompare(t, s, p, g, false)
	})
	t.Run("server-id-protocol-overflow", func(t *testing.T) {
		view := s.withRuntimeTables(s.Tables)
		view.ServerID = int64(math.MaxInt32) + 1
		powerProjectionCompare(t, view, p, g, false)
	})
	t.Run("server-id-protocol-maximum", func(t *testing.T) {
		view := s.withRuntimeTables(s.Tables)
		view.ServerID = math.MaxInt32
		powerProjectionCompare(t, view, p, g, true)
	})
	t.Run("replacement-talent-table", func(t *testing.T) {
		overlay := make(map[string][]map[string]string, len(s.Tables))
		for name, rows := range s.Tables {
			overlay[name] = rows
		}
		overlay["TalentAdvenced"] = append([]map[string]string(nil), s.Tables["TalentAdvenced"]...)
		found := false
		for i, row := range overlay["TalentAdvenced"] {
			if row["warId"] != "hero_047" || row["trunck"] != "1" {
				continue
			}
			replacement := make(map[string]string, len(row))
			for key, value := range row {
				replacement[key] = value
			}
			replacement["talentLevel"] = "invalid"
			overlay["TalentAdvenced"][i], found = replacement, true
			break
		}
		if !found {
			t.Fatal("fixture talent not found")
		}
		view := s.withRuntimeTables(overlay)
		if _, indexed := view.indexedTableGroup("TalentAdvenced", "warId", "hero_047"); indexed {
			t.Fatal("replacement table reused original groups")
		}
		powerProjectionCompare(t, view, p, g, false)
		powerProjectionCompare(t, s, p, g, true)
	})
	t.Run("valid-replacement-talent-values", func(t *testing.T) {
		overlay := make(map[string][]map[string]string, len(s.Tables))
		for name, rows := range s.Tables {
			overlay[name] = rows
		}
		overlay["TalentAdvenced"] = append([]map[string]string(nil), s.Tables["TalentAdvenced"]...)
		found := false
		for i, row := range overlay["TalentAdvenced"] {
			if row["warId"] != "hero_047" || row["trunck"] != "1" || row["talentLevel"] != "1" {
				continue
			}
			replacement := make(map[string]string, len(row))
			for key, value := range row {
				replacement[key] = value
			}
			replacement["attConstAdd"] = "999"
			overlay["TalentAdvenced"][i], found = replacement, true
			break
		}
		if !found {
			t.Fatal("fixture talent tier not found")
		}
		view := s.withRuntimeTables(overlay)
		if _, indexed := view.indexedTableGroup("TalentAdvenced", "warId", "hero_047"); indexed {
			t.Fatal("replacement table reused original groups")
		}
		powerProjectionCompare(t, view, p, g, true)
		basePower, baseErr := s.formationFightPower(p, powerProjectionClone(t, g))
		overlayPower, overlayErr := view.formationFightPower(p, powerProjectionClone(t, g))
		if baseErr != nil || overlayErr != nil || basePower == overlayPower {
			t.Fatalf("replacement values did not reach power: %d/%v vs %d/%v", basePower, baseErr, overlayPower, overlayErr)
		}
	})
}

func TestPowerProjectionIndexKeepsOrderedRowsAndFallback(t *testing.T) {
	rows := []map[string]string{{"warId": "a", "value": "first"}, {"warId": "b"}, {"warId": "a", "value": "second"}, nil, {"value": "empty"}, {"warId": "a", "value": "third"}}
	s := &GameService{Tables: map[string][]map[string]string{"TalentAdvenced": rows}}
	if _, indexed := s.indexedTableGroup("TalentAdvenced", "warId", "a"); indexed {
		t.Fatal("uninitialized lookup did not fall back")
	}
	s.InitializeGameTableLookup()
	for _, query := range [][2]string{{"TalentAdvenced", "wrongKey"}, {"missing", "warId"}} {
		if _, indexed := s.indexedTableGroup(query[0], query[1], "a"); indexed {
			t.Fatal("unknown group did not fall back", query)
		}
	}
	for _, id := range []string{"a", "b", "", "missing"} {
		var want []map[string]string
		for _, row := range rows {
			if row["warId"] == id {
				want = append(want, row)
			}
		}
		got, indexed := s.indexedTableGroup("TalentAdvenced", "warId", id)
		if !indexed || !reflect.DeepEqual(got, want) {
			t.Fatalf("ordered group %q changed", id)
		}
	}
	rows[0]["value"] = "changed in original row"
	got, _ := s.indexedTableGroup("TalentAdvenced", "warId", "a")
	if got[0]["value"] != rows[0]["value"] {
		t.Fatal("group copied its source row")
	}
	replaced := append([]map[string]string(nil), rows...)
	view := s.withRuntimeTables(map[string][]map[string]string{"TalentAdvenced": replaced})
	if _, indexed := view.indexedTableGroup("TalentAdvenced", "warId", "a"); indexed {
		t.Fatal("same-length replacement table did not fall back")
	}
}

func BenchmarkPowerProjection(b *testing.B) {
	s, player, source := powerProjectionFixture(b)
	legacy := powerProjectionReference(s)
	want, _, oldErr := powerProjectionLegacy(legacy, player, powerProjectionClone(b, source))
	got, newErr := s.formationFightPower(player, powerProjectionClone(b, source))
	if oldErr != nil || newErr != nil || want != got {
		b.Fatalf("benchmark inputs diverged: %d/%v vs %d/%v", want, oldErr, got, newErr)
	}
	for _, tc := range []struct {
		name    string
		service *GameService
		old     bool
	}{{"linear_json", legacy, true}, {"grouped_json", s, true}, {"grouped_direct", s, false}} {
		b.Run(tc.name, func(b *testing.B) {
			game := powerProjectionClone(b, source)
			if _, err := tc.service.battlePlayer(player, game); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if tc.old {
					if _, _, err := powerProjectionLegacy(tc.service, player, game); err != nil {
						b.Fatal(err)
					}
				} else if _, err := tc.service.formationFightPower(player, game); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
