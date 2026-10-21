package service

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// 新增交易回归可由主代理在共享MySQL申请的独立库中串行执行。
func feedback40Database(t *testing.T, filename string) *p1Fixture {
	t.Helper()
	f := &p1Fixture{ctx: context.Background(), path: filepath.Join(t.TempDir(), filename)}
	if name := os.Getenv("HYJS_P1_MYSQL_DATABASE"); name != "" {
		if !strings.HasPrefix(name, "hyjs_test_p1_") {
			t.Fatal("feedback40 requires an isolated project test database")
		}
		f.mysql = db.MySQLConfig{Host: os.Getenv("HYJS_P1_MYSQL_HOST"), User: os.Getenv("HYJS_P1_MYSQL_USER"), Password: os.Getenv("HYJS_P1_MYSQL_PASSWORD"), Database: name}
	}
	f.open(t)
	t.Cleanup(func() {
		connection, err := db.GetIns().DB()
		if err == nil {
			connection.Close()
		}
	})
	return f
}

func kingLuck40Setup(t *testing.T) (*GameService, context.Context) {
	t.Helper()
	f := feedback40Database(t, "luck40.db")
	s := &GameService{}
	if err := s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := s.InitializeDailyCopy(f.ctx); err != nil {
		t.Fatal(err)
	}
	initializeP2Test(t, s, f.ctx)
	return s, f.ctx
}

// 原抽卡、幸运兑换及重放必须共用持久余额，门槛调整不重置历史余额。
func TestKingLuck40AccumulateExchangeAndReplay(t *testing.T) {
	s, ctx := kingLuck40Setup(t)
	id := dailyCopyTestPlayer(t, ctx, "king-luck40", 20)
	p2Funding(t, s, ctx, id, "diamond,30000")
	_, world, _, err := s.kingConfiguration(db.GetIns(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = updateProfile(ctx, id, func(_ *pb.Player, g *model.GameState) error {
		g.KingRecruitLuck = map[int32]int32{world: 490, world%3 + 1: 730}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	first, err := s.warriorRecruit(ctx, id, p2Recruit(2, 0), 40001)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := s.warriorRecruit(ctx, id, p2Recruit(2, 0), 40001)
	if err != nil || !proto.Equal(first[0], replay[0]) {
		t.Fatal("draw replay", err)
	}
	if info := recruitRuntimeInfo(t, s, ctx, id, 3); info.GetLuck() != 590 {
		t.Fatal("balance clipped at exchange threshold", info)
	}
	if err = AdminInitialize(ctx, ""); err != nil {
		t.Fatal(err)
	}
	operator := db.AdminUser{Username: "luck40-operator", PasswordHash: "unused-fixture", Role: constUtil.AdminRoleOperator, Enabled: true}
	if err = db.GetIns().Create(&operator).Error; err != nil {
		t.Fatal(err)
	}
	actor := &model.AdminPrincipal{UserID: operator.ID, Username: operator.Username, Role: operator.Role}
	current, err := s.AdminRecruitSettings(ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	if current.Items[1].LuckExchangeCost != 500 {
		t.Fatal("legacy default", current.Items)
	}
	current.Items[1].LuckExchangeCost = 300
	saved, err := s.AdminSaveRecruitSettings(ctx, actor, model.AdminRecruitSettingsSaveRequest{Version: current.Version, Reason: "原抽卡兑换回归", Items: current.Items})
	if err != nil || saved.Items[1].LuckExchangeCost != 300 {
		t.Fatal("configured exchange threshold", saved, err)
	}
	_, rewards, err := s.loadGameplayPool(db.GetIns(), "recruit_3")
	if err != nil {
		t.Fatal(err)
	}
	_, red := s.kingRewards(rewards, world)
	request := &pb.CSWarriorRecruitLuckyReward{Reward: proto.String(red[0])}
	claim, err := s.kingRecruitLucky(ctx, id, request, 40002)
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.kingRecruitLucky(ctx, id, request, 40002)
	if err != nil || !proto.Equal(claim[0], again[0]) {
		t.Fatal("exchange replay", err)
	}
	_, game := p2Profile(t, id)
	if game.KingRecruitLuck[world] != 290 || game.KingRecruitLuck[world%3+1] != 730 {
		t.Fatal("threshold deducted or worlds mixed", game.KingRecruitLuck)
	}
	if _, err = s.kingRecruitLucky(ctx, id, request, 40003); err == nil {
		t.Fatal("exchange accepted insufficient balance")
	}
	if err = s.InitializeRecruitRuntime(ctx); err != nil {
		t.Fatal(err)
	}
	fresh, err := s.AdminRecruitSettings(ctx, actor)
	if err != nil || fresh.Items[1].LuckExchangeCost != 300 {
		t.Fatal("restart replaced configured threshold", fresh, err)
	}
}

// 协议整数溢出必须回滚全部抽卡消耗，不能裁剪或写成负数。
func TestKingLuck40OverflowRollsBack(t *testing.T) {
	s, ctx := kingLuck40Setup(t)
	id := dailyCopyTestPlayer(t, ctx, "king-overflow40", 20)
	p2Funding(t, s, ctx, id, "diamond,30000")
	_, world, _, err := s.kingConfiguration(db.GetIns(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = updateProfile(ctx, id, func(_ *pb.Player, g *model.GameState) error {
		g.KingRecruitLuck = map[int32]int32{world: math.MaxInt32 - 5}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	before, gameBefore := p2Profile(t, id)
	if _, err = s.warriorRecruit(ctx, id, p2Recruit(1, 0), 40004); err == nil {
		t.Fatal("overflow accepted")
	}
	after, gameAfter := p2Profile(t, id)
	if !proto.Equal(before, after) || len(gameBefore.Warriors) != len(gameAfter.Warriors) || gameAfter.KingRecruitLuck[world] != math.MaxInt32-5 {
		t.Fatal("overflow changed resources or balance")
	}
}
