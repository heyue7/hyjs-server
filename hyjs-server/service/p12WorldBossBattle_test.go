package service

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p12WorldBossBattlePrepare(t *testing.T, f *p1Fixture, id int64) []byte {
	t.Helper()
	// 隔离用例明确设置90级原主角和真实出生坐标；不修改首领表或模拟结果。
	if err := updateProfile(f.ctx, id, func(player *pb.Player, game *model.GameState) error {
		if player.GetLevel() < 90 {
			player.Level = proto.Int32(90)
		}
		point, err := fieldPoint("66.3,4.598071,22.42")
		if err != nil {
			return err
		}
		game.Position = &pb.Position{Scene: proto.String("field_002"), Current: point}
		game.PositionLoading = false
		game.Run = nil
		game.Field = &model.FieldState{Scene: "field_002", Side: &pb.PlayerSide{PlayerId: proto.Int64(id), State: proto.Int32(0), CurHp: proto.Float64(1), MaxHp: proto.Float64(1), Anger: proto.Float64(0)}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var profile db.Profile
	if err := db.GetIns().First(&profile, id).Error; err != nil {
		t.Fatal(err)
	}
	return append([]byte(nil), profile.Session...)
}

func p12WorldBossBattleFullBuild(t *testing.T, f *p1Fixture, id int64) {
	t.Helper()
	// 必要历史阵容沿已证001003/037/Pet002源恢复，不导入普通库存、钱包或战斗结果。
	for _, load := range []func(string) error{f.s.LoadP5Tables, f.s.LoadP9Tables} {
		if err := load("../../analysis/tables"); err != nil {
			t.Fatal(err)
		}
	}
	f.s.P5Ready, f.s.P9PetsReady = true, true
	if err := f.s.InitializeP9Pets(f.ctx); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("../../analysis/p7/party-rush-isolated-roster-r2-20261016.json")
	if err != nil || p6Digest(raw) != "e60e9774a43efe58f67b9eb84c1a30cf40a3a30f2e5f68c87d063d3aeb29a5c9" {
		t.Fatal("same proven actual roster", err)
	}
	var source struct {
		Player struct {
			Level int32  `json:"level"`
			Main  string `json:"heroWarId"`
		} `json:"playerPatch"`
		Game struct {
			Warriors  []*pb.Warrior       `json:"warriors"`
			Formation []*pb.FormationItem `json:"formation"`
			Equipment []*pb.Equipment     `json:"equipment"`
		} `json:"gamePatch"`
		Pet struct {
			GUID string `json:"guid"`
			ID   string `json:"id"`
			Wire string `json:"wireHex"`
		} `json:"pet"`
	}
	if json.Unmarshal(raw, &source) != nil || len(source.Game.Warriors) != 2 || len(source.Game.Equipment) != 1 || source.Pet.ID != "pet_002" {
		t.Fatal("actual roster shape")
	}
	petRaw, err := hex.DecodeString(source.Pet.Wire)
	pet := &pb.Pet{}
	if err != nil || proto.Unmarshal(petRaw, pet) != nil || pet.GetGuid() != source.Pet.GUID {
		t.Fatal("actual historical pet", err)
	}
	// 专属库两玩家各拥有自己的必要Pet实例；仅重标Pet所有权GUID，面板/成长保持原字节字段。
	pet.Guid = proto.String(source.Pet.GUID + "-p12-" + fmt.Sprint(id))
	petRaw, err = proto.Marshal(pet)
	if err != nil {
		t.Fatal(err)
	}
	if err = updateProfile(f.ctx, id, func(player *pb.Player, game *model.GameState) error {
		player.Level, player.HeroWarId = proto.Int32(source.Player.Level), proto.String(source.Player.Main)
		game.Warriors, game.Formation, game.Equipment, game.Treasures = source.Game.Warriors, source.Game.Formation, source.Game.Equipment, nil
		for _, formation := range game.Formation {
			for _, thing := range formation.Things {
				if thing.GetGuid() == source.Pet.GUID {
					thing.Guid = pet.Guid
				}
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Create(&db.P9PetInstance{GUID: pet.GetGuid(), PlayerID: id, PetID: pet.GetId(), Data: petRaw, CreatedAt: time.Now().UnixMilli(), Source: "finite historical equipped roster prerequisite; e60e9774; no natural acquisition claim"}).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.GetIns().Where("guid=? AND player_id=?", pet.GetGuid(), id).Delete(&db.P9PetInstance{}) })
	if _, err = f.s.p9GuardOn(f.ctx, id, &pb.CSFormationOn{Id: pet.Id, Guid: pet.Guid, FormatPos: proto.Int32(1)}, 20790); err != nil {
		t.Fatal("publish actual guard snapshot", err)
	}
}

func p12WorldBossBattleSetup(t *testing.T, unboosted ...bool) (*p1Fixture, int64, string, time.Time) {
	t.Helper()
	f, other := p12WorldBossSetup(t)
	// P6 original-test cache包含BuffInfExtend等实际引擎表；保留额外首领域表。
	for name, rows := range p6OriginalTables(t).Tables {
		if len(rows) > 0 {
			f.s.Tables[name] = rows
		}
	}
	if err := f.s.InitializeP12WorldBossBattle(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().AutoMigrate(&db.DailyCopyFight{}); err != nil {
		t.Fatal(err)
	}
	f.s.P12WorldBossBattleReady = true
	if err := db.GetIns().Model(&db.P12WorldBossPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P12WorldBossBattlePolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	now := p12WorldBossDay().Add(12*time.Hour + 10*time.Minute)
	uuid := p12WorldBossCall(t, f, f.id, &pb.CSWorldBossInfo{}, now).(*pb.SCWorldBossInfo).GetWorldBossShowInfo().GetBossuuid()
	p12WorldBossBattlePrepare(t, f, f.id)
	p12WorldBossBattlePrepare(t, f, other)
	p12WorldBossBattleFullBuild(t, f, f.id)
	p12WorldBossBattleFullBuild(t, f, other)
	if len(unboosted) == 0 || !unboosted[0] {
		if err := db.GetIns().Model(&db.P12WorldBossBattlePolicy{}).Where("id=1").Update("economy_enabled", true).Error; err != nil {
			t.Fatal(err)
		}
		for _, id := range []int64{f.id, other} {
			// 正伤害场景给明示的150钻购买前提，效果与扣费均走真实购买服务。
			if err := updateProfile(f.ctx, id, func(player *pb.Player, _ *model.GameState) error { player.Diamond = proto.Int64(150); return nil }); err != nil {
				t.Fatal(err)
			}
			if _, err := f.s.PurchaseP12WorldBossProp(f.ctx, id, "wordboss_1", now); err != nil {
				t.Fatal(err)
			}
		}
	}
	return f, other, uuid, now
}

func TestP12WorldBossBattlePrecomputeFinishRestartAndOwnership(t *testing.T) {
	f, other, boss, now := p12WorldBossBattleSetup(t)
	session := p12WorldBossBattlePrepare(t, f, f.id)
	start, err := f.s.startP12WorldBossBattleAt(f.ctx, f.id, boss, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	var pending db.P12WorldBossBattleReceipt
	if err = db.GetIns().First(&pending, "fight_id=?", start.GetFightId()).Error; err != nil {
		t.Fatal(err)
	}
	if pending.Committed || pending.Stamp != nil || pending.PredictedDamage <= 0 {
		t.Fatal("real precomputation must be pending with server damage", pending.PredictedDamage)
	}
	var frozen model.FrozenBattleInput
	var terminal model.CombatFinalHPOutcome
	if err = json.Unmarshal([]byte(pending.InputJSON), &frozen); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal([]byte(pending.TerminalJSON), &terminal); err != nil {
		t.Fatal(err)
	}
	if frozen.BattleType != 18 || frozen.MaxRound != 40 || len(frozen.Monster.Formation) != 4 || frozen.AppliedProperties["localWorldBossPolicy"][0] != constUtil.P12WorldBossBattlePolicy {
		t.Fatal("original type18/four-member input not frozen")
	}
	actual, err := f.s.ComputeFrozenBattleFinalHP(f.ctx, &frozen, nil)
	if err != nil {
		t.Fatal(err)
	}
	damage, err := p12WorldBossBattleDamage(&frozen, actual)
	if err != nil || damage != pending.PredictedDamage {
		t.Fatal("persisted damage differs from real frozen calculation", damage, pending.PredictedDamage, err)
	}
	var instance db.P12WorldBossBattleInstance
	if err = db.GetIns().First(&instance, "id=?", boss).Error; err != nil {
		t.Fatal(err)
	}
	if instance.CurrentHP != instance.TotalHP {
		t.Fatal("start deducted shared HP")
	}
	if start.GetDamageforwardPercent() != 1 || start.GetDamagelaterPercent() != float64(instance.TotalHP-pending.PredictedDamage)/float64(instance.TotalHP) {
		t.Fatal("original boss HP ratio projected as personal cumulative damage")
	}
	rank, err := p12WorldBossBattleRank(db.GetIns(), boss, f.id)
	if err != nil || len(rank.DamageInfos) != 0 {
		t.Fatal("pending entered damage rank", rank, err)
	}
	var participation db.P12WorldBossBattleParticipation
	db.GetIns().First(&participation, "player_id=?", f.id)
	if participation.Remaining != 4 {
		t.Fatal("start free use", participation)
	}
	again, err := f.s.startP12WorldBossBattleAt(f.ctx, f.id, boss, 0, now.Add(time.Minute))
	if err != nil || !proto.Equal(start, again) {
		t.Fatal("zero-stamp pending retry changed response", err)
	}
	otherSession := p12WorldBossBattlePrepare(t, f, other)
	if _, err = f.s.finishP12WorldBossBattleAt(f.ctx, other, otherSession, start.GetFightId(), now.Add(time.Minute)); err == nil {
		t.Fatal("other actor committed receipt")
	}
	if _, err = f.s.finishP12WorldBossBattleAt(f.ctx, f.id, []byte("invalid-session!"), start.GetFightId(), now.Add(time.Minute)); err == nil {
		t.Fatal("invalid session committed receipt")
	}
	// 关闭并重新打开同一隔离数据库，pending仍可重放和完成。
	connection, _ := db.GetIns().DB()
	if err = connection.Close(); err != nil {
		t.Fatal(err)
	}
	f.open(t)
	if err = f.s.InitializeP12WorldBossBattle(f.ctx); err != nil {
		t.Fatal(err)
	}
	restored, err := f.s.startP12WorldBossBattleAt(f.ctx, f.id, boss, 0, now.Add(2*time.Minute))
	if err != nil || !proto.Equal(start, restored) {
		t.Fatal("restart lost pending", err)
	}
	resumed, scene, err := f.s.resumeP12WorldBossBattleAt(f.ctx, f.id, session, now.Add(2*time.Minute))
	if err != nil || scene != "field_002" || !proto.Equal(start, resumed) {
		t.Fatal("authenticated resume lost frozen scene", scene, err)
	}
	if err = updateProfile(f.ctx, f.id, func(_ *pb.Player, game *model.GameState) error {
		game.Position.Scene = proto.String("field_003")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err = f.s.resumeP12WorldBossBattleAt(f.ctx, f.id, session, now.Add(2*time.Minute)); err == nil {
		t.Fatal("resume accepted scene mismatch")
	}
	if err = updateProfile(f.ctx, f.id, func(_ *pb.Player, game *model.GameState) error {
		game.Position.Scene = proto.String("field_002")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	finished, err := f.s.finishP12WorldBossBattleAt(f.ctx, f.id, session, start.GetFightId(), now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if finished.AlreadyCommitted || finished.AppliedDamage != pending.PredictedDamage || finished.CurrentHP != instance.TotalHP-finished.AppliedDamage {
		t.Fatal("finish did not apply frozen damage", finished)
	}
	bytes, err := base64.StdEncoding.DecodeString(finished.StartBase64)
	if err != nil {
		t.Fatal(err)
	}
	final := &pb.SCWorldBossFight{}
	if err = proto.Unmarshal(bytes, final); err != nil {
		t.Fatal(err)
	}
	if final.GetDamange() != int32(finished.AppliedDamage) || final.GetPlayerSide() == nil || final.GetPlayerSide().GetState() == 6 {
		t.Fatal("finish original result/side absent", final)
	}
	if final.GetDamageforwardPercent() != 1 || final.GetDamagelaterPercent() != float64(finished.CurrentHP)/float64(finished.TotalHP) {
		t.Fatal("finish boss HP labels do not describe actual shared HP")
	}
	for i := 0; i < 3; i++ {
		retry, err := f.s.finishP12WorldBossBattleAt(f.ctx, f.id, session, start.GetFightId(), now.Add(time.Minute))
		if err != nil || !retry.AlreadyCommitted || retry.StartBase64 != finished.StartBase64 || retry.CurrentHP != finished.CurrentHP {
			t.Fatal("non-idempotent finish", retry, err)
		}
	}
	if err = db.GetIns().First(&instance, "id=?", boss).Error; err != nil {
		t.Fatal(err)
	}
	if instance.CurrentHP != finished.CurrentHP {
		t.Fatal("repeat damage")
	}
	var finalReceipt db.P12WorldBossBattleReceipt
	if err = db.GetIns().First(&finalReceipt, "fight_id=?", start.GetFightId()).Error; err != nil {
		t.Fatal(err)
	}
	if string(finalReceipt.StartProto) != string(pending.StartProto) || finalReceipt.StartHash != pending.StartHash || len(finalReceipt.FinishProto) == 0 {
		t.Fatal("finish replaced immutable start evidence")
	}
	// 原零戳不被永久唯一键绑定，完成后重新设置隔离野外生命可以开始下一次。
	p12WorldBossBattlePrepare(t, f, f.id)
	next, err := f.s.startP12WorldBossBattleAt(f.ctx, f.id, boss, 0, now.Add(3*time.Minute))
	if err != nil || next.GetFightId() == start.GetFightId() {
		t.Fatal("zero-stamp became permanently replayed", err)
	}
}

func TestP12WorldBossBattleConcurrentFinishCapsSharedHPAndSingleKiller(t *testing.T) {
	f, other, boss, now := p12WorldBossBattleSetup(t)
	ids := []int64{f.id, other}
	starts := make([]*pb.SCWorldBossFight, 2)
	sessions := make([][]byte, 2)
	for i, id := range ids {
		sessions[i] = p12WorldBossBattlePrepare(t, f, id)
		var err error
		starts[i], err = f.s.startP12WorldBossBattleAt(f.ctx, id, boss, int64(i+1), now)
		if err != nil {
			t.Fatal(err)
		}
	}
	// 明确的低血量准备只改变共享实例，用于验证最后一滴HP的并发边界。
	if err := db.GetIns().Model(&db.P12WorldBossBattleInstance{}).Where("id=?", boss).Update("current_hp", 1).Error; err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan *model.P12WorldBossFinishResponse, 2)
	errors := make(chan error, 2)
	for i, id := range ids {
		wg.Add(1)
		go func(i int, id int64) {
			defer wg.Done()
			result, err := f.s.finishP12WorldBossBattleAt(f.ctx, id, sessions[i], starts[i].GetFightId(), now.Add(time.Minute))
			results <- result
			errors <- err
		}(i, id)
	}
	wg.Wait()
	close(results)
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	var total int64
	for result := range results {
		total += result.AppliedDamage
		if result.CurrentHP != 0 {
			t.Fatal("finish exceeded current HP", result)
		}
	}
	if total != 1 {
		t.Fatal("concurrent overkill was counted more than once", total)
	}
	var instance db.P12WorldBossBattleInstance
	if err := db.GetIns().First(&instance, "id=?", boss).Error; err != nil {
		t.Fatal(err)
	}
	if instance.CurrentHP != 0 || instance.KillerID == 0 || instance.KilledAt == 0 {
		t.Fatal("missing single kill", instance)
	}
	rank, err := p12WorldBossBattleRank(db.GetIns(), boss, f.id)
	if err != nil || len(rank.DamageInfos) != 1 || rank.GetLastDamageInfo().GetDamage() != 1 {
		t.Fatal("rank counted precomputed overkill", rank, err)
	}
}

func TestP12WorldBossBattleLocalAggregateFloorOnceAndNoBloodMultiplier(t *testing.T) {
	input := &model.FrozenBattleInput{Monster: &pb.BattleMonster{}}
	terminal := &model.CombatFinalHPOutcome{}
	for i := 0; i < 4; i++ {
		guid := string(rune('a' + i))
		input.Monster.Formation = append(input.Monster.Formation, &pb.BattleHero{Guid: proto.String(guid), BloodNumber: proto.Int32(9), Inst: &pb.BattleHeroInst{CurHp: proto.Float64(10)}})
		terminal.FinalActors = append(terminal.FinalActors, model.CombatFinalActorHP{Camp: 1, GUID: guid, CurrentHP: 9.6, BloodNumber: 9})
	}
	damage, err := p12WorldBossBattleDamage(input, terminal)
	if err != nil || damage != 1 {
		t.Fatal("local sum must floor once and ignore bar count", damage, err)
	}
	terminal.FinalActors[0].CurrentHP = 100
	damage, err = p12WorldBossBattleDamage(input, terminal)
	if err != nil || damage != 1 {
		t.Fatal("healing must not subtract other actor damage", damage, err)
	}
}

func TestP12WorldBossBattleUnboostedPartyZeroDamageIsValid(t *testing.T) {
	f, _, boss, now := p12WorldBossBattleSetup(t, true)
	session := p12WorldBossBattlePrepare(t, f, f.id)
	start, err := f.s.startP12WorldBossBattleAt(f.ctx, f.id, boss, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	var receipt db.P12WorldBossBattleReceipt
	if err = db.GetIns().First(&receipt, "fight_id=?", start.GetFightId()).Error; err != nil {
		t.Fatal(err)
	}
	if receipt.PredictedDamage != 0 {
		t.Fatal("original ungeared fixture no longer reproduces zero damage", receipt.PredictedDamage)
	}
	var terminal model.CombatFinalHPOutcome
	if err = json.Unmarshal([]byte(receipt.TerminalJSON), &terminal); err != nil {
		t.Fatal(err)
	}
	var releases, misses int
	for _, event := range terminal.Outcome.Events {
		if event.Camp == 0 && event.Kind == "release" {
			releases++
			if event.Miss {
				misses++
			}
		}
	}
	t.Logf("original ungeared hit=%v bossDodge=%v attackerReleases=%d misses=%d winner=%d", start.BattlePlayer.Formation[0].Prop.GetHit(), start.BattleMonsterList[0].Formation[0].Prop.GetDodge(), releases, misses, terminal.Outcome.Winner)
	finish, err := f.s.finishP12WorldBossBattleAt(f.ctx, f.id, session, start.GetFightId(), now.Add(time.Minute))
	if err != nil || finish.AppliedDamage != 0 || finish.CurrentHP != finish.TotalHP {
		t.Fatal("valid zero damage changed shared HP", finish, err)
	}
}

func TestP12WorldBossBattleBoostActualInputAndRewardCenter(t *testing.T) {
	f, _, boss, now := p12WorldBossBattleSetup(t, true)
	if err := f.s.InitializeP1(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P12WorldBossBattlePolicy{}).Where("id=1").Updates(map[string]any{"economy_enabled": true, "rewards_enabled": true}).Error; err != nil {
		t.Fatal(err)
	}
	// 精确450钻为隔离测试的有限购买前提；实际通过原购买服务扣费并生成加成凭据。
	if err := updateProfile(f.ctx, f.id, func(player *pb.Player, _ *model.GameState) error { player.Diamond = proto.Int64(450); return nil }); err != nil {
		t.Fatal(err)
	}
	for _, property := range []string{"wordboss_1", "wordboss_3", "wordboss_4"} {
		if _, err := f.s.PurchaseP12WorldBossProp(f.ctx, f.id, property, now); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.s.PurchaseP12WorldBossProp(f.ctx, f.id, "wordboss_1", now); err != nil {
		t.Fatal(err)
	}
	player, _ := f.profile(t)
	if player.GetDiamond() != 0 {
		t.Fatal("boost cost or duplicate buy drift", player.GetDiamond())
	}
	session := p12WorldBossBattlePrepare(t, f, f.id)
	start, err := f.s.startP12WorldBossBattleAt(f.ctx, f.id, boss, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	if start.BattlePlayer.Formation[0].Prop.GetHit() != 175 {
		t.Fatal("paid original hit80 did not enter input")
	}
	if _, err = f.s.finishP12WorldBossBattleAt(f.ctx, f.id, session, start.GetFightId(), now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err = f.s.SyncP12WorldBossRewards(f.ctx, f.id); err != nil {
		t.Fatal(err)
	}
	var awards []db.P12WorldBossAward
	if err = db.GetIns().Where("instance_id=? AND player_id=?", boss, f.id).Order("kind").Find(&awards).Error; err != nil {
		t.Fatal(err)
	}
	if len(awards) < 2 {
		t.Fatal("ended cycle participation and damage rank awards missing", awards)
	}
	var ids []int64
	for _, award := range awards {
		if award.DeliveryID > 0 {
			ids = append(ids, award.DeliveryID)
		}
	}
	if len(ids) < 2 {
		t.Fatal("awards not delivered to actual reward center")
	}
	if _, err = f.s.RewardCenter(f.ctx, f.id, &pb.CSAddRewardsToPlayer{RewardIds: ids}); err != nil {
		t.Fatal(err)
	}
	before, _ := f.profile(t)
	if _, err = f.s.RewardCenter(f.ctx, f.id, &pb.CSAddRewardsToPlayer{RewardIds: ids}); err != nil {
		t.Fatal(err)
	}
	after, _ := f.profile(t)
	if before.GetCoin() != after.GetCoin() {
		t.Fatal("duplicate reward center claim paid twice")
	}
	if err = f.s.SyncP12WorldBossRewards(f.ctx, f.id); err != nil {
		t.Fatal(err)
	}
	var count int64
	db.GetIns().Model(&db.P12WorldBossAward{}).Where("instance_id=? AND player_id=?", boss, f.id).Count(&count)
	if count != int64(len(awards)) {
		t.Fatal("reward refresh duplicated awards")
	}
}

func TestP12WorldBossBattlePurchaseUnsupportedRosterDoesNotCharge(t *testing.T) {
	f, _, _, now := p12WorldBossBattleSetup(t, true)
	if err := db.GetIns().Model(&db.P12WorldBossBattlePolicy{}).Where("id=1").Update("economy_enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(f.ctx, f.id, func(player *pb.Player, game *model.GameState) error {
		player.Diamond = proto.Int64(150)
		// 有限的另一合法红伙伴前提：P5重建真实004，仍在001003首领证明之外。
		game.Warriors[1].Id = proto.String("hero_004")
		for _, thing := range game.Formation[1].Things {
			if thing.GetPos() == 0 {
				thing.Id = proto.String("hero_004")
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	player, game := f.profile(t)
	party, err := f.s.battlePlayer(player, game)
	if err != nil || len(party.GetFormation()) != 2 || party.Formation[1].GetId() != "hero_004" {
		t.Fatal("negative prerequisite must reach a valid different P5 battle actor", err)
	}
	var before db.Profile
	if err := db.GetIns().First(&before, f.id).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.PurchaseP12WorldBossProp(f.ctx, f.id, "wordboss_1", now); err == nil {
		t.Fatal("unverified roster purchased a combat boost")
	}
	var after db.Profile
	if err := db.GetIns().First(&after, f.id).Error; err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := db.GetIns().Model(&db.P12WorldBossBoost{}).Where("player_id=?", f.id).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if before.Player == nil || after.Player == nil || before.Game == nil || after.Game == nil || *before.Player != *after.Player || *before.Game != *after.Game || count != 0 {
		t.Fatal("rejected purchase changed wallet, roster or boost ledger")
	}
}

func TestP12WorldBossBattleExpiredPendingAbandonsWithoutDamage(t *testing.T) {
	f, _, boss, now := p12WorldBossBattleSetup(t)
	session := p12WorldBossBattlePrepare(t, f, f.id)
	start, err := f.s.startP12WorldBossBattleAt(f.ctx, f.id, boss, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	if err = updateProfileWithTx(f.ctx, f.id, func(tx *gorm.DB, player *pb.Player, game *model.GameState) error {
		changed, err := f.s.ExpireP12WorldBossPendingLocked(tx, player, game, now.Add(time.Hour))
		if err != nil {
			return err
		}
		if !changed || game.Field.Side.GetState() == 6 {
			t.Fatal("expired pending retained busy state")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.finishP12WorldBossBattleAt(f.ctx, f.id, session, start.GetFightId(), now.Add(time.Minute)); err == nil {
		t.Fatal("expired abandoned fight applied damage")
	}
	var instance db.P12WorldBossBattleInstance
	db.GetIns().First(&instance, "id=?", boss)
	if instance.CurrentHP != instance.TotalHP {
		t.Fatal("expiry auto-applied damage")
	}
	if pending, err := HasPendingP12WorldBossBattle(f.ctx, f.id); err != nil || pending {
		t.Fatal("abandoned fight still blocks gameplay", pending, err)
	}
}

func TestP12WorldBossBattleOrdinaryActionExpiresSameObjectAndNextCycleStarts(t *testing.T) {
	f, _, boss, now := p12WorldBossBattleSetup(t)
	start, err := f.s.startP12WorldBossBattleAt(f.ctx, f.id, boss, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	// 正常存档动作通过公共角色事务；helper必须改同一对象，外层保存不能覆盖已恢复Side。
	if err = updateProfile(ctx, f.id, func(player *pb.Player, game *model.GameState) error {
		if game.Field.Side.GetState() == 6 {
			t.Fatal("ordinary action still sees expired world-boss busy state")
		}
		player.Coin = proto.Int64(player.GetCoin() + 1)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var receipt db.P12WorldBossBattleReceipt
	if err = db.GetIns().First(&receipt, "fight_id=?", start.GetFightId()).Error; err != nil {
		t.Fatal(err)
	}
	if receipt.AbandonedAt == 0 || receipt.Committed || receipt.AppliedDamage != 0 {
		t.Fatal("ordinary action submitted or retained expired pending", receipt.AbandonReason)
	}
	_, stored := f.profile(t)
	if stored.Field.Side.GetState() == 6 {
		t.Fatal("outer save overwrote restored Side")
	}
	newNow := now.Add(24 * time.Hour)
	newBoss := p12WorldBossCall(t, f, f.id, &pb.CSWorldBossInfo{}, newNow).(*pb.SCWorldBossInfo).GetWorldBossShowInfo().GetBossuuid()
	if newBoss == boss {
		t.Fatal("new cycle reused old UUID")
	}
	next, err := f.s.startP12WorldBossBattleAt(f.ctx, f.id, newBoss, 0, newNow)
	if err != nil || next.GetFightId() == start.GetFightId() {
		t.Fatal("expired previous cycle permanently blocked new battle", err)
	}
}
