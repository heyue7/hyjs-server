package service

import (
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"strings"
	"testing"
	"time"
)

func TestBulk58UnionFightQuantityReceiptAndCarry(t *testing.T) {
	f, ids, _, clock, match := p13UFFieldSetup(t)
	now := func() time.Time { return time.UnixMilli(clock.Load()) }
	if err := db.GetIns().Model(&db.P13UnionFightFieldPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "preparation_millis": 300000, "left_revive_point_id": 1, "right_revive_point_id": 3, "initial_guard_hp": 1000, "initial_gate_hp": 10000, "move_tolerance": 3, "source": "finite live shop actor"}).Error; err != nil {
		t.Fatal(err)
	}
	source := db.P13UnionFightShopSource{Day: match.Day, ShopItemID: "live-material", Enabled: true, ItemID: "ufitem_016", CostResource: "item_001", Price: 2, Quantity: 1, BuyCount: 3, RefreshMillis: 60000, PublishedAt: now().UnixMilli(), Source: "finite operator actual materials"}
	if err := db.GetIns().Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	id := ids[0]
	if _, err := f.s.p13UFFieldEnter(f.ctx, id, now()); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p13UFFieldFrames(f.ctx, id, &pb.CSCurrentPosition{}, now()); err != nil {
		t.Fatal(err)
	}
	var actor db.P13UnionFightFieldPlayer
	if err := db.GetIns().First(&actor, "match_id=? AND player_id=?", match.ID, id).Error; err != nil {
		t.Fatal(err)
	}
	npc := "3"
	if actor.Camp == 1 {
		npc = "6"
	}
	original, err := f.s.p13UFFieldOriginal(match, db.GetIns())
	if err != nil {
		t.Fatal(err)
	}
	point, err := fieldPoint(original.table("UnionFightNpc", "UFNId", npc)["Position"])
	if err != nil {
		t.Fatal(err)
	}
	// Accumulate legal walking time on the existing server-clock fixture.
	clock.Add(60000)
	if _, err = f.s.p13UFFieldMove(f.ctx, id, &pb.CSMove{PointNew: point}, now()); err != nil {
		t.Fatal(err)
	}
	if err = updateProfile(f.ctx, id, func(_ *pb.Player, g *model.GameState) error {
		g.Items = []*pb.SimpleItem{{Id: proto.String("item_001"), Num: proto.Int32(10)}, {Id: proto.String("frag_hero_001"), Num: proto.Int32(10)}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	list := func() *pb.InstUFShopItem {
		t.Helper()
		messages, e := f.s.p13UFShopList(f.ctx, id, &pb.CSUFShopList{NpcId: proto.String(npc)}, now())
		if e != nil {
			t.Fatal(e)
		}
		items := messages[0].(*pb.SCUFShopList).GetItems()
		if len(items) != 1 {
			t.Fatal("live UF source missing")
		}
		return items[0]
	}
	buy := func(sku string, stamp int64) ([]proto.Message, error) {
		request := &pb.CSUFPlayerBuyItem{ShopItemId: proto.String(sku), NpcId: proto.String(npc)}
		request.ProtoReflect().SetUnknown(protowire.AppendVarint(protowire.AppendTag(nil, 100, protowire.VarintType), 2))
		return f.s.p13UFShopBuy(f.ctx, id, request, stamp, now())
	}
	old := list()
	if !strings.HasPrefix(old.GetShopItemId(), "ufq:item_001:") {
		t.Fatal("UF missing explicit material quote", old)
	}
	first, err := buy(old.GetShopItemId(), 99001)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 4 || first[1].(*pb.SCItemList).GetItemList()[0].GetNum() != 6 {
		t.Fatal("UF actual material debit/current native sync", first)
	}
	if list().GetLeftBuyCount() != 1 {
		t.Fatal("batch stock was not decremented by two")
	}
	frame := first[0].(*pb.SCUFPlayerBuyItemDetail).GetPlayer()
	if len(frame.GetItems()) != 1 || frame.GetItems()[0].GetLeftUseNum() != 2 {
		t.Fatal("batch carry reward differs", frame.GetItems())
	}
	if _, err := buy(old.GetShopItemId(), 99002); err == nil {
		t.Fatal("batch accepted insufficient personal stock")
	}
	changed := &pb.CSUFPlayerBuyItem{ShopItemId: proto.String(old.GetShopItemId()), NpcId: proto.String(npc)}
	if _, err := f.s.p13UFShopBuy(f.ctx, id, changed, 99001, now()); err == nil {
		t.Fatal("same stamp accepted changed quantity")
	}
	f.restart(t)
	replay, err := buy(old.GetShopItemId(), 99001)
	if err != nil || !proto.Equal(first[0], replay[0]) || list().GetLeftBuyCount() != 1 {
		t.Fatal("batch cold receipt/stock changed", err)
	}
}
