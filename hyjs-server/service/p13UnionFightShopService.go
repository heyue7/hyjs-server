package service

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"math"
	"strconv"
	"strings"
	"time"
)

func (s *GameService) InitializeP13UnionFightShop(ctx context.Context) error {
	return db.GetIns().WithContext(ctx).AutoMigrate(&db.P13UnionFightShopSource{}, &db.P13UnionFightShopOffer{}, &db.P13UnionFightShopStock{}, &db.P13UnionFightShopVisit{}, &db.P13UnionFightShopPurchase{})
}

func p13UFShopCostResource(source db.P13UnionFightShopSource) string {
	if source.CostResource == "" {
		return constUtil.UnionFightShopTokenResource
	}
	return source.CostResource
}
func p13UFShopQuotedID(source db.P13UnionFightShopSource) string {
	return constUtil.UnionFightShopQuotePrefix + p13UFShopCostResource(source) + ":" + p6Digest(adminJSON(source))[:constUtil.LiveShopQuoteHashLength] + ":" + source.ShopItemID
}
func p13UFShopFrameCost(item *pb.InstUFShopItem) (string, string, error) {
	if !strings.HasPrefix(item.GetShopItemId(), constUtil.UnionFightShopQuotePrefix) {
		return constUtil.UnionFightShopTokenResource, item.GetShopItemId(), nil
	}
	pieces := strings.SplitN(item.GetShopItemId(), ":", 4)
	if len(pieces) != 4 || pieces[1] == "" || len(pieces[2]) != constUtil.LiveShopQuoteHashLength || pieces[3] == "" {
		return "", "", constUtil.ErrShopQuoteChanged
	}
	if _, err := hex.DecodeString(pieces[2]); err != nil || strings.ToLower(pieces[2]) != pieces[2] {
		return "", "", constUtil.ErrShopQuoteChanged
	}
	return pieces[1], pieces[3], nil
}
func p13UFShopStockItem(item *pb.InstUFShopItem, stableID string) *pb.InstUFShopItem {
	cloned := proto.Clone(item).(*pb.InstUFShopItem)
	cloned.ShopItemId = proto.String(stableID)
	return cloned
}

func p13UFShopDigest(row *db.P13UnionFightShopOffer) string {
	raw, _ := json.Marshal(struct{ MatchID, ShopItemID, FrameHash, Source string }{row.MatchID, row.ShopItemID, row.FrameHash, row.Source})
	return p6Digest(raw)
}
func p13UFShopOffer(row *db.P13UnionFightShopOffer) (*pb.InstUFShopItem, error) {
	if p6Digest(row.Frame) != row.FrameHash || p13UFShopDigest(row) != row.SourceHash {
		return nil, errors.New("公会战实际商店报价/来源SHA损坏")
	}
	frame := &pb.InstUFShopItem{}
	if err := proto.Unmarshal(row.Frame, frame); err != nil {
		return nil, err
	}
	_, stableID, err := p13UFShopFrameCost(frame)
	if err != nil {
		return nil, err
	}
	if stableID != row.ShopItemID || frame.GetItemId() == "" || frame.GetPrice() < 0 || frame.GetNum() <= 0 || frame.GetTotalBuyCount() <= 0 || frame.GetRefreshIntervalTime() < 1000 {
		return nil, errors.New("公会战原报价字段损坏")
	}
	return frame, nil
}

