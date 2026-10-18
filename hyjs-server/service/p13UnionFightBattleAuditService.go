package service

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
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

func (s *GameService) InitializeP13UnionFightBattleAudit(ctx context.Context) error {
	return db.GetIns().WithContext(ctx).AutoMigrate(&db.P13UnionFightBattleAuditSource{}, &db.P13UnionFightBattleAuditOffer{}, &db.P13UnionFightBattleAuditAttempt{})
}

func p13UFBattleAuditOfferHash(row *db.P13UnionFightBattleAuditOffer) string {
	copy := *row
	copy.SourceHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}

func p13UFBattleAuditAttemptHash(row *db.P13UnionFightBattleAuditAttempt) string {
	copy := *row
	copy.SourceHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}

func p13UFBattleAuditFactorValid(value *float64) bool {
	return value == nil || (*value >= 0 && *value <= 10000 && !math.IsNaN(*value) && !math.IsInf(*value, 0))
}

func p13UFBattleAuditQuoteValid(quote *db.P13UnionFightBattleAuditOffer) bool {
	return quote.SourceHash == p13UFBattleAuditOfferHash(quote) && quote.SeedSalt >= 0 && quote.ArriveRange == .8 && quote.MaxRounds == 40 && p13UFBattleAuditFactorValid(quote.AttackerDamageFactor) && p13UFBattleAuditFactorValid(quote.DefenderDamageFactor) && strings.TrimSpace(quote.Source) != ""
}

func (s *GameService) p13UFBattleAuditBuild(tx *gorm.DB, match *db.P13UnionFightMatch, original *GameService, now time.Time) error {
	var source db.P13UnionFightBattleAuditSource
	err := tx.First(&source, "day=? AND enabled=1", match.Day).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	rangeValue, err := strconv.ParseFloat(original.table("GameConfigure", "StaticName", "UnionFightArriveOther")["NameValue"], 64)
	if err != nil || rangeValue != .8 || source.SeedSalt < 0 || !p13UFBattleAuditFactorValid(source.AttackerDamageFactor) || !p13UFBattleAuditFactorValid(source.DefenderDamageFactor) || source.PublishedAt <= 0 || source.PublishedAt > now.UnixMilli() || strings.TrimSpace(source.Source) == "" {
		return errors.New("原公会战14/0.8与提前具名审计种子/倍率来源缺失")
	}
	quote := db.P13UnionFightBattleAuditOffer{MatchID: match.ID, SeedSalt: source.SeedSalt, ArriveRange: rangeValue, MaxRounds: 40, AttackerDamageFactor: source.AttackerDamageFactor, DefenderDamageFactor: source.DefenderDamageFactor, Source: constUtil.P13UnionFightBattleAuditSource + "；发布：" + source.Source}
	quote.SourceHash = p13UFBattleAuditOfferHash(&quote)
	return tx.Create(&quote).Error
}

