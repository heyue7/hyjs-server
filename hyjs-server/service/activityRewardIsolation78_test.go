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

func TestActivityRewardIsolation78ActualPeriodsAndCenter(t *testing.T) {
	f := p1Setup(t)
	now := time.Now()
	cases := []struct {
		id, kind, mode string
		hashed         bool
	}{
		{"p1:isolation78-login", constUtil.P1EventLoginDay, "cumulative", false},
		{"p1:isolation78-recruit", constUtil.P1EventRecruit, "daily", false},
		{"p1:isolation78-trial", constUtil.P7RushStarEvent, "single", true},
	}
	var configs []db.PeriodInstance
	for _, tc := range cases {
		cfg := p1Instance(t, f, tc.id, tc.kind, tc.mode, 1, now.Add(time.Hour), 0)
		tiers := []p1PeriodMilestone{{Goal: 1, Rewards: cfg.Rewards}, {Goal: 2, Rewards: "coin,7"}}
		if tc.hashed {
			for i := range tiers {
				tiers[i].SourceKey = activity77PeriodSource(cfg.ID, i)
			}
		}
		cfg.Milestones, _ = json.Marshal(tiers)
		if err := db.GetIns().Model(&db.PeriodInstance{}).Where("id=?", cfg.ID).Update("milestones", cfg.Milestones).Error; err != nil {
			t.Fatal(err)
		}
		configs = append(configs, cfg)
		f.event(t, "isolation78:event:"+tc.id, tc.kind, 2, now)
	}
	// 活动冻结后修改运营展示规则，真实领取继续遵循参与时的快照。
	if err := db.GetIns().Model(&db.PeriodInstance{}).Where("id=?", configs[0].ID).Updates(map[string]any{"goal": 99, "rewards": "coin,999"}).Error; err != nil {
		t.Fatal(err)
	}
	var pending []db.RewardDelivery
	for _, cfg := range configs {
		out, err := f.s.P1Activity(f.ctx, f.id, &pb.CSActivityInfo{ActivityId: proto.String(cfg.ID)})
		if err != nil {
			t.Fatal(err)
		}
		items := out[0].(*pb.SCActivityInfo).GetItemList()
		if len(items) != 2 || items[0].GetStatus() != 1 || items[1].GetStatus() != 1 || items[0].GetRewardList()[0].GetNum() != 17 {
			t.Fatal("earned frozen activity tiers missing", cfg.ID, items)
		}
		for _, source := range p1PeriodSources(cfg) {
			var row db.RewardDelivery
			if err = db.GetIns().First(&row, "player_id=? AND source_key=?", f.id, source).Error; err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(row.SourceKey, constUtil.P1ActivityRewardSourcePrefix) || row.ClaimedAt != 0 {
				t.Fatal("unexpected activity source", row)
			}
			pending = append(pending, row)
		}
	}
	if pending[4].SourceKey != activity77PeriodSource(configs[2].ID, 0) || pending[5].SourceKey != activity77PeriodSource(configs[2].ID, 1) {
		t.Fatal("77 hashed sources changed", pending[4].SourceKey, pending[5].SourceKey)
	}
	center, err := f.s.RewardCenter(f.ctx, f.id, &pb.CSQueryUserReward{})
	if err != nil || len(center[0].(*pb.SCQueryUserReward).GetRewardItemInfoList()) != 0 {
		t.Fatal("activity-only attachments appeared at center", center, err)
	}
	if points := inventoryPoints(t, f); points[constUtil.RedPointRewardCenter] != 0 {
		t.Fatal("activity-only attachments lit center red point", points)
	}
	// 已有活动记录超过一页时，之后的独立结算仍可在中心展示并领取。
	stale := make([]db.RewardDelivery, 0, constUtil.P1RewardMaximumBatch+5)
	for i := 0; i < constUtil.P1RewardMaximumBatch+5; i++ {
		stale = append(stale, db.RewardDelivery{PlayerID: f.id, SourceKey: fmt.Sprintf("activity:historical78:%03d", i), Title: "活动历史", Rewards: "coin,1", CreatedAt: now.UnixMilli()})
	}
	if err = db.GetIns().CreateInBatches(stale, 50).Error; err != nil {
		t.Fatal(err)
	}
	ordinary := []*db.RewardDelivery{
		f.reward(t, "rank:isolation78", "coin,1", 0),
		f.reward(t, "p13-war:isolation78", "coin,2", 0),
		f.reward(t, "worldboss:isolation78", "coin,4", 0),
		f.reward(t, "Activity:isolation78", "coin,8", 0),
	}
	brief, err := eligiblePendingRewards75(db.GetIns(), f.id, now.UnixMilli(), 1, true)
	if err != nil || len(brief) != 1 || brief[0].ID != ordinary[0].ID {
		t.Fatal("brief lookup failed to bypass activity rows", brief, err)
	}
	center, err = f.s.RewardCenter(f.ctx, f.id, &pb.CSQueryUserReward{})
	if err != nil {
		t.Fatal(err)
	}
	visible := center[0].(*pb.SCQueryUserReward).GetRewardItemInfoList()
	if len(visible) != len(ordinary) {
		t.Fatal("activity history hid or leaked into center page", visible)
	}
	var ordinaryIDs []int64
	for i, row := range ordinary {
		if visible[i].GetRewardId() != row.ID {
			t.Fatal("ordinary center order/source changed", visible)
		}
		ordinaryIDs = append(ordinaryIDs, row.ID)
	}
	if points := inventoryPoints(t, f); points[constUtil.RedPointRewardCenter] != 1 {
		t.Fatal("ordinary center reward lost red point", points)
	}
	before, beforeGame := f.profile(t)
	for _, row := range pending {
		// 已知 ID 或与正常奖励混合都不能绕过入口，批次保持原子性。
		for _, ids := range [][]int64{{row.ID}, {ordinary[0].ID, row.ID}} {
			if _, e := f.s.RewardCenter(f.ctx, f.id, &pb.CSAddRewardsToPlayer{RewardIds: ids}); e == nil || e.Error() != constUtil.P1ActivityRewardPageOnly {
				t.Fatal("pending activity accepted at center", ids, e)
			}
		}
		var after db.RewardDelivery
		if err = db.GetIns().First(&after, row.ID).Error; err != nil || !reflect.DeepEqual(row, after) {
			t.Fatal("rejected center claim changed activity receipt", err)
		}
	}
	unchanged, unchangedGame := f.profile(t)
	if !proto.Equal(before, unchanged) || !p6CopySeriesJSONEqual(t, beforeGame, unchangedGame) {
		t.Fatal("rejected center batch changed wallet or inventory")
	}
	for _, cfg := range configs {
		out, e := f.s.P1Activity(f.ctx, f.id, &pb.CSActivityInfo{ActivityId: proto.String(cfg.ID)})
		if e != nil {
			t.Fatal(e)
		}
		for _, item := range out[0].(*pb.SCActivityInfo).GetItemList() {
			req := &pb.CSActivityReward{ActivityId: proto.String(cfg.ID), Index: proto.Int32(item.GetIndex()), GetResources: item.GetRewardList()}
			claimed, e := f.s.P1Activity(f.ctx, f.id, req)
			if e != nil || len(claimed[0].(*pb.SCActivityReward).GetGet().GetSimps()) == 0 {
				t.Fatal("actual activity claim failed", cfg.ID, item.GetIndex(), e)
			}
			retry, e := f.s.P1Activity(f.ctx, f.id, req)
			if e != nil || len(retry[0].(*pb.SCActivityReward).GetGet().GetSimps()) != 0 {
				t.Fatal("activity retry granted twice", cfg.ID, item.GetIndex(), e)
			}
		}
	}
	afterActivity, _ := f.profile(t)
	if afterActivity.GetCoin()-before.GetCoin() != 72 {
		t.Fatal("frozen activity payout changed", before.GetCoin(), afterActivity.GetCoin())
	}
	for _, row := range pending {
		out, e := f.s.RewardCenter(f.ctx, f.id, &pb.CSAddRewardsToPlayer{RewardIds: []int64{row.ID}})
		if e != nil || len(out[0].(*pb.SCAddRewardsToPlayer).GetResult().GetSimps()) != 0 {
			t.Fatal("claimed activity receipt lost empty center retry", e)
		}
	}
	if _, err = f.s.RewardCenter(f.ctx, f.id, &pb.CSAddRewardsToPlayer{RewardIds: ordinaryIDs}); err != nil {
		t.Fatal("ordinary center settlement claim", err)
	}
	final, _ := f.profile(t)
	if final.GetCoin()-afterActivity.GetCoin() != 15 {
		t.Fatal("ordinary rank/guild/worldboss rewards changed", final.GetCoin(), afterActivity.GetCoin())
	}
	if points := inventoryPoints(t, f); points[constUtil.RedPointRewardCenter] != 0 {
		t.Fatal("hidden activity history kept center red point", points)
	}
	f.restart(t)
	finalCold, _ := f.profile(t)
	if !proto.Equal(final, finalCold) {
		t.Fatal("cold recovery changed claimed assets")
	}
	if out, e := f.s.P1Activity(f.ctx, f.id, &pb.CSActivityInfo{ActivityId: proto.String(configs[2].ID)}); e != nil || out[0].(*pb.SCActivityInfo).GetItemList()[0].GetStatus() != 2 {
		t.Fatal("cold recovery lost 77 activity claim", e)
	}
}
