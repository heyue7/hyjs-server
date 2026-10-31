package service

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestGGOBossFormation87OriginalRoster(t *testing.T) {
	s := &GameService{}
	if err := s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	event := &ggoBossEvent{ID: "formation87", Boss: defaultGGORuntime().Boss}
	event.Boss.HeroID = "monster_047"
	event.Boss.Level, event.Boss.HP, event.Boss.Attack = 80, 1000, 4500
	event.Boss.PhyDefend, event.Boss.MagicDefend = 1200, 1300
	monster, err := s.ggoBossMonster(event, 400)
	if err != nil || len(monster.GetFormation()) != 4 {
		t.Fatal("original roster", err)
	}
	actors := map[int32]*pb.BattleHero{}
	guids := map[string]bool{}
	for _, actor := range monster.Formation {
		if actors[actor.GetPose()] != nil || actor.GetGuid() == "" || guids[actor.GetGuid()] {
			t.Fatal("duplicate position or GUID", actor)
		}
		actors[actor.GetPose()] = actor
		guids[actor.GetGuid()] = true
	}
	leader := actors[13]
	if leader == nil || leader.GetId() != "monster_047" || leader.GetGuid() != ggoBossGUID(event.ID) ||
		leader.GetLevel() != 80 || leader.GetProp().GetHp() != 1000 || leader.GetInst().GetCurHp() != 400 ||
		leader.GetProp().GetAtt() != 4500 || leader.GetProp().GetPdef() != 1200 || leader.GetProp().GetMdef() != 1300 ||
		leader.GetSkillOrder() != "1047_1,,1047_3" || !reflect.DeepEqual(leader.GetTalentList(), []string{"tal_1011", "tal_3162"}) {
		t.Fatal("original leader and frozen event parameters", leader)
	}
	for _, pose := range []int32{7, 9, 3} {
		actor := actors[pose]
		if actor == nil || actor.GetId() != "monster_033" || actor.GetLevel() != 60 ||
			actor.GetProp().GetHp() != 573045 || actor.GetInst().GetCurHp() != 573045 || actor.GetProp().GetAtt() != 37500 ||
			actor.GetProp().GetPdef() != 7000 || actor.GetProp().GetMdef() != 7000 || actor.GetSkillOrder() != "1033_1,," ||
			!reflect.DeepEqual(actor.GetTalentList(), []string{"tal_1056", "tal_3162"}) {
			t.Fatal("original attendant baseline", pose, actor)
		}
	}
	// The next encounter retains the same slots and their shared event health.
	again, err := s.ggoBossMonsterWithHealth(event, 300, map[string]float64{ggoBossGUID(event.ID) + "-7": 0, ggoBossGUID(event.ID) + "-9": 100, ggoBossGUID(event.ID) + "-3": 573045})
	if err != nil {
		t.Fatal(err)
	}
	want := map[int32]float64{13: 300, 7: 0, 9: 100, 3: 573045}
	for _, actor := range again.Formation {
		if actor.GetInst().GetCurHp() != want[actor.GetPose()] {
			t.Fatal("shared encounter health", actor)
		}
	}
}

func TestGGOBossFormation87EventModelSelection(t *testing.T) {
	s := &GameService{}
	if err := s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	event := &ggoBossEvent{ID: "legacy-event87", Boss: defaultGGORuntime().Boss}
	event.Boss.HeroID = "hero_017"
	event.Boss.Level, event.Boss.HP, event.Boss.Attack = 65, 1000, 4500
	event.Boss.PhyDefend, event.Boss.MagicDefend = 1200, 1300
	monster, err := s.ggoBossMonster(event, 400)
	if err != nil || len(monster.GetFormation()) != 1 {
		t.Fatal("legacy event must keep single combatant", err)
	}
	actor := monster.Formation[0]
	hero := s.table("Hero", "id", "hero_017")
	wantSkills := fmt.Sprintf("%s,%s,%s", hero["SkillId1"], hero["SkillId2"], hero["UltSkillId"])
	if actor.GetId() != "hero_017" || actor.GetPose() != 13 || actor.GetGuid() != ggoBossGUID(event.ID) ||
		actor.GetLevel() != 65 || actor.GetProp().GetHp() != 1000 || actor.GetInst().GetCurHp() != 400 ||
		actor.GetProp().GetAtt() != 4500 || actor.GetProp().GetPdef() != 1200 || actor.GetProp().GetMdef() != 1300 ||
		actor.GetSkillOrder() != wantSkills || len(actor.GetTalentList()) != 0 {
		t.Fatal("legacy event model, panel or skills changed", actor)
	}
	for _, unknown := range []string{"", "hero_018", "monster_033"} {
		event.Boss.HeroID = unknown
		if _, err := s.ggoBossMonster(event, 400); err == nil {
			t.Fatalf("unknown event model %q accepted", unknown)
		}
	}
}

