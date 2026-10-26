package service

import (
	"bytes"
	"context"
	"encoding/binary"
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

func (s *GameService) InitializeP13UnionFightGuardAudit(ctx context.Context) error {
	return db.GetIns().WithContext(ctx).AutoMigrate(&db.P13UnionFightGuardAuditSource{}, &db.P13UnionFightGuardAuditOffer{}, &db.P13UnionFightGuardAuditAttempt{})
}
func p13UFGuardOfferHash(row *db.P13UnionFightGuardAuditOffer) string {
	copy := *row
	copy.SourceHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}
func p13UFGuardAttemptHash(row *db.P13UnionFightGuardAuditAttempt) string {
	copy := *row
	copy.SourceHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}

// 保存原建筑绑定的守卫索引，重复GUID会使整场来源失效。
func p13UFGuardAuditUnits(buildings *pb.SCUFBuildingRefresh) (map[int64]*pb.InstUFGuard, error) {
	rows := []*pb.InstUFGuard{}
	for _, row := range buildings.GetBaseCampInfos() {
		rows = append(rows, row.GetGuard())
	}
	for _, row := range buildings.GetTowerInfos() {
		rows = append(rows, row.GetGuard())
	}
	for _, row := range buildings.GetFactoryInfos() {
		rows = append(rows, row.GetGuard())
	}
	result := map[int64]*pb.InstUFGuard{}
	for _, row := range rows {
		if row == nil {
			continue
		}
		if row.GetGuid() <= 0 || result[row.GetGuid()] != nil || row.GetFightInfo().GetGuid() != row.GetGuid() {
			return nil, errors.New("原守卫GUID/生命映射损坏")
		}
		result[row.GetGuid()] = row
	}
	return result, nil
}
func p13UFGuardAuditMonster(offer *db.P13UnionFightGuardAuditOffer, guard *pb.InstUFGuard) (*pb.BattleMonster, error) {
	monster := &pb.BattleMonster{}
	hp := guard.GetFightInfo()
	if offer.SourceHash != p13UFGuardOfferHash(offer) || offer.MonsterHash != p6Digest(offer.Monster) || offer.ArriveRange != .8 || strings.TrimSpace(offer.Source) == "" || proto.Unmarshal(offer.Monster, monster) != nil || len(monster.GetFormation()) != 1 || monster.GetInst() == nil || guard.GetGuid() != offer.GuardGUID || guard.GetUFGuardId() != offer.GuardID || int64(guard.GetCamp()) != offer.Camp || guard.GetBindGuid() != offer.BindGUID || guard.GetState() != 0 || len(hp.GetIndex()) != 1 || hp.GetIndex()[0] != 0 || len(hp.GetHpPos()) != 1 || len(hp.GetMaxHpPos()) != 1 || hp.GetMaxHpPos()[0] != offer.InitialHP {
		return nil, errors.New("原守卫冻结模板/绑定SHA不符")
	}
	hero := monster.GetFormation()[0]
	if hero == nil || hero.GetId() != offer.HeroID || hero.GetGuid() == "" || hero.GetLevel() <= 0 || hero.GetBloodNumber() <= 0 || hero.GetProp() == nil || hero.GetInst() == nil || hero.GetProp().GetHp() != offer.InitialHP || offer.InitialHP <= 0 || offer.InitialHP > 1e15 || math.IsNaN(offer.InitialHP) || math.IsInf(offer.InitialHP, 0) || hero.GetInst().GetCurHp() < 0 || hero.GetInst().GetCurHp() > offer.InitialHP || hp.GetHpPos()[0] <= 0 || hp.GetHpPos()[0] > offer.InitialHP || math.IsNaN(hp.GetHpPos()[0]) || math.IsInf(hp.GetHpPos()[0], 0) {
		return nil, errors.New("守卫具名本地单实体HP/等级模板不符")
	}
	monster.MonsterId = proto.Int64(guard.GetGuid())
	hero.Guid = proto.String(fmt.Sprintf("guard/%d/%s", guard.GetGuid(), offer.GuardID))
	hero.Inst.CurHp = proto.Float64(hp.GetHpPos()[0])
	monster.Inst.Anger = proto.Float64(hp.GetAnger())
	return monster, nil
}
func (s *GameService) p13UFGuardAuditBuild(tx *gorm.DB, match *db.P13UnionFightMatch, buildings *pb.SCUFBuildingRefresh, original *GameService, now time.Time, frozenRules ...string) error {
	rule := ""
	if len(frozenRules) > 1 {
		return errors.New(constUtil.P13UFGuardConflict)
	}
	if len(frozenRules) == 1 {
		rule = frozenRules[0]
	}
	if !p13UFGuardTemplateRuleKnown(rule) {
		return errors.New(constUtil.P13UFGuardConflict)
	}
	var sources []db.P13UnionFightGuardAuditSource
	if err := tx.Find(&sources, "day=? AND enabled=1", match.Day).Error; err != nil {
		return err
	}
	if len(sources) == 0 {
		if rule == constUtil.P13UFGuardMirrorRule {
			return errors.New(constUtil.P13UFGuardConflict)
		}
		return nil
	}
	var common db.P13UnionFightBattleAuditOffer
	if err := tx.First(&common, "match_id=?", match.ID).Error; err != nil {
		return err
	}
	if !p13UFBattleAuditQuoteValid(&common) {
		return errors.New("守卫计算共用种子/倍率来源损坏")
	}
	distance, err := strconv.ParseFloat(original.table("GameConfigure", "StaticName", "UnionFightArriveGuard")["NameValue"], 64)
	if err != nil || distance != .8 {
		return errors.New("原抵达守卫范围来源损坏")
	}
	guards, err := p13UFGuardAuditUnits(buildings)
	if err != nil {
		return err
	}
	byID := map[string]*pb.InstUFGuard{}
	for _, guard := range guards {
		byID[guard.GetUFGuardId()] = guard
	}
	for _, source := range sources {
		guard := byID[source.GuardID]
		row := original.table("UnionFightGuard", "UFGuardId", source.GuardID)
		if guard == nil || row == nil || source.PublishedAt <= 0 || source.PublishedAt > now.UnixMilli() || strings.TrimSpace(source.Source) == "" {
			return errors.New("提前具名本地守卫模板/原守卫缺失")
		}
		if len(guard.GetFightInfo().GetMaxHpPos()) != 1 {
			return errors.New("原守卫单实体HP映射缺失")
		}
		heroID := row["HeroId"]
		if rule == constUtil.P13UFGuardMirrorRule {
			monster, e := p13UFGuardMirrorSource(&source, original, match, now)
			if e != nil {
				return e
			}
			heroID = monster.Formation[0].GetId()
		} else if source.GuardRule != "" {
			return errors.New(constUtil.P13UFGuardConflict)
		}
		offer := db.P13UnionFightGuardAuditOffer{GuardRule: rule, MatchID: match.ID, GuardGUID: guard.GetGuid(), GuardID: source.GuardID, HeroID: heroID, Camp: int64(guard.GetCamp()), BindGUID: guard.GetBindGuid(), InitialHP: guard.GetFightInfo().GetMaxHpPos()[0], ArriveRange: distance, BattleOfferHash: common.SourceHash, Monster: append([]byte(nil), source.Monster...), MonsterHash: p6Digest(source.Monster), Source: constUtil.P13UnionFightGuardAuditSource + "；发布：" + source.Source}
		offer.SourceHash = p13UFGuardOfferHash(&offer)
		if _, err = p13UFGuardAuditMonster(&offer, guard); err != nil {
			return err
		}
		if err = tx.Create(&offer).Error; err != nil {
			return err
		}
	}
	return nil
}

// 已有回执先确定入口，玩家离开/守卫新状态不会把重试路由到另一种战斗。
func (s *GameService) p13UFAskFightAnyAudit(ctx context.Context, id int64, request *pb.CSUFPlayerAskFight, stamp int64, now time.Time) ([]proto.Message, error) {
	guardTarget := false
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		_, match, snapshot, _, err := s.p13UFFieldCurrent(tx, id, now)
		if err != nil {
			return err
		}
		var previous db.P13UnionFightBattleAuditAttempt
		err = tx.First(&previous, "match_id=? AND player_id=? AND stamp=?", match.ID, id, stamp).Error
		if err == nil {
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var guardPrevious db.P13UnionFightGuardAuditAttempt
		err = tx.First(&guardPrevious, "match_id=? AND player_id=? AND stamp=?", match.ID, id, stamp).Error
		if err == nil {
			guardTarget = true
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		buildings := &pb.SCUFBuildingRefresh{}
		if proto.Unmarshal(snapshot.Buildings, buildings) != nil {
			return errors.New("原战场建筑快照损坏")
		}
		guards, err := p13UFGuardAuditUnits(buildings)
		if err != nil {
			return err
		}
		guardTarget = guards[request.GetTargetGuid()] != nil
		return nil
	})
	if err != nil {
		return nil, err
	}
	if guardTarget {
		return s.p13UFAskGuardAudit(ctx, id, request, stamp, now)
	}
	return s.p13UFAskFightAudit(ctx, id, request, stamp, now)
}

// 与玩家审计链共用原锁及同事务实际Capture/Compute，提交审计后返回结算拒绝。
func (s *GameService) p13UFAskGuardAudit(ctx context.Context, id int64, request *pb.CSUFPlayerAskFight, stamp int64, now time.Time) ([]proto.Message, error) {
	if request == nil || request.GetTargetGuid() <= 0 || stamp <= 0 {
		return nil, errors.New(constUtil.P13UnionFightBattleAuditRefusal)
	}
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var policy db.P13GuildPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&policy, 1).Error; err != nil {
			return err
		}
		actor, match, snapshot, frame, err := s.p13UFFieldCurrent(tx, id, now)
		if err != nil {
			return err
		}
		if actor.Hooked || (frame.GetState() != 0 && frame.GetState() != 5) || now.UnixMilli() < match.StartAt || now.UnixMilli() >= match.EndAt {
			return errors.New("当前角色不能发起守卫战斗")
		}
		var playerReceipt db.P13UnionFightBattleAuditAttempt
		if err = tx.First(&playerReceipt, "match_id=? AND player_id=? AND stamp=?", match.ID, id, stamp).Error; err == nil {
			return errors.New("原AskFight时间戳已绑定玩家战斗")
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var quote db.P13UnionFightGuardAuditOffer
		if err = tx.First(&quote, "match_id=? AND guard_guid=?", match.ID, request.GetTargetGuid()).Error; err != nil {
			return err
		}
		var common db.P13UnionFightBattleAuditOffer
		if err = tx.First(&common, "match_id=?", match.ID).Error; err != nil {
			return err
		}
		if !p13UFBattleAuditQuoteValid(&common) || quote.BattleOfferHash != common.SourceHash || quote.SourceHash != p13UFGuardOfferHash(&quote) {
			return errors.New("守卫计算冻结来源SHA损坏")
		}
		requestHash := p6Digest([]byte(match.ID + "\x00" + strconv.FormatInt(id, 10) + "\x00" + strconv.FormatInt(request.GetTargetGuid(), 10)))
		var previous db.P13UnionFightGuardAuditAttempt
		err = tx.First(&previous, "match_id=? AND player_id=? AND stamp=?", match.ID, id, stamp).Error
		if err == nil {
			if previous.SourceHash != p13UFGuardAttemptHash(&previous) || previous.RequestHash != requestHash || previous.OfferHash != quote.SourceHash || previous.GuardGUID != request.GetTargetGuid() || previous.Refusal != constUtil.P13UnionFightBattleAuditRefusal {
				return errors.New("守卫审计回执冲突")
			}
			return p13UFGuardAuditVerifyRecord(tx, &previous)
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		buildings := &pb.SCUFBuildingRefresh{}
		if proto.Unmarshal(snapshot.Buildings, buildings) != nil {
			return errors.New("原战场建筑快照损坏")
		}
		guards, err := p13UFGuardAuditUnits(buildings)
		if err != nil {
			return err
		}
		guard := guards[request.GetTargetGuid()]
		if guard == nil || guard.GetCamp() == int32(actor.Camp) || guard.GetInitPosition() == nil || p13UFResourceDistance(actor, guard.GetInitPosition().GetX(), guard.GetInitPosition().GetY(), guard.GetInitPosition().GetZ()) > quote.ArriveRange {
			return errors.New("目标尚未抵达原敌方守卫范围")
		}
		monster, err := p13UFGuardAuditMonster(&quote, guard)
		if err != nil {
			return err
		}
		attacker, applied, err := s.p13UFFieldBattle(tx, id, now)
		if err != nil {
			return err
		}
		monsterHero := monster.GetFormation()[0]
		if len(applied[monsterHero.GetGuid()]) > 0 {
			return errors.New("守卫与玩家英雄GUID冲突")
		}
		guardRaw, err := proto.MarshalOptions{Deterministic: true}.Marshal(guard)
		if err != nil {
			return err
		}
		applied[monsterHero.GetGuid()] = []string{fmt.Sprintf("guardFrameSHA=%s;templateSHA=%s;offerSHA=%s;originalHero=%s;camp=%d;bind=%d;localFieldIndex=0->templatePose=%d;CurHp=%.17g", p6Digest(guardRaw), quote.MonsterHash, quote.SourceHash, quote.HeroID, quote.Camp, quote.BindGUID, monsterHero.GetPose(), monsterHero.GetInst().GetCurHp())}
		guid := p13UFFieldGUID(match.ID, "guard-audit", strconv.FormatInt(id, 10)+"/"+strconv.FormatInt(stamp, 10))
		fightID := strconv.FormatInt(guid, 10)
		seedBytes, _ := hex.DecodeString(p6Digest([]byte(fightID + "/" + common.SourceHash + "/" + quote.SourceHash + "/" + strconv.FormatInt(common.SeedSalt, 10))))
		input := model.FrozenBattleInput{Attacker: attacker, Monster: monster, Seed: int64(binary.BigEndian.Uint32(seedBytes[:4]) & 0x7fffffff), MaxRound: common.MaxRounds, BattleType: 14, IsAttToDef: false, DefenderFirst: false, AttackerDamageFactor: common.AttackerDamageFactor, DefenderDamageFactor: common.DefenderDamageFactor, AppliedProperties: applied}
		input.ClientVersion = match.ClientVersion
		originalEngine, err := s.p13UFBattleAuditFrozenEngine(tx, match, applied)
		if err != nil {
			return err
		}
		recordID, err := originalEngine.CaptureBattleReplay(ctx, tx, id, constUtil.P13UnionFightGuardAuditP6Source, fightID, input)
		if err != nil {
			return err
		}
		outcome, err := originalEngine.ComputeBattleReplay(ctx, tx, id, constUtil.P13UnionFightGuardAuditP6Source, fightID, nil)
		if err != nil {
			return err
		}
		if outcome.Authoritative {
			return errors.New("守卫审计未允许权威战争结算切换")
		}
		var record db.P6BattleRecord
		if err = tx.First(&record, "id=?", recordID).Error; err != nil {
			return err
		}
		attempt := db.P13UnionFightGuardAuditAttempt{MatchID: match.ID, GUID: guid, PlayerID: id, GuardGUID: guard.GetGuid(), Stamp: stamp, RequestHash: requestHash, OfferHash: quote.SourceHash, P6RecordID: recordID, InputHash: record.InputHash, OutcomeHash: record.OutcomeHash, ReplayHash: record.ReplayHash, Refusal: constUtil.P13UnionFightBattleAuditRefusal, CreatedAt: now.UnixMilli()}
		attempt.SourceHash = p13UFGuardAttemptHash(&attempt)
		if err = p13UFGuardAuditVerifyRecord(tx, &attempt); err != nil {
			return err
		}
		return tx.Create(&attempt).Error
	})
	if err != nil {
		return nil, err
	}
	return nil, errors.New(constUtil.P13UnionFightBattleAuditRefusal)
}
func p13UFGuardAuditVerifyRecord(tx *gorm.DB, attempt *db.P13UnionFightGuardAuditAttempt) error {
	var record db.P6BattleRecord
	if err := tx.First(&record, "id=?", attempt.P6RecordID).Error; err != nil {
		return err
	}
	return p13UFGuardAuditVerifyLoaded(attempt, &record)
}

