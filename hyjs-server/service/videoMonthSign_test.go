package service

import (
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"testing"
	"time"
)

func TestVideoMonthSignCalendarsSeedOnceAndOriginalList(t *testing.T) {
	f, _ := adminModesFixture(t)
	if err := f.s.InitializeMonthSign(f.ctx); err != nil {
		t.Fatal(err)
	}
	var defaults []db.MonthSignRule
	if err := db.GetIns().Order("day").Find(&defaults).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.SeedUserMonthSignCalendars(f.ctx); err != nil {
		t.Fatal(err)
	}
	for _, month := range []string{"2026-10", "2026-11"} {
		rows, ok, err := monthSignCalendarRules(db.GetIns(), month)
		if err != nil || !ok || len(rows) != 31 {
			t.Fatal(month, err)
		}
		for i := 20; i < 31; i++ {
			if rows[i] != defaults[i] {
				t.Fatal("hidden reward overwritten", i)
			}
		}
		at, _ := time.ParseInLocation("2006-01-02", month+"-01", time.FixedZone("UTC+8", 8*3600))
		out, err := f.s.monthSignAt(f.ctx, f.id, &pb.CSRewardMonthSignList{}, at)
		if err != nil {
			t.Fatal(err)
		}
		list := out[0].(*pb.SCRewardMonthSignList)
		if list.GetList()[0].GetRewardId() != "coin,50000" {
			t.Fatal("reference day1")
		}
		if month == "2026-10" && (len(list.List) != 31 || list.List[1].GetRewardId() != "frag_equip_018,16" || list.List[4].GetDoubleNeedVipLevel() != 1) {
			t.Fatal("current reference")
		}
		if month == "2026-11" && (len(list.List) != 30 || list.List[2].GetRewardId() != "item_022,5" || list.List[19].GetDoubleNeedVipLevel() != 8) {
			t.Fatal("next reference")
		}
	}
	rows, _, _ := monthSignCalendarRules(db.GetIns(), "2026-11")
	rows[0].Reward = "coin,123"
	if err := db.GetIns().Model(&db.MonthSignCalendar{}).Where("month_key=?", "2026-11").Update("rules", adminJSON(rows)).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.SeedUserMonthSignCalendars(f.ctx); err != nil {
		t.Fatal(err)
	}
	rows, _, _ = monthSignCalendarRules(db.GetIns(), "2026-11")
	if rows[0].Reward != "coin,123" {
		t.Fatal("seed overwrote operator")
	}
	var count int64
	db.GetIns().Model(&db.MonthSignState{}).Where("player_id=?", f.id).Count(&count)
	if count != 0 {
		t.Fatal("list created claim")
	}
}
