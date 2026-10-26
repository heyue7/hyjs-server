package service

import (
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"strings"
	"testing"
	"time"
)

func TestTerminalSchedule74(t *testing.T) {
	loc := time.FixedZone("Asia/Shanghai", 28800)
	cases := []struct {
		r   model.TerminalWeeklyRule
		at  string
		due bool
	}{
		{model.TerminalWeeklyRule{Kind: "monthly", Day: 15, Time: "12:30", Enabled: true}, "2026-11-15 12:29", false},
		{model.TerminalWeeklyRule{Kind: "monthly", Day: 15, Time: "12:30", Enabled: true}, "2026-11-15 12:30", true},
		{model.TerminalWeeklyRule{Kind: "monthly", Day: 31, Enabled: true}, "2026-11-30 23:59", false},
		{model.TerminalWeeklyRule{Kind: "once", Date: "2026-11-16", Time: "08:00", Enabled: true}, "2026-11-16 08:01", true},
		{model.TerminalWeeklyRule{Kind: "once", Date: "2026-11-16", Time: "08:00", Enabled: true}, "2026-11-17 08:01", false},
		{model.TerminalWeeklyRule{Weekday: 1, Enabled: true}, "2026-11-16 00:00", true},
	}
	for _, c := range cases {
		at, e := time.ParseInLocation("2006-01-02 15:04", c.at, loc)
		if e != nil {
			t.Fatal(e)
		}
		_, due := terminalScheduleDue(c.r, at)
		if due != c.due {
			t.Fatal(c, due)
		}
	}
	for _, r := range []model.TerminalWeeklyRule{{Kind: "once", Date: "2026-02-30"}, {Kind: "weekly", Weekday: 8}, {Kind: "monthly", Day: 1, Time: "24:00"}} {
		if TerminalScheduleValid(r) {
			t.Fatal("invalid schedule accepted", r)
		}
	}
}
func TestTerminalManualReason74(t *testing.T) {
	f, id, ctx, a := terminalFixture(t)
	_ = id
	_ = f
	cfg, e := AdminTerminalSettings(ctx, a)
	if e != nil {
		t.Fatal(e)
	}
	// A manual award must also work with an empty random-reason catalogue.
	if e = db.GetIns().Model(&db.LocalWalletReason{}).Where("1=1").Update("enabled", false).Error; e != nil {
		t.Fatal(e)
	}
	req := model.TerminalDistributeRequest{RequestID: uuid.NewString(), Points: 128, Reason: "开服七日测试补给", Version: cfg.Version}
	out, e := AdminTerminalDistribute(ctx, a, req)
	if e != nil {
		t.Fatal(e)
	}
	var rows []db.TerminalRecipient
	if e = db.GetIns().Where("batch_id=?", out.BatchID).Find(&rows).Error; e != nil {
		t.Fatal(e)
	}
	if len(rows) == 0 {
		t.Fatal("no recipients")
	}
	for _, r := range rows {
		if r.Label != req.Reason || r.ReasonID != "" {
			t.Fatal("manual reason changed", r)
		}
	}
	replay, e := AdminTerminalDistribute(ctx, a, req)
	if e != nil || replay.BatchID != out.BatchID || !replay.AlreadyApplied {
		t.Fatal("manual replay", replay, e)
	}
}

