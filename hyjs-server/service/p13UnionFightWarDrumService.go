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
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func (s *GameService) InitializeP13UnionFightWarDrum(ctx context.Context) error {
	return db.GetIns().WithContext(ctx).AutoMigrate(&db.P13UnionFightWarDrumSource{}, &db.P13UnionFightWarDrumOffer{}, &db.P13UnionFightWarDrumEffect{})
}
func p13UFDrumOfferHash(row *db.P13UnionFightWarDrumOffer) string {
	copy := *row
	copy.SourceHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}
func p13UFDrumEffectHash(row *db.P13UnionFightWarDrumEffect) string {
	copy := *row
	copy.SourceHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}
func p13UFDrumQuoteValid(row *db.P13UnionFightWarDrumOffer) bool {
	return row.SourceHash == p13UFDrumOfferHash(row) && row.ItemID == "ufitem_014" && row.AttackPercent >= 1 && row.AttackPercent <= 500 && row.Radius > 0 && row.Radius <= 20 && !math.IsNaN(row.Radius) && !math.IsInf(row.Radius, 0) && row.DurationMillis >= 1000 && row.DurationMillis <= 300000 && row.AuthorityMask >= 1 && row.AuthorityMask <= 7 && row.CooldownMillis >= 100 && row.CooldownMillis <= 3600000 && row.ModelName == constUtil.P13UnionFightWarDrumLocalModel && row.ModelScale >= .01 && row.ModelScale <= 10 && !math.IsNaN(row.ModelScale) && !math.IsInf(row.ModelScale, 0)
}

