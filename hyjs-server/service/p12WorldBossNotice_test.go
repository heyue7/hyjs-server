package service

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func p12NoticeSink(t *testing.T, s *GameService, id int64, session []byte, send func(proto.Message) error) {
	t.Helper()
	key := p10ClientKey{s, id}
	sink := &p10ClientSink{session: session, send: send}
	p10ClientSinks.Store(key, sink)
	t.Cleanup(func() { p10ClientSinks.CompareAndDelete(key, sink) })
}

func TestP12WorldBossKillNoticeCommittedConcurrentRetryAndRestart(t *testing.T) {
	f, other, boss, now := p12WorldBossBattleSetup(t)
	ids := []int64{f.id, other}
	sessions := [][]byte{p12WorldBossBattlePrepare(t, f, f.id), p12WorldBossBattlePrepare(t, f, other)}
	starts := make([]*pb.SCWorldBossFight, 2)
	var mu sync.Mutex
	received := map[int64]int{}
	for i, id := range ids {
		i, id := i, id
		p12NoticeSink(t, f.s, id, sessions[i], func(message proto.Message) error {
			frame, ok := message.(*pb.SCPublicMessage)
			if !ok || frame.GetPubmsg().GetType() != 1 || frame.GetPubmsg().GetScrollType() != 0 || !strings.Contains(frame.GetPubmsg().GetMsg(), "世界首领") {
				t.Errorf("original native marquee frame: %v", message)
			}
			var instance db.P12WorldBossBattleInstance
			var committed int64
			if err := db.GetIns().First(&instance, "id=?", boss).Error; err != nil {
				t.Error(err)
			}
			if err := db.GetIns().Model(&db.P12WorldBossBattleReceipt{}).Where("instance_id=? AND committed=? AND applied_damage>0", boss, true).Count(&committed).Error; err != nil {
				t.Error(err)
			}
			if instance.CurrentHP != 0 || instance.KillerID == 0 || committed != 1 {
				t.Errorf("push happened before commit: %+v, receipts %d", instance, committed)
			}
			mu.Lock()
			received[id]++
			mu.Unlock()
			return nil
		})
		var err error
		starts[i], err = f.s.startP12WorldBossBattleAt(f.ctx, id, boss, int64(i+1), now)
		if err != nil {
			t.Fatal(err)
		}
	}
	// 同进程另一GameService的会话不属于本服广播域。
	p12NoticeSink(t, &GameService{}, f.id, sessions[0], func(proto.Message) error { t.Error("cross-service broadcast"); return nil })
	if len(received) != 0 {
		t.Fatal("precomputation announced a predicted kill")
	}
	// 只缩小隔离夹具的共享剩余血量；伤害仍由真实冻结战斗计算和原Finish提交。
	if err := db.GetIns().Model(&db.P12WorldBossBattleInstance{}).Where("id=?", boss).Update("current_hp", 1).Error; err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i, id := range ids {
		i, id := i, id
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := f.s.finishP12WorldBossBattleAt(f.ctx, id, sessions[i], starts[i].GetFightId(), now.Add(time.Minute)); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	for _, id := range ids {
		if received[id] != 1 {
			t.Fatalf("one global announcement per authenticated role, got %v", received)
		}
	}
	connection, _ := db.GetIns().DB()
	if err := connection.Close(); err != nil {
		t.Fatal(err)
	}
	f.open(t)
	for i, id := range ids {
		for retry := 0; retry < 2; retry++ {
			response, err := f.s.finishP12WorldBossBattleAt(f.ctx, id, sessions[i], starts[i].GetFightId(), now.Add(2*time.Minute))
			if err != nil || !response.AlreadyCommitted {
				t.Fatal("durable finish replay", response, err)
			}
		}
		if received[id] != 1 {
			t.Fatal("retry/cold recovery repeated marquee", received)
		}
	}
}

func TestP12WorldBossKillNoticeNonlethalAndRollback(t *testing.T) {
	f, _, boss, now := p12WorldBossBattleSetup(t)
	session := p12WorldBossBattlePrepare(t, f, f.id)
	pushes := 0
	p12NoticeSink(t, f.s, f.id, session, func(proto.Message) error { pushes++; return nil })
	start, err := f.s.startP12WorldBossBattleAt(f.ctx, f.id, boss, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.finishP12WorldBossBattleAt(f.ctx, f.id, session, start.GetFightId(), now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if pushes != 0 {
		t.Fatal("nonlethal damage announced kill")
	}
	p12WorldBossBattlePrepare(t, f, f.id)
	start, err = f.s.startP12WorldBossBattleAt(f.ctx, f.id, boss, 0, now.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Model(&db.P12WorldBossBattleInstance{}).Where("id=?", boss).Update("current_hp", 1).Error; err != nil {
		t.Fatal(err)
	}
	// 故意让死亡标记写入之后的回执更新失败，证明整个事务回滚不会产生全服提示。
	broken := errors.New("isolated notice rollback")
	callback := "p12-notice-rollback"
	if err = db.GetIns().Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "p12_world_boss_battle_receipts" {
			tx.AddError(broken)
		}
	}); err != nil {
		t.Fatal(err)
	}
	_, err = f.s.finishP12WorldBossBattleAt(f.ctx, f.id, session, start.GetFightId(), now.Add(3*time.Minute))
	db.GetIns().Callback().Update().Remove(callback)
	if !errors.Is(err, broken) || pushes != 0 {
		t.Fatal("rollback broadcast", err, pushes)
	}
	var instance db.P12WorldBossBattleInstance
	if err = db.GetIns().First(&instance, "id=?", boss).Error; err != nil || instance.CurrentHP != 1 || instance.KillerID != 0 {
		t.Fatal("rollback changed death fact", instance, err)
	}
	if _, err = f.s.finishP12WorldBossBattleAt(f.ctx, f.id, session, start.GetFightId(), now.Add(4*time.Minute)); err != nil || pushes != 1 {
		t.Fatal("successful retry should publish once", err, pushes)
	}
}

func TestP12WorldBossKillNoticeEscapesNGUITags(t *testing.T) {
	message := p12WorldBossKillNotice("[ff0000]勇者[-]\n")
	if strings.ContainsAny(message.GetPubmsg().GetMsg(), "[]\n") || !strings.Contains(message.GetPubmsg().GetMsg(), "勇者") {
		t.Fatal("untrusted player name may alter marquee formatting", message)
	}
}
