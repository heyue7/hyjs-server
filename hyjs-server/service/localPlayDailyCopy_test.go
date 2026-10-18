package service

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// 原资源副本16类/48档按个人星期与原日限投影。战斗使用有限协议
// 败场合同，只验证日额、续接和所有权，不授胜利或资源、不扩P6证明。
func TestLocalPlayDailyCopyWeekQuotaOwnershipAndCold(t *testing.T) {
	f, id, owner := localClockFixture(t)
	if err := f.s.InitializeDailyCopy(f.ctx); err != nil {
		t.Fatal(err)
	}
	localClockEnable(t)
	ctx := localClockBind(t, f, owner)
	if err := updateProfile(f.ctx, id, func(p *pb.Player, _ *model.GameState) error {
		p.Level = proto.Int32(75)
		p.Stamina = proto.Int32(200)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// 用固定真实周一核整个星期；个人偏移仍来自真实授权日期事务。
	real := time.Date(2026, 10, 12, 12, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
	for day := 1; day <= 7; day++ {
		if day > 1 {
			if _, err := f.s.LocalPlayTimeChange(ctx, id, "nextDay", "daily-week-"+strconv.Itoa(day)); err != nil {
				t.Fatal(err)
			}
			ctx = localClockBind(t, f, owner)
		}
		out, err := f.s.dailyCopyAt(ctx, id, &pb.CSDailyCopyState{}, int64(day), real)
		if err != nil {
			t.Fatal(err)
		}
		info := out[0].(*pb.SCDailyCopyState)
		if len(info.GetItems()) != 16 {
			t.Fatal("original resource types missing")
		}
		for _, item := range info.GetItems() {
			rule := f.s.table("DailyCopy", "Type", item.GetType())
			open := strings.Contains(","+rule["OpenDay"]+",", ","+strconv.Itoa(day)+",")
			if (item.GetIsOpen() == 1) != open || item.GetLastTime() != int32(tableNumber(rule, "Count")) || len(item.GetDailyCopyState()) != 3 {
				t.Fatal("personal weekday/quotas differ from original", day, item)
			}
		}
	}
	if _, err := f.s.LocalPlayTimeChange(ctx, id, "reset", "daily-reset"); err != nil {
		t.Fatal(err)
	}
	ctx = localClockBind(t, f, owner)
	start := func(ctx context.Context, chapter string, stamp int64) *pb.SCDailyCopyFightStart {
		t.Helper()
		out, err := f.s.dailyCopyAt(ctx, id, &pb.CSDailyCopyFightStart{ChapterId: proto.String(chapter)}, stamp, real)
		if err != nil {
			t.Fatal(err)
		}
		return out[0].(*pb.SCDailyCopyFightStart)
	}
	lose := func(ctx context.Context, fight *pb.SCDailyCopyFightStart, chapter string) {
		t.Helper()
		request := &pb.CSDailyCopyFightEnd{ChapterId: proto.String(chapter), FightId: fight.FightId, IsWin: proto.Int32(0), OptRecord: &pb.BattleOperate{FinalRoundNum: proto.Int32(1), AttackerTotalLeftHp: proto.Float64(0), DefenderTotalLeftHp: proto.Float64(1)}}
		if _, err := f.s.dailyCopyAt(ctx, id, request, 0, real); err != nil {
			t.Fatal(err)
		}
	}
	first := start(ctx, "1", 310001)
	lose(ctx, first, "1")
	if _, err := f.s.LocalPlayTimeChange(ctx, id, "nextDay", "daily-tuesday"); err != nil {
		t.Fatal(err)
	}
	ctx = localClockBind(t, f, owner)
	treasureChapter := f.s.table("DailyCopyChapter", "Type", "TreasureExp")["ChapterId"]
	second := start(ctx, treasureChapter, 310002)
	if err := f.s.ensureNoActiveDailyCopy(db.GetIns().WithContext(ctx), id, real); err == nil {
		t.Fatal("personal-day pending failed cross-mode ownership")
	}
	if _, err := f.s.LocalPlayTimeChange(ctx, id, "previousDay", "daily-monday-return"); err != nil {
		t.Fatal(err)
	}
	ctx = localClockBind(t, f, owner)
	if _, err := f.s.dailyCopyAt(ctx, id, &pb.CSDailyCopyFightStart{ChapterId: proto.String("1")}, 310003, real); err == nil {
		t.Fatal("rewind granted another original Monday attempt")
	}
	if _, err := f.s.LocalPlayTimeChange(ctx, id, "nextDay", "daily-tuesday-return"); err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	ctx = localClockBind(t, f, owner)
	resumed := start(ctx, treasureChapter, 310004)
	if !proto.Equal(second, resumed) {
		t.Fatal("cold original pending changed or charged another attempt")
	}
	lose(ctx, resumed, treasureChapter)
	var states []db.DailyCopyState
	if err := db.GetIns().Where("player_id=? AND used>0", id).Find(&states).Error; err != nil || len(states) != 2 {
		t.Fatal("date-scoped consumed rows", err)
	}
	for _, state := range states {
		if state.Used != 1 || state.Limit != 1 {
			t.Fatal("date revisit reset a consumed quota", state)
		}
	}
	if err := f.s.ensureNoActiveDailyCopy(db.GetIns().WithContext(ctx), id, real); err != nil {
		t.Fatal("settled personal-day fight still owns player", err)
	}
}
