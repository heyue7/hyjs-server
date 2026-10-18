package service

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestP12WantedUnknownDailyRewardsEmptyAndHistoricalReceiptOnly(t *testing.T) {
	f := p12WantedPersistenceFixture(t)
	now := time.Now()
	before, _ := f.profile(t)
	info, err := f.s.p12WantedDailyReward(f.ctx, f.id, 0, now)
	if err != nil || len(info[0].(*pb.SCGetRebelRewardInfo).GetRebelRewardInfos()) != 0 {
		t.Fatal("未知运营目标未留空", err)
	}
	for _, rewardID := range []int32{1, 2} {
		if _, err = f.s.p12WantedDailyReward(f.ctx, f.id, rewardID, now); err == nil {
			t.Fatal("旧原创目标仍可新领积分", rewardID)
		}
	}
	after, _ := f.profile(t)
	if !proto.Equal(before, after) || p1Count(t, &db.P12WantedRewardClaim{}, "player_id=?", f.id) != 0 {
		t.Fatal("空运营目标改变了余额或领取记录")
	}
	// 有限旧回执仅验证升级恢复，不补写旧奖励到当前钱包。
	old := &pb.SCGetRebelReward{Result: &pb.ResultObject{Simps: []*pb.SimpleItem{{Id: proto.String("score"), Num: proto.Int32(50)}}}}
	raw, _ := proto.Marshal(old)
	day, _, _ := p12WantedDay(now)
	prior := db.P12WantedRewardClaim{PlayerID: f.id, Day: day, RewardID: 1, Reward: "score,50", Response: raw, ResponseHash: p6Digest(raw), CreatedAt: now.UnixMilli(), Source: "finite legacy receipt compatibility"}
	if err = db.GetIns().Create(&prior).Error; err != nil {
		t.Fatal(err)
	}
	got, err := f.s.p12WantedDailyReward(f.ctx, f.id, 1, now)
	if err != nil || !proto.Equal(got[0], old) {
		t.Fatal("旧领取回执失去恢复能力", err)
	}
	after, _ = f.profile(t)
	if !proto.Equal(before, after) {
		t.Fatal("旧回执重放新发了积分")
	}
}

