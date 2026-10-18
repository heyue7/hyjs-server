package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func p13UFGuardAuditTemplate(s *GameService, id string, hp float64) *pb.BattleMonster {
	heroID := s.table("UnionFightGuard", "UFGuardId", id)["HeroId"]
	hero, err := s.actor(s.table("Hero", "id", heroID), "explicit-local-guard", 3, false)
	if err != nil {
		panic(err)
	}
	// 原守卫等级/血条/pose/属性转换未恢复；这些值属于提前公布的当地单实体模板。
	hero.Level = proto.Int32(1)
	hero.BloodNumber = proto.Int32(1)
	hero.Prop.Hp = proto.Float64(hp)
	hero.Inst.CurHp = proto.Float64(hp)
	return &pb.BattleMonster{MonsterId: proto.Int64(1), ServerId: proto.Int32(1), TaskId: proto.String(id), GroupIndex: proto.Int32(1), Formation: []*pb.BattleHero{hero}, Inst: &pb.BattleMonsterInst{Anger: proto.Float64(0)}}
}
func TestP13UnionFightGuardAuditActualTemplateFrozenPVE14ComputedRefusalRaceAndCold(t *testing.T) {
	f, ids, first, clock, match := p13UFFieldSetup(t)
	now := func() time.Time { return time.UnixMilli(clock.Load()) }
	id := ids[0]
	for _, table := range []any{&db.P13UnionFightGuardAuditSource{}, &db.P13UnionFightBattleAuditSource{}, &db.P13UnionFightGuardAuditOffer{}, &db.P13UnionFightGuardAuditAttempt{}, &db.P13UnionFightBattleAuditOffer{}, &db.P13UnionFightWarDrumSource{}, &db.P13UnionFightSpeedItemSource{}, &db.P13UnionFightHealAuraSource{}, &db.P13UnionFightItemSource{}, &db.P13UnionFightFactorySource{}, &db.P13UnionFightCarAttackSource{}, &db.P13UnionFightCarMotionSource{}, &db.P13UnionFightGateItemSource{}, &db.P13UnionFightCarItemSource{}} {
		if err := db.GetIns().Where("1=1").Delete(table).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.GetIns().Model(&db.P13UnionFightFieldPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "preparation_millis": 300000, "left_revive_point_id": 1, "right_revive_point_id": 3, "initial_guard_hp": 1000, "initial_gate_hp": 10000, "move_tolerance": 3, "source": "原地图/local显式单守卫1000HP"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP6(f.ctx); err != nil {
		t.Fatal(err)
	}
	zero, one := float64(0), float64(1)
	if err := db.GetIns().Create(&db.P13UnionFightBattleAuditSource{Day: match.Day, Enabled: true, SeedSalt: 42, AttackerDamageFactor: &zero, DefenderDamageFactor: &one, PublishedAt: now().UnixMilli(), Source: "显式本地Salt42/0与1倍率，非原参数"}).Error; err != nil {
		t.Fatal(err)
	}
	original, err := f.s.p13UFFieldOriginal(match, db.GetIns())
	if err != nil {
		t.Fatal(err)
	}
	for _, guardID := range []string{"factory_guard_001", "factory_guard_002"} {
		raw, _ := proto.Marshal(p13UFGuardAuditTemplate(original, guardID, 1000))
		if err = db.GetIns().Create(&db.P13UnionFightGuardAuditSource{Day: match.Day, GuardID: guardID, Enabled: true, Monster: raw, PublishedAt: now().UnixMilli(), Source: "提前显式locallevel1/blood1/pose3/表基础props但HP1000；非原守卫转换"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if _, err = f.s.p13UFFieldEnter(f.ctx, id, now()); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p13UFFieldFrames(f.ctx, id, &pb.CSCurrentPosition{}, now()); err != nil {
		t.Fatal(err)
	}
	var actor db.P13UnionFightFieldPlayer
	if err = db.GetIns().First(&actor, "match_id=? AND player_id=?", match.ID, id).Error; err != nil {
		t.Fatal(err)
	}
	guardID := "factory_guard_001"
	if actor.Camp == 1 {
		guardID = "factory_guard_002"
	}
	guid := p13UFFieldGUID(match.ID, "guard", guardID)
	if _, err = f.s.p13UFAskFightAnyAudit(f.ctx, id, &pb.CSUFPlayerAskFight{TargetGuid: proto.Int64(guid)}, 601, now()); err == nil {
		t.Fatal("preparation battle admitted")
	}
	clock.Store(first + 100000)
	point, _ := fieldPoint(original.table("UnionFightGuard", "UFGuardId", guardID)["Position"])
	if _, err = f.s.p13UFFieldMove(f.ctx, id, &pb.CSMove{PointNew: point}, now()); err != nil {
		t.Fatal(err)
	}
	var saved db.P13UnionFightField
	if err = db.GetIns().First(&saved, "match_id=?", match.ID).Error; err != nil {
		t.Fatal(err)
	}
	fieldBefore := append([]byte(nil), saved.Snapshot...)
	if err = db.GetIns().First(&actor, "match_id=? AND player_id=?", match.ID, id).Error; err != nil {
		t.Fatal(err)
	}
	frameBefore, battleBefore := actor.FrameHash, actor.BattleHash
	request := &pb.CSUFPlayerAskFight{TargetGuid: proto.Int64(guid)}
	var group sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := f.s.p13UFAskFightAnyAudit(f.ctx, id, request, 700, now())
			errs <- err
		}()
	}
	group.Wait()
	close(errs)
	for err := range errs {
		if err == nil || err.Error() != constUtil.P13UnionFightBattleAuditRefusal {
			t.Fatal("expected computed refusal", err)
		}
	}
	var attempt db.P13UnionFightGuardAuditAttempt
	if err = db.GetIns().First(&attempt, "match_id=? AND player_id=? AND stamp=700", match.ID, id).Error; err != nil {
		t.Fatal(err)
	}
	if err = p13UFGuardAuditVerifyRecord(db.GetIns(), &attempt); err != nil {
		t.Fatal(err)
	}
	var record db.P6BattleRecord
	if err = db.GetIns().First(&record, "id=?", attempt.P6RecordID).Error; err != nil {
		t.Fatal(err)
	}
	input, err := decodeP6Record(&record)
	if err != nil {
		t.Fatal(err)
	}
	if input.BattleType != 14 || input.DefenderFirst || input.IsAttToDef || input.Defender != nil || input.Monster.GetMonsterId() != guid || input.Monster.GetFormation()[0].GetInst().GetCurHp() != 1000 || input.Monster.GetFormation()[0].GetPose() != 3 || input.AttackerDamageFactor == nil || *input.AttackerDamageFactor != 0 {
		t.Fatal("original PVE shape/local explicit HP mapping", input)
	}
	var count int64
	db.GetIns().Model(&db.P13UnionFightGuardAuditAttempt{}).Where("match_id=? AND player_id=?", match.ID, id).Count(&count)
	if count != 1 {
		t.Fatal("same guard race duplicate", count)
	}
	if err = db.GetIns().First(&saved, "match_id=?", match.ID).Error; err != nil || !bytes.Equal(fieldBefore, saved.Snapshot) {
		t.Fatal("computed audit changed guard/building field", err)
	}
	if err = db.GetIns().First(&actor, "match_id=? AND player_id=?", match.ID, id).Error; err != nil || actor.FrameHash != frameBefore || actor.BattleHash != battleBefore {
		t.Fatal("computed audit changed player", err)
	}
	if err = db.GetIns().Model(&db.P13UnionFightGuardAuditSource{}).Where("day=?", match.Day).Updates(map[string]any{"enabled": false, "monster": []byte{255}}).Error; err != nil {
		t.Fatal(err)
	}
	sql, _ := db.GetIns().DB()
	sql.Close()
	f.open(t)
	if _, err = f.s.p13UFAskFightAnyAudit(f.ctx, id, request, 700, now()); err == nil || err.Error() != constUtil.P13UnionFightBattleAuditRefusal {
		t.Fatal("cold frozen receipt", err)
	}
	if _, err = f.s.p13UFAskFightAudit(f.ctx, id, &pb.CSUFPlayerAskFight{TargetGuid: proto.Int64(ids[1])}, 700, now()); err == nil || err.Error() == constUtil.P13UnionFightBattleAuditRefusal {
		t.Fatal("guard receipt reused by player branch", err)
	}
	clock.Store(first + 102000)
	outside := proto.Clone(point).(*pb.PointNew)
	outside.X = proto.Float32(point.GetX() + 5)
	if _, err = f.s.p13UFFieldMove(f.ctx, id, &pb.CSMove{PointNew: outside}, now()); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p13UFAskFightAnyAudit(f.ctx, id, request, 700, now()); err == nil || err.Error() != constUtil.P13UnionFightBattleAuditRefusal {
		t.Fatal("same guard receipt recomputed after range changed", err)
	}
	if _, err = f.s.p13UFAskFightAnyAudit(f.ctx, id, request, 703, now()); err == nil || err.Error() == constUtil.P13UnionFightBattleAuditRefusal {
		t.Fatal("new guard audit computed outside original .8", err)
	}
	quote := db.P13UnionFightGuardAuditOffer{}
	if err = db.GetIns().First(&quote, "match_id=? AND guard_guid=?", match.ID, guid).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Model(&quote).Update("source_hash", "corrupt").Error; err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p13UFAskFightAnyAudit(f.ctx, id, request, 701, now()); err == nil || err.Error() == constUtil.P13UnionFightBattleAuditRefusal {
		t.Fatal("guard corrupt source accepted", err)
	}
	if err = db.GetIns().Model(&quote).Update("source_hash", p13UFGuardOfferHash(&quote)).Error; err != nil {
		t.Fatal(err)
	}
	clock.Store(match.EndAt)
	if _, err = f.s.p13UFAskFightAnyAudit(f.ctx, id, request, 702, now()); err == nil {
		t.Fatal("ended guard audit accepted")
	}
}

// TestP13UnionFightGuardAuditNativeExport 保存隔离实际HTTP产生的未权威输入，供原机只读type14对照；不授予战争胜负。
func TestP13UnionFightGuardAuditNativeExport(t *testing.T) {
	raw := os.Getenv("HYJS_P13_GUARD_AUDIT_NATIVE_PLAYER")
	if raw == "" {
		t.Skip("显式隔离审计导出")
	}
	name := os.Getenv("HYJS_P13_MYSQL_DATABASE")
	if !strings.HasPrefix(name, "hyjs_test_p13_") {
		t.Fatal("native guild audit export schema guard")
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		t.Fatal("actual native owner required")
	}
	stamp, err := strconv.ParseInt(os.Getenv("HYJS_P13_GUARD_AUDIT_NATIVE_STAMP"), 10, 64)
	if err != nil || stamp <= 0 {
		t.Fatal("actual native request required")
	}
	dir, err := filepath.Abs(os.Getenv("HYJS_P13_GUARD_AUDIT_EXPORT_ROOT"))
	if err != nil || !strings.HasPrefix(filepath.Base(dir), "actual-battle14-") || filepath.Base(filepath.Dir(dir)) != "p13" || filepath.Base(filepath.Dir(filepath.Dir(dir))) != "analysis" {
		t.Fatal("bounded original14 input output path required")
	}
	f := &p1Fixture{ctx: context.Background(), mysql: db.MySQLConfig{Host: os.Getenv("HYJS_P13_MYSQL_HOST"), User: os.Getenv("HYJS_P13_MYSQL_USER"), Password: os.Getenv("HYJS_P13_MYSQL_PASSWORD"), Database: name}, s: &GameService{}}
	f.open(t)
	defer func() { sql, _ := db.GetIns().DB(); sql.Close() }()
	var attempt db.P13UnionFightGuardAuditAttempt
	if err = db.GetIns().First(&attempt, "player_id=? AND stamp=?", id, stamp).Error; err != nil {
		t.Fatal(err)
	}
	if attempt.SourceHash != p13UFGuardAttemptHash(&attempt) {
		t.Fatal("attempt source hash corrupt")
	}
	if err = p13UFGuardAuditVerifyRecord(db.GetIns(), &attempt); err != nil {
		t.Fatal(err)
	}
	var record db.P6BattleRecord
	if err = db.GetIns().First(&record, "id=?", attempt.P6RecordID).Error; err != nil {
		t.Fatal(err)
	}
	input, err := decodeP6Record(&record)
	if err != nil {
		t.Fatal(err)
	}
	var quote db.P13UnionFightGuardAuditOffer
	if err = db.GetIns().First(&quote, "match_id=? AND guard_guid=?", attempt.MatchID, attempt.GuardGUID).Error; err != nil || quote.SourceHash != p13UFGuardOfferHash(&quote) || quote.SourceHash != attempt.OfferHash {
		t.Fatal("actual frozen audit source missing", err)
	}
	attackerFactor, defenderFactor := input.AttackerDamageFactor, input.DefenderDamageFactor
	if attackerFactor == nil {
		attackerFactor = proto.Float64(1)
	}
	if defenderFactor == nil {
		defenderFactor = proto.Float64(1)
	}
	playback := &pb.SCFightPlayback{Fightid: proto.String(record.SourceFight), RandSeed: proto.Int64(input.Seed), BattleType: proto.Int32(input.BattleType), Attacker: input.Attacker, Defender2: input.Monster, AttackerFactor: attackerFactor, DefenderFactor: defenderFactor, MaxRound: proto.Int32(input.MaxRound)}
	// 原UF回放双方自动，没有client opts；未权威winner不会塞进用于原机计算的输入。
	wire, err := proto.MarshalOptions{Deterministic: true}.Marshal(playback)
	if err != nil {
		t.Fatal(err)
	}
	jsonInput, err := json.Marshal(input)
	if err != nil || p6Digest(jsonInput) != record.InputHash {
		t.Fatal("canonical frozen actual input hash mismatch", err)
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	key := "guard-actual-" + strconv.FormatInt(attempt.GUID, 10)
	files := map[string][]byte{key + ".SCFightPlayback.pb": wire, key + ".frozen-input.json": jsonInput, key + ".computed-outcome.json": record.Outcome}
	var common db.P13UnionFightBattleAuditOffer
	if err = db.GetIns().First(&common, "match_id=?", attempt.MatchID).Error; err != nil || !p13UFBattleAuditQuoteValid(&common) || quote.BattleOfferHash != common.SourceHash {
		t.Fatal("guard common seed/factor frozen source mismatch", err)
	}
	meta := map[string]any{"commonQuote": common, "sourceKind": "actual isolated local development realm; no original private server data", "originalCallback": "SCUFPveFightRecordReplay", "diagnosticShape": "SCFightPlayback with same type14 full attacker/monster/seed/factors; auto opts nil; no winner input", "status": constUtil.P13UnionFightBattleAuditComputed, "authoritative": false, "nativeComparedType14": false, "warApplied": false, "finiteInjury": "explicit bounded10pct preparation; no natural war damage", "guardTemplate": "prepublished local level1/blood1/pose3/TableHero base props with field localHP1000; originalguard level/props/HP transform unknown; fieldIndex0 mapped to templatePose3", "attempt": attempt, "quote": quote, "seed": input.Seed, "maxRound": input.MaxRound, "battleType": input.BattleType, "isAttToDef": input.IsAttToDef, "defenderFirst": input.DefenderFirst, "configurationHash": input.ConfigurationHash, "engineVersion": input.EngineVersion, "clientVersion": input.ClientVersion, "capabilityHashAtExport": P6CapabilityHash(), "p6InputHash": record.InputHash, "p6OutcomeHash": record.OutcomeHash, "p6ReplayHash": record.ReplayHash, "files": map[string]string{}}
	prebuilt := os.Getenv("HYJS_P13_FIELD_PREBUILT_NATIVE")
	if !strings.HasPrefix(filepath.Base(prebuilt), "hyjs-p13-prebuilt-") {
		t.Fatal("fixed native source proof required")
	}
	proof, err := os.ReadFile(filepath.Join(prebuilt, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var proofInfo map[string]any
	if json.Unmarshal(proof, &proofInfo) != nil {
		t.Fatal("fixed native source manifest invalid")
	}
	meta["fixedSourceProofSHA"] = p6Digest(proof)
	meta["fixedSourceProof"] = proofInfo
	for file, data := range files {
		if err = os.WriteFile(filepath.Join(dir, file), data, 0600); err != nil {
			t.Fatal(err)
		}
		meta["files"].(map[string]string)[file] = p6Digest(data)
	}
	encoded, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, key+".metadata.json"), append(encoded, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	fmt.Println("GUILD_GUARD_AUDIT_NATIVE_INPUT_EXPORTED " + filepath.Join(dir, key+".metadata.json"))
}

// 原HTTP场景建立前提前发布专库模板，避免把战斗请求临时参数写成原守卫属性。
func TestP13UnionFightGuardAuditNativePublish(t *testing.T) {
	raw := os.Getenv("HYJS_P13_GUARD_AUDIT_NATIVE_PUBLISH_AT")
	if raw == "" {
		t.Skip("显式专库提前发布")
	}
	database := os.Getenv("HYJS_P13_MYSQL_DATABASE")
	if !strings.HasPrefix(database, "hyjs_test_p13_") {
		t.Fatal("native guard publish schema guard")
	}
	at, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || at <= 0 {
		t.Fatal("explicit published time required")
	}
	f := &p1Fixture{ctx: context.Background(), mysql: db.MySQLConfig{Host: os.Getenv("HYJS_P13_MYSQL_HOST"), User: os.Getenv("HYJS_P13_MYSQL_USER"), Password: os.Getenv("HYJS_P13_MYSQL_PASSWORD"), Database: database}, s: &GameService{}}
	f.open(t)
	defer func() { sql, _ := db.GetIns().DB(); sql.Close() }()
	if err = f.s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err = f.s.LoadP13GuildTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, guardID := range []string{"factory_guard_001", "factory_guard_002"} {
		monster := p13UFGuardAuditTemplate(f.s, guardID, 1000)
		wire, err := proto.MarshalOptions{Deterministic: true}.Marshal(monster)
		if err != nil {
			t.Fatal(err)
		}
		if err = db.GetIns().Create(&db.P13UnionFightGuardAuditSource{Day: "2026-10-20", GuardID: guardID, Enabled: true, Monster: wire, PublishedAt: at, Source: "明确提前locallevel1/blood1/pose3/TableHero base props except HP1000；原守卫等级/属性变换未知"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	fmt.Println("GUILD_GUARD_AUDIT_LOCAL_TEMPLATE_PUBLISHED")
}
