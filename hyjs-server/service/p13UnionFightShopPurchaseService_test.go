package service

import (
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"testing"
)

func p13UFShopPurchaseFixture(t *testing.T) *db.P13UnionFightShopPurchase {
	t.Helper()
	item := &pb.InstUFShopItem{ShopItemId: proto.String("test-quote"), ItemId: proto.String("ufitem_020"), Price: proto.Int32(2), Num: proto.Int32(1), TotalBuyCount: proto.Int32(5), RefreshIntervalTime: proto.Int64(1000)}
	quoteRaw, err := proto.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	offer := &db.P13UnionFightShopOffer{MatchID: "isolated", ShopItemID: "test-quote", Frame: quoteRaw, FrameHash: p6Digest(quoteRaw), Source: "isolated quote fixture"}
	offer.SourceHash = p13UFShopDigest(offer)
	before := &pb.InstUFPlayer{Guid: proto.Int64(1), State: proto.Int32(0), OwnToken: proto.Float64(10)}
	after := proto.Clone(before).(*pb.InstUFPlayer)
	if err = p13UFShopAddItem(after, 1, "ufitem_020", 2, 5); err != nil {
		t.Fatal(err)
	}
	after.OwnToken = proto.Float64(6)
	row := &db.P13UnionFightShopPurchase{MatchID: "isolated", PlayerID: 1, Stamp: 0, ShopItemID: "test-quote", ItemID: "ufitem_020", Count: 2, Quantity: 2, Price: 2, TotalPrice: 4, CostCurrency: constUtil.UnionFightShopTokenResource, InventoryBefore: 0, InventoryAfter: 2, TokenBefore: 10, TokenAfter: 6, StockBefore: 5, StockAfter: 3, QuoteFrame: quoteRaw, QuoteSource: offer.Source, QuoteHash: offer.SourceHash, LogicalAt: 1000}
	row.Before, _ = proto.MarshalOptions{Deterministic: true}.Marshal(before)
	row.After, _ = proto.MarshalOptions{Deterministic: true}.Marshal(after)
	row.BeforeHash = p6Digest(row.Before)
	row.AfterHash = p6Digest(row.After)
	row.ID = p13UFShopPurchaseID(row)
	row.SourceHash = p13UFShopPurchaseHash(row)
	return row
}
func TestP13UFShopPurchaseExactFrozenQuoteStampZeroAndTamper(t *testing.T) {
	row := p13UFShopPurchaseFixture(t)
	if err := p13UFShopPurchaseVerifyLoaded(row); err != nil {
		t.Fatal(err)
	}
	originalID := row.ID
	row.LogicalAt++
	row.ID = p13UFShopPurchaseID(row)
	row.SourceHash = p13UFShopPurchaseHash(row)
	if row.ID == originalID {
		t.Fatal("stamp0 real purchases have no independent identity")
	}
	if err := p13UFShopPurchaseVerifyLoaded(row); err != nil {
		t.Fatal(err)
	}
	row.TotalPrice = 3
	row.SourceHash = p13UFShopPurchaseHash(row)
	if p13UFShopPurchaseVerifyLoaded(row) == nil {
		t.Fatal("repriced frozen quote accepted")
	}
	row = p13UFShopPurchaseFixture(t)
	after := &pb.InstUFPlayer{}
	if err := proto.Unmarshal(row.After, after); err != nil {
		t.Fatal(err)
	}
	after.KillNum = proto.Int32(1)
	row.After, _ = proto.Marshal(after)
	row.AfterHash = p6Digest(row.After)
	row.ID = p13UFShopPurchaseID(row)
	row.SourceHash = p13UFShopPurchaseHash(row)
	if p13UFShopPurchaseVerifyLoaded(row) == nil {
		t.Fatal("purchase changed combat counters")
	}
}