// p13UFShopBuild updates operational prices while retaining each original source
// identity and committed receipts. Empty/unpublished sources never create goods.
func (s *GameService) p13UFShopBuild(tx *gorm.DB, match *db.P13UnionFightMatch, original *GameService, now time.Time) (float64, error) {
	distance, err := strconv.ParseFloat(original.table("GameConfigure", "StaticName", "UnionFightNpcMinDistance")["NameValue"], 64)
	if err != nil || distance <= 0 || math.IsNaN(distance) || math.IsInf(distance, 0) {
		return 0, errors.New("原公会战NPC服务器验证距离缺失")
	}
	var sources []db.P13UnionFightShopSource
	if err = tx.Clauses(clause.Locking{Strength: "SHARE"}).Where("day=? AND enabled=1", match.Day).Order("priority,shop_item_id").Find(&sources).Error; err != nil {
		return 0, err
	}
	if len(sources) > 100 {
		return 0, errors.New("具名本地场内目录超过100项")
	}
	if len(sources) == 0 {
		return distance, nil
	}
	activeIDs := make([]string, 0, len(sources))
	for _, source := range sources {
		activeIDs = append(activeIDs, source.ShopItemID)
	}
	var previousRows []db.P13UnionFightShopOffer
	if err = tx.Where("match_id=? AND shop_item_id IN ?", match.ID, activeIDs).Order("shop_item_id").Limit(101).Find(&previousRows).Error; err != nil {
		return 0, err
	}
	if len(previousRows) > 100 {
		return 0, errors.New("本场冻结商店目录超过100项")
	}
	previousByID := make(map[string]*db.P13UnionFightShopOffer, len(previousRows))
	var legacyIDs []string
	var legacyArgs []any
	legacyCase := "CASE shop_item_id"
	for i := range previousRows {
		row := &previousRows[i]
		if _, e := p13UFShopOffer(row); e != nil {
			return 0, e
		}
		previousByID[row.ShopItemID] = row
	}
	for _, source := range sources {
		item := original.table("UnionFightItem", "UFItemId", source.ItemID)
		if item == nil || source.ShopItemID == "" || strings.TrimSpace(source.ShopItemID) != source.ShopItemID || len(source.ShopItemID) > 64 || strings.TrimSpace(source.Source) == "" || source.PublishedAt <= 0 || source.PublishedAt > now.UnixMilli() {
			return 0, errors.New("场内商店原物品/报价来源未完整发布")
		}
		maxCarry, err := strconv.ParseInt(item["ItemCatchMaxNum"], 10, 64)
		if err != nil || maxCarry <= 0 {
			return 0, errors.New("原UF携带上限缺失")
		}
		if source.Price < 0 || source.Price > math.MaxInt32 || source.Quantity <= 0 || source.Quantity > maxCarry || source.BuyCount <= 0 || source.BuyCount > math.MaxInt32 || source.Priority < math.MinInt32 || source.Priority > math.MaxInt32 || source.BuyCondition < 0 || source.BuyCondition > 2 || source.RefreshMillis < 1000 || source.RefreshMillis > 86400000 || source.RefreshMillis%1000 != 0 {
			return 0, errors.New("具名发布的原报价/库存/整秒恢复间隔无效")
		}
		costResource := p13UFShopCostResource(source)
		if costResource != constUtil.UnionFightShopTokenResource {
			if _, err := s.p4ParseCost(fmt.Sprintf("%s,%d", costResource, source.Price)); err != nil {
				return 0, err
			}
		}
		if previous := previousByID[source.ShopItemID]; previous != nil {
			priorFrame, e := p13UFShopOffer(previous)
			if e != nil {
				return 0, e
			}
			if priorFrame.GetShopItemId() == p13UFShopQuotedID(source) {
				continue
			}
			if int64(priorFrame.GetTotalBuyCount()) != source.BuyCount {
				legacyIDs = append(legacyIDs, source.ShopItemID)
				legacyCase += " WHEN ? THEN ?"
				legacyArgs = append(legacyArgs, source.ShopItemID, priorFrame.GetTotalBuyCount())
			}
		}
		frame := &pb.InstUFShopItem{ShopItemId: proto.String(p13UFShopQuotedID(source)), ItemId: proto.String(source.ItemID), Num: proto.Int32(int32(source.Quantity)), Price: proto.Int32(int32(source.Price)), TotalBuyCount: proto.Int32(int32(source.BuyCount)), LeftBuyCount: proto.Int32(int32(source.BuyCount)), Priority: proto.Int32(int32(source.Priority)), BuyCondition: proto.Int32(int32(source.BuyCondition)), LastRefreshTime: proto.Int64(now.UnixMilli()), RefreshIntervalTime: proto.Int64(source.RefreshMillis)}
		raw, err := proto.Marshal(frame)
		if err != nil {
			return 0, err
		}
		offer := db.P13UnionFightShopOffer{MatchID: match.ID, ShopItemID: source.ShopItemID, Frame: raw, FrameHash: p6Digest(raw), Source: constUtil.P13UnionFightShopSource + "；发布：" + source.Source}
		offer.SourceHash = p13UFShopDigest(&offer)
		if err = tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "match_id"}, {Name: "shop_item_id"}}, DoUpdates: clause.AssignmentColumns([]string{"frame", "frame_hash", "source_hash", "source"})}).Create(&offer).Error; err != nil {
			return 0, err
		}
	}
	// Preserve the prior capacity before a quota change. Unchanged catalogues
	// leave old zero-capacity rows to the existing per-member lazy migration.
	if len(legacyIDs) > 0 {
		legacyCase += " ELSE capacity END"
		if err = tx.Model(&db.P13UnionFightShopStock{}).Where("match_id=? AND capacity=0 AND shop_item_id IN ?", match.ID, legacyIDs).Update("capacity", gorm.Expr(legacyCase, legacyArgs...)).Error; err != nil {
			return 0, err
		}
	}
	return distance, nil
}