func p13UFGuardAuditVerifyLoaded(attempt *db.P13UnionFightGuardAuditAttempt, record *db.P6BattleRecord) error {
	if record.PlayerID != attempt.PlayerID || record.Source != constUtil.P13UnionFightGuardAuditP6Source || record.SourceFight != strconv.FormatInt(attempt.GUID, 10) || record.Status != constUtil.P6BattleComputed || record.Authoritative || record.InputHash != attempt.InputHash || record.OutcomeHash != attempt.OutcomeHash || record.ReplayHash != attempt.ReplayHash || p6Digest(record.Outcome) != record.OutcomeHash {
		return errors.New("守卫实际计算审计SHA/身份不符")
	}
	input, err := decodeP6Record(record)
	if err != nil {
		return err
	}
	if input.BattleType != 14 || input.IsAttToDef || input.DefenderFirst || input.Defender != nil || input.Attacker.GetPlayerId() != attempt.PlayerID || input.Monster.GetMonsterId() != attempt.GuardGUID {
		return errors.New("原守卫PVE14输入映射损坏")
	}
	var outcome model.CombatOutcome
	if json.Unmarshal(record.Outcome, &outcome) != nil || outcome.Authoritative {
		return errors.New("守卫实际计算结果损坏")
	}
	operate, err := proto.MarshalOptions{Deterministic: true}.Marshal(outcome.Operates)
	if err != nil || !bytes.Equal(operate, record.Operates) || record.ServerResult != record.ClientResult || record.ServerResult != p6PlaybackResult(outcome.Winner) || p6Digest(append(append([]byte(nil), operate...), byte(record.ClientResult))) != record.ReplayHash {
		return errors.New("守卫实际计算操作SHA损坏")
	}
	return nil
}
