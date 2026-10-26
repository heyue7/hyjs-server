package service

import (
	"encoding/json"
	"fmt"
	"github.com/go-sql-driver/mysql"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
)

type p6ActualGrowthPath struct {
	HeroID       string         `json:"heroId"`
	Warrior      *pb.Warrior    `json:"warrior"`
	BattleHero   *pb.BattleHero `json:"battleHero"`
	NodeIDs      []string       `json:"nodeIds"`
	HighestTrunk int32          `json:"highestTrunk"`
	BranchIDs    []string       `json:"branchIds"`
	AwakeStage   int32          `json:"awakeStage"`
	FuseLevel    int32          `json:"fuseLevel"`
	Fixture      string         `json:"fixture"`
}

// These actors come from real P5 mutations and cold database reopening. Their
// panel, talents, breakthrough and skill transport are used without rewriting.
func p6ReadActualGrowthPaths(t *testing.T) []p6ActualGrowthPath {
	t.Helper()
	var paths []p6ActualGrowthPath
	for _, tc := range []struct {
		name             string
		count, mutations int
	}{{"p5-highest-actors-20261014.json", 76, 927}, {"p5-awake-highest-actors-20261014.json", 15, 0}} {
		raw, err := readTestAnalysis75("../../analysis/p6/" + tc.name)
		if err != nil {
			t.Fatal(err)
		}
		var fixture struct {
			Paths       []p6ActualGrowthPath `json:"characterPaths"`
			TableSHA    map[string]string    `json:"originalTableSha256"`
			Activations int                  `json:"actualActivations"`
			Native      bool                 `json:"dynamicCombatCertified"`
		}
		if err = json.Unmarshal(raw, &fixture); err != nil || len(fixture.Paths) != tc.count || fixture.Activations != tc.mutations || fixture.Native {
			t.Fatal("real original growth fixture drift", tc.name, err)
		}
		for table, expected := range fixture.TableSHA {
			original, readErr := readTestAnalysis75("../../analysis/tables/Table" + table + ".tsv")
			if readErr != nil || p6Digest(original) != expected {
				t.Fatal("actual growth source table drift", table, readErr)
			}
		}
		for _, row := range fixture.Paths {
			if row.Warrior == nil || row.BattleHero == nil || row.HeroID != row.Warrior.GetId() || row.HeroID != row.BattleHero.GetId() || row.Warrior.GetGuid() != row.BattleHero.GetGuid() || row.BattleHero.GetBreachedLevel() != row.Warrior.GetBreached().GetLevel() || row.BattleHero.GetLevel() != 100 {
				t.Fatal("actual growth actor transport changed", tc.name, row.HeroID)
			}
			if tc.mutations == 927 && (row.HighestTrunk != row.Warrior.GetTalent() || !slices.Equal(row.BranchIDs, row.Warrior.GetTalentBranch()) || row.BattleHero.GetAwakeStage() != 0) {
				t.Fatal("actual highest talent path mismatch", row.HeroID)
			}
			if tc.mutations == 0 && (row.AwakeStage != 2 || row.FuseLevel != 100 || row.Warrior.GetAwake().GetStage() != 2 || row.Warrior.GetAwake().GetFuseLevel() != 100 || row.BattleHero.GetAwakeStage() != 2) {
				t.Fatal("actual highest enabled awakening path mismatch", row.HeroID)
			}
			row.Fixture = tc.name
			paths = append(paths, row)
		}
	}
	return paths
}