// p13UFShopStock 按原Countdown每间隔恢复一个余量，满额后的首次消费重新开始计时。
func p13UFShopStock(tx *gorm.DB, scope string, id int64, item *pb.InstUFShopItem, now time.Time) (*db.P13UnionFightShopStock, error) {
	var stock db.P13UnionFightShopStock
	err := tx.First(&stock, "match_id=? AND player_id=? AND shop_item_id=?", scope, id, item.GetShopItemId()).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		stock = db.P13UnionFightShopStock{MatchID: scope, PlayerID: id, ShopItemID: item.GetShopItemId(), LeftCount: int64(item.GetTotalBuyCount()), Capacity: int64(item.GetTotalBuyCount()), LastRefreshAt: now.UnixMilli()}
		return &stock, tx.Create(&stock).Error
	}
	if err != nil {
		return nil, err
	}
	// Another actor may have backfilled the legacy capacity while this role
	// waited on the shared quote. Read the actual row after that wait, without
	// locking an absent range before first creation for unrelated players.
	if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&stock, "match_id=? AND player_id=? AND shop_item_id=?", scope, id, item.GetShopItemId()).Error; err != nil {
		return nil, err
	}
	before := stock
	if err = p13UFShopRefreshStock(&stock, item, now); err != nil {
		return nil, err
	}
	if stock != before {
		if err = tx.Save(&stock).Error; err != nil {
			return nil, err
		}
	}
	return &stock, nil
}

func p13UFShopRefreshStock(stock *db.P13UnionFightShopStock, item *pb.InstUFShopItem, now time.Time) error {
	maximum := int64(item.GetTotalBuyCount())
	if stock.LeftCount < 0 || stock.LastRefreshAt <= 0 {
		return errors.New("场内个人购买余量损坏")
	}
	if stock.Capacity == 0 {
		stock.Capacity = maximum
	}
	if stock.Capacity < 1 || stock.LeftCount > stock.Capacity {
		return errors.New("场内个人购买余量损坏")
	}
	if stock.Capacity != maximum {
		used := max(int64(0), stock.Capacity-stock.LeftCount)
		stock.LeftCount = max(int64(0), maximum-used)
		stock.Capacity = maximum
	}
	if stock.LeftCount < maximum {
		steps := max(int64(0), now.UnixMilli()-stock.LastRefreshAt) / item.GetRefreshIntervalTime()
		if steps > 0 {
			stock.LeftCount = min(maximum, stock.LeftCount+steps)
			stock.LastRefreshAt += steps * item.GetRefreshIntervalTime()
		}
	}
	return nil
}

