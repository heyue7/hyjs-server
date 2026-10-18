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

func (s *GameService) InitializeP13UnionFightSpeedItem(ctx context.Context) error {
	return db.GetIns().WithContext(ctx).AutoMigrate(&db.P13UnionFightSpeedItemSource{}, &db.P13UnionFightSpeedItemOffer{}, &db.P13UnionFightSpeedItemEffect{})
}
func p13UFSpeedOfferHash(row *db.P13UnionFightSpeedItemOffer) string {
	copy := *row
	copy.SourceHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}
func p13UFSpeedEffectHash(row *db.P13UnionFightSpeedItemEffect) string {
	copy := *row
	copy.SourceHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}

// p13UFSpeedBuild 原015友方3米和buff008五秒来自原表，未知倍率及UseCondition1权限必须具名预发布。
func (s *GameService) p13UFSpeedBuild(tx *gorm.DB, match *db.P13UnionFightMatch, original *GameService, now time.Time) error {
	var sources []db.P13UnionFightSpeedItemSource
	if err := tx.Where("day=? AND enabled=1", match.Day).Find(&sources).Error; err != nil {
		return err
	}
	for _, source := range sources {
		cfg := original.table("UnionFightItem", "UFItemId", source.ItemID)
		buff := original.table("UnionFightBuff", "UFBuffId", "uf_buff_008")
		if source.ItemID != "ufitem_015" || cfg == nil || cfg["ItemSelector"] != "4,0,3" || cfg["ItemEffect"] != "buff,uf_buff_008" || cfg["ItemType"] != "1" || cfg["ItemUseCondition"] != "1" || cfg["ItemCatchMaxNum"] != "5" || buff == nil || buff["BuffClass"] != "0" || buff["lastTime"] != "5.0" || source.Multiplier <= 1 || source.Multiplier > 10 || math.IsNaN(source.Multiplier) || math.IsInf(source.Multiplier, 0) || source.AuthorityMask < 1 || source.AuthorityMask > 7 || source.PublishedAt <= 0 || source.PublishedAt > now.UnixMilli() || strings.TrimSpace(source.Source) == "" {
			return errors.New("原冲锋3米/5秒与未知资格/倍率提前来源缺失")
		}
		cd, err := strconv.ParseFloat(original.table("GameConfigure", "StaticName", "UnionFightPlayerUseDiamondItemCD")["NameValue"], 64)
		if err != nil || cd != 5 {
			return errors.New("原冲锋钻石类五秒CD缺失")
		}
		quote := db.P13UnionFightSpeedItemOffer{MatchID: match.ID, ItemID: source.ItemID, Multiplier: source.Multiplier, AuthorityMask: source.AuthorityMask, Radius: 3, DurationMillis: 5000, CooldownMillis: 5000, Source: constUtil.P13UnionFightSpeedItemSource + "；发布：" + source.Source}
		quote.SourceHash = p13UFSpeedOfferHash(&quote)
		if err = tx.Create(&quote).Error; err != nil {
			return err
		}
	}
	return nil
}
func p13UFSpeedEnd(row *db.P13UnionFightSpeedItemEffect) int64 {
	if row.EndedAt > 0 {
		return min(row.EndedAt, row.ExpiresAt)
	}
	return row.ExpiresAt
}

func p13UFSpeedRead(tx *gorm.DB, row *db.P13UnionFightSpeedItemEffect) (*db.P13UnionFightSpeedItemOffer, error) {
	var quote db.P13UnionFightSpeedItemOffer
	if err := tx.First(&quote, "match_id=? AND item_id=?", row.MatchID, "ufitem_015").Error; err != nil {
		return nil, err
	}
	if quote.SourceHash != p13UFSpeedOfferHash(&quote) || quote.ItemID != "ufitem_015" || quote.Multiplier <= 1 || quote.Multiplier > 10 || math.IsNaN(quote.Multiplier) || math.IsInf(quote.Multiplier, 0) || quote.AuthorityMask < 1 || quote.AuthorityMask > 7 || quote.Radius != 3 || quote.DurationMillis != 5000 || quote.CooldownMillis != 5000 || row.SourceHash != p13UFSpeedEffectHash(row) || row.Multiplier != quote.Multiplier || row.OfferHash != quote.SourceHash || row.TargetID <= 0 || row.CasterID <= 0 || row.StartedAt <= 0 || row.ExpiresAt <= row.StartedAt || row.ExpiresAt > row.StartedAt+quote.DurationMillis || (row.EndedAt != 0 && (row.EndedAt < row.StartedAt || row.EndedAt > row.ExpiresAt)) || row.GUID != p13UFFieldGUID(row.MatchID, "speed-buff", strconv.FormatInt(row.CasterID, 10)+"/"+strconv.FormatInt(row.StartedAt, 10)+"/"+strconv.FormatInt(row.TargetID, 10)) {
		return nil, errors.New("冲锋冻结效果SHA/原身份时序损坏")
	}
	return &quote, nil
}

