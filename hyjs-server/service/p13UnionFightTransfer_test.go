package service

import (
	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"sync"
	"testing"
	"time"
)

// TestP13UnionFightTransfer 覆盖原18点两阵营、原XZ半径/1秒/5秒、串行权限及冷库/重复完成。
func TestP13UnionFightTransferAll18OriginalTargetsTimersCurrentColdAndNoArbitraryPosition(t *testing.T) {
	f, ids, first, clock, match := p13UFFieldSetup(t)
	if err := db.GetIns().Model(&db.P13UnionFightFieldPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "preparation_millis": 300000, "left_revive_point_id": 1, "right_revive_point_id": 3, "initial_guard_hp": 1000, "initial_gate_hp": 10000, "move_tolerance": 3, "source": "原18传送点全量隔离；明确本地出生HP"}).Error; err != nil {
		t.Fatal(err)
	}
	now := func() time.Time { return time.UnixMilli(clock.Load()) }
	var opposing db.P13GuildMember
	own := p13GuildID(t, f, ids[0], now())
	enemy := match.RightGuildID
	if own == enemy {
		enemy = match.LeftGuildID
	}
	if err := db.GetIns().First(&opposing, "guild_id=?", enemy).Error; err != nil {
		t.Fatal(err)
	}
	players := []int64{ids[0], opposing.PlayerID}
	byCamp := map[int64]int64{}
	read := func(id int64) (db.P13UnionFightFieldPlayer, *pb.InstUFPlayer) {
		t.Helper()
		var actor db.P13UnionFightFieldPlayer
		if err := db.GetIns().First(&actor, "match_id=? AND player_id=?", match.ID, id).Error; err != nil {
			t.Fatal(err)
		}
		frame, err := p13UFFieldReadActor(&actor)
		if err != nil {
			t.Fatal(err)
		}
		return actor, frame
	}
	for _, id := range players {
		if _, err := f.s.p13UFFieldEnter(f.ctx, id, now()); err != nil {
			t.Fatal(err)
		}
		actor, _ := read(id)
		byCamp[actor.Camp] = id
	}
	original, err := f.s.p13UFFieldOriginal(match, db.GetIns())
	if err != nil {
		t.Fatal(err)
	}
	all := original.Tables["UnionFightTransfer"]
	if len(all) != 18 {
		t.Fatal("original full18", len(all))
	}
	firstTarget, _ := fieldPoint(all[0]["TransferEnd"])
	if _, err = f.s.p13UFTransferBegin(f.ctx, byCamp[0], &pb.CSUFBeginTransfer{TransferId: proto.String(all[0]["UFTransferId"]), Target: firstTarget}, now()); err == nil {
		t.Fatal("beforeCurrent accepted")
	}
	for _, id := range players {
		if _, err = f.s.p13UFFieldFrames(f.ctx, id, &pb.CSCurrentPosition{}, now()); err != nil {
			t.Fatal(err)
		}
	}
	total := 0
	for _, tr := range all {
		if tr["TransferType"] != "0" {
			continue
		}
		camp := int64(0)
		if tr["Camp"] == "1" {
			camp = 1
		}
		id := byCamp[camp]
		other := byCamp[1-camp]
		start, err := fieldPoint(tr["TransferStart"])
		if err != nil {
			t.Fatal(err)
		}
		target, err := fieldPoint(tr["TransferEnd"])
		if err != nil {
			t.Fatal(err)
		}
		clock.Add(20000)
		// 原入口算法仅XZ，同一水平入口即使角色Y为2也合法。
		start.Y = proto.Float32(2)
		if _, err = f.s.p13UFFieldMove(f.ctx, id, &pb.CSMove{PointNew: start}, now()); err != nil {
			t.Fatal(tr["UFTransferId"], err)
		}
		req := &pb.CSUFBeginTransfer{TransferId: proto.String(tr["UFTransferId"]), Target: target}
		if _, err = f.s.p13UFTransferBegin(f.ctx, other, req, now()); err == nil {
			t.Fatal("opposing camp used original transfer", tr)
		}
		bad := proto.Clone(req).(*pb.CSUFBeginTransfer)
		bad.Target.X = proto.Float32(target.GetX() + 1)
		if _, err = f.s.p13UFTransferBegin(f.ctx, id, bad, now()); err == nil {
			t.Fatal("client injected arbitrary transfer position")
		}
		replies, err := f.s.p13UFTransferBegin(f.ctx, id, req, now())
		if err != nil {
			t.Fatal(tr, err)
		}
		beginning := replies[0].(*pb.SCUFBeginTransfer)
		if beginning.GetCostTime() != 1 || !p13UFTransferSame(beginning.GetTarget(), target) {
			t.Fatal("original fixed1s", beginning)
		}
		again, err := f.s.p13UFTransferBegin(f.ctx, id, req, now())
		if err != nil || !proto.Equal(beginning, again[0]) {
			t.Fatal("duplicate original start deadline", err)
		}
		if _, err = f.s.p13UFTransferEnd(f.ctx, id, &pb.CSUFEndTransfer{Target: target}, now()); err == nil {
			t.Fatal("early original end")
		}
		if _, err = f.s.p13UFFieldMove(f.ctx, id, &pb.CSMove{PointNew: start}, now()); err == nil {
			t.Fatal("pending hidden controls accepted move")
		}
		if _, err = f.s.p13UFFieldHook(f.ctx, id, &pb.CSUFOnHook{OnHook: proto.Int32(1)}, now()); err == nil {
			t.Fatal("pending acceptedHook")
		}
		clock.Add(999)
		if err = f.s.TickP13UnionFightTransfer(f.ctx, now()); err != nil {
			t.Fatal(err)
		}
		actor, _ := read(id)
		if actor.X != start.GetX() || actor.Y != start.GetY() || actor.Z != start.GetZ() {
			t.Fatal("early transfer changed real position")
		}
		if total == 0 {
			sql, _ := db.GetIns().DB()
			sql.Close()
			f.open(t)
			if err = f.s.InitializeP13Guild(f.ctx); err != nil {
				t.Fatal(err)
			}
			recovered, err := f.s.p13UFFieldFrames(f.ctx, id, &pb.CSCurrentPosition{}, now())
			if err != nil {
				t.Fatal(err)
			}
			seen := false
			for _, m := range recovered {
				if x, ok := m.(*pb.SCUFBeginTransfer); ok {
					seen = x.GetCostTime() == 0.001 && p13UFTransferSame(x.GetTarget(), target)
				}
			}
			if !seen {
				t.Fatal("cold exact transfer deadline")
			}
		}
		clock.Add(1)
		if f.mysql.Database != "" && total == 0 {
			var wg sync.WaitGroup
			errs := make(chan error, 8)
			for i := 0; i < 8; i++ {
				wg.Add(1)
				go func() { defer wg.Done(); errs <- f.s.TickP13UnionFightTransfer(f.ctx, now()) }()
			}
			wg.Wait()
			close(errs)
			for err := range errs {
				if err != nil {
					t.Fatal(err)
				}
			}
		} else if err = f.s.TickP13UnionFightTransfer(f.ctx, now()); err != nil {
			t.Fatal(err)
		}
		actor, frame := read(id)
		if actor.X != target.GetX() || actor.Y != target.GetY() || actor.Z != target.GetZ() || frame.GetState() != 0 {
			t.Fatal("original target not atomically restored", tr, actor)
		}
		var profile db.Profile
		db.GetIns().First(&profile, id)
		_, game, err := decodeProfile(&profile)
		if err != nil || !p13UFTransferSame(game.Position.Current, target) {
			t.Fatal("actor/Profile actual target mismatch", err)
		}
		ended, err := f.s.p13UFTransferEnd(f.ctx, id, &pb.CSUFEndTransfer{Target: target}, now())
		if err != nil || ended[0].(*pb.SCUFEndTransfer).GetCostTime() != 1 {
			t.Fatal("canonical finished original end receipt", err)
		}
		if _, err = f.s.p13UFTransferEnd(f.ctx, id, &pb.CSUFEndTransfer{Target: start}, now()); err == nil {
			t.Fatal("completed reply accepted changed target")
		}
		total++
	}
	for _, tr := range all {
		if tr["TransferType"] != "1" {
			continue
		}
		camp := int64(0)
		if tr["Camp"] == "1" {
			camp = 1
		}
		id := byCamp[camp]
		target, _ := fieldPoint(tr["TransferEnd"])
		if clock.Load() < first {
			clock.Store(first)
		}
		replies, err := f.s.p13UFTransferBegin(f.ctx, id, &pb.CSUFBeginTransfer{TransferId: proto.String(tr["UFTransferId"]), Target: target}, now())
		if err != nil {
			t.Fatal(err)
		}
		if replies[0].(*pb.SCUFBeginTransfer).GetCostTime() != 5 {
			t.Fatal("original backcity5s")
		}
		_, frame := read(id)
		if frame.GetState() != 12 {
			t.Fatal("original >1s TransferBegin12")
		}
		clock.Add(5000)
		if err = f.s.TickP13UnionFightTransfer(f.ctx, now()); err != nil {
			t.Fatal(err)
		}
		actor, frame := read(id)
		if actor.X != target.GetX() || actor.Z != target.GetZ() || frame.GetState() != 0 {
			t.Fatal("original camp backcity end")
		}
		total++
	}
	if total != 18 {
		t.Fatal("partial sample instead of complete original table", total)
	}
	// 重新开始回城后真实退出取消任务；旧结束请求不能再改Town。
	tr := all[len(all)-2]
	id := byCamp[0]
	target, _ := fieldPoint(tr["TransferEnd"])
	if _, err = f.s.p13UFTransferBegin(f.ctx, id, &pb.CSUFBeginTransfer{TransferId: proto.String(tr["UFTransferId"]), Target: target}, now()); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p13UFFieldLeave(f.ctx, id, &pb.CSTransferScene{Target: proto.String("town_main")}, now()); err != nil {
		t.Fatal(err)
	}
	clock.Add(5000)
	if err = f.s.TickP13UnionFightTransfer(f.ctx, now()); err != nil {
		t.Fatal(err)
	}
	var pending int64
	db.GetIns().Model(&db.P13UnionFightTransferTask{}).Where("match_id=? AND status=0", match.ID).Count(&pending)
	if pending != 0 {
		t.Fatal("exit left a transfer lease")
	}
	if _, err = f.s.p13UFTransferEnd(f.ctx, id, &pb.CSUFEndTransfer{Target: target}, now()); err == nil {
		t.Fatal("old transfer ACK changed actual Town")
	}
}