// List takes all of this member's displayed stock rows in one indexed query.
// Guild policy serializes creation; existing rows additionally use row locks.
func p13UFShopListStocks(tx *gorm.DB, scope string, id int64, items map[string]*pb.InstUFShopItem, now time.Time) (map[string]db.P13UnionFightShopStock, error) {
	stocks := make(map[string]db.P13UnionFightShopStock, len(items))
	if len(items) == 0 {
		return stocks, nil
	}
	ids := make([]string, 0, len(items))
	for key := range items {
		ids = append(ids, key)
	}
	var rows []db.P13UnionFightShopStock
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("match_id=? AND player_id=? AND shop_item_id IN ?", scope, id, ids).Order("shop_item_id").Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		stocks[row.ShopItemID] = row
	}
	var changed []db.P13UnionFightShopStock
	for key, item := range items {
		stock, exists := stocks[key]
		before := stock
		if !exists {
			stock = db.P13UnionFightShopStock{MatchID: scope, PlayerID: id, ShopItemID: key, LeftCount: int64(item.GetTotalBuyCount()), Capacity: int64(item.GetTotalBuyCount()), LastRefreshAt: now.UnixMilli()}
		} else if err := p13UFShopRefreshStock(&stock, item, now); err != nil {
			return nil, err
		}
		stocks[key] = stock
		if stock != before {
			changed = append(changed, stock)
		}
	}
	if len(changed) > 0 {
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "match_id"}, {Name: "player_id"}, {Name: "shop_item_id"}}, DoUpdates: clause.AssignmentColumns([]string{"left_count", "capacity", "last_refresh_at"})}).Create(&changed).Error; err != nil {
			return nil, err
		}
	}
	return stocks, nil
}

// p13UFShopNPC 绑定真实本阵营NPC和当前位置；原购买后空npcId仅引用本场已授权的上次店。
func (s *GameService) p13UFShopNPC(tx *gorm.DB, actor *db.P13UnionFightFieldPlayer, match *db.P13UnionFightMatch, distance float64, npcID string, now time.Time) (string, error) {
	if npcID == "" {
		var visit db.P13UnionFightShopVisit
		if err := tx.First(&visit, "match_id=? AND player_id=?", match.ID, actor.PlayerID).Error; err != nil {
			return "", errors.New("角色没有实际打开的本阵营商店")
		}
		npcID = visit.NPCID
	}
	original, err := s.p13UFFieldOriginal(match, tx)
	if err != nil {
		return "", err
	}
	npc := original.table("UnionFightNpc", "UFNId", npcID)
	if npc == nil || npc["Building"] != "shop" || npc["Camp"] != strconv.FormatInt(actor.Camp, 10) {
		return "", errors.New("原商店NPC不属于本阵营")
	}
	point, err := fieldPoint(npc["Position"])
	if err != nil {
		return "", err
	}
	if p13UFResourceDistance(actor, point.GetX(), point.GetY(), point.GetZ()) > distance {
		return "", errors.New("角色尚未到达原商店NPC")
	}
	return npcID, tx.Save(&db.P13UnionFightShopVisit{MatchID: match.ID, PlayerID: actor.PlayerID, NPCID: npcID, VisitedAt: now.UnixMilli()}).Error
}

func (s *GameService) p13UFShopCurrentOffers(tx *gorm.DB, match *db.P13UnionFightMatch, now time.Time) ([]db.P13UnionFightShopOffer, error) {
	original, err := s.p13UFFieldOriginal(match, tx)
	if err != nil {
		return nil, err
	}
	if _, err = s.p13UFShopBuild(tx, match, original, now); err != nil {
		return nil, err
	}
	var sources []db.P13UnionFightShopSource
	if err = tx.Where("day=? AND enabled=?", match.Day, true).Order("priority,shop_item_id").Find(&sources).Error; err != nil {
		return nil, err
	}
	var ids []string
	for _, source := range sources {
		ids = append(ids, source.ShopItemID)
	}
	if len(ids) == 0 {
		return nil, nil
	}
	var rows []db.P13UnionFightShopOffer
	if err = tx.Where("match_id=? AND shop_item_id IN ?", match.ID, ids).Find(&rows).Error; err != nil {
		return nil, err
	}
	byID := make(map[string]db.P13UnionFightShopOffer, len(rows))
	for _, row := range rows {
		byID[row.ShopItemID] = row
	}
	offers := make([]db.P13UnionFightShopOffer, 0, len(sources))
	for _, source := range sources {
		offer, ok := byID[source.ShopItemID]
		if !ok {
			return nil, gorm.ErrRecordNotFound
		}
		offers = append(offers, offer)
	}
	return offers, nil
}

