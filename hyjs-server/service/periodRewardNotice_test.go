package service

import (
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func TestPeriodRewardNoticeCopyUsesActualGoalAndMode(t *testing.T) {
	for _, test := range []struct{ kind, mode, title, condition string }{
		{constUtil.P1EventLoginDay, "cumulative", "Re累计登录", "累计登录达到3天"},
		{constUtil.P1EventCopyWin, "cumulative", "Re开工挑战", "主线通关达到3次"},
		{constUtil.P1EventDiamondSpent, "cumulative", "Re消费奖励", "累计消耗达到3钻石"},
		{constUtil.P1EventDiamondSpent, "single", "消费奖励", "单次消耗达到3钻石"},
		{constUtil.P1EventCoinSpent, "daily", "消费奖励", "今日消耗达到3金币"},
		{constUtil.P4ArenaWinKind, "cumulative", "Re竞技胜场", "竞技场获胜达到3次"},
		{constUtil.P4PlunderActivityKind, "cumulative", "Re夺宝次数", "夺宝达到3次"},
		{constUtil.P4KindCollect, "cumulative", "Re英雄收藏", "收集角色达到3种"},
		{constUtil.P4KindPower, "single", "Re开服战力竞赛", "战力达到3"},
		{"payment_amount", "cumulative", "Re累计充值", "累计充值达到0.03元"},
		{"wanted_attack", "cumulative", "Re悬赏挑战", "悬赏挑战达到3次"},
		{"wanted_kill", "cumulative", "Re悬赏击杀", "悬赏击杀达到3次"},
	} {
		t.Run(test.title+test.mode, func(t *testing.T) {
			title, body := periodRewardNoticeCopy(db.PeriodInstance{Title: test.title, Kind: test.kind, Mode: test.mode, Goal: 3}, 0)
			if strings.Contains(title+body, "Re") || body != test.condition+"，奖励已发放。" || strings.Count(body, "。") != 1 {
				t.Fatal(title, body)
			}
		})
	}
}

func TestPeriodRewardNoticeHistoryBothListsPreserveReceipt(t *testing.T) {
	f := p1Setup(t)
	now := time.Now()
	cfg := p1Instance(t, f, "p1:notice-copy", constUtil.P1EventCopyWin, "cumulative", 3, now.Add(time.Hour), 0)
	if err := db.GetIns().Model(&db.PeriodInstance{}).Where("id=?", cfg.ID).Update("title", "Re主线通关").Error; err != nil {
		t.Fatal(err)
	}
	f.event(t, "notice-copy:win", cfg.Kind, 3, now)
	var saved db.RewardDelivery
	if err := db.GetIns().First(&saved, "player_id=? AND source_key=?", f.id, "activity:"+cfg.ID).Error; err != nil {
		t.Fatal(err)
	}
	// 后续编辑门槛和标题，旧奖励继续解释当时的真实达标条件。
	if err := db.GetIns().Model(&db.PeriodInstance{}).Where("id=?", cfg.ID).Updates(map[string]any{"title": "Re未来配置", "goal": 99}).Error; err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	before, _ := f.profile(t)
	for i := 0; i < 2; i++ {
		messages, err := f.s.RewardCenter(f.ctx, f.id, &pb.CSQueryUserReward{})
		if err != nil {
			t.Fatal(err)
		}
		list := messages[0].(*pb.SCQueryUserReward).GetRewardItemInfoList()
		if len(list) != 1 || list[0].GetRewardId() != saved.ID || list[0].GetRewardTitle() != "主线通关" || list[0].GetRewardContent() != "主线通关达到3次，奖励已发放。" {
			t.Fatal("reward copy", list)
		}
		messages, err = f.s.Mailbox(f.ctx, f.id, &pb.CSMailList{MailType: proto.String("allMail")})
		if err != nil {
			t.Fatal(err)
		}
		boxes := messages[0].(*pb.SCMailList).GetMailBoxList()
		if len(boxes) != 1 || len(boxes[0].GetMailList()) != 1 || boxes[0].GetMailList()[0].GetTitle() != list[0].GetRewardTitle() || boxes[0].GetMailList()[0].GetContent() != list[0].GetRewardContent() {
			t.Fatal("mail copy", boxes)
		}
	}
	var afterRow db.RewardDelivery
	if err := db.GetIns().First(&afterRow, saved.ID).Error; err != nil || afterRow.Title != saved.Title || afterRow.Content != saved.Content || afterRow.Rewards != saved.Rewards || afterRow.ClaimedAt != saved.ClaimedAt || p1Count(t, &db.RewardDelivery{}, "player_id=?", f.id) != 1 || p1Count(t, &db.MailMessage{}, "player_id=?", f.id) != 1 {
		t.Fatal("copy changed original receipt", err, afterRow)
	}
	after, _ := f.profile(t)
	if !proto.Equal(before, after) {
		t.Fatal("copy changed player resources")
	}
	for i := 0; i < 2; i++ {
		if _, err := f.s.RewardCenter(f.ctx, f.id, &pb.CSAddRewardsToPlayer{RewardIds: []int64{saved.ID}}); err != nil {
			t.Fatal(err)
		}
	}
	after, _ = f.profile(t)
	if after.GetCoin() != before.GetCoin()+17 {
		t.Fatal("claim repeated or lost")
	}
}