func TestGGOBossFormation87RetainedLegacyEventStartsSingle(t *testing.T) {
	t.Setenv("HYJS_P1_MYSQL_DATABASE", "")
	t.Setenv("HYJS_GAME_ACCOUNTS_DATABASE", "")
	f, ctx, _, event := ggoBossIntegrationFixture(t)
	newMaximum := event.Boss.HP
	oldMaximum, oldCurrent := newMaximum*4, float64(newMaximum*2)
	var frozenRules []byte
	if err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row db.SharedFieldBoss55
		if err := sharedFieldBoss55Query(tx, f.s.OriginalServerID(), constUtil.GGOFieldScene, constUtil.GGOFieldBossPosition, event.Line).Take(&row).Error; err != nil {
			return err
		}
		old, boss, hp, err := ggoBossDecode(&row)
		if err != nil {
			return err
		}
		old.Boss.HeroID, old.Boss.HP = "hero_017", oldMaximum
		boss.HeroId, boss.CurHp = proto.String("hero_017"), proto.Float64(oldCurrent/float64(oldMaximum))
		hp[ggoBossGUID(event.ID)] = oldCurrent
		row.GGOEvent, err = json.Marshal(old)
		if err != nil {
			return err
		}
		frozenRules = append([]byte(nil), row.GGOEvent...)
		return saveSharedFieldBoss55(tx, &row, boss, hp)
	}); err != nil {
		t.Fatal("prepare retained event", err)
	}
	messages, err := f.s.fieldBossFightStart(ctx, f.id, &pb.CSSidePveBossFightStart{Uuid: proto.String(event.ID)})
	if err != nil {
		t.Fatal("start retained legacy event", err)
	}
	start := messages[0].(*pb.SCSidePveBossFightStart)
	if len(start.BattleMonsterList) != 1 || len(start.BattleMonsterList[0].Formation) != 1 ||
		start.BattleMonsterList[0].Formation[0].GetId() != "hero_017" || start.BattleMonsterList[0].Formation[0].GetInst().GetCurHp() != oldCurrent {
		t.Fatal("retained event did not start its frozen model", start)
	}
	var after db.SharedFieldBoss55
	if err = sharedFieldBoss55Query(db.GetIns(), f.s.OriginalServerID(), constUtil.GGOFieldScene, constUtil.GGOFieldBossPosition, event.Line).Take(&after).Error; err != nil {
		t.Fatal(err)
	}
	retained, projection, hp, err := ggoBossDecode(&after)
	if err != nil || retained.ID != event.ID || retained.Boss.HeroID != "hero_017" || retained.Boss.HP != oldMaximum ||
		projection.GetHeroId() != "hero_017" || hp[ggoBossGUID(event.ID)] != oldCurrent || !bytes.Equal(after.GGOEvent, frozenRules) {
		t.Fatal("retained event changed", err)
	}
}

func ggoBossFormation87TerminalFixture(single bool) (*model.FieldFight, *model.FrozenBattleInput, *model.CombatFinalHPOutcome) {
	const eventID = "terminal87"
	hero := func(guid string, hp, current float64) *pb.BattleHero {
		return &pb.BattleHero{Guid: proto.String(guid), Prop: &pb.BaseProp{Hp: proto.Float64(hp)}, Inst: &pb.BattleHeroInst{CurHp: proto.Float64(current)}}
	}
	leaderGUID := ggoBossGUID(eventID)
	leader := hero(leaderGUID, 100, 70)
	formation := []*pb.BattleHero{leader}
	final := []model.CombatFinalActorHP{{Camp: 1, GUID: leaderGUID, MaxHP: 100, CurrentHP: 50}}
	if !single {
		for _, guid := range []string{"attendant-7", "attendant-9", "attendant-3"} {
			formation = append(formation, hero(guid, 1000, 1000))
			final = append(final, model.CombatFinalActorHP{Camp: 1, GUID: guid, MaxHP: 1000, CurrentHP: 0})
		}
		// Final order can differ from the original formation order.
		final[0], final[3] = final[3], final[0]
	}
	return &model.FieldFight{GGOBoss: &model.GGOFieldBossFight{EventID: eventID}},
		&model.FrozenBattleInput{Monster: &pb.BattleMonster{Formation: formation}},
		&model.CombatFinalHPOutcome{FinalActors: final}
}

