package service

import (
	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"sync"
	"testing"
	"time"
)

// TestP13GuildBuilding 核对原建设费用、实际职位、同stamp并发与恢复，不以科技目录授予未知效果。
func TestP13GuildBuildingOriginalCostsPermissionsConcurrentReceiptsAndRestart(t *testing.T) {
	f, ids := p13GuildSetup(t)
	now := time.Now()
	p13GuildOpen(t, true)
	p13GuildCall(t, f, ids[0], &pb.CSUnionCreate{Name: proto.String("原建设事务")}, 1, now)
	guild := p13GuildID(t, f, ids[0], now)
	p13Join(t, f, ids[0], ids[1], guild, 2, now)
	p13Join(t, f, ids[0], ids[2], guild, 3, now)
	req := &pb.CSUnionOpenBuilder{Type: proto.String("HeroStatue")}
	before, _ := p10LuckyProfile(t, ids[0])
	if _, _, err := f.s.DispatchP13Guild(f.ctx, ids[0], req, 10); err == nil {
		t.Fatal("insufficient shared assets opened")
	}
	if err := db.GetIns().Model(&db.P13Guild{}).Where("id=?", guild).Updates(map[string]any{"coin": int64(20000000), "badge": int32(40)}).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.s.DispatchP13Guild(f.ctx, ids[1], req, 10); err == nil {
		t.Fatal("ordinary member built")
	}
	if _, _, err := f.s.DispatchP13Guild(f.ctx, ids[3], req, 10); err == nil {
		t.Fatal("outsider built")
	}
	if _, _, err := f.s.DispatchP13Guild(f.ctx, ids[0], &pb.CSUnionOpenBuilder{Type: proto.String("Church")}, 10); err == nil {
		t.Fatal("unknown building accepted")
	}
	p13GuildOpen(t, false)
	if _, _, err := f.s.DispatchP13Guild(f.ctx, ids[0], req, 10); err == nil {
		t.Fatal("closed economy built")
	}
	p13GuildOpen(t, true)
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			messages, handled, err := f.s.DispatchP13Guild(f.ctx, ids[0], req, 10)
			if err == nil && (!handled || len(messages) != 2) {
				t.Error("original response missing")
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
	var row db.P13Guild
	if err := db.GetIns().First(&row, "id=?", guild).Error; err != nil {
		t.Fatal(err)
	}
	if row.Coin != 10000000 || row.Badge != 20 || row.HeroStatueOpen != 1 || row.GuardTreeOpen != 0 {
		t.Fatal("original shared cost", row)
	}
	if _, _, err := f.s.DispatchP13Guild(f.ctx, ids[0], req, 11); err == nil {
		t.Fatal("new stamp double constructed")
	}
	if _, _, err := f.s.DispatchP13Guild(f.ctx, ids[0], &pb.CSUnionOpenBuilder{Type: proto.String("GuardTree")}, 10); err == nil {
		t.Fatal("same stamp changed type")
	}
	p13GuildCall(t, f, ids[0], &pb.CSPromotionViceChairman{PlayerId: proto.Int64(ids[1])}, 12, now)
	if _, _, err := f.s.DispatchP13Guild(f.ctx, ids[1], &pb.CSUnionOpenBuilder{Type: proto.String("GuardTree")}, 13); err != nil {
		t.Fatal(err)
	}
	after, _ := p10LuckyProfile(t, ids[0])
	if !proto.Equal(before, after) {
		t.Fatal("shared construction charged personal wallet")
	}
	sql, _ := db.GetIns().DB()
	sql.Close()
	f.open(t)
	if err := f.s.InitializeP13Guild(f.ctx); err != nil {
		t.Fatal(err)
	}
	info := p13GuildCall(t, f, ids[2], &pb.CSGetUnionInfo{}, 0, now).(*pb.SCGetUnionInfo)
	if info.GetHeroStatueOpen() != 1 || info.GetGuardTreeOpen() != 1 || info.GetCoinNum() != 0 || info.GetBadgeNum() != 0 {
		t.Fatal("construction persistence", info)
	}
	state, err := f.s.P13GuildBuildingState(db.GetIns(), ids[2])
	if err != nil || state.GetHeroStatueOpen() != 1 || state.GetGuardTreeOpen() != 1 {
		t.Fatal("scene building source", state, err)
	}
	if _, _, err = f.s.DispatchP13Guild(f.ctx, ids[0], req, 10); err != nil {
		t.Fatal("receipt retry after restart", err)
	}
	p13GuildCall(t, f, ids[2], &pb.CSQuitUnion{}, 14, now)
	if _, err = f.s.P13GuildBuildingState(db.GetIns(), ids[2]); err == nil {
		t.Fatal("former member building state")
	}
}

// TestP13GuildBuilding 不同成员同时开启同建筑只成功一次，资产与回执保持一致。
func TestP13GuildBuildingDistinctRequestConcurrentSharedDebitOnce(t *testing.T) {
	f, ids := p13GuildSetup(t)
	now := time.Now()
	p13GuildOpen(t, true)
	p13GuildCall(t, f, ids[0], &pb.CSUnionCreate{Name: proto.String("建设争用")}, 1, now)
	guild := p13GuildID(t, f, ids[0], now)
	p13Join(t, f, ids[0], ids[1], guild, 2, now)
	p13GuildCall(t, f, ids[0], &pb.CSPromotionViceChairman{PlayerId: proto.Int64(ids[1])}, 3, now)
	if err := db.GetIns().Model(&db.P13Guild{}).Where("id=?", guild).Updates(map[string]any{"coin": int64(20000000), "badge": int32(40)}).Error; err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, id := range ids[:2] {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			_, _, err := f.s.DispatchP13Guild(f.ctx, id, &pb.CSUnionOpenBuilder{Type: proto.String("HeroStatue")}, 10)
			errs <- err
		}(id)
	}
	wg.Wait()
	close(errs)
	success := 0
	for err := range errs {
		if err == nil {
			success++
		}
	}
	var row db.P13Guild
	db.GetIns().First(&row, "id=?", guild)
	if success != 1 || row.Coin != 10000000 || row.Badge != 20 || row.HeroStatueOpen != 1 {
		t.Fatal("concurrent different requests", success, row)
	}
}
