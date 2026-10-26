package service

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Pin currentHP construction, conditional totalHP accumulation, unconditional
// actor admission and the initially empty battle-death queue. Probe equivalence alone cannot establish an original rule.
func TestP6InitialActorAdmissionOriginalSource(t *testing.T) {
	for _, item := range []struct {
		file  string
		sha   string
		terms []string
	}{
		{"initial-actor-source-v92-original.il.txt", "f70792fd23df03e118f38939af61ed80c2772b2b31a9bb57ec3593553017b392", []string{"006d callvirt ProtoBuf.Message.BattleHeroInst::get_curHp", "0072 stfld .BattleActorData::hp", "0014 ldfld Battle.Engine.InputData.InActorData::hp", "0019 stfld Battle.Engine.LogicModel.FAttribute::hp"}},
		{"initial-actor-admission-v92-original.il.txt", "6ef743ec70b8f322a1a4d96ecf3b28003f2d54ccfe950ceced0df6262d3637aa", []string{"METHOD Battle.Engine.LogicModel.FPlayer::init", "0116 ble.un 307", "0133 ldarg.0 None\n0134 ldfld Battle.Engine.LogicModel.FPlayer::actors\n0139 ldloc.2 None\n013a callvirt Add", "0162 stfld Battle.Engine.LogicModel.FPlayer::deadActors", "006d callvirt Battle.Engine.LogicModel.FBuffProc::LoadBuffByTimer"}},
	} {
		raw, err := readTestAnalysis75(filepath.Join("../../analysis/p6", item.file))
		if err != nil || p6Digest(raw) != item.sha {
			t.Fatal("immutable original actor source", err)
		}
		for _, term := range item.terms {
			if !strings.Contains(string(raw), term) {
				t.Fatal("original actor admission source missing", item.file, term)
			}
		}
	}
}

func p6InitialActorInput(t *testing.T, s *GameService, camp int) *model.FrozenBattleInput {
	t.Helper()
	f := p6Input(t, s, p6Actor("hero_047", "initial-live-a", "047_1,047_2,047_3", 3, 80000), p6Actor("hero_047", "initial-live-d", "047_1,047_2,047_3", 3, 80000), 6)
	dead := p6Actor("hero_047", "initial-zero", "047_1,047_2,047_3", 7, 90000)
	dead.Inst.CurHp = proto.Float64(0)
	// This real original timer1 talent would consume chance RNG and alter its
	// attributes if an initial DEAD actor reached BattleBegin.
	dead.TalentList = []string{"tal_260"}
	if camp == 0 {
		f.Attacker.Formation = append(f.Attacker.Formation, dead)
	} else {
		f.Defender.Formation = append(f.Defender.Formation, dead)
	}
	frozen, err := s.FreezeBattleInput(*f)
	if err != nil {
		t.Fatal(err)
	}
	return frozen
}

func TestP6InitialZeroHPAdmissionDoesNotRunTalentsOrEnterRevivalPool(t *testing.T) {
	s := p6OriginalTables(t)
	for _, camp := range []int{0, 1} {
		f := p6InitialActorInput(t, s, camp)
		before, _ := json.Marshal(f)
		e := &combatRoundEngine{service: &GameService{Tables: f.Tables}, input: f, rng: NewCombatRandom(0), operate: &pb.BattleOperate{}}
		if err := e.initialize(); err != nil {
			t.Fatal(err)
		}
		if len(e.players[camp].actors) != 2 || len(e.players[camp].deadActors) != 0 || e.rng.Draws() != 0 {
			t.Fatal("original initial actor membership/death pool", camp)
		}
		selected, err := e.selectTargets(e.players[camp].actors[0], 130, nil, nil)
		if err != nil || len(selected) != 0 {
			t.Fatal("initial zero HP was available to teammate revival", camp, err)
		}
		actual, err := s.ComputeFrozenBattle(t.Context(), f, nil)
		if err != nil {
			t.Fatal(err)
		}
		baseline := p6Input(t, s, proto.Clone(f.Attacker.Formation[0]).(*pb.BattleHero), proto.Clone(f.Defender.Formation[0]).(*pb.BattleHero), 6)
		expected, err := s.ComputeFrozenBattle(t.Context(), baseline, nil)
		if err != nil || !proto.Equal(actual.Operates, expected.Operates) || actual.RandomDraws != expected.RandomDraws || actual.Winner != expected.Winner || !reflect.DeepEqual(actual.Events, expected.Events) {
			t.Fatal("initially depleted actor altered battle triggers/AI/HP/RNG", camp, err)
		}
		audited, err := s.ComputeFrozenBattleFinalHP(t.Context(), f, nil)
		if err != nil || len(audited.FinalActors) != 3 {
			t.Fatal("initial zero HP original runtime actor was omitted", camp, err)
		}
		for _, actor := range audited.FinalActors {
			if actor.GUID == "initial-zero" && (actor.State != 4 || actor.CurrentHP != 0 || actor.MaxHP != 90000) {
				t.Fatal("original zeroHP runtime actor state/maxHP differs")
			}
		}
		replay, err := s.ComputeFrozenBattle(t.Context(), f, actual.Operates)
		if err != nil || !p6CopySeriesJSONEqual(t, actual, replay) {
			t.Fatal("zero HP input whole replay", camp, err)
		}
		after, _ := json.Marshal(f)
		if !bytes.Equal(before, after) {
			t.Fatal("initial actor admission changed source formation")
		}
	}
}

