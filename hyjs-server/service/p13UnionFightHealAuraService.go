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

func (s *GameService) InitializeP13UnionFightHealAura(ctx context.Context) error {
	return db.GetIns().WithContext(ctx).AutoMigrate(&db.P13UnionFightHealAuraSource{}, &db.P13UnionFightHealAuraOffer{}, &db.P13UnionFightHealAuraEffect{}, &db.P13UnionFightPlayerRecovery{}, &db.P13UnionFightHealAuraPending{})
}
func p13UFHealAuraOfferHash(row *db.P13UnionFightHealAuraOffer) string {
	copy := *row
	copy.SourceHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}
func p13UFHealAuraEffectHash(row *db.P13UnionFightHealAuraEffect) string {
	copy := *row
	copy.SourceHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}
func p13UFHealAuraQuoteValid(row *db.P13UnionFightHealAuraOffer) bool {
	return p13UFAuraEffectRuleValid(row.EffectRule) && (row.EffectRule == "" || row.HPPercent == 2 && row.TickMillis == 1000 && row.DurationMillis == 5000) && row.SourceHash == p13UFHealAuraOfferHash(row) && row.ItemID == "ufitem_013" && row.HPPercent >= 1 && row.HPPercent <= 100 && row.Radius > 0 && row.Radius <= 20 && !math.IsNaN(row.Radius) && !math.IsInf(row.Radius, 0) && row.AuthorityMask >= 1 && row.AuthorityMask <= 7 && row.DurationMillis == 5000 && row.CooldownMillis >= 100 && row.CooldownMillis <= 3600000 && row.TickMillis >= 100 && row.TickMillis <= 5000 && 5000%row.TickMillis == 0
}

// p13UFHealAuraBuild 原013自体挂载、buff009五秒及团队治疗语义保留；未知数量/范围/tick/资格须具名预发布。
func (s *GameService) p13UFHealAuraBuild(tx *gorm.DB, match *db.P13UnionFightMatch, original *GameService, now time.Time) error {
	var sources []db.P13UnionFightHealAuraSource
	if err := tx.Where("day=? AND enabled=1", match.Day).Find(&sources).Error; err != nil {
		return err
	}
	for _, source := range sources {
		cfg := original.table("UnionFightItem", "UFItemId", source.ItemID)
		buff := original.table("UnionFightBuff", "UFBuffId", "uf_buff_009")
		if source.ItemID != "ufitem_013" || cfg == nil || cfg["ItemSelector"] != "0" || cfg["ItemEffect"] != "buff,uf_buff_009" || cfg["ItemType"] != "1" || cfg["ItemUseCondition"] != "2" || cfg["ItemCatchMaxNum"] != "5" || buff == nil || buff["BuffClass"] != "2" || buff["lastTime"] != "5.0" || buff["buffLastEffect"] != "recoverMatrix" || source.PublishedAt <= 0 || source.PublishedAt > now.UnixMilli() || strings.TrimSpace(source.Source) == "" {
			return errors.New("原回复法阵/团队治疗来源未完整提前发布")
		}
		cd, cdErr := strconv.ParseFloat(original.table("GameConfigure", "StaticName", "UnionFightPlayerUseDiamondItemCD")["NameValue"], 64)
		if cdErr != nil || math.IsNaN(cd) || math.IsInf(cd, 0) || cd < 0.1 || cd > 3600 {
			return errors.New("原法阵钻石类五秒CD缺失")
		}
		quote := db.P13UnionFightHealAuraOffer{EffectRule: source.EffectRule, MatchID: match.ID, ItemID: source.ItemID, HPPercent: source.HPPercent, Radius: source.Radius, TickMillis: source.TickMillis, AuthorityMask: source.AuthorityMask, DurationMillis: 5000, CooldownMillis: int64(cd * 1000), Source: constUtil.P13UnionFightHealAuraSource + "；发布：" + source.Source}
		if source.EffectRule == constUtil.P13UFHealAuraPendingRule && source.PublishedAt >= match.StartAt {
			return errors.New("占用回复法阵规则必须开场前发布")
		}
		quote.SourceHash = p13UFHealAuraOfferHash(&quote)
		if !p13UFHealAuraQuoteValid(&quote) {
			return errors.New("本地法阵比例/范围/tick/资格数值无效")
		}
		if err := tx.Create(&quote).Error; err != nil {
			return err
		}
	}
	return nil
}
func p13UFHealAuraRead(tx *gorm.DB, row *db.P13UnionFightHealAuraEffect) (*db.P13UnionFightHealAuraOffer, error) {
	var quote db.P13UnionFightHealAuraOffer
	if err := tx.First(&quote, "match_id=? AND item_id=?", row.MatchID, "ufitem_013").Error; err != nil {
		return nil, err
	}
	if !p13UFHealAuraQuoteValid(&quote) || row.SourceHash != p13UFHealAuraEffectHash(row) || row.OfferHash != quote.SourceHash || row.CasterID <= 0 || row.StartedAt <= 0 || row.ExpiresAt <= row.StartedAt || row.ExpiresAt > row.StartedAt+quote.DurationMillis || row.NextTickAt < row.StartedAt+quote.TickMillis || (row.NextTickAt-row.StartedAt)%quote.TickMillis != 0 || row.NextTickAt > row.StartedAt+quote.DurationMillis+quote.TickMillis || (row.EndedAt != 0 && (row.EndedAt < row.StartedAt || row.EndedAt > row.ExpiresAt)) || row.GUID != p13UFFieldGUID(row.MatchID, "heal-aura", strconv.FormatInt(row.CasterID, 10)+"/"+strconv.FormatInt(row.StartedAt, 10)) {
		return nil, errors.New("实际回复法阵持久SHA/时序损坏")
	}
	return &quote, nil
}
func p13UFHealAuraProject(tx *gorm.DB, scope string, id int64, frame *pb.InstUFPlayer, now time.Time) (bool, error) {
	var rows []db.P13UnionFightHealAuraEffect
	if err := tx.Where("match_id=? AND caster_id=?", scope, id).Order("guid").Find(&rows).Error; err != nil {
		return false, err
	}
	known := map[int64]bool{}
	var buffs []*pb.InstUFBuff
	for _, row := range rows {
		if _, err := p13UFHealAuraRead(tx, &row); err != nil {
			return false, err
		}
		known[row.GUID] = true
		if row.EndedAt == 0 && row.StartedAt <= now.UnixMilli() && now.UnixMilli() < row.ExpiresAt {
			buffs = append(buffs, &pb.InstUFBuff{Guid: proto.Int64(row.GUID), BuffId: proto.String("uf_buff_009"), MountGuid: proto.Int64(id)})
		}
	}
	for _, buff := range frame.Buffs {
		if !known[buff.GetGuid()] {
			buffs = append(buffs, buff)
		}
	}
	before, _ := proto.Marshal(frame)
	frame.Buffs = buffs
	after, _ := proto.Marshal(frame)
	return !bytes.Equal(before, after), nil
}

