package service

import (
	"reflect"
	"sync"
	"testing"
	"time"

	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// TestP13UnionFightCarAttackActual 使用真正原120秒生产、原路线和原释放计时，伤害70%及半径3.5为具名本地来源。
func TestP13UnionFightCarAttackActualNaturalTimerOriginalReleaseColdConcurrentClampAndNoWinner(t *testing.T) {
	f, ids, first, clock, match := p13UFFieldSetup(t)
	now := func() time.Time { return time.UnixMilli(clock.Load()) }
	for _, model := range []any{&db.P13UnionFightCarMotionSource{}, &db.P13UnionFightCarMotionOffer{}, &db.P13UnionFightCarMotionState{}, &db.P13UnionFightCarAttackSource{}, &db.P13UnionFightCarAttackOffer{}, &db.P13UnionFightCarAttackState{}, &db.P13UnionFightFactorySource{}, &db.P13UnionFightGateItemSource{}, &db.P13UnionFightGateItemOffer{}, &db.P13UnionFightGateState{}, &db.P13UnionFightCarItemSource{}, &db.P13UnionFightCarItemOffer{}} {
		if err := db.GetIns().Where("1=1").Delete(model).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.GetIns().Model(&db.P13UnionFightFieldPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "preparation_millis": 300000, "left_revive_point_id": 1, "right_revive_point_id": 3, "initial_guard_hp": 1000, "initial_gate_hp": 10000, "move_tolerance": 3, "source": "原攻门实际实例/具名本地HP"}).Error; err != nil {
		t.Fatal(err)
	}
	for factory := int64(1); factory <= 2; factory++ {
		if err := db.GetIns().Create(&db.P13UnionFightFactorySource{Day: match.Day, FactoryID: factory, Enabled: true, ChariotHP: 1000, PublishedAt: now().UnixMilli(), Source: "实际120s首车/local1000HP"}).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.GetIns().Create(&db.P13UnionFightCarMotionSource{Day: match.Day, FactoryID: factory, Enabled: true, Speed: 20, PublishedAt: now().UnixMilli(), Source: "原速度未知/隔离local20米秒"}).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.GetIns().Create(&db.P13UnionFightCarAttackSource{Day: match.Day, FactoryID: factory, Enabled: true, HPPercent: 70, Radius: 3.5, PublishedAt: now().UnixMilli(), Source: "原攻击数额/范围未知/隔离local70%+3.5米"}).Error; err != nil {
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
	var factory db.P13UnionFightFactoryState
	if err := db.GetIns().First(&factory, "match_id=? AND factory_id=1", match.ID).Error; err != nil {
		t.Fatal(err)
	}
	car, err := p13UFCarItemRead(&factory)
	if err != nil {
		t.Fatal(err)
	}
	var offer db.P13UnionFightCarAttackOffer
	if err = db.GetIns().First(&offer, "match_id=? AND factory_guid=?", match.ID, factory.GUID).Error; err != nil {
		t.Fatal(err)
	}
	if offer.GateID != 6 || offer.CooldownMillis != 3500 || offer.ReleaseMillis != 600 {
		t.Fatal("actual original target/CD/release", offer)
	}
	clock.Store(first + 121000)
	if err = f.s.TickP13UnionFightCarMotion(f.ctx, now()); err != nil {
		t.Fatal(err)
	}
	if err = f.s.TickP13UnionFightCarAttack(f.ctx, now()); err != nil {
		t.Fatal(err)
	}
	var early int64
	db.GetIns().Model(&db.P13UnionFightCarAttackState{}).Where("match_id=? AND factory_guid=?", match.ID, factory.GUID).Count(&early)
	if early != 0 {
		t.Fatal("attacked before actual range")
	}
	clock.Store(first + 125000)
	if err = f.s.TickP13UnionFightCarMotion(f.ctx, now()); err != nil {
		t.Fatal(err)
	}
	if err = f.s.TickP13UnionFightCarAttack(f.ctx, now()); err != nil {
		t.Fatal(err)
	}
	readGate := func() *pb.InstUFGate {
		t.Helper()
		var row db.P13UnionFightGateState
		if err := db.GetIns().First(&row, "match_id=? AND guid=?", match.ID, offer.GateGUID).Error; err != nil {
			t.Fatal(err)
		}
		frame, err := p13UFGateRead(&row)
		if err != nil {
			t.Fatal(err)
		}
		return frame
	}
	if readGate().GetHp() != 10000 {
		t.Fatal("attack animation directly damaged before600ms")
	}
	clock.Store(first + 125599)
	if err = f.s.TickP13UnionFightCarAttack(f.ctx, now()); err != nil {
		t.Fatal(err)
	}
	if readGate().GetHp() != 10000 {
		t.Fatal("early original release")
	}
	sql, _ := db.GetIns().DB()
	sql.Close()
	f.open(t)
	if err = f.s.InitializeP13Guild(f.ctx); err != nil {
		t.Fatal(err)
	}
	frames, err := f.s.p13UFFactoryCars(db.GetIns(), match, map[int64]bool{car.GetGuid(): true})
	if err != nil || len(frames) != 2 || frames[1].(*pb.SCUFCariotAttackGate).GetGateGuid() != offer.GateGUID {
		t.Fatal("cold pending actual attack", err)
	}
	concurrent := func() {
		t.Helper()
		if f.mysql.Database == "" {
			if err := f.s.TickP13UnionFightCarAttack(f.ctx, now()); err != nil {
				t.Fatal(err)
			}
			return
		}
		var wg sync.WaitGroup
		errs := make(chan error, 8)
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); errs <- f.s.TickP13UnionFightCarAttack(f.ctx, now()) }()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	clock.Store(first + 125600)
	concurrent()
	if readGate().GetHp() != 3000 {
		t.Fatal("eight workers released more than one70% hit")
	}
	db.GetIns().Model(&db.P13UnionFightCarAttackSource{}).Where("day=?", match.Day).Updates(map[string]any{"enabled": false, "hp_percent": 1, "radius": 9})
	db.GetIns().Model(&db.P13UnionFightCarMotionSource{}).Where("day=?", match.Day).Updates(map[string]any{"enabled": false, "speed": 99})
	clock.Store(first + 128499)
	concurrent()
	var state db.P13UnionFightCarAttackState
	db.GetIns().First(&state, "match_id=? AND factory_guid=?", match.ID, factory.GUID)
	if state.PendingReleaseAt != 0 {
		t.Fatal("attack CD early")
	}
	clock.Store(first + 128500)
	concurrent()
	db.GetIns().First(&state, "match_id=? AND factory_guid=?", match.ID, factory.GUID)
	if state.PendingReleaseAt != first+129100 || state.Hits != 1 {
		t.Fatal("actual second original animation clock", state)
	}
	clock.Store(first + 129100)
	concurrent()
	if readGate().GetHp() != 0 {
		t.Fatal("frozen70% final clamped gate hit")
	}
	db.GetIns().First(&state, "match_id=? AND factory_guid=?", match.ID, factory.GUID)
	if !state.Completed || state.Hits != 2 {
		t.Fatal("terminal gate attack state", state)
	}
	frames, err = f.s.p13UFFactoryCars(db.GetIns(), match, map[int64]bool{car.GetGuid(): true})
	if err != nil || len(frames) != 2 || frames[1].(*pb.SCUFCariotComplete).GetGuid() != car.GetGuid() {
		t.Fatal("cold original terminal state", err)
	}
	if readGate().GetId() != 6 || readGate().GetHp() != 0 {
		t.Fatal("original gateID identity")
	}
	var after db.P13UnionFightMatch
	if err = db.GetIns().First(&after, "id=?", match.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after, *match) {
		t.Fatal("gate crash forged match winner")
	}
	db.GetIns().Model(&db.P13UnionFightCarAttackState{}).Where("match_id=? AND factory_guid=?", match.ID, factory.GUID).Update("state_hash", "bad")
	if _, err = f.s.p13UFFactoryCars(db.GetIns(), match, nil); err == nil {
		t.Fatal("corrupt attack SHA displayed")
	}
	clock.Store(match.EndAt)
	if err = f.s.TickP13UnionFightCarAttack(f.ctx, now()); err != nil {
		t.Fatal("End should not consume corrupt archived attack", err)
	}
}
