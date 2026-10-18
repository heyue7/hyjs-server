package service

import (
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"math"
	"sync"
	"testing"
	"time"
)

func TestP13UnionFightFactoryCycleActual120SecondIndependentCarsColdMotionAttackConcurrentAndEnd(t *testing.T) {
	f, ids, first, clock, match := p13UFFieldSetup(t)
	now := func() time.Time { return time.UnixMilli(clock.Load()) }
	for _, model := range []any{&db.P13UnionFightCarMotionSource{}, &db.P13UnionFightCarMotionOffer{}, &db.P13UnionFightCarMotionState{}, &db.P13UnionFightFactorySource{}, &db.P13UnionFightCarItemSource{}, &db.P13UnionFightCarItemOffer{}, &db.P13UnionFightCarAttackSource{}, &db.P13UnionFightCarAttackOffer{}, &db.P13UnionFightCarAttackState{}, &db.P13UnionFightFactoryCycleSource{}, &db.P13UnionFightFactoryCycleOffer{}, &db.P13UnionFightFactoryCycleState{}, &db.P13UnionFightCarGeneration{}} {
		if err := db.GetIns().Where("1=1").Delete(model).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.GetIns().Model(&db.P13UnionFightFieldPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "preparation_millis": 300000, "left_revive_point_id": 1, "right_revive_point_id": 3, "initial_guard_hp": 1000, "initial_gate_hp": 10000, "move_tolerance": 3, "source": "隔离实际路径/本地初始HP"}).Error; err != nil {
		t.Fatal(err)
	}
	for factory := int64(1); factory <= 2; factory++ {
		if err := db.GetIns().Create(&db.P13UnionFightFactorySource{Day: match.Day, FactoryID: factory, Enabled: true, ChariotHP: 1000, PublishedAt: now().UnixMilli(), Source: "实际timer本地HP1000"}).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.GetIns().Create(&db.P13UnionFightCarMotionSource{Day: match.Day, FactoryID: factory, Enabled: true, Speed: 2, PublishedAt: now().UnixMilli(), Source: "原速度未知，隔离具名local2米/秒"}).Error; err != nil {
			t.Fatal(err)
		}
	}

	t.Cleanup(func() { db.GetIns().Where("1=1").Delete(&db.P13UnionFightFactoryCycleSource{}) })
	for factory := int64(1); factory <= 2; factory++ {
		if err := db.GetIns().Create(&db.P13UnionFightFactoryCycleSource{Day: match.Day, FactoryID: factory, Enabled: true, MaxGenerations: 2, MaxAlive: 2, PublishedAt: now().UnixMilli(), Source: "原产能上限未知/local每厂2代2存活，原120s"}).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.GetIns().Create(&db.P13UnionFightCarAttackSource{Day: match.Day, FactoryID: factory, Enabled: true, HPPercent: 1, Radius: 3.5, PublishedAt: now().UnixMilli(), Source: "原伤害/半径未知/local1%+3.5m"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.s.p13UFFieldEnter(f.ctx, ids[0], now()); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p13UFFieldFrames(f.ctx, ids[0], &pb.CSCurrentPosition{}, now()); err != nil {
		t.Fatal(err)
	}
	clock.Store(first + 120000)
	if err := f.s.TickP13UnionFightFactory(f.ctx, now()); err != nil {
		t.Fatal(err)
	}
	count := func(want int64) {
		t.Helper()
		var count int64
		if err := db.GetIns().Model(&db.P13UnionFightCarGeneration{}).Where("match_id=?", match.ID).Count(&count).Error; err != nil || count != want {
			t.Fatal("actual independent car generation count", count, want, err)
		}
	}
	count(0)
	db.GetIns().Model(&db.P13UnionFightFactoryCycleSource{}).Where("day=?", match.Day).Updates(map[string]any{"enabled": false, "max_generations": 99, "max_alive": 1})
	clock.Store(first + 239999)
	if err := f.s.TickP13UnionFightFactory(f.ctx, now()); err != nil {
		t.Fatal(err)
	}
	count(0)
	clock.Store(first + 240000)
	if f.mysql.Database != "" {
		var wg sync.WaitGroup
		errs := make(chan error, 8)
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); errs <- f.s.TickP13UnionFightFactory(f.ctx, now()) }()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
	} else if err := f.s.TickP13UnionFightFactory(f.ctx, now()); err != nil {
		t.Fatal(err)
	}
	count(2)
	rows, err := f.s.p13UFCarScopeRows(db.GetIns(), match.ID, nil)
	if err != nil || len(rows) != 4 {
		t.Fatal("first cars and new cars must coexist", len(rows), err)
	}
	seen := map[int64]bool{}
	for _, row := range rows {
		car, err := p13UFCarItemRead(&row)
		if err != nil || seen[car.GetGuid()] || car.GetFactoryGuid() != row.GUID {
			t.Fatal("independent GUID preserves original factory", err)
		}
		seen[car.GetGuid()] = true
		if row.CarGeneration == 2 && row.CompletedAt != first+240000 {
			t.Fatal("actual second 120s window", row)
		}
	}
	clock.Store(first + 250000)
	if err = f.s.TickP13UnionFightCarMotion(f.ctx, now()); err != nil {
		t.Fatal(err)
	}
	var states []db.P13UnionFightCarMotionState
	if err = db.GetIns().Where("match_id=?", match.ID).Find(&states).Error; err != nil || len(states) != 4 {
		t.Fatal("independent mutable motion", len(states), err)
	}
	for _, row := range rows {
		if row.CarGeneration != 2 {
			continue
		}
		var state db.P13UnionFightCarMotionState
		if err = db.GetIns().First(&state, "match_id=? AND factory_guid=?", match.ID, p13UFCarStateGUID(&row)).Error; err != nil || state.Distance != 20 || state.CarGUID != p13UFCarInstanceGUID(&row) {
			t.Fatal("new generation original motion frozen2", state, err)
		}
	}
	sql, _ := db.GetIns().DB()
	sql.Close()
	f.open(t)
	if err = f.s.InitializeP13Guild(f.ctx); err != nil {
		t.Fatal(err)
	}
	frames, err := f.s.p13UFFactoryCars(db.GetIns(), match, nil)
	if err != nil || len(frames[0].(*pb.SCUFCaropt).Cariot) != 4 {
		t.Fatal("cold preserves all actual cars", err)
	}
	clock.Store(first + 280000)
	if err = f.s.TickP13UnionFightCarMotion(f.ctx, now()); err != nil {
		t.Fatal(err)
	}
	if err = f.s.TickP13UnionFightCarAttack(f.ctx, now()); err != nil {
		t.Fatal(err)
	}
	clock.Add(600)
	if err = f.s.TickP13UnionFightCarAttack(f.ctx, now()); err != nil {
		t.Fatal(err)
	}
	var attacks []db.P13UnionFightCarAttackState
	if err = db.GetIns().Where("match_id=?", match.ID).Find(&attacks).Error; err != nil {
		t.Fatal(err)
	}
	var gate db.P13UnionFightGateState
	if err = db.GetIns().First(&gate, "match_id=? AND guid=?", match.ID, p13UFFieldGUID(match.ID, "gate", "6")).Error; err != nil {
		t.Fatal(err)
	}
	gateFrame, err := p13UFGateRead(&gate)
	if err != nil || math.Abs(gateFrame.GetHp()-9800) > 1e-8 {
		t.Fatal("two independent actual cars release one percent each", gateFrame, err)
	}
	count(2)
	clock.Store(match.EndAt)
	if err = f.s.TickP13UnionFightFactory(f.ctx, now()); err != nil {
		t.Fatal(err)
	}
	count(2)
}
