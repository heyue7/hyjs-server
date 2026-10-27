package service

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func TestActivity77RestaurantPreservesReceiptAndRetries(t *testing.T) {
	f, actor := reuseFixture(t, false)
	if err := f.s.InitializeReRestaurant(f.ctx); err != nil {
		t.Fatal(err)
	}
	key := "p4:meal:admin_re1051_schedule_1"
	var original db.P4WelfareRule
	if err := db.GetIns().First(&original, "id=?", key).Error; err != nil {
		t.Fatal(err)
	}
	frozen := original
	original.Frozen = adminJSON(frozen)
	if err := db.GetIns().Save(&original).Error; err != nil {
		t.Fatal(err)
	}
	payload := []byte(`{"mealClaims":{"2026-10-27:0":true}}`)
	state := db.P4WelfareState{PlayerID: f.id, RuleID: key, Snapshot: adminJSON(frozen), Payload: payload}
	if err := db.GetIns().Create(&state).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.activity77Restaurant(f.ctx, actor); err != nil {
		t.Fatal(err)
	}
	var got db.P4WelfareRule
	if err := db.GetIns().First(&got, "id=?", key).Error; err != nil {
		t.Fatal(err)
	}
	if got.EndAt <= time.Now().AddDate(9, 0, 0).UnixMilli() || got.StartAt != original.StartAt || !got.Enabled {
		t.Fatal("invalid extended rule", got)
	}
	if err := db.GetIns().First(&state, "player_id=? AND rule_id=?", f.id, key).Error; err != nil {
		t.Fatal(err)
	}
	var snapshot db.P4WelfareRule
	if err := json.Unmarshal(state.Snapshot, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.EndAt != got.EndAt || !bytes.Equal(state.Payload, payload) {
		t.Fatal("lost frozen receipt", snapshot, string(state.Payload))
	}
	if err := f.s.activity77Restaurant(f.ctx, actor); err != nil {
		t.Fatal(err)
	}
	var after db.P4WelfareRule
	if err := db.GetIns().First(&after, "id=?", key).Error; err != nil || after.EndAt != got.EndAt {
		t.Fatal("retry changed window", err)
	}
}

func TestActivity77KingScheduleAndRetry(t *testing.T) {
	f, actor, _ := adminKingFixture(t)
	if err := f.s.activity77King(f.ctx, actor); err != nil {
		t.Fatal(err)
	}
	calendar, err := f.s.AdminActivitySchedule(f.ctx, actor, f.s.OriginalServerID())
	if err != nil {
		t.Fatal(err)
	}
	var realm db.AdminGameServer
	if err = db.GetIns().First(&realm, f.s.OriginalServerID()).Error; err != nil {
		t.Fatal(err)
	}
	zone := time.FixedZone("UTC+8", 8*3600)
	opened := time.UnixMilli(realm.OpenAt).In(zone)
	start := time.Date(opened.Year(), opened.Month(), opened.Day(), 0, 0, 0, 0, zone)
	counts := map[int]int{}
	for _, item := range calendar.Items {
		if item.Enabled && item.Mode == "king-recruit" {
			day := int(time.UnixMilli(item.StartAt).Sub(start) / (24 * time.Hour))
			counts[day]++
			if item.EndAt-item.StartAt != 3*24*60*60*1000 {
				t.Fatal("bad duration", item)
			}
		}
	}
	if counts[0] != 3 || counts[3] != 1 || counts[6] != 1 || counts[9] != 1 || counts[180] != 1 || len(counts) != 61 {
		t.Fatal("bad rotation", counts)
	}
	if err = f.s.activity77King(f.ctx, actor); err != nil {
		t.Fatal(err)
	}
	after, err := f.s.AdminActivitySchedule(f.ctx, actor, f.s.OriginalServerID())
	if err != nil || len(after.Items) != len(calendar.Items) {
		t.Fatal("retry duplicated slots", err, len(after.Items), len(calendar.Items))
	}
}

func TestActivity77RestaurantNativeClaimAcrossRepair(t *testing.T) {
	f, actor := reuseFixture(t, false)
	if err := f.s.InitializeLocalPlayActivities(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeReRestaurant(f.ctx); err != nil {
		t.Fatal(err)
	}
	ctx := serverClockBindTest(t, f, f.id)
	at := p4At(3, 11).Add(10 * time.Minute)
	if _, err := f.s.p4Welfare(ctx, f.id, &pb.CSStaminaGet{}, 7001, at); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.AdminActivityOccurrence{}).Where("id=?", "restore_re1051_meal_1").Updates(map[string]any{"enabled": false, "archived": true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P4WelfareRule{}).Where("id=?", "p4:meal:admin_re1051_schedule_1").Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.activity77Restaurant(f.ctx, actor); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p4Welfare(ctx, f.id, &pb.CSStaminaGet{}, 7002, at); err == nil {
		t.Fatal("same window granted twice")
	}
	if _, err := f.s.p4Welfare(ctx, f.id, &pb.CSStaminaGet{}, 7003, at.AddDate(0, 0, 1)); err != nil {
		t.Fatal("next day claim", err)
	}
}
