package service

import (
	"sync"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestBigClientRewardOriginalOnceClaimAndCold(t *testing.T) {
	f := p1Setup(t)
	before, _ := f.profile(t)
	// 兼容旧空包已经保存的通知标志，唯一发奖来源由P1账本判断。
	if err := updateProfile(f.ctx, f.id, func(_ *pb.Player, game *model.GameState) error {
		game.FullClientVerified = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 6)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := f.s.bigClientReward(f.ctx, f.id)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	after, game := f.profile(t)
	if !proto.Equal(before, after) || !game.FullClientVerified {
		t.Fatal("notification must only enqueue, not change balances")
	}
	if p1Count(t, &db.RewardDelivery{}, "player_id = ? AND source_key = ?", f.id, constUtil.BigClientRewardSource) != 1 || p1Count(t, &db.MailMessage{}, "player_id = ? AND source_key = ?", f.id, constUtil.BigClientRewardSource) != 0 {
		t.Fatal("notification created duplicate reward or mail")
	}
	f.restart(t)
	messages, err := f.s.RewardCenter(f.ctx, f.id, &pb.CSQueryUserReward{})
	if err != nil {
		t.Fatal(err)
	}
	list := messages[0].(*pb.SCQueryUserReward).GetRewardItemInfoList()
	if len(list) != 1 || len(list[0].GetRewardList()) != 1 || list[0].GetRewardList()[0].GetId() != "diamond" || list[0].GetRewardList()[0].GetNum() != 300 {
		t.Fatal("original 300 diamond reward missing")
	}
	var delivery db.RewardDelivery
	if err := db.GetIns().Where("player_id = ? AND source_key = ?", f.id, constUtil.BigClientRewardSource).First(&delivery).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err = f.s.RewardCenter(f.ctx, f.id, &pb.CSAddRewardsToPlayer{RewardIds: []int64{delivery.ID}}); err != nil {
			t.Fatal(err)
		}
		if _, err = f.s.bigClientReward(f.ctx, f.id); err != nil {
			t.Fatal(err)
		}
	}
	f.restart(t)
	after, game = f.profile(t)
	if after.GetDiamond() != before.GetDiamond()+300 || !game.FullClientVerified {
		t.Fatal("claim was lost or repeated")
	}
	messages, err = f.s.RewardCenter(f.ctx, f.id, &pb.CSQueryUserReward{})
	if err != nil || len(messages[0].(*pb.SCQueryUserReward).GetRewardItemInfoList()) != 0 {
		t.Fatal("login replay resurrected claimed reward")
	}
}

func TestBigClientRewardMissingConfigRollsBackNotification(t *testing.T) {
	f := p1Setup(t)
	for _, row := range f.s.Tables["GameConfigure"] {
		if row["StaticName"] == "BigClientReward" {
			row["NameValue"] = "unknown,300"
		}
	}
	if _, err := f.s.bigClientReward(f.ctx, f.id); err == nil {
		t.Fatal("missing original reward was accepted")
	}
	_, game := f.profile(t)
	if game.FullClientVerified || p1Count(t, &db.RewardDelivery{}, "player_id = ?", f.id) != 0 || p1Count(t, &db.MailMessage{}, "player_id = ?", f.id) != 0 {
		t.Fatal("failed notification left partial state")
	}
}
