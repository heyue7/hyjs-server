package service

import (
	"encoding/json"
	"fmt"
	"reflect"
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

func TestPeriodRankRewardNoticeFrozenOwnerAndFallback(t *testing.T) {
	f := p1Setup(t)
	cfg := db.PeriodInstance{ID: "p1:rank-copy", Title: "Re试炼星数", Kind: constUtil.P7RushStarEvent, Source: "internal-source-must-not-display"}
	snapshot, err := json.Marshal(p1Settlement{Rules: cfg, Ranks: []p1Rank{{PlayerID: f.id + 1, Rank: 1}, {PlayerID: f.id, Rank: 7}}})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Create(&db.PeriodSettlement{InstanceID: cfg.ID, SettledAt: time.Now().UnixMilli(), Snapshot: snapshot}).Error; err != nil {
		t.Fatal(err)
	}
	// A later mutable leaderboard cannot replace the already settled rank.
	if err = db.GetIns().Create(&db.PeriodProgress{InstanceID: cfg.ID, PlayerID: f.id, Value: 999999, Snapshot: []byte(`{}`)}).Error; err != nil {
		t.Fatal(err)
	}
	original := []db.RewardDelivery{{ID: 101, PlayerID: f.id, SourceKey: "rank:" + cfg.ID, Title: "Re试炼星数排行奖励", Content: cfg.Source, Rewards: "coin,13", ClaimedAt: 123, ExpiresAt: 456}, {ID: 102, PlayerID: f.id, SourceKey: "rank:archived", Title: "Reinternal", Content: "internal-other", Rewards: "coin,9"}}
	rows := append([]db.RewardDelivery(nil), original...)
	if err = projectPeriodRewardNotices(db.GetIns(), f.id, rows); err != nil {
		t.Fatal(err)
	}
	if rows[0].Title != "试炼之地排名奖励" || rows[0].Content != "本期试炼排名第7名，排名奖励已发放。" {
		t.Fatal(rows[0])
	}
	if rows[1].Title != "排名奖励" || rows[1].Content != "本期排名奖励已发放。" {
		t.Fatal(rows[1])
	}
	for i := range rows {
		rows[i].Title = original[i].Title
		rows[i].Content = original[i].Content
	}
	if !reflect.DeepEqual(rows, original) {
		t.Fatal("projection changed reward receipt")
	}
	if err = projectPeriodRewardNotices(db.GetIns(), f.id+9, rows); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rows[0].Content, "第") || strings.Contains(rows[0].Title+rows[0].Content, "Re") || strings.Contains(rows[0].Content, cfg.Source) {
		t.Fatal("missing owner rank guessed", rows[0])
	}
	var actual db.PeriodSettlement
	if err = db.GetIns().First(&actual, "instance_id=?", cfg.ID).Error; err != nil || string(actual.Snapshot) != string(snapshot) {
		t.Fatal("frozen snapshot changed", err)
	}
}

func TestPeriodRankRewardNoticeCopyAndFieldBossLastHit(t *testing.T) {
	title, body := periodRankRewardNoticeCopy(db.PeriodInstance{Title: " Re竞技场 "}, 3)
	if title != "竞技场排名奖励" || body != "本期竞技场排名第3名，排名奖励已发放。" {
		t.Fatal(title, body)
	}
	title, body = periodRankRewardNoticeCopy(db.PeriodInstance{}, 0)
	if title != "活动排名奖励" || body != "本期活动排名奖励已发放。" {
		t.Fatal(title, body)
	}
	notice := fmt.Sprintf(constUtil.FieldBossKillNoticeFormat, p12NoticeName("[FF0000]勇者[-]"), p12NoticeName("森林首领"))
	if !strings.Contains(notice, "完成最后一击，击败了森林首领！") || strings.Contains(notice, "[") || strings.Contains(notice, "%!") {
		t.Fatal(notice)
	}
}

func TestPeriodRankRewardNoticeNewSettlement(t *testing.T) {
	f := p1Setup(t)
	cfg := p1Instance(t, f, "p1:rank-copy-settle", "rank-copy-kind", "cumulative", 99, time.Now().Add(-time.Second), 1)
	cfg.Title = "Re竞技场"
	cfg.Source = "private-rule-source"
	if err := db.GetIns().Model(&db.PeriodInstance{}).Where("id=?", cfg.ID).Updates(map[string]any{"title": cfg.Title, "source": cfg.Source}).Error; err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Create(&db.PeriodProgress{InstanceID: cfg.ID, PlayerID: f.id, Value: 5, Snapshot: raw}).Error; err != nil {
		t.Fatal(err)
	}
	before, _ := f.profile(t)
	if err = f.s.SettleP1Periods(f.ctx); err != nil {
		t.Fatal(err)
	}
	var saved db.RewardDelivery
	if err = db.GetIns().First(&saved, "player_id=? AND source_key=?", f.id, "rank:"+cfg.ID).Error; err != nil {
		t.Fatal(err)
	}
	if saved.Title != "竞技场排名奖励" || saved.Content != "本期竞技场排名第1名，排名奖励已发放。" || saved.Rewards != cfg.RankRewards || saved.ExpiresAt != cfg.RewardEndAt || saved.ClaimedAt != 0 {
		t.Fatal(saved)
	}
	if err = f.s.SettleP1Periods(f.ctx); err != nil {
		t.Fatal(err)
	}
	if p1Count(t, &db.RewardDelivery{}, "source_key=?", "rank:"+cfg.ID) != 1 {
		t.Fatal("settlement duplicated")
	}
	after, _ := f.profile(t)
	if !proto.Equal(before, after) {
		t.Fatal("notice delivery changed player resources")
	}
}