// p13UFAskFightAudit 同事务保存真实当前输入/实际P6计算审计；结算门禁在提交后返回，审计不会随拒绝回滚。
func (s *GameService) p13UFAskFightAudit(ctx context.Context, id int64, request *pb.CSUFPlayerAskFight, stamp int64, now time.Time) ([]proto.Message, error) {
	if request == nil || request.GetTargetGuid() <= 0 || request.GetTargetGuid() == id || stamp <= 0 {
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
		actor, match, _, frame, err := s.p13UFFieldCurrent(tx, id, now)
		if err != nil {
			return err
		}
		if actor.Hooked || (frame.GetState() != 0 && frame.GetState() != 5) || now.UnixMilli() < match.StartAt || now.UnixMilli() >= match.EndAt {
			return errors.New("当前角色不能发起公会战战斗")
		}
		var guardReceipt db.P13UnionFightGuardAuditAttempt
		if err = tx.First(&guardReceipt, "match_id=? AND player_id=? AND stamp=?", match.ID, id, stamp).Error; err == nil {
			return errors.New("原AskFight时间戳已绑定守卫战斗")
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var quote db.P13UnionFightBattleAuditOffer
		if err = tx.First(&quote, "match_id=?", match.ID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New(constUtil.P13UnionFightBattleAuditRefusal)
			}
			return err
		}
		if !p13UFBattleAuditQuoteValid(&quote) {
			return errors.New("公会战审计冻结来源SHA损坏")
		}
		raw := []byte(match.ID + "\x00" + strconv.FormatInt(id, 10) + "\x00" + strconv.FormatInt(request.GetTargetGuid(), 10))
		requestHash := p6Digest(raw)
		var previous db.P13UnionFightBattleAuditAttempt
		err = tx.First(&previous, "match_id=? AND player_id=? AND stamp=?", match.ID, id, stamp).Error
		if err == nil {
			if previous.SourceHash != p13UFBattleAuditAttemptHash(&previous) || previous.RequestHash != requestHash || previous.OfferHash != quote.SourceHash || previous.TargetID != request.GetTargetGuid() || previous.Refusal != constUtil.P13UnionFightBattleAuditRefusal {
				return errors.New("公会战审计回执冲突")
			}
			return p13UFBattleAuditVerifyRecord(tx, &previous)
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		target, targetMatch, _, targetFrame, err := s.p13UFFieldCurrent(tx, request.GetTargetGuid(), now)
		if err != nil {
			return err
		}
		if targetMatch.ID != match.ID || target.Camp == actor.Camp || target.GuildID == actor.GuildID || target.Hooked || (targetFrame.GetState() != 0 && targetFrame.GetState() != 5) || p13UFResourceDistance(actor, target.X, target.Y, target.Z) > quote.ArriveRange {
			return errors.New("目标尚未在本场原敌方抵达范围内")
		}
		attacker, applied, err := s.p13UFFieldBattle(tx, id, now)
		if err != nil {
			return err
		}
		defender, targetApplied, err := s.p13UFFieldBattle(tx, target.PlayerID, now)
		if err != nil {
			return err
		}
		for guid, sources := range targetApplied {
			if len(applied[guid]) > 0 {
				return errors.New("原战斗双方冻结英雄GUID重复")
			}
			applied[guid] = sources
		}
		guid := p13UFFieldGUID(match.ID, "battle-audit", strconv.FormatInt(id, 10)+"/"+strconv.FormatInt(stamp, 10))
		fightID := strconv.FormatInt(guid, 10)
		seedBytes, _ := hex.DecodeString(p6Digest([]byte(fightID + "/" + quote.SourceHash + "/" + strconv.FormatInt(quote.SeedSalt, 10))))
		seed := int64(binary.BigEndian.Uint32(seedBytes[:4]) & 0x7fffffff)
		input := model.FrozenBattleInput{Attacker: attacker, Defender: defender, Seed: seed, MaxRound: quote.MaxRounds, BattleType: 14, IsAttToDef: false, DefenderFirst: attacker.GetInst().GetFirstOrder() == 0 && defender.GetInst().GetFirstOrder() == 1, AttackerDamageFactor: quote.AttackerDamageFactor, DefenderDamageFactor: quote.DefenderDamageFactor, AppliedProperties: applied}
		input.ClientVersion = match.ClientVersion
		originalEngine, err := s.p13UFBattleAuditFrozenEngine(tx, match, applied)
		if err != nil {
			return err
		}
		recordID, err := originalEngine.CaptureBattleReplay(ctx, tx, id, constUtil.P13UnionFightBattleAuditP6Source, fightID, input)
		if err != nil {
			return err
		}
		outcome, err := originalEngine.ComputeBattleReplay(ctx, tx, id, constUtil.P13UnionFightBattleAuditP6Source, fightID, nil)
		if err != nil {
			return err
		}
		// 公共P6目前false；原UF14尚未golden，审计链不执行战争HP/分数/胜负/道具结算。
		if outcome.Authoritative {
			return errors.New("公会战审计链未允许权威结算切换")
		}
		var computed db.P6BattleRecord
		if err = tx.First(&computed, "id=?", recordID).Error; err != nil {
			return err
		}
		attempt := db.P13UnionFightBattleAuditAttempt{MatchID: match.ID, GUID: guid, PlayerID: id, TargetID: target.PlayerID, Stamp: stamp, RequestHash: requestHash, OfferHash: quote.SourceHash, P6RecordID: recordID, InputHash: computed.InputHash, OutcomeHash: computed.OutcomeHash, ReplayHash: computed.ReplayHash, Refusal: constUtil.P13UnionFightBattleAuditRefusal, CreatedAt: now.UnixMilli()}
		attempt.SourceHash = p13UFBattleAuditAttemptHash(&attempt)
		if err = p13UFBattleAuditVerifyRecord(tx, &attempt); err != nil {
			return err
		}
		return tx.Create(&attempt).Error
	})
	if err != nil {
		return nil, err
	}
	return nil, errors.New(constUtil.P13UnionFightBattleAuditRefusal)
}

func p13UFBattleAuditVerifyRecord(tx *gorm.DB, attempt *db.P13UnionFightBattleAuditAttempt) error {
	var record db.P6BattleRecord
	if err := tx.First(&record, "id=?", attempt.P6RecordID).Error; err != nil {
		return err
	}
	if record.PlayerID != attempt.PlayerID || record.Source != constUtil.P13UnionFightBattleAuditP6Source || record.SourceFight != strconv.FormatInt(attempt.GUID, 10) || record.Status != constUtil.P6BattleComputed || record.Authoritative || record.InputHash != attempt.InputHash || record.OutcomeHash != attempt.OutcomeHash || record.ReplayHash != attempt.ReplayHash || p6Digest(record.Outcome) != record.OutcomeHash {
		return errors.New("公会战审计计算实例SHA/身份不符")
	}
	input, err := decodeP6Record(&record)
	if err != nil {
		return err
	}
	if input.BattleType != 14 || input.IsAttToDef || input.Attacker.GetPlayerId() != attempt.PlayerID || input.Defender.GetPlayerId() != attempt.TargetID {
		return errors.New("公会战原14输入/角色映射损坏")
	}
	var outcome model.CombatOutcome
	if json.Unmarshal(record.Outcome, &outcome) != nil || outcome.Authoritative {
		return errors.New("公会战审计结果损坏")
	}
	operate, err := proto.MarshalOptions{Deterministic: true}.Marshal(outcome.Operates)
	if err != nil || !bytes.Equal(operate, record.Operates) || record.ServerResult != record.ClientResult || record.ServerResult != p6PlaybackResult(outcome.Winner) || p6Digest(append(append([]byte(nil), operate...), byte(record.ClientResult))) != record.ReplayHash {
		return errors.New("公会战计算操作SHA损坏")
	}
	return nil
}
