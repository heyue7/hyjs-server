package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

func TestShopRefresh75ActualWeekAndVirtualDay(t *testing.T) {
	f, actor := reuseFixture(t, false)
	if err := f.s.InitializeAdminShopAvailability(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.AdminShopAvailability{}).Where("shop_key=?", "point").Updates(map[string]any{"unlimited": false}).Error; err != nil {
		t.Fatal(err)
	}
	// Shift the trusted game calendar; its daily reset must not use host midnight.
	for i := 0; i < 2; i++ {
		reuseClockDay(t, f, actor)
	}
	bound := serverClockBindTest(t, f, f.id)
	now := LocalPlayNow(bound)
	start, end := now.Add(-2*24*time.Hour).UnixMilli(), now.Add(5*24*time.Hour).UnixMilli()
	key := "shop_schedule_point_refresh75"
	products := []p4Product{p4BaseProduct("red-ten", "frag_equip_041,10", "point,5000", 1, 1)}
	products[0].DailyLimit, products[0].TotalLimit = 1, 1
	rule := db.P4CommerceRule{ID: key, Kind: "point", ShopType: 5, Enabled: true, StartAt: start, EndAt: end, ClaimEndAt: end, Rules: adminJSON(p4CommerceRules{Products: products}), Source: "isolated weekly point shop"}
	if err := db.GetIns().Create(&rule).Error; err != nil {
		t.Fatal(err)
	}
	form := model.AdminActivityForm{Mode: "shop-window", TemplateID: "shop-window-original", RepeatPolicy: "repeat", Options: model.AdminActivityOptions{ShopKey: "point"}}
	slot := db.AdminActivityOccurrence{ID: "refresh75-week", ServerID: f.s.OriginalServerID(), TemplateID: "refresh75-template", ConsumerKey: key, ConsumerID: "commerce:" + key, Mode: "shop-window", Timing: "calendar", StartAt: start, EndAt: end, ClaimEndAt: end, Enabled: true, FormJSON: adminJSON(form), Revision: 1}
	if err := db.GetIns().Create(&slot).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Save(&db.AdminActivityScheduleRevision{ServerID: f.s.OriginalServerID(), Revision: 1}).Error; err != nil {
		t.Fatal(err)
	}
	var owner db.Profile
	if err := db.GetIns().First(&owner, f.id).Error; err != nil {
		t.Fatal(err)
	}
	ctx := context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	view, err := f.s.ShopRefresh75Status(ctx, f.id, owner.Session)
	if err != nil || len(view.Menus) != 4 {
		t.Fatal(view, err)
	}
	if delta := view.ServerTime - now.UnixMilli(); delta < 0 || delta > 10000 {
		t.Fatal("status used wall clock", view.ServerTime, now.UnixMilli())
	}
	zone := time.FixedZone("Asia/Shanghai", 28800)
	d := now.In(zone)
	wantDay := time.Date(d.Year(), d.Month(), d.Day()+1, 0, 0, 0, 0, zone).UnixMilli()
	for _, menu := range view.Menus[:3] {
		if !menu.Available || menu.Period != "day" || menu.ResetAt == nil || *menu.ResetAt != wantDay {
			t.Fatal("daily menu did not follow virtual next midnight", menu)
		}
	}
	weekly := view.Menus[3]
	if !weekly.Available || weekly.ResetAt == nil || *weekly.ResetAt != end {
		t.Fatal("weekly reset invented a weekday instead of real window end", weekly, end)
	}
	if err := db.GetIns().Model(&db.AdminActivityOccurrence{}).Where("id=?", slot.ID).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	closed, err := f.s.ShopRefresh75Status(ctx, f.id, owner.Session)
	if err != nil || closed.Menus[3].Available || closed.Menus[3].ResetAt != nil {
		t.Fatal("disabled weekly window retained a fake countdown", closed, err)
	}
	if _, err = f.s.ShopRefresh75Status(ctx, f.id, []byte("foreign-session!")); err == nil {
		t.Fatal("foreign session read refresh status")
	}
	if err = db.GetIns().Model(&db.Profile{}).Where("id=?", owner.Id).Update("server_id", 2).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.ShopRefresh75Status(ctx, f.id, owner.Session); err == nil {
		t.Fatal("foreign realm role read local shop schedule")
	}
	raw, err := json.Marshal(closed.Menus[3])
	if err != nil || string(raw) != `{"group":4,"name":"红装","period":"week","available":false,"resetAt":null}` {
		t.Fatal("closed countdown wire contract", string(raw), err)
	}
}
