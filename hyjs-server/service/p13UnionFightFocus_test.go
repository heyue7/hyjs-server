package service

import (
	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"sync"
	"testing"
	"time"
)

// TestP13UnionFightFocus 真实声明职位/场内权限与原大地图点120秒绝对期限、精确回执、同阵营通信和恢复。
func TestP13UnionFightFocusActualChairmanFrozenAuthorityPrivateRawUI120sReceiptRestartExpiry(t *testing.T) {
	f, ids, first, clock, match := p13UFFieldSetup(t)
	if err := db.GetIns().Where("1=1").Delete(&db.P13UnionFightFocus{}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P13UnionFightFieldPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "preparation_millis": 300000, "left_revive_point_id": 1, "right_revive_point_id": 3, "initial_guard_hp": 1000, "initial_gate_hp": 10000, "move_tolerance": 3, "source": "隔离集结初始化；原固定复活点/明确非权威HP"}).Error; err != nil {
		t.Fatal(err)
	}
	now := time.UnixMilli(clock.Load())
	own := p13GuildID(t, f, ids[0], now)
	enemy := match.RightGuildID
	if enemy == own {
		enemy = match.LeftGuildID
	}
	var enemyMember db.P13GuildMember
	if err := db.GetIns().First(&enemyMember, "guild_id=?", enemy).Error; err != nil {
		t.Fatal(err)
	}
	other := enemyMember.PlayerID
	for _, id := range []int64{ids[0], ids[10], ids[11], other} {
		if _, err := f.s.p13UFFieldEnter(f.ctx, id, now); err != nil {
			t.Fatal(err)
		}
	}
	received := map[int64][]proto.Message{}
	for _, id := range []int64{ids[0], ids[10], other} {
		var row db.Profile
		if err := db.GetIns().First(&row, id).Error; err != nil {
			t.Fatal(err)
		}
		playerID := id
		cleanup := f.s.RegisterP10Client(f.ctx, id, row.Session, func(m proto.Message) error {
			received[playerID] = append(received[playerID], proto.Clone(m))
			return nil
		})
		t.Cleanup(cleanup)
		if _, err := f.s.SyncP13UnionFightField(f.ctx, id, false); err != nil {
			t.Fatal(err)
		}
	}
	received[ids[10]] = nil
	received[other] = nil
	request := &pb.CSUFLeaderFocus{Pos: &pb.PointNew{X: proto.Float32(200), Y: proto.Float32(30), Z: proto.Float32(0)}}
	for _, id := range []int64{ids[10], ids[11]} {
		if _, err := f.s.p13UFFocus(f.ctx, id, request, 500, now); err == nil {
			t.Fatal("ordinary/actual vice chairman set chairman-only focus", id)
		}
	}
	firstReply, err := f.s.p13UFFocus(f.ctx, ids[0], request, 500, now)
	if err != nil {
		t.Fatal(err)
	}
	set := firstReply[0].(*pb.SCUFLeaderFocus)
	if len(set.Pos) != 1 || !proto.Equal(set.Pos[0], request.Pos) || set.GetLastTime() != now.UnixMilli()+120000 {
		t.Fatal("raw map UI coordinates or absolute120seconds changed", set)
	}
	if len(received[ids[10]]) != 1 || !proto.Equal(received[ids[10]][0], set) || len(received[other]) != 0 {
		t.Fatal("focus did not remain original own-guild private scope", received)
	}
	for i := 0; i < 3; i++ {
		response, err := f.s.p13UFFocus(f.ctx, ids[0], request, 500, now)
		if err != nil || !proto.Equal(response[0], set) {
			t.Fatal("exact original receipt", err)
		}
	}
	if f.mysql.Database != "" {
		var workers sync.WaitGroup
		failures := make(chan error, 8)
		for i := 0; i < 8; i++ {
			workers.Add(1)
			go func() {
				defer workers.Done()
				_, err := f.s.p13UFFocus(f.ctx, ids[0], request, 500, now)
				failures <- err
			}()
		}
		workers.Wait()
		close(failures)
		for err := range failures {
			if err != nil {
				t.Fatal("eight concurrent exact focus frames", err)
			}
		}
	}
	if len(received[ids[10]]) != 1 {
		t.Fatal("exact retries repeated command broadcast")
	}
	conflict := &pb.CSUFLeaderFocus{Pos: &pb.PointNew{X: proto.Float32(201), Y: proto.Float32(30), Z: proto.Float32(0)}}
	if _, err = f.s.p13UFFocus(f.ctx, ids[0], conflict, 500, now); err == nil {
		t.Fatal("same stamp different point accepted")
	}
	if _, err = f.s.p13UFFocus(f.ctx, ids[0], conflict, 501, now); err == nil {
		t.Fatal("original120s cooldown bypassed")
	}
	if _, err = f.s.p13UFFieldHook(f.ctx, ids[0], &pb.CSUFOnHook{OnHook: proto.Int32(1)}, now); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p13UFFocus(f.ctx, ids[0], conflict, 502, now); err == nil {
		t.Fatal("original hooked focus operation accepted")
	}
	if _, err = f.s.p13UFFieldHook(f.ctx, ids[0], &pb.CSUFOnHook{}, now); err != nil {
		t.Fatal(err)
	}
	sql, _ := db.GetIns().DB()
	sql.Close()
	f.open(t)
	if err = f.s.InitializeP13Guild(f.ctx); err != nil {
		t.Fatal(err)
	}
	query, err := f.s.p13UFFocus(f.ctx, ids[10], &pb.CSUFLeaderFocusInfo{}, 0, now)
	if err != nil || !proto.Equal(query[0], set) {
		t.Fatal("restart did not restore exact raw focus deadline", err)
	}
	clock.Store(first + 60001)
	expired := time.UnixMilli(clock.Load())
	if err = f.s.TickP13UnionFightFocus(f.ctx, expired); err != nil {
		t.Fatal(err)
	}
	query, err = f.s.p13UFFocus(f.ctx, ids[10], &pb.CSUFFocusPointInfo{}, 0, expired)
	if err != nil || len(query[0].(*pb.SCUFFocusPointInfo).Pos) != 0 {
		t.Fatal("actual expiry did not clear original points", err)
	}
	setAgain, err := f.s.p13UFFocus(f.ctx, ids[0], conflict, 503, expired)
	if err != nil || setAgain[0].(*pb.SCUFLeaderFocus).GetLastTime() != expired.UnixMilli()+120000 {
		t.Fatal("new command after original cooldown", err)
	}
	// 原当时职位与当前职位共同授权；转会长不把冻结普通成员升为本场指挥者。
	p13GuildCall(t, f, ids[0], &pb.CSTransferChairman{PlayerId: proto.Int64(ids[10])}, 900, expired)
	if err = f.s.TickP13UnionFightFocus(f.ctx, expired); err != nil {
		t.Fatal(err)
	}
	query, err = f.s.p13UFFocus(f.ctx, ids[11], &pb.CSUFFocusPointInfo{}, 0, expired)
	if err != nil || len(query[0].(*pb.SCUFFocusPointInfo).Pos) != 0 {
		t.Fatal("demoted original commander did not lose focus authority", err)
	}
	if _, err = f.s.p13UFFocus(f.ctx, ids[10], request, 504, expired); err == nil {
		t.Fatal("current chairman without frozen authority accepted")
	}
	p13GuildCall(t, f, ids[10], &pb.CSTransferChairman{PlayerId: proto.Int64(ids[0])}, 901, expired)
	clock.Store(first + 180002)
	closeTime := time.UnixMilli(clock.Load())
	if _, err = f.s.p13UFFocus(f.ctx, ids[0], &pb.CSUFLeaderFocus{Pos: &pb.PointNew{}}, 505, closeTime); err != nil {
		t.Fatal("restored real/frozen chairman", err)
	}
	if err = db.GetIns().Model(&db.P13UnionFightFieldPolicy{}).Where("id=1").Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if err = f.s.TickP13UnionFightFocus(f.ctx, closeTime); err != nil {
		t.Fatal("closed field policy caused service lifecycle failure", err)
	}
}