// p13UFSpeedMoveAllowance 对每段实际时钟积分，重叠效果取具名本地最大倍率，过期后不会借用整段历史加速。
func p13UFSpeedMoveAllowance(tx *gorm.DB, actor *db.P13UnionFightFieldPlayer, base float64, at int64) (float64, float64, error) {
	start := actor.UpdatedAt
	if at < start {
		start = at
	}
	var effects []db.P13UnionFightSpeedItemEffect
	if err := tx.Where("match_id=? AND target_id=? AND started_at<=? AND expires_at>?", actor.MatchID, actor.PlayerID, at, start).Find(&effects).Error; err != nil {
		return 0, 0, err
	}
	boundaries := []int64{start, at}
	rate := base
	for _, row := range effects {
		if _, err := p13UFSpeedRead(tx, &row); err != nil {
			return 0, 0, err
		}
		end := p13UFSpeedEnd(&row)
		if end <= start {
			continue
		}
		boundaries = append(boundaries, max(start, row.StartedAt), min(at, end))
		if row.StartedAt <= at && at < end {
			rate = math.Max(rate, base*row.Multiplier)
		}
	}
	sort.Slice(boundaries, func(i, j int) bool { return boundaries[i] < boundaries[j] })
	earned := float64(0)
	for i := 1; i < len(boundaries); i++ {
		left, right := boundaries[i-1], boundaries[i]
		if right <= left {
			continue
		}
		multiplier := float64(1)
		for _, row := range effects {
			if row.StartedAt <= left && p13UFSpeedEnd(&row) >= right {
				multiplier = math.Max(multiplier, row.Multiplier)
			}
		}
		earned += base * multiplier * float64(right-left) / 1000
	}
	return earned, rate, nil
}

// p13UFSpeedProject 当前原帧只携带尚未到期的实际冲锋buff；保留其它领域buff并核对每个来源。
func p13UFSpeedProject(tx *gorm.DB, scope string, id int64, frame *pb.InstUFPlayer, now time.Time) (bool, error) {
	var effects []db.P13UnionFightSpeedItemEffect
	if err := tx.Where("match_id=? AND target_id=?", scope, id).Order("guid").Find(&effects).Error; err != nil {
		return false, err
	}
	known := map[int64]bool{}
	var active []*pb.InstUFBuff
	for _, row := range effects {
		if _, err := p13UFSpeedRead(tx, &row); err != nil {
			return false, err
		}
		known[row.GUID] = true
		if row.StartedAt <= now.UnixMilli() && now.UnixMilli() < p13UFSpeedEnd(&row) {
			active = append(active, &pb.InstUFBuff{Guid: proto.Int64(row.GUID), BuffId: proto.String("uf_buff_008"), MountGuid: proto.Int64(id)})
		}
	}
	var buffs []*pb.InstUFBuff
	for _, buff := range frame.Buffs {
		if !known[buff.GetGuid()] {
			buffs = append(buffs, buff)
		}
	}
	buffs = append(buffs, active...)
	before, _ := proto.Marshal(frame)
	frame.Buffs = buffs
	after, _ := proto.Marshal(frame)
	return !bytes.Equal(before, after), nil
}

