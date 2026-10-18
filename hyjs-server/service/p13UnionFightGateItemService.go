package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"sort"
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

// InitializeP13UnionFightGateItem 独立保存城门道具的已发布来源与实际动态门HP，不默认投放未知原数量。
func (s *GameService) InitializeP13UnionFightGateItem(ctx context.Context) error {
	return db.GetIns().WithContext(ctx).AutoMigrate(&db.P13UnionFightGateItemSource{}, &db.P13UnionFightGateItemOffer{}, &db.P13UnionFightGateState{})
}
func p13UFGateItemOfferHash(row *db.P13UnionFightGateItemOffer) string {
	copy := *row
	copy.SourceHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}
func p13UFGateStateHash(row *db.P13UnionFightGateState) string {
	copy := *row
	copy.SourceHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}
func p13UFGateRead(row *db.P13UnionFightGateState) (*pb.InstUFGate, error) {
	if row.SourceHash != p13UFGateStateHash(row) || row.FrameHash != p6Digest(row.Frame) || len(row.InitialHash) != 64 {
		return nil, errors.New("实际城门状态/来源SHA损坏")
	}
	frame := &pb.InstUFGate{}
	if err := proto.Unmarshal(row.Frame, frame); err != nil {
		return nil, err
	}
	if frame.GetGuid() != row.GUID || int64(frame.GetId()) != row.GateID || int64(frame.GetCamp()) != row.Camp || row.GUID != p13UFFieldGUID(row.MatchID, "gate", strconv.FormatInt(row.GateID, 10)) || frame.GetLevel() != 1 || frame.GetMaxHp() <= 0 || frame.GetHp() < 0 || frame.GetHp() > frame.GetMaxHp() || math.IsNaN(frame.GetHp()) || math.IsInf(frame.GetHp(), 0) || math.IsNaN(frame.GetMaxHp()) || math.IsInf(frame.GetMaxHp(), 0) {
		return nil, errors.New("原城门GUID/阵营/本地初始等级与HP损坏")
	}
	return frame, nil
}
func p13UFGateSave(tx *gorm.DB, row *db.P13UnionFightGateState, frame *pb.InstUFGate) error {
	raw, err := proto.Marshal(frame)
	if err != nil {
		return err
	}
	row.Frame = raw
	row.FrameHash = p6Digest(raw)
	row.SourceHash = p13UFGateStateHash(row)
	return tx.Save(row).Error
}

// p13UFGateItemBuild 在实际地图初次冻结时附上8原门实体与已发布的本地修复/攻城比例。
func (s *GameService) p13UFGateItemBuild(tx *gorm.DB, match *db.P13UnionFightMatch, buildings *pb.SCUFBuildingRefresh, original *GameService, now time.Time) error {
	var sources []db.P13UnionFightGateItemSource
	if err := tx.Where("day=? AND enabled=1", match.Day).Order("item_id").Find(&sources).Error; err != nil {
		return err
	}
	if len(sources) == 0 {
		return nil
	}
	if len(buildings.GateInfos) != 8 {
		return errors.New("原地图8门实体不完整")
	}
	for _, source := range sources {
		selector, effect := "", ""
		switch source.ItemID {
		case "ufitem_019":
			selector = "2,0,5"
			effect = "buff,uf_buff_005"
		case "ufitem_020":
			selector = "2,1,5"
			effect = "buff,uf_buff_004"
		default:
			return errors.New("城门道具来源超出原修复工具/攻城之锤")
		}
		cfg := original.table("UnionFightItem", "UFItemId", source.ItemID)
		if cfg == nil || cfg["ItemSelector"] != selector || cfg["ItemEffect"] != effect || cfg["ItemType"] != "0" || cfg["ItemUseCondition"] != "0" || cfg["ItemCatchMaxNum"] != "5" || source.HPPercent < 1 || source.HPPercent > 100 || source.PublishedAt <= 0 || source.PublishedAt > now.UnixMilli() || strings.TrimSpace(source.Source) == "" {
			return errors.New("原城门道具5米/阵营规则或具名提前发布比例无效")
		}
		if original.table("GameConfigure", "StaticName", "UnionFightPlayerUseNormalItemCD")["NameValue"] != "5" {
			return errors.New("原城门普通道具5秒CD缺失")
		}
		quote := db.P13UnionFightGateItemOffer{MatchID: match.ID, ItemID: source.ItemID, HPPercent: source.HPPercent, Enemy: source.ItemID == "ufitem_020", Radius: 5, CooldownMillis: 5000, Source: constUtil.P13UnionFightGateItemSource + "；发布：" + source.Source}
		quote.SourceHash = p13UFGateItemOfferHash(&quote)
		if err := tx.Create(&quote).Error; err != nil {
			return err
		}
	}
	return p13UFGateFreeze(tx, match, buildings)
}

