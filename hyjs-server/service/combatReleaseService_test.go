package service

import (
	"encoding/csv"
	"math"
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/protobuf/proto"
	pb "hyjs-server/model/protocol"
)

func combatTestTables(t *testing.T) *GameService {
	t.Helper()
	s := &GameService{Tables: map[string][]map[string]string{}}
	for _, name := range []string{"Hero", "Skill", "SkillCast"} {
		f, err := os.Open(filepath.Join("../../analysis/tables", "Table"+name+".tsv"))
		if err != nil {
			t.Fatal(err)
		}
		reader := csv.NewReader(f)
		reader.Comma = '\t'
		reader.FieldsPerRecord = -1
		rows, err := reader.ReadAll()
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range rows[5:] {
			entry := map[string]string{}
			for i, v := range row {
				if i < len(rows[1]) {
					entry[rows[1][i]] = v
				}
			}
			s.Tables[name] = append(s.Tables[name], entry)
		}
	}
	return s
}

func combatTestActor(id, guid, skills string) *pb.BattleHero {
	return &pb.BattleHero{Id: proto.String(id), Guid: proto.String(guid), Level: proto.Int32(1), SkillOrder: proto.String(skills), Prop: &pb.BaseProp{Hp: proto.Float64(1000), Att: proto.Float64(500), Pdef: proto.Float64(200), Mdef: proto.Float64(50), Hit: proto.Float64(100), CriFactor: proto.Float64(50)}, Inst: &pb.BattleHeroInst{CurHp: proto.Float64(1000)}}
}

func closeCombat(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-10 {
		t.Fatalf("got %.15f want %.15f", got, want)
	}
}

func TestCombatJRandomGoldenValues(t *testing.T) {
	// 对照Java兼容JRandom的已知黄金向量，验证48位截断和26+27位拼接。
	vectors := []struct {
		seed   int64
		values []float64
	}{
		{0, []float64{0.730967787376657, 0.24053641567148587, 0.6374174253501083, 0.5504370051176339}},
		{1, []float64{0.7308781907032909, 0.41008081149220166, 0.20771484130971707, 0.3327170559595112}},
	}
	for _, v := range vectors {
		r := NewCombatRandom(v.seed)
		for _, want := range v.values {
			closeCombat(t, r.Value(), want)
		}
		if r.Draws() != 4 {
			t.Fatal(r.Draws())
		}
	}
}

func TestCombatPhysicalMagicCriticalAndHP(t *testing.T) {
	s := combatTestTables(t)
	a := combatTestActor("hero_001", "a", "001_1,001_2,001_3")
	d := combatTestActor("hero_003", "d", "003_1,003_2,003_3")
	originalA := proto.Clone(a)
	originalD := proto.Clone(d)
	rng := NewCombatRandom(0)
	result, err := s.EvaluateUnbuffedRelease(a, d, "4", rng)
	if err != nil {
		t.Fatal(err)
	}
	closeCombat(t, result.Damage, 77.06126138025162)
	closeCombat(t, result.TargetHP, 922.9387386197484)
	if result.Miss || result.Critical || result.TargetDead || result.RandomDraws != 4 || rng.Draws() != 4 {
		t.Fatal(result)
	}
	if !proto.Equal(a, originalA) || !proto.Equal(d, originalD) {
		t.Fatal("release changed frozen actors")
	}
	a.Prop.Cri = proto.Float64(100)
	critical, err := s.EvaluateUnbuffedRelease(a, d, "4", NewCombatRandom(0))
	if err != nil {
		t.Fatal(err)
	}
	if !critical.Critical {
		t.Fatal("missing critical")
	}
	closeCombat(t, critical.Damage, 115.59189207037743)
	magic, err := s.EvaluateUnbuffedRelease(d, a, "293", NewCombatRandom(0))
	if err != nil {
		t.Fatal(err)
	}
	// magic uses mdef50 instead of pdef200, and original SkillCast293 factor18%.
	closeCombat(t, magic.Damage, 83.22616229067175)
	d.Inst.CurHp = proto.Float64(50)
	lethal, err := s.EvaluateUnbuffedRelease(a, d, "4", NewCombatRandom(0))
	if err != nil {
		t.Fatal(err)
	}
	if !lethal.TargetDead || lethal.TargetHP != 0 {
		t.Fatal(lethal)
	}
}

