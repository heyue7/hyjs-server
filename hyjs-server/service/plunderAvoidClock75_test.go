package service

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func TestPlunderAvoidClock75AdminForwardRewindColdAndPaidReplay(t *testing.T) {
	f, other, foreign := p11ArenaSetup(t)
	if err := f.s.InitializeServerGameClock(f.ctx); err != nil {
		t.Fatal(err)
	}
	actor := adminWalletTestActor(t, f, constUtil.AdminRoleAdmin)
	if err := db.GetIns().Model(&db.P11PlunderPolicy{}).Where("id=1").Update("avoid_enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	// These finite original stones are the only material fixture; protection,
	// consumption and immutable receipts are produced by the original consumer.
	p11PlunderGrant(t, f, "item_015,2;item_014,1")
	ctx := guildClock75Move(t, f, actor, f.id, 6)
	now := LocalPlayNow(ctx)
	request := &pb.CSUseAvoidPlunder{UsedItem: proto.String("item_015")}
	if _, err := f.s.p11PlunderAt(ctx, f.id, request, 759001, now); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p11PlunderAt(ctx, f.id, &pb.CSUseAvoidPlunder{UsedItem: proto.String("item_014")}, 759002, now); err != nil {
		t.Fatal(err)
	}
	var receipt db.P11ArenaReceipt
	if err := db.GetIns().First(&receipt, "player_id=? AND operation=? AND stamp=?", f.id, "plunder-avoid", 759001).Error; err != nil {
		t.Fatal(err)
	}
	receiptBytes, _ := json.Marshal(receipt)
	_, inventory := p10LuckyProfile(t, f.id)
	inventoryBytes, _ := json.Marshal(inventory.Items)
	// A previously expired finite duration must remain expired on both sides of
	// a calendar change. It has no receipt and cannot grant or consume anything.
	expired := db.P11PlunderState{PlayerID: other, AvoidUntil: now.Add(-time.Hour).UnixMilli(), LastUseAt: now.Add(-2 * time.Hour).UnixMilli(), LastItem: "item_015", UpdatedAt: now.Add(-2 * time.Hour).UnixMilli()}
	if err := db.GetIns().Create(&expired).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.Profile{}).Where("id=?", foreign).Update("server_id", 2).Error; err != nil {
		t.Fatal(err)
	}
	foreignState := db.P11PlunderState{PlayerID: foreign, AvoidUntil: 1, LastUseAt: 1, LastItem: "item_015", UpdatedAt: 1}
	if err := db.GetIns().Create(&foreignState).Error; err != nil {
		t.Fatal(err)
	}
	check := func(current context.Context) {
		t.Helper()
		at := LocalPlayNow(current)
		out, err := f.s.p11PlunderAt(current, f.id, &pb.CSRefreshTreasFrag{}, 759003, at)
		if err != nil {
			t.Fatal(err)
		}
		remaining := out[0].(*pb.SCRefreshTreasFrag).GetAvoidWarTime()
		want := int64(9 * time.Hour / time.Millisecond)
		if remaining > want || remaining < want-5000 {
			t.Fatal("calendar changed paid one/eight-hour remaining duration", remaining, want)
		}
		protected, err := P11PlunderProtected(db.GetIns(), f.id, at)
		if err != nil || !protected {
			t.Fatal("paid protection lost on calendar change", err)
		}
		protected, err = P11PlunderProtected(db.GetIns(), other, at)
		if err != nil || protected {
			t.Fatal("calendar resurrected expired protection", err)
		}
		out, err = f.s.p11PlunderAt(current, f.id, request, 759001, at)
		if err != nil {
			t.Fatal(err)
		}
		if got := out[0].(*pb.SCUseAvoidPlunder).GetAvoidSurplusTime(); got != remaining {
			t.Fatal("paid replay changed remaining duration", got, remaining)
		}
		_, game := p10LuckyProfile(t, f.id)
		itemBytes, _ := json.Marshal(game.Items)
		if !bytes.Equal(inventoryBytes, itemBytes) {
			t.Fatal("calendar or paid replay consumed another stone")
		}
		var saved db.P11ArenaReceipt
		if err := db.GetIns().First(&saved, "player_id=? AND operation=? AND stamp=?", receipt.PlayerID, receipt.Operation, receipt.Stamp).Error; err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(saved)
		if !bytes.Equal(raw, receiptBytes) {
			t.Fatal("calendar rewrote immutable paid receipt")
		}
		var untouched db.P11PlunderState
		if err := db.GetIns().First(&untouched, "player_id=?", foreign).Error; err != nil || untouched != foreignState {
			t.Fatal("calendar changed a different realm", untouched, err)
		}
	}
	check(ctx)
	ctx = guildClock75Move(t, f, actor, f.id, 1)
	check(ctx)
	ctx = guildClock75Move(t, f, actor, f.id, -15)
	check(ctx)
	connection, _ := db.GetIns().DB()
	if err := connection.Close(); err != nil {
		t.Fatal(err)
	}
	f.open(t)
	check(ctx)
	ctx = guildClock75Move(t, f, actor, f.id, 15)
	check(ctx)
}
