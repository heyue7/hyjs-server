package service

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Finite domain inventory isolates the calendar regression. Natural acquisition
// remains covered by the separate original purchase/gear/gem business chains.
func TestLocalPlayEquipmentRewindStrengthenRefundAndCold(t *testing.T) {
	f, id, owner := localClockFixture(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP5(f.ctx); err != nil {
		t.Fatal(err)
	}
	f.s.P5Ready = true
	if err := updateProfile(f.ctx, id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(20)
		_, err := f.s.GrantFixedReward(p, g, "equip_005,1;coin,1000000;diamond,500")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	localClockEnable(t)
	f.ctx = context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	ctx := localClockBind(t, f, owner)
	if _, err := f.s.LocalPlayTimeChange(ctx, id, "previousDay", "equipment-previous-day"); err != nil {
		t.Fatal(err)
	}
	ctx = localClockBind(t, f, owner)
	before, game := f.profile(t)
	if len(game.Equipment) != 1 {
		t.Fatal("finite original equipment prerequisite")
	}
	guid := game.Equipment[0].GetGuid()
	strength := &pb.CSEquipStrengthenOneKey{Guid: proto.String(guid)}
	requestCtx := context.WithValue(ctx, constUtil.GameRequestContextKey, strength)
	out, err := f.s.p5StrengthenOne(requestCtx, id, strength, 818001)
	if err != nil {
		t.Fatal("one-key strengthen must work in the previous personal day", err)
	}
	result := out[0].(*pb.SCEquipStrengthenOneKey)
	spent := int64(result.GetCost()[0].GetNum())
	after, grown := f.profile(t)
	if spent <= 0 || before.GetCoin()-after.GetCoin() != spent || result.GetCostTimes() < 1 || grown.Equipment[0].GetLevel() <= 1 {
		t.Fatal("original real strengthen costs and growth")
	}
	retry, err := f.s.p5StrengthenOne(requestCtx, id, strength, 818001)
	same, _ := f.profile(t)
	if err != nil || !proto.Equal(result, retry[0]) || !proto.Equal(after, same) {
		t.Fatal("strengthen replay charged twice", err)
	}
	reset := &pb.CSEquipReBuild{Guid: proto.String(guid)}
	resetCtx := context.WithValue(ctx, constUtil.GameRequestContextKey, reset)
	resetOut, err := f.s.p5EquipmentReset(resetCtx, id, reset, 818002)
	if err != nil {
		t.Fatal("reset must work in the previous personal day", err)
	}
	restored, restoredGame := f.profile(t)
	if restored.GetCoin() != before.GetCoin() || before.GetDiamond()-restored.GetDiamond() != 50 {
		t.Fatal("original fifty-diamond reset did not refund actual investment")
	}
	if _, err := f.s.equipmentRecord(restoredGame, guid); err == nil {
		t.Fatal("reset retained retired equipment GUID")
	}
	fragments, err := f.s.p5WarriorFragmentReward("equip_005", 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range fragments {
		if simpleOwned(restored, restoredGame, fragment.GetId()) != int64(fragment.GetNum()) {
			t.Fatal("reset lost original synthesis fragments", fragment)
		}
	}
	replay, err := f.s.p5EquipmentReset(resetCtx, id, reset, 818002)
	replayed, _ := f.profile(t)
	if err != nil || !proto.Equal(resetOut[0], replay[0]) || !proto.Equal(restored, replayed) {
		t.Fatal("reset replay refunded or charged twice", err)
	}
	for _, source := range []string{fmt.Sprintf("p5:strength-one:%d", 818001), fmt.Sprintf("p5:equipment-reset:%d", 818002)} {
		var fact db.EventFact
		if err := db.GetIns().First(&fact, "player_id=? AND source_key=?", id, source).Error; err != nil || fact.Day != p1Day(LocalPlayNow(ctx)) || fact.OccurredAt > LocalPlayNow(ctx).UnixMilli() {
			t.Fatal("growth fact escaped the authenticated personal date", source, fact, err)
		}
	}
	f.restart(t)
	cold, coldGame := f.profile(t)
	if !proto.Equal(restored, cold) || !reflect.DeepEqual(restoredGame, coldGame) {
		t.Fatal("cold restore changed actual refund or equipment")
	}
}