func TestGGOBossFormation87LeaderDamageAndValidation(t *testing.T) {
	for _, single := range []bool{true, false} {
		fight, input, terminal := ggoBossFormation87TerminalFixture(single)
		if damage, err := ggoBossLeaderDamage(fight, input, terminal); err != nil || damage != 20 {
			t.Fatal("leader-only damage", single, damage, err)
		}
	}
	mutations := map[string]func(*model.FieldFight, *model.FrozenBattleInput, *model.CombatFinalHPOutcome){
		"wrong leader": func(f *model.FieldFight, _ *model.FrozenBattleInput, _ *model.CombatFinalHPOutcome) {
			f.BossLeaderGUID = "other"
		},
		"missing leader": func(_ *model.FieldFight, i *model.FrozenBattleInput, _ *model.CombatFinalHPOutcome) {
			i.Monster.Formation[0].Guid = proto.String("other")
		},
		"duplicate frozen GUID": func(_ *model.FieldFight, i *model.FrozenBattleInput, _ *model.CombatFinalHPOutcome) {
			i.Monster.Formation[1].Guid = i.Monster.Formation[0].Guid
		},
		"empty frozen GUID": func(_ *model.FieldFight, i *model.FrozenBattleInput, _ *model.CombatFinalHPOutcome) {
			i.Monster.Formation[1].Guid = proto.String("")
		},
		"invalid frozen max": func(_ *model.FieldFight, i *model.FrozenBattleInput, _ *model.CombatFinalHPOutcome) {
			i.Monster.Formation[1].Prop.Hp = proto.Float64(math.Inf(1))
		},
		"invalid frozen current": func(_ *model.FieldFight, i *model.FrozenBattleInput, _ *model.CombatFinalHPOutcome) {
			i.Monster.Formation[1].Inst.CurHp = proto.Float64(1001)
		},
		"missing enemy": func(_ *model.FieldFight, _ *model.FrozenBattleInput, o *model.CombatFinalHPOutcome) {
			o.FinalActors = o.FinalActors[:3]
		},
		"extra enemy": func(_ *model.FieldFight, _ *model.FrozenBattleInput, o *model.CombatFinalHPOutcome) {
			o.FinalActors = append(o.FinalActors, model.CombatFinalActorHP{Camp: 1, GUID: "foreign", MaxHP: 1})
		},
		"duplicate enemy": func(_ *model.FieldFight, _ *model.FrozenBattleInput, o *model.CombatFinalHPOutcome) {
			o.FinalActors[1] = o.FinalActors[0]
		},
		"invalid camp": func(_ *model.FieldFight, _ *model.FrozenBattleInput, o *model.CombatFinalHPOutcome) {
			o.FinalActors = append(o.FinalActors, model.CombatFinalActorHP{Camp: 2, GUID: "invalid", MaxHP: 1})
		},
		"wrong max": func(_ *model.FieldFight, _ *model.FrozenBattleInput, o *model.CombatFinalHPOutcome) {
			o.FinalActors[1].MaxHP++
		},
		"nonfinite current": func(_ *model.FieldFight, _ *model.FrozenBattleInput, o *model.CombatFinalHPOutcome) {
			o.FinalActors[1].CurrentHP = math.NaN()
		},
		"over max": func(_ *model.FieldFight, _ *model.FrozenBattleInput, o *model.CombatFinalHPOutcome) {
			o.FinalActors[1].CurrentHP = o.FinalActors[1].MaxHP + 1
		},
		"negative current": func(_ *model.FieldFight, _ *model.FrozenBattleInput, o *model.CombatFinalHPOutcome) {
			o.FinalActors[1].CurrentHP = -1
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			fight, input, terminal := ggoBossFormation87TerminalFixture(false)
			mutate(fight, input, terminal)
			if _, err := ggoBossLeaderDamage(fight, input, terminal); err == nil {
				t.Fatal("invalid enemy accepted")
			}
		})
	}
}

