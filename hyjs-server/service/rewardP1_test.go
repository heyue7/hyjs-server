package service

import (
	"context"
	"encoding/json"
	"errors"
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

type p1Fixture struct {
	s     *GameService
	ctx   context.Context
	id    int64
	path  string
	mysql db.MySQLConfig
}

func p1Setup(t *testing.T) *p1Fixture {
	t.Helper()
	f := &p1Fixture{ctx: context.Background(), path: filepath.Join(t.TempDir(), "p1.db")}
	name := os.Getenv("HYJS_P1_MYSQL_DATABASE")
	if name != "" {
		if !strings.HasPrefix(name, "hyjs_test_p1_") {
			t.Fatal("P1 tests require an isolated database")
		}
		f.mysql = db.MySQLConfig{Host: os.Getenv("HYJS_P1_MYSQL_HOST"), User: os.Getenv("HYJS_P1_MYSQL_USER"), Password: os.Getenv("HYJS_P1_MYSQL_PASSWORD"), Database: name}
	}
	f.open(t)
	f.s = &GameService{}
	if err := f.s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP1(f.ctx); err != nil {
		t.Fatal(err)
	}
	account, err := LoginAccount(f.ctx, "p1-"+t.Name()+fmt.Sprint(time.Now().UnixNano()), "isolated-test-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(f.ctx, account.Id, "公共奖励回归", "hero_043"); err != nil {
		t.Fatal(err)
	}
	f.id = account.Id
	t.Cleanup(func() { sqlDB, _ := db.GetIns().DB(); sqlDB.Close() })
	return f
}
func (f *p1Fixture) open(t *testing.T) {
	t.Helper()
	var err error
	if f.mysql.Database != "" {
		err = db.InitGameMySQL(f.ctx, f.mysql)
	} else {
		err = db.InitGame(f.ctx, f.path)
	}
	if err != nil {
		t.Fatal(err)
	}
}
func (f *p1Fixture) restart(t *testing.T) {
	t.Helper()
	sqlDB, _ := db.GetIns().DB()
	sqlDB.Close()
	f.open(t)
	if err := f.s.InitializeP1(f.ctx); err != nil {
		t.Fatal(err)
	}
}
func (f *p1Fixture) profile(t *testing.T) (*pb.Player, *model.GameState) {
	t.Helper()
	var row db.Profile
	if err := db.GetIns().First(&row, f.id).Error; err != nil {
		t.Fatal(err)
	}
	p, g, err := decodeProfile(&row)
	if err != nil {
		t.Fatal(err)
	}
	return p, g
}
func (f *p1Fixture) reward(t *testing.T, source, content string, expiry int64) *db.RewardDelivery {
	t.Helper()
	var reward *db.RewardDelivery
	err := updateProfileWithTx(f.ctx, f.id, func(tx *gorm.DB, _ *pb.Player, _ *model.GameState) error {
		var err error
		reward, err = f.s.EnqueueRewardNotice(tx, f.id, source, "回归奖励", "隔离回归", content, expiry)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return reward
}
func (f *p1Fixture) event(t *testing.T, source, kind string, amount int64, at time.Time) {
	t.Helper()
	err := updateProfileWithTx(f.ctx, f.id, func(tx *gorm.DB, _ *pb.Player, _ *model.GameState) error {
		return f.s.RecordP1Event(tx, f.id, source, kind, amount, at)
	})
	if err != nil {
		t.Fatal(err)
	}
}
func p1Count(t *testing.T, table any, query string, args ...any) int64 {
	t.Helper()
	var count int64
	if err := db.GetIns().Model(table).Where(query, args...).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	return count
}
func p1Instance(t *testing.T, f *p1Fixture, key, kind, mode string, goal int64, end time.Time, rank int32) db.PeriodInstance {
	t.Helper()
	cfg := db.PeriodInstance{ID: key, Enabled: true, Title: "隔离活动", Kind: kind, Mode: mode, Goal: goal, Rewards: "coin,17;item_001,1", StartAt: time.Now().Add(-48 * time.Hour).UnixMilli(), EndAt: end.UnixMilli(), RewardEndAt: end.Add(time.Hour).UnixMilli(), RankPlaces: rank, RankRewards: "coin,13", Source: "隔离测试，不是原服概率规则"}
	if err := db.GetIns().Create(&cfg).Error; err != nil {
		t.Fatal(err)
	}
	return cfg
}
func p1ProgressValue(t *testing.T, f *p1Fixture, key string) db.PeriodProgress {
	t.Helper()
	var progress db.PeriodProgress
	if err := db.GetIns().Where("instance_id = ? AND player_id = ?", key, f.id).First(&progress).Error; err != nil {
		t.Fatal(err)
	}
	return progress
}

func TestP1FixedRewardsAtomicAndOriginalInstances(t *testing.T) {
	f := p1Setup(t)
	p, g := f.profile(t)
	gain, err := f.s.GrantFixedReward(p, g, "coin,7;diamond,3;energy,2;stamina,2;light,1;point,1;popularity,1;hero_001,1;hero_exp_001,1;equip_001,1;frag_hero_001,1;item_119,1")
	if err != nil {
		t.Fatal(err)
	}
	if p.GetCoin() != 7 || p.GetDiamond() != 3 || len(gain.Wars) != 2 || len(gain.Equips) != 1 || gain.Wars[0].GetGuid() == gain.Wars[1].GetGuid() {
		t.Fatal("mixed original reward incorrect")
	}
	beforeP := proto.Clone(p)
	beforeG, _ := json.Marshal(g)
	for _, invalid := range []string{"coin,1;unknown,1", "coin,1;diamond,-1", "coin,1;hero_001,101", "coin,1;item_119,2147483648"} {
		if _, err = f.s.GrantFixedReward(p, g, invalid); err == nil {
			t.Fatalf("accepted %s", invalid)
		}
		after, _ := json.Marshal(g)
		if !proto.Equal(p, beforeP) || string(after) != string(beforeG) {
			t.Fatal("partial in-memory award")
		}
	}
	p.Coin = proto.Int64(math.MaxInt64)
	if _, err = f.s.GrantFixedReward(p, g, "diamond,1;coin,1"); err == nil || p.GetDiamond() != 3 {
		t.Fatal("overflow changed earlier resource")
	}
	p.Coin = proto.Int64(7)
	p.EquipBagSize = proto.Int32(1)
	if _, err = f.s.GrantFixedReward(p, g, "coin,1;equip_001,1"); err == nil || p.GetCoin() != 7 {
		t.Fatal("bag capacity left partial coin")
	}
}

func TestP1OfflineRewardConcurrencyReplayAndRestart(t *testing.T) {
	f := p1Setup(t)
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- updateProfileWithTx(f.ctx, f.id, func(tx *gorm.DB, _ *pb.Player, _ *model.GameState) error {
				_, err := f.s.EnqueueRewardNotice(tx, f.id, "order:one", "离线补给", "业务来源", "coin,37;equip_001,1", 0)
				return err
			})
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if p1Count(t, &db.RewardDelivery{}, "player_id = ?", f.id) != 1 || p1Count(t, &db.MailMessage{}, "player_id = ?", f.id) != 1 {
		t.Fatal("source duplicated")
	}
	f.restart(t)
	result, err := f.s.RewardCenter(f.ctx, f.id, &pb.CSQueryUserReward{})
	if err != nil {
		t.Fatal(err)
	}
	list := result[0].(*pb.SCQueryUserReward).GetRewardItemInfoList()
	if len(list) != 1 || list[0].GetRewardList()[0].GetNum() != 37 {
		t.Fatal("offline reward missing")
	}
	reward := f.reward(t, "order:one", "diamond,999", 0)
	if reward.Rewards != "coin,37;equip_001,1" {
		t.Fatal("source replay overwrote frozen reward")
	}
	errs = make(chan error, 6)
	results := make(chan *pb.ResultObject, 6)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			messages, e := f.s.RewardCenter(f.ctx, f.id, &pb.CSAddRewardsToPlayer{RewardIds: []int64{reward.ID}})
			errs <- e
			if e == nil {
				results <- messages[0].(*pb.SCAddRewardsToPlayer).GetResult()
			}
		}()
	}
	wg.Wait()
	close(errs)
	close(results)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	awards := 0
	for gain := range results {
		if len(gain.GetEquips()) > 0 {
			awards++
		}
	}
	p, g := f.profile(t)
	if awards != 1 || p.GetCoin() != 37 || len(g.Equipment) != 1 {
		t.Fatal("concurrent claim paid twice")
	}
	f.restart(t)
	p, g = f.profile(t)
	if p.GetCoin() != 37 || len(g.Equipment) != 1 {
		t.Fatal("restart lost award")
	}
	messages, err := f.s.RewardCenter(f.ctx, f.id, &pb.CSAddRewardsToPlayer{RewardIds: []int64{reward.ID}})
	if err != nil || len(messages[0].(*pb.SCAddRewardsToPlayer).GetResult().GetSimps()) != 0 {
		t.Fatal("retry returned duplicate UI increment")
	}
}

func TestP1BatchOwnershipExpiryOverflowAndRollback(t *testing.T) {
	f := p1Setup(t)
	first := f.reward(t, "batch:first", "coin,10", 0)
	second := f.reward(t, "batch:second", "coin,2147483647;diamond,1", 0)
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error { p.Coin = proto.Int64(math.MaxInt64 - 20); return nil }); err != nil {
		t.Fatal(err)
	}
	claim := func(ids ...int64) error {
		_, err := f.s.RewardCenter(f.ctx, f.id, &pb.CSAddRewardsToPlayer{RewardIds: ids})
		return err
	}
	for _, ids := range [][]int64{{first.ID, first.ID}, {first.ID, 999999999}, {first.ID, second.ID}} {
		if claim(ids...) == nil {
			t.Fatal("invalid batch accepted")
		}
	}
	p, _ := f.profile(t)
	if p.GetCoin() != math.MaxInt64-20 || p.GetDiamond() != 0 || p1Count(t, &db.RewardDelivery{}, "player_id = ? AND claimed_at > 0", f.id) != 0 {
		t.Fatal("batch partially committed")
	}
	expired := f.reward(t, "expired", "coin,1", time.Now().Add(-time.Second).UnixMilli())
	if claim(expired.ID) == nil {
		t.Fatal("expired claimed")
	}
	other, err := LoginAccount(f.ctx, "p1-other"+fmt.Sprint(time.Now().UnixNano()), "isolated-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(f.ctx, other.Id, "其他角色", "hero_043"); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.RewardCenter(f.ctx, other.Id, &pb.CSAddRewardsToPlayer{RewardIds: []int64{first.ID}}); err == nil {
		t.Fatal("foreign reward claimed")
	}
}