func TestP6InitialBothCampsZeroHPEndsBeforeAnySkillOrRNG(t *testing.T) {
	s := p6OriginalTables(t)
	f := p6Input(t, s, p6Actor("hero_047", "initial-zero-a", "047_1", 3, 1000), p6Actor("hero_047", "initial-zero-d", "047_1", 3, 1000), 2)
	f.Attacker.Formation[0].Inst.CurHp = proto.Float64(0)
	f.Defender.Formation[0].Inst.CurHp = proto.Float64(0)
	f.Attacker.Formation[0].TalentList = []string{"tal_260"}
	f.Defender.Formation[0].TalentList = []string{"tal_260"}
	audit, err := s.ComputeFrozenBattleFinalHP(t.Context(), f, nil)
	if err != nil {
		t.Fatal(err)
	}
	out := audit.Outcome
	if len(audit.FinalActors) != 2 || out.Winner != 0 || out.FinalRound != 1 || out.RandomDraws != 0 || len(out.Events) != 1 || out.Events[0].Kind != "round_begin" || len(out.Operates.SkillOpts) != 0 || out.Operates.GetAttackerTotalLeftHp() != 0 || out.Operates.GetDefenderTotalLeftHp() != 0 || out.Authoritative {
		t.Fatal("original enemy-empty precedence/zero actor startup", out)
	}
}

// Export two limited original-rule probes for the single device owner. The
// source has original hero/skill/talent rows with an explicit zero currentHP;
// it does not represent an observed formal account or an authoritative winner.
func TestP6ExportInitialZeroHPNativeInputs(t *testing.T) {
	directory := os.Getenv("HYJS_P6_INITIAL_ZERO_HP_OUTPUT")
	if directory == "" {
		t.Skip("explicit zero-HP source export disabled")
	}
	root, err := filepath.EvalSymlinks("../../analysis/p6")
	if err != nil {
		t.Fatal(err)
	}
	root, err = filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	directory, err = filepath.Abs(directory)
	if err != nil || !strings.HasPrefix(directory, root+string(filepath.Separator)) {
		t.Fatal("owned source directory required", err)
	}
	if err = os.MkdirAll(directory, 0755); err != nil {
		t.Fatal(err)
	}
	s := p6OriginalTables(t)
	var cases []map[string]any
	for camp := 0; camp < 2; camp++ {
		f := p6InitialActorInput(t, s, camp)
		wire := &pb.SCFightPlayback{Attacker: f.Attacker, Defender1: f.Defender, RandSeed: proto.Int64(f.Seed), MaxRound: proto.Int32(f.MaxRound), BattleType: proto.Int32(f.BattleType), Fightid: proto.String([]string{"original-initial-zero-camp0-v92", "original-initial-zero-camp1-v92"}[camp]), AttackerFactor: proto.Float64(1), DefenderFactor: proto.Float64(1)}
		data, err := proto.MarshalOptions{Deterministic: true}.Marshal(wire)
		if err != nil {
			t.Fatal(err)
		}
		name := []string{"000.pb", "001.pb"}[camp]
		if err = os.WriteFile(filepath.Join(directory, name), data, 0644); err != nil {
			t.Fatal(err)
		}
		cases = append(cases, map[string]any{"case": wire.GetFightid(), "wire": name, "wireSHA256": p6Digest(data), "seed": f.Seed, "battleType": f.BattleType, "fightId": wire.GetFightid(), "firstCamp": 0, "damageFactors": map[string]float64{"attacker": 1, "defender": 1}, "sourceBoundary": "Original-rule probe with real APK047 skills/tal260, explicit initial currentHP0. Not a formal-account observation; original-auto native evidence pending. No record/predicted winner provided."})
	}
	raw, err := json.MarshalIndent(map[string]any{"scope": "Limited initial zeroHP original PVP automatic rule probes; no formal-account or settlement authority claim", "nativeCompared": false, "authoritative": false, "cases": cases}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, '\n')
	if err = os.WriteFile(filepath.Join(directory, "manifest.json"), raw, 0644); err != nil {
		t.Fatal(err)
	}
	t.Log("initial-zeroHP source manifest", p6Digest(raw))
}