func TestGGOBossFormation87NativeStartEndAndColdReceipt(t *testing.T) {
	t.Setenv("HYJS_P1_MYSQL_DATABASE", "")
	t.Setenv("HYJS_GAME_ACCOUNTS_DATABASE", "")
	f, ctx, _, event := ggoBossIntegrationFixture(t)
	var before db.SharedFieldBoss55
	if err := sharedFieldBoss55Query(db.GetIns(), f.s.OriginalServerID(), constUtil.GGOFieldScene, constUtil.GGOFieldBossPosition, event.Line).Take(&before).Error; err != nil {
		t.Fatal(err)
	}
	messages, err := f.s.fieldBossFightStart(ctx, f.id, &pb.CSSidePveBossFightStart{Uuid: proto.String(event.ID)})
	if err != nil {
		t.Fatal("native start", err)
	}
	start := messages[0].(*pb.SCSidePveBossFightStart)
	if len(start.BattleMonsterList) != 1 || len(start.BattleMonsterList[0].Formation) != 4 {
		t.Fatal("native four-member start", start)
	}
	var profile db.Profile
	if err = db.GetIns().First(&profile, f.id).Error; err != nil {
		t.Fatal(err)
	}
	_, game, err := decodeProfile(&profile)
	if err != nil {
		t.Fatal(err)
	}
	input, err := fieldBossFrozenInput(game.Field.Fight)
	if err != nil {
		t.Fatal(err)
	}
	terminal, _, err := f.s.ComputeP6GenericMechanismBattle(ctx, input, nil)
	if err != nil || terminal == nil || terminal.OriginalOperates == nil {
		t.Fatal("original four-member battle calculation", err)
	}
	wantDamage, err := ggoBossLeaderDamage(game.Field.Fight, input, terminal)
	if err != nil {
		t.Fatal(err)
	}
	result := int32(0)
	if terminal.Outcome.Winner == 0 {
		result = 1
	}
	badRecord := proto.Clone(terminal.OriginalOperates).(*pb.BattleOperate)
	badRecord.DefenderTotalLeftHp = proto.Float64(badRecord.GetDefenderTotalLeftHp() + 1)
	if _, err = f.s.ComputeOriginalP6ProtocolFinalHP(ctx, input, badRecord); err == nil {
		t.Fatal("invalid recorded HP unexpectedly passed strict replay")
	}
	fallback, fallbackResult, err := f.s.ggoBattleTerminal(ctx, input, badRecord, 1-result, nil, f.id, false)
	if err != nil || fallbackResult != result || fallback.OriginalRecordSHA256 != "" || !reflect.DeepEqual(fallback.FinalActors, terminal.FinalActors) {
		t.Fatal("four-member invalid-record fallback", fallbackResult, err)
	}
	request := &pb.CSSidePveBossFightEnd{FightId: start.FightId, Uuid: proto.String(event.ID), Result: proto.Int32(1 - result), OptRecord: badRecord}
	ended, err := f.s.fieldBossFightEnd(ctx, f.id, request)
	if err != nil {
		t.Fatal("native end", err)
	}
	native := ended[0].(*pb.SCSidePveBossFightEnd)
	if native.GetCode() != 1 {
		t.Fatal("native end code", native)
	}
	var after db.SharedFieldBoss55
	if err = sharedFieldBoss55Query(db.GetIns(), f.s.OriginalServerID(), constUtil.GGOFieldScene, constUtil.GGOFieldBossPosition, event.Line).Take(&after).Error; err != nil {
		t.Fatal(err)
	}
	beforeEvent, _, _, err := ggoBossDecode(&before)
	if err != nil {
		t.Fatal(err)
	}
	afterEvent, _, hp, err := ggoBossDecode(&after)
	if err == nil {
		beforeEvent.AttendantHP, afterEvent.AttendantHP = nil, nil
	}
	if err != nil || !reflect.DeepEqual(beforeEvent, afterEvent) || hp[ggoBossGUID(event.ID)] != float64(event.Boss.HP)-wantDamage {
		t.Fatal("shared leader HP and frozen event", hp, wantDamage, err)
	}
	var member db.FieldBossContribution73
	if err = db.GetIns().Where("server_id=? AND spawn_uuid=? AND player_id=?", f.s.OriginalServerID(), event.ID, f.id).Take(&member).Error; err != nil || member.Damage != wantDamage {
		t.Fatal("leader contribution", member.Damage, wantDamage, err)
	}
	replayed, err := f.s.fieldBossFightEnd(ctx, f.id, request)
	if err != nil || !proto.Equal(native, replayed[0]) {
		t.Fatal("idempotent native receipt", err)
	}
	f.restart(t)
	replayed, err = f.s.fieldBossFightEnd(ctx, f.id, request)
	if err != nil || !proto.Equal(native, replayed[0]) {
		t.Fatal("cold native receipt", err)
	}
}

