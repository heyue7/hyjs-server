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
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestP13UnionFightBattleAuditActualMineShopFrozen14ComputedCommittedRefusalColdRaceAndNoWarResult(t *testing.T) {
	f, ids, first, clock, match := p13UFFieldSetup(t)
	now := func() time.Time { return time.UnixMilli(clock.Load()) }
	id := ids[0]
	for _, model := range []any{&db.P13UnionFightBattleAuditSource{}, &db.P13UnionFightBattleAuditOffer{}, &db.P13UnionFightBattleAuditAttempt{}, &db.P13UnionFightWarDrumSource{}, &db.P13UnionFightWarDrumOffer{}, &db.P13UnionFightWarDrumEffect{}, &db.P13UnionFightSpeedItemSource{}, &db.P13UnionFightHealAuraSource{}, &db.P13UnionFightItemSource{}, &db.P13UnionFightItemOffer{}, &db.P13UnionFightFactorySource{}, &db.P13UnionFightCarAttackSource{}, &db.P13UnionFightCarMotionSource{}, &db.P13UnionFightGateItemSource{}, &db.P13UnionFightCarItemSource{}} {
		if err := db.GetIns().Where("1=1").Delete(model).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.GetIns().Model(&db.P13UnionFightFieldPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "preparation_millis": 300000, "left_revive_point_id": 1, "right_revive_point_id": 3, "initial_guard_hp": 1000, "initial_gate_hp": 10000, "move_tolerance": 3, "source": "隔离原冲锋地图/本地HP"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Create(&db.P13UnionFightWarDrumSource{Day: match.Day, ItemID: "ufitem_014", Enabled: true, AttackPercent: 20, Radius: 3, DurationMillis: 10000, AuthorityMask: 4, ModelName: constUtil.P13UnionFightWarDrumLocalModel, ModelScale: 1, PublishedAt: now().UnixMilli(), Source: "原model/totem1数字/UseCondition2未知，隔离local已核shield-prefab/Atk20%/3m/10s/Chair2资格"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Create(&db.P13UnionFightItemSource{Day: match.Day, ItemID: "ufitem_011", Enabled: true, HPPercent: 20, PublishedAt: now().UnixMilli(), Source: "原比例未知/local20共享CD"}).Error; err != nil {
		t.Fatal(err)
	}
	for _, item := range []string{"ufitem_014", "ufitem_011"} {
		if err := db.GetIns().Create(&db.P13UnionFightShopSource{Day: match.Day, ShopItemID: item, ItemID: item, Enabled: true, Price: 10, Quantity: 1, BuyCount: 3, RefreshMillis: 60000, PublishedAt: now().UnixMilli(), Source: "原报价未知/local矿币10买1"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	original, err := f.s.p13UFFieldOriginal(match, db.GetIns())
	if err != nil {
		t.Fatal(err)
	}
	for camp, npc := range map[int]string{0: "2", 1: "5"} {
		point, _ := fieldPoint(original.table("UnionFightNpc", "UFNId", npc)["Position"])
		if err = db.GetIns().Create(&db.P13UnionFightResourceSource{Day: match.Day, SourceKey: fmt.Sprintf("speed-mine-%d", camp), Enabled: true, ResourceID: "1", X: point.GetX(), Y: point.GetY(), Z: point.GetZ(), RewardRes: 50, PublishedAt: now().UnixMilli(), Source: "真实矿1/原回收点/local50"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	var roster p13UFMatchRoster
	if err = json.Unmarshal(match.Roster, &roster); err != nil {
		t.Fatal(err)
	}
	guild := p13GuildID(t, f, id, now())
	enemy := int64(0)
	for _, side := range [][]p13UFMatchFighter{roster.Left, roster.Right} {
		for _, fighter := range side {
			if fighter.GuildID != guild {
				enemy = fighter.PlayerID
				break
			}
		}
	}
	if enemy <= 0 {
		t.Fatal("actual opposing roster missing")
	}
	if err := f.s.InitializeP6(f.ctx); err != nil {
		t.Fatal(err)
	}
	factorZero, factorOne := float64(0), float64(1)
	if err := db.GetIns().Create(&db.P13UnionFightBattleAuditSource{Day: match.Day, Enabled: true, SeedSalt: 42, AttackerDamageFactor: &factorZero, DefenderDamageFactor: &factorOne, PublishedAt: now().UnixMilli(), Source: "原种子/倍率映射未知/localSalt42 explicitatt0-def1，真实计算仅审计"}).Error; err != nil {
		t.Fatal(err)
	}
	for _, player := range []int64{id, ids[11], ids[10], enemy} {
		if _, err = f.s.p13UFFieldEnter(f.ctx, player, now()); err != nil {
			t.Fatal(err)
		}
		if _, err = f.s.p13UFFieldFrames(f.ctx, player, &pb.CSCurrentPosition{}, now()); err != nil {
			t.Fatal(err)
		}
	}
	read := func(player int64) (db.P13UnionFightFieldPlayer, *pb.InstUFPlayer) {
		t.Helper()
		var row db.P13UnionFightFieldPlayer
		if err := db.GetIns().First(&row, "match_id=? AND player_id=?", match.ID, player).Error; err != nil {
			t.Fatal(err)
		}
		frame, err := p13UFFieldReadActor(&row)
		if err != nil {
			t.Fatal(err)
		}
		return row, frame
	}
	actor, _ := read(id)
	direction := float32(1)
	if actor.Camp == 1 {
		direction = -1
	}
	recycler, shop := "2", "3"
	if actor.Camp == 1 {
		recycler, shop = "5", "6"
	}
	point, _ := fieldPoint(original.table("UnionFightNpc", "UFNId", recycler)["Position"])
	clock.Store(first + 20000)
	if _, err = f.s.p13UFFieldMove(f.ctx, id, &pb.CSMove{PointNew: point}, now()); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p13UFResourceBegin(f.ctx, id, &pb.CSUFPlayerCollect{ResGuid: proto.Int64(p13UFFieldGUID(match.ID, "resource", fmt.Sprintf("speed-mine-%d", actor.Camp)))}, now()); err != nil {
		t.Fatal(err)
	}
	if _, _, err = f.s.ProjectP13UnionFightWarDrumBattle(f.ctx, id, now()); err == nil {
		t.Fatal("actual original resource mining state2 granted fighter benefit")
	}
	clock.Add(3000)
	if err = f.s.TickP13UnionFightResource(f.ctx, now()); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p13UFResourceDeposit(f.ctx, id, &pb.CSUFPlayerUpResource{NpcId: proto.String(recycler)}, 500, now()); err != nil {
		t.Fatal(err)
	}
	clock.Add(10000)
	point, _ = fieldPoint(original.table("UnionFightNpc", "UFNId", shop)["Position"])
	for i, player := range []int64{id, ids[11], ids[10], enemy} {
		target := proto.Clone(point).(*pb.PointNew)
		if i == 1 {
			target.X = proto.Float32(target.GetX() + direction*2)
		}
		if i == 2 {
			target.X = proto.Float32(target.GetX() + direction*10)
		}
		if _, err = f.s.p13UFFieldMove(f.ctx, player, &pb.CSMove{PointNew: target}, now()); err != nil {
			t.Fatal(err)
		}
	}
	shopQuotes := p13UFTestShopQuotes(t, f, id, shop, now())
	for i, item := range []string{"ufitem_014", "ufitem_014", "ufitem_011"} {
		if _, err = f.s.p13UFShopBuy(f.ctx, id, &pb.CSUFPlayerBuyItem{ShopItemId: proto.String(p13UFTestShopQuote(t, shopQuotes, item)), NpcId: proto.String(shop)}, 600+int64(i), now()); err != nil {
			t.Fatal(err)
		}
	}
	_, inventory := read(id)
	if inventory.GetOwnToken() != 120 {
		t.Fatal("actual mining150 bought30", inventory.GetOwnToken())
	}
	db.GetIns().Model(&db.P13UnionFightItemPolicy{}).Where("id=1").Update("enabled", true)
	request := &pb.CSUFPlayerUseItem{ItemId: proto.String("ufitem_014")}
	baseline := func(player int64) *pb.BattlePlayer {
		t.Helper()
		row, _ := read(player)
		battle := &pb.BattlePlayer{}
		if err := proto.Unmarshal(row.Battle, battle); err != nil {
			t.Fatal(err)
		}
		return battle
	}
	check := func(player int64, multiplier float64) (*pb.BattlePlayer, map[string][]string) {
		t.Helper()
		before := baseline(player)
		projected, applied, err := f.s.ProjectP13UnionFightWarDrumBattle(f.ctx, player, now())
		if err != nil {
			t.Fatal(err)
		}
		for i, hero := range projected.Formation {
			if math.Abs(hero.Prop.GetAtt()-before.Formation[i].Prop.GetAtt()*multiplier) > 1e-8 || !proto.Equal(hero.Inst, before.Formation[i].Inst) {
				t.Fatal("explicit Att/currentHP mapping boundary", player, hero.Prop.GetAtt())
			}
			if multiplier == 1 && len(applied[hero.GetGuid()]) != 0 {
				t.Fatal("no eligible benefit got audit")
			}
			if multiplier > 1 && len(applied[hero.GetGuid()]) == 0 {
				t.Fatal("real benefit missing source provenance")
			}
		}
		return projected, applied
	}
	for _, player := range []int64{id, ids[11], ids[10], enemy} {
		check(player, 1)
	}
	used, err := f.s.p13UFItemUse(f.ctx, id, request, 800, now())
	if err != nil {
		t.Fatal(err)
	}
	created := used[1].(*pb.SCUFTotemCreate)
	if created.Totem.GetTotemId() != "1" || created.Totem.GetModelName() != constUtil.P13UnionFightWarDrumLocalModel || created.Totem.GetModelScale() != 1 || created.Totem.GetLastTime() != 10 || created.Totem.GetSourcePlayerGuid() != id {
		t.Fatal("actual local totem instance", created)
	}

	t.Cleanup(func() { db.GetIns().Where("1=1").Delete(&db.P13UnionFightBattleAuditSource{}) })
	before, _ := read(id)
	enemyBefore, _ := read(enemy)
	var beforeMatch db.P13UnionFightMatch
	if err = db.GetIns().First(&beforeMatch, "id=?", match.ID).Error; err != nil {
		t.Fatal(err)
	}
	ask := &pb.CSUFPlayerAskFight{TargetGuid: proto.Int64(enemy)}
	refused := func(stamp int64) {
		t.Helper()
		_, err := f.s.p13UFAskFightAudit(f.ctx, id, ask, stamp, now())
		if err == nil || err.Error() != constUtil.P13UnionFightBattleAuditRefusal {
			t.Fatal("computed audit must retain authority gate", err)
		}
	}
	refused(900)
	if f.mysql.Database != "" {
		var wg sync.WaitGroup
		errs := make(chan error, 8)
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); _, err := f.s.p13UFAskFightAudit(f.ctx, id, ask, 900, now()); errs <- err }()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err == nil || err.Error() != constUtil.P13UnionFightBattleAuditRefusal {
				t.Fatal("same request gate", err)
			}
		}
	}
	var attempts []db.P13UnionFightBattleAuditAttempt
	if err = db.GetIns().Where("match_id=?", match.ID).Find(&attempts).Error; err != nil || len(attempts) != 1 {
		t.Fatal("non-authoritative receipt committed exactly once", len(attempts), err)
	}
	attempt := attempts[0]
	var record db.P6BattleRecord
	if err = db.GetIns().First(&record, "id=?", attempt.P6RecordID).Error; err != nil {
		t.Fatal(err)
	}
	input, err := decodeP6Record(&record)
	if err != nil || record.Status != constUtil.P6BattleComputed || record.Authoritative || input.BattleType != 14 || input.IsAttToDef || input.AttackerDamageFactor == nil || *input.AttackerDamageFactor != 0 || input.DefenderDamageFactor == nil || *input.DefenderDamageFactor != 1 {
		t.Fatal("actual original14/factorzero computed record", record.Status, input, err)
	}
	if len(record.Outcome) == 0 || len(record.OutcomeHash) != 64 || len(record.ReplayHash) != 64 || len(input.AppliedProperties) == 0 {
		t.Fatal("actual calculation/provenance absent")
	}
	if err = p13UFBattleAuditVerifyRecord(db.GetIns(), &attempt); err != nil {
		t.Fatal(err)
	}
	after, _ := read(id)
	enemyAfter, _ := read(enemy)
	var afterMatch db.P13UnionFightMatch
	if err = db.GetIns().First(&afterMatch, "id=?", match.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before.Frame, after.Frame) || !bytes.Equal(enemyBefore.Frame, enemyAfter.Frame) || !bytes.Equal(before.Battle, after.Battle) || !reflect.DeepEqual(beforeMatch, afterMatch) {
		t.Fatal("unverified computed audit mutated war HP/inventory/score/match")
	}
	if _, err = f.s.p13UFAskFightAudit(f.ctx, id, &pb.CSUFPlayerAskFight{TargetGuid: proto.Int64(ids[11])}, 900, now()); err == nil || err.Error() == constUtil.P13UnionFightBattleAuditRefusal {
		t.Fatal("same stamp changed target accepted", err)
	}
	db.GetIns().Model(&db.P13UnionFightBattleAuditSource{}).Where("day=?", match.Day).Updates(map[string]any{"enabled": false, "seed_salt": 99, "attacker_damage_factor": 99})
	clock.Store(first + 35000)
	enemyRow, _ := read(enemy)
	if _, err = f.s.p13UFFieldMove(f.ctx, enemy, &pb.CSMove{PointNew: &pb.PointNew{X: proto.Float32(enemyRow.X + 10), Y: proto.Float32(enemyRow.Y), Z: proto.Float32(enemyRow.Z)}}, now()); err != nil {
		t.Fatal(err)
	}
	refused(900)
	if _, err = f.s.p13UFAskFightAudit(f.ctx, id, ask, 901, now()); err == nil || err.Error() == constUtil.P13UnionFightBattleAuditRefusal {
		t.Fatal("fresh remote target was computed", err)
	}
	sql, _ := db.GetIns().DB()
	sql.Close()
	f.open(t)
	if err = f.s.InitializeP13Guild(f.ctx); err != nil {
		t.Fatal(err)
	}
	refused(900)
	if err = db.GetIns().First(&record, "id=?", attempt.P6RecordID).Error; err != nil {
		t.Fatal(err)
	}
	if record.OutcomeHash != attempt.OutcomeHash {
		t.Fatal("cold retry recomputed latest source")
	}
	db.GetIns().Model(&db.P6BattleRecord{}).Where("id=?", record.ID).Update("outcome_hash", "bad")
	if _, err = f.s.p13UFAskFightAudit(f.ctx, id, ask, 900, now()); err == nil || err.Error() == constUtil.P13UnionFightBattleAuditRefusal {
		t.Fatal("corrupt computed record accepted", err)
	}
	clock.Store(match.EndAt)
	if _, err = f.s.p13UFAskFightAudit(f.ctx, id, ask, 900, now()); err == nil {
		t.Fatal("End old audit attempt accepted")
	}
}

// TestP13UnionFightBattleAuditNativeExport 保存隔离实际HTTP产生的未权威输入，供原机只读type14对照；不授予战争胜负。
func TestP13UnionFightBattleAuditNativeExport(t *testing.T) {
	raw := os.Getenv("HYJS_P13_BATTLE_AUDIT_NATIVE_PLAYER")
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
	stamp, err := strconv.ParseInt(os.Getenv("HYJS_P13_BATTLE_AUDIT_NATIVE_STAMP"), 10, 64)
	if err != nil || stamp <= 0 {
		t.Fatal("actual native request required")
	}
	dir, err := filepath.Abs(os.Getenv("HYJS_P13_BATTLE_AUDIT_EXPORT_ROOT"))
	if err != nil || !strings.HasPrefix(filepath.Base(dir), "actual-battle14-") || filepath.Base(filepath.Dir(dir)) != "p13" || filepath.Base(filepath.Dir(filepath.Dir(dir))) != "analysis" {
		t.Fatal("bounded original14 input output path required")
	}
	f := &p1Fixture{ctx: context.Background(), mysql: db.MySQLConfig{Host: os.Getenv("HYJS_P13_MYSQL_HOST"), User: os.Getenv("HYJS_P13_MYSQL_USER"), Password: os.Getenv("HYJS_P13_MYSQL_PASSWORD"), Database: name}, s: &GameService{}}
	f.open(t)
	defer func() { sql, _ := db.GetIns().DB(); sql.Close() }()
	var attempt db.P13UnionFightBattleAuditAttempt
	if err = db.GetIns().First(&attempt, "player_id=? AND stamp=?", id, stamp).Error; err != nil {
		t.Fatal(err)
	}
	if attempt.SourceHash != p13UFBattleAuditAttemptHash(&attempt) {
		t.Fatal("attempt source hash corrupt")
	}
	if err = p13UFBattleAuditVerifyRecord(db.GetIns(), &attempt); err != nil {
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
	var quote db.P13UnionFightBattleAuditOffer
	if err = db.GetIns().First(&quote, "match_id=?", attempt.MatchID).Error; err != nil || !p13UFBattleAuditQuoteValid(&quote) || quote.SourceHash != attempt.OfferHash {
		t.Fatal("actual frozen audit source missing", err)
	}
	attackerFactor, defenderFactor := input.AttackerDamageFactor, input.DefenderDamageFactor
	if attackerFactor == nil {
		attackerFactor = proto.Float64(1)
	}
	if defenderFactor == nil {
		defenderFactor = proto.Float64(1)
	}
	playback := &pb.SCFightPlayback{Fightid: proto.String(record.SourceFight), RandSeed: proto.Int64(input.Seed), BattleType: proto.Int32(input.BattleType), Attacker: input.Attacker, Defender1: input.Defender, AttackerFactor: attackerFactor, DefenderFactor: defenderFactor, MaxRound: proto.Int32(input.MaxRound)}
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
	key := "actual-" + strconv.FormatInt(attempt.GUID, 10)
	files := map[string][]byte{key + ".SCFightPlayback.pb": wire, key + ".frozen-input.json": jsonInput, key + ".computed-outcome.json": record.Outcome}
	meta := map[string]any{"sourceKind": "actual isolated local development realm; no original private server data", "originalCallback": "SCUFFightRecordReplay", "diagnosticShape": "SCFightPlayback with same type14 full attacker/defender/seed/factors; auto opts nil; no winner input", "status": constUtil.P13UnionFightBattleAuditComputed, "authoritative": false, "nativeComparedType14": false, "warApplied": false, "finiteInjury": "explicit bounded10pct native test preparation; not natural P6 war damage", "attempt": attempt, "quote": quote, "seed": input.Seed, "maxRound": input.MaxRound, "battleType": input.BattleType, "isAttToDef": input.IsAttToDef, "defenderFirst": input.DefenderFirst, "configurationHash": input.ConfigurationHash, "engineVersion": input.EngineVersion, "clientVersion": input.ClientVersion, "capabilityHashAtExport": P6CapabilityHash(), "p6InputHash": record.InputHash, "p6OutcomeHash": record.OutcomeHash, "p6ReplayHash": record.ReplayHash, "files": map[string]string{}}
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
	fmt.Println("GUILD_AUDIT_NATIVE_INPUT_EXPORTED " + filepath.Join(dir, key+".metadata.json"))
}
