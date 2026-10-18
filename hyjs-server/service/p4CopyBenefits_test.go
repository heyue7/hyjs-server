package service

import (
	"math"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p4CopySetup(t *testing.T) (*p1Fixture, db.P4CopyBenefitRule, time.Time) {
	t.Helper()
	f := p4Setup(t)
	if err := f.s.InitializeP4CopyBenefits(f.ctx); err != nil {
		t.Fatal(err)
	}
	for _, table := range []any{&db.P4CopyBenefitGrant{}, &db.P4CopyBenefitRule{}} {
		if err := db.GetIns().Where("1=1").Delete(table).Error; err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	local := now.In(time.FixedZone("UTC+8", 8*3600))
	start := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, local.Location())
	cfg := db.P4CopyBenefitRule{ID: "p4:copy:" + start.Format("200601"), Enabled: true, StartAt: start.UnixMilli(), EndAt: start.AddDate(0, 1, 0).UnixMilli(), WeekdayMask: 127, StartMinute: 0, EndMinute: 1440, CurrencyFactor: 2, DropFactor: 2, Source: "专属测试库确定两倍策略"}
	if err := db.GetIns().Create(&cfg).Error; err != nil {
		t.Fatal(err)
	}
	return f, cfg, now
}

func TestP4CopyBenefitsRealDropAtomicReplayConcurrentRestart(t *testing.T) {
	f, _, now := p4CopySetup(t)
	var base, first *pb.ResultObject
	err := updateProfileWithTx(f.ctx, f.id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		var err error
		base, err = f.s.GrantFixedReward(p, g, "item_005,5;hero_exp_001,1")
		if err != nil {
			return err
		}
		var resources []*pb.BaseResource
		first, resources, err = f.s.ApplyP4CopyDropBonus(tx, f.id, p, g, "copy:confirmed-1", base, now)
		if err == nil && (len(resources) != 2 || resources[1].GetGuid() == "") {
			t.Fatal("real bonus instance absent")
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	out := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out <- updateProfileWithTx(f.ctx, f.id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
				r, _, err := f.s.ApplyP4CopyDropBonus(tx, f.id, p, g, "copy:confirmed-1", base, now)
				if err == nil && !proto.Equal(r, first) {
					t.Error("concurrent bonus receipt differs")
				}
				return err
			})
		}()
	}
	wg.Wait()
	close(out)
	for err := range out {
		if err != nil {
			t.Fatal(err)
		}
	}
	p, g := f.profile(t)
	if simpleOwned(p, g, "item_005") != 10 || len(g.Warriors) != 3 {
		t.Fatal("duplicate/missing extra actual drop")
	}
	if p1Count(t, &db.P4CopyBenefitGrant{}, "player_id=?", f.id) != 1 || p1Count(t, &db.EventFact{}, "player_id=? AND kind=?", f.id, constUtil.P4CopyBenefitsEventKind) != 1 {
		t.Fatal("extra delivery or fact")
	}
	f.restart(t)
	if err = f.s.InitializeP4CopyBenefits(f.ctx); err != nil {
		t.Fatal(err)
	}
	err = updateProfileWithTx(f.ctx, f.id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		r, _, e := f.s.ApplyP4CopyDropBonus(tx, f.id, p, g, "copy:confirmed-1", base, now.AddDate(0, 2, 0))
		if e == nil && !proto.Equal(r, first) {
			t.Error("expired/restart receipt differs")
		}
		return e
	})
	if err != nil {
		t.Fatal(err)
	}
	p, g = f.profile(t)
	if simpleOwned(p, g, "item_005") != 10 || len(g.Warriors) != 3 {
		t.Fatal("restart regranted bonus")
	}
}