// Optional evidence from a separately run original-assembly host. This measures
// whether its actual four-enemy record passes this server's replay gate; it
// does not replace a phone test or claim numerical equivalence.
func TestGGOBossFormation87OriginalHostRecordGate(t *testing.T) {
	root := os.Getenv("HYJS_GGO_ORIGINAL_RECORD_DIR")
	if root == "" {
		t.Skip("explicit original host evidence directory")
	}
	s := &GameService{}
	for _, load := range []func(string) error{s.LoadTables, s.LoadP5Tables, s.LoadP8Tables, s.LoadP12WorldBossTables} {
		if err := load("../../analysis/tables"); err != nil {
			t.Fatal(err)
		}
	}
	for _, sample := range []string{"team1/t1-p1", "team1/t1-p2", "team1/t1-p3", "team1/t1-p4", "team1/t1-p5", "team2/t2-p1", "team2/t2-p2"} {
		t.Run(sample, func(t *testing.T) {
			wireRaw, err := os.ReadFile(filepath.Join(root, sample+".pb"))
			if err != nil {
				t.Fatal(err)
			}
			var wire pb.SCFightPlayback
			if err = proto.Unmarshal(wireRaw, &wire); err != nil || wire.GetBattleType() != constUtil.FieldBossBattleType || len(wire.GetDefender2().GetFormation()) != 4 {
				t.Fatal("original host frozen input", err)
			}
			input, err := s.freezeAndQualifyP6GenericMechanisms(t.Context(), model.FrozenBattleInput{Attacker: wire.Attacker, Monster: wire.Defender2, Seed: wire.GetRandSeed(), MaxRound: wire.GetMaxRound(), BattleType: wire.GetBattleType(), AttackerDamageFactor: wire.AttackerFactor, DefenderDamageFactor: wire.DefenderFactor})
			if err != nil {
				t.Fatal("qualify frozen input", err)
			}
			original, err := os.Open(filepath.Join(root, sample+".native.jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			defer original.Close()
			type nativeLine struct {
				Kind        string `json:"kind"`
				AttackerWon bool   `json:"attackerWon"`
				Data        struct {
					ProtobufBase64 string `json:"protobufBase64"`
				} `json:"data"`
				State struct {
					Defender struct {
						Actors []struct {
							UID       string `json:"uid"`
							Attribute struct {
								HP float64 `json:"hp"`
							} `json:"attribute"`
						} `json:"actors"`
					} `json:"defender"`
				} `json:"state"`
			}
			var nativeLeader float64
			var recordRaw []byte
			var hasLeader, hasResult, attackerWon bool
			scanner := bufio.NewScanner(original)
			scanner.Buffer(make([]byte, 4096), 4*1024*1024)
			for scanner.Scan() {
				var last nativeLine
				if err = json.Unmarshal(scanner.Bytes(), &last); err != nil {
					t.Fatal(err)
				}
				if last.Kind == "state" {
					for _, actor := range last.State.Defender.Actors {
						if actor.UID == "1_"+wire.Defender2.Formation[0].GetGuid() {
							nativeLeader, hasLeader = actor.Attribute.HP, true
						}
					}
				}
				if last.Kind == "result" {
					attackerWon, hasResult = last.AttackerWon, true
				}
				if last.Kind == "original_final_operates" {
					recordRaw, err = base64.StdEncoding.Strict().DecodeString(last.Data.ProtobufBase64)
					if err != nil {
						t.Fatal("original host record", err)
					}
				}
			}
			if err = scanner.Err(); err != nil || !hasLeader || !hasResult || len(recordRaw) == 0 {
				t.Fatal("original host terminal", err)
			}
			var record pb.BattleOperate
			if err = proto.Unmarshal(recordRaw, &record); err != nil {
				t.Fatal("original host record decoding", err)
			}
			reported := int32(0)
			if attackerWon {
				reported = 1
			}
			strict, strictErr := s.ComputeOriginalP6ProtocolFinalHP(t.Context(), input, &record)
			selected, outcome, err := s.ggoBattleTerminal(t.Context(), input, &record, reported, nil, wire.Attacker.GetPlayerId(), false)
			if err != nil {
				t.Fatal("record or automatic settlement calculation", err)
			}
			path := "automatic-fallback"
			if selected.OriginalRecordSHA256 != "" {
				path = "recorded"
				if strictErr != nil || strict == nil || outcome != reported {
					t.Fatal("record selected without strict replay", strictErr)
				}
			} else {
				automatic, err := s.ComputeOriginalP6ProtocolFinalHP(t.Context(), input, nil)
				if err != nil || !reflect.DeepEqual(automatic.FinalActors, selected.FinalActors) {
					t.Fatal("fallback did not preserve the automatic terminal", err)
				}
			}
			var selectedLeader float64
			for _, actor := range selected.FinalActors {
				if actor.Camp == 1 && actor.GUID == wire.Defender2.Formation[0].GetGuid() {
					selectedLeader = actor.CurrentHP
				}
			}
			t.Logf("seed=%d path=%s nativeLeaderHP=%.9f selectedLeaderHP=%.9f nativeResult=%d selectedResult=%d inputSHA=%s recordSHA=%s strictErr=%v", wire.GetRandSeed(), path, nativeLeader, selectedLeader, reported, outcome, p6Digest(wireRaw), p6Digest(recordRaw), strictErr)
		})
	}
}

func TestGGOBossFormation87OldSingleFrozenEnd(t *testing.T) {
	t.Setenv("HYJS_P1_MYSQL_DATABASE", "")
	t.Setenv("HYJS_GAME_ACCOUNTS_DATABASE", "")
	f, ctx, _, event := ggoBossIntegrationFixture(t)
	messages, err := f.s.fieldBossFightStart(ctx, f.id, &pb.CSSidePveBossFightStart{Uuid: proto.String(event.ID)})
	if err != nil {
		t.Fatal("native start", err)
	}
	start := messages[0].(*pb.SCSidePveBossFightStart)
	// Recreate the pre-change single-actor snapshot, including its old model,
	// original hero skills and absence of talents, without changing the event.
	base := f.s.fieldBossLeaderInScene(constUtil.GGOFieldBossID, constUtil.GGOFieldScene)
	hero := f.s.table("Hero", "id", "hero_017")
	row := make(map[string]string, len(base))
	for k, v := range base {
		row[k] = v
	}
	row["HeroId"] = "hero_017"
	row["Level"], row["Hp"], row["Attack"] = fmt.Sprint(event.Boss.Level), fmt.Sprint(event.Boss.HP), fmt.Sprint(event.Boss.Attack)
	row["PhyDefend"], row["MagicDefend"] = fmt.Sprint(event.Boss.PhyDefend), fmt.Sprint(event.Boss.MagicDefend)
	for _, k := range []string{"SkillId1", "SkillId2", "UltSkillId"} {
		row[k] = hero[k]
	}
	for i := 0; i < 6; i++ {
		delete(row, fmt.Sprintf("Talent%d", i))
	}
	oldActor, err := f.s.actor(row, ggoBossGUID(event.ID), 13, true)
	if err != nil {
		t.Fatal(err)
	}
	oldMonster := &pb.BattleMonster{MonsterId: proto.Int64(1), ServerId: proto.Int32(1), TaskId: proto.String(constUtil.GGOFieldBossID), GroupIndex: proto.Int32(1), Formation: []*pb.BattleHero{oldActor}, Inst: &pb.BattleMonsterInst{Anger: proto.Float64(0)}}
	if err = updateProfile(ctx, f.id, func(_ *pb.Player, game *model.GameState) error {
		fight := game.Field.Fight
		input, err := fieldBossFrozenInput(fight)
		if err != nil {
			return err
		}
		source := *input
		source.Monster = oldMonster
		oldInput, err := f.s.freezeAndQualifyP6GenericMechanisms(ctx, source)
		if err != nil {
			return err
		}
		raw, err := json.Marshal(oldInput)
		if err != nil {
			return err
		}
		fight.BossInput = oldInput
		fight.BossInputStored, fight.BossInputHash = nil, ""
		fight.GGOBoss.InputHash = p6Digest(raw)
		fight.BossStart.BattleMonsterList = []*pb.BattleMonster{oldMonster}
		fight.Start.BattleMonsterList = []*pb.BattleMonster{oldMonster}
		return nil
	}); err != nil {
		t.Fatal("persist legacy fixture", err)
	}
	f.restart(t)
	resumed, err := f.s.fieldBossFightStart(ctx, f.id, &pb.CSSidePveBossFightStart{Uuid: proto.String(event.ID)})
	if err != nil {
		t.Fatal("resume legacy frozen start", err)
	}
	resumedStart := resumed[0].(*pb.SCSidePveBossFightStart)
	if resumedStart.GetFightId() != start.GetFightId() || len(resumedStart.BattleMonsterList) != 1 || len(resumedStart.BattleMonsterList[0].Formation) != 1 || resumedStart.BattleMonsterList[0].Formation[0].GetId() != "hero_017" {
		t.Fatal("legacy start changed after cold resume", resumedStart)
	}
	var profile db.Profile
	if err = db.GetIns().First(&profile, f.id).Error; err != nil {
		t.Fatal(err)
	}
	_, game, err := decodeProfile(&profile)
	if err != nil {
		t.Fatal(err)
	}
	input, err := fieldBossFrozenInput(game.Field.Fight)
	if err != nil || len(input.Monster.GetFormation()) != 1 || input.Monster.Formation[0].GetId() != "hero_017" {
		t.Fatal("legacy frozen input", err)
	}
	terminal, _, err := f.s.ComputeP6GenericMechanismBattle(ctx, input, nil)
	if err != nil || terminal == nil || terminal.OriginalOperates == nil {
		t.Fatal("legacy original battle", err)
	}
	result := int32(0)
	if terminal.Outcome.Winner == 0 {
		result = 1
	}
	request := &pb.CSSidePveBossFightEnd{FightId: start.FightId, Uuid: proto.String(event.ID), Result: proto.Int32(result), OptRecord: terminal.OriginalOperates}
	ended, err := f.s.fieldBossFightEnd(ctx, f.id, request)
	if err != nil || ended[0].(*pb.SCSidePveBossFightEnd).GetCode() != 1 {
		t.Fatal("legacy frozen native end", err)
	}
}

func TestGGOBossFormation87SharedLastHitRollbackAndLines(t *testing.T) {
	t.Setenv("HYJS_P1_MYSQL_DATABASE", "")
	t.Setenv("HYJS_GAME_ACCOUNTS_DATABASE", "")
	f, ctx, _, event := ggoBossIntegrationFixture(t)
	var lineTwo db.SharedFieldBoss55
	if err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, _, _, _, err := f.s.ggoBossCurrent(tx, f.s.GGOFieldConfig, time.Now().UnixMilli(), true, 2)
		if err == nil {
			lineTwo = *row
		}
		return err
	}); err != nil {
		t.Fatal("second line event", err)
	}
	monster, err := f.s.ggoBossMonster(event, 100)
	if err != nil {
		t.Fatal(err)
	}
	query := func(tx *gorm.DB, line int64) *gorm.DB {
		return sharedFieldBoss55Query(tx.Model(&db.SharedFieldBoss55{}), f.s.OriginalServerID(), constUtil.GGOFieldScene, constUtil.GGOFieldBossPosition, line)
	}
	if err = query(db.GetIns(), event.Line).Update("hp", adminJSON(map[string]float64{ggoBossGUID(event.ID): 100})).Error; err != nil {
		t.Fatal(err)
	}
	rules, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	member := db.FieldBossContribution73{ServerID: f.s.OriginalServerID(), SpawnUUID: event.ID, PlayerID: f.id, Damage: float64(event.Boss.HP) - 100, Rules: rules, DailyLimit: 3}
	if err = db.GetIns().Create(&member).Error; err != nil {
		t.Fatal(err)
	}
	fight := &model.FieldFight{BossLeaderGUID: ggoBossGUID(event.ID), GGOBoss: &model.GGOFieldBossFight{EventID: event.ID, Line: event.Line, Boss: &pb.SideBoss{Uuid: proto.String(event.ID)}}}
	input := &model.FrozenBattleInput{Monster: monster}
	terminal := &model.CombatFinalHPOutcome{}
	for _, actor := range monster.Formation {
		hp := float64(0)
		if actor.GetGuid() == ggoBossGUID(event.ID) {
			hp = 100
		}
		terminal.FinalActors = append(terminal.FinalActors, model.CombatFinalActorHP{Camp: 1, GUID: actor.GetGuid(), CurrentHP: hp, MaxHP: actor.GetProp().GetHp()})
	}
	// Attendant damage does not change leader HP or the contribution ledger.
	if err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		_, damage, err := f.s.ggoBossApplyDamageLocked(tx, f.id, fight, input, terminal, time.Now().UnixMilli())
		if err != nil || damage != 0 {
			return fmt.Errorf("attendant-only damage %v: %w", damage, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for i := range terminal.FinalActors {
		if terminal.FinalActors[i].GUID == ggoBossGUID(event.ID) {
			terminal.FinalActors[i].CurrentHP = 0
		} else {
			terminal.FinalActors[i].CurrentHP = terminal.FinalActors[i].MaxHP
		}
	}
	forced := errors.New("test rollback")
	apply := func(abort bool) error {
		return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			_, damage, err := f.s.ggoBossApplyDamageLocked(tx, f.id, fight, input, terminal, time.Now().UnixMilli())
			if err != nil {
				return err
			}
			if damage != 100 {
				return fmt.Errorf("last-hit damage: %v", damage)
			}
			if abort {
				return forced
			}
			return nil
		})
	}
	if err = apply(true); !errors.Is(err, forced) {
		t.Fatal("rollback transaction", err)
	}
	var before db.SharedFieldBoss55
	if err = query(db.GetIns(), event.Line).Take(&before).Error; err != nil {
		t.Fatal(err)
	}
	_, _, hp, err := ggoBossDecode(&before)
	if err != nil || hp[ggoBossGUID(event.ID)] != 100 || before.KillerID != 0 {
		t.Fatal("rollback shared HP", hp, err)
	}
	if err = db.GetIns().First(&member, member.ID).Error; err != nil || member.Frozen || member.Damage != float64(event.Boss.HP)-100 {
		t.Fatal("rollback contribution", member, err)
	}
	if err = apply(false); err != nil {
		t.Fatal("commit last hit", err)
	}
	var after db.SharedFieldBoss55
	if err = query(db.GetIns(), event.Line).Take(&after).Error; err != nil {
		t.Fatal(err)
	}
	_, _, hp, err = ggoBossDecode(&after)
	if err != nil || hp[ggoBossGUID(event.ID)] != 0 || after.KillerID != f.id || !bytes.Equal(after.GGOEvent, before.GGOEvent) {
		t.Fatal("committed leader kill", hp, after.KillerID, err)
	}
	if err = db.GetIns().First(&member, member.ID).Error; err != nil || !member.Frozen || member.Rank != 1 || member.Damage != float64(event.Boss.HP) || len(member.GGOChips) == 0 {
		t.Fatal("frozen killer reward", member, err)
	}
	if err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		_, damage, err := f.s.ggoBossApplyDamageLocked(tx, f.id, fight, input, terminal, time.Now().UnixMilli())
		if err != nil || damage != 0 {
			return fmt.Errorf("finished event reapplied damage %v: %w", damage, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var unchanged db.SharedFieldBoss55
	if err = query(db.GetIns(), 2).Take(&unchanged).Error; err != nil || unchanged.UUID != lineTwo.UUID || !bytes.Equal(unchanged.HP, lineTwo.HP) || !bytes.Equal(unchanged.GGOEvent, lineTwo.GGOEvent) || unchanged.KillerID != lineTwo.KillerID {
		t.Fatal("other line changed", err)
	}
	// A replaced event still lets the old accepted battle finish without
	// damaging the replacement, and the simulated update is rolled back.
	if err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := query(tx, event.Line).Update("uuid", event.ID+"-replacement").Error; err != nil {
			return err
		}
		boss, damage, err := f.s.ggoBossApplyDamageLocked(tx, f.id, fight, input, terminal, time.Now().UnixMilli())
		if err != nil || damage != 0 || boss.GetState() != 3 {
			return fmt.Errorf("old battle after replacement %v: %w", damage, err)
		}
		return forced
	}); !errors.Is(err, forced) {
		t.Fatal("replacement-event compatibility", err)
	}
}
