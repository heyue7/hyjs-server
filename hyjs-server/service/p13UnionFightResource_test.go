package service

import (
	"encoding/json"
	"fmt"
	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"sync"
	"testing"
	"time"
)

// TestP13UnionFightResource 覆盖真实原匹配、四类显式矿源、3秒/5次/50容量、上交距离/三倍战争贡献及冷恢复。
func TestP13UnionFightResourceActualSourcesMiningLeaseCapacityDepositRestartAndPermission(t *testing.T) {
	f, ids, first, clock, match := p13UFFieldSetup(t)
	now := time.UnixMilli(clock.Load())
	if err := db.GetIns().Model(&db.P13UnionFightFieldPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "preparation_millis": 300000, "left_revive_point_id": 1, "right_revive_point_id": 3, "initial_guard_hp": 1000, "initial_gate_hp": 10000, "move_tolerance": 3, "source": "隔离资源完整流程；明确本地出生/HP"}).Error; err != nil {
		t.Fatal(err)
	}
	original, err := f.s.p13UFFieldOriginal(match, db.GetIns())
	if err != nil {
		t.Fatal(err)
	}
	positions := map[int64]*pb.PointNew{}
	for camp, npc := range map[int64]string{0: "2", 1: "5"} {
		point, err := fieldPoint(original.table("UnionFightNpc", "UFNId", npc)["Position"])
		if err != nil {
			t.Fatal(err)
		}
		positions[camp] = point
		for i, yield := range []int64{2, 5, 20, 20} {
			source := db.P13UnionFightResourceSource{Day: match.Day, SourceKey: fmt.Sprintf("fixture-camp%d-type%d", camp, i+1), Enabled: true, ResourceID: fmt.Sprint(i + 1), X: point.GetX(), Y: point.GetY(), Z: point.GetZ(), RewardRes: yield, PublishedAt: now.UnixMilli(), Source: "隔离明确发布：原ResourceID1..4；本地位置为原回收NPC2/5位置，本地单次产量2/5/20/20；不声称原spawn"}
			if err = db.GetIns().Create(&source).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	enter := func(id int64) {
		t.Helper()
		if _, err := f.s.p13UFFieldEnter(f.ctx, id, time.UnixMilli(clock.Load())); err != nil {
			t.Fatal(err)
		}
	}
	enter(ids[0])
	enter(ids[10])
	var actor db.P13UnionFightFieldPlayer
	read := func() *pb.InstUFPlayer {
		t.Helper()
		if err := db.GetIns().First(&actor, "match_id=? AND player_id=?", match.ID, ids[0]).Error; err != nil {
			t.Fatal(err)
		}
		frame, err := p13UFFieldReadActor(&actor)
		if err != nil {
			t.Fatal(err)
		}
		return frame
	}
	frame := read()
	camp := actor.Camp
	point := positions[camp]
	npc := "2"
	if camp == 1 {
		npc = "5"
	}
	node := func(kind int) db.P13UnionFightResourceNode {
		t.Helper()
		var row db.P13UnionFightResourceNode
		guid := p13UFFieldGUID(match.ID, "resource", fmt.Sprintf("fixture-camp%d-type%d", camp, kind))
		if err := db.GetIns().First(&row, "match_id=? AND guid=?", match.ID, guid).Error; err != nil {
			t.Fatal(err)
		}
		return row
	}
	one := node(1)
	if err = db.GetIns().Model(&db.P13UnionFightResourceNode{}).Where("match_id=? AND guid=?", match.ID, one.GUID).Update("x", one.X+1).Error; err != nil {
		t.Fatal(err)
	}
	corrupt := node(1)
	if _, err = p13UFResourcePoint(&corrupt); err == nil {
		t.Fatal("published node position drift passed source SHA")
	}
	if err = db.GetIns().Model(&db.P13UnionFightResourceNode{}).Where("match_id=? AND guid=?", match.ID, one.GUID).Update("x", one.X).Error; err != nil {
		t.Fatal(err)
	}
	if one.GatherMillis != 3000 || one.TotalUses != 5 || one.RemainingUses != 5 {
		t.Fatal("original node3s/5uses", one)
	}
	if _, err = f.s.p13UFResourceBegin(f.ctx, ids[0], &pb.CSUFPlayerCollect{ResGuid: proto.Int64(one.GUID)}, now); err == nil {
		t.Fatal("Ready accepted collect")
	}
	clock.Store(first + 20000)
	if _, err = f.s.p13UFResourceBegin(f.ctx, ids[0], &pb.CSUFPlayerCollect{ResGuid: proto.Int64(one.GUID)}, time.UnixMilli(clock.Load())); err == nil {
		t.Fatal("loading accepted collect")
	}
	messages, err := f.s.p13UFFieldFrames(f.ctx, ids[0], &pb.CSCurrentPosition{}, time.UnixMilli(clock.Load()))
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, m := range messages {
		if x, ok := m.(*pb.SCUFResourcePoint); ok {
			count = len(x.Point)
			if len(x.Local) != count {
				t.Fatal("parallel resource position")
			}
		}
	}
	if count != 8 {
		t.Fatal("explicit all four original resource types for both published positions", count)
	}
	move := func(p *pb.PointNew) {
		t.Helper()
		clock.Add(20000)
		if _, err := f.s.p13UFFieldMove(f.ctx, ids[0], &pb.CSMove{PointNew: p}, time.UnixMilli(clock.Load())); err != nil {
			t.Fatal(err)
		}
	}
	move(point)
	// 第二真实同阵营首发也完成Current并到同点，busy拒绝才验证矿点独占而非距离/加载拒绝。
	if _, err = f.s.p13UFFieldFrames(f.ctx, ids[10], &pb.CSCurrentPosition{}, time.UnixMilli(clock.Load())); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p13UFFieldMove(f.ctx, ids[10], &pb.CSMove{PointNew: point}, time.UnixMilli(clock.Load())); err != nil {
		t.Fatal(err)
	}
	collect := func(guid int64) {
		t.Helper()
		reply, err := f.s.p13UFResourceBegin(f.ctx, ids[0], &pb.CSUFPlayerCollect{ResGuid: proto.Int64(guid)}, time.UnixMilli(clock.Load()))
		if err != nil {
			t.Fatal(err)
		}
		x := reply[0].(*pb.SCUFPlayerCollect)
		if x.CollectTime[0] != 3 || x.ResGuid[0] != guid {
			t.Fatal("original3second start", x)
		}
	}
	collect(one.GUID)
	if _, err = f.s.p13UFResourceBegin(f.ctx, ids[10], &pb.CSUFPlayerCollect{ResGuid: proto.Int64(one.GUID)}, time.UnixMilli(clock.Load())); err == nil {
		t.Fatal("other miner acquired busy lease")
	}
	clock.Add(2999)
	if err = f.s.TickP13UnionFightResource(f.ctx, time.UnixMilli(clock.Load())); err != nil {
		t.Fatal(err)
	}
	if read().GetOwnRes().GetResourceNum() != 0 {
		t.Fatal("early mining resource")
	}
	// 冷库后重新读原并行采集帧，剩余1毫秒以0.001秒回给原HUD。
	sql, _ := db.GetIns().DB()
	sql.Close()
	f.open(t)
	if err = f.s.InitializeP13Guild(f.ctx); err != nil {
		t.Fatal(err)
	}
	restored, err := f.s.p13UFFieldFrames(f.ctx, ids[0], &pb.CSCurrentPosition{}, time.UnixMilli(clock.Load()))
	if err != nil {
		t.Fatal(err)
	}
	recovered := false
	for _, m := range restored {
		if x, ok := m.(*pb.SCUFPlayerCollect); ok {
			recovered = x.CollectTime[0] == 0.001 && x.ResGuid[0] == one.GUID
		}
	}
	if !recovered {
		t.Fatal("cold exact mining deadline HUD")
	}
	clock.Add(1)
	tick := func() {
		t.Helper()
		if err := f.s.TickP13UnionFightResource(f.ctx, time.UnixMilli(clock.Load())); err != nil {
			t.Fatal(err)
		}
	}
	if f.mysql.Database != "" {
		var wg sync.WaitGroup
		errs := make(chan error, 8)
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); errs <- f.s.TickP13UnionFightResource(f.ctx, time.UnixMilli(clock.Load())) }()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
	} else {
		tick()
	}
	if read().GetOwnRes().GetResourceNum() != 2 || node(1).RemainingUses != 4 {
		t.Fatal("cold/concurrent completion must grant exactly once")
	}
	tick()
	if read().GetOwnRes().GetResourceNum() != 2 {
		t.Fatal("duplicate completion")
	}
	collect(one.GUID)
	clock.Add(1000)
	move(point)
	if read().GetState() != 0 || node(1).RemainingUses != 4 || node(1).BusyTaskID != "" {
		t.Fatal("valid manual movement did not cancel lease without yield/use")
	}
	tick()
	for i := 0; i < 4; i++ {
		collect(one.GUID)
		clock.Add(3000)
		tick()
	}
	if read().GetOwnRes().GetResourceNum() != 10 || node(1).RemainingUses != 0 {
		t.Fatal("original5times exhausted")
	}
	if _, err = f.s.p13UFResourceBegin(f.ctx, ids[0], &pb.CSUFPlayerCollect{ResGuid: proto.Int64(one.GUID)}, time.UnixMilli(clock.Load())); err == nil {
		t.Fatal("exhausted regenerated")
	}
	// 同一HTTP原时间戳并发上交只消费一次真实矿石，战争贡献与阵营资源都来自采集完成。
	deposit := func(stamp int64) *pb.SCUFPlayerUpResource {
		t.Helper()
		reply, err := f.s.p13UFResourceDeposit(f.ctx, ids[0], &pb.CSUFPlayerUpResource{NpcId: proto.String(npc)}, stamp, time.UnixMilli(clock.Load()))
		if err != nil {
			t.Fatal(err)
		}
		return reply[0].(*pb.SCUFPlayerUpResource)
	}
	if f.mysql.Database != "" {
		var wg sync.WaitGroup
		errs := make(chan error, 8)
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := f.s.p13UFResourceDeposit(f.ctx, ids[0], &pb.CSUFPlayerUpResource{NpcId: proto.String(npc)}, 900, time.UnixMilli(clock.Load()))
				errs <- err
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatal("concurrent exact deposit receipt", err)
			}
		}
	}
	deposited := deposit(900)
	if deposited.GetResourceNum() != 10 || deposited.GetPlayerReward() != 30 || read().GetOwnToken() != 30 || read().GetOwnRes().GetResourceNum() != 0 {
		t.Fatal("actual deposit source/conversion", deposited)
	}
	if !proto.Equal(deposited, deposit(900)) {
		t.Fatal("HTTP exact receipt replay")
	}
	if _, err = f.s.p13UFResourceDeposit(f.ctx, ids[0], &pb.CSUFPlayerUpResource{NpcId: proto.String(npc)}, 0, time.UnixMilli(clock.Load())); err == nil {
		t.Fatal("TCP duplicate minted contribution")
	}
	two, three := node(2), node(3)
	for _, guid := range []int64{three.GUID, three.GUID, two.GUID, two.GUID} {
		collect(guid)
		clock.Add(3000)
		tick()
	}
	if read().GetOwnRes().GetResourceNum() != 50 {
		t.Fatal("full capacity50")
	}
	if _, err = f.s.p13UFResourceBegin(f.ctx, ids[0], &pb.CSUFPlayerCollect{ResGuid: proto.Int64(three.GUID)}, time.UnixMilli(clock.Load())); err == nil {
		t.Fatal("capacity overflow")
	}
	far := proto.Clone(point).(*pb.PointNew)
	far.X = proto.Float32(point.GetX() + 4)
	move(far)
	if _, err = f.s.p13UFResourceDeposit(f.ctx, ids[0], &pb.CSUFPlayerUpResource{NpcId: proto.String(npc)}, 901, time.UnixMilli(clock.Load())); err == nil {
		t.Fatal("recycle3meter boundary")
	}
	if read().GetOwnRes().GetResourceNum() != 50 {
		t.Fatal("failed deposit consumed inventory")
	}
	move(point)
	deposit(901)
	if read().GetOwnToken() != 180 {
		t.Fatal("cumulative exact war contribution")
	}
	var field db.P13UnionFightField
	db.GetIns().First(&field, "match_id=?", match.ID)
	var snapshot p13UFFieldSnapshot
	if json.Unmarshal(field.Snapshot, &snapshot) != nil || snapshot.ResourceConfig.Capacity != 50 {
		t.Fatal("frozen original capacity")
	}
	var campRow db.P13UnionFightCampResource
	db.GetIns().First(&campRow, "match_id=? AND guild_id=?", match.ID, actor.GuildID)
	if campRow.Amount != 60 {
		t.Fatal("actual camp resources", campRow)
	}
	// 原样初始化投影本场资源，当前表/已发布源漂移不能重造冻结矿点。
	db.GetIns().Model(&db.P13UnionFightResourceSource{}).Where("day=?", match.Day).Update("reward_res", 49)
	after, err := f.s.p13UFFieldFrames(f.ctx, ids[0], &pb.CSUFInit{}, time.UnixMilli(clock.Load()))
	if err != nil {
		t.Fatal(err)
	}
	seen := false
	for _, m := range after {
		if x, ok := m.(*pb.SCUnionFightTransfer); ok {
			for _, u := range x.UnionInfos {
				if u.GetUnionId() == actor.GuildID {
					seen = u.GetUnionResource().GetResourceNum() == 60
				}
			}
		}
	}
	if !seen {
		t.Fatal("restored camp actual resources")
	}
	collect(three.GUID)
	if _, err = f.s.p13UFFieldLeave(f.ctx, ids[0], &pb.CSTransferScene{Target: proto.String("town_main")}, time.UnixMilli(clock.Load())); err != nil {
		t.Fatal(err)
	}
	if node(3).BusyTaskID != "" {
		t.Fatal("exit left mining lease")
	}
	tick()
	frame = read()
	if frame.GetOwnRes().GetResourceNum() != 0 || frame.GetState() != 0 {
		t.Fatal("exit granted resource")
	}
	// 已在合法时段到期的实际任务，服务停至End+4秒仍先完成，再按原规则退场；普通钱包没有归奖。
	four := node(4)
	clock.Store(first + 290000)
	if _, err = f.s.p13UFResourceBegin(f.ctx, ids[10], &pb.CSUFPlayerCollect{ResGuid: proto.Int64(four.GUID)}, time.UnixMilli(clock.Load())); err != nil {
		t.Fatal(err)
	}
	sql, _ = db.GetIns().DB()
	sql.Close()
	f.open(t)
	if err = f.s.InitializeP13Guild(f.ctx); err != nil {
		t.Fatal(err)
	}
	clock.Store(first + 304001)
	if err = f.s.TickP13UnionFightField(f.ctx, time.UnixMilli(clock.Load())); err != nil {
		t.Fatal(err)
	}
	var buddy db.P13UnionFightFieldPlayer
	if err = db.GetIns().First(&buddy, "match_id=? AND player_id=?", match.ID, ids[10]).Error; err != nil {
		t.Fatal(err)
	}
	late, err := p13UFFieldReadActor(&buddy)
	if err != nil {
		t.Fatal(err)
	}
	if late.GetOwnRes().GetResourceNum() != 20 || buddy.LeftAt != clock.Load() || node(4).RemainingUses != 4 {
		t.Fatal("due insideFight lost on cold recovery pastEnd", buddy.LeftAt, late)
	}
}