func TestP1MailUnreadDeleteTombstoneAndExpiry(t *testing.T) {
	f := p1Setup(t)
	reward := f.reward(t, "mail:linked", "coin,1", 0)
	messages, err := f.s.Mailbox(f.ctx, f.id, &pb.CSCheckNewMail{})
	if err != nil || messages[0].(*pb.SCCheckNewMail).GetHasNew() != 1 {
		t.Fatal("unread missing")
	}
	messages, err = f.s.Mailbox(f.ctx, f.id, &pb.CSMailList{})
	if err != nil {
		t.Fatal(err)
	}
	boxes := messages[0].(*pb.SCMailList).GetMailBoxList()
	if len(boxes) != 1 || len(boxes[0].GetMailList()) != 1 {
		t.Fatal("mail missing")
	}
	mail := boxes[0].GetMailList()[0]
	if _, err = f.s.Mailbox(f.ctx, f.id, &pb.CSDeleteMail{Type: proto.Int32(mail.GetType()), CreateTime: proto.Int64(mail.GetTime())}); err == nil {
		t.Fatal("unclaimed linked mail deleted")
	}
	messages, err = f.s.Mailbox(f.ctx, f.id, &pb.CSCheckNewMail{})
	if err != nil || messages[0].(*pb.SCCheckNewMail).GetHasNew() != 0 {
		t.Fatal("read marker lost")
	}
	if _, err = f.s.RewardCenter(f.ctx, f.id, &pb.CSAddRewardsToPlayer{RewardIds: []int64{reward.ID}}); err != nil {
		t.Fatal(err)
	}
	deletion := &pb.CSDeleteMail{Type: proto.Int32(mail.GetType()), CreateTime: proto.Int64(mail.GetTime())}
	for i := 0; i < 2; i++ {
		if _, err = f.s.Mailbox(f.ctx, f.id, deletion); err != nil {
			t.Fatal(err)
		}
	}
	f.reward(t, "mail:linked", "coin,99", 0)
	f.reward(t, "mail:expired", "coin,99", time.Now().Add(-time.Second).UnixMilli())
	f.restart(t)
	messages, err = f.s.Mailbox(f.ctx, f.id, &pb.CSMailList{})
	if err != nil || len(messages[0].(*pb.SCMailList).GetMailBoxList()) != 0 {
		t.Fatal("deleted/expired mail resurrected")
	}
}