// 已通首关、成熟阵容和体力为有限前提；实际扫荡按原25%抽发现，不改概率或预设命中。
// 未跑主线战斗，新产悬赏的Start/End、逐体扣血、奖励和回执均走现有原消费者。
func TestP12WantedNaturalSweepDiscoveryOriginalBattleAndColdReceipt(t *testing.T) {
	f := p12WantedPersistenceFixture(t)
	if err := f.s.InitializeCopyDrops(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP4CopyBenefits(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP9CopyIntimacy(f.ctx); err != nil {
		t.Fatal(err)
	}
	local := time.Now().In(time.FixedZone("UTC+8", 8*3600)).AddDate(0, 0, -1)
	now := time.Date(local.Year(), local.Month(), local.Day(), 20, 0, 0, 0, local.Location())
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(100)
		p.Stamina = proto.Int32(1000)
		f.s.refreshCopyProgress(g)
		for _, info := range g.Copy.PassedCopyInfo {
			if info.GetChapterId() == "1" && info.GetMissionId() == "1" {
				info.Star = proto.Int32(5)
			}
		}
		for _, hero := range g.Warriors {
			hero.Level = proto.Int32(100)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var (
		found *pb.SCCopySweep
		stamp int64
	)
	request := &pb.CSCopySweep{ChapterId: proto.String("1"), MissionId: proto.String("1")}
	ctx := context.WithValue(f.ctx, constUtil.GameRequestContextKey, request)
	for attempt := int64(1); attempt <= 64; attempt++ {
		stamp = 914000 + attempt
		out, err := f.s.copySweepAt(ctx, f.id, request, stamp, now)
		if err != nil {
			t.Fatal("原扫荡自然发现入口", attempt, err)
		}
		found = out[0].(*pb.SCCopySweep)
		if found.GetWantedId() != "" {
			break
		}
	}
	if found.GetWantedId() == "" {
		t.Fatal("64次原25%抽取均未发现；保留随机结果，不改概率")
	}
	var discovered db.P12WantedDiscovery
	if err := db.GetIns().Where("player_id=? AND instance_id<>?", f.id, "").First(&discovered).Error; err != nil {
		t.Fatal(err)
	}
	if discovered.Odds != 25 || discovered.Draw >= discovered.Odds {
		t.Fatal("发现没有沿原时段/任务", discovered)
	}
	var selected db.P12WantedInstance
	if err := db.GetIns().First(&selected, "id=?", discovered.InstanceID).Error; err != nil {
		t.Fatal(err)
	}
	if selected.Level != 100 || selected.Difficulty < 1 || selected.Difficulty > 3 || selected.WantedID != found.GetWantedId() {
		t.Fatal("original catalog player100 selection", selected)
	}
	before, _ := f.profile(t)
	retry, err := f.s.copySweepAt(ctx, f.id, request, stamp, now)
	after, _ := f.profile(t)
	if err != nil || !proto.Equal(found, retry[0]) || !proto.Equal(before, after) {
		t.Fatal("发现重试重复发钻或重新抽取", err)
	}
	startRequest := &pb.CSRebelFightStart{RebelId: proto.String(discovered.InstanceID), Type: proto.Int32(1)}
	startCtx := context.WithValue(f.ctx, constUtil.GameRequestContextKey, startRequest)
	out, err := f.s.p12WantedStart(startCtx, f.id, startRequest, 915001, now)
	if err != nil {
		t.Fatal("自然发现实例原Start", err)
	}
	start := out[0].(*pb.SCRebelFightStart)
	var fight db.P12WantedFight
	if err = db.GetIns().First(&fight, "id=?", start.GetFightId()).Error; err != nil {
		t.Fatal(err)
	}
	var terminal model.CombatFinalHPOutcome
	if json.Unmarshal([]byte(fight.TerminalJSON), &terminal) != nil || terminal.Outcome == nil {
		t.Fatal("缺真实预计算终末")
	}
	won := int32(0)
	if terminal.Outcome.Winner == 0 {
		won = 1
	}
	end := &pb.CSRebelFightEnd{FightId: start.FightId, Result: proto.Int32(won), OptRecord: terminal.OriginalOperates}
	endCtx := context.WithValue(f.ctx, constUtil.GameRequestContextKey, end)
	settled, err := f.s.p12WantedEnd(endCtx, f.id, end, now)
	if err != nil {
		t.Fatal("自然发现实例原End", err)
	}
	var saved db.P12WantedInstance
	if err = db.GetIns().First(&saved, "id=?", discovered.InstanceID).Error; err != nil {
		t.Fatal(err)
	}
	result := settled[0].(*pb.SCRebelFightEnd)
	if difference := math.Abs((saved.MaxHP - saved.CurrentHP) - result.GetDamage()); difference > math.Max(1, saved.MaxHP)*1e-12 {
		t.Fatalf("共享扣血与真实伤害不一致: maxHP=%g currentHP=%g damage=%g difference=%g", saved.MaxHP, saved.CurrentHP, result.GetDamage(), difference)
	}
	before, game := f.profile(t)
	gameBytes, _ := json.Marshal(game)
	f.restart(t)
	again, err := f.s.p12WantedEnd(endCtx, f.id, end, now)
	after, current := f.profile(t)
	currentBytes, _ := json.Marshal(current)
	if err != nil || !proto.Equal(again[0], result) || !proto.Equal(before, after) || !bytes.Equal(gameBytes, currentBytes) {
		t.Fatal("自然链冷End重复扣发", err)
	}
	t.Logf("actual sweep draws=%d damage=%v score=%d; original25%% unchanged, first-copy clear/mature roster/stamina finite prerequisites", stamp-914000, result.GetDamage(), result.GetScoreAdd())
}

func TestP12WantedExistingDisabledPolicyKeepsDefinitionAndDiscoveryNoOp(t *testing.T) {
	f := p12WantedPersistenceFixture(t)
	var original db.P12WantedPolicy
	if err := db.GetIns().First(&original, 1).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P12WantedPolicy{}).Where("id=1").Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	// 初始化必须保留此前存储的定义/规则/来源，不因消费者清理回写旧JSON。
	if err := f.s.InitializeP12Wanted(f.ctx); err != nil {
		t.Fatal(err)
	}
	var retained db.P12WantedPolicy
	if err := db.GetIns().First(&retained, 1).Error; err != nil {
		t.Fatal(err)
	}
	if retained.Enabled || original.DefinitionJSON != retained.DefinitionJSON || original.DefinitionHash != retained.DefinitionHash || original.Source != retained.Source {
		t.Fatal("关闭的既存定义被新初始化覆盖")
	}
	before, _ := f.profile(t)
	if err := updateProfileWithTx(f.ctx, f.id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		found, gain, err := f.s.DiscoverP12Wanted(tx, f.id, p, g, "copy-complete:legacy-disabled-policy", time.Now())
		if err != nil {
			return err
		}
		if found != nil || len(gain.GetSimps()) != 0 {
			t.Fatal("关闭策略仍制造发现奖励")
		}
		return nil
	}); err != nil {
		t.Fatal("关闭旧策略拖累原副本完成", err)
	}
	after, _ := f.profile(t)
	if !proto.Equal(before, after) || p1Count(t, &db.P12WantedDiscovery{}, "player_id=?", f.id) != 0 {
		t.Fatal("关闭策略写入发现事实或资源")
	}
}