func TestP4CopyBenefitsWindowsFreezeDisableOverlapAndOverflow(t *testing.T) {
	f, cfg, now := p4CopySetup(t)
	coin := int64(0)
	err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		var err error
		coin, _, err = f.s.ApplyP4CopyCurrencyBonus(tx, now, 100, 20)
		return err
	})
	if err != nil || coin != 200 {
		t.Fatal("actual currency factor", coin, err)
	}
	if err = db.GetIns().Model(&cfg).Updates(map[string]any{"currency_factor": 0, "end_at": now.Add(-time.Hour).UnixMilli()}).Error; err != nil {
		t.Fatal(err)
	}
	err = db.GetIns().Transaction(func(tx *gorm.DB) error {
		var err error
		coin, _, err = f.s.ApplyP4CopyCurrencyBonus(tx, now, 100, 20)
		return err
	})
	if err != nil || coin != 200 {
		t.Fatal("frozen operating values changed", err)
	}
	tabs, err := f.s.P4CopyBenefitsTabs(f.ctx, now)
	if err != nil || len(tabs) != 1 || tabs[0].GetActivityId() != "allCopyDouble" {
		t.Fatal("original hardcoded activity id", err)
	}
	if err = db.GetIns().Transaction(func(tx *gorm.DB) error { _, _, e := f.s.ApplyP4CopyCurrencyBonus(tx, now, math.MaxInt32, 1); return e }); err == nil {
		t.Fatal("currency overflow")
	}
	if err = db.GetIns().Model(&cfg).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	tabs, err = f.s.P4CopyBenefitsTabs(f.ctx, now)
	if err != nil || len(tabs) != 0 {
		t.Fatal("disabled copy tab shown", err)
	}
	err = db.GetIns().Transaction(func(tx *gorm.DB) error {
		var err error
		coin, _, err = f.s.ApplyP4CopyCurrencyBonus(tx, now, 100, 20)
		return err
	})
	if err != nil || coin != 100 {
		t.Fatal("closed operating gate still multiplies", err)
	}
	// 窗口排除截止、分钟和星期不匹配；原配置在没有冻结的新实例中校验。
	cfg.ID = "p4:copy:boundary"
	cfg.Frozen = nil
	cfg.Enabled = true
	cfg.CurrencyFactor = 2
	cfg.StartAt = now.Add(-time.Minute).UnixMilli()
	cfg.EndAt = now.Add(time.Hour).UnixMilli()
	local := now.In(time.FixedZone("UTC+8", 8*3600))
	cfg.WeekdayMask = 1 << uint(local.Weekday())
	cfg.StartMinute = int32(local.Hour() * 60)
	cfg.EndMinute = cfg.StartMinute + 60
	if err = db.GetIns().Create(&cfg).Error; err != nil {
		t.Fatal(err)
	}
	for _, at := range []time.Time{time.UnixMilli(cfg.StartAt).Add(-time.Millisecond), time.UnixMilli(cfg.EndAt), now.AddDate(0, 0, 1)} {
		err = db.GetIns().Transaction(func(tx *gorm.DB) error {
			var err error
			coin, _, err = f.s.ApplyP4CopyCurrencyBonus(tx, at, 100, 20)
			return err
		})
		if err != nil || coin != 100 {
			t.Fatal("window leak", at, err)
		}
	}
	second := cfg
	second.ID = "p4:copy:overlap"
	if err = db.GetIns().Create(&second).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Transaction(func(tx *gorm.DB) error { _, _, e := f.s.ApplyP4CopyCurrencyBonus(tx, now, 100, 20); return e }); err == nil {
		t.Fatal("overlap stacked bonuses")
	}
}

func TestP4CopyBenefitsRejectUnknownDropRollbackAndSourceConflict(t *testing.T) {
	f, _, now := p4CopySetup(t)
	if err := updateProfileWithTx(f.ctx, f.id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		base, err := f.s.GrantFixedReward(p, g, "item_005,5")
		if err != nil {
			return err
		}
		base.Wars = []*pb.Warrior{{Id: proto.String("hero_001")}}
		_, _, err = f.s.ApplyP4CopyDropBonus(tx, f.id, p, g, "copy:unknown", base, now)
		return err
	}); err == nil {
		t.Fatal("unknown drop multiplied")
	}
	p, g := f.profile(t)
	if simpleOwned(p, g, "item_005") != 0 || p1Count(t, &db.P4CopyBenefitGrant{}, "player_id=?", f.id) != 0 {
		t.Fatal("failure partially granted")
	}
	base := &pb.ResultObject{Simps: []*pb.SimpleItem{{Id: proto.String("item_005"), Num: proto.Int32(1)}}}
	if err := updateProfileWithTx(f.ctx, f.id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		_, _, err := f.s.ApplyP4CopyDropBonus(tx, f.id, p, g, "copy:collision", base, now)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	base.Simps[0].Num = proto.Int32(2)
	if err := updateProfileWithTx(f.ctx, f.id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		_, _, err := f.s.ApplyP4CopyDropBonus(tx, f.id, p, g, "copy:collision", base, now)
		return err
	}); err == nil {
		t.Fatal("source payload conflict accepted")
	}
	p, g = f.profile(t)
	if simpleOwned(p, g, "item_005") != 1 {
		t.Fatal("source conflict created extra item")
	}
}

