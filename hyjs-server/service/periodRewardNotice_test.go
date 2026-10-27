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
	// 显式保留旧历史邮件，后续列表投影继续支持旧附件关联。
	mail, err := f.s.EnqueueMail(db.GetIns(), f.id, saved.SourceKey, constUtil.P1MailboxSystem, saved.Title, saved.Content, "", constUtil.P1MailSystemType, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Model(mail).Update("reward_id", saved.ID).Error; err != nil {
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
		if len(list) != 0 {
			t.Fatal("activity receipt appeared at reward center", list)
		}
		messages, err = f.s.Mailbox(f.ctx, f.id, &pb.CSMailList{MailType: proto.String("allMail")})
		if err != nil {
			t.Fatal(err)
		}
		boxes := messages[0].(*pb.SCMailList).GetMailBoxList()
		if len(boxes) != 1 || len(boxes[0].GetMailList()) != 1 || boxes[0].GetMailList()[0].GetTitle() != "主线通关" || boxes[0].GetMailList()[0].GetContent() != "主线通关达到3次，奖励已发放。" {
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
	if _, err := f.s.RewardCenter(f.ctx, f.id, &pb.CSAddRewardsToPlayer{RewardIds: []int64{saved.ID}}); err == nil || err.Error() != constUtil.P1ActivityRewardPageOnly {
		t.Fatal("unclaimed activity receipt accepted at center", err)
	}
	deleteMail := &pb.CSDeleteMail{Type: proto.Int32(mail.Type), CreateTime: proto.Int64(mail.CreatedAt)}
	if _, err := f.s.Mailbox(f.ctx, f.id, deleteMail); err == nil || err.Error() != constUtil.P1ActivityRewardPageOnly {
		t.Fatal("unclaimed historical mail pointed to the wrong claim page", err)
	}
	info, err := f.s.P1Activity(f.ctx, f.id, &pb.CSActivityInfo{ActivityId: proto.String(cfg.ID)})
	if err != nil {
		t.Fatal(err)
	}
	item := info[0].(*pb.SCActivityInfo).GetItemList()[0]
	if item.GetStatus() != 1 {
		t.Fatal("frozen activity no longer claimable", item)
	}
	claim := &pb.CSActivityReward{ActivityId: proto.String(cfg.ID), Index: proto.Int32(item.GetIndex()), GetResources: item.GetRewardList()}
	for i := 0; i < 2; i++ {
		if _, err := f.s.P1Activity(f.ctx, f.id, claim); err != nil {
			t.Fatal(err)
		}
	}
	after, _ = f.profile(t)
	if after.GetCoin() != before.GetCoin()+17 {
		t.Fatal("claim repeated or lost")
	}
	f.restart(t)
	if out, err := f.s.RewardCenter(f.ctx, f.id, &pb.CSAddRewardsToPlayer{RewardIds: []int64{saved.ID}}); err != nil || len(out[0].(*pb.SCAddRewardsToPlayer).GetResult().GetSimps()) != 0 {
		t.Fatal("claimed receipt did not remain idempotent at center", err)
	}
	info, err = f.s.P1Activity(f.ctx, f.id, &pb.CSActivityInfo{ActivityId: proto.String(cfg.ID)})
	if err != nil || info[0].(*pb.SCActivityInfo).GetItemList()[0].GetStatus() != 2 {
		t.Fatal("cold activity state lost claimed receipt", err)
	}
	cold, _ := f.profile(t)
	if !proto.Equal(after, cold) {
		t.Fatal("cold receipt retry changed assets")
	}
	if _, err = f.s.Mailbox(f.ctx, f.id, deleteMail); err != nil {
		t.Fatal("claimed historical mail could not be deleted", err)
	}
}
