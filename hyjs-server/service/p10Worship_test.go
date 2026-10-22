package service

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p10WorshipSetup(t *testing.T) (*p1Fixture, []int64) {
	t.Helper()
	f, b, c := p10Setup(t)
	if err := f.s.InitializeP10Worship(f.ctx); err != nil {
		t.Fatal(err)
	}
	for _, table := range []any{&db.P10WorshipPolicy{}, &db.P10WorshipDaily{}, &db.P10WorshipAction{}} {
		if err := db.GetIns().Where("1=1").Delete(table).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := f.s.InitializeP10Worship(f.ctx); err != nil {
		t.Fatal(err)
	}
	ids := []int64{b, c}
	for i := 0; i < 2; i++ {
		login, err := LoginAccount(f.ctx, fmt.Sprintf("p10-worship-%s-%d-%d", t.Name(), i, time.Now().UnixNano()), "isolated-local-pass")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := CreatePlayer(f.ctx, login.Id, fmt.Sprintf("真实膜拜%d", i), "hero_047"); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, login.Id)
	}
	for i, id := range ids {
		if err := updateProfile(f.ctx, id, func(p *pb.Player, g *model.GameState) error {
			p.Level = proto.Int32(90 - int32(i))
			g.Warriors[0].Level = proto.Int32(70 - int32(i))
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := LoginAccount(f.ctx, fmt.Sprintf("p10-worship-uncreated-%s-%d", t.Name(), time.Now().UnixNano()), "isolated-local-pass"); err != nil {
		t.Fatal(err)
	}
	return f, ids
}

// TestP10Worship 原每日3次/一万金币来自真实双榜前三，8并发与跨榜重试不重复发奖。
func TestP10WorshipActualRankSourceCoinDailyThreeSharedTargetsConcurrencyAndCold(t *testing.T) {
	f, ids := p10WorshipSetup(t)
	now := time.Now().Add(-24 * time.Hour)
	query := &pb.CSFamous{Type: proto.Int32(1)}
	closed, err := f.s.p10Worship(f.ctx, f.id, query, 0, now)
	if err != nil || closed[0].(*pb.SCFamous).GetLeftAddGood() != 0 || len(closed[0].(*pb.SCFamous).PlayerList) != 3 {
		t.Fatal("closed rank query", err)
	}
	if _, err := f.s.p10Worship(f.ctx, f.id, &pb.CSGood{PlayerId: proto.Int64(ids[0])}, 10, now); err == nil {
		t.Fatal("closed award")
	}
	if err := db.GetIns().Model(&db.P10WorshipPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	for _, kind := range []int32{0, 1} {
		out, err := f.s.p10Worship(f.ctx, f.id, &pb.CSFamous{Type: proto.Int32(kind)}, 0, now)
		if err != nil {
			t.Fatal(err)
		}
		rank := out[0].(*pb.SCFamous)
		if rank.GetLeftAddGood() != 3 || len(rank.PlayerList) != 3 || rank.PlayerList[0].GetPlayerId() != ids[0] {
			t.Fatal("real highest rank", rank)
		}
		if rank.PlayerList[0].GetFightPower() <= rank.PlayerList[2].GetFightPower() {
			t.Fatal("power not actual level panel")
		}
	}
	if _, err := f.s.p10Worship(f.ctx, f.id, &pb.CSGood{PlayerId: proto.Int64(ids[3])}, 11, now); err == nil {
		t.Fatal("fourth rank fabricated qualification")
	}
	before, _ := f.profile(t)
	request := &pb.CSGood{PlayerId: proto.Int64(ids[0])}
	var results []proto.Message
	if f.mysql.Database != "" {
		var wg sync.WaitGroup
		var mu sync.Mutex
		errs := make(chan error, 8)
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				out, err := f.s.p10Worship(f.ctx, f.id, request, 12, now)
				if err == nil {
					mu.Lock()
					results = append(results, out[0])
					mu.Unlock()
				}
				errs <- err
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
	} else {
		for i := 0; i < 8; i++ {
			out, err := f.s.p10Worship(f.ctx, f.id, request, 12, now)
			if err != nil {
				t.Fatal(err)
			}
			results = append(results, out[0])
		}
	}
	for _, out := range results {
		if !proto.Equal(out, results[0]) {
			t.Fatal("exact repeated request changed reward")
		}
	}
	after, _ := f.profile(t)
	if after.GetCoin()-before.GetCoin() != 10000 || p1Count(t, &db.P10WorshipAction{}, "player_id=?", f.id) != 1 || p1Count(t, &db.EventFact{}, "player_id=? AND kind=?", f.id, constUtil.P10WorshipEvent) != 1 {
		t.Fatal("concurrent worship duplicated coin or facts")
	}
	if _, err := f.s.p10Worship(f.ctx, f.id, request, 13, now); err == nil {
		t.Fatal("same target new stamp re-awarded")
	}
	if _, err := f.s.p10Worship(f.ctx, f.id, &pb.CSGood{PlayerId: proto.Int64(ids[1])}, 12, now); err == nil {
		t.Fatal("same stamp changed target")
	}
	for i, id := range ids[1:3] {
		if _, err := f.s.p10Worship(f.ctx, f.id, &pb.CSGood{PlayerId: proto.Int64(id)}, 20+int64(i), now); err != nil {
			t.Fatal(err)
		}
	}
	for _, kind := range []int32{0, 1} {
		out, err := f.s.p10Worship(f.ctx, f.id, &pb.CSFamous{Type: proto.Int32(kind)}, 0, now)
		if err != nil || out[0].(*pb.SCFamous).GetLeftAddGood() != 0 || len(out[0].(*pb.SCFamous).GoodList) != 3 {
			t.Fatal("daily cap not shared across ranks", err)
		}
	}
	if _, err := f.s.p10Worship(f.ctx, f.id, &pb.CSGood{PlayerId: proto.Int64(ids[3])}, 22, now); err == nil {
		t.Fatal("daily fourth worship")
	}
	// 次日使用真实当前日而不伪造未来事件；旧精确回执不占新日余额。
	current := time.Now()
	out, err := f.s.p10Worship(f.ctx, f.id, query, 0, current)
	if err != nil || out[0].(*pb.SCFamous).GetLeftAddGood() != 3 || len(out[0].(*pb.SCFamous).GoodList) != 0 {
		t.Fatal("new Beijing day", err)
	}
	if _, err := f.s.p10Worship(f.ctx, f.id, request, 30, current); err != nil {
		t.Fatal(err)
	}
	sql, _ := db.GetIns().DB()
	sql.Close()
	f.open(t)
	if err := f.s.InitializeP10Worship(f.ctx); err != nil {
		t.Fatal(err)
	}
	replayed, err := f.s.p10Worship(f.ctx, f.id, request, 12, current)
	if err != nil || !proto.Equal(replayed[0], results[0]) {
		t.Fatal("cold exact old receipt", err)
	}
	after, _ = f.profile(t)
	if after.GetCoin()-before.GetCoin() != 40000 {
		t.Fatal("cold receipt or day reset duplicated reward")
	}
	if err := db.GetIns().Model(&db.P10WorshipAction{}).Where("player_id=? AND day=?", f.id, p1Day(current)).Update("rank_position", 3).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p10Worship(f.ctx, f.id, query, 0, current); err == nil {
		t.Fatal("tampered actual rank source accepted")
	}
}

// The viewer may be ranked too. Both original famous tabs use formation slot order.
func TestP10WorshipModelFollowsFirstFormationSlotAfterReload(t *testing.T) {
	f, ids := p10WorshipSetup(t)
	id := ids[0]
	if err := updateProfile(f.ctx, id, func(p *pb.Player, g *model.GameState) error {
		hero := proto.Clone(g.Warriors[0]).(*pb.Warrior)
		hero.Id = proto.String("hero_001")
		hero.Guid = proto.String("rank-model-first")
		g.Warriors = append(g.Warriors, hero)
		g.Equipment = append(g.Equipment, &pb.Equipment{EquipId: proto.String("equip_005"), Guid: proto.String("rank-first-weapon"), Level: proto.Int32(1)})
		// Store the list out of order to prove the protocol's formatPos owns the first place.
		g.Formation = []*pb.FormationItem{
			{FormatPos: proto.Int32(3), Things: []*pb.FormationThing{{Pos: proto.Int32(0)}}},
			{FormatPos: proto.Int32(2), Things: []*pb.FormationThing{{Pos: proto.Int32(0), Id: g.Warriors[0].Id, Guid: g.Warriors[0].Guid}}},
			{FormatPos: proto.Int32(1), Things: []*pb.FormationThing{{Pos: proto.Int32(0), Id: hero.Id, Guid: hero.Guid}, {Pos: proto.Int32(1), Id: proto.String("equip_005"), Guid: proto.String("rank-first-weapon")}}},
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	for _, kind := range []int32{0, 1} {
		out, err := f.s.p10Worship(f.ctx, id, &pb.CSFamous{Type: proto.Int32(kind)}, 0, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, row := range out[0].(*pb.SCFamous).PlayerList {
			if row.GetPlayerId() == id {
				found = true
				if row.GetHeroWarId() != "hero_001" {
					t.Fatalf("rank tab %d uses %s", kind, row.GetHeroWarId())
				}
				if row.GetWeaponId() != "equip_005" {
					t.Fatalf("rank tab %d lost actual first-slot weapon: %s", kind, row.GetWeaponId())
				}
			}
		}
		if !found {
			t.Fatalf("self not on rank tab %d", kind)
		}
	}
}
