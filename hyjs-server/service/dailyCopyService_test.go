package service

import (
	"context"
	"math"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func newDailyCopyTest(t *testing.T) (*GameService, context.Context, string) {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "daily-copy.db")
	if err := db.InitGame(ctx, path); err != nil {
		t.Fatal(err)
	}
	connection, _ := db.GetIns().DB()
	t.Cleanup(func() { connection.Close() })
	s := &GameService{}
	if err := s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := s.InitializeDailyCopy(ctx); err != nil {
		t.Fatal(err)
	}
	return s, ctx, path
}

func dailyCopyTestPlayer(t *testing.T, ctx context.Context, uid string, level int32) int64 {
	t.Helper()
	account, err := LoginAccount(ctx, uid, "password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CreatePlayer(ctx, account.Id, "资源副本", "hero_043"); err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(ctx, account.Id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(level)
		p.Stamina = proto.Int32(200)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return account.Id
}

func dailyCopyWin(start *pb.SCDailyCopyFightStart, chapter string) *pb.CSDailyCopyFightEnd {
	return &pb.CSDailyCopyFightEnd{ChapterId: proto.String(chapter), FightId: start.FightId, IsWin: proto.Int32(1), OptRecord: &pb.BattleOperate{FinalRoundNum: proto.Int32(1), AttackerTotalLeftHp: proto.Float64(1), DefenderTotalLeftHp: proto.Float64(0)}}
}

// TestDailyCopyCalendar 验证三个难度的等级门槛及原表所有开放日，周日使用7且日切按UTC+8。
func TestDailyCopyCalendar(t *testing.T) {
	s, ctx, _ := newDailyCopyTest(t)
	id := dailyCopyTestPlayer(t, ctx, "daily-copy-calendar", 1)
	for day := 1; day <= 7; day++ {
		now := time.Date(2026, 10, 11+day, 0, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
		messages, err := s.dailyCopyAt(ctx, id, &pb.CSDailyCopyState{}, 1, now)
		if err != nil {
			t.Fatal(err)
		}
		info := messages[0].(*pb.SCDailyCopyState)
		if len(info.Items) != 16 {
			t.Fatal("original types omitted", info)
		}
		for _, item := range info.Items {
			rule := s.table("DailyCopy", "Type", item.GetType())
			wantOpen := strings.Contains(","+rule["OpenDay"]+",", ","+strconv.Itoa(day)+",")
			wantCount, _ := strconv.Atoi(rule["Count"])
			if (item.GetIsOpen() == 1) != wantOpen || int(item.GetLastTime()) != wantCount || len(item.DailyCopyState) != 3 {
				t.Fatal("weekday or type quota", day, item)
			}
			for _, chapter := range item.DailyCopyState {
				if chapter.GetIsOpen() != 0 || chapter.GetLevelLimit() < 26 || chapter.GetDrop() == "" {
					t.Fatal("level gate or drop preview", chapter)
				}
			}
		}
	}
	if got := dailyCopyDay(time.Date(2026, 10, 12, 15, 59, 59, 0, time.UTC)); got != "2026-10-12" {
		t.Fatal(got)
	}
	if got := dailyCopyDay(time.Date(2026, 10, 12, 16, 0, 0, 0, time.UTC)); got != "2026-10-13" {
		t.Fatal(got)
	}
}

// TestDailyCopyAllOriginalChapters 每个原始难度都实际启动并结算，验证实例类型与概率数量范围。
func TestDailyCopyAllOriginalChapters(t *testing.T) {
	s, ctx, _ := newDailyCopyTest(t)
	if len(s.Tables["DailyCopyChapter"]) != 48 {
		t.Fatal("chapter count", len(s.Tables["DailyCopyChapter"]))
	}
	for _, chapter := range s.Tables["DailyCopyChapter"] {
		t.Run(chapter["ChapterId"], func(t *testing.T) {
			id := dailyCopyTestPlayer(t, ctx, "daily-copy-chapter-"+chapter["ChapterId"], 100)
			now := time.Date(2026, 10, 18, 4, 0, 0, 0, time.UTC)
			if strings.HasPrefix(chapter["Type"], "PetFood_") {
				now = now.Add(-24 * time.Hour)
			}
			messages, err := s.dailyCopyAt(ctx, id, &pb.CSDailyCopyFightStart{ChapterId: proto.String(chapter["ChapterId"])}, 100, now)
			if err != nil {
				t.Fatal(err)
			}
			start := messages[0].(*pb.SCDailyCopyFightStart)
			var count int
			for _, enemy := range s.Tables["DailyCopyEnemy"] {
				if enemy["ChapterId"] == chapter["ChapterId"] {
					count++
					found := false
					for _, actor := range start.GetMonster().GetFormation() {
						if actor.GetId() == enemy["HeroId"] && actor.GetPose() == int32(tableNumber(enemy, "Location")) && actor.GetProp().GetHp() == tableNumber(enemy, "Hp") && actor.GetProp().GetAtt() == tableNumber(enemy, "Attack") {
							found = true
						}
					}
					if !found {
						t.Fatal("original enemy omitted", enemy["CopyEnemyId"], start)
					}
				}
			}
			if len(start.GetMonster().GetFormation()) != count || start.GetRoomId() != "map_forest_002" || start.GetMaxRoundNum() != 40 {
				t.Fatal("battle snapshot", start)
			}
			var fight db.DailyCopyFight
			if err := db.GetIns().First(&fight, "fight_id = ?", start.GetFightId()).Error; err != nil {
				t.Fatal(err)
			}
			drops, err := s.dailyCopyDrops(chapter)
			if err != nil {
				t.Fatal(err)
			}
			drawn := map[string]int64{}
			for _, part := range strings.Split(fight.Reward, ";") {
				if part != "" {
					pair := strings.Split(part, ",")
					drawn[pair[0]], _ = strconv.ParseInt(pair[1], 10, 32)
				}
			}
			for _, drop := range drops {
				found := false
				for _, option := range drop.Options {
					found = found || option[0] == drawn[drop.ID]
				}
				if !found {
					t.Fatal("reward outside original weights", drop, fight.Reward)
				}
			}
			result, err := s.dailyCopyAt(ctx, id, dailyCopyWin(start, chapter["ChapterId"]), 101, now)
			if err != nil {
				t.Fatal(err)
			}
			gain := result[0].(*pb.SCDailyCopyFightEnd)
			if gain.GetIsWin() != 1 || gain.GetStarNum() != 5 || gain.GetLastTime() != int32(tableNumber(s.table("DailyCopy", "Type", chapter["Type"]), "Count"))-1 {
				t.Fatal(gain)
			}
			for _, warrior := range gain.GetGet().GetWars() {
				if warrior.GetId() != "hero_exp_004" || warrior.GetGuid() == "" {
					t.Fatal("experience materials not instantiated", warrior)
				}
			}
			if chapter["Type"] == "HeroExp" && len(gain.GetGet().GetWars()) != int(drawn["hero_exp_004"]) {
				t.Fatal("missing experience materials", gain)
			}
			if chapter["Type"] == "TreasureExp" && len(gain.GetGet().GetTreas()) != int(drawn["trea_010"]) {
				t.Fatal("missing treasure materials", gain)
			}
		})
	}
}

// TestDailyCopyTransactions 验证并发重放、失败回滚、跨日、重启及已冻结战斗不受规则更新影响。
func TestDailyCopyTransactions(t *testing.T) {
	s, ctx, path := newDailyCopyTest(t)
	id := dailyCopyTestPlayer(t, ctx, "daily-copy-transactions", 100)
	now := time.Date(2026, 10, 12, 15, 59, 0, 0, time.UTC)
	request := &pb.CSDailyCopyFightStart{ChapterId: proto.String("9")}
	var wg sync.WaitGroup
	starts := make(chan *pb.SCDailyCopyFightStart, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			messages, err := s.dailyCopyAt(ctx, id, request, 100, now)
			if err != nil {
				t.Error(err)
				return
			}
			starts <- messages[0].(*pb.SCDailyCopyFightStart)
		}()
	}
	wg.Wait()
	close(starts)
	var start *pb.SCDailyCopyFightStart
	for got := range starts {
		if start == nil {
			start = got
		} else if !proto.Equal(start, got) {
			t.Fatal("duplicate start snapshot")
		}
	}
	if start == nil {
		t.Fatal("no fight created")
	}
	var profile db.Profile
	if err := db.GetIns().First(&profile, id).Error; err != nil {
		t.Fatal(err)
	}
	p, _, err := decodeProfile(&profile)
	if err != nil || p.GetStamina() != 180 {
		t.Fatal("duplicate stamina charge", p, err)
	}
	// 同日重新进入原章节即使使用新包时间戳，也复用当前未结算战斗。
	resumed, err := s.dailyCopyAt(ctx, id, request, 101, now)
	if err != nil || !proto.Equal(resumed[0], start) {
		t.Fatal("active fight not restored", err)
	}
	for _, bad := range []*pb.CSDailyCopyFightStart{{ChapterId: proto.String("10")}, {ChapterId: proto.String("999")}} {
		if _, err := s.dailyCopyAt(ctx, id, bad, 102, now); err == nil {
			t.Fatal("parallel different chapter accepted")
		}
	}
	win := dailyCopyWin(start, "9")
	for _, hp := range []float64{-1, math.Inf(1), math.NaN(), math.MaxFloat64} {
		bad := proto.Clone(win).(*pb.CSDailyCopyFightEnd)
		bad.OptRecord.AttackerTotalLeftHp = proto.Float64(hp)
		if _, err := s.dailyCopyAt(ctx, id, bad, 102, now); err == nil {
			t.Fatal("invalid HP accepted", hp)
		}
	}
	bad := proto.Clone(win).(*pb.CSDailyCopyFightEnd)
	bad.OptRecord.DefenderTotalLeftHp = proto.Float64(1)
	if _, err := s.dailyCopyAt(ctx, id, bad, 102, now); err == nil {
		t.Fatal("living defender counted as win")
	}
	if err := updateProfile(ctx, id, func(p *pb.Player, g *model.GameState) error { p.Coin = proto.Int64(math.MaxInt64); return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := s.dailyCopyAt(ctx, id, win, 102, now); err == nil {
		t.Fatal("overflow granted")
	}
	var fight db.DailyCopyFight
	if err := db.GetIns().First(&fight, "fight_id = ?", start.GetFightId()).Error; err != nil || fight.Settled {
		t.Fatal("failed reward consumed receipt", fight, err)
	}
	if err := updateProfile(ctx, id, func(p *pb.Player, g *model.GameState) error { p.Coin = proto.Int64(0); return nil }); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.DailyCopyPolicy{}).Where("id = ?", constUtil.DailyCopyPolicyID).Updates(map[string]any{"enabled": false, "win_stars": 1}).Error; err != nil {
		t.Fatal(err)
	}
	next := now.Add(time.Minute)
	ends := make(chan *pb.SCDailyCopyFightEnd, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			messages, err := s.dailyCopyAt(ctx, id, win, 102, next)
			if err != nil {
				t.Error(err)
				return
			}
			ends <- messages[0].(*pb.SCDailyCopyFightEnd)
		}()
	}
	wg.Wait()
	close(ends)
	var result *pb.SCDailyCopyFightEnd
	for got := range ends {
		if result == nil {
			result = got
		} else if !proto.Equal(got, result) {
			t.Fatal("end replay changed result")
		}
	}
	latest, err := s.dailyCopyAt(ctx, id, win, 102, next)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range latest[2].(*pb.SCDailyCopyState).Items {
		if item.GetType() == "Coin" && (item.GetLastTime() != 1 || item.GetIsOpen() != 0) {
			t.Fatal("cross-day settlement did not include current quota", item)
		}
	}
	if result == nil || result.GetStarNum() != 5 || result.GetLastTime() != 0 {
		t.Fatal("frozen policy changed", result)
	}
	resumed, err = s.dailyCopyAt(ctx, id, request, 101, next)
	if err != nil || !proto.Equal(resumed[0], start) {
		t.Fatal("re-entry packet replay created another fight", err)
	}
	if err := db.GetIns().First(&profile, id).Error; err != nil {
		t.Fatal(err)
	}
	p, _, _ = decodeProfile(&profile)
	if p.GetCoin() != 630000 || p.GetStamina() != 180 {
		t.Fatal("duplicate reward or charge", p)
	}
	bad = proto.Clone(win).(*pb.CSDailyCopyFightEnd)
	bad.IsWin = proto.Int32(0)
	if _, err := s.dailyCopyAt(ctx, id, bad, 103, next); err == nil {
		t.Fatal("conflicting replay accepted")
	}
	connection, _ := db.GetIns().DB()
	connection.Close()
	if err := db.InitGame(ctx, path); err != nil {
		t.Fatal(err)
	}
	connection, _ = db.GetIns().DB()
	t.Cleanup(func() { connection.Close() })
	if err := s.InitializeDailyCopy(ctx); err != nil {
		t.Fatal(err)
	}
	replayed, err := s.dailyCopyAt(ctx, id, win, 104, next)
	if err != nil || !proto.Equal(replayed[0], result) {
		t.Fatal("restart lost end receipt", err)
	}
	if err := db.GetIns().Model(&db.DailyCopyPolicy{}).Where("id = ?", constUtil.DailyCopyPolicyID).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	info, err := s.dailyCopyAt(ctx, id, &pb.CSDailyCopyState{}, 105, next)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range info[0].(*pb.SCDailyCopyState).Items {
		if item.GetLastTime() != int32(tableNumber(s.table("DailyCopy", "Type", item.GetType()), "Count")) {
			t.Fatal("old day quota leaked", item)
		}
	}
	// 周二可以进入饰品经验副本，周一的旧胜利不消耗该日次数。
	if _, err := s.dailyCopyAt(ctx, id, &pb.CSDailyCopyFightStart{ChapterId: proto.String("5")}, 200, next); err != nil {
		t.Fatal(err)
	}
}
