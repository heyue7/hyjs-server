package service

import (
	"fmt"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func TestP7ReRushDeferredAdditionQueueActualWins(t *testing.T) {
	f := p7PartyBusinessFixture(t)
	if err := f.s.PublishP7ReRush(f.ctx); err != nil {
		t.Fatal(err)
	}
	call := func(req proto.Message, stamp int64) proto.Message {
		t.Helper()
		out, handled, err := f.s.DispatchP7(f.ctx, f.id, req, stamp)
		if err != nil || !handled {
			t.Fatalf("%T: %v", req, err)
		}
		return out[0]
	}
	choose := func(star int32, stamp int64) proto.Message {
		t.Helper()
		option := map[int32]string{3: "perhp,0.03", 6: "peratt,0.06", 9: "perpdef,0.09"}
		return call(&pb.CSRushAddition{Star: proto.Int32(star), Select: proto.String(option[star])}, stamp)
	}
	for floor := int32(1); floor <= 6; floor++ {
		start := call(&pb.CSRushFightStart{RushType: proto.Int32(1), TaskId: proto.String(fmt.Sprintf("local_rush_%d", floor)), Star: proto.Int32(3)}, 71000+int64(floor)).(*pb.SCRushFightStart)
		series, err := f.s.FreezeP6RushSeriesInput(start, true, false, nil, constUtil.P6OriginalClientHash)
		if err != nil {
			t.Fatal(err)
		}
		input, err := p7GenericMechanismRushInputAt(series, 0)
		if err != nil {
			t.Fatal(err)
		}
		client, err := f.s.ComputeOriginalP6ProtocolFinalHP(f.ctx, input, nil)
		if err != nil || client.Outcome.Winner != 0 {
			t.Fatalf("actual floor %d win required: %v", floor, err)
		}
		call(&pb.CSRushFightEnd{FightId: start.FightId, Result: proto.Int32(1), OptRecord: client.OriginalOperates}, 0)
		if floor%3 == 0 {
			if _, _, err = f.s.DispatchP7(f.ctx, f.id, &pb.CSRush3Hit{}, 71050+int64(floor)); err == nil {
				t.Fatal("sweep skipped unclaimed latest section")
			}
			call(&pb.CSRushGetSectionReward{}, 71100+int64(floor))
			call(&pb.CSRushGetSectionAddition{}, 0)
			choose(3, 71200+int64(floor))
		}
	}
	call(&pb.CSRushResetChallengeCount{}, 71300)
	for i := int64(0); i < 2; i++ {
		sweep := call(&pb.CSRush3Hit{}, 71301+i).(*pb.SCRush3Hit)
		if sweep.GetIsValid() != 1 || len(sweep.Floor) != 3 {
			t.Fatal("bounded real-history sweep", sweep)
		}
	}
	state, err := p7State(db.GetIns(), f.id, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if state.AdditionOfferSection != 0 {
		t.Fatal("reset retained offer")
	}
	var rows []db.P7ReRushSection
	if err = db.GetIns().Where("player_id=? AND run_id=?", f.id, state.RunID).Order("section").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Selected != "" || rows[1].Selected != "" || !rows[0].Claimed || !rows[1].Claimed {
		t.Fatal("sweep auto-selected or failed queue", rows)
	}
	bonus := call(&pb.CSRushAdditionView{}, 0).(*pb.SCRushAdditionView).Prop
	if bonus.GetPerhp() != 0 || bonus.GetPeratt() != 0 || bonus.GetPerpdef() != 0 {
		t.Fatal("pending bonuses applied", bonus)
	}
	// 待选后段来源损坏不应阻塞当前段的懒读取，打开该后段时仍必须拒绝。
	if err = db.GetIns().Model(&db.P7ReRushSection{}).Where("player_id=? AND run_id=? AND section=2", f.id, state.RunID).Update("acceptance_hash", "damaged").Error; err != nil {
		t.Fatal(err)
	}
	info := call(&pb.CSGetRushInfo{}, 0).(*pb.SCGetRushInfo)
	if info.GetIsAdditionNeedGet() != 1 || info.GetIsRewardNeedGet() != 0 {
		t.Fatal("main query did not isolate earliest pending source", info)
	}
	reward, addition, err := p7ReRushSectionFlags(db.GetIns(), state)
	if err != nil || reward != 0 || addition != 1 {
		t.Fatal("flags loaded later pending source", reward, addition, err)
	}
	if err = db.GetIns().Model(&db.P7ReRushSection{}).Where("player_id=? AND run_id=? AND section=2", f.id, state.RunID).Update("claimed", false).Error; err != nil {
		t.Fatal(err)
	}
	info = call(&pb.CSGetRushInfo{}, 0).(*pb.SCGetRushInfo)
	if info.GetIsRewardNeedGet() != 1 || info.GetIsAdditionNeedGet() != 1 {
		t.Fatal("earlier pending hid latest unclaimed reward", info)
	}
	if _, _, err = f.s.DispatchP7(f.ctx, f.id, &pb.CSRush3Hit{}, 71310); err == nil {
		t.Fatal("earlier pending allowed sweep past latest unclaimed reward")
	}
	if err = db.GetIns().Model(&db.P7ReRushSection{}).Where("player_id=? AND run_id=? AND section=2", f.id, state.RunID).Update("claimed", true).Error; err != nil {
		t.Fatal(err)
	}
	call(&pb.CSRushGetSectionAddition{}, 0)
	first := choose(6, 71400)
	f.restart(t)
	if !proto.Equal(first, choose(6, 71400)) {
		t.Fatal("cold receipt changed")
	}
	if _, _, err = f.s.DispatchP7(f.ctx, f.id, &pb.CSRushAddition{Star: proto.Int32(9), Select: proto.String("perpdef,0.09")}, 71401); err == nil {
		t.Fatal("new stamp consumed next pending section")
	}
	var second db.P7ReRushSection
	if err = db.GetIns().First(&second, "player_id=? AND run_id=? AND section=2", f.id, state.RunID).Error; err != nil || second.Selected != "" {
		t.Fatal("next section consumed", err)
	}
	if _, _, err = f.s.DispatchP7(f.ctx, f.id, &pb.CSRushGetSectionAddition{}, 0); err == nil {
		t.Fatal("bad pending source accepted")
	}
	if err = db.GetIns().Model(&db.P7ReRushSection{}).Where("player_id=? AND run_id=? AND section=2", f.id, state.RunID).Update("acceptance_hash", rows[1].AcceptanceHash).Error; err != nil {
		t.Fatal(err)
	}
	call(&pb.CSRushGetSectionAddition{}, 0)
	choose(9, 71402)
	bonus = call(&pb.CSRushAdditionView{}, 0).(*pb.SCRushAdditionView).Prop
	if bonus.GetPerhp() != 0 || bonus.GetPeratt() != .06 || bonus.GetPerpdef() != .09 || bonus.GetPermdef() != .09 {
		t.Fatal("selected bonuses not applied", bonus)
	}
	var old []db.P7ReRushSection
	if err = db.GetIns().Where("player_id=? AND run_id=?", f.id, state.RunID-1).Find(&old).Error; err != nil || len(old) != 2 || old[0].Selected != "perhp,0.03" || old[1].Selected != "perhp,0.03" {
		t.Fatal("history rewritten", err)
	}
}
