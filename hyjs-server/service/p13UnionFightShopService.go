package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"math"
	"strconv"
	"strings"
	"time"
)

func (s *GameService) InitializeP13UnionFightShop(ctx context.Context) error {
	return db.GetIns().WithContext(ctx).AutoMigrate(&db.P13UnionFightShopSource{}, &db.P13UnionFightShopOffer{}, &db.P13UnionFightShopStock{}, &db.P13UnionFightShopVisit{})
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
	if frame.GetShopItemId() != row.ShopItemID || frame.GetItemId() == "" || frame.GetPrice() < 0 || frame.GetNum() <= 0 || frame.GetTotalBuyCount() <= 0 || frame.GetRefreshIntervalTime() < 1000 {
		return nil, errors.New("公会战原报价字段损坏")
	}
	return frame, nil
}

// p13UFShopBuild 冻结明确发布的原UF物品目录；服务器报价无原表来源时不放入默认价目。
func (s *GameService) p13UFShopBuild(tx *gorm.DB, match *db.P13UnionFightMatch, original *GameService, now time.Time) (float64, error) {
	distance, err := strconv.ParseFloat(original.table("GameConfigure", "StaticName", "UnionFightNpcMinDistance")["NameValue"], 64)
	if err != nil || distance <= 0 || math.IsNaN(distance) || math.IsInf(distance, 0) {
		return 0, errors.New("原公会战NPC服务器验证距离缺失")
	}
	var sources []db.P13UnionFightShopSource
	if err = tx.Where("day=? AND enabled=1", match.Day).Order("priority,shop_item_id").Find(&sources).Error; err != nil {
		return 0, err
	}
	if len(sources) > 100 {
		return 0, errors.New("具名本地场内目录超过100项")
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
		frame := &pb.InstUFShopItem{ShopItemId: proto.String(source.ShopItemID), ItemId: proto.String(source.ItemID), Num: proto.Int32(int32(source.Quantity)), Price: proto.Int32(int32(source.Price)), TotalBuyCount: proto.Int32(int32(source.BuyCount)), LeftBuyCount: proto.Int32(int32(source.BuyCount)), Priority: proto.Int32(int32(source.Priority)), BuyCondition: proto.Int32(int32(source.BuyCondition)), LastRefreshTime: proto.Int64(now.UnixMilli()), RefreshIntervalTime: proto.Int64(source.RefreshMillis)}
		raw, err := proto.Marshal(frame)
		if err != nil {
			return 0, err
		}
		offer := db.P13UnionFightShopOffer{MatchID: match.ID, ShopItemID: source.ShopItemID, Frame: raw, FrameHash: p6Digest(raw), Source: constUtil.P13UnionFightShopSource + "；发布：" + source.Source}
		offer.SourceHash = p13UFShopDigest(&offer)
		if err = tx.Create(&offer).Error; err != nil {
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
		stock = db.P13UnionFightShopStock{MatchID: scope, PlayerID: id, ShopItemID: item.GetShopItemId(), LeftCount: int64(item.GetTotalBuyCount()), LastRefreshAt: now.UnixMilli()}
		return &stock, tx.Create(&stock).Error
	}
	if err != nil {
		return nil, err
	}
	maximum := int64(item.GetTotalBuyCount())
	if stock.LeftCount < 0 || stock.LeftCount > maximum || stock.LastRefreshAt <= 0 {
		return nil, errors.New("场内个人购买余量损坏")
	}
	if stock.LeftCount < maximum {
		steps := max(int64(0), now.UnixMilli()-stock.LastRefreshAt) / item.GetRefreshIntervalTime()
		if steps > 0 {
			stock.LeftCount = min(maximum, stock.LeftCount+steps)
			stock.LastRefreshAt += steps * item.GetRefreshIntervalTime()
			if err = tx.Save(&stock).Error; err != nil {
				return nil, err
			}
		}
	}
	return &stock, nil
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
		var offers []db.P13UnionFightShopOffer
		if err = tx.Where("match_id=?", match.ID).Order("shop_item_id").Find(&offers).Error; err != nil {
			return err
		}
		for _, offer := range offers {
			item, err := p13UFShopOffer(&offer)
			if err != nil {
				return err
			}
			stock, err := p13UFShopStock(tx, match.ID, id, item, now)
			if err != nil {
				return err
			}
			item.LeftBuyCount = proto.Int32(int32(stock.LeftCount))
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
		var offer db.P13UnionFightShopOffer
		if err = tx.First(&offer, "match_id=? AND shop_item_id=?", match.ID, request.GetShopItemId()).Error; err != nil {
			return errors.New("本场未发布请求的真实报价")
		}
		item, err := p13UFShopOffer(&offer)
		if err != nil {
			return err
		}
		role := frame.GetUnionAuthority()
		if item.GetBuyCondition() == 1 && role == 0 || item.GetBuyCondition() == 2 && role != 2 {
			return errors.New("原购买资格与当前/冻结职位不符")
		}
		stock, err := p13UFShopStock(tx, scope, id, item, now)
		if err != nil {
			return err
		}
		if stock.LeftCount <= 0 {
			return errors.New("原个人购买次数尚未恢复")
		}
		owned := frame.GetOwnToken()
		if owned < 0 || math.IsNaN(owned) || math.IsInf(owned, 0) || owned != math.Trunc(owned) || owned > 9007199254740991 || owned < float64(item.GetPrice()) {
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
		if err = p13UFShopAddItem(frame, id, item.GetItemId(), item.GetNum(), int32(maximum)); err != nil {
			return err
		}
		if stock.LeftCount == int64(item.GetTotalBuyCount()) {
			stock.LastRefreshAt = now.UnixMilli()
		}
		stock.LeftCount--
		if err = tx.Save(stock).Error; err != nil {
			return err
		}
		frame.OwnToken = proto.Float64(owned - float64(item.GetPrice()))
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
	return []proto.Message{reply}, nil
}
