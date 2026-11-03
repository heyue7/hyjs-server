package service

import (
	"encoding/json"
	"math"
	"reflect"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestFormationSharedBuffSpaceConditions(t *testing.T) {
	const prefix = "type:space,src1:2,op:"
	hero := map[string]string{"space": "2"}
	for _, tt := range []struct {
		condition string
		want      bool
	}{
		{"", true},
		{prefix + "1,src2:1,src2_val:1", true}, {prefix + "1,src2:1,src2_val:2", false},
		{prefix + "2,src2:1,src2_val:2", true}, {prefix + "2,src2:1,src2_val:3", false},
		{prefix + "3,src2:1,src2_val:2", true}, {prefix + "3,src2:1,src2_val:3", false},
		{prefix + "4,src2:1,src2_val:2", true}, {prefix + "4,src2:1,src2_val:1", false},
		{prefix + "5,src2:1,src2_val:3", true}, {prefix + "5,src2:1,src2_val:2", false},
		{prefix + "9,src2:1,src2_val:2", false}, {prefix, false}, {prefix + "3,src2_val:x", false},
		{"type:hp,src1:2,op:3,src2:1,src2_val:2", false},
	} {
		if got := formationSharedBuffFits(tt.condition, hero); got != tt.want {
			t.Fatalf("condition %q: got %v, want %v", tt.condition, got, tt.want)
		}
	}
}

func formationSharedFixture() (*GameService, *model.GameState) {
	s := &GameService{Tables: map[string][]map[string]string{
		"TalentAdvenced": {
			{"warId": "a", "trunck": "1", "talentLevel": "0"},
			{"warId": "a", "trunck": "1", "talentLevel": "1", "talentId": "main"},
			{"warId": "a", "trunck": "1", "talentLevel": "2", "talentId": "future"},
			{"warId": "a", "trunck": "0", "talentLevel": "chosen", "talentId": "branch"},
			{"warId": "a", "trunck": "0", "talentLevel": "not-chosen", "talentId": "future"},
			{"warId": "b", "trunck": "1", "talentLevel": "0", "talentId": "main"},
			{"warId": "reserve", "trunck": "1", "talentLevel": "0", "talentId": "future"},
		},
		"Talent": {
			{"Id": "main", "Buff0": "flat", "Buff0TriggerTimer": "0", "Buff0Select": "110", "Buff1": "future-buff", "Buff1TriggerTimer": "1", "Buff1Select": "110", "Buff2": "future-buff", "Buff2TriggerTimer": "0", "Buff2Select": "0", "Buff3": "future-buff", "Buff3TriggerTimer": "0", "Buff3Select": "110"},
			{"Id": "branch", "Buff0": "flat", "Buff0TriggerTimer": "0", "Buff0Select": "110", "Buff0ConditionArgs": "  type:space,src1:2,op:3,src2:1,src2_val:1  "},
			{"Id": "future", "Buff0": "future-buff", "Buff0TriggerTimer": "0", "Buff0Select": "110"},
			{"Id": "awake", "Buff0": "awake-buff", "Buff0TriggerTimer": "0", "Buff0Select": "110"},
		},
		"Buff": {
			{"Id": "flat", "Factor": "att", "Args1": "10"},
			{"Id": "future-buff", "Factor": "att", "Args1": "1000"},
			{"Id": "awake-buff", "Factor": "perhp", "Args1": "20"},
		},
		"HeroAwakenAdd": {
			{"heroId": "a", "stage": "1", "talentId": "awake"},
			{"heroId": "a", "stage": "2", "talentId": "future"},
		},
	}}
	a := &pb.Warrior{Id: proto.String("a"), Guid: proto.String("a-guid"), Talent: proto.Int32(1), TalentBranch: []string{"chosen"}, Awake: &pb.Awake{Stage: proto.Int32(2)}}
	b := &pb.Warrior{Id: proto.String("b"), Guid: proto.String("b-guid")}
	reserve := &pb.Warrior{Id: proto.String("reserve"), Guid: proto.String("reserve-guid")}
	g := &model.GameState{
		Warriors: []*pb.Warrior{a, b, reserve},
		Formation: []*pb.FormationItem{
			{Things: []*pb.FormationThing{{Pos: proto.Int32(0), Guid: a.Guid, Id: a.Id}}},
			{Things: []*pb.FormationThing{{Pos: proto.Int32(0), Guid: b.Guid, Id: b.Id}}},
		},
	}
	s.InitializeGameTableLookup()
	return s, g
}

func TestFormationSharedSourcesActiveDuplicatesAwakeAndNoMutation(t *testing.T) {
	s, g := formationSharedFixture()
	before, err := json.Marshal(g)
	if err != nil {
		t.Fatal(err)
	}
	members, shared, err := s.formationSharedSources(g)
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 2 || len(members[0].selection.active) != 3 || len(members[1].selection.active) != 1 {
		t.Fatalf("mounted source selections: %+v", members)
	}
	ids := make([]string, 0, len(shared))
	for _, buff := range shared {
		ids = append(ids, buff.row["Id"])
	}
	if want := []string{"flat", "flat", "awake-buff", "flat"}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("shared source order and exclusions: got %v, want %v", ids, want)
	}
	for _, tt := range []struct {
		space string
		att   float64
	}{{"1", 30}, {"2", 20}} {
		values, percent := map[string]float64{}, map[string]float64{}
		for _, buff := range shared {
			if formationSharedBuffFits(buff.condition, map[string]string{"space": tt.space}) {
				copyTalentProperty(values, percent, buff.row)
			}
		}
		if values["att"] != tt.att || percent["hp"] != 0.2 {
			t.Fatalf("space %s: values=%v percent=%v", tt.space, values, percent)
		}
	}
	after, err := json.Marshal(g)
	if err != nil || string(before) != string(after) {
		t.Fatalf("shared source discovery changed state: %v", err)
	}
	for _, stage := range []int32{0, 1, 2} {
		g.Warriors[0].Awake.Stage = proto.Int32(stage)
		_, buffs, err := s.formationSharedSources(g)
		if err != nil {
			t.Fatal(err)
		}
		awakeCount := 0
		for _, buff := range buffs {
			if buff.row["Id"] == "awake-buff" {
				awakeCount++
			}
		}
		want := 0
		if stage >= 1 {
			want = 1
		}
		if awakeCount != want {
			t.Fatalf("stage %d produced %d first-stage effects", stage, awakeCount)
		}
	}
	s.Tables["Buff"] = s.Tables["Buff"][1:]
	if _, _, err := s.formationSharedSources(g); err == nil {
		t.Fatal("missing permanent shared buff was accepted")
	}
}

