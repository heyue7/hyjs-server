package service

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func serverClockBindTest(t *testing.T, f *p1Fixture, id int64) context.Context {
	t.Helper()
	var owner db.Profile
	if err := db.GetIns().First(&owner, id).Error; err != nil {
		t.Fatal(err)
	}
	ctx := context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	bound, err := f.s.BindLocalPlayContext(ctx, &owner)
	if err != nil || !ServerGameClockContextBound(bound) || LocalPlayContextBound(bound) || GameCalendarOwner(bound) != id {
		t.Fatal("ordinary original authentication did not bind isolated server calendar", err)
	}
	return bound
}

// TestServerGameClockAllPlayersCalendarRecoveryAndCold covers real daily consumers across ordinary and demo accounts.
func TestServerGameClockAllPlayersCalendarRecoveryAndCold(t *testing.T) {
	adminWalletTestEnvironment(t)
	f, demoID, _ := localClockFixture(t)
	admin := adminWalletTestActor(t, f, constUtil.AdminRoleAdmin)
	operator := adminWalletTestActor(t, f, constUtil.AdminRoleOperator)
	account, err := LoginAccount(f.ctx, "ordinary-clock-"+uuid.NewString(), "isolated-normal-player")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CreatePlayer(f.ctx, account.Id, "普通全服日期", "hero_043"); err != nil {
		t.Fatal(err)
	}
	normalID := account.Id
	if err := db.GetIns().Save(&db.LocalPlayClockState{PlayerID: demoID, OffsetDays: 7, Revision: 1}).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeServerGameClock(f.ctx); err != nil {
		t.Fatal(err)
	}
	f.s.LocalPlayToolsEnabled = false
	f.s.P3LocalWalletEnabled = false
	// The unrecovered operating reward calendar remains closed in production.
	// This owned fixture explicitly supplies a one-coin calendar solely to check dates and historical claims.
	var priorSignRules []db.MonthSignRule
	if err := db.GetIns().Order("day").Find(&priorSignRules).Error; err != nil {
		t.Fatal(err)
	}
	if len(priorSignRules) != 31 {
		t.Fatal("finite sign fixture needs all 31 calendar slots")
	}
	t.Cleanup(func() {
		for _, rule := range priorSignRules {
			if err := db.GetIns().Save(&rule).Error; err != nil {
				t.Error(err)
			}
		}
	})
	if err := db.GetIns().Model(&db.MonthSignRule{}).Where("day>=1 AND day<=31").Updates(map[string]any{"enabled": true, "reward": "coin,1", "source": "finite server-clock test calendar; not an operating reward schedule"}).Error; err != nil {
		t.Fatal(err)
	}
	// Binding includes protected historical player 1 and leaves its persisted account untouched.
	var protectedBefore, protectedAfter db.Profile
	if err := db.GetIns().First(&protectedBefore, 1).Error; err != nil {
		t.Fatal(err)
	}
	serverClockBindTest(t, f, 1)
	if err := db.GetIns().First(&protectedAfter, 1).Error; err != nil || !reflect.DeepEqual(protectedBefore, protectedAfter) {
		t.Fatal("read-only clock binding modified account one", err)
	}
	oldContexts := map[int64]context.Context{}
	consume := func(id int64) {
		t.Helper()
		if err := updateProfile(f.ctx, id, func(p *pb.Player, _ *model.GameState) error {
			p.Energy = proto.Int32(10)
			p.Stamina = proto.Int32(40)
			p.EnergyLastRefreshTime = proto.Int64(time.Now().UnixMilli())
			p.StaminaLastRefreshTime = proto.Int64(time.Now().UnixMilli())
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []int64{demoID, normalID} {
		consume(id)
		ctx := serverClockBindTest(t, f, id)
		oldContexts[id] = ctx
		real := time.Now()
		if LocalPlayAt(ctx, real).UnixMilli() != real.UnixMilli() {
			t.Fatal("old personal calendar offset leaked into global calendar")
		}
		if _, err := f.s.dailyTasks(ctx, id, &pb.CSTaskList{}); err != nil {
			t.Fatal(err)
		}
		if _, err := f.s.monthSign(ctx, id, &pb.CSReceiveMonthSign{}, id*100+1); err != nil {
			t.Fatal("original day-zero sign", err)
		}
	}
	request := model.ServerGameClockChangeRequest{DeltaDays: 1, Revision: 0, RequestID: uuid.NewString()}
	if _, err := f.s.AdminChangeServerGameClock(f.ctx, operator, request); !errors.Is(err, constUtil.ErrAdminPermission) {
		t.Fatal("operator changed whole server clock", err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	views := make(chan *model.ServerGameClockView, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			view, err := f.s.AdminChangeServerGameClock(f.ctx, admin, request)
			errs <- err
			if err == nil {
				views <- view
			}
		}()
	}
	wg.Wait()
	close(errs)
	close(views)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var first *model.ServerGameClockView
	for view := range views {
		if first == nil {
			first = view
		} else if !reflect.DeepEqual(first, view) {
			t.Fatal("concurrent server retry changed frozen response")
		}
	}
	if first == nil || first.OffsetDays != 1 || first.Revision != 1 || first.Scope != "server" || first.ServerID != 1 {
		t.Fatal("global request advanced more than once", first)
	}
	if _, err := f.s.AdminChangeServerGameClock(f.ctx, admin, model.ServerGameClockChangeRequest{DeltaDays: 1, Revision: 0, RequestID: uuid.NewString()}); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatal("stale global revision was accepted", err)
	}
	// Worker contexts use the shared cache without needing a player or wallet identity.
	worker := context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	real := time.Now()
	if LocalPlayAt(worker, real).Sub(real) != 24*time.Hour {
		t.Fatal("server worker calendar did not advance")
	}
	for _, id := range []int64{demoID, normalID} {
		if err := updateProfile(oldContexts[id], id, func(*pb.Player, *model.GameState) error { return nil }); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
			t.Fatal("old request snapshot crossed global date revision", err)
		}
		ctx := serverClockBindTest(t, f, id)
		if err := updateProfile(ctx, id, func(*pb.Player, *model.GameState) error { return nil }); err != nil {
			t.Fatal(err)
		}
		f.id = id
		p, _ := f.profile(t)
		if p.GetEnergy() != constUtil.EnergyRecoveryLimit || p.GetStamina() != constUtil.StaminaRecoveryLimit {
			t.Fatal("ordinary player did not receive original forward recovery", id, p.GetEnergy(), p.GetStamina())
		}
		if _, err := f.s.dailyTasks(ctx, id, &pb.CSTaskList{}); err != nil {
			t.Fatal(err)
		}
		if _, err := f.s.monthSign(ctx, id, &pb.CSReceiveMonthSign{}, id*100+2); err != nil {
			t.Fatal("original day-one sign", err)
		}
		if count := p1Count(t, &db.DailyTaskState{}, "player_id=?", id); count != 2 {
			t.Fatal("ordinary daily consumer did not retain both dates", id, count)
		}
		consume(id)
	}
	if _, err := f.s.AdminChangeServerGameClock(f.ctx, admin, model.ServerGameClockChangeRequest{DeltaDays: -1, Revision: 1, RequestID: uuid.NewString()}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{demoID, normalID} {
		ctx := serverClockBindTest(t, f, id)
		if _, err := f.s.monthSign(ctx, id, &pb.CSReceiveMonthSign{}, id*100+3); err == nil {
			t.Fatal("rewind repeated a historical sign reward", id)
		}
	}
	if _, err := f.s.AdminChangeServerGameClock(f.ctx, admin, model.ServerGameClockChangeRequest{DeltaDays: 1, Revision: 2, RequestID: uuid.NewString()}); err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	if err := f.s.InitializeServerGameClock(f.ctx); err != nil {
		t.Fatal(err)
	}
	cold, err := f.s.AdminChangeServerGameClock(f.ctx, admin, request)
	if err != nil || !reflect.DeepEqual(cold, first) {
		t.Fatal("cold original global receipt changed", err)
	}
	current, err := f.s.AdminServerGameClock(f.ctx, operator)
	if err != nil || current.OffsetDays != 1 || current.Revision != 3 {
		t.Fatal("old receipt rewound current server state", current, err)
	}
	for _, id := range []int64{demoID, normalID} {
		ctx := serverClockBindTest(t, f, id)
		if err := updateProfile(ctx, id, func(*pb.Player, *model.GameState) error { return nil }); err != nil {
			t.Fatal(err)
		}
		f.id = id
		p, _ := f.profile(t)
		if p.GetEnergy() != 10 || p.GetStamina() != 40 {
			t.Fatal("rewind and revisit repeated a resource recovery", id)
		}
	}
	if count := p1Count(t, &db.ServerGameClockOperation{}, "server_id=?", 1); count != 3 {
		t.Fatal("global idempotency duplicated date operations", count)
	}
}

// TestServerGameClockPaymentWallTimeAndActiveBattle keeps accounting and original membership calendars separate.
func TestServerGameClockPaymentWallTimeAndActiveBattle(t *testing.T) {
	adminWalletTestEnvironment(t)
	f, id, _ := localClockFixture(t)
	admin := adminWalletTestActor(t, f, constUtil.AdminRoleAdmin)
	if err := f.s.InitializeServerGameClock(f.ctx); err != nil {
		t.Fatal(err)
	}
	localWalletEnable(t)
	first, err := f.s.AdminChangeServerGameClock(f.ctx, admin, model.ServerGameClockChangeRequest{DeltaDays: 1, Revision: 0, RequestID: uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	ctx := serverClockBindTest(t, f, id)
	var product db.PaymentProduct
	if err := db.GetIns().Where("kind=? AND channels LIKE ?", "month", "%gf_default%").Order("id").First(&product).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.LocalWalletAdd(ctx, id, "global-month-fund", product.Cents); err != nil {
		t.Fatal(err)
	}
	f.s.P3AutoConfirmDemoPayments = true
	real := time.Now()
	response, err := f.s.generatePayment(ctx, id, p3DemoProviderRequest(t, product.ID), 88501, real)
	if err != nil {
		t.Fatal(err)
	}
	orderID := response[0].(*pb.SCGenerateOrder).GetOrderId()
	var order db.PaymentOrder
	if err := db.GetIns().First(&order, "id=?", orderID).Error; err != nil || order.Status != constUtil.P3PaymentConfirmed || order.CreatedAt != real.UnixMilli() || order.ConfirmedAt != real.UnixMilli() || order.ExpiresAt != real.UnixMilli()+constUtil.P3PaymentOrderLifetimeMS {
		t.Fatal("global calendar changed real accounting/order expiry", err)
	}
	var debit db.LocalWalletEntry
	if err := db.GetIns().First(&debit, "order_id=?", orderID).Error; err != nil || debit.At != real.UnixMilli() || debit.DeltaCents != -product.Cents {
		t.Fatal("global clock changed wallet accounting", err)
	}
	var projection db.LocalPlayPaymentTime
	if err := db.GetIns().First(&projection, "order_id=?", orderID).Error; err != nil || projection.GameAt != LocalPlayAt(ctx, real).UnixMilli() || projection.RealAt != real.UnixMilli() || projection.OffsetDays != 1 {
		t.Fatal("paid activity calendar lost original wall-time facts", err)
	}
	if _, err := f.s.membership(ctx, id, &pb.CSClaimCard{CardType: proto.String("month")}, LocalPlayAt(ctx, real)); err != nil {
		t.Fatal("global purchase-day original membership", err)
	}
	if _, err := f.s.AdminChangeServerGameClock(f.ctx, admin, model.ServerGameClockChangeRequest{DeltaDays: -1, Revision: first.Revision, RequestID: uuid.NewString()}); err != nil {
		t.Fatal(err)
	}
	ctx = serverClockBindTest(t, f, id)
	if _, err := f.s.membership(ctx, id, &pb.CSClaimCard{CardType: proto.String("month")}, LocalPlayAt(ctx, real)); err == nil {
		t.Fatal("rewound server borrowed a future-paid entitlement")
	}
	// A finite original active-run fixture must block the entire clock change atomically.
	if err := updateProfile(ctx, id, func(_ *pb.Player, g *model.GameState) error {
		g.Run = &model.CopyRun{Chapter: "1", Mission: "1", Complete: false}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	before, err := f.s.AdminServerGameClock(f.ctx, admin)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.AdminChangeServerGameClock(f.ctx, admin, model.ServerGameClockChangeRequest{DeltaDays: 1, Revision: before.Revision, RequestID: uuid.NewString()}); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatal("server changed date during an original active battle", err)
	}
	after, err := f.s.AdminServerGameClock(f.ctx, admin)
	if err != nil || after.OffsetDays != before.OffsetDays || after.Revision != before.Revision {
		t.Fatal("rejected calendar mutation changed persistent state", err)
	}
	if err := updateProfile(ctx, id, func(_ *pb.Player, g *model.GameState) error { g.Run = nil; return nil }); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().AutoMigrate(&db.P12WantedInstance{}, &db.P12WantedFight{}); err != nil {
		t.Fatal(err)
	}
	// This finite pending fixture uses the original persisted lease fields without fabricating battle results.
	instance := db.P12WantedInstance{ID: "clock-pending-" + uuid.NewString(), State: constUtil.P12WantedActive, ExpiresAt: time.Now().Add(time.Hour).UnixMilli(), CurrentHP: 1}
	fight := db.P12WantedFight{ID: uuid.NewString(), PlayerID: id, InstanceID: instance.ID, CreatedAt: time.Now().UnixMilli(), StartProto: []byte{}, EndProto: []byte{}, InputJSON: "{}", TerminalJSON: "{}"}
	if err := db.GetIns().Create(&instance).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Create(&fight).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.AdminChangeServerGameClock(f.ctx, admin, model.ServerGameClockChangeRequest{DeltaDays: 1, Revision: before.Revision, RequestID: uuid.NewString()}); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatal("independent live pending battle did not block global date", err)
	}
	if err := db.GetIns().Model(&instance).Update("expires_at", time.Now().Add(-time.Minute).UnixMilli()).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.AdminChangeServerGameClock(f.ctx, admin, model.ServerGameClockChangeRequest{DeltaDays: 1, Revision: before.Revision, RequestID: uuid.NewString()}); err != nil {
		t.Fatal("expired historical pending battle permanently blocked calendar", err)
	}
	var kept db.PaymentOrder
	if err := db.GetIns().First(&kept, "id=?", orderID).Error; err != nil || !reflect.DeepEqual(kept, order) {
		t.Fatal("calendar changed frozen accounting order", err)
	}
}