// p13UFGateFreeze 为真实道具与战车攻击冻结同一原八门实例，已有动态门不覆盖。
func p13UFGateFreeze(tx *gorm.DB, match *db.P13UnionFightMatch, buildings *pb.SCUFBuildingRefresh) error {
	var count int64
	if err := tx.Model(&db.P13UnionFightGateState{}).Where("match_id=?", match.ID).Count(&count).Error; err != nil {
		return err
	}
	if count == 8 {
		return p13UFGateItemOverlay(tx, match.ID, buildings)
	}
	if count != 0 || len(buildings.GateInfos) != 8 {
		return errors.New("实际原八门实例不完整")
	}
	for _, frame := range buildings.GateInfos {
		raw, err := proto.Marshal(frame)
		if err != nil {
			return err
		}
		row := db.P13UnionFightGateState{MatchID: match.ID, GUID: frame.GetGuid(), GateID: int64(frame.GetId()), Camp: int64(frame.GetCamp()), InitialHash: p6Digest(raw), Source: constUtil.P13UnionFightGateItemSource + "；初始八门源：" + constUtil.P13UnionFightFieldSource}
		if err = p13UFGateSave(tx, &row, frame); err != nil {
			return err
		}
	}
	return nil
}

// p13UFGateItemOverlay 原地图查询投影同一个实际动态城门，原初始快照仍参与完整SHA验证。
func p13UFGateItemOverlay(tx *gorm.DB, scope string, buildings *pb.SCUFBuildingRefresh) error {
	var rows []db.P13UnionFightGateState
	if err := tx.Where("match_id=?", scope).Find(&rows).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	if len(rows) != 8 {
		return errors.New("实际动态城门目录不是原8门")
	}
	known := map[int64]*pb.InstUFGate{}
	for _, row := range rows {
		frame, err := p13UFGateRead(&row)
		if err != nil {
			return err
		}
		known[row.GUID] = frame
	}
	for i, initial := range buildings.GateInfos {
		frame, ok := known[initial.GetGuid()]
		if !ok {
			return errors.New("实际动态门遗漏原建筑")
		}
		raw, err := proto.Marshal(initial)
		if err != nil {
			return err
		}
		found := false
		for _, row := range rows {
			if row.GUID == initial.GetGuid() {
				found = true
				if row.InitialHash != p6Digest(raw) || frame.GetMaxHp() != initial.GetMaxHp() {
					return errors.New("动态门与本场冻结初始HP来源不符")
				}
			}
		}
		if !found {
			return errors.New("动态门初始SHA不存在")
		}
		buildings.GateInfos[i] = frame
	}
	return nil
}

// p13UFGateItemTarget 冻结表决定本方/敌方与5米，最近同距按GUID为具名本地稳定顺序。
func (s *GameService) p13UFGateItemTarget(tx *gorm.DB, match *db.P13UnionFightMatch, actor *db.P13UnionFightFieldPlayer, quote *db.P13UnionFightGateItemOffer) (*db.P13UnionFightGateState, *pb.InstUFGate, error) {
	original, err := s.p13UFFieldOriginal(match, tx)
	if err != nil {
		return nil, nil, err
	}
	var rows []db.P13UnionFightGateState
	if err = tx.Where("match_id=?", match.ID).Find(&rows).Error; err != nil {
		return nil, nil, err
	}
	type candidate struct {
		row      db.P13UnionFightGateState
		frame    *pb.InstUFGate
		distance float64
	}
	var eligible []candidate
	for _, row := range rows {
		frame, err := p13UFGateRead(&row)
		if err != nil {
			return nil, nil, err
		}
		if (row.Camp != actor.Camp) != quote.Enemy || frame.GetHp() <= 0 {
			continue
		}
		cfg := original.table("UnionFightGate", "UFGId", strconv.FormatInt(row.GateID, 10))
		if cfg == nil || cfg["Camp"] != strconv.FormatInt(row.Camp, 10) {
			return nil, nil, errors.New("原城门冻结阵营不存在")
		}
		point, err := fieldPoint(cfg["Position"])
		if err != nil {
			return nil, nil, err
		}
		distance := p13UFResourceDistance(actor, point.GetX(), point.GetY(), point.GetZ())
		if distance <= quote.Radius {
			eligible = append(eligible, candidate{row, frame, distance})
		}
	}
	if len(eligible) == 0 {
		return nil, nil, errors.New("原5米内无本方/敌方存活城门目标")
	}
	sort.Slice(eligible, func(i, j int) bool {
		if eligible[i].distance != eligible[j].distance {
			return eligible[i].distance < eligible[j].distance
		}
		return eligible[i].row.GUID < eligible[j].row.GUID
	})
	return &eligible[0].row, eligible[0].frame, nil
}