func p6ActualGrowthInput(t *testing.T, s *GameService, subject p6ActualGrowthPath, camp int, originals map[string]*pb.BattleHero) *model.FrozenBattleInput {
	t.Helper()
	a := proto.Clone(subject.BattleHero).(*pb.BattleHero)
	a.Pose = proto.Int32(1)
	// Formation positions and starting AP define this mechanism scenario. Actual
	// inventory panels, levels, talents, skills and the original GUID stay intact.
	var mates []*pb.BattleHero
	for _, id := range []string{"hero_047", "hero_003", "hero_018", "hero_009"} {
		if id == subject.HeroID || originals[id] == nil {
			continue
		}
		mate := proto.Clone(originals[id]).(*pb.BattleHero)
		mate.Pose = proto.Int32(int32(len(mates) + 2))
		mates = append(mates, mate)
		if len(mates) == 1 {
			break
		}
	}
	var enemies []*pb.BattleHero
	for _, id := range []string{"hero_047", "hero_005", "hero_018", "hero_003", "hero_009", "hero_001"} {
		if originals[id] == nil {
			continue
		}
		enemy := proto.Clone(originals[id]).(*pb.BattleHero)
		enemy.Pose = proto.Int32(int32(len(enemies) + 1))
		enemies = append(enemies, enemy)
		if len(enemies) == 3 {
			break
		}
	}
	if len(mates) != 1 || len(enemies) != 3 {
		t.Fatal("actual growth fixture lacks recorded mechanism companions")
	}
	owner := &pb.BattlePlayer{PlayerId: proto.Int64(42), Formation: append([]*pb.BattleHero{a}, mates...), Inst: &pb.BattlePlayerInst{Anger: proto.Float64(100)}}
	enemy := &pb.BattlePlayer{PlayerId: proto.Int64(43), Formation: enemies, Inst: &pb.BattlePlayerInst{Anger: proto.Float64(100)}}
	if camp == 1 {
		owner, enemy = enemy, owner
	}
	if err := SetP6FirstOrder(owner, enemy, camp == 1); err != nil {
		t.Fatal(err)
	}
	f, err := s.FreezeBattleInput(model.FrozenBattleInput{Attacker: owner, Defender: enemy, Seed: 42, MaxRound: 10, BattleType: 2, DefenderFirst: camp == 1})
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestP6ActualHighestTalentAndAwakeningAllActorsWholeReplay(t *testing.T) {
	if os.Getenv("HYJS_P6_ACTUAL_GROWTH_TEST") != "1" {
		t.Skip("explicit full actual P5 growth matrix disabled")
	}
	paths := p6ReadActualGrowthPaths(t)
	originals := map[string]*pb.BattleHero{}
	for _, row := range paths[:76] {
		originals[row.HeroID] = row.BattleHero
	}
	s := p6OriginalTables(t)
	var mutex sync.Mutex
	var rows []map[string]any
	limit := make(chan struct{}, 4)
	t.Cleanup(func() {
		if len(rows) != 182 || t.Failed() {
			return
		}
		if output := os.Getenv("HYJS_P6_ACTUAL_GROWTH_OUTPUT"); output != "" {
			slices.SortFunc(rows, func(a, b map[string]any) int { return strings.Compare(a["case"].(string), b["case"].(string)) })
			raw, err := json.MarshalIndent(map[string]any{"engineVersion": constUtil.P6EngineVersion, "cases": rows, "nativeCompared": false, "authoritative": false, "scope": "real P5 cold-reopened highest talent/level100 and awake2/fuse100 panels with original skills/talents; complete per-branch/native/highest-other-growth pending"}, "", "  ")
			if err != nil {
				t.Error(err)
				return
			}
			if err = os.MkdirAll(filepath.Dir(output), 0755); err != nil {
				t.Error(err)
				return
			}
			if err = os.WriteFile(output, append(raw, '\n'), 0644); err != nil {
				t.Error(err)
			}
		}
	})
	for _, path := range paths {
		for _, camp := range []int{0, 1} {
			label := path.Fixture + "/" + path.HeroID + "/camp" + strconv.Itoa(camp)
			t.Run(label, func(t *testing.T) {
				t.Parallel()
				limit <- struct{}{}
				defer func() { <-limit }()
				input := p6ActualGrowthInput(t, s, path, camp, originals)
				before, _ := json.Marshal(input)
				service := &GameService{Tables: input.Tables}
				out, err := service.ComputeFrozenBattle(t.Context(), input, nil)
				if err != nil {
					t.Fatal(label, err)
				}
				replay, err := service.ComputeFrozenBattle(t.Context(), input, out.Operates)
				if err != nil || out.Authoritative || !proto.Equal(out.Operates, replay.Operates) || !proto.Equal(out.Attacker, replay.Attacker) || !proto.Equal(out.Defender, replay.Defender) || out.RandomDraws != replay.RandomDraws || !reflect.DeepEqual(out.Events, replay.Events) {
					t.Fatal("actual original growth full replay drift", label, err)
				}
				after, _ := json.Marshal(input)
				if string(before) != string(after) {
					t.Fatal("actual frozen P5 inputs mutated")
				}
				var skills, buffs []string
				for _, event := range out.Events {
					if event.Actor != fmt.Sprintf("%d_%s", camp, path.BattleHero.GetGuid()) {
						continue
					}
					if event.Kind == "skill_begin" && !slices.Contains(skills, event.Skill) {
						skills = append(skills, event.Skill)
					}
					if event.Kind == "buff_load" && !slices.Contains(buffs, event.Buff) {
						buffs = append(buffs, event.Buff)
					}
				}
				if len(skills) == 0 {
					t.Fatal("actual original actor never released an owned skill", label)
				}
				panel, _ := json.Marshal(path.BattleHero.Prop)
				mutex.Lock()
				rows = append(rows, map[string]any{"case": label, "inputSHA256": p6Digest(before), "panelSHA256": p6Digest(panel), "heroId": path.HeroID, "camp": camp, "warrior": path.Warrior, "battleHero": path.BattleHero, "nodeIds": path.NodeIDs, "skillsStarted": skills, "buffsApplied": buffs, "finalRound": out.FinalRound, "randomDraws": out.RandomDraws, "eventCount": len(out.Events), "nativeAndAllConditionBranches": "pending"})
				mutex.Unlock()
			})
		}
	}
}

func p6OriginalGreenWholeInputs(t *testing.T) []*model.FrozenBattleInput {
	t.Helper()
	s := &GameService{}
	if err := s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	var inputs []*model.FrozenBattleInput
	for _, row := range s.Tables["Hero"] {
		if row["heroClass"] != "0" || row["quality"] != "12" {
			continue
		}
		p := &pb.Player{PlayerId: proto.Int64(77), HeroWarId: proto.String("other-main"), Level: proto.Int32(100), WarriorBagSize: proto.Int32(100), EquipBagSize: proto.Int32(100)}
		g := &model.GameState{}
		gain, err := s.GrantFixedReward(p, g, row["id"]+",1")
		if err != nil || len(gain.GetWars()) != 1 {
			t.Fatal("actual original green reward", err)
		}
		w := gain.GetWars()[0]
		a, err := s.battleWarrior(p, w, g, &pb.FormationItem{FormatPos: proto.Int32(1), BattlePos: proto.Int32(1), Things: []*pb.FormationThing{{Pos: proto.Int32(0), Id: w.Id, Guid: w.Guid}}})
		if err != nil || a.GetSkillOrder() != ",," || a.GetProp().GetHp() != tableNumber(row, "hpBase") || a.GetBreachedLevel() != w.GetBreached().GetLevel() {
			t.Fatal("actual original green projection", row["id"], err)
		}
		for _, camp := range []int{0, 1} {
			input := p6Input(t, s, proto.Clone(a).(*pb.BattleHero), p6Actor("hero_047", "green-opponent", "047_1,047_2,047_3", 1, 80000), 4)
			if camp == 1 {
				input.Attacker, input.Defender = input.Defender, input.Attacker
				input.DefenderFirst = true
			}
			if err := SetP6FirstOrder(input.Attacker, input.Defender, input.DefenderFirst); err != nil {
				t.Fatal(err)
			}
			inputs = append(inputs, input)
		}
	}
	if len(inputs) != 12 {
		t.Fatal("six original green actor count drift", len(inputs))
	}
	return inputs
}

func TestP6OriginalAllSixGreenActorsRetainNativeEmptyImposStateRejection(t *testing.T) {
	for _, input := range p6OriginalGreenWholeInputs(t) {
		s := &GameService{Tables: input.Tables}
		before, _ := json.Marshal(input)
		if _, err := s.ComputeFrozenBattle(t.Context(), input, nil); err == nil || !strings.Contains(err.Error(), "ImposState") {
			t.Fatal("original six green rows invented an absent native state default", err)
		}
		after, _ := json.Marshal(input)
		if string(before) != string(after) {
			t.Fatal("rejected original green input changed")
		}
	}
	e := p6PropertyTransferEngine(t)
	e.input.Attacker.Formation[0].SkillOrder = proto.String(",,")
	if _, err := e.service.ComputeFrozenBattle(t.Context(), e.input, nil); err == nil {
		t.Fatal("defined skill hero forged an empty skill list")
	}
}

func p6OriginalIdleNPCWholeInputs(t *testing.T) []*model.FrozenBattleInput {
	t.Helper()
	s := p6OriginalTables(t)
	var inputs []*model.FrozenBattleInput
	for _, row := range s.Tables["Hero"] {
		if row["heroClass"] != "1" || row["SkillId1"] != "" || row["SkillId2"] != "" || row["UltSkillId"] != "" {
			continue
		}
		if row["ImposState"] != "0" {
			t.Fatal("original idle NPC states drifted", row["id"])
		}
		a, err := s.actor(row, "original-idle-"+row["id"], 1, false)
		if err != nil {
			t.Fatal(err)
		}
		enemy, err := s.actor(s.table("Hero", "id", "hero_047"), "original-active-047", 1, false)
		if err != nil {
			t.Fatal(err)
		}
		for _, camp := range []int{0, 1} {
			input := p6Input(t, s, proto.Clone(a).(*pb.BattleHero), proto.Clone(enemy).(*pb.BattleHero), 4)
			if camp == 1 {
				input.Attacker, input.Defender = input.Defender, input.Attacker
				input.DefenderFirst = true
			}
			if err := SetP6FirstOrder(input.Attacker, input.Defender, input.DefenderFirst); err != nil {
				t.Fatal(err)
			}
			input.Attacker.Inst.Anger, input.Defender.Inst.Anger = proto.Float64(100), proto.Float64(100)
			inputs = append(inputs, input)
		}
	}
	if len(inputs) != 26 {
		t.Fatal("all 13 original skill-free NPC row count drifted", len(inputs))
	}
	return inputs
}

func TestP6OriginalAllThirteenSkillFreeNPCsIdleTargetAndReplay(t *testing.T) {
	for _, input := range p6OriginalIdleNPCWholeInputs(t) {
		s := &GameService{Tables: input.Tables}
		out, err := s.ComputeFrozenBattle(t.Context(), input, nil)
		if err != nil {
			t.Fatal(err)
		}
		targeted := false
		for _, event := range out.Events {
			if (event.Kind == "skill_begin" || event.Kind == "skill_press" || event.Kind == "release") && strings.Contains(event.Actor, "original-idle-") {
				t.Fatal("native empty original skills generated an attack", event)
			}
			if event.Kind == "damage" && strings.Contains(event.Target, "original-idle-") {
				targeted = true
			}
		}
		if !targeted {
			t.Fatal("original idle NPC vanished from target selection")
		}
		replay, err := s.ComputeFrozenBattle(t.Context(), input, out.Operates)
		if err != nil || out.Authoritative || !proto.Equal(out.Operates, replay.Operates) || !proto.Equal(out.Attacker, replay.Attacker) || !proto.Equal(out.Defender, replay.Defender) || out.RandomDraws != replay.RandomDraws || !reflect.DeepEqual(out.Events, replay.Events) {
			t.Fatal("original idle NPC full replay", err)
		}
	}
}

// Each actual P5 actor/camp is persisted once, then recovered through the original
// SCFightPlayback message after closing the entire MySQL connection pool.
func TestP6MySQLActualGrowthFullMatrix(t *testing.T) {
	dsn := os.Getenv("HYJS_MYSQL_P6_DSN")
	if dsn == "" || os.Getenv("HYJS_P6_ACTUAL_GROWTH_TEST") != "1" {
		t.Skip("explicit isolated full actual growth MySQL disabled")
	}
	config, err := mysql.ParseDSN(dsn)
	if err != nil || !strings.HasPrefix(config.DBName, "hyjs_test_p6_") {
		t.Fatal("dedicated actual growth MySQL required")
	}
	ctx := t.Context()
	open := func() {
		t.Helper()
		if e := db.InitGameMySQL(ctx, db.MySQLConfig{Host: config.Addr, User: config.User, Password: config.Passwd, Database: config.DBName}); e != nil {
			t.Fatal(e)
		}
	}
	open()
	conn, _ := db.GetIns().DB()
	defer func() { conn.Close() }()
	s := &GameService{}
	if err = s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err = s.InitializeP6(ctx); err != nil {
		t.Fatal(err)
	}
	id := dailyCopyTestPlayer(t, ctx, "p6-actual-growth-cold", 100)
	paths := p6ReadActualGrowthPaths(t)
	source := "p6_actual_growth"
	if os.Getenv("HYJS_P6_ACTUAL_TRAINING_TEST") == "1" {
		paths = p6ReadActualTrainingPaths(t)
		source = "p6_actual_training"
	}
	if os.Getenv("HYJS_P6_ACTUAL_BREACH_TEST") == "1" {
		if os.Getenv("HYJS_P6_ACTUAL_TRAINING_TEST") == "1" {
			t.Fatal("select one cold MySQL actual growth stage")
		}
		paths = p6ReadActualBreachPaths(t)
		source = "p6_actual_breach"
	}
	if os.Getenv("HYJS_P6_ACTUAL_COMBINED_TEST") == "1" {
		if os.Getenv("HYJS_P6_ACTUAL_TRAINING_TEST") == "1" || os.Getenv("HYJS_P6_ACTUAL_BREACH_TEST") == "1" {
			t.Fatal("select one cold MySQL actual combined stage")
		}
		paths = p6ReadActualCombinedPaths(t)
		source = "p6_actual_combined"
	}
	caseCount := len(paths) * 2
	originals := map[string]*pb.BattleHero{}
	for _, row := range paths {
		if originals[row.HeroID] != nil {
			continue
		}
		originals[row.HeroID] = row.BattleHero
	}
	var records []struct {
		id                 string
		caseName           string
		attacker, defender *pb.BattlePlayer
		operates           *pb.BattleOperate
		outcome            *model.CombatOutcome
	}
	for pathIndex, path := range paths {
		for _, camp := range []int{0, 1} {
			input := p6ActualGrowthInput(t, s, path, camp, originals)
			input.Attacker.PlayerId = proto.Int64(id)
			fight := fmt.Sprintf("actual-growth-%d-%s-%d", pathIndex, path.HeroID, camp)
			var recordID string
			var out *model.CombatOutcome
			if err = p6Tx(ctx, id, func(tx *gorm.DB) error {
				var e error
				recordID, e = s.CaptureBattleReplay(ctx, tx, id, source, fight, *input)
				if e != nil {
					return e
				}
				out, e = s.ComputeBattleReplay(ctx, tx, id, source, fight, nil)
				return e
			}); err != nil {
				t.Fatal(path.Fixture, path.HeroID, camp, err)
			}
			if out.Authoritative || len(out.Operates.GetSkillOpts()) == 0 {
				t.Fatal("actual growth record lost computed original operations")
			}
			records = append(records, struct {
				id                 string
				caseName           string
				attacker, defender *pb.BattlePlayer
				operates           *pb.BattleOperate
				outcome            *model.CombatOutcome
			}{recordID, path.Fixture + "/" + path.HeroID + "/camp" + strconv.Itoa(camp), input.Attacker, input.Defender, out.Operates, out})
		}
	}
	if len(records) != caseCount {
		t.Fatal("actual original growth persistence count drift", len(records))
	}
	if err = conn.Close(); err != nil {
		t.Fatal(err)
	}
	open()
	conn, _ = db.GetIns().DB()
	var exported []map[string]any
	for index, record := range records {
		messages, e := s.fightPlayback(ctx, id, &pb.CSFightPlayback{FightId: proto.String(record.id), ServerId: proto.Int32(1)})
		if e != nil || len(messages) != 1 {
			t.Fatal("actual growth original replay after cold connection", e)
		}
		wire, e := proto.Marshal(messages[0].(*pb.SCFightPlayback))
		if e != nil {
			t.Fatal(e)
		}
		decoded := &pb.SCFightPlayback{}
		if e = proto.Unmarshal(wire, decoded); e != nil {
			t.Fatal(e)
		}
		if !proto.Equal(decoded.Attacker, record.attacker) || !proto.Equal(decoded.Defender1, record.defender) || !proto.Equal(decoded.OptRecord, record.operates) {
			t.Fatal("actual original panel/GUID/talent/operations lost on cold original wire", record.id)
		}
		// Derive initiative from the decoded original flags, independently of the
		// server's internal camp selection, after the entire connection is reopened.
		if decoded.Attacker.GetInst().GetFirstOrder()+decoded.Defender1.GetInst().GetFirstOrder() != 1 || len(decoded.OptRecord.GetSkillOpts()) == 0 {
			t.Fatal("actual growth replay lost canonical original initiative", record.id)
		}
		firstPlayer := decoded.Attacker
		firstCamp := int32(0)
		if decoded.Defender1.GetInst().GetFirstOrder() == 1 {
			firstPlayer = decoded.Defender1
			firstCamp = 1
		}
		firstOperation := decoded.OptRecord.GetSkillOpts()[0]
		firstGUID := firstOperation.GetActorUid()
		found := false
		for _, actor := range firstPlayer.GetFormation() {
			found = found || actor.GetGuid() == firstGUID
		}
		if !found || firstOperation.GetCamp() != firstCamp || firstOperation.GetRoundNum() != 1 {
			t.Fatal("original wire initiative disagrees with first computed operation", record.id, firstGUID)
		}
		// A diagnostic native batch consumes these exact decoded original frames.
		// Export is opt-in and occurs only after cold-query and all wire assertions.
		if directory := os.Getenv("HYJS_P6_ACTUAL_GROWTH_REPLAY_DIR"); directory != "" {
			if !filepath.IsAbs(directory) {
				t.Fatal("native replay export directory must be absolute")
			}
			if e = os.MkdirAll(directory, 0755); e != nil {
				t.Fatal(e)
			}
			name := fmt.Sprintf("%03d", index)
			expected, encodeErr := json.Marshal(record.outcome)
			if encodeErr != nil {
				t.Fatal(encodeErr)
			}
			if e = os.WriteFile(filepath.Join(directory, name+".pb"), wire, 0644); e != nil {
				t.Fatal(e)
			}
			if e = os.WriteFile(filepath.Join(directory, name+"-go-timeline.json"), expected, 0644); e != nil {
				t.Fatal(e)
			}
			exported = append(exported, map[string]any{"case": record.caseName, "wire": name + ".pb", "wireSHA256": p6Digest(wire), "goTimeline": name + "-go-timeline.json", "goTimelineSHA256": p6Digest(expected), "firstCamp": firstCamp, "seed": decoded.GetRandSeed(), "battleType": decoded.GetBattleType()})
		}
	}
	if directory := os.Getenv("HYJS_P6_ACTUAL_GROWTH_REPLAY_DIR"); directory != "" {
		if len(exported) != caseCount {
			t.Fatal("complete native replay export matrix missing")
		}
		manifest, e := json.MarshalIndent(map[string]any{"engineVersion": constUtil.P6EngineVersion, "nativeCompared": false, "authoritative": false, "cases": exported}, "", "  ")
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(directory, "manifest.json"), append(manifest, '\n'), 0644); e != nil {
			t.Fatal(e)
		}
	}
}
