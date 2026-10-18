package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p4Setup(t *testing.T) *p1Fixture {
	t.Helper()
	f := &p1Fixture{ctx: context.Background(), path: filepath.Join(t.TempDir(), "p4.db")}
	if name := os.Getenv("HYJS_P4_MYSQL_DATABASE"); name != "" {
		if !strings.HasPrefix(name, "hyjs_test_p4_") {
			t.Fatal("P4 requires isolated schema")
		}
		f.mysql = db.MySQLConfig{Host: os.Getenv("HYJS_P4_MYSQL_HOST"), User: os.Getenv("HYJS_P4_MYSQL_USER"), Password: os.Getenv("HYJS_P4_MYSQL_PASSWORD"), Database: name}
	}
	f.open(t)
	f.s = &GameService{}
	if err := f.s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if f.mysql.Database != "" && db.GetIns().Migrator().HasTable(&db.P4ActivityRegistry{}) {
		f.s.P1Ready = true
		f.s.P4Ready = true
	} else {
		for _, initialize := range []func(context.Context) error{f.s.InitializeGameplayPools, f.s.InitializeShop, f.s.InitializeP1, f.s.InitializeP4} {
			if err := initialize(f.ctx); err != nil {
				t.Fatal(err)
			}
		}
	}
	// 专属fixture清空P4实例配置，避免同一MySQL库中的配置故障用例污染后续用例。
	for _, table := range []any{&db.P4WelfareRule{}, &db.P4WelfareState{}, &db.P4Receipt{}, &db.P4PlayerStats{}, &db.P4CommerceRule{}, &db.P4CommerceState{}, &db.P4CommerceStock{}, &db.P4RecruitRotation{}, &db.P4ActivityRegistry{}, &db.P15SeasonalBundle{}} {
		if err := db.GetIns().Where("1 = 1").Delete(table).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, seed := range []func(*gorm.DB, time.Time) error{f.s.seedP4Welfare, f.s.seedP4Commerce, f.s.seedP4Periods} {
		if err := seed(db.GetIns().WithContext(f.ctx), time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.s.seedP4Registry(db.GetIns().WithContext(f.ctx)); err != nil {
		t.Fatal(err)
	}
	a, err := LoginAccount(f.ctx, "p4-"+t.Name()+fmt.Sprint(time.Now().UnixNano()), "isolated-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(f.ctx, a.Id, "福利商店回归", "hero_043"); err != nil {
		t.Fatal(err)
	}
	f.id = a.Id
	t.Cleanup(func() { d, _ := db.GetIns().DB(); d.Close() })
	return f
}
func p4At(day, hour int) time.Time {
	now := time.Now().In(time.FixedZone("UTC+8", 8*3600))
	return time.Date(now.Year(), now.Month(), day, hour, 0, 0, 0, now.Location())
}
func p4Call(t *testing.T, f *p1Fixture, b proto.Message, stamp int64, at time.Time) proto.Message {
	t.Helper()
	r, e := f.s.p4Welfare(f.ctx, f.id, b, stamp, at)
	if e != nil {
		t.Fatal(e)
	}
	return r[0]
}
func p4LoginFact(t *testing.T, f *p1Fixture, at time.Time) {
	t.Helper()
	f.event(t, "login:"+p1Day(at), constUtil.P1EventLoginDay, 1, at)
}

func TestP4LoginFactsFrozenChoicesReplayAndRestart(t *testing.T) {
	f := p4Setup(t)
	at := p4At(1, 10)
	r := p4Call(t, f, &pb.CSLoginRewardInfo{}, 0, at).(*pb.SCLoginRewardInfo)
	if r.GetHasReward() != 0 || len(r.GetItems()) != 10 {
		t.Fatal("query fabricated login or missing calendar")
	}
	if _, e := f.s.p4Welfare(f.ctx, f.id, &pb.CSGetLoginReward{}, 1, at); e == nil {
		t.Fatal("unearned login")
	}
	for d := 1; d <= 10; d++ {
		p4LoginFact(t, f, p4At(d, 10))
	}
	if e := db.GetIns().Model(&db.P4WelfareRule{}).Where("kind=?", constUtil.P4WelfareLogin).Update("rules", []byte(`{"days":[{"rewards":"coin,999999"}]}`)).Error; e != nil {
		t.Fatal(e)
	}
	for d := 1; d <= 9; d++ {
		p4Call(t, f, &pb.CSGetLoginReward{}, int64(d), p4At(10, 10))
	}
	if _, e := f.s.p4Welfare(f.ctx, f.id, &pb.CSGetLoginReward{}, 10, p4At(10, 10)); e == nil {
		t.Fatal("choice absent")
	}
	request := &pb.CSGetLoginReward{ItemId: proto.String("item_021")}
	first := p4Call(t, f, request, 10, p4At(10, 10))
	before, _ := f.profile(t)
	f.restart(t)
	if e := f.s.InitializeP4(f.ctx); e != nil {
		t.Fatal(e)
	}
	second := p4Call(t, f, request, 10, p4At(11, 10))
	after, g := f.profile(t)
	if !proto.Equal(first, second) || after.GetCoin() != before.GetCoin() || simpleOwned(after, g, "item_021") != 3 || after.GetCoin() != 1000 {
		t.Fatal("frozen choices/replay/restart")
	}
	if _, e := f.s.p4Welfare(f.ctx, f.id, &pb.CSGetLoginReward{ItemId: proto.String("item_048")}, 10, p4At(11, 10)); e == nil {
		t.Fatal("stamp payload collision accepted")
	}
}

func TestP4LimitDailyAndMealBoundariesAtomicOverflow(t *testing.T) {
	f := p4Setup(t)
	p4Call(t, f, &pb.CSReceiveLimitMonthSign{}, 1, p4At(1, 10))
	again := p4Call(t, f, &pb.CSReceiveLimitMonthSign{}, 2, p4At(1, 11)).(*pb.SCReceiveLimitMonthSign)
	if len(again.GetGet().GetSimps()) != 0 {
		t.Fatal("limit daily duplicated")
	}
	p4Call(t, f, &pb.CSReceiveLimitMonthSign{}, 3, p4At(2, 10))
	before, g := f.profile(t)
	if simpleOwned(before, g, "item_016") != 1 || before.GetCoin() != 1000 {
		t.Fatal("limit next slot")
	}
	if _, e := f.s.p4Welfare(f.ctx, f.id, &pb.CSStaminaGet{}, 1, p4At(2, 11)); e == nil {
		t.Fatal("meal outside window")
	}
	first := p4Call(t, f, &pb.CSStaminaGet{}, 1, p4At(2, 12)).(*pb.SCStaminaGet)
	if first.GetDataList()[0].GetStatus() != 2 || first.GetDataList()[1].GetStatus() != 0 {
		t.Fatal("meal state")
	}
	if _, e := f.s.p4Welfare(f.ctx, f.id, &pb.CSStaminaGet{}, 2, p4At(2, 13)); e == nil {
		t.Fatal("meal same window duplicated")
	}
	p4Call(t, f, &pb.CSStaminaGet{}, 2, p4At(2, 18))
	after, _ := f.profile(t)
	if after.GetStamina() != before.GetStamina()+100 {
		t.Fatal("two meal awards")
	}
	if e := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error { p.Stamina = proto.Int32(math.MaxInt32); return nil }); e != nil {
		t.Fatal(e)
	}
	if _, e := f.s.p4Welfare(f.ctx, f.id, &pb.CSStaminaGet{}, 3, p4At(3, 12)); e == nil {
		t.Fatal("overflow meal")
	}
	var receipts int64
	db.GetIns().Model(&db.P4Receipt{}).Where("player_id=? AND domain=?", f.id, "welfare:meal").Count(&receipts)
	if receipts != 2 {
		t.Fatal("failed meal persisted")
	}
}

func TestP4CatCooldownDailyTotalAndConcurrentFinal(t *testing.T) {
	f := p4Setup(t)
	request := &pb.CSFortuneCatReward{Type: proto.Int32(0)}
	stamp := int64(1)
	for day := 1; day <= 5; day++ {
		for _, hour := range []int{8, 10, 12} {
			at := p4At(day, hour)
			first := p4Call(t, f, request, stamp, at)
			again := p4Call(t, f, request, stamp, at.Add(time.Minute))
			if !proto.Equal(first, again) {
				t.Fatal("cat replay")
			}
			stamp++
			if day < 5 || hour < 12 {
				if _, e := f.s.p4Welfare(f.ctx, f.id, request, stamp, at.Add(time.Minute)); e == nil {
					t.Fatal("cat cooldown")
				}
			}
		}
		if day < 5 {
			if _, e := f.s.p4Welfare(f.ctx, f.id, request, stamp, p4At(day, 18)); e == nil {
				t.Fatal("cat day cap")
			}
		}
	}
	before, g := f.profile(t)
	if simpleOwned(before, g, "item_005") != 15 || before.GetCoin() != 0 {
		t.Fatal("exploration unexpectedly paid pool")
	}
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := f.s.p4Welfare(f.ctx, f.id, &pb.CSFortuneCatReward{Type: proto.Int32(1)}, stamp, p4At(5, 12))
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
	after, g := f.profile(t)
	if after.GetCoin() < 45000 || after.GetCoin() > 150000 || simpleOwned(after, g, "item_021") != 1 {
		t.Fatal("cat final multiplicity")
	}
	if _, e := f.s.p4Welfare(f.ctx, f.id, &pb.CSFortuneCatReward{Type: proto.Int32(1)}, stamp+1, p4At(5, 12)); e == nil {
		t.Fatal("cat repeat final")
	}
	if p1Count(t, &db.EventFact{}, "player_id=? AND kind=?", f.id, "fortune_explore") != 15 {
		t.Fatal("cat fact count")
	}
}

func TestP4WelfareBadRulesNoStateOrAward(t *testing.T) {
	f := p4Setup(t)
	var cfg db.P4WelfareRule
	if e := db.GetIns().Where("kind=?", constUtil.P4WelfareLimit).First(&cfg).Error; e != nil {
		t.Fatal(e)
	}
	bad, _ := json.Marshal(p4WelfareRules{Days: []p4SignDay{{Rewards: "coin,1;diamond,1"}}})
	db.GetIns().Model(&cfg).Update("rules", bad)
	if _, e := f.s.p4Welfare(f.ctx, f.id, &pb.CSReceiveLimitMonthSign{}, 1, p4At(1, 10)); e == nil {
		t.Fatal("unsupported multiple limit resources")
	}
	if p1Count(t, &db.P4WelfareState{}, "player_id=?", f.id) != 0 {
		t.Fatal("invalid config changed state")
	}
	p, _ := f.profile(t)
	if p.GetCoin() != 0 {
		t.Fatal("invalid config award")
	}
}

// TestP4MonthGraceKeepsEarnedOldRewards 验证无实例ID协议在月切换后可补领旧档，领完切入新档。
func TestP4MonthGraceKeepsEarnedOldRewards(t *testing.T) {
	f := p4Setup(t)
	at := p4At(1, 10)
	p4LoginFact(t, f, at)
	p4Call(t, f, &pb.CSLoginRewardInfo{}, 0, at)
	local := at.In(time.FixedZone("UTC+8", 8*3600))
	next := time.Date(local.Year(), local.Month()+1, 1, 10, 0, 0, 0, local.Location())
	old := p4Call(t, f, &pb.CSLoginRewardInfo{}, 0, next).(*pb.SCLoginRewardInfo)
	if old.GetHasReward() != 1 || old.GetItems()[0].GetState() != 1 {
		t.Fatal("old earned calendar hidden by new month")
	}
	p4Call(t, f, &pb.CSGetLoginReward{}, 1, next)
	current := p4Call(t, f, &pb.CSLoginRewardInfo{}, 0, next).(*pb.SCLoginRewardInfo)
	if current.GetHasReward() != 0 || current.GetItems()[0].GetState() != 0 {
		t.Fatal("completed old calendar blocked current month")
	}
	// 隔离存档准备已探索15次的终奖，检验月切换只领取一次并保留当前新池。
	p4Call(t, f, &pb.CSFortuneCatInfo{}, 0, at)
	var row db.P4WelfareState
	if err := db.GetIns().Where("player_id=? AND rule_id=?", f.id, "p4:cat:"+at.Format("200601")).First(&row).Error; err != nil {
		t.Fatal(err)
	}
	var state p4WelfareProgress
	if err := json.Unmarshal(row.Payload, &state); err != nil {
		t.Fatal(err)
	}
	state.CatTotal = 15
	state.CoinPool = 45000
	raw, _ := json.Marshal(state)
	if err := db.GetIns().Model(&row).Update("payload", raw).Error; err != nil {
		t.Fatal(err)
	}
	cat := p4Call(t, f, &pb.CSFortuneCatInfo{}, 0, next).(*pb.SCFortuneCatInfo)
	if cat.GetInfo().GetTotalStatus() != 1 || cat.GetInfo().GetCoinPool() != 45000 {
		t.Fatal("old completed exploration hidden")
	}
	request := &pb.CSFortuneCatReward{Type: proto.Int32(1)}
	first := p4Call(t, f, request, 1, next)
	if !proto.Equal(first, p4Call(t, f, request, 1, next.Add(time.Hour))) {
		t.Fatal("old final receipt changed after month transition")
	}
	cat = p4Call(t, f, &pb.CSFortuneCatInfo{}, 0, next).(*pb.SCFortuneCatInfo)
	if cat.GetInfo().GetTotalLeft() != 15 || cat.GetInfo().GetCoinPool() != 0 {
		t.Fatal("old final blocked current exploration")
	}
	p, g := f.profile(t)
	if p.GetCoin() != 46000 || simpleOwned(p, g, "item_021") != 1 {
		t.Fatal("grace award duplicated")
	}
}
