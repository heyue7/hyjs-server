package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func feedback81TestForm() model.AdminActivityForm {
	return model.AdminActivityForm{
		Mode: constUtil.Feedback81Mode, TemplateID: constUtil.Feedback81ArtTemplate, Name: constUtil.Feedback81OriginalName,
		Source: constUtil.Feedback81Source, Alias: constUtil.Feedback81Alias, RepeatPolicy: "repeat",
		Products: []model.AdminActivityProduct{}, Prizes: []model.AdminActivityPrize{},
		Milestones: []model.AdminActivityMilestone{
			{Goal: 1, Rewards: []model.AdminResourceAmount{{ResourceID: "item_9760", Amount: 1}}},
			{Goal: 2, Rewards: []model.AdminResourceAmount{{ResourceID: "equip_025", Amount: 1}, {ResourceID: "equip_026", Amount: 1}}},
			{Goal: 3, Rewards: []model.AdminResourceAmount{{ResourceID: "equip_027", Amount: 1}, {ResourceID: "equip_028", Amount: 1}}},
		},
	}
}

func feedback81Fixture(t *testing.T) (*p1Fixture, *model.AdminPrincipal) {
	t.Helper()
	f, actor := reuseFixture(t, false)
	f.s.InstallGrowth74Resources()
	form := feedback81TestForm()
	definition, err := f.s.AdminSaveActivityDefinition(f.ctx, actor, "", model.AdminActivityDefinitionSaveRequest{Name: form.Name, Form: form, DefaultDays: 28, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Model(&db.AdminActivityDefinition{}).Where("id=?", definition.ID).Update("id", constUtil.Feedback81TemplateID).Error; err != nil {
		t.Fatal(err)
	}
	var stored db.AdminActivityDefinition
	if err = db.GetIns().First(&stored, "id=?", constUtil.Feedback81TemplateID).Error; err != nil {
		t.Fatal(err)
	}
	definition.ID = stored.ID
	definition.Version = adminVersion(stored)
	occ := reusePublish(t, f, actor, *definition, 1, 28, "character-day")
	if err = db.GetIns().Model(&db.AdminActivityOccurrence{}).Where("id=?", occ.ID).Updates(map[string]any{"id": constUtil.Feedback81OccurrenceID, "lane": 2}).Error; err != nil {
		t.Fatal(err)
	}
	if err = f.s.InitializeClientUpdates(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Model(&db.ClientUpdatePolicy{}).Where("id=?", 1).Update("maintenance", true).Error; err != nil {
		t.Fatal(err)
	}
	return f, actor
}

func feedback81NewPlayer(t *testing.T, f *p1Fixture) int64 {
	t.Helper()
	account, err := LoginAccount(f.ctx, fmt.Sprintf("feedback81-%d", time.Now().UnixNano()), "isolated-test-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(f.ctx, account.Id, "登录迁移夹具", "hero_043"); err != nil {
		t.Fatal(err)
	}
	return account.Id
}

func feedback81Issue(t *testing.T, f *p1Fixture, playerID int64, legacy bool, receiptState string) (db.AdminCharacterActivityInstance, db.PeriodInstance, db.RewardDelivery) {
	t.Helper()
	ctx := serverClockBindTest(t, f, playerID)
	if err := f.s.PrepareAdminCharacterActivitySchedules(ctx, playerID); err != nil {
		t.Fatal(err)
	}
	var issued db.AdminCharacterActivityInstance
	if err := db.GetIns().First(&issued, "player_id=? AND occurrence_id=?", playerID, constUtil.Feedback81OccurrenceID).Error; err != nil {
		t.Fatal(err)
	}
	var period db.PeriodInstance
	if err := db.GetIns().First(&period, "id=?", issued.ConsumerKey).Error; err != nil {
		t.Fatal(err)
	}
	if legacy {
		tiers := p1PeriodMilestones(period)
		for i := range tiers {
			tiers[i].SourceKey = activity77PeriodSource(period.ID, i)
		}
		period.Milestones = adminJSON(tiers)
	}
	if receiptState == "none" {
		if legacy {
			if err := db.GetIns().Save(&period).Error; err != nil {
				t.Fatal(err)
			}
		}
		return issued, period, db.RewardDelivery{}
	}
	period.Frozen = adminJSON(period)
	if err := db.GetIns().Save(&period).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	receipt := db.RewardDelivery{PlayerID: playerID, SourceKey: p1PeriodSources(period)[0], Title: period.Title, Content: period.Source,
		Rewards: constUtil.Feedback81OldReward, CreatedAt: now - 2000, ExpiresAt: period.RewardEndAt}
	if receiptState == "claimed" {
		receipt.ClaimedAt = now - 1000
		receipt.Result = []byte{1, 2, 3}
	}
	if err := db.GetIns().Create(&receipt).Error; err != nil {
		t.Fatal(err)
	}
	progress := db.PeriodProgress{PlayerID: playerID, InstanceID: period.ID, Value: 1, LastDay: p1Day(time.Now()), RewardID: receipt.ID, Snapshot: adminJSON(period)}
	if err := db.GetIns().Create(&progress).Error; err != nil {
		t.Fatal(err)
	}
	return issued, period, receipt
}

func TestFeedback81MigrationAndOriginalClaim(t *testing.T) {
	f, actor := feedback81Fixture(t)
	pendingID := f.id
	claimedID := feedback81NewPlayer(t, f)
	noReceiptID := feedback81NewPlayer(t, f)
	_, pending, pendingReceipt := feedback81Issue(t, f, pendingID, false, "pending")
	_, claimed, claimedReceipt := feedback81Issue(t, f, claimedID, true, "claimed")
	_, empty, _ := feedback81Issue(t, f, noReceiptID, true, "none")
	// 77 left the earlier four-orange source as a separate claimed history.
	legacyReceipt := db.RewardDelivery{PlayerID: claimedID, SourceKey: "activity:" + claimed.ID, Title: claimed.Title, Content: claimed.Source,
		Rewards: "equip_025,1;equip_026,1;equip_027,1;equip_028,1", CreatedAt: time.Now().UnixMilli() - 5000, ExpiresAt: claimed.RewardEndAt, ClaimedAt: time.Now().UnixMilli() - 4000}
	if err := db.GetIns().Create(&legacyReceipt).Error; err != nil {
		t.Fatal(err)
	}
	var profilesBefore []db.Profile
	if err := db.GetIns().Where("id IN ?", []int64{pendingID, claimedID, noReceiptID}).Order("id").Find(&profilesBefore).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.ApplyFeedback81Config(f.ctx, actor); err != nil {
		t.Fatal("apply", err)
	}
	var profilesAfter []db.Profile
	if err := db.GetIns().Where("id IN ?", []int64{pendingID, claimedID, noReceiptID}).Order("id").Find(&profilesAfter).Error; err != nil || !reflect.DeepEqual(profilesBefore, profilesAfter) {
		t.Fatal("maintenance changed player assets", err)
	}
	var savedPending, savedClaimed, savedLegacy db.RewardDelivery
	for _, pair := range []struct {
		id  int64
		out *db.RewardDelivery
	}{{pendingReceipt.ID, &savedPending}, {claimedReceipt.ID, &savedClaimed}, {legacyReceipt.ID, &savedLegacy}} {
		if err := db.GetIns().First(pair.out, pair.id).Error; err != nil {
			t.Fatal(err)
		}
	}
	if savedPending.Rewards != constUtil.Feedback81NewReward {
		t.Fatal("existing unclaimed receipt not updated", savedPending)
	}
	pendingReceipt.Rewards = constUtil.Feedback81NewReward
	if !reflect.DeepEqual(pendingReceipt, savedPending) || !reflect.DeepEqual(claimedReceipt, savedClaimed) || !reflect.DeepEqual(legacyReceipt, savedLegacy) {
		t.Fatal("receipt metadata or claimed history changed")
	}
	if count := p1Count(t, &db.RewardDelivery{}, "player_id IN ?", []int64{pendingID, claimedID, noReceiptID}); count != 3 {
		t.Fatal("maintenance created a receipt", count)
	}
	for _, old := range []db.PeriodInstance{pending, claimed, empty} {
		var period db.PeriodInstance
		if err := db.GetIns().First(&period, "id=?", old.ID).Error; err != nil {
			t.Fatal(err)
		}
		if period.Rewards != constUtil.Feedback81NewReward || !reflect.DeepEqual(p1PeriodSources(old), p1PeriodSources(period)) {
			t.Fatal("period sources or reward changed incorrectly", old.ID)
		}
		if len(old.Frozen) > 0 {
			var frozen db.PeriodInstance
			if err := json.Unmarshal(period.Frozen, &frozen); err != nil || frozen.Rewards != constUtil.Feedback81NewReward || !reflect.DeepEqual(p1PeriodSources(old), p1PeriodSources(frozen)) {
				t.Fatal("frozen period", err)
			}
		}
	}
	for _, old := range []db.PeriodInstance{pending, claimed} {
		var progress db.PeriodProgress
		if err := db.GetIns().First(&progress, "instance_id=?", old.ID).Error; err != nil {
			t.Fatal(err)
		}
		var snapshot db.PeriodInstance
		if json.Unmarshal(progress.Snapshot, &snapshot) != nil || progress.Value != 1 || snapshot.Rewards != constUtil.Feedback81NewReward || !reflect.DeepEqual(p1PeriodSources(old), p1PeriodSources(snapshot)) {
			t.Fatal("progress snapshot changed incorrectly")
		}
	}
	if err := f.s.ApplyFeedback81Config(f.ctx, actor); err != nil {
		t.Fatal("verified retry", err)
	}
	if count := p1Count(t, &db.AdminAudit{}, "action=? AND target=?", constUtil.Feedback81AuditAction, constUtil.Feedback81OccurrenceID); count != 1 {
		t.Fatal("retry duplicated audit", count)
	}
	newPlayer := feedback81NewPlayer(t, f)
	_, later, _ := feedback81Issue(t, f, newPlayer, false, "none")
	if later.Rewards != constUtil.Feedback81NewReward {
		t.Fatal("newly issued period did not inherit updated form")
	}
	f.restart(t)
	pendingCtx := serverClockBindTest(t, f, pendingID)
	info, err := f.s.P1Activity(pendingCtx, pendingID, &pb.CSActivityInfo{ActivityId: proto.String(pending.ID)})
	if err != nil {
		t.Fatal("cold original activity info", err)
	}
	items := info[0].(*pb.SCActivityInfo).GetItemList()
	if len(items) != 3 || items[0].GetStatus() != 1 || !p1SameResources(items[0].GetRewardList(), []*pb.BaseResource{{Id: proto.String("item_9760"), Num: proto.Int32(1)}, {Id: proto.String("equip_041"), Num: proto.Int32(1)}}) {
		t.Fatal("pending original protocol reward", items)
	}
	request := &pb.CSActivityReward{ActivityId: proto.String(pending.ID), Index: proto.Int32(1), GetResources: items[0].GetRewardList()}
	if _, err = f.s.P1Activity(pendingCtx, pendingID, request); err != nil {
		t.Fatal("original protocol claim", err)
	}
	f.id = pendingID
	player, game := f.profile(t)
	var swords int
	for _, equip := range game.Equipment {
		if equip.GetEquipId() == "equip_041" {
			swords++
		}
	}
	if simpleOwned(player, game, "item_9760") != 1 || swords != 1 {
		t.Fatal("original claim did not grant one OS box and sword")
	}
	if _, err = f.s.P1Activity(pendingCtx, pendingID, request); err != nil {
		t.Fatal("original claim retry", err)
	}
	f.restart(t)
	playerAfter, gameAfter := f.profile(t)
	if !proto.Equal(player, playerAfter) || !bytes.Equal(adminJSON(game), adminJSON(gameAfter)) {
		t.Fatal("cold retry granted twice")
	}
	claimedCtx := serverClockBindTest(t, f, claimedID)
	claimedInfo, err := f.s.P1Activity(claimedCtx, claimedID, &pb.CSActivityInfo{ActivityId: proto.String(claimed.ID)})
	if err != nil {
		t.Fatal(err)
	}
	claimedItems := claimedInfo[0].(*pb.SCActivityInfo).GetItemList()
	if claimedItems[0].GetStatus() != 2 {
		t.Fatal("old claimed tier became claimable")
	}
	claimedReq := &pb.CSActivityReward{ActivityId: proto.String(claimed.ID), Index: proto.Int32(1), GetResources: claimedItems[0].GetRewardList()}
	if result, err := f.s.P1Activity(claimedCtx, claimedID, claimedReq); err != nil || len(result[0].(*pb.SCActivityReward).GetGet().GetEquips()) != 0 {
		t.Fatal("claimed historical reward reissued", err)
	}
}

func TestFeedback81RejectsAndRollsBack(t *testing.T) {
	f, actor := feedback81Fixture(t)
	_, period, receipt := feedback81Issue(t, f, f.id, false, "pending")
	other := feedback81NewPlayer(t, f)
	_, bad, badReceipt := feedback81Issue(t, f, other, true, "pending")
	if err := db.GetIns().Model(&db.PeriodProgress{}).Where("instance_id=? AND player_id=?", bad.ID, other).Update("snapshot", []byte(`{}`)).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.ApplyFeedback81Config(f.ctx, actor); err == nil {
		t.Fatal("corrupt progress accepted")
	}
	var saved db.PeriodInstance
	var savedReceipt db.RewardDelivery
	if err := db.GetIns().First(&saved, "id=?", period.ID).Error; err != nil || saved.Rewards != constUtil.Feedback81OldReward {
		t.Fatal("failed migration changed period", err)
	}
	if err := db.GetIns().First(&savedReceipt, receipt.ID).Error; err != nil || !reflect.DeepEqual(savedReceipt, receipt) {
		t.Fatal("failed migration changed receipt", err)
	}
	var progress db.PeriodProgress
	if err := db.GetIns().First(&progress, "instance_id=? AND player_id=?", period.ID, f.id).Error; err != nil || !bytes.Equal(progress.Snapshot, adminJSON(period)) {
		t.Fatal("failed migration changed earlier snapshot", err)
	}
	if count := p1Count(t, &db.AdminAudit{}, "action=?", constUtil.Feedback81AuditAction); count != 0 {
		t.Fatal("failed migration wrote audit", count)
	}
	if err := db.GetIns().Model(&db.PeriodProgress{}).Where("instance_id=? AND player_id=?", bad.ID, other).Update("snapshot", adminJSON(bad)).Error; err != nil {
		t.Fatal(err)
	}
	// This error occurs after both progress snapshots have been patched in the
	// transaction, so the assertions exercise rollback of already-written rows.
	if err := db.GetIns().Model(&db.RewardDelivery{}).Where("id=?", badReceipt.ID).Update("rewards", "item_9760,2").Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.ApplyFeedback81Config(f.ctx, actor); err == nil {
		t.Fatal("invalid receipt accepted")
	}
	if err := db.GetIns().First(&progress, "instance_id=? AND player_id=?", period.ID, f.id).Error; err != nil || !bytes.Equal(progress.Snapshot, adminJSON(period)) {
		t.Fatal("later failure did not roll back the snapshot", err)
	}
	if err := db.GetIns().First(&savedReceipt, receipt.ID).Error; err != nil || !reflect.DeepEqual(savedReceipt, receipt) {
		t.Fatal("later failure did not roll back the receipt", err)
	}
	if err := db.GetIns().Model(&db.RewardDelivery{}).Where("id=?", badReceipt.ID).Update("rewards", badReceipt.Rewards).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.ClientUpdatePolicy{}).Where("id=?", 1).Update("maintenance", false).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.ApplyFeedback81Config(f.ctx, actor); err == nil {
		t.Fatal("open server accepted maintenance")
	}
	if err := db.GetIns().Model(&db.ClientUpdatePolicy{}).Where("id=?", 1).Update("maintenance", true).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.AdminUser{}).Where("id=?", actor.UserID).Update("role", constUtil.AdminRoleOperator).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.ApplyFeedback81Config(f.ctx, actor); err == nil {
		t.Fatal("operator accepted")
	}
	if err := db.GetIns().Model(&db.AdminUser{}).Where("id=?", actor.UserID).Update("role", constUtil.AdminRoleAdmin).Error; err != nil {
		t.Fatal(err)
	}
	f.s.ServerID = 2
	if err := f.s.ApplyFeedback81Config(f.ctx, actor); err == nil {
		t.Fatal("foreign serving realm accepted")
	}
	f.s.ServerID = 0
	foreign := db.AdminActivityOccurrence{ID: "foreign81", TemplateID: constUtil.Feedback81TemplateID, ServerID: 2, ConsumerKey: "foreign81", ConsumerID: "period:foreign81", FormJSON: adminJSON(feedback81TestForm())}
	if err := db.GetIns().Create(&foreign).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.ApplyFeedback81Config(f.ctx, actor); err == nil {
		t.Fatal("foreign template reference accepted")
	}
	if err := db.GetIns().Delete(&foreign).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.ApplyFeedback81Config(f.ctx, actor); err != nil {
		t.Fatal("repaired migration", err)
	}
	if err := db.GetIns().Model(&db.PeriodInstance{}).Where("id=?", period.ID).Update("rewards", "item_9760,2").Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.ApplyFeedback81Config(f.ctx, actor); err == nil {
		t.Fatal("audited retry skipped verification")
	}
}

func TestFeedback81MySQLConcurrentClaim(t *testing.T) {
	if os.Getenv("HYJS_P1_MYSQL_DATABASE") == "" {
		t.Skip("requires an isolated hyjs_test_p1_ MySQL database")
	}
	f, actor := feedback81Fixture(t)
	_, period, receipt := feedback81Issue(t, f, f.id, true, "pending")
	ctx := serverClockBindTest(t, f, f.id)
	info, err := f.s.P1Activity(ctx, f.id, &pb.CSActivityInfo{ActivityId: proto.String(period.ID)})
	if err != nil {
		t.Fatal(err)
	}
	request := &pb.CSActivityReward{ActivityId: proto.String(period.ID), Index: proto.Int32(1), GetResources: info[0].(*pb.SCActivityInfo).GetItemList()[0].GetRewardList()}
	start := make(chan struct{})
	applyResult := make(chan error, 1)
	claimResult := make(chan error, 1)
	go func() {
		<-start
		applyResult <- f.s.ApplyFeedback81Config(f.ctx, actor)
	}()
	go func() {
		<-start
		_, e := f.s.P1Activity(ctx, f.id, request)
		claimResult <- e
	}()
	close(start)
	if err := <-applyResult; err != nil {
		t.Fatal("concurrent maintenance", err)
	}
	claimErr := <-claimResult
	info, err = f.s.P1Activity(ctx, f.id, &pb.CSActivityInfo{ActivityId: proto.String(period.ID)})
	if err != nil {
		t.Fatal(err)
	}
	current := info[0].(*pb.SCActivityInfo).GetItemList()[0]
	request.GetResources = current.GetRewardList()
	if claimErr != nil {
		if _, err = f.s.P1Activity(ctx, f.id, request); err != nil {
			t.Fatal("claim after updated rules", err)
		}
	}
	if _, err = f.s.P1Activity(ctx, f.id, request); err != nil {
		t.Fatal("claim retry", err)
	}
	var saved db.RewardDelivery
	if err := db.GetIns().First(&saved, receipt.ID).Error; err != nil || saved.ClaimedAt == 0 || (saved.Rewards != constUtil.Feedback81OldReward && saved.Rewards != constUtil.Feedback81NewReward) {
		t.Fatal("concurrent receipt", saved, err)
	}
	player, game := f.profile(t)
	swords := 0
	for _, equip := range game.Equipment {
		if equip.GetEquipId() == "equip_041" {
			swords++
		}
	}
	wantSwords := 0
	if saved.Rewards == constUtil.Feedback81NewReward {
		wantSwords = 1
	}
	if simpleOwned(player, game, "item_9760") != 1 || swords != wantSwords || p1Count(t, &db.RewardDelivery{}, "player_id=? AND source_key=?", f.id, receipt.SourceKey) != 1 {
		t.Fatal("concurrent claim and receipt disagree or duplicated")
	}
	f.restart(t)
	playerAfter, gameAfter := f.profile(t)
	if !proto.Equal(player, playerAfter) || !bytes.Equal(adminJSON(game), adminJSON(gameAfter)) {
		t.Fatal("cold concurrent result changed")
	}
}