func TestP1EventModesFrozenInstancesAndFailedBusiness(t *testing.T) {
	f := p1Setup(t)
	now := time.Now()
	kind := "test:" + fmt.Sprint(now.UnixNano())
	prefix := "p1:test:" + fmt.Sprint(now.UnixNano())
	cumulative := p1Instance(t, f, prefix+":sum", kind, "cumulative", 3, now.Add(time.Hour), 0)
	daily := p1Instance(t, f, prefix+":day", kind, "daily", 3, now.Add(time.Hour), 0)
	single := p1Instance(t, f, prefix+":one", kind, "single", 3, now.Add(time.Hour), 0)
	// 早于实例开始的历史事实不会串入当前活动窗口。
	f.event(t, "fact:before-window", kind, 50, time.UnixMilli(cumulative.StartAt-1))
	if p1Count(t, &db.PeriodProgress{}, "player_id = ? AND instance_id IN ?", f.id, []string{cumulative.ID, daily.ID, single.ID}) != 0 {
		t.Fatal("out-of-window historical fact counted")
	}
	yesterday := now.AddDate(0, 0, -1)
	f.event(t, "fact:yesterday", kind, 2, yesterday)
	f.event(t, "fact:today", kind, 1, now)
	f.event(t, "fact:today", kind, 1, now)
	if p1ProgressValue(t, f, cumulative.ID).Value != 3 || p1ProgressValue(t, f, daily.ID).Value != 1 || p1ProgressValue(t, f, single.ID).Value != 2 {
		t.Fatal("mode or duplicate source incorrect")
	}
	if err := db.GetIns().Model(&db.PeriodInstance{}).Where("id = ?", daily.ID).Updates(map[string]any{"goal": 1, "rewards": "diamond,999"}).Error; err != nil {
		t.Fatal(err)
	}
	f.event(t, "fact:next", kind, 2, now)
	progress := p1ProgressValue(t, f, daily.ID)
	if progress.Value != 3 || progress.RewardID == 0 {
		t.Fatal("daily progress missing")
	}
	var delivery db.RewardDelivery
	if err := db.GetIns().First(&delivery, progress.RewardID).Error; err != nil {
		t.Fatal(err)
	}
	if delivery.Rewards != "coin,17;item_001,1" {
		t.Fatal("future config rewrote frozen prize")
	}
	f.event(t, "fact:late-yesterday", kind, 9, yesterday)
	if p1ProgressValue(t, f, daily.ID).Value != 3 {
		t.Fatal("late previous day rewound daily metric")
	}
	err := updateProfileWithTx(f.ctx, f.id, func(tx *gorm.DB, p *pb.Player, _ *model.GameState) error {
		p.Coin = proto.Int64(100)
		if err := f.s.RecordP1Event(tx, f.id, "fact:failed", kind, 3, now); err != nil {
			return err
		}
		return errors.New("business failed")
	})
	if err == nil || p1Count(t, &db.EventFact{}, "player_id = ? AND source_key = ?", f.id, "fact:failed") != 0 {
		t.Fatal("failed action counted")
	}
	p, _ := f.profile(t)
	if p.GetCoin() != 0 {
		t.Fatal("failed resources committed")
	}
	err = updateProfileWithTx(f.ctx, f.id, func(tx *gorm.DB, _ *pb.Player, _ *model.GameState) error {
		return f.s.RecordP1Event(tx, f.id, "fact:today", kind, 2, now)
	})
	if err == nil {
		t.Fatal("same source altered")
	}
	// 两个领奖入口共享附件，客户端活动请求不能篡改资源。
	request := &pb.CSActivityReward{ActivityId: proto.String(cumulative.ID), Index: proto.Int32(1), GetResources: []*pb.BaseResource{{Id: proto.String("coin"), Num: proto.Int32(17)}, {Id: proto.String("item_001"), Num: proto.Int32(1)}}}
	if _, err = f.s.P1Activity(f.ctx, f.id, request); err != nil {
		t.Fatal(err)
	}
	sum := p1ProgressValue(t, f, cumulative.ID)
	messages, err := f.s.RewardCenter(f.ctx, f.id, &pb.CSAddRewardsToPlayer{RewardIds: []int64{sum.RewardID}})
	if err != nil || len(messages[0].(*pb.SCAddRewardsToPlayer).GetResult().GetSimps()) != 0 {
		t.Fatal("cross-entry claim duplicated")
	}
}

