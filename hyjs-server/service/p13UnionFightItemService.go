package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// InitializeP13UnionFightItem 单独迁移生命道具消费域，生产不默认发布原服未留的效果量。
func (s *GameService) InitializeP13UnionFightItem(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.P13UnionFightItemPolicy{}, &db.P13UnionFightItemSource{}, &db.P13UnionFightItemOffer{}, &db.P13UnionFightItemState{}, &db.P13UnionFightPlayerRecovery{}); err != nil {
		return err
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.P13UnionFightItemPolicy{ID: 1, Source: constUtil.P13UnionFightItemSource}).Error
}
func p13UFItemOfferHash(row *db.P13UnionFightItemOffer) string {
	copy := *row
	copy.SourceHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}
func p13UFItemStateHash(row *db.P13UnionFightItemState) string {
	copy := *row
	copy.SourceHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}

// p13UFItemBuild 第一名实际入场时冻结提前发布的比例与原两类冷却，后续不读取当前发布行。
func (s *GameService) p13UFItemBuild(tx *gorm.DB, match *db.P13UnionFightMatch, original *GameService, now time.Time) error {
	var sources []db.P13UnionFightItemSource
	if err := tx.Where("day=? AND enabled=1", match.Day).Order("item_id").Find(&sources).Error; err != nil {
		return err
	}
	for _, source := range sources {
		expected := ""
		switch source.ItemID {
		case "ufitem_011":
			expected = "buff,uf_buff_001"
		case "ufitem_012":
			expected = "buff,uf_buff_007"
		case "ufitem_016":
			expected = "buff,uf_buff_015"
		default:
			return errors.New("本批生命道具发布超出已审三个原本人效果")
		}
		cfg := original.table("UnionFightItem", "UFItemId", source.ItemID)
		if cfg == nil || cfg["ItemSelector"] != "0" || cfg["ItemUseCondition"] != "0" || cfg["ItemEffect"] != expected || source.HPPercent < 1 || source.HPPercent > 100 || strings.TrimSpace(source.Source) == "" || source.PublishedAt <= 0 || source.PublishedAt > now.UnixMilli() {
			return errors.New("原本人道具或提前发布的具名HP比例来源不完整")
		}
		kind, err := strconv.ParseInt(cfg["ItemType"], 10, 64)
		if err != nil || kind < 0 || kind > 1 {
			return errors.New("原普通/钻石道具目录损坏")
		}
		maximum, err := strconv.ParseInt(cfg["ItemCatchMaxNum"], 10, 64)
		if err != nil || maximum != 5 {
			return errors.New("原生命道具携带5上限缺失")
		}
		key := "UnionFightPlayerUseNormalItemCD"
		if kind == 1 {
			key = "UnionFightPlayerUseDiamondItemCD"
		}
		cd, err := strconv.ParseFloat(original.table("GameConfigure", "StaticName", key)["NameValue"], 64)
		if err != nil || math.IsNaN(cd) || math.IsInf(cd, 0) || cd < 0.1 || cd > 3600 {
			return errors.New("原生命道具5秒冷却缺失")
		}
		quote := db.P13UnionFightItemOffer{MatchID: match.ID, ItemID: source.ItemID, HPPercent: source.HPPercent, ItemType: kind, CooldownMillis: int64(cd * 1000), Maximum: maximum, Source: constUtil.P13UnionFightItemSource + "；发布：" + source.Source}
		quote.SourceHash = p13UFItemOfferHash(&quote)
		if err = tx.Create(&quote).Error; err != nil {
			return err
		}
	}
	return nil
}

// p13UFItemProject 只读绝对冷却的剩余秒数，列表和冷启动不创建新日状态或刷新5秒。
func p13UFItemProject(tx *gorm.DB, scope string, id int64, frame *pb.InstUFPlayer, now time.Time) (*db.P13UnionFightItemState, error) {
	row := db.P13UnionFightItemState{MatchID: scope, PlayerID: id}
	err := tx.First(&row, "match_id=? AND player_id=?", scope, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		row.SourceHash = p13UFItemStateHash(&row)
	} else if err != nil {
		return nil, err
	}
	if row.MatchID != scope || row.PlayerID != id || row.NormalUntil < 0 || row.DiamondUntil < 0 || row.SourceHash != p13UFItemStateHash(&row) {
		return nil, errors.New("本场两类道具冷却SHA损坏")
	}
	frame.UseNormalItemCD = proto.Float64(float64(max(int64(0), row.NormalUntil-now.UnixMilli())) / 1000)
	frame.UseDiamondItemCD = proto.Float64(float64(max(int64(0), row.DiamondUntil-now.UnixMilli())) / 1000)
	return &row, nil
}

