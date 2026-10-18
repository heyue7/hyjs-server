package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"sync"
	"testing"
	"time"
)

// TestPersonalTurntableTwoOwnersInventoryJackpotAndColdReceipt consumes actual original wheel requests.
func TestPersonalTurntableTwoOwnersInventoryJackpotAndColdReceipt(t *testing.T) {
	f := p1Setup(t)
	if err := f.s.InitializeTurntable(f.ctx); err != nil {
		t.Fatal(err)
	}
	key := "turntable_20261011"
	now := time.Now().UnixMilli()
	if err := db.GetIns().Model(&db.GameplayPool{}).Where("id=?", key).Updates(map[string]any{"enabled": true, "start_at": now - 60000, "end_at": now + 3600000, "reward_end_at": now + 7200000, "jackpot_per_million": 0, "free_per_day": 0}).Error; err != nil {
		t.Fatal(err)
	}
	// Finite representative: all nine positions use ordinary coins, each allows one draw per owner.
	if err := db.GetIns().Model(&db.GameplayPoolReward{}).Where("pool_id=?", key).Updates(map[string]any{"resource_id": "coin", "amount": 1, "weight": 1, "stock": 1}).Error; err != nil {
		t.Fatal(err)
	}
	other, err := LoginAccount(f.ctx, fmt.Sprintf("personal-other-%d", now), "fixture-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(f.ctx, other.Id, "个人盘乙", "hero_043"); err != nil {
		t.Fatal(err)
	}
	ids := []int64{f.id, other.Id}
	for _, id := range ids {
		if err := updateProfile(f.ctx, id, func(p *pb.Player, _ *model.GameState) error { p.Diamond = proto.Int64(10000); return nil }); err != nil {
			t.Fatal(err)
		}
	}
	var globalBefore db.TurntableGlobal
	if err := db.GetIns().First(&globalBefore, "pool_id=?", key).Error; err != nil {
		t.Fatal(err)
	}
	beforeRaw, _ := json.Marshal(globalBefore)
	req := &pb.CSPlayBigTurn{Count: proto.Int32(1)}
	var first proto.Message
	for _, id := range ids {
		msgs, err := f.s.playBigTurn(f.ctx, id, req, 99101)
		if err != nil {
			t.Fatal(err)
		}
		if id == f.id {
			first = msgs[0]
		}
	}
	a := db.TurntablePersonal{}
	b := db.TurntablePersonal{}
	if err := db.GetIns().First(&a, "pool_id=? AND player_id=?", key, f.id).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().First(&b, "pool_id=? AND player_id=?", key, other.Id).Error; err != nil {
		t.Fatal(err)
	}
	if a.DiamondPool != 10001 || b.DiamondPool != 10001 || a.LegacyScore != 0 || b.LegacyScore != 0 {
		t.Fatal("pool contributions shared", a, b)
	}
	// Same timestamp fan-out must read a single committed cost/reward.
	var wg sync.WaitGroup
	errs := make(chan error, 6)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			msgs, e := f.s.playBigTurn(f.ctx, f.id, req, 99101)
			if e == nil && !proto.Equal(msgs[0], first) {
				e = fmt.Errorf("receipt changed")
			}
			errs <- e
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	if p1Count(t, &db.TurntableRecord{}, "pool_id=? AND player_id=?", key, f.id) != 1 {
		t.Fatal("replayed draw")
	}
	// Exhaust this owner's remaining eight prizes; the other player's remaining stock stays eight.
	for i := 0; i < 8; i++ {
		if _, err := f.s.playBigTurn(f.ctx, f.id, req, int64(99102+i)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.s.playBigTurn(f.ctx, f.id, req, 99120); err == nil {
		t.Fatal("exhausted personal stock accepted")
	}
	if _, err := f.s.playBigTurn(f.ctx, other.Id, req, 99120); err != nil {
		t.Fatal("other inventory consumed", err)
	}
	// A guaranteed jackpot consumes only the owner's pool; no finite ordinary inventory is bypassed.
	if err := db.GetIns().Model(&db.GameplayPool{}).Where("id=?", key).Update("jackpot_per_million", 1000000).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().First(&a, "pool_id=? AND player_id=?", key, f.id).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.playBigTurn(f.ctx, other.Id, req, 99121); err != nil {
		t.Fatal(err)
	}
	var unchanged db.TurntablePersonal
	db.GetIns().First(&unchanged, "pool_id=? AND player_id=?", key, f.id)
	if unchanged.DiamondPool != a.DiamondPool {
		t.Fatal("foreign jackpot consumed owner pool")
	}
	reads, err := f.s.bigTurnRecord(f.ctx, other.Id)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(reads[0])
	if bytes.Contains(raw, []byte("公共奖励回归")) {
		t.Fatal("foreign draw history leaked")
	}
	ranks, err := f.s.bigTurnRank(f.ctx, other.Id)
	if err != nil || len(ranks[0].(*pb.SCBigTurnRank).GetRanks()) != 0 {
		t.Fatal("private wheel exposed global contest", err)
	}
	var globalAfter db.TurntableGlobal
	db.GetIns().First(&globalAfter, "pool_id=?", key)
	afterRaw, _ := json.Marshal(globalAfter)
	if !bytes.Equal(beforeRaw, afterRaw) {
		t.Fatal("retired shared pool mutated")
	}
	f.restart(t)
	again, err := f.s.playBigTurn(context.Background(), f.id, req, 99101)
	if err != nil || !proto.Equal(again[0], first) {
		t.Fatal("cold replay changed", err)
	}
}