func TestP1PeriodCutoffRankingTieOfflineRecoveryAndOps(t *testing.T) {
	f := p1Setup(t)
	now := time.Now()
	kind := "rank:" + fmt.Sprint(now.UnixNano())
	key := "p1:rank:" + fmt.Sprint(now.UnixNano())
	account, err := LoginAccount(f.ctx, "rank-other"+fmt.Sprint(now.UnixNano()), "isolated-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(f.ctx, account.Id, "排行角色", "hero_043"); err != nil {
		t.Fatal(err)
	}
	// 先完成账号密码计算，再开始截止窗口，确保测试的两条事实确实在窗口内提交。
	now = time.Now()
	cfg := p1Instance(t, f, key, kind, "cumulative", 100, now.Add(3*time.Second), 2)
	f.event(t, "rank:first", kind, 3, now)
	if err = updateProfileWithTx(f.ctx, account.Id, func(tx *gorm.DB, _ *pb.Player, _ *model.GameState) error {
		return f.s.RecordP1Event(tx, account.Id, "rank:second", kind, 3, now)
	}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Until(time.UnixMilli(cfg.EndAt)) + 50*time.Millisecond)
	f.restart(t)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- f.s.SettleP1Periods(f.ctx) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if p1Count(t, &db.PeriodSettlement{}, "instance_id = ?", key) != 1 || p1Count(t, &db.RewardDelivery{}, "source_key = ?", "rank:"+key) != 2 {
		t.Fatal("cutoff snapshot/rewards duplicated")
	}
	var settled db.PeriodSettlement
	if err := db.GetIns().Where("instance_id = ?", key).First(&settled).Error; err != nil {
		t.Fatal(err)
	}
	var snapshot p1Settlement
	if err = json.Unmarshal(settled.Snapshot, &snapshot); err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Ranks) != 2 || snapshot.Ranks[0].PlayerID != min(f.id, account.Id) {
		t.Fatal("tie unstable")
	}
	f.event(t, "rank:late", kind, 100, time.Now())
	if p1ProgressValue(t, f, key).Value != 3 {
		t.Fatal("late event changed cutoff")
	}
	if err = f.s.SettleP1Periods(f.ctx); err != nil {
		t.Fatal(err)
	}
	var after db.PeriodSettlement
	db.GetIns().Where("instance_id = ?", key).First(&after)
	if string(after.Snapshot) != string(settled.Snapshot) {
		t.Fatal("cutoff rewritten")
	}
	ops, err := f.s.P1Operations(f.ctx, f.id, key)
	if err != nil {
		t.Fatal(err)
	}
	if ops.PendingRewards != 1 || ops.SettledPeriods != 1 || len(ops.Recent) != 1 {
		t.Fatal("ops source trace missing", ops)
	}
	if f.mysql.Database != "" {
		var count int64
		if err = db.GetIns().Raw("SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME LIKE 'p1_%' AND ENGINE <> 'InnoDB'").Scan(&count).Error; err != nil || count != 0 {
			t.Fatal("P1 nontransactional tables")
		}
	}
}