// p13UFItemLife 核对真实全部Pose生命并按具名冻结百分比恢复；原死亡状态3仅复活之种可处理。
func p13UFItemLife(frame *pb.InstUFPlayer, quote *db.P13UnionFightItemOffer) error {
	fight := frame.GetFightInfo()
	if fight == nil || fight.GetGuid() != frame.GetGuid() || len(fight.Index) == 0 || len(fight.Index) != len(fight.HpPos) || len(fight.Index) != len(fight.MaxHpPos) {
		return errors.New("原道具效果目标HP/Pose身份不完整")
	}
	seen := map[int32]bool{}
	allDead := true
	for i, index := range fight.Index {
		hp, maximum := fight.HpPos[i], fight.MaxHpPos[i]
		if seen[index] || maximum <= 0 || math.IsNaN(maximum) || math.IsInf(maximum, 0) || hp < 0 || hp > maximum || math.IsNaN(hp) || math.IsInf(hp, 0) {
			return errors.New("原道具目标HP/Pose损坏")
		}
		seen[index] = true
		allDead = allDead && hp == 0
	}
	if quote.ItemID == "ufitem_012" {
		if frame.GetState() != 3 || !allDead {
			return errors.New("原复活之种仅完整死亡角色可使用")
		}
	} else if (frame.GetState() != 0 && frame.GetState() != 5) || allDead {
		return errors.New("原生命恢复仅存活空闲/移动角色可使用")
	}
	for i, maximum := range fight.MaxHpPos {
		// 每Pose按明确本地比例恢复，已死伙伴保持死亡；复活之种恢复原全部Pose。
		if quote.ItemID == "ufitem_012" || fight.HpPos[i] > 0 {
			fight.HpPos[i] = min(maximum, fight.HpPos[i]+maximum*float64(quote.HPPercent)/100)
		}
	}
	if quote.ItemID == "ufitem_012" {
		frame.State = proto.Int32(0)
		frame.LeftTime = proto.Float32(0)
		frame.TotalTime = proto.Float32(0)
	}
	return nil
}

