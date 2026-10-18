package service

import (
	"math"
	"strconv"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// TestP13UnionFightCarMotionActual 验证实际120秒首车的原导航、同一持久目标位置、冻结来源及冷恢复。
func TestP13UnionFightCarMotionActualOriginalPathNaturalTimerTargetColdRaceAndSHA(t *testing.T) {
	f, ids, first, clock, match := p13UFFieldSetup(t)
	now := func() time.Time { return time.UnixMilli(clock.Load()) }
	for _, model := range []any{&db.P13UnionFightCarMotionSource{}, &db.P13UnionFightCarMotionOffer{}, &db.P13UnionFightCarMotionState{}, &db.P13UnionFightFactorySource{}, &db.P13UnionFightCarItemSource{}, &db.P13UnionFightCarItemOffer{}} {
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
	if _, err := f.s.p13UFFieldEnter(f.ctx, ids[0], now()); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p13UFFieldFrames(f.ctx, ids[0], &pb.CSCurrentPosition{}, now()); err != nil {
		t.Fatal(err)
	}
	clock.Store(first + 119999)
	if err := f.s.TickP13UnionFightFactory(f.ctx, now()); err != nil {
		t.Fatal(err)
	}
	var count int64
	db.GetIns().Model(&db.P13UnionFightFactoryState{}).Where("match_id=? AND completed_at>0", match.ID).Count(&count)
	if count != 0 {
		t.Fatal("early first cars")
	}
	clock.Store(first + 120000)
	if err := f.s.TickP13UnionFightFactory(f.ctx, now()); err != nil {
		t.Fatal(err)
	}
	var actor db.P13UnionFightFieldPlayer
	if err := db.GetIns().First(&actor, "match_id=? AND player_id=?", match.ID, ids[0]).Error; err != nil {
		t.Fatal(err)
	}
	var target db.P13UnionFightFactoryState
	if err := db.GetIns().First(&target, "match_id=? AND completed_at>0 AND camp<>?", match.ID, actor.Camp).Error; err != nil {
		t.Fatal(err)
	}
	var offer db.P13UnionFightCarMotionOffer
	if err := db.GetIns().First(&offer, "match_id=? AND factory_guid=?", match.ID, target.GUID).Error; err != nil {
		t.Fatal(err)
	}
	points, total, err := p13UFCarMotionPath(&offer)
	if err != nil {
		t.Fatal(err)
	}
	clock.Store(first + 130000)
	if f.mysql.Database != "" {
		var wg sync.WaitGroup
		errs := make(chan error, 8)
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); errs <- f.s.TickP13UnionFightCarMotion(f.ctx, now()) }()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
	} else if err := f.s.TickP13UnionFightCarMotion(f.ctx, now()); err != nil {
		t.Fatal(err)
	}
	var state db.P13UnionFightCarMotionState
	if err := db.GetIns().First(&state, "match_id=? AND factory_guid=?", match.ID, target.GUID).Error; err != nil {
		t.Fatal(err)
	}
	if state.Distance != 20 || state.LastAt != clock.Load() {
		t.Fatal("worker advanced same clock more than once", state)
	}
	original, err := f.s.p13UFFieldOriginal(match, db.GetIns())
	if err != nil {
		t.Fatal(err)
	}
	point, err := f.s.p13UFCarMotionPoint(db.GetIns(), &target, original)
	if err != nil || !proto.Equal(point, p13UFCarMotionAt(points, 20)) {
		t.Fatal("actual original route point", point, err)
	}
	if _, err = f.s.p13UFFieldMove(f.ctx, ids[0], &pb.CSMove{PointNew: points[0]}, now()); err != nil {
		t.Fatal(err)
	}
	db.GetIns().First(&actor, "match_id=? AND player_id=?", match.ID, ids[0])
	quote := db.P13UnionFightCarItemOffer{Radius: 5}
	if _, _, _, err = f.s.p13UFCarItemTarget(db.GetIns(), match, &actor, &quote); err == nil {
		t.Fatal("bomb selected stale birth position after actual movement")
	}
	clock.Add(5000)
	if _, err = f.s.p13UFFieldMove(f.ctx, ids[0], &pb.CSMove{PointNew: point}, now()); err != nil {
		t.Fatal(err)
	}
	db.GetIns().First(&actor, "match_id=? AND player_id=?", match.ID, ids[0])
	selected, _, selectedPoint, err := f.s.p13UFCarItemTarget(db.GetIns(), match, &actor, &quote)
	if err != nil || selected.GUID != target.GUID || !proto.Equal(selectedPoint, point) {
		t.Fatal("actual moving car target", err)
	}
	db.GetIns().Model(&db.P13UnionFightCarMotionSource{}).Where("day=?", match.Day).Updates(map[string]any{"enabled": false, "speed": 99})
	clock.Store(first + 136000)
	if err = f.s.TickP13UnionFightCarMotion(f.ctx, now()); err != nil {
		t.Fatal(err)
	}
	sql, _ := db.GetIns().DB()
	sql.Close()
	f.open(t)
	if err = f.s.InitializeP13Guild(f.ctx); err != nil {
		t.Fatal(err)
	}
	frames, err := f.s.p13UFFactoryCars(db.GetIns(), match, map[int64]bool{p13UFFieldGUID(match.ID, "cariot", strconv.FormatInt(target.GUID, 10)): true})
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 1 || !proto.Equal(frames[0].(*pb.SCUFCaropt).Local[0], p13UFCarMotionAt(points, 32)) {
		t.Fatal("cold/frozen original path reset", frames)
	}
	clock.Store(first + int64(120000+math.Ceil(total/2*1000)) + 1)
	if clock.Load() >= match.EndAt {
		t.Fatal("isolated path exceeds test match")
	}
	if err = f.s.TickP13UnionFightCarMotion(f.ctx, now()); err != nil {
		t.Fatal(err)
	}
	db.GetIns().First(&state, "match_id=? AND factory_guid=?", match.ID, target.GUID)
	if !state.Arrived || state.Distance != total {
		t.Fatal("original path end clamp", state)
	}
	db.GetIns().Model(&db.P13UnionFightCarMotionState{}).Where("match_id=? AND factory_guid=?", match.ID, target.GUID).Update("state_hash", "bad")
	if _, err = f.s.p13UFFactoryCars(db.GetIns(), match, nil); err == nil {
		t.Fatal("corrupt path state displayed")
	}
}