func TestP1SuccessfulLoginDayAndServerHook(t *testing.T) {
	f := p1Setup(t)
	p1PublishDirectoryCompatibilityFixture(t, f)
	for i := 0; i < 3; i++ {
		if err := f.s.RecordP1Login(f.ctx, f.id); err != nil {
			t.Fatal(err)
		}
	}
	if p1Count(t, &db.EventFact{}, "player_id = ? AND kind = ?", f.id, constUtil.P1EventLoginDay) != 1 {
		t.Fatal("login day duplicated")
	}
	if p1Count(t, &db.RewardDelivery{}, "player_id = ?", f.id) != 1 {
		t.Fatal("two login instances did not remain independent")
	}
	// 将既有成功培养回调接入共享事实；相同已提交存档不产生第二次养成/消费。
	ctx := context.WithValue(f.ctx, constUtil.GameRequestContextKey, &pb.CSEquipStrengthen{Guid: proto.String("owned-equip")})
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Coin = proto.Int64(100)
		g.Equipment = []*pb.Equipment{{Guid: proto.String("owned-equip"), EquipId: proto.String("equip_001"), Level: proto.Int32(1)}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := updateProfileWithTx(ctx, f.id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		var row db.Profile
		if err := tx.First(&row, f.id).Error; err != nil {
			return err
		}
		p.Coin = proto.Int64(90)
		g.Equipment[0].Level = proto.Int32(2)
		return f.s.RecordP1Save(tx, &row, p, g)
	}); err != nil {
		t.Fatal(err)
	}
	if p1Count(t, &db.EventFact{}, "player_id = ? AND kind = ?", f.id, constUtil.P1EventEquipStrengthen) != 1 || p1Count(t, &db.EventFact{}, "player_id = ? AND kind = ?", f.id, constUtil.P1EventCoinSpent) != 1 {
		t.Fatal("successful event hook missing")
	}
}