// p13UFDrumBuild 原014/totem1和周围提高战力语义保留；原模型绑定/范围/时长/属性及资格缺失须具名本地发布。
func (s *GameService) p13UFDrumBuild(tx *gorm.DB, match *db.P13UnionFightMatch, original *GameService, now time.Time) error {
	var sources []db.P13UnionFightWarDrumSource
	if err := tx.Where("day=? AND enabled=1", match.Day).Find(&sources).Error; err != nil {
		return err
	}
	for _, source := range sources {
		cfg := original.table("UnionFightItem", "UFItemId", source.ItemID)
		if source.ItemID != "ufitem_014" || cfg == nil || cfg["ItemSelector"] != "0" || cfg["ItemEffect"] != "totem,1" || cfg["ItemType"] != "1" || cfg["ItemUseCondition"] != "2" || cfg["ItemCatchMaxNum"] != "5" || source.PublishedAt <= 0 || source.PublishedAt > now.UnixMilli() || strings.TrimSpace(source.Source) == "" {
			return errors.New("原战鼓/totem1和未知规则来源未完整提前发布")
		}
		cd, cdErr := strconv.ParseFloat(original.table("GameConfigure", "StaticName", "UnionFightPlayerUseDiamondItemCD")["NameValue"], 64)
		if cdErr != nil || math.IsNaN(cd) || math.IsInf(cd, 0) || cd < 0.1 || cd > 3600 {
			return errors.New("公会战道具冷却配置无效")
		}
		row := db.P13UnionFightWarDrumOffer{MatchID: match.ID, ItemID: source.ItemID, AttackPercent: source.AttackPercent, Radius: source.Radius, DurationMillis: source.DurationMillis, AuthorityMask: source.AuthorityMask, CooldownMillis: int64(cd * 1000), ModelName: source.ModelName, ModelScale: source.ModelScale, Source: constUtil.P13UnionFightWarDrumSource + "；发布：" + source.Source}
		row.SourceHash = p13UFDrumOfferHash(&row)
		if !p13UFDrumQuoteValid(&row) {
			return errors.New("具名战鼓本地数值/已核模型/资格无效")
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
	}
	return nil
}
func p13UFDrumRead(tx *gorm.DB, row *db.P13UnionFightWarDrumEffect) (*db.P13UnionFightWarDrumOffer, error) {
	var quote db.P13UnionFightWarDrumOffer
	if err := tx.First(&quote, "match_id=? AND item_id=?", row.MatchID, "ufitem_014").Error; err != nil {
		return nil, err
	}
	return p13UFDrumReadLoaded(row, &quote)
}
func p13UFDrumReadLoaded(row *db.P13UnionFightWarDrumEffect, frozen *db.P13UnionFightWarDrumOffer) (*db.P13UnionFightWarDrumOffer, error) {
	if row == nil || frozen == nil || frozen.MatchID != row.MatchID || frozen.ItemID != "ufitem_014" {
		return nil, errors.New("战鼓冻结报价缺失")
	}
	quote := *frozen
	if !p13UFDrumQuoteValid(&quote) || row.SourceHash != p13UFDrumEffectHash(row) || row.OfferHash != quote.SourceHash || row.CasterID <= 0 || row.GuildID == "" || (row.Camp != 0 && row.Camp != 1) || row.StartedAt <= 0 || row.ExpiresAt <= row.StartedAt || row.ExpiresAt > row.StartedAt+quote.DurationMillis || (row.EndedAt != 0 && (row.EndedAt < row.StartedAt || row.EndedAt > row.ExpiresAt)) || row.GUID != p13UFFieldGUID(row.MatchID, "war-drum", strconv.FormatInt(row.CasterID, 10)+"/"+strconv.FormatInt(row.StartedAt, 10)) || math.IsNaN(float64(row.X)) || math.IsNaN(float64(row.Y)) || math.IsNaN(float64(row.Z)) || math.IsInf(float64(row.X), 0) || math.IsInf(float64(row.Y), 0) || math.IsInf(float64(row.Z), 0) {
		return nil, errors.New("战鼓持久源SHA/原实例/时序损坏")
	}
	return &quote, nil
}
func p13UFDrumFrame(row *db.P13UnionFightWarDrumEffect, quote *db.P13UnionFightWarDrumOffer, now time.Time) *pb.SCUFTotemCreate {
	return &pb.SCUFTotemCreate{Totem: &pb.InstUFTotem{Guid: proto.Int64(row.GUID), TotemId: proto.String("1"), ModelName: proto.String(quote.ModelName), ModelScale: proto.Float64(quote.ModelScale), SourcePlayerGuid: proto.Int64(row.CasterID), LastTime: proto.Float64(float64(max(int64(0), row.ExpiresAt-now.UnixMilli())) / 1000), Range: proto.Float64(quote.Radius)}, Pos: &pb.PointNew{X: proto.Float32(row.X), Y: proto.Float32(row.Y), Z: proto.Float32(row.Z)}}
}

// p13UFDrumFrames 恢复当前同场真实静止图腾，原lastTime按绝对到期投影，重登不延长。
func p13UFDrumFrames(tx *gorm.DB, scope string, wanted map[int64]bool, now time.Time) ([]proto.Message, error) {
	var rows []db.P13UnionFightWarDrumEffect
	if err := tx.Where("match_id=? AND ended_at=0 AND expires_at>?", scope, now.UnixMilli()).Order("guid").Find(&rows).Error; err != nil {
		return nil, err
	}
	var frames []proto.Message
	for _, row := range rows {
		quote, err := p13UFDrumRead(tx, &row)
		if err != nil {
			return nil, err
		}
		if len(wanted) > 0 && !wanted[row.GUID] {
			continue
		}
		member, err := p13GuildMember(tx, row.CasterID)
		if err != nil {
			return nil, err
		}
		if member.GuildID != row.GuildID {
			continue
		}
		var caster db.P13UnionFightFieldPlayer
		if err = tx.First(&caster, "match_id=? AND player_id=?", row.MatchID, row.CasterID).Error; err != nil {
			return nil, err
		}
		if caster.LeftAt != 0 || caster.GuildID != row.GuildID || caster.Camp != row.Camp {
			continue
		}
		frames = append(frames, p13UFDrumFrame(&row, quote, now))
	}
	return frames, nil
}

// p13UFDrumUse 同事务消费真实战鼓库存和diamondCD并生成原静止totem1，不把提高战力文本当原数值。
func (s *GameService) p13UFDrumUse(ctx context.Context, id int64, request *pb.CSUFPlayerUseItem, stamp int64, now time.Time) ([]proto.Message, error) {
	var reply *pb.SCUFPlayerUseItemDetail
	var created *pb.SCUFTotemCreate
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
			return errors.New("实际战鼓需要本场存活非挂机比赛角色")
		}
		if err = p13UFItemLife(frame, &db.P13UnionFightItemOffer{ItemID: "ufitem_014", HPPercent: 0}); err != nil {
			return err
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
			return errors.New("战鼓原Current握手未完成")
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
			return errors.New("战鼓真实库存消费未开放")
		}
		var quote db.P13UnionFightWarDrumOffer
		if err = tx.First(&quote, "match_id=? AND item_id=?", scope, request.GetItemId()).Error; err != nil {
			return errors.New("本场原未知战鼓规则来源尚未提前冻结")
		}
		if !p13UFDrumQuoteValid(&quote) {
			return errors.New("战鼓冻结来源SHA损坏")
		}
		if frame.GetUnionAuthority() < 0 || frame.GetUnionAuthority() > 2 || quote.AuthorityMask&(1<<frame.GetUnionAuthority()) == 0 {
			return errors.New("具名战鼓资格不含当前冻结职位")
		}
		state, err := p13UFItemProject(tx, scope, id, frame, now)
		if err != nil {
			return err
		}
		if state.DiamondUntil > now.UnixMilli() {
			return errors.New("原战鼓共钻石类五秒CD未结束")
		}
		var item *pb.InstUFItem
		slots := map[int32]bool{}
		ids := map[string]bool{}
		for _, entry := range frame.Items {
			if entry.GetIndex() < 0 || entry.GetIndex() >= 5 || slots[entry.GetIndex()] || ids[entry.GetId()] || entry.GetPlayerGuid() != id || entry.GetLeftUseNum() < 0 || entry.GetTotalUseNum() < entry.GetLeftUseNum() {
				return errors.New("实际战鼓库存身份/格子损坏")
			}
			slots[entry.GetIndex()] = true
			ids[entry.GetId()] = true
			if entry.GetId() == quote.ItemID {
				item = entry
			}
		}
		if item == nil || item.GetLeftUseNum() < 1 || item.GetLeftUseNum() > 5 {
			return errors.New("真实战鼓库存不足")
		}
		effect := db.P13UnionFightWarDrumEffect{MatchID: scope, GUID: p13UFFieldGUID(scope, "war-drum", strconv.FormatInt(id, 10)+"/"+strconv.FormatInt(now.UnixMilli(), 10)), CasterID: id, GuildID: actor.GuildID, Camp: actor.Camp, X: actor.X, Y: actor.Y, Z: actor.Z, StartedAt: now.UnixMilli(), ExpiresAt: min(now.UnixMilli()+quote.DurationMillis, match.EndAt), OfferHash: quote.SourceHash}
		effect.SourceHash = p13UFDrumEffectHash(&effect)
		if err = tx.Create(&effect).Error; err != nil {
			return err
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
		created = p13UFDrumFrame(&effect, &quote, now)
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
	s.p13UFFieldBroadcast(ctx, scope, id, created)
	return []proto.Message{reply, created}, nil
}

// ProjectP13UnionFightWarDrumBattle 深克隆原冻结战斗快照，仅施加来源明确的攻击属性；CurrentHP投影仍由真实战斗producer负责。
// 返回AppliedProperties仅供FreezeBattleInput审计；此函数不运行计算、不改变原存档，不生成winner/score/奖励。
func (s *GameService) ProjectP13UnionFightWarDrumBattle(ctx context.Context, id int64, now time.Time) (*pb.BattlePlayer, map[string][]string, error) {
	var battle *pb.BattlePlayer
	var applied map[string][]string
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var err error
		battle, applied, err = s.p13UFDrumBattle(tx, id, now)
		return err
	})
	return battle, applied, err
}

