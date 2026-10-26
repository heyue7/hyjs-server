package service

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// A real published character-day plan is left lazy by an authenticated AskFight.
// The paired roles use genuine frozen parties and admission. Their next native
// welfare request still materializes the unrelated personal schedule.
func TestP13AskFightDefersRealActivityScheduleUntilConsumer(t *testing.T) {
	f, id, enemy, _, match := p13UFPlayerBattle172ActualSetup(t)
	f.id = id
	for _, initialize := range []func() error{func() error { return AdminInitialize(f.ctx, "") }, func() error { return InitializeAdminOperations(f.ctx) }, func() error { return InitializeAdminActivityModes(f.ctx) }} {
		if err := initialize(); err != nil {
			t.Fatal(err)
		}
	}
	user := db.AdminUser{Username: "ask-activity-" + p6Digest([]byte(t.Name()))[:24], PasswordHash: "unused-fixture", Role: constUtil.AdminRoleOperator, Enabled: true}
	if err := db.GetIns().Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	operator := &model.AdminPrincipal{UserID: user.ID, Username: user.Username, Role: user.Role}
	for _, pid := range []int64{id, enemy} {
		var profile db.Profile
		if err := db.GetIns().First(&profile, pid).Error; err != nil {
			t.Fatal(err)
		}
		t.Cleanup(f.s.RegisterP10Client(f.ctx, pid, profile.Session, func(proto.Message) error { return nil }))
		if _, err := f.s.SyncP13UnionFightField(f.ctx, pid, false); err != nil {
			t.Fatal(err)
		}
	}
	f.ctx = context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	// Match adminModesFixture's real P4 consumer prerequisites without opening
	// another fixture/database or replacing the genuine frozen battle parties.
	// P1 is already initialized by p13GuildSetup.
	for _, initialize := range []func(context.Context) error{f.s.InitializeGameplayPools, f.s.InitializeShop, f.s.InitializeP4, f.s.InitializeTurntable, f.s.InitializePlayerOpeningCalendar, f.s.InitializeAdminGameServers, f.s.InitializeAdminActivitySchedules, f.s.InitializeServerGameClock} {
		if err := initialize(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	form := adminModeForm("login-rewards")
	form.Milestones = []model.AdminActivityMilestone{{Goal: 1, Rewards: []model.AdminResourceAmount{{ResourceID: "coin", Amount: 7}}}}
	calendar, err := f.s.AdminActivitySchedule(f.ctx, operator, 1)
	if err != nil {
		t.Fatal(err)
	}
	definition, err := f.s.AdminSaveActivityDefinition(f.ctx, operator, "", model.AdminActivityDefinitionSaveRequest{Name: form.Mode, Form: form, DefaultDays: 7, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.s.AdminSaveActivitySchedule(f.ctx, operator, 1, model.AdminActivityScheduleSaveRequest{Version: calendar.Version, Items: []model.AdminActivityOccurrenceDTO{{TemplateID: definition.ID, TemplateVersion: definition.Version, Mode: form.Mode, Timing: "character-day", StartDay: 1, DurationDays: 7, StartAt: now.UnixMilli(), EndAt: now.AddDate(0, 0, 7).UnixMilli(), ClaimEndAt: now.AddDate(0, 0, 7).UnixMilli(), Enabled: true}}})
	if err != nil {
		t.Fatal(err)
	}
	count := func() int64 { return p1Count(t, &db.AdminCharacterActivityInstance{}, "player_id=?", f.id) }
	if count() != 0 {
		t.Fatal("publication eagerly materialized role")
	}
	for _, transport := range []struct {
		opcode int32
		body   proto.Message
	}{
		{30024, &pb.CSCurrentPosition{}},
		{30053, &pb.CSLoginScene{}},
		{80044, &pb.CSUFInit{}},
	} {
		messages := f.s.Dispatch(f.ctx, scene49Packet(t, f, f.id, transport.opcode, transport.body), "127.0.0.1")
		for _, message := range messages {
			if failure, ok := message.(*pb.ErrorMessage); ok {
				t.Fatal("authenticated frozen field transport", failure.GetMsg())
			}
		}
		if count() != 0 {
			t.Fatal("war transport materialized unrelated personal activity", transport.body)
		}
	}
	reply := f.s.Dispatch(f.ctx, scene49Packet(t, f, f.id, 80069, &pb.CSUFPlayerAskFight{TargetGuid: proto.Int64(enemy)}), "127.0.0.1")
	var attempt db.P13UnionFightPlayerBattleAttempt
	if err := db.GetIns().First(&attempt, "match_id=? AND player_id=? AND target_id=?", match.ID, id, enemy).Error; err != nil {
		t.Fatal("authenticated real AskFight was not captured", err, reply)
	}
	if attempt.Status != constUtil.P13UFPlayerBattlePending {
		t.Fatal("expected real admitted pending battle", attempt.Status)
	}
	if count() != 0 {
		t.Fatal("AskFight materialized unrelated personal activity")
	}
	f.s.Dispatch(f.ctx, scene49Packet(t, f, f.id, 57145, &pb.CSGetLoginReward{}), "127.0.0.1")
	if count() != 1 {
		t.Fatal("native activity consumer failed to materialize published plan", count())
	}
}