// TestP1DowntimeAfterDeadline 保留截止快照与过期投递，停机恢复不延长领取时间。
func TestP1DowntimeAfterDeadline(t *testing.T) {
	f := p1Setup(t)
	now := time.Now()
	kind := "deadline:" + fmt.Sprint(now.UnixNano())
	key := "p1:end:" + fmt.Sprint(now.UnixNano())
	cfg := p1Instance(t, f, key, kind, "cumulative", 100, now.Add(400*time.Millisecond), 1)
	cfg.RewardEndAt = now.Add(700 * time.Millisecond).UnixMilli()
	if err := db.GetIns().Model(&db.PeriodInstance{}).Where("id = ?", key).Update("reward_end_at", cfg.RewardEndAt).Error; err != nil {
		t.Fatal(err)
	}
	f.event(t, "deadline:one", kind, 1, now)
	time.Sleep(time.Until(time.UnixMilli(cfg.RewardEndAt)) + 50*time.Millisecond)
	f.restart(t)
	if err := f.s.SettleP1Periods(f.ctx); err != nil {
		t.Fatal(err)
	}
	var reward db.RewardDelivery
	if err := db.GetIns().Where("player_id = ? AND source_key = ?", f.id, "rank:"+key).First(&reward).Error; err != nil {
		t.Fatal(err)
	}
	if reward.ExpiresAt != cfg.RewardEndAt || reward.ClaimedAt != 0 {
		t.Fatal("restore changed deadline")
	}
	if _, err := f.s.RewardCenter(f.ctx, f.id, &pb.CSAddRewardsToPlayer{RewardIds: []int64{reward.ID}}); err == nil {
		t.Fatal("downtime expiry granted")
	}
	ops, err := f.s.P1Operations(f.ctx, f.id, key)
	if err != nil || ops.ExpiredRewards != 1 || ops.PendingRewards != 0 {
		t.Fatal("expiry trace missing", err, ops)
	}
	if err = f.s.SettleP1Periods(f.ctx); err != nil {
		t.Fatal(err)
	}
	if p1Count(t, &db.RewardDelivery{}, "source_key = ?", "rank:"+key) != 1 {
		t.Fatal("expired reward recreated")
	}
}

// TestP1ServeStopsOnContext 验证后台任务退出，避免开发进程在结算等待中挂住。
func TestP1ServeStopsOnContext(t *testing.T) {
	f := p1Setup(t)
	ctx, cancel := context.WithCancel(f.ctx)
	done := make(chan error, 1)
	go func() { done <- f.s.ServeP1Periods(ctx) }()
	time.Sleep(10 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("period worker did not stop")
	}
}