func (s *GameService) p13UFDrumBattle(tx *gorm.DB, id int64, now time.Time) (*pb.BattlePlayer, map[string][]string, error) {
	actor, match, _, frame, err := s.p13UFFieldCurrent(tx, id, now)
	if err != nil {
		return nil, nil, err
	}
	var profile db.Profile
	if err = tx.First(&profile, id).Error; err != nil {
		return nil, nil, err
	}
	_, game, err := decodeProfileForCalendarUpdate(tx, &profile)
	if err != nil {
		return nil, nil, err
	}
	return s.p13UFDrumBattleLoaded(tx, actor, match, frame, game, now)
}

// Loaded consumes the exact actor/frame and locked profile already authenticated
// by the caller in this guild-gated transaction; effects/expiry remain live reads.
func (s *GameService) p13UFDrumBattleLoaded(tx *gorm.DB, actor *db.P13UnionFightFieldPlayer, match *db.P13UnionFightMatch, frame *pb.InstUFPlayer, game *model.GameState, now time.Time) (*pb.BattlePlayer, map[string][]string, error) {
	var battle *pb.BattlePlayer
	applied := map[string][]string{}
	err := func() error {
		if actor == nil || match == nil || frame == nil || game == nil || actor.MatchID != match.ID || actor.PlayerID != frame.GetGuid() || actor.Camp != int64(frame.GetCamp()) || actor.LeftAt != 0 {
			return errors.New("战鼓受益需要当前实际存活比赛角色")
		}
		id := actor.PlayerID
		if p6Digest(actor.Battle) != actor.BattleHash {
			return errors.New("战鼓受益原冻结角色身份损坏")
		}
		if actor.Hooked || (frame.GetState() != 0 && frame.GetState() != 1 && frame.GetState() != 6 && frame.GetState() != 5) || now.UnixMilli() < match.StartAt || now.UnixMilli() >= match.EndAt {
			return errors.New("战鼓受益需要当前实际存活比赛角色")
		}
		var err error
		if game.PositionLoading || game.Position.GetScene() != constUtil.P13UnionFightFieldTarget {
			return errors.New("战鼓受益原Current尚未加载")
		}
		battle = &pb.BattlePlayer{}
		if err = proto.Unmarshal(actor.Battle, battle); err != nil {
			return err
		}
		if battle.GetPlayerId() != id || len(battle.Formation) == 0 {
			return errors.New("战鼓受益原冻结角色身份损坏")
		}
		var rows []db.P13UnionFightWarDrumEffect
		if err = tx.Where("match_id=? AND camp=? AND guild_id=? AND ended_at=0 AND started_at<=? AND expires_at>?", match.ID, actor.Camp, actor.GuildID, now.UnixMilli(), now.UnixMilli()).Order("guid").Find(&rows).Error; err != nil {
			return err
		}
		var quotes []db.P13UnionFightWarDrumOffer
		if len(rows) > 0 {
			if err = tx.Where("match_id=? AND item_id=?", match.ID, "ufitem_014").Find(&quotes).Error; err != nil {
				return err
			}
		}
		var quote *db.P13UnionFightWarDrumOffer
		if len(quotes) == 1 {
			quote = &quotes[0]
		}
		ids := make([]int64, 0, len(rows))
		for _, row := range rows {
			ids = append(ids, row.CasterID)
		}
		members := map[int64]db.P13GuildMember{}
		casters := map[int64]db.P13UnionFightFieldPlayer{}
		for offset := 0; offset < len(ids); offset += 128 {
			batch := ids[offset:min(offset+128, len(ids))]
			var ms []db.P13GuildMember
			if err = tx.Where("player_id IN ?", batch).Find(&ms).Error; err != nil {
				return err
			}
			for _, m := range ms {
				members[m.PlayerID] = m
			}
			var actors []db.P13UnionFightFieldPlayer
			if err = tx.Where("match_id=? AND player_id IN ?", match.ID, batch).Find(&actors).Error; err != nil {
				return err
			}
			for _, a := range actors {
				casters[a.PlayerID] = a
			}
		}
		percent := int64(0)
		var benefits []string
		for _, row := range rows {
			quote, err := p13UFDrumReadLoaded(&row, quote)
			if err != nil {
				return err
			}
			member := members[row.CasterID]
			if member.GuildID != row.GuildID || p13UFResourceDistance(actor, row.X, row.Y, row.Z) > quote.Radius {
				continue
			}
			caster, exists := casters[row.CasterID]
			if !exists {
				return gorm.ErrRecordNotFound
			}
			if caster.LeftAt != 0 || caster.GuildID != row.GuildID || caster.Camp != row.Camp {
				continue
			}
			if quote.AttackPercent > percent {
				percent = quote.AttackPercent
				benefits = nil
			}
			if quote.AttackPercent == percent {
				benefits = append(benefits, constUtil.P13UnionFightWarDrumSource+";effect="+strconv.FormatInt(row.GUID, 10)+";offer="+quote.SourceHash+";effectsha="+row.SourceHash)
			}
		}
		if percent == 0 {
			return nil
		}
		seen := map[string]bool{}
		poses := map[int32]bool{}
		for _, hero := range battle.Formation {
			if hero == nil || hero.Prop == nil || hero.GetGuid() == "" || seen[hero.GetGuid()] || poses[hero.GetPose()] {
				return errors.New("战鼓冻结Formation GUID/pose身份损坏")
			}
			seen[hero.GetGuid()] = true
			poses[hero.GetPose()] = true
			value := hero.Prop.GetAtt()
			if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) || value > 1e15 {
				return errors.New("原战鼓冻结攻击数值无效")
			}
			value *= 1 + float64(percent)/100
			if math.IsInf(value, 0) || value > 1e15 {
				return errors.New("具名战鼓倍率超出完整战斗属性界限")
			}
			hero.Prop.Att = proto.Float64(value)
			applied[hero.GetGuid()] = append([]string(nil), benefits...)
		}
		return nil
	}()
	return battle, applied, err
}

