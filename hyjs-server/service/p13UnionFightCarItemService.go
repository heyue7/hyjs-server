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

// InitializeP13UnionFightCarItem 炸弹效果来源单独迁移，原伤害量没有默认本地值。
func (s *GameService) InitializeP13UnionFightCarItem(ctx context.Context) error {
	if err := db.GetIns().WithContext(ctx).AutoMigrate(&db.P13UnionFightCarItemSource{}, &db.P13UnionFightCarItemOffer{}, &db.P13UnionFightCarImpact{}); err != nil {
		return err
	}
	return s.InitializeP13UnionFightItemBudget(ctx)
}
func p13UFCarItemOfferHash(row *db.P13UnionFightCarItemOffer) string {
	copy := *row
	copy.SourceHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}

// p13UFCarItemBuild 同场初次建图冻结原小/大型炸弹敌方5米选择与预先发布的伤害比例。
func (s *GameService) p13UFCarItemBuild(tx *gorm.DB, match *db.P13UnionFightMatch, original *GameService, now time.Time) error {
	var sources []db.P13UnionFightCarItemSource
	if err := tx.Where("day=? AND enabled=1", match.Day).Order("item_id").Find(&sources).Error; err != nil {
		return err
	}
	for _, source := range sources {
		if err := p13UFItemBudgetValidate(source.ItemID, p13UFItemBudgetConfig{source.BudgetRule, source.TeamUseLimit, source.TargetCooldownMillis, source.EffectFamily}); err != nil {
			return err
		}
		effect := ""
		switch source.ItemID {
		case "ufitem_017":
			effect = "buff,uf_buff_013"
		case "ufitem_018":
			effect = "buff,uf_buff_014"
		default:
			return errors.New("车炸弹来源超出原小/大型炸弹")
		}
		cfg := original.table("UnionFightItem", "UFItemId", source.ItemID)
		if cfg == nil || cfg["ItemSelector"] != "1,1,5" || cfg["ItemType"] != "0" || cfg["ItemUseCondition"] != "0" || cfg["ItemCatchMaxNum"] != "5" || cfg["ItemEffect"] != effect || source.HPPercent < 1 || source.HPPercent > 100 || source.PublishedAt <= 0 || source.PublishedAt > now.UnixMilli() || strings.TrimSpace(source.Source) == "" {
			return errors.New("原5米敌方车炸弹或提前具名比例来源无效")
		}
		cd, err := strconv.ParseFloat(original.table("GameConfigure", "StaticName", "UnionFightPlayerUseNormalItemCD")["NameValue"], 64)
		if err != nil || math.IsNaN(cd) || math.IsInf(cd, 0) || cd < 0.1 || cd > 3600 {
			return errors.New("原炸弹普通5秒CD缺失")
		}
		quote := db.P13UnionFightCarItemOffer{BudgetRule: source.BudgetRule, TeamUseLimit: source.TeamUseLimit, TargetCooldownMillis: source.TargetCooldownMillis, EffectFamily: source.EffectFamily, MatchID: match.ID, ItemID: source.ItemID, HPPercent: source.HPPercent, Radius: 5, CooldownMillis: int64(cd * 1000), Source: constUtil.P13UnionFightCarItemSource + "；发布：" + source.Source}
		quote.SourceHash = p13UFCarItemOfferHash(&quote)
		if err = tx.Create(&quote).Error; err != nil {
			return err
		}
	}
	return nil
}

// p13UFCarItemRead 车必须来自本场实际完成的原厂timer，核对厂/车两个持久SHA与冻结初始HP。
func p13UFCarItemRead(row *db.P13UnionFightFactoryState) (*pb.InstUFCariot, error) {
	if _, err := p13UFFactoryFrame(row); err != nil {
		return nil, err
	}
	if row.CompletedAt <= 0 || row.CarHash != p6Digest(row.CarFrame) {
		return nil, errors.New("原车厂尚未实际完成或车帧SHA损坏")
	}
	car := &pb.InstUFCariot{}
	if err := proto.Unmarshal(row.CarFrame, car); err != nil {
		return nil, err
	}
	if car.GetGuid() != p13UFCarInstanceGUID(row) || car.GetFactoryGuid() != row.GUID || int64(car.GetCamp()) != row.Camp || car.GetMaxHp() != row.ChariotHP || car.GetHp() < 0 || car.GetHp() > car.GetMaxHp() || car.GetMaxHp() <= 0 || math.IsNaN(car.GetHp()) || math.IsInf(car.GetHp(), 0) || math.IsNaN(car.GetMaxHp()) || math.IsInf(car.GetMaxHp(), 0) {
		return nil, errors.New("实际原首车身份/冻结初始HP损坏")
	}
	return car, nil
}