// p13UFGateItemUse 真实库存与普通CD、唯一最近门HP、原回执同事务，修改机关状态不生成P6胜者。
func (s *GameService) p13UFGateItemUse(ctx context.Context, id int64, request *pb.CSUFPlayerUseItem, stamp int64, now time.Time) ([]proto.Message, error) {
	var reply *pb.SCUFPlayerUseItemDetail
	var update *pb.SCUFGate
	var tick *pb.SCUFElementTickExport
	var crash *pb.SCUFGateCrash
	scope := ""
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
		if actor.Hooked || now.UnixMilli() < match.StartAt || now.UnixMilli() >= match.EndAt || (frame.GetState() != 0 && frame.GetState() != 5) {
			return errors.New("原城门物品仅本場空闲/移动且非挂机角色可用")
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
			return errors.New("实际场内道具消费尚未开放")
		}
		var quote db.P13UnionFightGateItemOffer
		if err = tx.First(&quote, "match_id=? AND item_id=?", scope, request.GetItemId()).Error; err != nil {
			return errors.New("本场原城门道具效果未提前发布并冻结")
		}
		if quote.SourceHash != p13UFGateItemOfferHash(&quote) || quote.Radius != 5 || quote.CooldownMillis != 5000 || quote.HPPercent < 1 || quote.HPPercent > 100 || quote.Enemy != (quote.ItemID == "ufitem_020") || (quote.ItemID != "ufitem_019" && quote.ItemID != "ufitem_020") {
			return errors.New("原城门道具目标或具名效果SHA损坏")
		}
		state, err := p13UFItemProject(tx, scope, id, frame, now)
		if err != nil {
			return err
		}
		if state.NormalUntil > now.UnixMilli() {
			return errors.New("原普通道具共用5秒CD尚未结束")
		}
		occupied := map[int32]bool{}
		ids := map[string]bool{}
		var item *pb.InstUFItem
		for _, entry := range frame.Items {
			if entry.GetIndex() < 0 || entry.GetIndex() >= 5 || occupied[entry.GetIndex()] || ids[entry.GetId()] || entry.GetPlayerGuid() != id || entry.GetLeftUseNum() < 0 || entry.GetTotalUseNum() < entry.GetLeftUseNum() {
				return errors.New("原道具库存位置/身份损坏")
			}
			occupied[entry.GetIndex()] = true
			ids[entry.GetId()] = true
			if entry.GetId() == quote.ItemID {
				item = entry
			}
		}
		if item == nil || item.GetLeftUseNum() < 1 || item.GetLeftUseNum() > 5 {
			return errors.New("原城门道具实际库存不足或损坏")
		}
		row, target, err := s.p13UFGateItemTarget(tx, match, actor, &quote)
		if err != nil {
			return err
		}
		oldHP := target.GetHp()
		delta := target.GetMaxHp() * (float64(quote.HPPercent) / 100)
		if quote.Enemy {
			target.Hp = proto.Float64(max(float64(0), target.GetHp()-delta))
		} else {
			target.Hp = proto.Float64(min(target.GetMaxHp(), target.GetHp()+delta))
		}
		if err = p13UFGateSave(tx, row, target); err != nil {
			return err
		}
		item.LeftUseNum = proto.Int32(item.GetLeftUseNum() - 1)
		state.NormalUntil = now.UnixMilli() + quote.CooldownMillis
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
		reply = &pb.SCUFPlayerUseItemDetail{Player: frame, ItemId: request.ItemId}
		update = &pb.SCUFGate{Gate: []*pb.InstUFGate{target}}
		// 原GateRefresh不刷新已存在AI的instData；Tick先驱动HP/受击动画，再发原完整门数据同步查询。
		tick = &pb.SCUFElementTickExport{TargetGuid: []int64{row.GUID}, HpDelta: []float64{target.GetHp() - oldHP}, SourceGuid: []int64{id}}
		if target.GetHp() == 0 {
			crash = &pb.SCUFGateCrash{GateId: proto.Int64(row.GateID)}
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
		s.p13UFFieldBroadcast(ctx, scope, id, &pb.SCUFPlayerUseItem{Guid: proto.Int64(id), ItemId: request.ItemId})
		s.p13UFFieldBroadcast(ctx, scope, id, tick)
		s.p13UFFieldBroadcast(ctx, scope, id, update)
		messages = append(messages, tick, update)
		if crash != nil {
			s.p13UFFieldBroadcast(ctx, scope, id, crash)
			messages = append(messages, crash)
		}
	}
	return messages, nil
}
