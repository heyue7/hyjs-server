package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// This fixture creates finite local configuration and pre-existing account state;
// it does not simulate combat or payment transactions.
func TestActivity77ApplyIntegrated(t *testing.T) {
	f, actor, _ := adminKingFixture(t)
	f.s.InstallGrowth74Resources()
	for _, init := range []func(context.Context) error{f.s.InitializeClientUpdates, f.s.InitializeKingRecruitWorlds, f.s.InitializeReRestaurant, f.s.InitializeAdminShopAvailability} {
		if err := init(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err := InitializeAdminNativeActivitySettings(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Save(&db.ClientUpdatePolicy{ID: 1, Maintenance: true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().AutoMigrate(&db.GameAccountRole{}); err != nil {
		t.Fatal(err)
	}
	if err := initializeGGOAccountWeekly(db.GetIns()); err != nil {
		t.Fatal(err)
	}
	var point db.P4CommerceRule
	if err := db.GetIns().Where("kind=?", "point").First(&point).Error; err != nil {
		t.Fatal(err)
	}
	resources := []string{"item_007", "item_013", "item_010", "item_005", "item_009", "item_008", "item_012", "item_006", "item_017", "frag_equip_013", "frag_equip_025", "frag_equip_037", "frag_equip_038", "frag_equip_039", "frag_equip_040"}
	var source p4CommerceRules
	for i, id := range resources {
		source.Products = append(source.Products, p4BaseProduct(fmt.Sprintf("fixture77_%d", i), id+",1", "point,150", int32(i+1), 1))
	}
	point.Rules = adminJSON(source)
	if err := db.GetIns().Save(&point).Error; err != nil {
		t.Fatal(err)
	}
	zone := time.FixedZone("UTC+8", 8*3600)
	now := time.Now().In(zone)
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, zone)
	if err := db.GetIns().Model(&db.AdminGameServer{}).Where("id=?", 1).Update("open_at", start.UnixMilli()).Error; err != nil {
		t.Fatal(err)
	}
	pairs, err := activity77PeriodPairs()
	if err != nil {
		t.Fatal(err)
	}
	calendar, err := f.s.AdminActivitySchedule(f.ctx, actor, 1)
	if err != nil {
		t.Fatal(err)
	}
	items := calendar.Items
	for i, pair := range pairs {
		def, e := f.s.AdminSaveActivityDefinition(f.ctx, actor, "", model.AdminActivityDefinitionSaveRequest{Name: pair.old.Name, Form: pair.old, DefaultDays: 7, Enabled: true})
		if e != nil {
			t.Fatal(e)
		}
		at := start
		end := start.AddDate(0, 0, 1)
		if i == 3 {
			at = start.AddDate(0, 0, 2)
			end = at.AddDate(0, 0, 7)
		}
		items = append(items, model.AdminActivityOccurrenceDTO{TemplateID: def.ID, TemplateVersion: def.Version, Timing: "calendar", StartAt: at.UnixMilli(), EndAt: end.UnixMilli(), ClaimEndAt: end.UnixMilli(), Enabled: true})
	}
	var pool db.GameplayPool
	if err = db.GetIns().First(&pool, "id=?", "recruit_3").Error; err != nil {
		t.Fatal(err)
	}
	var prizes []db.GameplayPoolReward
	if err = db.GetIns().Where("pool_id=?", "recruit_3").Order("id").Find(&prizes).Error; err != nil {
		t.Fatal(err)
	}
	oldForm, err := f.s.adminKingRecruitActivityForm(db.GetIns(), pool, prizes)
	if err != nil {
		t.Fatal(err)
	}
	oldForm.Name = "王者招募"
	oldForm.Source = "成长投放V12 · 原SAO/ALO/GGO轮转"
	oldForm.Alias = "成长投放V12"
	def, err := f.s.AdminSaveActivityDefinition(f.ctx, actor, "", model.AdminActivityDefinitionSaveRequest{Name: oldForm.Name, Form: oldForm, DefaultDays: 183, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	items = append(items, model.AdminActivityOccurrenceDTO{TemplateID: def.ID, TemplateVersion: def.Version, Timing: "calendar", StartAt: start.UnixMilli(), EndAt: start.AddDate(0, 0, 183).UnixMilli(), ClaimEndAt: start.AddDate(0, 0, 183).UnixMilli(), Enabled: true})
	calendar, err = f.s.AdminSaveActivitySchedule(f.ctx, actor, 1, model.AdminActivityScheduleSaveRequest{Version: calendar.Version, Items: items})
	if err != nil {
		t.Fatal(err)
	}
	var oldKey string
	for _, item := range calendar.Items {
		if item.TemplateID == def.ID {
			_, oldKey, _ = strings.Cut(item.ConsumerID, ":")
		}
	}
	if oldKey == "" {
		t.Fatal("old pool absent")
	}
	if err = db.GetIns().Model(&db.KingRecruitActivitySnapshot{}).Where("pool_id=?", oldKey).Update("frozen", true).Error; err != nil {
		t.Fatal(err)
	}
	session := []byte("activity77-test1")
	if err = db.GetIns().Model(&db.Profile{}).Where("id=?", f.id).Update("session", session).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Save(&db.KingRecruitSelection{PlayerID: f.id, PoolID: oldKey}).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Save(&db.KingRecruitState{PlayerID: f.id, Day: p1Day(now), Draws: 9, Luck: 70}).Error; err != nil {
		t.Fatal(err)
	}
	if err = updateProfile(f.ctx, f.id, func(_ *pb.Player, g *model.GameState) error { v := int32(70); g.KingRecruitSharedLuck = &v; return nil }); err != nil {
		t.Fatal(err)
	}
	var before db.Profile
	if err = db.GetIns().First(&before, f.id).Error; err != nil {
		t.Fatal(err)
	}
	var clock db.ServerGameClock
	if err = db.GetIns().First(&clock, "server_id=?", 1).Error; err != nil {
		t.Fatal(err)
	}
	if err = f.s.ApplyActivity77(f.ctx); err != nil {
		t.Fatal("apply", err)
	}
	if err = f.s.ApplyActivity77(f.ctx); err != nil {
		t.Fatal("retry", err)
	}
	var after db.Profile
	if err = db.GetIns().First(&after, f.id).Error; err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before.Player, after.Player) {
		t.Fatal("maintenance changed player assets")
	}
	var policy db.ClientUpdatePolicy
	if err = db.GetIns().First(&policy, 1).Error; err != nil || !policy.Maintenance {
		t.Fatal("maintenance", err)
	}
	var clockAfter db.ServerGameClock
	if err = db.GetIns().First(&clockAfter, "server_id=?", 1).Error; err != nil || clockAfter.OffsetDays != clock.OffsetDays {
		t.Fatal("clock changed", err)
	}
	var oldSnapshot db.KingRecruitActivitySnapshot
	if err = db.GetIns().First(&oldSnapshot, "pool_id=?", oldKey).Error; err != nil || !oldSnapshot.Frozen {
		t.Fatal("old frozen pool lost", err)
	}
	bound := serverClockBindTest(t, f, f.id)
	if _, err = f.s.warriorRecruit(bound, f.id, p2Recruit(1, 0), 778801); !errors.Is(err, constUtil.ErrKingRecruitActivityClosed) {
		t.Fatal("stale selected pool accepted", err)
	}
	worlds, err := f.s.KingRecruitWorlds(bound, f.id, session, "", "")
	if err != nil || len(worlds.Worlds) != 3 || worlds.SelectedPoolID == oldKey {
		t.Fatal("D1 selection", worlds, err)
	}
	info := adminKingInfo(t, f, bound)
	if info.GetLuck() != 70 || info.GetNum() != 41 {
		t.Fatal("shared luck or daily draw count", info)
	}
	d4 := start.AddDate(0, 0, 3).Add(time.Hour)
	var candidates []kingRecruitAvailablePool
	err = db.GetIns().WithContext(bound).Transaction(func(tx *gorm.DB) error {
		var e error
		candidates, _, e = f.s.kingRecruitAvailablePools(tx, d4.UnixMilli())
		return e
	})
	if err != nil || len(candidates) != 1 || candidates[0].World != 1 {
		t.Fatal("D4 SAO rotation", candidates, err)
	}
	var restored db.P4WelfareRule
	if err = db.GetIns().First(&restored, "id=?", "p4:meal:admin_re1051_schedule_1").Error; err != nil || !restored.Enabled || restored.EndAt <= now.AddDate(9, 0, 0).UnixMilli() {
		t.Fatal("restaurant", err)
	}
	vip, _, _, err := f.s.adminNativeVIPPolicy(db.GetIns(), false)
	if err != nil {
		t.Fatal(err)
	}
	tier, _ := adminNativeVIPTierFor(vip, 2)
	if tier.DailyRewards[0].Amount != 3000+constUtil.Activity77MemberDayCoin {
		t.Fatal("vip policy", tier)
	}
	if err = db.GetIns().First(&point, "id=?", point.ID).Error; err != nil {
		t.Fatal(err)
	}
	var updated p4CommerceRules
	if json.Unmarshal(point.Rules, &updated) != nil || updated.Products[7].MinTrialStars != 24 {
		t.Fatal("point shop")
	}
	score, err := f.s.AdminShop(f.ctx, "score")
	if err != nil {
		t.Fatal(err)
	}
	var weekly int
	for _, p := range score.Items {
		if p.AccountWeeklyLimit == 1 {
			weekly++
		}
	}
	if weekly < 5 {
		t.Fatal("score weekly items", weekly)
	}
	finalCalendar, err := f.s.AdminActivitySchedule(f.ctx, actor, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, pair := range pairs {
		found := false
		for _, occ := range finalCalendar.Items {
			if !activity77FormMatch(occ.Form, pair.next) {
				continue
			}
			_, key, _ := strings.Cut(occ.ConsumerID, ":")
			var period db.PeriodInstance
			if err = db.GetIns().First(&period, "id=?", key).Error; err != nil {
				t.Fatal(err)
			}
			expected, e := f.s.activity77PeriodTiers(pair.next, key, true)
			if e != nil || !reflect.DeepEqual(p1PeriodMilestones(period), expected) {
				t.Fatal("period consumer not updated", key, e)
			}
			found = true
		}
		if !found {
			t.Fatal("period occurrence not updated", pair.next.Mode)
		}
	}
	for _, pair := range pairs {
		var defs []db.AdminActivityDefinition
		if err = db.GetIns().Find(&defs).Error; err != nil {
			t.Fatal(err)
		}
		found := false
		for _, d := range defs {
			var form model.AdminActivityForm
			if json.Unmarshal(d.FormJSON, &form) == nil && activity77FormMatch(form, pair.next) {
				found = true
			}
		}
		if !found {
			t.Fatal("period template not updated", pair.next.Mode)
		}
	}
}