// p13UFSpeedUse 同事务扣真实冲锋库存/钻石类CD、冻结当前位置内真实友方和每位buff，原请求回执不重复施加。
func (s *GameService) p13UFSpeedUse(ctx context.Context, id int64, request *pb.CSUFPlayerUseItem, stamp int64, now time.Time) ([]proto.Message, error) {
	var reply *pb.SCUFPlayerUseItemDetail
	var on *pb.SCUFElementBuffOn
	var players *pb.SCUFPlayer
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
			return errors.New("实际冲锋需要当前非挂机存活比赛角色")
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
			return errors.New("冲锋原Current握手尚未完成")
		}
		raw := []byte(scope + "\x00" + request.GetItemId())
		if stamp > 0 {
			var receipt db.P13GuildReceipt
			err = tx.First(&receipt, "player_id=? AND operation=? AND stamp=?", id, constUtil.P13UnionFightItemUseOperation, stamp).Error
			if err == nil {
				if !bytes.Equal(raw, receipt.Request) {
					return errors.New("公会战道具回执冲突")
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
			return errors.New("冲锋真实库存消费尚未开放")
		}
		var quote db.P13UnionFightSpeedItemOffer
		if err = tx.First(&quote, "match_id=? AND item_id=?", scope, request.GetItemId()).Error; err != nil {
			return errors.New("本场未知冲锋资格/倍率来源未提前发布")
		}
		if quote.SourceHash != p13UFSpeedOfferHash(&quote) || quote.ItemID != "ufitem_015" || quote.DurationMillis != 5000 || quote.CooldownMillis != 5000 || quote.Radius != 3 || quote.Multiplier <= 1 || quote.Multiplier > 10 || math.IsNaN(quote.Multiplier) || math.IsInf(quote.Multiplier, 0) || quote.AuthorityMask < 1 || quote.AuthorityMask > 7 {
			return errors.New("冲锋冻结资格/倍率SHA损坏")
		}
		if frame.GetUnionAuthority() < 0 || frame.GetUnionAuthority() > 2 || quote.AuthorityMask&(1<<frame.GetUnionAuthority()) == 0 {
			return errors.New("本场具名冲锋资格不包含当前真实职位")
		}
		state, err := p13UFItemProject(tx, scope, id, frame, now)
		if err != nil {
			return err
		}
		if state.DiamondUntil > now.UnixMilli() {
			return errors.New("原冲锋钻石类五秒CD尚未结束")
		}
		var item *pb.InstUFItem
		slots := map[int32]bool{}
		ids := map[string]bool{}
		for _, entry := range frame.Items {
			if entry.GetIndex() < 0 || entry.GetIndex() >= 5 || slots[entry.GetIndex()] || ids[entry.GetId()] || entry.GetPlayerGuid() != id || entry.GetLeftUseNum() < 0 || entry.GetTotalUseNum() < entry.GetLeftUseNum() {
				return errors.New("实际冲锋库存身份/格子损坏")
			}
			slots[entry.GetIndex()] = true
			ids[entry.GetId()] = true
			if entry.GetId() == quote.ItemID {
				item = entry
			}
		}
		if item == nil || item.GetLeftUseNum() < 1 || item.GetLeftUseNum() > 5 {
			return errors.New("本场真实冲锋库存不足")
		}
		var targets []db.P13UnionFightFieldPlayer
		if err = tx.Where("match_id=? AND camp=? AND left_at=0", scope, actor.Camp).Order("player_id").Find(&targets).Error; err != nil {
			return err
		}
		on = &pb.SCUFElementBuffOn{}
		players = &pb.SCUFPlayer{}
		for _, target := range targets {
			if p13UFResourceDistance(&target, actor.X, actor.Y, actor.Z) > quote.Radius {
				continue
			}
			member, memberErr := p13GuildMember(tx, target.PlayerID)
			if memberErr != nil {
				return memberErr
			}
			if member.GuildID != target.GuildID {
				continue
			}
			var targetProfile db.Profile
			if err = tx.First(&targetProfile, target.PlayerID).Error; err != nil {
				return err
			}
			_, targetGame, err := decodeProfileForCalendarUpdate(tx, &targetProfile)
			if err != nil {
				return err
			}
			if targetGame.PositionLoading || targetGame.Position.GetScene() != constUtil.P13UnionFightFieldTarget {
				continue
			}
			current, _, _, targetFrame, currentErr := s.p13UFFieldCurrent(tx, target.PlayerID, now)
			if currentErr != nil {
				return currentErr
			}
			if current.Hooked || (targetFrame.GetState() != 0 && targetFrame.GetState() != 5) {
				continue
			}
			effect := db.P13UnionFightSpeedItemEffect{MatchID: scope, GUID: p13UFFieldGUID(scope, "speed-buff", strconv.FormatInt(id, 10)+"/"+strconv.FormatInt(now.UnixMilli(), 10)+"/"+strconv.FormatInt(target.PlayerID, 10)), TargetID: target.PlayerID, CasterID: id, StartedAt: now.UnixMilli(), ExpiresAt: min(now.UnixMilli()+quote.DurationMillis, match.EndAt), Multiplier: quote.Multiplier, OfferHash: quote.SourceHash}
			effect.SourceHash = p13UFSpeedEffectHash(&effect)
			if err = tx.Create(&effect).Error; err != nil {
				return err
			}
			if target.PlayerID == id {
				targetFrame = frame
				current = actor
			}
			if _, err = p13UFSpeedProject(tx, scope, target.PlayerID, targetFrame, now); err != nil {
				return err
			}
			if err = p13UFFieldSaveActor(tx, current, targetFrame); err != nil {
				return err
			}
			buff := &pb.InstUFBuff{Guid: proto.Int64(effect.GUID), BuffId: proto.String("uf_buff_008"), MountGuid: proto.Int64(target.PlayerID)}
			on.TargetGuid = append(on.TargetGuid, target.PlayerID)
			on.Buff = append(on.Buff, buff)
			players.Player = append(players.Player, targetFrame)
			players.Local = append(players.Local, &pb.PointNew{X: proto.Float32(current.X), Y: proto.Float32(current.Y), Z: proto.Float32(current.Z)})
		}
		if len(on.TargetGuid) == 0 {
			return errors.New("原友方3米内没有实际存活加载角色")
		}
		item.LeftUseNum = proto.Int32(item.GetLeftUseNum() - 1)
		state.DiamondUntil = now.UnixMilli() + quote.CooldownMillis
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
	if !fresh {
		return []proto.Message{reply}, nil
	}
	s.p13UFFieldBroadcast(ctx, scope, id, &pb.SCUFPlayerUseItem{Guid: proto.Int64(id), ItemId: request.ItemId})
	s.p13UFFieldBroadcast(ctx, scope, id, on)
	s.p13UFFieldBroadcast(ctx, scope, id, players)
	return []proto.Message{reply, on, players}, nil
}

// TickP13UnionFightSpeedItem 到期一次撤销原buff视觉，移动历史仍保留完整五秒积分来源。
func (s *GameService) TickP13UnionFightSpeedItem(ctx context.Context, now time.Time) error {
	var effects []db.P13UnionFightSpeedItemEffect
	if err := db.GetIns().WithContext(ctx).Where("ended_at=0 AND expires_at<=?", now.UnixMilli()).Order("match_id,guid").Find(&effects).Error; err != nil {
		return err
	}
	for _, candidate := range effects {
		fresh := false
		err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := ServerGameClockGuardTransaction(tx); err != nil {
				return err
			}

			var policy db.P13GuildPolicy
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&policy, 1).Error; err != nil {
				return err
			}
			var row db.P13UnionFightSpeedItemEffect
			if err := tx.First(&row, "match_id=? AND guid=?", candidate.MatchID, candidate.GUID).Error; err != nil {
				return err
			}
			if _, err := p13UFSpeedRead(tx, &row); err != nil {
				return err
			}
			if row.EndedAt != 0 || row.ExpiresAt > now.UnixMilli() {
				return nil
			}
			row.EndedAt = row.ExpiresAt
			row.SourceHash = p13UFSpeedEffectHash(&row)
			if err := tx.Save(&row).Error; err != nil {
				return err
			}
			var actor db.P13UnionFightFieldPlayer
			if err := tx.First(&actor, "match_id=? AND player_id=?", row.MatchID, row.TargetID).Error; err != nil {
				return err
			}
			frame, err := p13UFFieldReadActor(&actor)
			if err != nil {
				return err
			}
			if _, err = p13UFSpeedProject(tx, row.MatchID, row.TargetID, frame, now); err != nil {
				return err
			}
			if err = p13UFFieldSaveActor(tx, &actor, frame); err != nil {
				return err
			}
			fresh = true
			return nil
		})
		if err != nil {
			return err
		}
		if fresh {
			s.p13UFFieldBroadcast(ctx, candidate.MatchID, 0, &pb.SCUFElementBuffOff{BuffGuid: []int64{candidate.GUID}})
		}
	}
	return nil
}
