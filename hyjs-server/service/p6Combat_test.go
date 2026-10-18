package service

import (
	"context"
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"math"
	"strconv"
	"testing"
)

func p6OriginalTables(t *testing.T) *GameService {
	t.Helper()
	// Parse immutable APK tables once, then give each test independent mutable rows.
	p6OriginalTestCache.once.Do(func() {
		p6OriginalTestCache.tables, p6OriginalTestCache.err = readP6OriginalTestTables()
	})
	if p6OriginalTestCache.err != nil {
		t.Fatal(p6OriginalTestCache.err)
	}
	return &GameService{Tables: cloneP6OriginalTestTables(p6OriginalTestCache.tables)}
}

func p6Actor(id, guid, skills string, pose int32, hp float64) *pb.BattleHero {
	a := combatTestActor(id, guid, skills)
	a.Pose = proto.Int32(pose)
	a.Prop.Hp = proto.Float64(hp)
	a.Inst.CurHp = proto.Float64(hp)
	return a
}
func p6Input(t *testing.T, s *GameService, a, d *pb.BattleHero, rounds int32) *model.FrozenBattleInput {
	t.Helper()
	f, err := s.FreezeBattleInput(model.FrozenBattleInput{Attacker: &pb.BattlePlayer{PlayerId: proto.Int64(42), Formation: []*pb.BattleHero{a}, Inst: &pb.BattlePlayerInst{Anger: proto.Float64(0)}}, Defender: &pb.BattlePlayer{PlayerId: proto.Int64(43), Formation: []*pb.BattleHero{d}, Inst: &pb.BattlePlayerInst{Anger: proto.Float64(0)}}, Seed: 0, MaxRound: rounds, BattleType: 2})
	if err != nil {
		t.Fatal(err)
	}
	return f
}
func TestP6OriginalWholeBattleAndOperationReplay(t *testing.T) {
	s := p6OriginalTables(t)
	f := p6Input(t, s, p6Actor("hero_047", "player", "047_1", 1, 1000), p6Actor("hero_047", "enemy", "047_1", 1, 1000), 12)
	saved, _ := json.Marshal(f)
	out, err := s.ComputeFrozenBattle(context.Background(), f, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.Authoritative || out.RandomDraws == 0 || len(out.Operates.SkillOpts) < 3 || out.Winner == 2 {
		t.Fatal("incomplete whole fight", out)
	}
	replay, err := s.ComputeFrozenBattle(context.Background(), f, out.Operates)
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(out.Operates, replay.Operates) || out.RandomDraws != replay.RandomDraws || out.Winner != replay.Winner || !proto.Equal(out.Attacker, replay.Attacker) || !proto.Equal(out.Defender, replay.Defender) {
		t.Fatal("operation replay diverged")
	}
	final, _ := json.Marshal(f)
	if string(saved) != string(final) {
		t.Fatal("simulation mutated frozen input")
	}
	var first *model.CombatEvent
	for i := range out.Events {
		v := &out.Events[i]
		if v.Kind == "release" {
			first = v
			break
		}
	}
	if first == nil || first.Cast != "895" || first.RandomDraws != 4 {
		t.Fatal("original first cast RNG", first)
	}
	closeCombat(t, first.Value, 61.6490091042013)
	if first.Time < .8 || first.Time > 1 {
		t.Fatal("logical .1 movement/release", first.Time)
	}
}
func TestP6RedSkillLifestealOrderAndRoundReset(t *testing.T) {
	s := p6OriginalTables(t)
	a := p6Actor("hero_001", "red", "001_1", 1, 2000)
	a.Inst.CurHp = proto.Float64(1000)
	f := p6Input(t, s, a, p6Actor("hero_047", "enemy", "047_1", 1, 4000), 4)
	out, err := s.ComputeFrozenBattle(context.Background(), f, nil)
	if err != nil {
		t.Fatal(err)
	}
	loads, heals := 0, 0
	for _, v := range out.Events {
		if v.Kind == "buff_load" && v.Buff == "buff_488" {
			loads++
		}
		if v.Kind == "lifesteal" {
			heals++
		}
		if v.Kind == "release" && v.Cast == "1" && v.Round == 1 {
			if v.RandomDraws != 5 {
				t.Fatal("hit-buff must consume RNG before critical", v)
			}
			closeCombat(t, v.Value, 109.0894393105409)
		}
	}
	if loads < 3 || heals != loads {
		t.Fatal("real red skill not simulated", loads, heals)
	}
	replay, err := s.ComputeFrozenBattle(context.Background(), f, out.Operates)
	if err != nil || replay.RandomDraws != out.RandomDraws || !proto.Equal(out.Operates, replay.Operates) {
		t.Fatal("red replay", err)
	}
}
func TestP6FreezeInvalidOwnershipMutationAndCancelledContext(t *testing.T) {
	s := p6OriginalTables(t)
	f := p6Input(t, s, p6Actor("hero_047", "player", "047_1", 1, 1000), p6Actor("hero_047", "enemy", "047_1", 1, 1000), 2)
	originalHash := f.ConfigurationHash
	s.table("SkillCast", "SkillCastId", "895")["ExportFactor"] = "9999"
	out, err := s.ComputeFrozenBattle(context.Background(), f, nil)
	if err != nil || f.ConfigurationHash != originalHash {
		t.Fatal("live config entered frozen fight", err)
	}
	bad := proto.Clone(out.Operates).(*pb.BattleOperate)
	bad.SkillOpts[0].ActorUid = proto.String("0_foreign")
	if _, err = s.ComputeFrozenBattle(context.Background(), f, bad); err == nil {
		t.Fatal("foreign actor accepted")
	}
	bad = proto.Clone(out.Operates).(*pb.BattleOperate)
	bad.SkillOpts[0].Camp = proto.Int32(1)
	bad.SkillOpts[0].ActorUid = proto.String("1_enemy")
	if _, err = s.ComputeFrozenBattle(context.Background(), f, bad); err == nil {
		t.Fatal("inactive camp accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = s.ComputeFrozenBattle(ctx, f, nil); err != context.Canceled {
		t.Fatal(err)
	}
	f.Tables["SkillCast"][0]["ExportFactor"] = "9999"
	if _, err = s.ComputeFrozenBattle(context.Background(), f, nil); err == nil {
		t.Fatal("tampered hash accepted")
	}
}
func TestP6BuffStacksCoverDurationShieldAndStaticNoDoubleApply(t *testing.T) {
	s := p6OriginalTables(t)
	a := p6Actor("hero_047", "player", "047_1", 1, 1000)
	a.TalentList = []string{"tal_1"}
	f := p6Input(t, s, a, p6Actor("hero_047", "enemy", "047_1", 1, 1000), 4)
	e := &combatRoundEngine{service: &GameService{Tables: f.Tables}, input: f, rng: NewCombatRandom(0), operate: &pb.BattleOperate{}}
	if err := e.initialize(); err != nil {
		t.Fatal(err)
	}
	actor := e.players[0].actors[0]
	if actor.attribute("att") != 500 {
		t.Fatal("static timer0 applied twice")
	}
	for _, x := range []float64{10, 20, 5} {
		row := map[string]string{"Id": "stack", "Group": "atk", "ShouldCover": "1", "BuffClass": "0", "TotalRound": "1", "Factor": "peratt", "Args1": strconv.FormatFloat(x, 'f', -1, 64)}
		if err := e.loadBuff(actor, actor, row, 4); err != nil {
			t.Fatal(err)
		}
	}
	if len(actor.buffs) != 1 || actor.attribute("att") != 600 {
		t.Fatal("cover/strongest mismatch", actor.attribute("att"))
	}
	shield := map[string]string{"Id": "shield", "Factor": "sourceperattToshield", "Args1": "50", "BuffClass": "1", "TotalRound": "2"}
	if err := e.loadBuff(actor, actor, shield, 4); err != nil {
		t.Fatal(err)
	}
	e.damageHP(e.players[1].actors[0], actor, 350, 1)
	closeCombat(t, actor.hp, 950)
	if actor.shield("shield") != 0 {
		t.Fatal("shield not consumed")
	}
	if err := e.endRound(); err != nil {
		t.Fatal(err)
	}
	if actor.attribute("att") != 500 {
		t.Fatal("round expiration failed")
	}
}
func TestP6CapabilitiesRejectUnsupportedDynamicsAndNonfinite(t *testing.T) {
	s := p6OriginalTables(t)
	f := p6Input(t, s, p6Actor("hero_047", "player", "047_1", 1, 1000), p6Actor("hero_047", "enemy", "047_1", 1, 1000), 2)
	f.Attacker.Formation[0].TalentList = []string{"missing"}
	if _, err := s.ComputeFrozenBattle(context.Background(), f, nil); err == nil {
		t.Fatal("unknown dynamic ignored")
	}
	f.Attacker.Formation[0].TalentList = nil
	f.Attacker.Formation[0].AwakeStage = proto.Int32(1)
	if _, err := s.ComputeFrozenBattle(context.Background(), f, nil); err == nil {
		t.Fatal("awake ignored")
	}
	f.Attacker.Formation[0].AwakeStage = nil
	f.Attacker.Formation[0].Prop.Hp = proto.Float64(math.NaN())
	if _, err := s.ComputeFrozenBattle(context.Background(), f, nil); err == nil {
		t.Fatal("NaN accepted")
	}
	if constUtil.P6EngineVersion == "" {
		t.Fatal("missing version")
	}
}

func TestP6OriginalNegativeMonsterDodgeIsValidFrozenInput(t *testing.T) {
	s := p6OriginalTables(t)
	enemyRow := s.table("CopyEnemy", "CopyEnemyId", "1")
	enemy, err := s.actor(enemyRow, "monster-1", 3, true)
	if err != nil {
		t.Fatal(err)
	}
	if enemy.Prop.GetDodge() != -5 {
		t.Fatal("original enemy changed")
	}
	a := p6Actor("hero_047", "player", "047_1", 1, 1000)
	input, err := s.FreezeBattleInput(model.FrozenBattleInput{Attacker: &pb.BattlePlayer{PlayerId: proto.Int64(42), Formation: []*pb.BattleHero{a}}, Monster: &pb.BattleMonster{Formation: []*pb.BattleHero{enemy}}, Seed: 0, MaxRound: 40, BattleType: 1})
	if err != nil {
		t.Fatal("original legal signed dodge rejected", err)
	}
	if input.Monster.Formation[0].Prop.GetDodge() != -5 {
		t.Fatal("signed original value was normalized")
	}
}