// p13UFCarItemTarget 与实际原路线producer及冷恢复共用持久位置，未发布速度的首车保留出生点。
func (s *GameService) p13UFCarItemTarget(tx *gorm.DB, match *db.P13UnionFightMatch, actor *db.P13UnionFightFieldPlayer, quote *db.P13UnionFightCarItemOffer) (*db.P13UnionFightFactoryState, *pb.InstUFCariot, *pb.PointNew, error) {
	original, err := s.p13UFFieldOriginal(match, tx)
	if err != nil {
		return nil, nil, nil, err
	}
	rows, err := s.p13UFCarScopeRows(tx, match.ID, nil)
	if err != nil {
		return nil, nil, nil, err
	}
	type candidate struct {
		row      db.P13UnionFightFactoryState
		car      *pb.InstUFCariot
		point    *pb.PointNew
		distance float64
	}
	var eligible []candidate
	for _, row := range rows {
		car, err := p13UFCarItemRead(&row)
		if err != nil {
			return nil, nil, nil, err
		}
		if row.Camp == actor.Camp || car.GetHp() <= 0 {
			continue
		}
		terminalFrames, terminalErr := p13UFCarAttackFrames(tx, &row)
		if terminalErr != nil {
			return nil, nil, nil, terminalErr
		}
		if len(terminalFrames) > 0 {
			if _, done := terminalFrames[0].(*pb.SCUFCariotComplete); done {
				continue
			}
		}
		cfg := original.table("UnionFightFactory", "UFFId", strconv.FormatInt(row.FactoryID, 10))
		if cfg == nil || cfg["Camp"] != strconv.FormatInt(row.Camp, 10) || cfg["CariotId"] != strconv.FormatInt(int64(car.GetCariotId()), 10) {
			return nil, nil, nil, errors.New("原战车目录/阵营来源不符")
		}
		point, err := s.p13UFCarMotionPoint(tx, &row, original)
		if err != nil {
			return nil, nil, nil, err
		}
		distance := p13UFResourceDistance(actor, point.GetX(), point.GetY(), point.GetZ())
		if distance <= quote.Radius {
			eligible = append(eligible, candidate{row, car, point, distance})
		}
	}
	if len(eligible) == 0 {
		return nil, nil, nil, errors.New("原5米内没有实际已产的敌方存活战车")
	}
	sort.Slice(eligible, func(i, j int) bool {
		if eligible[i].distance != eligible[j].distance {
			return eligible[i].distance < eligible[j].distance
		}
		return eligible[i].car.GetGuid() < eligible[j].car.GetGuid()
	})
	return &eligible[0].row, eligible[0].car, eligible[0].point, nil
}