func TestGrowth74Boxes(t *testing.T) {
	s, ctx := newP2Test(t)
	if e := s.LoadP5Tables("../../analysis/tables"); e != nil {
		t.Fatal(e)
	}
	if e := s.LoadP9Tables("../../analysis/tables"); e != nil {
		t.Fatal(e)
	}
	if e := s.InitializeP5(ctx); e != nil {
		t.Fatal(e)
	}
	if e := s.InitializeP9Pets(ctx); e != nil {
		t.Fatal(e)
	}
	s.InstallGrowth74Resources()
	counts := len(s.Tables["ItemUse"])
	s.InstallGrowth74Resources()
	if len(s.Tables["ItemUse"]) != counts {
		t.Fatal("duplicate install")
	}
	id := dailyCopyTestPlayer(t, ctx, "growth74-boxes", 80)
	var resources struct{ Items []map[string]string }
	if e := json.Unmarshal(growth74ResourcesJSON, &resources); e != nil {
		t.Fatal(e)
	}
	var stamp int64 = 1000
	for _, r := range resources.Items {
		p2Funding(t, s, ctx, id, r["itemId"]+",1")
		req := &pb.CSItemUse{CostItem: p2Item(r["itemId"], 1), GetItemIndex: proto.Int32(1)}
		first, e := s.itemUse(ctx, id, req, stamp)
		if e != nil {
			t.Fatalf("%s %s: %v", r["itemId"], r["name"], e)
		}
		replay, e := s.itemUse(ctx, id, req, stamp)
		if e != nil || !proto.Equal(first[0], replay[0]) {
			t.Fatal("box replay", r["itemId"], e)
		}
		stamp++
	}
	for _, key := range []string{"item_067", "item_339", "item_340"} {
		p2Funding(t, s, ctx, id, key+",1")
		out, e := s.itemUse(ctx, id, &pb.CSItemUse{CostItem: p2Item(key, 1), GetItemIndex: proto.Int32(1)}, stamp)
		if e != nil {
			t.Fatal(key, e)
		}
		if key == "item_340" {
			got := out[0].(*pb.SCItemUse).GetGet().GetEquips()
			if len(got) != 1 || !strings.HasPrefix(got[0].GetEquipId(), "equip_04") {
				t.Fatal("glory random content", got)
			}
		}
		stamp++
	}
}
func TestServerControl74(t *testing.T) {
	f, _, ctx, operator := terminalFixture(t)
	s := f.s
	if e := s.InitializeClientUpdates(ctx); e != nil {
		t.Fatal(e)
	}
	a := adminWalletTestActor(t, f, constUtil.AdminRoleAdmin)
	policy, e := clientUpdatePolicy(ctx)
	if e != nil {
		t.Fatal(e)
	}
	req := model.AdminClientUpdateActionRequest{Action: "server_stop", Version: clientUpdateVersion(policy), RequestID: uuid.NewString(), Reason: "隔离维护验证"}
	if _, e = s.AdminClientUpdateAction(ctx, operator, req); !errors.Is(e, constUtil.ErrAdminPermission) {
		t.Fatal("operator toggled server", e)
	}
	out, e := s.AdminClientUpdateAction(ctx, a, req)
	if e != nil || !out.Policy.Maintenance {
		t.Fatal(out, e)
	}
	replay, e := s.AdminClientUpdateAction(ctx, a, req)
	if e != nil || replay.Version != out.Version {
		t.Fatal("control replay", replay, e)
	}

	req.Action = "server_start"
	req.RequestID = uuid.NewString()
	req.Version = out.Version
	opened, e := s.AdminClientUpdateAction(ctx, a, req)
	if e != nil || opened.Policy.Maintenance || opened.Policy.NoticeBatch != policy.NoticeBatch || opened.Policy.ReleaseID != policy.ReleaseID {
		t.Fatal("start changed release", opened, e)
	}
}

func TestTerminalWeeklyEdit74(t *testing.T) {
	f, _, ctx, a := terminalFixture(t)
	cfg, e := AdminTerminalSettings(ctx, a)
	if e != nil {
		t.Fatal(e)
	}
	cfg.Weekly = []model.TerminalWeeklyRule{{ID: "stable-week", Kind: "weekly", Weekday: 1, Time: "08:00", Points: 648, Enabled: true}}
	cfg, e = AdminSaveTerminalSettings(ctx, a, *cfg)
	if e != nil {
		t.Fatal(e)
	}
	now := time.Date(2026, 11, 16, 8, 0, 0, 0, time.FixedZone("Asia/Shanghai", 28800))
	if e = f.s.processTerminalDistributionsAt(ctx, now); e != nil {
		t.Fatal(e)
	}
	cfg.Weekly[0].Weekday = 2
	cfg.Weekly[0].Points = 3000
	cfg, e = AdminSaveTerminalSettings(ctx, a, *cfg)
	if e != nil {
		t.Fatal(e)
	}
	if e = f.s.processTerminalDistributionsAt(ctx, now.AddDate(0, 0, 1)); e != nil {
		t.Fatal(e)
	}
	var n int64
	if e = db.GetIns().Model(&db.TerminalBatch{}).Where("request_id LIKE ?", "weekly-%stable-week").Count(&n).Error; e != nil || n != 1 {
		t.Fatal("weekday edit repeated weekly award", n, e)
	}
	if e = f.s.processTerminalDistributionsAt(ctx, now.AddDate(0, 0, 8)); e != nil {
		t.Fatal(e)
	}
	if e = db.GetIns().Model(&db.TerminalBatch{}).Where("request_id LIKE ?", "weekly-%stable-week").Count(&n).Error; e != nil || n != 2 {
		t.Fatal("next week did not renew", n, e)
	}
}

func TestServerGameClockSettledPKRewind74(t *testing.T) {
	terminalFixture(t)
	d := db.GetIns()
	if e := d.AutoMigrate(&db.P12FieldPVPReceipt{}); e != nil {
		t.Fatal(e)
	}
	now := time.Now()
	row := db.P12FieldPVPReceipt{FightID: "rewind-lease", P6RecordID: "rewind-record", CreatedAt: now.UnixMilli(), BusyUntil: now.UnixMilli() + 6000, Response: []byte{}, Terminal: []byte{}, Replay: []byte{}, MV: []byte{}}
	if e := d.Create(&row).Error; e != nil {
		t.Fatal(e)
	}
	if e := serverGameClockPendingBattles(d, now, 0); e == nil {
		t.Fatal("live replay lease accepted")
	}
	if e := serverGameClockPendingBattles(d, now, -4); e == nil {
		t.Fatal("live real-time PK lease was bypassed by a virtual rewind")
	}
	if e := serverGameClockPendingBattles(d, now.Add(7*time.Second), -4); e != nil {
		t.Fatal("expired real-time PK lease blocked virtual rewind", e)
	}
}