// TestP1SourceCaseAndGetAllBounds 防止来源大小写折叠误去重，原客户端每次全领最多100项。
func TestP1SourceCaseAndGetAllBounds(t *testing.T) {
	f := p1Setup(t)
	err := updateProfileWithTx(f.ctx, f.id, func(tx *gorm.DB, _ *pb.Player, _ *model.GameState) error {
		for _, source := range []string{"source:Case", "source:case"} {
			if _, err := f.s.EnqueueReward(tx, f.id, source, "大小写来源", "隔离测试", "coin,1", 0); err != nil {
				return err
			}
		}
		for i := 0; i < 103; i++ {
			if _, err := f.s.EnqueueReward(tx, f.id, fmt.Sprintf("bounded:%d", i), "分批领取", "隔离测试", "coin,1", 0); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if p1Count(t, &db.RewardDelivery{}, "player_id = ?", f.id) != 105 {
		t.Fatal("case-sensitive source collapsed")
	}
	for _, want := range []int{100, 5, 0} {
		messages, err := f.s.RewardCenter(f.ctx, f.id, &pb.CSQueryUserReward{})
		if err != nil {
			t.Fatal(err)
		}
		rows := messages[0].(*pb.SCQueryUserReward).GetRewardItemInfoList()
		if len(rows) != want {
			t.Fatal("list exceeds client claim capacity", len(rows), want)
		}
		if want > 0 {
			var ids []int64
			for _, row := range rows {
				ids = append(ids, row.GetRewardId())
			}
			if _, err = f.s.RewardCenter(f.ctx, f.id, &pb.CSAddRewardsToPlayer{RewardIds: ids}); err != nil {
				t.Fatal(err)
			}
		}
	}
	p, _ := f.profile(t)
	if p.GetCoin() != 105 {
		t.Fatal("bounded batches lost reward")
	}
}

// TestP1FutureMonthInstance 发布新月实例不覆盖旧月，也不提前冻结/计入未来活动。
func TestP1FutureMonthInstance(t *testing.T) {
	f := p1Setup(t)
	p1PublishDirectoryCompatibilityFixture(t, f)
	future := time.Now().AddDate(0, 1, 0)
	for i := 0; i < 2; i++ {
		if err := f.s.seedP1Periods(f.ctx, future); err != nil {
			t.Fatal(err)
		}
	}
	futureMonth := future.In(time.FixedZone("UTC+8", 8*3600)).Format("200601")
	// Publish only this fixture's future pair so the test exercises the time
	// boundary even when an operator has prepared the next month in advance.
	futurePublication := db.GetIns().Model(&db.PeriodInstance{}).Where("id IN ?", []string{"totalLogin_p1_" + futureMonth + "_welcome", "totalLogin_p1_" + futureMonth + "_three"}).Update("enabled", true)
	if futurePublication.Error != nil || futurePublication.RowsAffected != 2 {
		t.Fatal("finite future publication", futurePublication.Error, futurePublication.RowsAffected)
	}

	if p1Count(t, &db.PeriodInstance{}, "id LIKE ?", "totalLogin_p1_"+futureMonth+"_%") != 2 {
		t.Fatal("new month duplicated")
	}
	if err := f.s.RecordP1Login(f.ctx, f.id); err != nil {
		t.Fatal(err)
	}
	var rows []db.PeriodInstance
	if err := db.GetIns().Where("id LIKE ?", "totalLogin_p1_"+futureMonth+"_%").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	for _, cfg := range rows {
		if len(cfg.Frozen) != 0 || p1Count(t, &db.PeriodProgress{}, "instance_id = ?", cfg.ID) != 0 {
			t.Fatal("future instance touched by current login")
		}
	}
	if p1Count(t, &db.RewardDelivery{}, "player_id = ?", f.id) != 1 {
		t.Fatal("future month paid early")
	}
}

// TestP1ActualRecruitAndPurchaseCosts 回执中的原始扣费独立于同次返奖，并发重试不重复计数。
func TestP1ActualRecruitAndPurchaseCosts(t *testing.T) {
	f := p1Setup(t)
	for _, initialize := range []func(context.Context) error{f.s.InitializeGameplayPools, f.s.InitializeShop, f.s.InitializeOpenServerActivity, f.s.InitializeDailyTasks} {
		if err := initialize(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Coin = proto.Int64(1000000)
		p.Diamond = proto.Int64(1000)
		p.Level = proto.Int32(75)
		g.Items = []*pb.SimpleItem{{Id: proto.String("item_016"), Num: proto.Int32(1)}}
		g.Recruit = map[int32]*model.RecruitState{1: {Day: p1Day(time.Now()), NextFree: time.Now().Add(time.Hour).UnixMilli(), FreeUsed: 0}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	recruit := &pb.CSWarriorRecruit{CostType: proto.Int32(1), NumType: proto.Int32(1)}
	ctx := context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	ctx = context.WithValue(ctx, constUtil.GameRequestContextKey, recruit)
	before, _ := f.profile(t)
	messages, err := f.s.warriorRecruit(ctx, f.id, recruit, 101)
	if err != nil {
		t.Fatal(err)
	}
	result := messages[0].(*pb.SCWarriorRecruit)
	if len(result.GetCost()) != 1 {
		t.Fatal("paid recruit has no original cost")
	}
	cost := int64(result.GetCost()[0].GetNum())
	after, _ := f.profile(t)
	var fact db.EventFact
	if err = db.GetIns().Where("player_id = ? AND kind = ?", f.id, "spend:item_016").First(&fact).Error; err != nil {
		t.Fatal(err)
	}
	if fact.Amount != cost || cost != 1 || after.GetCoin() <= before.GetCoin() {
		t.Fatal("returned coins confused with consumption", fact.Amount, cost)
	}
	if _, err = f.s.warriorRecruit(ctx, f.id, recruit, 101); err != nil {
		t.Fatal(err)
	}
	if p1Count(t, &db.EventFact{}, "player_id = ? AND kind = ?", f.id, "spend:item_016") != 1 {
		t.Fatal("recruit retry counted twice")
	}
	// The historical shelf is default-off. This isolated cost-accounting fixture
	// explicitly publishes its one finite product; production remains closed.
	publication := db.GetIns().Model(&db.ShopProduct{}).Where("id=?", "itemShopId_8").Update("enabled", true)
	if publication.Error != nil || publication.RowsAffected != 1 {
		t.Fatal("finite shelf publication", publication.Error, publication.RowsAffected)
	}
	buy := &pb.CSShopBuy{ShopType: proto.Int32(1), ShopItemId: proto.String("itemShopId_8"), BuyCount: proto.Int32(2)}
	ctx = context.WithValue(ctx, constUtil.GameRequestContextKey, buy)
	if _, err = f.s.shopBuy(ctx, f.id, buy, 102); err != nil {
		t.Fatal(err)
	}
	var firstFacts []db.EventFact
	if err = db.GetIns().Where("player_id=?", f.id).Order("source_key,kind").Find(&firstFacts).Error; err != nil {
		t.Fatal(err)
	}
	firstFactsJSON, _ := json.Marshal(firstFacts)
	if _, err = f.s.shopBuy(ctx, f.id, buy, 102); err != nil {
		t.Fatal(err)
	}
	var purchase db.EventFact
	if err = db.GetIns().Where("player_id = ? AND kind = ?", f.id, constUtil.P1EventPurchase).First(&purchase).Error; err != nil || purchase.Amount != 2 {
		t.Fatal("real purchase fact missing", err, purchase)
	}
	var spent db.EventFact
	if err = db.GetIns().Where("player_id = ? AND kind = ? AND source_key=?", f.id, constUtil.P1EventDiamondSpent, fmt.Sprintf("shop:%d:102", f.id)).First(&spent).Error; err != nil || spent.Amount != 40 {
		t.Fatal("shop original cost missing", err, spent)
	}
	var replayFacts []db.EventFact
	if err = db.GetIns().Where("player_id=?", f.id).Order("source_key,kind").Find(&replayFacts).Error; err != nil {
		t.Fatal(err)
	}
	replayFactsJSON, _ := json.Marshal(replayFacts)
	if string(firstFactsJSON) != string(replayFactsJSON) {
		t.Fatal("shop retry changed accepted event facts")
	}
	var diamondSpent int64
	if err = db.GetIns().Model(&db.EventFact{}).Select("COALESCE(SUM(amount),0)").Where("player_id=? AND kind=?", f.id, constUtil.P1EventDiamondSpent).Scan(&diamondSpent).Error; err != nil || diamondSpent != 40 {
		t.Fatal("same actual shop payment recorded under multiple sources", diamondSpent, err)
	}

}
