package service

import (
	"context"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"testing"
	"time"
)

func TestNewPlayerWalletRegularAccount(t *testing.T) {
	f, id, ctx := localWalletFixture(t)
	localWalletEnable(t)
	f.s.P3AutoConfirmDemoPayments = true
	if err := db.GetIns().Model(&db.Profile{}).Where("id=?", id).Update("uid", "registered-wallet-player").Error; err != nil {
		t.Fatal(err)
	}
	view, err := f.s.LocalWalletSnapshot(ctx, id)
	if err != nil || view.BalanceCents != 0 {
		t.Fatal("ordinary registered account wallet", view, err)
	}
	if _, err = f.s.LocalWalletAdd(ctx, id, "registered-credit", 600); err != nil {
		t.Fatal(err)
	}
	out, err := f.s.generatePayment(ctx, id, p3DemoProviderRequest(t, "djsy.wuyou.andp0006"), 80091, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var order db.PaymentOrder
	if err = db.GetIns().First(&order, "id=?", out[0].(*pb.SCGenerateOrder).GetOrderId()).Error; err != nil || order.Status != constUtil.P3PaymentConfirmed {
		t.Fatal("ordinary original purchase", order.Status, err)
	}
	view, err = f.s.LocalWalletSnapshot(ctx, id)
	if err != nil || view.BalanceCents != 0 || len(view.Entries) != 2 {
		t.Fatal("one credit and one debit", view, err)
	}
	var owner db.Profile
	if err = db.GetIns().First(&owner, id).Error; err != nil {
		t.Fatal(err)
	}
	if localWalletOwner(&owner) {
		t.Fatal("wallet eligibility opened retired personal tools")
	}
	bad := context.WithValue(ctx, constUtil.LocalWalletSessionContextKey, make([]byte, 16))
	if _, err = f.s.LocalWalletSnapshot(bad, id); err == nil {
		t.Fatal("wrong session accepted")
	}
	f.restart(t)
	view, err = f.s.LocalWalletSnapshot(ctx, id)
	if err != nil || len(view.Entries) != 2 {
		t.Fatal("cold wallet", err)
	}
}

func TestPlayerOpeningDaysSeparateAndPersistent(t *testing.T) {
	f, id := p3DemoProviderFixture(t)
	loadOpenServerTaskFixture(t, f.s)
	if err := f.s.InitializeOpenServerActivity(f.ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	earlier := now.AddDate(0, 0, -2).UnixMilli()
	if err := db.GetIns().Model(&db.Profile{}).Where("id=?", id).Update("created_at", earlier).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializePlayerOpeningCalendar(f.ctx); err != nil {
		t.Fatal(err)
	}
	row, err := LoginAccount(f.ctx, "opening-new-player", "test-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(f.ctx, row.Id, "新角色", "hero_043"); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		id  int64
		day int32
	}{{id, 3}, {row.Id, 1}} {
		result, e := f.s.openServerActivityAt(f.ctx, test.id, &pb.CSOpenServerActivity{DayType: proto.Int32(7)}, 0, now)
		if e != nil {
			t.Fatal(e)
		}
		reply := result[0].(*pb.SCOpenServerActivity)
		if reply.GetCurrentDay() != test.day {
			t.Fatal("per-role opening", test, reply)
		}
		states, e := f.s.openServerOpenStatesAt(f.ctx, test.id, now)
		if e != nil || states[0].GetIsOpen() != 1 {
			t.Fatal("first seven day entry", states, e)
		}
	}
	cfg, err := loadPlayerOpenServerRule(db.GetIns(), row.Id)
	if err != nil {
		t.Fatal(err)
	}
	if day := openServerDay(cfg, now.AddDate(0, 0, 1)); day != 2 {
		t.Fatal("global next day advances personal age", day)
	}
	marker := db.OpenServerActivityState{PlayerID: row.Id, TaskID: 1, Claims: 1, Achieved: true, Progress: 1}
	if err = db.GetIns().Create(&marker).Error; err != nil {
		t.Fatal(err)
	}
	if err = f.s.InitializePlayerOpeningCalendar(f.ctx); err != nil {
		t.Fatal(err)
	}
	var saved db.OpenServerActivityState
	if err = db.GetIns().First(&saved, "player_id=? AND task_id=1", row.Id).Error; err != nil || saved.Claims != 1 {
		t.Fatal("calendar changed old claim", err)
	}
	// 已推进全服日期时，后台建号同样从该角色第1天开始。
	if err = AdminInitialize(f.ctx, ""); err != nil {
		t.Fatal(err)
	}
	if err = f.s.InitializeServerGameClock(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Model(&db.ServerGameClock{}).Where("server_id=?", f.s.serverGameClockID()).Updates(map[string]any{"offset_days": 7, "high_days": 7, "revision": 1}).Error; err != nil {
		t.Fatal(err)
	}
	operator := db.AdminUser{Username: "opening-admin", PasswordHash: "unused-fixture", Role: constUtil.AdminRoleOperator, Enabled: true}
	if err = db.GetIns().Create(&operator).Error; err != nil {
		t.Fatal(err)
	}
	created, err := f.s.AdminCreateGamePlayer(f.ctx, &model.AdminPrincipal{UserID: operator.ID, Username: operator.Username, Role: operator.Role}, model.AdminGamePlayerCreateRequest{Username: "opening-admin-role", Password: "isolated-password", Name: "后台新角色", HeroID: "hero_043", Reason: "isolated calendar verification"})
	if err != nil {
		t.Fatal(err)
	}
	adminCfg, err := loadPlayerOpenServerRule(db.GetIns(), created.ID)
	if err != nil || openServerDay(adminCfg, now.AddDate(0, 0, 7)) != 1 {
		t.Fatal("admin creation used real date instead of global game date", err)
	}
	f.restart(t)
	again, err := loadPlayerOpenServerRule(db.GetIns(), row.Id)
	if err != nil || again.StartAt != cfg.StartAt {
		t.Fatal("cold start changed role day one", err)
	}
}

func TestStrategyHubLockedWantedEmpty(t *testing.T) {
	f := p12WantedPersistenceFixture(t)
	if err := f.s.p3Profile(f.ctx, f.id, func(tx *gorm.DB, row *db.Profile, p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(1)
		return saveProfile(tx, row, p, g)
	}); err != nil {
		t.Fatal(err)
	}
	before, _ := f.profile(t)
	out, handled, err := f.s.DispatchP12Wanted(f.ctx, f.id, &pb.CSGetRebels{}, 0)
	if err != nil || !handled || len(out[0].(*pb.SCGetRebels).GetRebels()) != 0 {
		t.Fatal("strategy hub blocked by locked wanted", err)
	}
	if _, handled, err = f.s.DispatchP12Wanted(f.ctx, f.id, &pb.CSGetRebelRewardInfo{}, 0); err != nil || !handled {
		t.Fatal("locked reward directory", err)
	}
	after, _ := f.profile(t)
	if !proto.Equal(before, after) {
		t.Fatal("locked list granted recovery or changed player")
	}
	if _, _, err = f.s.DispatchP12Wanted(f.ctx, f.id, &pb.CSRebelFightStart{RebelId: proto.String("missing"), Type: proto.Int32(1)}, 991); err == nil {
		t.Fatal("low-level battle unexpectedly allowed")
	}
}