// p13UFHealAuraUse 真实库存/共diamondCD、原自体buff、持久逐tick及回执原子创建，不即时假造战斗伤害或领奖。
func (s *GameService) p13UFHealAuraUse(ctx context.Context, id int64, request *pb.CSUFPlayerUseItem, stamp int64, now time.Time) ([]proto.Message, error) {
	var reply *pb.SCUFPlayerUseItemDetail
	var on *pb.SCUFElementBuffOn
	scope := ""
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
		if actor.Hooked || (frame.GetState() != 0 && frame.GetState() != 5) || now.UnixMilli() < match.StartAt || now.UnixMilli() >= match.EndAt {
			return errors.New("回复法阵需要当前存活非挂机比赛角色")
		}
		if err = p13UFItemLife(frame, &db.P13UnionFightItemOffer{ItemID: "ufitem_013", HPPercent: 0}); err != nil {
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
			return errors.New("法阵原Current握手尚未完成")
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
			return errors.New("回复法阵真实库存消费未开放")
		}
		var quote db.P13UnionFightHealAuraOffer
		if err = tx.First(&quote, "match_id=? AND item_id=?", scope, request.GetItemId()).Error; err != nil {
			return errors.New("原未知法阵数额/范围/资格来源未提前冻结")
		}
		if !p13UFHealAuraQuoteValid(&quote) {
			return errors.New("回复法阵冻结SHA损坏")
		}
		if frame.GetUnionAuthority() < 0 || frame.GetUnionAuthority() > 2 || quote.AuthorityMask&(1<<frame.GetUnionAuthority()) == 0 {
			return errors.New("具名法阵资格不包含当前冻结职位")
		}
		if err = p13UFAuraReserve(tx, scope, &quote, int64(snapshot.PlayerLimit)); err != nil {
			return err
		}
		state, err := p13UFItemProject(tx, scope, id, frame, now)
		if err != nil {
			return err
		}
		if state.DiamondUntil > now.UnixMilli() {
			return errors.New("原法阵共钻石类五秒CD未结束")
		}
		var item *pb.InstUFItem
		slots := map[int32]bool{}
		ids := map[string]bool{}
		for _, entry := range frame.Items {
			if entry.GetIndex() < 0 || entry.GetIndex() >= 5 || slots[entry.GetIndex()] || ids[entry.GetId()] || entry.GetPlayerGuid() != id || entry.GetLeftUseNum() < 0 || entry.GetTotalUseNum() < entry.GetLeftUseNum() {
				return errors.New("实际法阵库存身份/格子损坏")
			}
			slots[entry.GetIndex()] = true
			ids[entry.GetId()] = true
			if entry.GetId() == quote.ItemID {
				item = entry
			}
		}
		if item == nil || item.GetLeftUseNum() < 1 || item.GetLeftUseNum() > 5 {
			return errors.New("真实回复法阵库存不足")
		}
		effect := db.P13UnionFightHealAuraEffect{MatchID: scope, GUID: p13UFFieldGUID(scope, "heal-aura", strconv.FormatInt(id, 10)+"/"+strconv.FormatInt(now.UnixMilli(), 10)), CasterID: id, StartedAt: now.UnixMilli(), ExpiresAt: min(now.UnixMilli()+quote.DurationMillis, match.EndAt), NextTickAt: now.UnixMilli() + quote.TickMillis, OfferHash: quote.SourceHash}
		effect.SourceHash = p13UFHealAuraEffectHash(&effect)
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
		if _, err = p13UFHealAuraProject(tx, scope, id, frame, now); err != nil {
			return err
		}
		if err = p13UFFieldSaveActor(tx, actor, frame); err != nil {
			return err
		}
		reply = &pb.SCUFPlayerUseItemDetail{Player: frame, ItemId: request.ItemId}
		on = &pb.SCUFElementBuffOn{TargetGuid: []int64{id}, Buff: []*pb.InstUFBuff{{Guid: proto.Int64(effect.GUID), BuffId: proto.String("uf_buff_009"), MountGuid: proto.Int64(id)}}}
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
	return []proto.Message{reply, on}, nil
}

