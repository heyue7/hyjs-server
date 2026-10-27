package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func adminKingFixture(t *testing.T) (*p1Fixture, *model.AdminPrincipal, model.AdminActivityForm) {
	t.Helper()
	f, actor := reuseFixture(t, false)
	if err := f.s.InitializeLocalPlayActivities(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP2(f.ctx); err != nil {
		t.Fatal(err)
	}
	account, err := LoginAccount(f.ctx, "king-"+p6Digest([]byte(t.Name()))[:24], "isolated-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(f.ctx, account.Id, "王者原入口", "hero_043"); err != nil {
		t.Fatal(err)
	}
	f.id = account.Id
	p4Fund(t, f)
	var cfg db.GameplayPool
	if err = db.GetIns().First(&cfg, "id=?", "recruit_3").Error; err != nil {
		t.Fatal(err)
	}
	var rewards []db.GameplayPoolReward
	if err = db.GetIns().Where("pool_id=?", cfg.ID).Find(&rewards).Error; err != nil {
		t.Fatal(err)
	}
	form, err := f.s.adminKingRecruitActivityForm(db.GetIns().WithContext(f.ctx), cfg, rewards)
	if err != nil {
		t.Fatal(err)
	}
	// A finite isolated catalogue gives one actual original red per native world;
	// production seeds and their existing weights are never modified by this test.
	form.Name = "原王者有限代表"
	form.Source = "isolated original costType3 contract fixture"
	form.RepeatPolicy = "repeat"
	form.Prizes = []model.AdminActivityPrize{{ID: 1, ResourceID: "hero_001", Amount: 1, Weight: 1, Guaranteed: true}, {ID: 2, ResourceID: "hero_003", Amount: 1, Weight: 1, Guaranteed: true}, {ID: 3, ResourceID: "hero_005", Amount: 1, Weight: 1, Guaranteed: true}}
	return f, actor, form
}
func adminKingDefinition(t *testing.T, f *p1Fixture, actor *model.AdminPrincipal, form model.AdminActivityForm) model.AdminActivityDefinitionDTO {
	t.Helper()
	row, err := f.s.AdminSaveActivityDefinition(f.ctx, actor, "", model.AdminActivityDefinitionSaveRequest{Name: form.Name, Form: form, DefaultDays: 1, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	return *row
}
func adminKingInfo(t *testing.T, f *p1Fixture, ctx context.Context) *pb.WarriorRecruit {
	t.Helper()
	messages, err := f.s.recruitInfo(ctx, f.id, &pb.CSWarriorRecruitGet{CostType: []int32{3}})
	if err != nil || len(messages) != 1 {
		t.Fatal("original king info", err)
	}
	entries := messages[0].(*pb.SCWarriorRecruitGet).GetRecruit()
	if len(entries) != 1 {
		t.Fatal("original king info entry missing")
	}
	return entries[0]
}

func TestAdminKingRecruitScheduledNativeConsumersAndCold(t *testing.T) {
	f, actor, form := adminKingFixture(t)
	var original db.GameplayPool
	db.GetIns().First(&original, "id=?", "recruit_3")
	originalRaw := adminJSON(original)
	bound := serverClockBindTest(t, f, f.id)
	if adminKingInfo(t, f, bound).GetOn() != 1 {
		t.Fatal("legacy permanent king changed before explicit scheduling")
	}
	definition := adminKingDefinition(t, f, actor, form)
	slot := reusePublish(t, f, actor, definition, 1, 1, "calendar")
	info := adminKingInfo(t, f, bound)
	if info.GetOn() != 1 || len(info.GetRewardList()) != 1 || info.GetNum() != 50 || info.GetLeftTimes() != 9 {
		t.Fatal("new native king header did not use scheduled pool", info)
	}
	preview, err := f.s.kingRecruitPreview(bound, f.id, &pb.CSWarriorRecruitGetPreview{CostType: proto.Int32(3)})
	if err != nil || len(preview[0].(*pb.SCWarriorRecruitGetPreview).GetRewards()) != 1 || preview[0].(*pb.SCWarriorRecruitGetPreview).GetRewards()[0] != info.GetRewardList()[0] {
		t.Fatal("preview differs from actual native pool", err)
	}
	before, _ := f.profile(t)
	request := p2Recruit(1, 0)
	paid, err := f.s.warriorRecruit(bound, f.id, request, 95001)
	if err != nil {
		t.Fatal("original costType3 paid draw", err)
	}
	after, _ := f.profile(t)
	if before.GetDiamond()-after.GetDiamond() != 300 || after.GetCoin()-before.GetCoin() != 10000 || len(paid[0].(*pb.SCWarriorRecruit).GetGet().GetWars()) != 1 || paid[0].(*pb.SCWarriorRecruit).GetGet().GetWars()[0].GetId() != info.GetRewardList()[0] {
		t.Fatal("native price/result differs from configured preview")
	}
	_, key, _ := strings.Cut(slot.ConsumerID, ":")
	var snapshot db.KingRecruitActivitySnapshot
	if err = db.GetIns().First(&snapshot, "pool_id=?", key).Error; err != nil || !snapshot.Frozen {
		t.Fatal("issued original rules were not frozen", err)
	}
	frozenRaw := append([]byte{}, snapshot.Payload...)
	reuseToggle(t, f, actor, slot.ID, false)
	if adminKingInfo(t, f, bound).GetOn() != 0 {
		t.Fatal("stopped king window remained active")
	}
	if _, err = f.s.warriorRecruit(bound, f.id, request, 95002); !errors.Is(err, constUtil.ErrKingRecruitActivityClosed) {
		t.Fatal("stopped paid draw accepted", err)
	}
	if _, err = f.s.kingRecruitPreview(bound, f.id, &pb.CSWarriorRecruitGetPreview{CostType: proto.Int32(3)}); !errors.Is(err, constUtil.ErrKingRecruitActivityClosed) {
		t.Fatal("stopped preview accepted", err)
	}
	if _, err = f.s.kingRecruitLucky(bound, f.id, &pb.CSWarriorRecruitLuckyReward{Reward: proto.String(info.GetRewardList()[0])}, 95003); !errors.Is(err, constUtil.ErrKingRecruitActivityClosed) {
		t.Fatal("stopped lucky grant accepted", err)
	}
	retry, err := f.s.warriorRecruit(bound, f.id, request, 95001)
	if err != nil || !proto.Equal(paid[0], retry[0]) {
		t.Fatal("stop broke already committed native receipt", err)
	}
	reuseToggle(t, f, actor, slot.ID, true)
	reopened := adminKingInfo(t, f, bound)
	if reopened.GetNum() != 49 || reopened.GetLeftTimes() != 9 || reopened.GetLuck() != 10 {
		t.Fatal("stop/start reset daily count, pity or luck", reopened)
	}
	f.restart(t)
	bound = serverClockBindTest(t, f, f.id)
	retry, err = f.s.warriorRecruit(bound, f.id, request, 95001)
	if err != nil || !proto.Equal(paid[0], retry[0]) {
		t.Fatal("cold receipt changed", err)
	}
	if err = db.GetIns().First(&snapshot, "pool_id=?", key).Error; err != nil || string(snapshot.Payload) != string(frozenRaw) {
		t.Fatal("cold restore changed original rules snapshot", err)
	}
	db.GetIns().First(&original, "id=?", "recruit_3")
	if string(adminJSON(original)) != string(originalRaw) {
		t.Fatal("scheduling changed the legacy permanent pool")
	}
}

func TestAdminKingRecruitOnceRepeatOwnershipAndCounters(t *testing.T) {
	for _, policy := range []string{"once", "repeat"} {
		t.Run(policy, func(t *testing.T) {
			f, actor, form := adminKingFixture(t)
			form.RepeatPolicy = policy
			definition := adminKingDefinition(t, f, actor, form)
			reusePublish(t, f, actor, definition, 1, 1, "calendar")
			reusePublish(t, f, actor, definition, 2, 1, "calendar")
			bound := serverClockBindTest(t, f, f.id)
			if _, err := f.s.warriorRecruit(bound, f.id, p2Recruit(1, 0), 95101); err != nil {
				t.Fatal(err)
			}
			adminKingInfo(t, f, bound)
			reuseClockDay(t, f, actor)
			bound = serverClockBindTest(t, f, f.id)
			info := adminKingInfo(t, f, bound)
			if policy == "once" {
				if info.GetOn() != 0 {
					t.Fatal("once template accepted a second occurrence")
				}
				return
			}
			// The permanent shared balance survives a date and world change.
			wantLuck := int32(10)
			if info.GetOn() != 1 || info.GetLuck() != wantLuck {
				t.Fatal("repeat occurrence did not retain shared luck", info)
			}
			if _, err := f.s.warriorRecruit(bound, f.id, p2Recruit(1, 0), 95102); err != nil {
				t.Fatal(err)
			}
			info = adminKingInfo(t, f, bound)
			if info.GetLuck() != wantLuck+10 || info.GetNum() != 49 || info.GetLeftTimes() != 9 {
				t.Fatal("repeat date broke native global accounting", info)
			}
		})
	}
}

func TestAdminKingRecruitCharacterScheduleAndResources(t *testing.T) {
	f, actor, form := adminKingFixture(t)
	resources, err := f.s.AdminKingRecruitResources(f.ctx, actor)
	if err != nil || len(resources.Items) == 0 {
		t.Fatal("king resource selector", err)
	}
	worlds := map[int64]bool{}
	for _, item := range resources.Items {
		if item.Space < 0 || item.Space > 3 || item.SpaceLabel == "" || f.s.table("Hero", "id", item.ID)["heroClass"] != "0" || (item.OriginalQuality != 12 && item.OriginalQuality != 13 && item.OriginalQuality != 18 && item.OriginalQuality != 20 && item.OriginalQuality != 23) {
			t.Fatal("invalid selectable king resource", item.ID)
		}
		if item.Space > 0 {
			worlds[item.Space] = true
		}
	}
	if len(worlds) != 3 {
		t.Fatal("native world selector incomplete")
	}
	definition := adminKingDefinition(t, f, actor, form)
	slot := reusePublish(t, f, actor, definition, 1, 1, "character-day")
	bound := serverClockBindTest(t, f, f.id)
	if err = f.s.PrepareAdminCharacterActivitySchedules(bound, f.id); err != nil {
		t.Fatal(err)
	}
	if adminKingInfo(t, f, bound).GetOn() != 1 {
		t.Fatal("relative native king unavailable")
	}
	var issued db.AdminCharacterActivityInstance
	if err = db.GetIns().First(&issued, "player_id=? AND occurrence_id=?", f.id, slot.ID).Error; err != nil || !strings.Contains(issued.ConsumerKey, "_character_") {
		t.Fatal("relative pool identity missing", err)
	}
	var frozen db.KingRecruitActivitySnapshot
	if err = db.GetIns().First(&frozen, "pool_id=?", issued.ConsumerKey).Error; err != nil || !frozen.Frozen {
		t.Fatal("relative rules not frozen", err)
	}
	// Another serving realm cannot select this role's issued pool.
	foreign := &GameService{ServerID: 2, Tables: f.s.Tables}
	key, managed, err := foreign.adminScheduledKingRecruitPool(db.GetIns().WithContext(bound), LocalPlayNow(bound).UnixMilli())
	if err != nil || managed || key != "recruit_3" {
		t.Fatal("foreign realm consumed scheduled king", key, managed, err)
	}
	// Save-time canonicalization strips arbitrary submitted fixed-price payloads.
	form.Options.KingPoolSource = `{"pool":{"singleCost":"diamond,1"}}`
	safe, err := f.s.adminKingRecruitCanonicalForm(db.GetIns().WithContext(f.ctx), form)
	var source adminKingRecruitSource
	if err != nil || json.Unmarshal([]byte(safe.Options.KingPoolSource), &source) != nil || source.Pool.SingleCost != "diamond,300" || source.Rules.DailyLimit != 50 {
		t.Fatal("fixed native source accepted operator JSON spoof", err)
	}
}
