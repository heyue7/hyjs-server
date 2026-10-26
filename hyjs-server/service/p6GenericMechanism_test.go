package service

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p6GenericGrownActors(t *testing.T) map[string]*pb.BattleHero {
	t.Helper()
	data, err := readTestAnalysis75("../../analysis/p6/p5-highest-actors-20261014.json")
	if err != nil {
		t.Fatal(err)
	}
	var source struct {
		CharacterPaths []struct {
			BattleHero *pb.BattleHero `json:"battleHero"`
		} `json:"characterPaths"`
	}
	if err := json.Unmarshal(data, &source); err != nil {
		t.Fatal(err)
	}
	actors := map[string]*pb.BattleHero{}
	for _, path := range source.CharacterPaths {
		actors[path.BattleHero.GetId()] = path.BattleHero
	}
	return actors
}

func p6GenericInput(t *testing.T, s *GameService, actor *pb.BattleHero, rounds int32) *model.FrozenBattleInput {
	t.Helper()
	actor = proto.Clone(actor).(*pb.BattleHero)
	// The prior growth artifact uses underscores in GUIDs. Native actor UID
	// parsing splits camp_GUID, so probes use legal fixed UUID identities only.
	actor.Guid = proto.String("00000000-0000-4000-8000-000000000001")
	actor.Pose = proto.Int32(3)
	other := p6Actor("hero_047", "00000000-0000-4000-8000-000000000002", "047_1,047_2,047_3", 3, 1000000)
	other.Prop.Att = proto.Float64(100)
	input, err := s.FreezeBattleInput(model.FrozenBattleInput{Attacker: &pb.BattlePlayer{PlayerId: proto.Int64(1001), ServerId: proto.Int32(1), Formation: []*pb.BattleHero{actor}, Inst: &pb.BattlePlayerInst{FirstOrder: proto.Int32(1), Anger: proto.Float64(100)}}, Defender: &pb.BattlePlayer{PlayerId: proto.Int64(1002), ServerId: proto.Int32(1), Formation: []*pb.BattleHero{other}, Inst: &pb.BattlePlayerInst{FirstOrder: proto.Int32(0)}}, MaxRound: rounds, Seed: 42, BattleType: 11})
	if err != nil {
		t.Fatal(err)
	}
	return input
}

func TestP6GenericMechanismAllOriginalActorAndPetAdmission(t *testing.T) {
	s := p6OriginalTables(t)
	if err := s.LoadP9Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	grown := p6GenericGrownActors(t)
	admitted, rejected, pets := 0, map[string]string{}, 0
	for _, row := range s.Tables["Hero"] {
		if !strings.HasPrefix(row["id"], "hero_") || row["heroClass"] != "0" {
			continue
		}
		actor := grown[row["id"]]
		if actor == nil {
			actor = p6Actor(row["id"], "catalog-material-"+row["id"], row["SkillId1"]+","+row["SkillId2"]+","+row["UltSkillId"], 3, 10000)
		}
		input := p6GenericInput(t, s, actor, 4)
		qualification, err := s.ValidateP6GenericMechanisms(t.Context(), input)
		if err != nil {
			rejected[row["id"]] = err.Error()
			t.Logf("partner=%s REJECT original quality=%s ImposState=%q skills=%q error=%s", row["id"], row["quality"], row["ImposState"], row["SkillId1"]+","+row["SkillId2"]+","+row["UltSkillId"], err)
			if row["quality"] != "12" || row["ImposState"] != "" || !strings.Contains(err.Error(), "ImposState") {
				t.Fatal("actual unimplemented actor branch", row["id"], err)
			}
			continue
		}
		if qualification.ActorCount != 2 || qualification.NativeCertified || len(qualification.SkillIDs) < 3 || len(qualification.BuffIDs) == 0 || qualification.MechanismHash == "" {
			t.Fatal("incomplete admitted original source", qualification)
		}
		t.Logf("partner=%s ADMIT actual skills=%d talents=%d mechanism=%s", row["id"], len(qualification.SkillIDs), len(qualification.TalentIDs), qualification.MechanismHash)
		admitted++
	}
	if admitted != 68 || len(rejected) != 6 {
		t.Fatal("whole original 74-partner denominator changed", admitted, rejected)
	}
	for _, row := range s.Tables["Pet"] {
		if row["id"] == "" || row["defaultTalent"] == "" {
			continue
		}
		actor := proto.Clone(grown["hero_011"]).(*pb.BattleHero)
		for _, skill := range s.Tables["PetTalentSkill"] {
			if skill["petId"] == row["id"] {
				actor.TalentList = append(actor.TalentList, skill["talent"])
			}
		}
		// The original flat constructor accepts grown equipment/pet fractional
		// contributions without rounding them to old demo integer panels.
		actor.Prop.Att = proto.Float64(actor.GetProp().GetAtt() + 0.25)
		if _, err := s.ValidateP6GenericMechanisms(t.Context(), p6GenericInput(t, s, actor, 4)); err != nil {
			t.Fatal("actual guarding pet branch", row["id"], err)
		}
		t.Logf("pet=%s ADMIT actual original talent IDs=%v", row["id"], actor.GetTalentList()[len(grown["hero_011"].GetTalentList()):])
		pets++
	}
	if pets != 78 {
		t.Fatal("whole original pet denominator changed", pets)
	}
	t.Logf("74 original partner rows: %d admitted, six original blank ImposState rows explicitly rejected; all %d guarding-pet talent sets admitted", admitted, pets)
}