// TickP13UnionFightHealAura 持久NextTickAt逐次核对现存活友军与法阵中心；同刻worker不会重复恢复HP。
func (s *GameService) TickP13UnionFightHealAura(ctx context.Context, now time.Time) error {
	var pending []db.P13UnionFightHealAuraEffect
	if err := db.GetIns().WithContext(ctx).Where("ended_at=0 AND (next_tick_at<=? OR expires_at<=?)", now.UnixMilli(), now.UnixMilli()).Order("match_id,guid").Find(&pending).Error; err != nil {
		return err
	}
	scopes := make([]string, 0, len(pending))
	for _, row := range pending {
		scopes = append(scopes, row.MatchID)
	}
	clocks, clockErr := p13UFWorldClockBatch(ctx, scopes, now)
	if clockErr != nil {
		return clockErr
	}
	for _, candidate := range pending {
		if clocks.paused(candidate.MatchID) {
			continue
		}
		var frames []proto.Message
		err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := ServerGameClockGuardTransaction(tx); err != nil {
				return err
			}

			var policy db.P13GuildPolicy
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&policy, 1).Error; err != nil {
				return err
			}
			if closed, err := p13UFWorldSealed(tx, candidate.MatchID); err != nil {
				return err
			} else if closed {
				return nil
			}
			var effect db.P13UnionFightHealAuraEffect
			if err := tx.First(&effect, "match_id=? AND guid=?", candidate.MatchID, candidate.GUID).Error; err != nil {
				return err
			}
			quote, err := p13UFHealAuraRead(tx, &effect)
			if err != nil {
				return err
			}
			if effect.EndedAt != 0 {
				return nil
			}
			var match db.P13UnionFightMatch
			if err = tx.First(&match, "id=?", effect.MatchID).Error; err != nil {
				return err
			}
			if quote.EffectRule == constUtil.P13UFHealAuraPendingRule {
				frames, err = s.p13UFHealAuraPendingTick(tx, &effect, quote, &match, policy.ManagementEnabled, clocks.end(&match), now)
				return err
			}
			cancel := !policy.ManagementEnabled || effect.NextTickAt > clocks.end(&match)
			var caster db.P13UnionFightFieldPlayer
			if err = tx.First(&caster, "match_id=? AND player_id=?", effect.MatchID, effect.CasterID).Error; err != nil {
				return err
			}
			casterFrame, err := p13UFFieldReadActor(&caster)
			if err != nil {
				return err
			}
			member, err := p13GuildMember(tx, effect.CasterID)
			if err != nil {
				return err
			}
			cancel = cancel || member.GuildID != caster.GuildID || caster.LeftAt != 0 || caster.Hooked || (casterFrame.GetState() != 0 && casterFrame.GetState() != 5)
			if cancel {
				effect.EndedAt = min(now.UnixMilli(), effect.ExpiresAt, clocks.end(&match))
			} else {
				var targets []p13UFFieldLiveRow
				sceneExpression := "json_extract(p.game,'$.position.scene')"
				if tx.Dialector.Name() == "mysql" {
					sceneExpression = "JSON_UNQUOTE(" + sceneExpression + ")"
				}
				if err = tx.Table("p13_union_fight_field_players AS a").Select("a.*,m.guild_id AS current_guild_id,p.session AS profile_session,"+sceneExpression+" AS actual_scene,COALESCE(json_extract(p.game,'$.position_loading'),0) AS position_loading").Joins("JOIN profiles AS p ON p.id=a.player_id").Joins("JOIN p13_guild_members AS m ON m.player_id=a.player_id").Where("a.match_id=? AND a.camp=? AND a.left_at=0", effect.MatchID, caster.Camp).Order("a.player_id").Scan(&targets).Error; err != nil {
					return err
				}
				for effect.NextTickAt <= now.UnixMilli() && effect.NextTickAt <= effect.ExpiresAt && effect.NextTickAt <= clocks.end(&match) {
					full := &pb.SCUFPlayer{}
					tick := &pb.SCUFElementTickExport{}
					for index := range targets {
						live := &targets[index]
						target := &live.P13UnionFightFieldPlayer
						if live.CurrentGuildID != target.GuildID || live.ActualScene != constUtil.P13UnionFightFieldTarget || live.PositionLoading || target.SessionHash != p6Digest(live.ProfileSession) {
							continue
						}
						if p13UFResourceDistance(target, caster.X, caster.Y, caster.Z) > quote.Radius {
							continue
						}
						frame, err := p13UFFieldReadActor(target)
						if err != nil {
							return err
						}
						if target.Hooked || (frame.GetState() != 0 && frame.GetState() != 5) {
							continue
						}
						old := float64(0)
						for _, hp := range frame.GetFightInfo().GetHpPos() {
							old += hp
						}
						beforeLife := proto.Clone(frame).(*pb.InstUFPlayer)
						if err = p13UFItemLife(frame, &db.P13UnionFightItemOffer{ItemID: quote.ItemID, HPPercent: quote.HPPercent}); err != nil {
							return err
						}
						current := float64(0)
						for _, hp := range frame.GetFightInfo().GetHpPos() {
							current += hp
						}
						if current == old {
							continue
						}
						if err = p13UFPlayerRecoveryRecordOpen(tx, beforeLife, frame, db.P13UnionFightPlayerRecovery{
							MatchID: effect.MatchID, Kind: constUtil.P13UFRecoveryAura, ItemID: quote.ItemID,
							CasterID: effect.CasterID, EffectGUID: effect.GUID, OfferHash: quote.SourceHash,
							HPPercent: quote.HPPercent, LogicalAt: effect.NextTickAt, AppliedAt: now.UnixMilli(),
						}); err != nil {
							return err
						}
						if err = p13UFFieldSaveActor(tx, target, frame); err != nil {
							return err
						}
						tick.TargetGuid = append(tick.TargetGuid, target.PlayerID)
						tick.SourceGuid = append(tick.SourceGuid, effect.CasterID)
						tick.HpDelta = append(tick.HpDelta, current-old)
						full.Player = append(full.Player, frame)
						full.Local = append(full.Local, &pb.PointNew{X: proto.Float32(target.X), Y: proto.Float32(target.Y), Z: proto.Float32(target.Z)})
					}
					if len(tick.TargetGuid) > 0 {
						frames = append(frames, tick, full)
					}
					effect.NextTickAt += quote.TickMillis
				}
				if now.UnixMilli() >= min(effect.ExpiresAt, clocks.end(&match)) {
					effect.EndedAt = min(effect.ExpiresAt, clocks.end(&match))
				}
			}
			effect.SourceHash = p13UFHealAuraEffectHash(&effect)
			if err = tx.Save(&effect).Error; err != nil {
				return err
			}
			if effect.EndedAt > 0 {
				if err = tx.First(&caster, "match_id=? AND player_id=?", effect.MatchID, effect.CasterID).Error; err != nil {
					return err
				}
				casterFrame, err = p13UFFieldReadActor(&caster)
				if err != nil {
					return err
				}
				if _, err = p13UFHealAuraProject(tx, effect.MatchID, effect.CasterID, casterFrame, now); err != nil {
					return err
				}
				if err = p13UFFieldSaveActor(tx, &caster, casterFrame); err != nil {
					return err
				}
				frames = append(frames, &pb.SCUFElementBuffOff{BuffGuid: []int64{effect.GUID}})
			}
			return nil
		})
		if err != nil {
			return err
		}
		if len(frames) > 0 {
			if err := p13UFPublishBeforeAbort(ctx, candidate.MatchID, func() {
				for _, frame := range frames {
					s.p13UFFieldBroadcast(ctx, candidate.MatchID, 0, frame)
				}
			}); err != nil {
				return err
			}
		}
	}
	return nil
}
