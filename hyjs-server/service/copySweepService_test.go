package service

import (
	"context"
	"encoding/json"
	"math"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// TestCopySweepOriginalRules 验证每包一次、五星/等级或VIP、日限、资源及持久回执。
func TestCopySweepOriginalRules(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "sweep.db")
	if err := db.InitGame(ctx, path); err != nil {
		t.Fatal(err)
	}
	connection, _ := db.GetIns().DB()
	defer func() { connection.Close() }()
	s := &GameService{}
	if err := s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := s.InitCooking(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.InitializeDailyCopy(ctx); err != nil {
		t.Fatal(err)
	}
	account, err := LoginAccount(ctx, "isolated-sweep", "password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CreatePlayer(ctx, account.Id, "扫荡", "hero_043"); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 12, 15, 59, 0, 0, time.UTC)
	mutate := func(fn func(*pb.Player, *model.GameState)) {
		t.Helper()
		if err := updateProfile(ctx, account.Id, func(p *pb.Player, g *model.GameState) error { fn(p, g); return nil }); err != nil {
			t.Fatal(err)
		}
	}
	load := func() (*pb.Player, *model.GameState) {
		t.Helper()
		var row db.Profile
		if err := db.GetIns().First(&row, account.Id).Error; err != nil {
			t.Fatal(err)
		}
		p, g, err := decodeProfile(&row)
		if err != nil {
			t.Fatal(err)
		}
		return p, g
	}
	mutate(func(p *pb.Player, g *model.GameState) {
		p.Level = proto.Int32(14)
		g.Copy.PassedCopyInfo = []*pb.CopySimpleInfo{{ChapterId: proto.String("1"), MissionId: proto.String("1"), Star: proto.Int32(5), ChallengeTimes: proto.Int32(0)}}
		refreshCopySweepDay(g, now)
	})
	request := &pb.CSCopySweep{ChapterId: proto.String("1"), MissionId: proto.String("1")}
	deny := func(req *pb.CSCopySweep, stamp int64) {
		t.Helper()
		p, g := load()
		beforeP := proto.Clone(p)
		beforeGame, _ := json.Marshal(g)
		_, err := s.copySweepAt(ctx, account.Id, req, stamp, now)
		if err == nil {
			t.Fatal("invalid sweep granted", req, stamp)
		}
		afterP, afterG := load()
		afterGame, _ := json.Marshal(afterG)
		if !proto.Equal(beforeP, afterP) || string(beforeGame) != string(afterGame) {
			t.Fatal("failed sweep mutation", err)
		}
	}
	deny(&pb.CSCopySweep{ChapterId: proto.String("2"), MissionId: proto.String("6")}, 100)
	deny(&pb.CSCopySweep{ChapterId: proto.String("1"), MissionId: proto.String("999")}, 100)
	deny(request, 0)
	mutate(func(p *pb.Player, g *model.GameState) { p.Level = proto.Int32(13) })
	deny(request, 100)
	mutate(func(p *pb.Player, g *model.GameState) {
		p.Level = proto.Int32(14)
		g.Copy.PassedCopyInfo[0].Star = proto.Int32(4)
	})
	deny(request, 100)
	mutate(func(p *pb.Player, g *model.GameState) {
		g.Copy.PassedCopyInfo[0].Star = proto.Int32(5)
		g.Run = &model.CopyRun{Chapter: "1", Mission: "1"}
	})
	deny(request, 100)
	mutate(func(p *pb.Player, g *model.GameState) { g.Run = nil; p.Stamina = proto.Int32(9) })
	deny(request, 100)
	mutate(func(p *pb.Player, g *model.GameState) { p.Stamina = proto.Int32(100) })
	// 相同包并发均返回同一增量，资源与挑战次数只提交一笔。
	var wg sync.WaitGroup
	results := make(chan *pb.SCCopySweep, 8)
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			messages, err := s.copySweepAt(ctx, account.Id, request, 100, now)
			errs <- err
			if err == nil {
				results <- messages[0].(*pb.SCCopySweep)
			}
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var first *pb.SCCopySweep
	for result := range results {
		if first == nil {
			first = result
		}
		if !proto.Equal(first, result) {
			t.Fatal("replay mismatch")
		}
	}
	p, g := load()
	coin := int64(tableNumber(s.table("CopyExp", "level", "14"), "coin"))
	exp := int64(tableNumber(s.table("CopyExp", "level", "14"), "exp"))
	if p.GetCoin() != coin || p.GetStamina() != 90 || p.GetExp() != exp || g.Copy.PassedCopyInfo[0].GetChallengeTimes() != 1 || len(first.GetGain().GetWars()) != 0 {
		t.Fatal("per packet accounting", p, g.Copy, first)
	}
	deny(request, 99)
	deny(&pb.CSCopySweep{ChapterId: proto.String("1"), MissionId: proto.String("2")}, 100)
	// 请求的回执与奖表快照重启后保持一致，热改表不能改变旧回执。
	s.table("CopyExp", "level", "14")["coin"] = "999999"
	connection.Close()
	if err := db.InitGame(ctx, path); err != nil {
		t.Fatal(err)
	}
	connection, _ = db.GetIns().DB()
	messages, err := s.copySweepAt(ctx, account.Id, request, 100, now)
	if err != nil || !proto.Equal(first, messages[0]) {
		t.Fatal("restart replay", err)
	}
	p, g = load()
	if p.GetCoin() != coin || g.Copy.PassedCopyInfo[0].GetChallengeTimes() != 1 {
		t.Fatal("restart duplicate")
	}
	s.table("CopyExp", "level", "14")["coin"] = strconv.FormatInt(coin, 10)
	// 第五关沿原表每天3次；下一北京时间日期重置，跨日同包继续复用旧回执。
	mutate(func(p *pb.Player, g *model.GameState) {
		for _, info := range g.Copy.PassedCopyInfo {
			if info.GetMissionId() == "5" {
				info.Star = proto.Int32(5)
				info.ChallengeTimes = proto.Int32(2)
			}
		}
	})
	five := &pb.CSCopySweep{ChapterId: proto.String("1"), MissionId: proto.String("5")}
	if _, err := s.copySweepAt(ctx, account.Id, five, 101, now); err != nil {
		t.Fatal(err)
	}
	deny(five, 102)
	next := now.Add(time.Minute)
	if _, err := s.copySweepAt(ctx, account.Id, five, 102, next); err != nil {
		t.Fatal(err)
	}
	p, g = load()
	if g.CopyChallengeDay != "2026-10-13" || g.Copy.PassedCopyInfo[4].GetChallengeTimes() != 1 {
		t.Fatal("UTC+8 boundary", g.Copy)
	}
	priorCoin := p.GetCoin()
	if _, err := s.copySweepAt(ctx, account.Id, five, 102, next.Add(24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	p, g = load()
	if p.GetCoin() != priorCoin || g.Copy.PassedCopyInfo[4].GetChallengeTimes() != 0 {
		t.Fatal("cross day replay counted")
	}
	// VIP2 提供另一条原客户端允许的入口，低于14级也可扫荡已五星关卡。
	mutate(func(p *pb.Player, g *model.GameState) {
		p.Level = proto.Int32(13)
		p.Vip = proto.Int32(2)
		p.Exp = proto.Int64(0)
		p.Stamina = proto.Int32(100)
	})
	if _, err := s.copySweepAt(ctx, account.Id, request, 103, next.Add(24*time.Hour)); err != nil {
		t.Fatal("VIP alternative", err)
	}
	mutate(func(p *pb.Player, g *model.GameState) { p.Coin = proto.Int64(math.MaxInt64) })
	deny(request, 104)
	// 升级奖励逐级验证 int32 上限，失败包保留已持久化的上一次回执。
	mutate(func(p *pb.Player, g *model.GameState) {
		p.Coin = proto.Int64(0)
		p.Level = proto.Int32(14)
		p.Exp = proto.Int64(279)
		p.Energy = proto.Int32(math.MaxInt32)
	})
	deny(request, 104)
	mutate(func(p *pb.Player, g *model.GameState) {
		p.Energy = proto.Int32(100)
		p.Stamina = proto.Int32(math.MaxInt32)
	})
	deny(request, 104)
	mutate(func(p *pb.Player, g *model.GameState) {
		p.Stamina = proto.Int32(100)
		g.Copy.PassedCopyInfo = append(g.Copy.PassedCopyInfo, nil)
	})
	deny(request, 104)
	mutate(func(p *pb.Player, g *model.GameState) {
		g.Copy.PassedCopyInfo = g.Copy.PassedCopyInfo[:len(g.Copy.PassedCopyInfo)-1]
		g.Copy.PassedCopyInfo = append(g.Copy.PassedCopyInfo, proto.Clone(g.Copy.PassedCopyInfo[0]).(*pb.CopySimpleInfo))
	})
	deny(request, 104)
	mutate(func(p *pb.Player, g *model.GameState) {
		g.Copy.PassedCopyInfo = g.Copy.PassedCopyInfo[:len(g.Copy.PassedCopyInfo)-1]
	})
	// 经验阈值损坏必须拒绝并完整回滚，修复表后相同时间戳可重试成功。
	s.table("UserExp", "level", "14")["exp"] = "0"
	deny(request, 104)
	s.table("UserExp", "level", "14")["exp"] = "280"
	if _, err := s.copySweepAt(ctx, account.Id, request, 104, next.Add(24*time.Hour)); err != nil {
		t.Fatal("retry after repair", err)
	}
	p, _ = load()
	if p.GetLevel() != 15 || p.GetExp() != 139 || p.GetStamina() != 110 || p.GetEnergy() != 110 {
		t.Fatal("upgrade resource accounting", p)
	}
	// 原表100级以 -1 截止且资源列空白，跨入满级只发明示的金币和经验。
	mutate(func(p *pb.Player, g *model.GameState) {
		p.Level = proto.Int32(99)
		p.Exp = proto.Int64(1733489)
		p.Stamina = proto.Int32(100)
		p.Energy = proto.Int32(100)
	})
	if _, err := s.copySweepAt(ctx, account.Id, request, 105, next.Add(24*time.Hour)); err != nil {
		t.Fatal("terminal level", err)
	}
	p, _ = load()
	if p.GetLevel() != 100 || p.GetExp() != 1979 || p.GetStamina() != 90 || p.GetEnergy() != 100 {
		t.Fatal("terminal level accounting", p)
	}
}
