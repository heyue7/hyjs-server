package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func (s *GameService) InitializeP13UnionFightShopPurchase(ctx context.Context) error {
	return db.GetIns().WithContext(ctx).AutoMigrate(&db.P13UnionFightShopPurchase{})
}
func p13UFShopPurchaseHash(row *db.P13UnionFightShopPurchase) string {
	copy := *row
	copy.SourceHash = ""
	raw, _ := json.Marshal(struct {
		Row                  db.P13UnionFightShopPurchase
		Quote, Before, After []byte
	}{copy, copy.QuoteFrame, copy.Before, copy.After})
	return p6Digest(raw)
}
func p13UFShopPurchaseID(row *db.P13UnionFightShopPurchase) string {
	return p6Digest([]byte(fmt.Sprintf("%s/%d/%d/%d/%s/%s/%d/%d/shop-purchase", row.MatchID, row.PlayerID, row.Stamp, row.LogicalAt, row.BeforeHash, row.AfterHash, row.StockBefore, row.StockAfter)))
}
func p13UFShopPurchaseInventory(frame *pb.InstUFPlayer, itemID string) (int64, error) {
	amount := int64(0)
	seen := false
	for _, item := range frame.GetItems() {
		if item.GetId() != itemID {
			continue
		}
		if seen || item.GetPlayerGuid() != frame.GetGuid() || item.GetLeftUseNum() < 0 {
			return 0, p13UFMixedError()
		}
		seen = true
		amount = int64(item.GetLeftUseNum())
	}
	return amount, nil
}

// VerifyLoaded replays only the existing purchase contract, with no quote lookup
// and no repricing against later operational publications.
func p13UFShopPurchaseVerifyLoaded(row *db.P13UnionFightShopPurchase) error {
	if row == nil || row.ID != p13UFShopPurchaseID(row) || row.SourceHash != p13UFShopPurchaseHash(row) || row.BeforeHash != p6Digest(row.Before) || row.AfterHash != p6Digest(row.After) || row.MatchID == "" || row.PlayerID <= 0 || row.Stamp < 0 || row.LogicalAt <= 0 || row.Count < 1 || row.Count > math.MaxInt32 || row.Quantity < 1 || row.Quantity > math.MaxInt32 || row.Price < 0 || row.Price > math.MaxInt32 || row.TotalPrice != row.Price*row.Count || row.TotalPrice > math.MaxInt32 || row.StockBefore < row.Count || row.StockAfter != row.StockBefore-row.Count {
		return p13UFMixedError()
	}
	offer := &db.P13UnionFightShopOffer{MatchID: row.MatchID, ShopItemID: row.ShopItemID, Frame: row.QuoteFrame, FrameHash: p6Digest(row.QuoteFrame), Source: row.QuoteSource, SourceHash: row.QuoteHash}
	item, err := p13UFShopOffer(offer)
	if err != nil {
		return err
	}
	currency, _, err := p13UFShopFrameCost(item)
	if err != nil || currency != row.CostCurrency || item.GetItemId() != row.ItemID || int64(item.GetPrice()) != row.Price || int64(item.GetNum())*row.Count != row.Quantity {
		return p13UFMixedError()
	}
	before, after := &pb.InstUFPlayer{}, &pb.InstUFPlayer{}
	if proto.Unmarshal(row.Before, before) != nil || proto.Unmarshal(row.After, after) != nil || before.GetGuid() != row.PlayerID || after.GetGuid() != row.PlayerID || before.GetState() != 0 || row.TokenBefore != before.GetOwnToken() || row.TokenAfter != after.GetOwnToken() {
		return p13UFMixedError()
	}
	inventory, err := p13UFShopPurchaseInventory(before, row.ItemID)
	if err != nil || inventory != row.InventoryBefore {
		return p13UFMixedError()
	}
	inventory, err = p13UFShopPurchaseInventory(after, row.ItemID)
	if err != nil || inventory != row.InventoryAfter || row.InventoryAfter != row.InventoryBefore+row.Quantity {
		return p13UFMixedError()
	}
	expected := proto.Clone(before).(*pb.InstUFPlayer)
	if err = p13UFShopAddItem(expected, row.PlayerID, row.ItemID, int32(row.Quantity), 5); err != nil {
		return err
	}
	if currency == constUtil.UnionFightShopTokenResource {
		if !combatFinite(row.TokenBefore) || row.TokenBefore < float64(row.TotalPrice) {
			return p13UFMixedError()
		}
		expected.OwnToken = proto.Float64(row.TokenBefore - float64(row.TotalPrice))
	}
	if !proto.Equal(expected, after) {
		return p13UFMixedError()
	}
	return nil
}
func p13UFShopPurchaseRecord(tx *gorm.DB, matchID string, playerID, stamp, at, count int64, offer *db.P13UnionFightShopOffer, stockBefore, stockAfter int64, before, after *pb.InstUFPlayer) error {
	item, err := p13UFShopOffer(offer)
	if err != nil {
		return err
	}
	currency, _, err := p13UFShopFrameCost(item)
	if err != nil {
		return err
	}
	row := db.P13UnionFightShopPurchase{MatchID: matchID, PlayerID: playerID, Stamp: stamp, ShopItemID: offer.ShopItemID, ItemID: item.GetItemId(), Count: count, Quantity: int64(item.GetNum()) * count, Price: int64(item.GetPrice()), TotalPrice: int64(item.GetPrice()) * count, CostCurrency: currency, StockBefore: stockBefore, StockAfter: stockAfter, QuoteFrame: append([]byte(nil), offer.Frame...), QuoteSource: offer.Source, QuoteHash: offer.SourceHash, TokenBefore: before.GetOwnToken(), TokenAfter: after.GetOwnToken(), LogicalAt: at}
	row.Before, err = proto.MarshalOptions{Deterministic: true}.Marshal(before)
	if err != nil {
		return err
	}
	row.After, err = proto.MarshalOptions{Deterministic: true}.Marshal(after)
	if err != nil {
		return err
	}
	row.BeforeHash = p6Digest(row.Before)
	row.AfterHash = p6Digest(row.After)
	row.InventoryBefore, err = p13UFShopPurchaseInventory(before, row.ItemID)
	if err != nil {
		return err
	}
	row.InventoryAfter, err = p13UFShopPurchaseInventory(after, row.ItemID)
	if err != nil {
		return err
	}
	row.ID = p13UFShopPurchaseID(&row)
	row.SourceHash = p13UFShopPurchaseHash(&row)
	if err = p13UFShopPurchaseVerifyLoaded(&row); err != nil {
		return err
	}
	return tx.Create(&row).Error
}
