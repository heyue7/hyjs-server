package service

import (
	"context"
	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// TestMonthSignCalendar 验证真实时区边界、同日补差、月份重置及事务防重。
func TestMonthSignCalendar(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "sign.db")
	if err := db.InitGame(ctx, path); err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.GetIns().DB()
	defer func() { sqlDB.Close() }()
	s := &GameService{}
	if err := s.InitializeMonthSign(ctx); err != nil {
		t.Fatal(err)
	}
	// 固定金币夹具只验日历、VIP补差和冷回执；默认RE奖励另由VideoMonthSign测试核对。
	if err := db.GetIns().Model(&db.MonthSignRule{}).Where("day>0").Updates(map[string]any{"reward": "coin,1000", "double_vip": 2}).Error; err != nil {
		t.Fatal(err)
	}
	row, err := LoginAccount(ctx, "isolated-sign", "test-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(ctx, row.Id, "签到测试", "hero_043"); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 31, 15, 59, 0, 0, time.UTC)
	call := func(req proto.Message, at time.Time) ([]proto.Message, error) {
		return s.monthSignAt(ctx, row.Id, req, at)
	}
	info := func(at time.Time) *pb.SCRewardMonthSignList {
		t.Helper()
		m, e := call(&pb.CSRewardMonthSignList{}, at)
		if e != nil {
			t.Fatal(e)
		}
		return m[0].(*pb.SCRewardMonthSignList)
	}
	if got := info(now); got.GetCurrentDay() != 1 || len(got.List) != 31 || got.List[0].GetGetState() != 1 {
		t.Fatal("initial calendar", got)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := call(&pb.CSReceiveMonthSign{}, now); errs <- e }()
	}
	wg.Wait()
	close(errs)
	wins := 0
	for e := range errs {
		if e == nil {
			wins++
		}
	}
	if wins != 1 {
		t.Fatal("duplicate claims", wins)
	}
	if got := info(now); got.List[0].GetGetState() != 2 {
		t.Fatal("base receipt state")
	}
	// 修改SQL不能放大已经领取档的VIP差额快照。
	if err := db.GetIns().Model(&db.MonthSignRule{}).Where("day = ?", 1).Update("reward", "coin,999999").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().First(row, row.Id).Error; err != nil {
		t.Fatal(err)
	}
	p, g, err := decodeProfile(row)
	if err != nil {
		t.Fatal(err)
	}
	p.Vip = proto.Int32(2)
	if err := saveProfile(db.GetIns(), row, p, g); err != nil {
		t.Fatal(err)
	}
	if _, err = call(&pb.CSReceiveMonthSign{}, now); err != nil {
		t.Fatal(err)
	}
	if _, err = call(&pb.CSReceiveMonthSign{}, now); err == nil {
		t.Fatal("double duplicate accepted")
	}
	if err := db.GetIns().First(row, row.Id).Error; err != nil {
		t.Fatal(err)
	}
	p, _, err = decodeProfile(row)
	if err != nil || p.GetCoin() != 2000 {
		t.Fatal("snapshot balance", p.GetCoin(), err)
	}
	if got := info(now); got.List[0].GetGetState() != 3 {
		t.Fatal("double state")
	}
	if _, err = call(&pb.CSReceiveLuxurySign{}, now); err == nil {
		t.Fatal("luxury granted without payment")
	}
	sqlDB.Close()
	if err := db.InitGame(ctx, path); err != nil {
		t.Fatal(err)
	}
	sqlDB, _ = db.GetIns().DB()
	if err := s.InitializeMonthSign(ctx); err != nil {
		t.Fatal(err)
	}
	if got := info(now); got.List[0].GetGetState() != 3 {
		t.Fatal("restart receipt lost")
	}
	next := now.Add(time.Minute)
	if got := info(next); got.GetCurrentDay() != 1 || len(got.List) != 30 || got.List[0].GetGetState() != 1 {
		t.Fatal("UTC+8 month boundary", got)
	}
	if _, err = call(&pb.CSReceiveMonthSign{}, next); err != nil {
		t.Fatal(err)
	}
	later := next.Add(48 * time.Hour)
	if got := info(later); got.GetCurrentDay() != 2 {
		t.Fatal("missed date incorrectly added")
	}
	// 不合法SQL在发奖前拒绝，领取状态与余额均保持原值。
	if err := db.GetIns().Model(&db.MonthSignRule{}).Where("day = ?", 2).Update("reward", "coin,1;unknown,1").Error; err != nil {
		t.Fatal(err)
	}
	if _, err = call(&pb.CSReceiveMonthSign{}, later); err == nil {
		t.Fatal("invalid SQL granted")
	}
	var state db.MonthSignState
	if err := db.GetIns().Where("player_id = ? AND month = ?", row.Id, "2026-11").First(&state).Error; err != nil || state.Days != 1 {
		t.Fatal("rollback", state, err)
	}
}