func (s *GameService) p13UFShopList(ctx context.Context, id int64, request *pb.CSUFShopList, now time.Time) ([]proto.Message, error) {
	reply := &pb.SCUFShopList{}
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		actor, match, snapshot, frame, err := s.p13UFFieldCurrent(tx, id, now)
		if err != nil {
			return err
		}
		if err := s.requireShopAvailability(tx, "union-fight", now); err != nil {
			return err
		}
		if actor.Hooked || frame.GetState() != 0 || now.UnixMilli() >= match.EndAt {
			return errors.New("原非挂机空闲/未结束才可打开商店")
		}
		if snapshot.ShopDistance == nil {
			return errors.New("本场原商店来源尚未冻结")
		}
		var profile db.Profile
		if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&profile, id).Error; err != nil {
			return err
		}
		_, game, err := decodeProfileForCalendarUpdate(tx, &profile)
		if err != nil {
			return err
		}
		if game.PositionLoading || game.Position.GetScene() != constUtil.P13UnionFightFieldTarget {
			return errors.New("原Current加载握手尚未完成")
		}
		if _, err = s.p13UFShopNPC(tx, actor, match, *snapshot.ShopDistance, request.GetNpcId(), now); err != nil {
			return err
		}
		offers, err := s.p13UFShopCurrentOffers(tx, match, now)
		if err != nil {
			return err
		}
		items := make(map[string]*pb.InstUFShopItem, len(offers))
		for _, offer := range offers {
			item, err := p13UFShopOffer(&offer)
			if err != nil {
				return err
			}
			items[offer.ShopItemID] = item
		}
		stocks, err := p13UFShopListStocks(tx, match.ID, id, items, now)
		if err != nil {
			return err
		}
		itemIDs := make([]string, 0, len(items))
		for _, item := range items {
			itemIDs = append(itemIDs, item.GetItemId())
		}
		budgets, err := p13UFShopBudgetBatch(tx, match, actor, itemIDs)
		if err != nil {
			return err
		}
		for _, offer := range offers {
			item, stock := items[offer.ShopItemID], stocks[offer.ShopItemID]
			item.LeftBuyCount = proto.Int32(int32(stock.LeftCount))
			if budget := budgets[item.GetItemId()]; budget.Enabled {
				item.LeftBuyCount = proto.Int32(int32(min(stock.LeftCount, budget.Remaining/int64(item.GetNum()))))
			}
			item.LastRefreshTime = proto.Int64(stock.LastRefreshAt)
			reply.Items = append(reply.Items, item)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{reply}, nil
}

func p13UFShopAddItem(frame *pb.InstUFPlayer, id int64, itemID string, quantity, maximum int32) error {
	occupied := map[int32]bool{}
	ids := map[string]bool{}
	var existing *pb.InstUFItem
	for _, item := range frame.Items {
		if item.GetIndex() < 0 || item.GetIndex() >= constUtil.P13UnionFightShopMaxSlots || occupied[item.GetIndex()] || item.GetPlayerGuid() != id || item.GetLeftUseNum() < 0 || ids[item.GetId()] {
			return errors.New("原五格道具背包身份/位置损坏")
		}
		occupied[item.GetIndex()] = true
		ids[item.GetId()] = true
		if item.GetId() == itemID {
			existing = item
		}
	}
	if existing != nil {
		if int64(existing.GetLeftUseNum())+int64(quantity) > int64(maximum) || int64(existing.GetTotalUseNum())+int64(quantity) > math.MaxInt32 {
			return errors.New("本地已发布原携带上限不能容纳本次道具")
		}
		existing.LeftUseNum = proto.Int32(existing.GetLeftUseNum() + quantity)
		existing.TotalUseNum = proto.Int32(existing.GetTotalUseNum() + quantity)
		return nil
	}
	index := int32(-1)
	for slot := int32(0); slot < constUtil.P13UnionFightShopMaxSlots; slot++ {
		if !occupied[slot] {
			index = slot
			break
		}
	}
	if index < 0 {
		for i, item := range frame.Items {
			if item.GetLeftUseNum() == 0 {
				index = item.GetIndex()
				frame.Items = append(frame.Items[:i], frame.Items[i+1:]...)
				break
			}
		}
	}
	if index < 0 {
		return errors.New("原场内物品背包五格已满")
	}
	frame.Items = append(frame.Items, &pb.InstUFItem{Id: proto.String(itemID), PlayerGuid: proto.Int64(id), Index: proto.Int32(index), LeftUseNum: proto.Int32(quantity), TotalUseNum: proto.Int32(quantity)})
	return nil
}

