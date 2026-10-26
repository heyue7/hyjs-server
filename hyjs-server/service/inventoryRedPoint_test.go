package service

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func inventoryPoints(t *testing.T, f *p1Fixture) map[string]int32 {
	t.Helper()
	// 完整认证分发保留原20117/20118消息，不以直接helper调用代替接线。
	result := copyOverflowWire(t, f, &pb.CSRedPoint{})
	points, ok := result[0].(*pb.SCRedPoint)
	if !ok {
		t.Fatal("original redpoint response", result)
	}
	values := map[string]int32{}
	for _, point := range points.GetRedPointList() {
		values[point.GetType()] = point.GetValue()
	}
	return values
}

func TestInventoryRedPointOriginalFragmentThresholdsAndCold(t *testing.T) {
	f := p1Setup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	var items []*pb.SimpleItem
	kinds := []string{constUtil.RedPointWarriorFrag, constUtil.RedPointEquipmentFrag, constUtil.RedPointPetFrag}
	for _, prefix := range []string{"hero_", "equip_", "pet_"} {
		for _, row := range f.s.Tables["Frag"] {
			need, err := strconv.ParseInt(row["num"], 10, 32)
			if err == nil && need > 1 && strings.HasPrefix(row["target"], prefix) {
				items = append(items, &pb.SimpleItem{Id: proto.String(row["id"]), Num: proto.Int32(int32(need - 1))})
				break
			}
		}
	}
	if len(items) != 3 {
		t.Fatal("three original fragment categories missing")
	}
	if err := updateProfile(f.ctx, f.id, func(_ *pb.Player, g *model.GameState) error { g.Items = items; return nil }); err != nil {
		t.Fatal(err)
	}
	values := inventoryPoints(t, f)
	for _, kind := range append(kinds, constUtil.RedPointBackpack) {
		if value, exists := values[kind]; !exists || value != 0 {
			t.Fatal("below threshold must send explicit zero", kind, values)
		}
	}
	for index, kind := range kinds {
		if err := updateProfile(f.ctx, f.id, func(_ *pb.Player, g *model.GameState) error {
			g.Items[index].Num = proto.Int32(g.Items[index].GetNum() + 1)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		values = inventoryPoints(t, f)
		if values[kind] != 1 || values[constUtil.RedPointBackpack] != 1 {
			t.Fatal("exact original threshold missing", kind, values)
		}
	}
	f.restart(t)
	values = inventoryPoints(t, f)
	for _, kind := range kinds {
		if values[kind] != 1 {
			t.Fatal("cold fragment projection", values)
		}
	}
	if err := updateProfile(f.ctx, f.id, func(_ *pb.Player, g *model.GameState) error { g.Items = nil; return nil }); err != nil {
		t.Fatal(err)
	}
	values = inventoryPoints(t, f)
	if values[constUtil.RedPointBackpack] != 0 {
		t.Fatal("removed fragments retained point", values)
	}
}

func TestInventoryRedPointMailPartialClaimExpiryAndCold(t *testing.T) {
	f := p1Setup(t)
	first := f.reward(t, "red:first", "coin,1", 0)
	second := f.reward(t, "red:second", "coin,1", 0)
	f.reward(t, "red:expired", "coin,1", time.Now().Add(-time.Minute).UnixMilli())
	if _, err := f.s.EnqueueMail(db.GetIns(), f.id, "red:explicit-business-mail", constUtil.P1MailboxSystem, "业务通知", "附件在领奖中心领取", "", constUtil.P1MailSystemType, 0); err != nil {
		t.Fatal(err)
	}
	values := inventoryPoints(t, f)
	if values[constUtil.RedPointRewardCenter] != 1 || values[constUtil.RedPointMessage] != 1 {
		t.Fatal("new mail outside panels", values)
	}
	if _, err := f.s.Mailbox(f.ctx, f.id, &pb.CSMailList{}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.RewardCenter(f.ctx, f.id, &pb.CSAddRewardsToPlayer{RewardIds: []int64{first.ID}}); err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	values = inventoryPoints(t, f)
	if values[constUtil.RedPointRewardCenter] != 1 || values[constUtil.RedPointMessage] != 0 {
		t.Fatal("read mail or partial claim cleared remaining reward", values)
	}
	if _, err := f.s.RewardCenter(f.ctx, f.id, &pb.CSAddRewardsToPlayer{RewardIds: []int64{second.ID}}); err != nil {
		t.Fatal(err)
	}
	values = inventoryPoints(t, f)
	if values[constUtil.RedPointRewardCenter] != 0 || values[constUtil.RedPointMessage] != 0 {
		t.Fatal("claimed or expired mail retains point", values)
	}
}