func TestP6GenericMechanismRepresentativeStrictBattles(t *testing.T) {
	s := p6OriginalTables(t)
	grown := p6GenericGrownActors(t)
	for _, scenario := range []struct{ hero, family string }{{"hero_025", "damage"}, {"hero_011", "healing"}, {"hero_071", "control"}, {"hero_001", "death"}} {
		t.Run(scenario.family, func(t *testing.T) {
			actor := proto.Clone(grown[scenario.hero]).(*pb.BattleHero)
			if scenario.family == "healing" {
				actor.Inst.CurHp = proto.Float64(actor.GetProp().GetHp() / 2)
			}
			if scenario.family == "death" {
				actor.Inst.CurHp = proto.Float64(1)
			}
			input := p6GenericInput(t, s, actor, 6)
			if scenario.family == "death" {
				input.Defender.Formation[0].Prop.Att = proto.Float64(1000000)
				var err error
				input, err = s.FreezeBattleInput(*input)
				if err != nil {
					t.Fatal(err)
				}
			}
			before, _ := json.Marshal(input)
			result, qualification, err := s.ComputeP6GenericMechanismBattle(t.Context(), input, nil)
			if err != nil {
				t.Fatal("actual complete mechanism execution", err)
			}
			counts := map[string]int{}
			for _, event := range result.Outcome.Events {
				counts[event.Kind]++
				if event.Kind == "buff_load" && event.Buff == "buff_134" {
					counts["control_dizzy"]++
				}
			}
			if result.Outcome.Authoritative || qualification.NativeCertified {
				t.Fatal("unreviewed generic family promoted authority")
			}
			if scenario.family == "damage" && counts["damage"] == 0 {
				t.Fatal("damage handler never executed", counts)
			}
			if scenario.family == "healing" && counts["buff_heal"]+counts["round_heal"]+counts["skill_heal"] == 0 {
				t.Fatal("healing handler never executed", counts)
			}
			if scenario.family == "control" && counts["control_dizzy"] == 0 {
				t.Fatal("control handler never executed", counts)
			}
			if scenario.family == "death" && counts["revive"] == 0 {
				t.Fatal("death handler never executed", counts)
			}
			p6GenericExportRepresentative(t, scenario.family, input, result, qualification)
			replay, _, err := s.ComputeP6GenericMechanismBattle(t.Context(), input, result.OriginalOperates)
			if err != nil || !proto.Equal(result.OriginalOperates, replay.OriginalOperates) || result.Outcome.RandomDraws != replay.Outcome.RandomDraws {
				t.Fatal("full original operation replay changed", err)
			}
			result.FinalActors[0].CurrentHP++
			if validateP6GenericMechanismTerminal(input, result) == nil {
				t.Fatal("tampered terminal HP accepted")
			}
			after, _ := json.Marshal(input)
			if string(before) != string(after) {
				t.Fatal("generic calculation rewrote source actors/tables")
			}
			t.Logf("%s actual events %v", scenario.hero, counts)
		})
	}
	bad := p6GenericInput(t, s, grown["hero_025"], 4)
	bad.Attacker.Formation[0].TalentList = append(bad.Attacker.Formation[0].TalentList, "tal_unknown_generic")
	if _, err := s.ValidateP6GenericMechanisms(t.Context(), bad); err == nil || !strings.Contains(err.Error(), "tal_unknown_generic") {
		t.Fatal("unknown original source did not name the missing branch", err)
	}
}

func p6GenericExportRepresentative(t *testing.T, name string, input *model.FrozenBattleInput, result *model.CombatFinalHPOutcome, qualification *model.P6MechanismQualification) {
	t.Helper()
	directory := os.Getenv("HYJS_P6_GENERIC_EXPORT_DIR")
	if directory == "" {
		return
	}
	root, err := filepath.Abs("../../analysis")
	if err != nil {
		t.Fatal(err)
	}
	directory, err = filepath.Abs(directory)
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(root, directory)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		t.Fatal("generic native probes must remain under analysis")
	}
	if err := os.MkdirAll(directory, 0755); err != nil {
		t.Fatal(err)
	}
	wire := &pb.SCFightPlayback{Fightid: proto.String("generic-mechanism-" + name), RandSeed: proto.Int64(input.Seed), BattleType: proto.Int32(input.BattleType), Attacker: input.Attacker, Defender1: input.Defender, Defender2: input.Monster, MaxRound: proto.Int32(input.MaxRound), AttackerFactor: proto.Float64(1), DefenderFactor: proto.Float64(1)}
	encodedWire, err := proto.MarshalOptions{Deterministic: true}.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{name + ".pb": encodedWire}
	for suffix, value := range map[string]any{".frozen-input.json": input, ".go-final.json": result, ".qualification.json": qualification} {
		encoded, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		files[name+suffix] = encoded
	}
	for filename, data := range files {
		file, err := os.OpenFile(filepath.Join(directory, filename), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if err != nil {
			t.Fatal(fmt.Errorf("preserve prior generic probe %s: %w", filename, err))
		}
		_, writeErr := file.Write(data)
		closeErr := file.Close()
		if writeErr != nil || closeErr != nil {
			t.Fatal("write original generic probe", writeErr, closeErr)
		}
	}
}