// p13UFItemUse 原真实库存扣1与HP、两类CD、成功回执同事务；未审效果不能仅扣库存后回假成功。
func (s *GameService) p13UFItemUse(ctx context.Context, id int64, request *pb.CSUFPlayerUseItem, stamp int64, now time.Time) ([]proto.Message, error) {
	if request.GetItemId() == "ufitem_014" {
		return s.p13UFDrumUse(ctx, id, request, stamp, now)
	}
	if request.GetItemId() == "ufitem_013" {
		return s.p13UFHealAuraUse(ctx, id, request, stamp, now)
	}
	if request.GetItemId() == "ufitem_015" {
		return s.p13UFSpeedUse(ctx, id, request, stamp, now)
	}
	if request.GetItemId() == "ufitem_017" || request.GetItemId() == "ufitem_018" {
		return s.p13UFCarItemUse(ctx, id, request, stamp, now)
	}
	if request.GetItemId() == "ufitem_019" || request.GetItemId() == "ufitem_020" {
		return s.p13UFGateItemUse(ctx, id, request, stamp, now)
	}
	var reply *pb.SCUFPlayerUseItemDetail
	var full *pb.SCUFPlayer
	var revive *pb.SCUFPlayerRevive
	var scope string
	fresh := false
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		actor, match, _, frame, err := s.p13UFFieldCurrent(tx, id, now)
		if err != nil {
			return err
		}
		scope = match.ID
		if actor.Hooked || now.UnixMilli() < match.StartAt || now.UnixMilli() >= match.EndAt {
			return errors.New("原道具仅非挂机且正在比赛的实际角色可用")
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
		raw := []byte(scope + "\x00" + request.GetItemId())
		if stamp > 0 {
			var receipt db.P13GuildReceipt
			err = tx.First(&receipt, "player_id=? AND operation=? AND stamp=?", id, constUtil.P13UnionFightItemUseOperation, stamp).Error
			if err == nil {
				if !bytes.Equal(raw, receipt.Request) {
					return errors.New("公会战道具请求时间戳冲突")
				}
				reply = &pb.SCUFPlayerUseItemDetail{}
				return proto.Unmarshal(receipt.Response, reply)
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}
		var policy db.P13UnionFightItemPolicy
		if err = tx.First(&policy, 1).Error; err != nil {
			return err
		}
		if !policy.Enabled {
			return errors.New("实际场内生命道具消费尚未开放")
		}
		var quote db.P13UnionFightItemOffer
		if err = tx.First(&quote, "match_id=? AND item_id=?", scope, request.GetItemId()).Error; err != nil {
			return errors.New("本场请求的生命道具效果未提前发布并冻结")
		}
		if quote.SourceHash != p13UFItemOfferHash(&quote) || quote.HPPercent < 1 || quote.HPPercent > 100 || quote.Maximum != 5 || (quote.CooldownMillis < 100 || quote.CooldownMillis > 3600000) || quote.ItemType < 0 || quote.ItemType > 1 {
			return errors.New("本场生命道具效果来源SHA损坏")
		}
		switch quote.ItemID {
		case "ufitem_011", "ufitem_012", "ufitem_016":
		default:
			return errors.New("本批仅已审原本人生命效果可消费")
		}
		state, err := p13UFItemProject(tx, scope, id, frame, now)
		if err != nil {
			return err
		}
		if quote.ItemType == 0 && state.NormalUntil > now.UnixMilli() || quote.ItemType == 1 && state.DiamondUntil > now.UnixMilli() {
			return errors.New("原该类型道具5秒冷却尚未结束")
		}
		occupied := map[int32]bool{}
		ids := map[string]bool{}
		var item *pb.InstUFItem
		for _, row := range frame.Items {
			if row.GetIndex() < 0 || row.GetIndex() >= constUtil.P13UnionFightShopMaxSlots || occupied[row.GetIndex()] || ids[row.GetId()] || row.GetPlayerGuid() != id || row.GetLeftUseNum() < 0 || row.GetTotalUseNum() < row.GetLeftUseNum() {
				return errors.New("本场原库存格/数量/身份损坏")
			}
			occupied[row.GetIndex()] = true
			ids[row.GetId()] = true
			if row.GetId() == quote.ItemID {
				item = row
			}
		}
		if item == nil || item.GetLeftUseNum() < 1 || int64(item.GetLeftUseNum()) > quote.Maximum {
			return errors.New("本场原道具库存不足或携带上限损坏")
		}
		beforeLife := proto.Clone(frame).(*pb.InstUFPlayer)
		if err = p13UFItemLife(frame, &quote); err != nil {
			return err
		}
		if err = p13UFPlayerRecoveryRecord(tx, beforeLife, frame, db.P13UnionFightPlayerRecovery{
			MatchID: scope, Kind: constUtil.P13UFRecoveryItem, ItemID: quote.ItemID,
			Stamp: stamp, CasterID: id, OfferHash: quote.SourceHash, HPPercent: quote.HPPercent,
			InventoryBefore: item.GetLeftUseNum(), InventoryAfter: item.GetLeftUseNum() - 1,
			LogicalAt: now.UnixMilli(), AppliedAt: now.UnixMilli(),
		}); err != nil {
			return err
		}
		item.LeftUseNum = proto.Int32(item.GetLeftUseNum() - 1)
		if quote.ItemType == 0 {
			state.NormalUntil = now.UnixMilli() + quote.CooldownMillis
		} else {
			state.DiamondUntil = now.UnixMilli() + quote.CooldownMillis
		}
		state.SourceHash = p13UFItemStateHash(state)
		if err = tx.Save(state).Error; err != nil {
			return err
		}
		if _, err = p13UFItemProject(tx, scope, id, frame, now); err != nil {
			return err
		}
		if err = p13UFFieldSaveActor(tx, actor, frame); err != nil {
			return err
		}
		reply = &pb.SCUFPlayerUseItemDetail{Player: frame, ItemId: proto.String(quote.ItemID)}
		full = &pb.SCUFPlayer{Player: []*pb.InstUFPlayer{frame}, Local: []*pb.PointNew{{X: proto.Float32(actor.X), Y: proto.Float32(actor.Y), Z: proto.Float32(actor.Z)}}}
		if quote.ItemID == "ufitem_012" {
			revive = &pb.SCUFPlayerRevive{Guid: []int64{id}, Pos: full.Local}
		}
		if stamp > 0 {
			encoded, err := proto.Marshal(reply)
			if err != nil {
				return err
			}
			if err = tx.Create(&db.P13GuildReceipt{PlayerID: id, Operation: constUtil.P13UnionFightItemUseOperation, Stamp: stamp, Request: raw, Response: encoded, CreatedAt: now.UnixMilli()}).Error; err != nil {
				return err
			}
		}
		fresh = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	messages := []proto.Message{reply}
	if fresh {
		broadcast := &pb.SCUFPlayerUseItem{Guid: proto.Int64(id), ItemId: request.ItemId}
		s.p13UFFieldBroadcast(ctx, scope, id, broadcast)
		s.p13UFFieldBroadcast(ctx, scope, id, full)
		messages = append(messages, full)
		if revive != nil {
			s.p13UFFieldBroadcast(ctx, scope, id, revive)
			messages = append(messages, revive)
		}
	}
	return messages, nil
}