// p13UFShopBuy 把真实战争贡献消费、个人购买余量和原UF道具背包同事务保存；HTTP同stamp回执不重复消费。
func (s *GameService) p13UFShopBuy(ctx context.Context, id int64, request *pb.CSUFPlayerBuyItem, stamp int64, now time.Time) ([]proto.Message, error) {
	count, countErr := model.GamePurchaseCount(request)
	if countErr != nil {
		return nil, countErr
	}
	var reply *pb.SCUFPlayerBuyItemDetail
	var broadcast *pb.SCUFPlayerBuyItem
	var scope string
	fresh := false
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		actor, match, snapshot, frame, err := s.p13UFFieldCurrent(tx, id, now)
		if err != nil {
			return err
		}
		scope = match.ID
		raw := []byte(scope + "\x00" + request.GetShopItemId() + "\x00" + request.GetNpcId())
		// Keep old single-purchase receipt bytes exact; quantities bind the same stamp.
		if count > 1 {
			raw = append(raw, []byte("\x00batch:"+strconv.Itoa(int(count)))...)
		}
		if stamp > 0 {
			var receipt db.P13GuildReceipt
			err = tx.First(&receipt, "player_id=? AND operation=? AND stamp=?", id, constUtil.P13UnionFightShopBuyOperation, stamp).Error
			if err == nil {
				if !bytes.Equal(receipt.Request, raw) {
					return errors.New("场内购买请求时间戳冲突")
				}
				reply = &pb.SCUFPlayerBuyItemDetail{}
				return proto.Unmarshal(receipt.Response, reply)
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}
		if err := s.requireShopAvailability(tx, "union-fight", now); err != nil {
			return err
		}
		if actor.Hooked || frame.GetState() != 0 || now.UnixMilli() >= match.EndAt {
			return errors.New("原非挂机空闲/未结束才可购买道具")
		}
		if snapshot.ShopDistance == nil {
			return errors.New("本场原商店来源尚未冻结")
		}
		var profile db.Profile
		if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&profile, id).Error; err != nil {
			return err
		}
		player, game, err := decodeProfileForCalendarUpdate(tx, &profile)
		if err != nil {
			return err
		}
		if game.PositionLoading || game.Position.GetScene() != constUtil.P13UnionFightFieldTarget {
			return errors.New("原Current加载握手尚未完成")
		}
		if _, err = s.p13UFShopNPC(tx, actor, match, *snapshot.ShopDistance, request.GetNpcId(), now); err != nil {
			return err
		}
		offers, err := s.p13UFShopCurrentOffers(tx, match, now)
		if err != nil {
			return err
		}
		var item *pb.InstUFShopItem
		var frozenPurchaseOffer *db.P13UnionFightShopOffer
		var stableID string
		for _, offer := range offers {
			candidate, e := p13UFShopOffer(&offer)
			if e != nil {
				return e
			}
			if candidate.GetShopItemId() == request.GetShopItemId() {
				item = candidate
				stableID = offer.ShopItemID
				copy := offer
				frozenPurchaseOffer = &copy
				break
			}
		}
		if item == nil {
			return constUtil.ErrShopQuoteChanged
		}
		costResource, _, err := p13UFShopFrameCost(item)
		if err != nil {
			return err
		}
		role := frame.GetUnionAuthority()
		if item.GetBuyCondition() == 1 && role == 0 || item.GetBuyCondition() == 2 && role != 2 {
			return errors.New("原购买资格与当前/冻结职位不符")
		}
		stock, err := p13UFShopStock(tx, scope, id, p13UFShopStockItem(item, stableID), now)
		if err != nil {
			return err
		}
		if stock.LeftCount < int64(count) {
			return errors.New("原个人购买次数尚未恢复")
		}
		if item.GetPrice() < 0 || int64(item.GetPrice()) > math.MaxInt32/int64(count) || item.GetNum() < 1 || int64(item.GetNum()) > math.MaxInt32/int64(count) {
			return errors.New("批量价格或数量超限")
		}
		purchaseBefore := proto.Clone(frame).(*pb.InstUFPlayer)
		purchaseStockBefore := stock.LeftCount
		totalPrice := int64(item.GetPrice()) * int64(count)
		totalQuantity := item.GetNum() * count
		budgets, err := p13UFShopBudgetBatch(tx, match, actor, []string{item.GetItemId()})
		if err != nil {
			return err
		}
		if err = p13UFShopBudgetCheck(budgets[item.GetItemId()], int64(totalQuantity)); err != nil {
			return err
		}
		owned := frame.GetOwnToken()
		if costResource == constUtil.UnionFightShopTokenResource && (owned < 0 || math.IsNaN(owned) || math.IsInf(owned, 0) || owned != math.Trunc(owned) || owned > 9007199254740991 || owned < float64(totalPrice)) {
			return errors.New("原战争贡献余额不足或损坏")
		}
		original, err := s.p13UFFieldOriginal(match, tx)
		if err != nil {
			return err
		}
		definition := original.table("UnionFightItem", "UFItemId", item.GetItemId())
		maximum, err := strconv.ParseInt(definition["ItemCatchMaxNum"], 10, 32)
		if err != nil || maximum <= 0 {
			return errors.New("原物品携带上限缺失")
		}
		if err = p13UFShopAddItem(frame, id, item.GetItemId(), totalQuantity, int32(maximum)); err != nil {
			return err
		}
		if stock.LeftCount == int64(item.GetTotalBuyCount()) {
			stock.LastRefreshAt = now.UnixMilli()
		}
		stock.LeftCount -= int64(count)
		if err = tx.Save(stock).Error; err != nil {
			return err
		}
		if costResource == constUtil.UnionFightShopTokenResource {
			frame.OwnToken = proto.Float64(owned - float64(totalPrice))
		} else {
			cost, err := s.p4ParseCost(fmt.Sprintf("%s,%d", costResource, totalPrice))
			if err != nil {
				return err
			}
			if err = s.p4Deduct(tx, id, player, game, cost, fmt.Sprintf("ufshop:%s:%d", scope, stamp), LocalPlayNow(ctx)); err != nil {
				return err
			}
			if err = saveProfile(tx, &profile, player, game); err != nil {
				return err
			}
		}
		if err = p13UFShopPurchaseRecord(tx, scope, id, stamp, now.UnixMilli(), int64(count), frozenPurchaseOffer, purchaseStockBefore, stock.LeftCount, purchaseBefore, frame); err != nil {
			return err
		}
		if err = p13UFFieldSaveActor(tx, actor, frame); err != nil {
			return err
		}
		reply = &pb.SCUFPlayerBuyItemDetail{Player: frame}
		broadcast = &pb.SCUFPlayerBuyItem{PlayerGuid: proto.Int64(id), ShopItemId: proto.String(request.GetShopItemId())}
		fresh = true
		if stamp > 0 {
			encoded, err := proto.Marshal(reply)
			if err != nil {
				return err
			}
			return tx.Create(&db.P13GuildReceipt{PlayerID: id, Operation: constUtil.P13UnionFightShopBuyOperation, Stamp: stamp, Request: raw, Response: encoded, CreatedAt: now.UnixMilli()}).Error
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if fresh {
		s.p13UFFieldBroadcast(ctx, scope, id, broadcast)
	}
	// Global materials are outside InstUFPlayer.OwnToken. Sync their current
	// native bags/balances after commit and on receipt replay, never an old wallet.
	var current db.Profile
	if err := db.GetIns().WithContext(ctx).First(&current, id).Error; err != nil {
		return nil, err
	}
	player, game, err := decodeProfile(&current)
	if err != nil {
		return nil, err
	}
	return []proto.Message{reply, &pb.SCItemList{ItemList: game.Items}, &pb.SCPlayer{Player: player}, &pb.SCGemAndCookingList{GemList: game.Gems, CookingList: game.Cooking}}, nil
}
