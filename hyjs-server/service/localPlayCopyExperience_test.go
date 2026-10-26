package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// 原TableCopyExp实际消费：有限14级/首关历史五星沿既有扫荡fixture，
// 活动经真实私人目录生产，不注入经验事实，不把此测试称作新号自然14级。
func TestLocalPlayCopyExperiencePrivateCalendarRealSweepLevelupAndCold(t *testing.T) {
	f, id := localPlayActivitiesFixture(t)
	for _, init := range []func(context.Context) error{f.s.InitCooking, f.s.InitializeDailyCopy} {
		if err := init(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	// 仅隔离运营窗口前提，沿原具名2倍策略；全天用于稳定回归日历差量，不改变倍率。
	var source db.P4CopyBenefitRule
	if err := db.GetIns().Where("enabled=? AND id NOT LIKE ?", true, "%lp-%").Order("start_at DESC,id DESC").First(&source).Error; err != nil {
		t.Fatal(err)
	}
	prior := source
	t.Cleanup(func() {
		if err := db.GetIns().Save(&prior).Error; err != nil {
			t.Error(err)
		}
	})
	source.StartMinute, source.EndMinute, source.WeekdayMask = 0, 1440, 127
	source.Frozen = nil
	if err := db.GetIns().Save(&source).Error; err != nil {
		t.Fatal(err)
	}
	var owner db.Profile
	if err := db.GetIns().First(&owner, id).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.LocalPlayTimeChange(f.ctx, id, "nextDay", "copy-exp-next"); err != nil {
		t.Fatal(err)
	}
	f.ctx = localClockBind(t, f, &owner)
	if _, err := f.s.LocalPlayActivitiesApply(f.ctx, id, "open", "CopyBenefits", "00000000-0000-4000-8000-000000009011"); err != nil {
		t.Fatal(err)
	}
	var cycle db.LocalPlayActivityCycle
	if err := db.GetIns().First(&cycle, "id=?", localPlayCycleID(t, f, id, "CopyBenefits")).Error; err != nil {
		t.Fatal(err)
	}
	var snapshot localPlayScopedSnapshot
	if json.Unmarshal(cycle.Snapshot, &snapshot) != nil || len(snapshot.Copy) != 1 {
		t.Fatal("private copy source missing")
	}
	rule := snapshot.Copy[0]
	real := time.Now()
	if real.UnixMilli() >= rule.StartAt || LocalPlayAt(f.ctx, real).UnixMilli() < rule.StartAt || rule.CurrencyFactor != 2 {
		t.Fatal("fixture did not isolate future personal window", rule)
	}
	info, err := f.s.P4CopyBenefitsInfo(f.ctx, LocalPlayAt(f.ctx, real))
	if err != nil || info[0].(*pb.SCActivityInfo).GetParam() != "2" {
		t.Fatal("original bonus page unavailable", err)
	}
	if err = updateProfile(f.ctx, id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(14)
		p.Exp = proto.Int64(0)
		g.Copy.PassedCopyInfo = []*pb.CopySimpleInfo{{ChapterId: proto.String("1"), MissionId: proto.String("1"), Star: proto.Int32(5), ChallengeTimes: proto.Int32(0)}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	before, _ := f.profile(t)
	request := &pb.CSCopySweep{ChapterId: proto.String("1"), MissionId: proto.String("1")}
	replies, err := f.s.copySweepAt(f.ctx, id, request, 901001, real)
	if err != nil {
		t.Fatal(err)
	}
	result := replies[0].(*pb.SCCopySweep)
	exp := int32(0)
	for _, gain := range result.GetGain().GetSimps() {
		if gain.GetId() == "exp" {
			exp += gain.GetNum()
		}
	}
	after, game := f.profile(t)
	if exp != 310 || after.GetLevel() != 15 || after.GetExp() != 30 || after.GetCoin()-before.GetCoin() != 4380 || after.GetStamina() != before.GetStamina()-10+20 || game.Copy.GetPassedCopyInfo()[0].GetChallengeTimes() != 1 {
		t.Fatal("visible private2x window did not feed Player.exp and original level15 stamina20", result, after)
	}
	// 回拨后原已结收益仍按冻结回执，不以新的窗口时间重算或多发经验。
	if _, err = f.s.LocalPlayTimeChange(f.ctx, id, "reset", "copy-exp-reset"); err != nil {
		t.Fatal(err)
	}
	f.ctx = localClockBind(t, f, &owner)
	f.restart(t)
	replay, err := f.s.copySweepAt(f.ctx, id, request, 901001, time.Now())
	if err != nil || !proto.Equal(replies[0], replay[0]) {
		t.Fatal("cold rewind repriced original earned gain", err)
	}
	current, _ := f.profile(t)
	if current.GetLevel() != 15 || current.GetExp() != 30 || current.GetCoin() != after.GetCoin() || current.GetStamina() != after.GetStamina() {
		t.Fatal("old receipt granted Player experience twice", current)
	}
	var stored db.LocalPlayActivityCycle
	if err = db.GetIns().First(&stored, "id=?", cycle.ID).Error; err != nil || string(stored.Snapshot) != string(cycle.Snapshot) {
		t.Fatal("original private source overwritten", err)
	}
}
