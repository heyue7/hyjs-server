package service

import (
	"bytes"
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"sync"
	"testing"
	"time"
)

func TestP13UnionFightRevivePointActualLiveNeutralCaptureContestEnemyColdGapAndIsolation(t *testing.T) {
	f, ids, first, clock, match := p13UFFieldSetup(t)
	now := func() time.Time { return time.UnixMilli(clock.Load()) }
	id := ids[0]
	for _, table := range []any{&db.P13UnionFightRevivePointSource{}, &db.P13UnionFightRevivePointOffer{}, &db.P13UnionFightRevivePointState{}, &db.P13UnionFightGuardAuditSource{}, &db.P13UnionFightBattleAuditSource{}, &db.P13UnionFightWarDrumSource{}, &db.P13UnionFightSpeedItemSource{}, &db.P13UnionFightHealAuraSource{}, &db.P13UnionFightItemSource{}, &db.P13UnionFightFactorySource{}, &db.P13UnionFightCarAttackSource{}, &db.P13UnionFightCarMotionSource{}, &db.P13UnionFightGateItemSource{}, &db.P13UnionFightCarItemSource{}} {
		if err := db.GetIns().Where("1=1").Delete(table).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.GetIns().Model(&db.P13UnionFightFieldPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "preparation_millis": 300000, "left_revive_point_id": 1, "right_revive_point_id": 3, "initial_guard_hp": 1000, "initial_gate_hp": 10000, "move_tolerance": 3, "source": "原地图/explicitlocalHP"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Create(&db.P13UnionFightRevivePointSource{Day: match.Day, PointID: 5, Enabled: true, MaxStepMillis: 1000, MaxGapMillis: 2000, PublishedAt: now().UnixMilli(), Source: "原多人数/争夺算法未知；明确local0左red/1右blue/countDiff/先中和/再推进/idleMove非Hook存活；maxStep1秒/maxGap2秒"}).Error; err != nil {
		t.Fatal(err)
	}
	var roster p13UFMatchRoster
	if json.Unmarshal(match.Roster, &roster) != nil {
		t.Fatal("roster invalid")
	}
	guild := p13GuildID(t, f, id, now())
	enemy := int64(0)
	for _, side := range [][]p13UFMatchFighter{roster.Left, roster.Right} {
		for _, row := range side {
			if row.GuildID != guild {
				enemy = row.PlayerID
				break
			}
		}
	}
	if enemy <= 0 {
		t.Fatal("actual opposite roster missing")
	}
	captured := map[int64][]proto.Message{}
	for _, player := range []int64{id, ids[11], enemy} {
		if _, err := f.s.p13UFFieldEnter(f.ctx, player, now()); err != nil {
			t.Fatal(err)
		}
		if _, err := f.s.p13UFFieldFrames(f.ctx, player, &pb.CSCurrentPosition{}, now()); err != nil {
			t.Fatal(err)
		}
		var profile db.Profile
		if err := db.GetIns().First(&profile, player).Error; err != nil {
			t.Fatal(err)
		}
		p := player
		t.Cleanup(f.s.RegisterP10Client(f.ctx, player, profile.Session, func(m proto.Message) error { captured[p] = append(captured[p], proto.Clone(m)); return nil }))
		if _, err := f.s.SyncP13UnionFightField(f.ctx, player, false); err != nil {
			t.Fatal(err)
		}
	}
	var actor db.P13UnionFightFieldPlayer
	db.GetIns().First(&actor, "match_id=? AND player_id=?", match.ID, id)
	camp := int32(actor.Camp)
	original, err := f.s.p13UFFieldOriginal(match, db.GetIns())
	if err != nil {
		t.Fatal(err)
	}
	point, _ := fieldPoint(original.table("UnionFightRevivePoint", "UFRPId", "5")["Position"])
	clock.Store(first + 100000)
	if _, err = f.s.p13UFFieldMove(f.ctx, id, &pb.CSMove{PointNew: point}, now()); err != nil {
		t.Fatal(err)
	}
	var field db.P13UnionFightField
	db.GetIns().First(&field, "match_id=?", match.ID)
	fieldSHA := field.SnapshotHash
	read := func() *pb.InstUFRevivePoint {
		t.Helper()
		var state db.P13UnionFightRevivePointState
		if err := db.GetIns().First(&state, "match_id=?", match.ID).Error; err != nil {
			t.Fatal(err)
		}
		var quote db.P13UnionFightRevivePointOffer
		if err := db.GetIns().First(&quote, "match_id=?", match.ID).Error; err != nil {
			t.Fatal(err)
		}
		frame, _, err := p13UFRevivePointRead(&state, &quote)
		if err != nil {
			t.Fatal(err)
		}
		return frame
	}
	tick := func(at int64) {
		t.Helper()
		clock.Store(at)
		if err := f.s.TickP13UnionFightRevivePoint(f.ctx, now()); err != nil {
			t.Fatal(err)
		}
	}
	tick(first + 100000)
	if frame := read(); frame.GetCamp() != 2 || frame.GetLeftOccupyProgress() != 0 || frame.GetRightOccupyProgress() != 0 {
		t.Fatal("first observation credited prior time", frame)
	}
	// 八个worker在相同边界仍只结算一次。广播收集加锁，使race验证真实并发业务。
	var mu sync.Mutex
	for _, player := range []int64{id, ids[11], enemy} {
		var profile db.Profile
		db.GetIns().First(&profile, player)
		p := player
		t.Cleanup(f.s.RegisterP10Client(f.ctx, player, profile.Session, func(m proto.Message) error {
			mu.Lock()
			defer mu.Unlock()
			captured[p] = append(captured[p], proto.Clone(m))
			return nil
		}))
	}
	clock.Store(first + 101000)
	var group sync.WaitGroup
	failures := make(chan error, 8)
	for i := 0; i < 8; i++ {
		group.Add(1)
		go func() { defer group.Done(); failures <- f.s.TickP13UnionFightRevivePoint(f.ctx, now()) }()
	}
	group.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	for second := int64(102); second <= 105; second++ {
		tick(first + second*1000)
	}
	frame := read()
	if frame.GetCamp() != camp {
		t.Fatal("actual single live5sec capture", frame, camp)
	}
	messages, err := f.s.p13UFFieldElements(f.ctx, id, []int64{frame.GetGuid()}, now())
	if err != nil {
		t.Fatal(err)
	}
	restored := false
	for _, m := range messages {
		if points, ok := m.(*pb.SCUFRevivePoint); ok && len(points.GetPoint()) == 1 && proto.Equal(frame, points.GetPoint()[0]) {
			restored = true
		}
	}
	if !restored {
		t.Fatal("point element refresh reverted immutable camp")
	}
	seenOccupy := false
	for _, m := range captured[id] {
		if out, ok := m.(*pb.SCUFRevivePointOccupy); ok && out.GetId() == 5 && out.GetCamp() == camp {
			seenOccupy = true
		}
	}
	if !seenOccupy {
		t.Fatal("original id5 occupy frame missing")
	}
	if _, err = f.s.p13UFFieldMove(f.ctx, enemy, &pb.CSMove{PointNew: point}, now()); err != nil {
		t.Fatal(err)
	}
	tick(first + 105100)
	before := proto.Clone(read())
	tick(first + 106100)
	if !proto.Equal(before, read()) {
		t.Fatal("equal camps progressed contested point")
	}
	outside := proto.Clone(point).(*pb.PointNew)
	outside.X = proto.Float32(point.GetX() + 10)
	clock.Store(first + 109000)
	if _, err = f.s.p13UFFieldMove(f.ctx, id, &pb.CSMove{PointNew: outside}, now()); err != nil {
		t.Fatal(err)
	}
	tick(first + 109000)
	for second := int64(110); second <= 119; second++ {
		tick(first + second*1000)
	}
	frame = read()
	if frame.GetCamp() != 1-camp {
		t.Fatal("enemy neutralize pluscapture", frame)
	}
	clock.Store(first + 121000)
	if _, err = f.s.p13UFFieldMove(f.ctx, id, &pb.CSMove{PointNew: point}, now()); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p13UFFieldMove(f.ctx, enemy, &pb.CSMove{PointNew: outside}, now()); err != nil {
		t.Fatal(err)
	}
	tick(first + 121000)
	tick(first + 122000)
	partial := proto.Clone(read())
	if err = db.GetIns().Model(&db.P13UnionFightFieldPolicy{}).Where("id=1").Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	tick(first + 123000)
	if err = db.GetIns().Model(&db.P13UnionFightFieldPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	tick(first + 124000)
	if !proto.Equal(partial, read()) {
		t.Fatal("reopened gate credited closed observation")
	}
	if err = db.GetIns().Model(&db.P13UnionFightRevivePointSource{}).Where("day=?", match.Day).Updates(map[string]any{"enabled": false, "max_step_millis": 1, "max_gap_millis": 60000}).Error; err != nil {
		t.Fatal(err)
	}
	sql, _ := db.GetIns().DB()
	sql.Close()
	f.open(t)
	frameBefore, _ := proto.Marshal(read())
	tick(first + 150000)
	frameAfter, _ := proto.Marshal(read())
	if !bytes.Equal(frameBefore, frameAfter) {
		t.Fatal("cold long gap awarded unobserved progress")
	}
	db.GetIns().First(&field, "match_id=?", match.ID)
	if field.SnapshotHash != fieldSHA {
		t.Fatal("occupy changed immutable field source")
	}
	clock.Store(match.EndAt)
	if err = f.s.TickP13UnionFightRevivePoint(f.ctx, now()); err != nil {
		t.Fatal(err)
	}
	atEnd, _ := proto.Marshal(read())
	if !bytes.Equal(frameAfter, atEnd) {
		t.Fatal("end changed captured camp")
	}
	var count int64
	db.GetIns().Model(&db.P13UnionFightRevivePointState{}).Where("match_id<>?", match.ID).Count(&count)
	if count != 0 {
		t.Fatal("other unbuiltmatch occupation leaked", count)
	}
}