func TestFormationSharedActualTableFourNativeBuffs(t *testing.T) {
	s := &GameService{}
	if err := s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	s.InitializeGameTableLookup()
	g := &model.GameState{}
	for _, id := range []string{"hero_051", "hero_007", "hero_012", "hero_010"} {
		w := &pb.Warrior{Id: proto.String(id), Guid: proto.String(id + "-fixture"), Talent: proto.Int32(6)}
		g.Warriors = append(g.Warriors, w)
		g.Formation = append(g.Formation, &pb.FormationItem{Things: []*pb.FormationThing{{Pos: proto.Int32(0), Guid: w.Guid, Id: w.Id}}})
	}
	members, buffs, err := s.formationSharedSources(g)
	if err != nil || len(members) != 4 {
		t.Fatalf("original source selection: %v", err)
	}
	ids := make([]string, 0, len(buffs))
	values, percent := map[string]float64{}, map[string]float64{}
	for _, buff := range buffs {
		ids = append(ids, buff.row["Id"])
		if formationSharedBuffFits(buff.condition, map[string]string{"space": "1"}) {
			copyTalentProperty(values, percent, buff.row)
		}
	}
	if want := []string{"buff_83", "buff_79", "buff_87", "buff_90"}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("native team buff sources: got %v, want %v", ids, want)
	}
	if values["damageAdd"] != 6 || values["damageReduce"] != 6 || percent["att"] != 0.06 || percent["hp"] != 0.06 {
		t.Fatalf("native team buff values: %v / %v", values, percent)
	}
}

