package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestAdminTreasureTemplate63UpdatesPublishedQuotesPreservesPaidReceipt(t *testing.T) {
	f, actor := adminModesFixture(t)
	f.ctx = context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	if err := f.s.InitializeAdminGameServers(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeAdminActivitySchedules(f.ctx); err != nil {
		t.Fatal(err)
	}
	at := time.Now()
	form := reOperationsWelfareForms()[0]
	template, err := f.s.AdminSaveActivityDefinition(f.ctx, actor, "", model.AdminActivityDefinitionSaveRequest{Name: form.Name, Form: form, DefaultDays: 7, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	calendar, err := f.s.AdminActivitySchedule(f.ctx, actor, 1)
	if err != nil {
		t.Fatal(err)
	}
	published, err := f.s.AdminSaveActivitySchedule(f.ctx, actor, 1, model.AdminActivityScheduleSaveRequest{Version: calendar.Version, Items: []model.AdminActivityOccurrenceDTO{{TemplateID: template.ID, TemplateVersion: template.Version, StartAt: at.Add(-time.Hour).UnixMilli(), EndAt: at.AddDate(0, 0, 7).UnixMilli(), ClaimEndAt: at.AddDate(0, 0, 7).UnixMilli(), Enabled: true}}})
	if err != nil || len(published.Items) != 1 {
		t.Fatal("publish", err)
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error { p.Diamond = proto.Int64(10000); return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p4Welfare(f.ctx, f.id, &pb.CSFortuneCatInfo{}, 0, at); err != nil {
		t.Fatal("first info", err)
	}
	req := &pb.CSFortuneCatReward{Type: proto.Int32(0)}
	first, err := f.s.p4Welfare(f.ctx, f.id, req, 63001, at)
	if err != nil {
		t.Fatal("first paid tier", err)
	}
	wantFirst, _ := proto.Marshal(first[0])
	_, key, _ := strings.Cut(published.Items[0].ConsumerID, ":")
	var before db.P4WelfareRule
	if err = db.GetIns().First(&before, "id=?", key).Error; err != nil {
		t.Fatal(err)
	}
	form.Milestones[1].Rewards[0].Amount = 88888
	updated, err := f.s.AdminSaveActivityDefinition(f.ctx, actor, template.ID, model.AdminActivityDefinitionSaveRequest{Name: template.Name, Form: form, DefaultDays: 7, Enabled: true, Version: template.Version})
	if err != nil {
		t.Fatal("edit live template", err)
	}
	var after db.P4WelfareRule
	if err = db.GetIns().First(&after, "id=?", key).Error; err != nil {
		t.Fatal(err)
	}
	if string(before.Frozen) != string(after.Frozen) || before.StartAt != after.StartAt || before.EndAt != after.EndAt || before.Enabled != after.Enabled {
		t.Fatal("template edit changed frozen history or calendar")
	}
	var occurrence db.AdminActivityOccurrence
	if err = db.GetIns().First(&occurrence, "id=?", published.Items[0].ID).Error; err != nil {
		t.Fatal(err)
	}
	var display model.AdminActivityForm
	if json.Unmarshal(occurrence.FormJSON, &display) != nil || display.Milestones[1].Rewards[0].Amount != 88888 || occurrence.TemplateVersion != updated.Version {
		t.Fatal("published admin preview stale")
	}
	info, err := f.s.p4Welfare(f.ctx, f.id, &pb.CSFortuneCatInfo{}, 0, at)
	if err != nil || info[0].(*pb.SCFortuneCatInfo).GetInfo().GetTodayLeft() != 4 || catQuote63(t, info[0].(*pb.SCFortuneCatInfo).GetInfo()) != 88888 {
		t.Fatal("next Info did not bind new tier and preserve uses", err)
	}
	second, err := f.s.p4Welfare(f.ctx, f.id, req, 63002, at)
	if err != nil || second[0].(*pb.SCFortuneCatReward).GetGet().GetSimps()[0].GetNum() != 88888 {
		t.Fatal("new quote settlement", err)
	}
	f.restart(t)
	replay, err := f.s.p4Welfare(f.ctx, f.id, req, 63001, at)
	if err != nil {
		t.Fatal("old receipt", err)
	}
	gotFirst, _ := proto.Marshal(replay[0])
	if string(gotFirst) != string(wantFirst) {
		t.Fatal("template update rewrote prior paid result")
	}
	current, err := f.s.AdminActivitySchedule(f.ctx, actor, 1)
	if err != nil || current.Version == published.Version {
		t.Fatal("calendar version did not change", err)
	}
}