// TickP13UnionFightWarDrum 原固定totem按已保存到期销毁；施放者退会/退出同场及时撤销，不延长重登生命周期。
func (s *GameService) TickP13UnionFightWarDrum(ctx context.Context, now time.Time) error {
	var rows []db.P13UnionFightWarDrumEffect
	if err := db.GetIns().WithContext(ctx).Where("ended_at=0").Order("match_id,guid").Find(&rows).Error; err != nil {
		return err
	}
	scopes := make([]string, 0, len(rows))
	for _, candidate := range rows {
		scopes = append(scopes, candidate.MatchID)
	}
	clocks, clockErr := p13UFWorldClockBatch(ctx, scopes, now)
	if clockErr != nil {
		return clockErr
	}
	for _, candidate := range rows {
		if clocks.paused(candidate.MatchID) {
			continue
		}
		fresh := false
		err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := ServerGameClockGuardTransaction(tx); err != nil {
				return err
			}

			var gate db.P13GuildPolicy
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&gate, 1).Error; err != nil {
				return err
			}
			if closed, err := p13UFWorldSealed(tx, candidate.MatchID); err != nil {
				return err
			} else if closed {
				return nil
			}
			var row db.P13UnionFightWarDrumEffect
			if err := tx.First(&row, "match_id=? AND guid=?", candidate.MatchID, candidate.GUID).Error; err != nil {
				return err
			}
			if _, err := p13UFDrumRead(tx, &row); err != nil {
				return err
			}
			if row.EndedAt != 0 {
				return nil
			}
			var match db.P13UnionFightMatch
			if err := tx.First(&match, "id=?", row.MatchID).Error; err != nil {
				return err
			}
			member, err := p13GuildMember(tx, row.CasterID)
			if err != nil {
				return err
			}
			var actor db.P13UnionFightFieldPlayer
			if err = tx.First(&actor, "match_id=? AND player_id=?", row.MatchID, row.CasterID).Error; err != nil {
				return err
			}
			if now.UnixMilli() < row.ExpiresAt && now.UnixMilli() < clocks.end(&match) && member.GuildID == row.GuildID && actor.GuildID == row.GuildID && actor.LeftAt == 0 && gate.ManagementEnabled {
				return nil
			}
			row.EndedAt = min(now.UnixMilli(), row.ExpiresAt, clocks.end(&match))
			if row.EndedAt < row.StartedAt {
				return nil
			}
			row.SourceHash = p13UFDrumEffectHash(&row)
			if err = tx.Save(&row).Error; err != nil {
				return err
			}
			fresh = true
			return nil
		})
		if err != nil {
			return err
		}
		if fresh {
			if err := p13UFPublishBeforeAbort(ctx, candidate.MatchID, func() {
				s.p13UFFieldBroadcast(ctx, candidate.MatchID, 0, &pb.SCUFTotemDestroy{Guid: proto.Int64(candidate.GUID)})
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

// p13UFDrumCancel 在实际退场事务里永久撤销本次场上图腾，快速退场重入不会绕过一次销毁。
func p13UFDrumCancel(tx *gorm.DB, scope string, id int64, now time.Time) ([]proto.Message, error) {
	var rows []db.P13UnionFightWarDrumEffect
	if err := tx.Where("match_id=? AND caster_id=? AND ended_at=0", scope, id).Order("guid").Find(&rows).Error; err != nil {
		return nil, err
	}
	var frames []proto.Message
	for _, row := range rows {
		if _, err := p13UFDrumRead(tx, &row); err != nil {
			return nil, err
		}
		row.EndedAt = max(row.StartedAt, min(row.ExpiresAt, now.UnixMilli()))
		row.SourceHash = p13UFDrumEffectHash(&row)
		if err := tx.Save(&row).Error; err != nil {
			return nil, err
		}
		frames = append(frames, &pb.SCUFTotemDestroy{Guid: proto.Int64(row.GUID)})
	}
	return frames, nil
}