func TestFormationSharedBatchAndUnmultipliedGemPet(t *testing.T) {
	s, player, base := powerProjectionFixture(t)
	base.Warriors[0].TalentBranch = append(base.Warriors[0].TalentBranch, "shared-test")
	s.Tables["TalentAdvenced"] = append(s.Tables["TalentAdvenced"], map[string]string{"warId": base.Warriors[0].GetId(), "trunck": "0", "talentLevel": "shared-test", "talentId": "shared-test-talent"})
	s.Tables["Talent"] = append(s.Tables["Talent"], map[string]string{"Id": "shared-test-talent", "Buff0": "shared-test-att", "Buff0Select": "110", "Buff1": "shared-test-hp", "Buff1Select": "110"})
	s.Tables["Buff"] = append(s.Tables["Buff"], map[string]string{"Id": "shared-test-att", "Factor": "peratt", "Args1": "10"}, map[string]string{"Id": "shared-test-hp", "Factor": "perhp", "Args1": "20"})
	s.Tables["Yuan"] = nil
	s.InitializeGameTableLookup()
	if err := s.PrepareFormation(base); err != nil {
		t.Fatal(err)
	}
	before, err := json.Marshal(base)
	if err != nil {
		t.Fatal(err)
	}
	members, buffs, err := s.formationSharedSources(base)
	if err != nil || len(members) != 4 || len(buffs) != 2 {
		t.Fatalf("expected one source's two team effects for four actors: %v, members=%d buffs=%d", err, len(members), len(buffs))
	}
	batch, err := s.battleFormationActors(player, base)
	if err != nil || len(batch) != len(members) {
		t.Fatalf("batch projection: %v", err)
	}
	overlay := make(map[string][]map[string]string, len(s.Tables))
	for name, rows := range s.Tables {
		overlay[name] = rows
	}
	overlay["Talent"] = append([]map[string]string(nil), s.Tables["Talent"]...)
	for i, row := range overlay["Talent"] {
		if row["Id"] == "shared-test-talent" {
			overlay["Talent"][i] = map[string]string{"Id": "shared-test-talent"}
		}
	}
	withoutShared, err := s.withRuntimeTables(overlay).battleFormationActors(player, powerProjectionClone(t, base))
	if err != nil || len(withoutShared) != len(batch) {
		t.Fatalf("baseline actor projection: %v", err)
	}
	for i, member := range members {
		direct, err := s.battleWarrior(player, member.warrior, base, member.slot)
		if err != nil || !proto.Equal(direct, batch[i]) {
			t.Fatalf("direct actor %d differs from batch: %v", i, err)
		}
		baseline := withoutShared[i].GetProp()
		actual := batch[i].GetProp()
		if math.Abs(actual.GetAtt()-baseline.GetAtt()*1.1) > 1e-7 || math.Abs(actual.GetHp()-baseline.GetHp()*1.2) > 1e-7 {
			t.Fatalf("team effects did not apply once to actor %d: baseline=%v actual=%v", i, baseline, actual)
		}
	}
	after, err := json.Marshal(base)
	if err != nil || string(before) != string(after) {
		t.Fatalf("batch projection changed prepared player state: %v", err)
	}

	plain := powerProjectionClone(t, base)
	withExtras := powerProjectionClone(t, base)
	withExtras.Equipment[0].EquipId = proto.String("equip_037")
	withExtras.Equipment[0].Gems = []string{"MineItem_080"}
	withExtras.Formation[0].Things[1].Id = proto.String("equip_037")
	// Keep the same underlying equipment in both variants; only gem and pet differ.
	plain.Equipment[0].EquipId = proto.String("equip_037")
	plain.Formation[0].Things[1].Id = proto.String("equip_037")
	pet := &pb.Pet{Id: proto.String("pet_002"), Guid: proto.String("shared-test-pet"), Level: proto.Int32(1), Exp: proto.Int64(0), InitProp: proto.String("PetTalent_11"), Eat: &pb.PetEat{Level: proto.Int32(0), Exp: proto.Int32(0), HdTimeList: []int32{0, 0, 0}}, Breached: &pb.PetBreached{Level: proto.Int32(0)}}
	if err := p9ProjectPet(withExtras, pet); err != nil {
		t.Fatal(err)
	}
	withExtras.Formation[0].Things = append(withExtras.Formation[0].Things, &pb.FormationThing{Pos: proto.Int32(7), Id: pet.Id, Guid: pet.Guid})
	gemValues := map[string]float64{}
	if err := s.equipmentGemProperties(withExtras.Equipment[0], gemValues); err != nil {
		t.Fatal(err)
	}
	petValues, _, _, err := s.P9GuardProperties(withExtras, withExtras.Formation[0])
	if err != nil {
		t.Fatal(err)
	}
	project := func(game *model.GameState) *pb.BaseProp {
		t.Helper()
		actors, err := s.battleFormationActors(player, game)
		if err != nil || len(actors) == 0 {
			t.Fatalf("actor projection: %v", err)
		}
		return actors[0].GetProp()
	}
	without, with := project(plain), project(withExtras)
	for _, tt := range []struct {
		key         string
		before, now float64
	}{{"hp", without.GetHp(), with.GetHp()}, {"att", without.GetAtt(), with.GetAtt()}} {
		expected := gemValues[tt.key] + petValues[tt.key]
		if expected <= 0 || math.Abs((tt.now-tt.before)-expected) > 1e-7 {
			t.Fatalf("%s gem/pet received shared percentage: delta=%v, raw=%v", tt.key, tt.now-tt.before, expected)
		}
	}
}