// p13UFCarItemUse 实际敌车HP/原库存/普通CD/回执一次原子消费，不推断厂循环或战争胜者。
func (s *GameService) p13UFCarItemUse(ctx context.Context, id int64, request *pb.CSUFPlayerUseItem, stamp int64, now time.Time) ([]proto.Message, error) {
	var reply *pb.SCUFPlayerUseItemDetail
	var tick *pb.SCUFElementTickExport
	var update *pb.SCUFCaropt
	var crash *pb.SCUFCariotCrash
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
		if actor.Hooked || (frame.GetState() != 0 && frame.GetState() != 5) || now.UnixMilli() < match.StartAt || now.UnixMilli() >= match.EndAt {
			return errors.New("原炸弹只允许本场非挂机空闲/移动且正在比赛")
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
			return errors.New("本場实际道具消费尚未开放")
		}
		var quote db.P13UnionFightCarItemOffer
		if err = tx.First(&quote, "match_id=? AND item_id=?", scope, request.GetItemId()).Error; err != nil {
			return errors.New("本场原炸弹实际伤害来源未提前发布并冻结")
		}
		if quote.SourceHash != p13UFCarItemOfferHash(&quote) || quote.Radius != 5 || (quote.CooldownMillis < 100 || quote.CooldownMillis > 3600000) || quote.HPPercent < 1 || quote.HPPercent > 100 || (quote.ItemID != "ufitem_017" && quote.ItemID != "ufitem_018") {
			return errors.New("原车炸弹目标/具名比例SHA损坏")
		}
		if err = p13UFItemBudgetValidate(quote.ItemID, p13UFItemBudgetConfig{quote.BudgetRule, quote.TeamUseLimit, quote.TargetCooldownMillis, quote.EffectFamily}); err != nil {
			return err
		}
		state, err := p13UFItemProject(tx, scope, id, frame, now)
		if err != nil {
			return err
		}
		if state.NormalUntil > now.UnixMilli() {
			return errors.New("原炸弹共用普通5秒CD尚未结束")
		}
		occupied := map[int32]bool{}
		ids := map[string]bool{}
		var item *pb.InstUFItem
		for _, entry := range frame.Items {
			if entry.GetIndex() < 0 || entry.GetIndex() >= 5 || occupied[entry.GetIndex()] || ids[entry.GetId()] || entry.GetPlayerGuid() != id || entry.GetLeftUseNum() < 0 || entry.GetTotalUseNum() < entry.GetLeftUseNum() {
				return errors.New("本场实际炸弹库存格/数量损坏")
			}
			occupied[entry.GetIndex()] = true
			ids[entry.GetId()] = true
			if entry.GetId() == quote.ItemID {
				item = entry
			}
		}
		if item == nil || item.GetLeftUseNum() < 1 || item.GetLeftUseNum() > 5 {
			return errors.New("本場实际炸弹库存不足")
		}
		row, target, point, err := s.p13UFCarItemTarget(tx, match, actor, &quote)
		if err != nil {
			return err
		}
		oldHP := target.GetHp()
		newHP := max(float64(0), oldHP-target.GetMaxHp()*(float64(quote.HPPercent)/100))
		if newHP >= oldHP {
			return errors.New("本次炸弹没有可生效的实际车HP")
		}
		if err = p13UFItemBudgetConsume(tx, scope, actor.Camp, quote.ItemID, target.GetGuid(), quote.SourceHash, p13UFItemBudgetConfig{quote.BudgetRule, quote.TeamUseLimit, quote.TargetCooldownMillis, quote.EffectFamily}, now); err != nil {
			return err
		}
		target.Hp = proto.Float64(newHP)
		if err = p13UFCarImpactRecord(tx, row, target, db.P13UnionFightCarImpact{PlayerID: id, Camp: actor.Camp, ItemID: quote.ItemID, Stamp: stamp, OfferHash: quote.SourceHash, HPPercent: quote.HPPercent, InventoryBefore: int64(item.GetLeftUseNum()), InventoryAfter: int64(item.GetLeftUseNum() - 1), LogicalAt: now.UnixMilli(), AppliedAt: now.UnixMilli()}); err != nil {
			return err
		}
		rawCar, err := proto.Marshal(target)
		if err != nil {
			return err
		}
		row.CarFrame = rawCar
		row.CarHash = p6Digest(rawCar)
		if err = p13UFCarSave(tx, row); err != nil {
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
		tick = &pb.SCUFElementTickExport{TargetGuid: []int64{target.GetGuid()}, HpDelta: []float64{target.GetHp() - oldHP}, SourceGuid: []int64{id}}
		update = &pb.SCUFCaropt{Cariot: []*pb.InstUFCariot{target}, Local: []*pb.PointNew{point}}
		if target.GetHp() == 0 {
			crash = &pb.SCUFCariotCrash{Guid: target.Guid}
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
