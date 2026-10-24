package service

import (
	"errors"
	"strconv"
	"testing"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func feedback66SpendFact(t *testing.T, f *p1Fixture, source string, amount int64, day string) {
	t.Helper()
	var facts []db.EventFact
	if err := db.GetIns().Where("player_id=? AND source_key=? AND kind=?", f.id, source, constUtil.P1EventDiamondSpent).Find(&facts).Error; err != nil {
		t.Fatal(err)
	}
	if amount == 0 {
		if len(facts) != 0 {
			t.Fatal("failed purchase left spending facts", facts)
		}
		return
	}
	if len(facts) != 1 || facts[0].Amount != amount || facts[0].Day != day {
		t.Fatal("actual purchase spending fact", facts, amount, day)
	}
}

// Fail the final wallet write after the real purchase has created its spending fact.
func feedback66RejectProfileSave(t *testing.T, call func() error) {
	t.Helper()
	injected := errors.New("feedback66 final profile save failure")
	callbacks := db.GetIns().Callback().Update()
	name := "feedback66:reject-profile-save"
	if err := callbacks.Before("gorm:update").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == "profiles" {
			tx.AddError(injected)
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer callbacks.Remove(name)
	if err := call(); !errors.Is(err, injected) {
		t.Fatal("purchase did not reach failing final save", err)
	}
}

func TestFeedback66FundPurchaseSpendAtomicAndCold(t *testing.T) {
	f := p1Setup(t)
	if err := f.s.InitializeActivities(f.ctx); err != nil {
		t.Fatal(err)
	}
	var cfg db.ActivityRules
	if err := db.GetIns().First(&cfg, "id=?", "open_server_fund").Error; err != nil {
		t.Fatal(err)
	}
	source := "fund:" + cfg.ID + ":" + strconv.FormatInt(f.id, 10)
	setBalance := func(balance int64) {
		t.Helper()
		if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error {
			p.Vip = proto.Int32(cfg.MinVIP)
			p.Diamond = proto.Int64(balance)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	buy := func() error { _, err := f.s.openServerFund(f.ctx, f.id, &pb.CSBuyOpenServerFund{}); return err }
	setBalance(int64(cfg.DiamondCost) - 1)
	if err := buy(); err == nil {
		t.Fatal("insufficient fund purchase accepted")
	}
	feedback66SpendFact(t, f, source, 0, "")
	setBalance(int64(cfg.DiamondCost) + 50)
	feedback66RejectProfileSave(t, buy)
	feedback66SpendFact(t, f, source, 0, "")
	p, _ := f.profile(t)
	if p.GetDiamond() != int64(cfg.DiamondCost)+50 || p1Count(t, &db.ActivityState{}, "activity_id=? AND player_id=? AND bought=?", cfg.ID, f.id, true) != 0 {
		t.Fatal("fund rollback changed wallet or ownership")
	}
	before := LocalPlayNow(f.ctx)
	if err := buy(); err != nil {
		t.Fatal(err)
	}
	after := LocalPlayNow(f.ctx)
	// A test crossing midnight may legitimately commit on either adjacent date.
	var fact db.EventFact
	if err := db.GetIns().First(&fact, "player_id=? AND source_key=? AND kind=?", f.id, source, constUtil.P1EventDiamondSpent).Error; err != nil {
		t.Fatal(err)
	}
	if fact.Day != p1Day(before) && fact.Day != p1Day(after) {
		t.Fatal("fund spending used a different calendar date", fact.Day)
	}
	feedback66SpendFact(t, f, source, int64(cfg.DiamondCost), fact.Day)
	if err := buy(); err == nil {
		t.Fatal("duplicate fund purchase accepted")
	}
	f.restart(t)
	if err := buy(); err == nil {
		t.Fatal("cold duplicate fund purchase accepted")
	}
	feedback66SpendFact(t, f, source, int64(cfg.DiamondCost), fact.Day)
	p, _ = f.profile(t)
	if p.GetDiamond() != 50 {
		t.Fatal("fund charged more than once", p.GetDiamond())
	}
}

func TestFeedback66WorldBossBoostSpendAtomicAndCold(t *testing.T) {
	f, _, boss, now := p12WorldBossBattleSetup(t, true)
	if err := f.s.InitializeP1(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P12WorldBossBattlePolicy{}).Where("id=1").Update("economy_enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	property := "wordboss_1"
	source := "worldboss:boost:" + boss + ":" + strconv.FormatInt(f.id, 10) + ":" + property
	setBalance := func(balance int64) {
		t.Helper()
		if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error { p.Diamond = proto.Int64(balance); return nil }); err != nil {
			t.Fatal(err)
		}
	}
	buy := func() error { _, err := f.s.PurchaseP12WorldBossProp(f.ctx, f.id, property, now); return err }
	setBalance(149)
	if err := buy(); err == nil {
		t.Fatal("insufficient boost purchase accepted")
	}
	feedback66SpendFact(t, f, source, 0, "")
	setBalance(200)
	feedback66RejectProfileSave(t, buy)
	feedback66SpendFact(t, f, source, 0, "")
	p, _ := f.profile(t)
	if p.GetDiamond() != 200 || p1Count(t, &db.P12WorldBossBoost{}, "instance_id=? AND player_id=? AND property_id=?", boss, f.id, property) != 0 {
		t.Fatal("boost rollback changed wallet or purchase ledger")
	}
	if err := buy(); err != nil {
		t.Fatal(err)
	}
	feedback66SpendFact(t, f, source, 150, p1Day(now))
	if err := buy(); err != nil {
		t.Fatal("boost duplicate query", err)
	}
	f.restart(t)
	if err := buy(); err != nil {
		t.Fatal("cold boost duplicate query", err)
	}
	feedback66SpendFact(t, f, source, 150, p1Day(now))
	p, _ = f.profile(t)
	if p.GetDiamond() != 50 {
		t.Fatal("boost charged more than once", p.GetDiamond())
	}
}