// TestP4CopyBenefitsAllConfirmedOriginalTypes protects valid first-drop settlement.
func TestP4CopyBenefitsAllConfirmedOriginalTypes(t *testing.T) {
	f, _, now := p4CopySetup(t)
	var base *pb.ResultObject
	if err := updateProfileWithTx(f.ctx, f.id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		var err error
		base, err = f.s.GrantFixedReward(p, g, "item_005,5;frag_hero_001,3;MineItem_029,1;hero_001,1;hero_exp_001,1;equip_037,1;trea_001,1")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	replies := make(chan *pb.ResultObject, 8)
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := updateProfileWithTx(f.ctx, f.id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
				gain, resources, err := f.s.ApplyP4CopyDropBonus(tx, f.id, p, g, "copy:all-confirmed", base, now)
				if err == nil {
					if len(resources) != 7 {
						t.Error("full original resources missing", resources)
					}
					replies <- gain
				}
				return err
			})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	close(replies)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var result *pb.ResultObject
	for gain := range replies {
		if result == nil {
			result = gain
		}
		if !proto.Equal(result, gain) {
			t.Fatal("confirmed instance bonus rerolled")
		}
	}
	if len(result.GetEquips()) != 1 || result.GetEquips()[0].GetGuid() == base.GetEquips()[0].GetGuid() || len(result.GetTreas()) != 1 || result.GetTreas()[0].GetGuid() == base.GetTreas()[0].GetGuid() {
		t.Fatal("bonus copied original instance GUID")
	}
	if len(result.GetWars()) != 2 {
		t.Fatal("original hero/exp instances")
	}
	p, g := f.profile(t)
	if simpleOwned(p, g, "item_005") != 10 || simpleOwned(p, g, "frag_hero_001") != 6 {
		t.Fatal("simple source doubled exactly")
	}
	if len(g.Equipment) != 2 || len(g.Treasures) != 2 || len(g.Warriors) != 5 {
		t.Fatal("actual independent inventory", len(g.Equipment), len(g.Treasures), len(g.Warriors))
	}
	gemCount := int32(0)
	for _, gem := range g.Gems {
		if gem.GetId() == "MineItem_029" {
			gemCount += gem.GetNum()
		}
	}
	if gemCount != 2 {
		t.Fatal("MineItem source not doubled into real inventory")
	}
	sql, _ := db.GetIns().DB()
	if err := sql.Close(); err != nil {
		t.Fatal(err)
	}
	f.open(t)
	if err := updateProfileWithTx(f.ctx, f.id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		gain, _, err := f.s.ApplyP4CopyDropBonus(tx, f.id, p, g, "copy:all-confirmed", base, now)
		if err == nil && !proto.Equal(gain, result) {
			t.Fatal("cold full bonus changed")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if p1Count(t, &db.P4CopyBenefitGrant{}, "player_id=?", f.id) != 1 {
		t.Fatal("concurrent/cold bonus duplicated")
	}
	bad := proto.Clone(base).(*pb.ResultObject)
	bad.Equips[0].Guid = proto.String("")
	if err := updateProfileWithTx(f.ctx, f.id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		_, _, err := f.s.ApplyP4CopyDropBonus(tx, f.id, p, g, "copy:invalid-instance", bad, now)
		return err
	}); err == nil {
		t.Fatal("missing instance GUID accepted")
	}
}
