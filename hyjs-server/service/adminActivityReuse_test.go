package service

import (
	"context"
	"fmt"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"strings"
	"testing"
	"time"
)

func reuseFixture(t *testing.T, opening bool) (*p1Fixture, *model.AdminPrincipal) {
	t.Helper()
	f, actor := adminModesFixture(t)
	f.ctx = context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	if opening {
		loadOpenServerTaskFixture(t, f.s)
		if err := f.s.InitializeOpenServerActivity(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	for _, init := range []func(context.Context) error{f.s.InitializePlayerOpeningCalendar, f.s.InitializeAdminGameServers, f.s.InitializeAdminActivitySchedules, f.s.InitializeServerGameClock} {
		if err := init(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.GetIns().Model(&db.AdminUser{}).Where("id=?", actor.UserID).Update("role", constUtil.AdminRoleAdmin).Error; err != nil {
		t.Fatal(err)
	}
	actor.Role = constUtil.AdminRoleAdmin
	return f, actor
}
func reuseClockDay(t *testing.T, f *p1Fixture, actor *model.AdminPrincipal) {
	t.Helper()
	clock, err := f.s.AdminServerGameClock(f.ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.AdminChangeServerGameClock(f.ctx, actor, model.ServerGameClockChangeRequest{DeltaDays: 1, Revision: clock.Revision, RequestID: fmt.Sprintf("repeat-fixture-%d", time.Now().UnixNano())}); err != nil {
		t.Fatal(err)
	}
}
func reusePublish(t *testing.T, f *p1Fixture, actor *model.AdminPrincipal, definition model.AdminActivityDefinitionDTO, startDay, duration int64, timing string) model.AdminActivityOccurrenceDTO {
	t.Helper()
	calendar, err := f.s.AdminActivitySchedule(f.ctx, actor, 1)
	if err != nil {
		t.Fatal(err)
	}
	zone := time.FixedZone("UTC+8", 8*3600)
	now := time.Now().In(zone)
	base := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, zone)
	item := model.AdminActivityOccurrenceDTO{TemplateID: definition.ID, TemplateVersion: definition.Version, Mode: definition.Mode, Timing: timing, StartDay: startDay, DurationDays: duration, StartAt: base.AddDate(0, 0, int(startDay-1)).UnixMilli(), EndAt: base.AddDate(0, 0, int(startDay-1+duration)).UnixMilli(), ClaimEndAt: base.AddDate(0, 0, int(startDay-1+duration)).UnixMilli(), Enabled: true}
	next, err := f.s.AdminSaveActivitySchedule(f.ctx, actor, 1, model.AdminActivityScheduleSaveRequest{Version: calendar.Version, Items: append(calendar.Items, item)})
	if err != nil {
		t.Fatal(err)
	}
	for _, published := range next.Items {
		if published.TemplateID == definition.ID && published.StartDay == startDay && published.StartAt == item.StartAt {
			return published
		}
	}
	t.Fatal("published occurrence missing")
	return item
}
func reuseToggle(t *testing.T, f *p1Fixture, actor *model.AdminPrincipal, id string, enabled bool) {
	t.Helper()
	calendar, err := f.s.AdminActivitySchedule(f.ctx, actor, 1)
	if err != nil {
		t.Fatal(err)
	}
	for i := range calendar.Items {
		if calendar.Items[i].ID == id {
			calendar.Items[i].Enabled = enabled
		}
	}
	if _, err = f.s.AdminSaveActivitySchedule(f.ctx, actor, 1, model.AdminActivityScheduleSaveRequest{Version: calendar.Version, Items: calendar.Items}); err != nil {
		t.Fatal(err)
	}
}
func reuseTrade(t *testing.T, f *p1Fixture, ctx context.Context, source model.AdminActivityOccurrenceDTO, stamp int64) ([]proto.Message, error) {
	t.Helper()
	_, key, _ := strings.Cut(source.ConsumerID, ":")
	req := &pb.CSGetActivityShopReward{ActivityId: proto.String(key), Index: proto.Int32(1), CostResources: []*pb.BaseResource{{Id: proto.String("diamond"), Num: proto.Int32(1)}}, GetResources: []*pb.BaseResource{{Id: proto.String("coin"), Num: proto.Int32(7)}}}
	return f.s.p4Commerce(ctx, f.id, req, stamp, LocalPlayNow(ctx))
}

// Global fixed-date entrances and real private materialization both enforce the
// template policy from the issued snapshot, not its currently edited definition.
func TestAdminActivityReuseOnceRepeatAndSnapshot(t *testing.T) {
	f, actor := reuseFixture(t, false)
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error { p.Diamond = proto.Int64(100); return nil }); err != nil {
		t.Fatal(err)
	}
	form := adminModeForm("exchange")
	form.RepeatPolicy = "once"
	form.Products = []model.AdminActivityProduct{{ID: "reuse-trade", Name: "普通奖励", Index: 1, Rewards: []model.AdminResourceAmount{{ResourceID: "coin", Amount: 7}}, Cost: model.AdminResourceAmount{ResourceID: "diamond", Amount: 1}, DailyLimit: 1, TotalLimit: 1, Stock: 1, MinLevel: 1, DiscountBps: 10000, Weight: 1}}
	definition, err := f.s.AdminSaveActivityDefinition(f.ctx, actor, "", model.AdminActivityDefinitionSaveRequest{Name: "单次原兑换", Form: form, DefaultDays: 1, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	first := reusePublish(t, f, actor, *definition, 1, 1, "calendar")
	second := reusePublish(t, f, actor, *definition, 2, 1, "calendar")
	loginForm := adminModeForm("login-rewards")
	loginForm.RepeatPolicy = "once"
	loginForm.Milestones = []model.AdminActivityMilestone{{Goal: 1, Rewards: []model.AdminResourceAmount{{ResourceID: "coin", Amount: 3}}}}
	login, err := f.s.AdminSaveActivityDefinition(f.ctx, actor, "", model.AdminActivityDefinitionSaveRequest{Name: "单次登录", Form: loginForm, DefaultDays: 1, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	reusePublish(t, f, actor, *login, 1, 1, "character-day")
	reusePublish(t, f, actor, *login, 2, 1, "character-day")
	bound := serverClockBindTest(t, f, f.id)
	if err = f.s.PrepareAdminCharacterActivitySchedules(bound, f.id); err != nil {
		t.Fatal(err)
	}
	paid, err := reuseTrade(t, f, bound, first, 64001)
	if err != nil {
		t.Fatal("first once trade", err)
	}
	reuseClockDay(t, f, actor)
	bound = serverClockBindTest(t, f, f.id)
	if err = f.s.PrepareAdminCharacterActivitySchedules(bound, f.id); err != nil {
		t.Fatal(err)
	}
	if _, err = reuseTrade(t, f, bound, second, 64002); err == nil {
		t.Fatal("once template accepted second fixed period")
	}
	if got := p1Count(t, &db.AdminCharacterActivityInstance{}, "player_id=? AND mode=?", f.id, "login-rewards"); got != 1 {
		t.Fatal("once relative template materialized second period", got)
	}
	// A template edit cannot turn an already-issued once snapshot into repeat.
	form.RepeatPolicy = "repeat"
	definition, err = f.s.AdminSaveActivityDefinition(f.ctx, actor, definition.ID, model.AdminActivityDefinitionSaveRequest{Name: definition.Name, Form: form, DefaultDays: 1, Enabled: true, Version: definition.Version})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = reuseTrade(t, f, bound, second, 64003); err == nil {
		t.Fatal("template edit rewrote issued once policy")
	}
	third := reusePublish(t, f, actor, *definition, 3, 1, "calendar")
	fourth := reusePublish(t, f, actor, *definition, 4, 1, "calendar")
	if third.Form.RepeatPolicy != "repeat" || second.Form.RepeatPolicy != "once" {
		t.Fatal("source policy snapshot incorrect")
	}
	reuseClockDay(t, f, actor)
	bound = serverClockBindTest(t, f, f.id)
	repeatPaid, err := reuseTrade(t, f, bound, third, 64004)
	if err != nil {
		t.Fatal("repeat third period", err)
	}
	reuseToggle(t, f, actor, third.ID, false)
	reuseToggle(t, f, actor, third.ID, true)
	if _, err = reuseTrade(t, f, bound, third, 64005); err == nil {
		t.Fatal("stop/start reset same-period stock")
	}
	again, err := reuseTrade(t, f, bound, third, 64004)
	if err != nil || !proto.Equal(again[0], repeatPaid[0]) {
		t.Fatal("same-period receipt changed", err)
	}
	reuseClockDay(t, f, actor)
	bound = serverClockBindTest(t, f, f.id)
	if _, err = reuseTrade(t, f, bound, fourth, 64006); err != nil {
		t.Fatal("repeat fourth period", err)
	}
	// Distinct operator definitions may share the same original art template.
	// They keep independent once eligibility.
	cloneForm := form
	cloneForm.RepeatPolicy = "once"
	clone, err := f.s.AdminSaveActivityDefinition(f.ctx, actor, "", model.AdminActivityDefinitionSaveRequest{Name: "独立运营活动同美术", Form: cloneForm, DefaultDays: 1, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	independent := reusePublish(t, f, actor, *clone, 4, 1, "calendar")
	if _, err = reuseTrade(t, f, bound, independent, 64007); err != nil {
		t.Fatal("art template identity incorrectly shared once eligibility", err)
	}
	f.restart(t)
	if err = f.s.InitializeAdminActivitySchedules(f.ctx); err != nil {
		t.Fatal(err)
	}
	again, err = reuseTrade(t, f, bound, first, 64001)
	if err != nil || !proto.Equal(again[0], paid[0]) {
		t.Fatal("cold original once receipt changed", err)
	}
}

// Each repeat opening period has independent progress/counters/stock while
// receipt stamps remain unique across periods and legacy rows are never cleared.
func TestAdminOpeningReuseOnceRepeatAndCold(t *testing.T) {
	for _, policy := range []string{"once", "repeat"} {
		t.Run(policy, func(t *testing.T) {
			f, actor := reuseFixture(t, true)
			role, err := LoginAccount(f.ctx, "opening-reuse-"+policy, "fixture-password")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = CreatePlayer(f.ctx, role.Id, "按期领取角色", "hero_043"); err != nil {
				t.Fatal(err)
			}
			f.id = role.Id
			forms, err := f.s.AdminActivityDefinitions(f.ctx, actor)
			if err != nil {
				t.Fatal(err)
			}
			var definition model.AdminActivityDefinitionDTO
			for _, d := range forms {
				if d.ID == "original_opening_14" {
					definition = d
				}
			}
			definition.Form.RepeatPolicy = policy
			saved, err := f.s.AdminSaveActivityDefinition(f.ctx, actor, definition.ID, model.AdminActivityDefinitionSaveRequest{Name: definition.Name, Form: definition.Form, DefaultDays: 1, Enabled: true, Version: definition.Version})
			if err != nil {
				t.Fatal(err)
			}
			first := reusePublish(t, f, actor, *saved, 1, 1, "character-day")
			second := reusePublish(t, f, actor, *saved, 2, 1, "character-day")
			bound := serverClockBindTest(t, f, f.id)
			now := LocalPlayNow(bound)
			request := &pb.CSGetOpenServerReward{ItemId: proto.Int32(1)}
			// First information acceptance fixes this phase before the real login fact.
			if _, err = f.s.openServerActivityAt(bound, f.id, &pb.CSOpenServerActivity{DayType: proto.Int32(7)}, 0, now); err != nil {
				t.Fatal(err)
			}
			if err = recordOpenServerCounterAt(db.GetIns().WithContext(bound), f.id, constUtil.OpenServerActivityCounterLogin+p1Day(now), 1, now); err != nil {
				t.Fatal(err)
			}
			firstClaim, err := f.s.openServerActivityAt(bound, f.id, request, 65001, now)
			if err != nil {
				t.Fatal("first real opening reward", err)
			}
			reuseClockDay(t, f, actor)
			bound = serverClockBindTest(t, f, f.id)
			now = LocalPlayNow(bound)
			result, err := f.s.openServerActivityAt(bound, f.id, &pb.CSOpenServerActivity{DayType: proto.Int32(7)}, 0, now)
			if policy == "once" {
				if err == nil {
					t.Fatal("once opening accepted another phase", result)
				}
			} else {
				if err != nil {
					t.Fatal("repeat opening entrance", err)
				}
				if _, err = f.s.openServerActivityAt(bound, f.id, request, 65002, now); err == nil {
					t.Fatal("second phase reused first login/progress")
				}
				if err = recordOpenServerCounterAt(db.GetIns().WithContext(bound), f.id, constUtil.OpenServerActivityCounterLogin+p1Day(now), 1, now); err != nil {
					t.Fatal(err)
				}
				secondClaim, e := f.s.openServerActivityAt(bound, f.id, request, 65002, now)
				if e != nil {
					t.Fatal("second real period reward", e)
				}
				reuseToggle(t, f, actor, second.ID, false)
				reuseToggle(t, f, actor, second.ID, true)
				if _, e = f.s.openServerActivityAt(bound, f.id, request, 65003, now); e == nil {
					t.Fatal("same period stop/start reset claims")
				}
				again, e := f.s.openServerActivityAt(bound, f.id, request, 65002, now)
				if e != nil || !proto.Equal(again[0], secondClaim[0]) {
					t.Fatal("second phase receipt changed", e)
				}
				for _, source := range []model.AdminActivityOccurrenceDTO{first, second} {
					if got := p1Count(t, &db.AdminOpeningState{}, "player_id=? AND occurrence_id=? AND task_id=? AND claims=?", f.id, source.ID, 1, 1); got != 1 {
						t.Fatal("period-owned original task claim missing", source.ID, got)
					}
				}
			}
			old, err := f.s.openServerActivityAt(bound, f.id, request, 65001, now)
			if err != nil || !proto.Equal(old[0], firstClaim[0]) {
				t.Fatal("old stamp granted current phase instead of replay", err)
			}
			f.restart(t)
			if err = f.s.InitializeAdminActivitySchedules(f.ctx); err != nil {
				t.Fatal(err)
			}
			cold, err := f.s.openServerActivityAt(bound, f.id, request, 65001, now)
			if err != nil || !proto.Equal(cold[0], firstClaim[0]) {
				t.Fatal("cold old receipt changed", err)
			}
		})
	}
}