func TestCombatMissDefenseFloorAndRNGContinuity(t *testing.T) {
	s := combatTestTables(t)
	a := combatTestActor("hero_001", "a", "001_1,001_2,001_3")
	d := combatTestActor("hero_003", "d", "003_1,003_2,003_3")
	a.Prop.Hit = proto.Float64(50)
	rng := NewCombatRandom(0)
	miss, err := s.EvaluateUnbuffedRelease(a, d, "4", rng)
	if err != nil {
		t.Fatal(err)
	}
	if !miss.Miss || miss.Damage != 0 || miss.TargetHP != 1000 || miss.RandomDraws != 1 {
		t.Fatal(miss)
	}
	closeCombat(t, rng.Value(), 0.24053641567148587)
	a.Prop.Hit = proto.Float64(100)
	d.Prop.Pdef = proto.Float64(10000)
	floor, err := s.EvaluateUnbuffedRelease(a, d, "4", NewCombatRandom(0))
	if err != nil {
		t.Fatal(err)
	}
	if floor.Damage != 1 || floor.TargetHP != 999 {
		t.Fatal(floor)
	}
	// 相同种子和同一次出手输入得到相同原子结果，不据此宣称整场重放。
	first, err := s.EvaluateUnbuffedRelease(a, d, "4", NewCombatRandom(12345))
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.EvaluateUnbuffedRelease(a, d, "4", NewCombatRandom(12345))
	if err != nil {
		t.Fatal(err)
	}
	if *first != *second {
		t.Fatal("non-deterministic release")
	}
}

func TestCombatUnsupportedEffectsOwnershipAndInvalidInputsAreAtomic(t *testing.T) {
	s := combatTestTables(t)
	baseA := combatTestActor("hero_001", "a", "001_1,001_2,001_3")
	baseD := combatTestActor("hero_003", "d", "003_1,003_2,003_3")
	cases := []struct {
		name, cast string
		mutate     func(*pb.BattleHero, *pb.BattleHero)
	}{
		{"unknown", "missing", func(a, d *pb.BattleHero) {}},
		{"buff", "1", func(a, d *pb.BattleHero) {}},
		{"foreign skill", "293", func(a, d *pb.BattleHero) {}},
		{"talent", "4", func(a, d *pb.BattleHero) { d.TalentList = []string{"tal_1087"} }},
		{"awake", "4", func(a, d *pb.BattleHero) { a.AwakeStage = proto.Int32(1) }},
		{"dead", "4", func(a, d *pb.BattleHero) { d.Inst.CurHp = proto.Float64(0) }},
		{"same actor", "4", func(a, d *pb.BattleHero) { d.Guid = a.Guid }},
		{"NaN", "4", func(a, d *pb.BattleHero) { a.Prop.Att = proto.Float64(math.NaN()) }},
		{"overheal", "4", func(a, d *pb.BattleHero) { d.Inst.CurHp = proto.Float64(1001) }},
		{"overflow", "4", func(a, d *pb.BattleHero) {
			a.Prop.Att = proto.Float64(math.MaxFloat64)
			a.Prop.Cri = proto.Float64(100)
			a.Prop.CriFactor = proto.Float64(math.MaxFloat64)
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			a := proto.Clone(baseA).(*pb.BattleHero)
			d := proto.Clone(baseD).(*pb.BattleHero)
			test.mutate(a, d)
			rng := NewCombatRandom(0)
			saved := *rng
			_, err := s.EvaluateUnbuffedRelease(a, d, test.cast, rng)
			if err == nil {
				t.Fatal("unsupported input accepted")
			}
			if *rng != saved {
				t.Fatal("rejected input advanced random state")
			}
		})
	}
}
