package service

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// p13UFRecordIdentity 只读当前真实场次和冻结身份；不触发FieldCurrent的回血、buff或过期投影写入。
func (s *GameService) p13UFRecordIdentity(tx *gorm.DB, id int64, now time.Time) (*db.P13UnionFightMatch, error) {
	var profile db.Profile
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&profile, id).Error; err != nil {
		return nil, err
	}
	_, game, err := decodeProfile(&profile)
	if err != nil {
		return nil, err
	}
	if game.Position.GetScene() != constUtil.P13UnionFightFieldTarget {
		return nil, errors.New(constUtil.P13UFRecordUnavailable)
	}
	var actors []db.P13UnionFightFieldPlayer
	if err = tx.Where("player_id=? AND left_at=0", id).Limit(2).Find(&actors).Error; err != nil {
		return nil, err
	}
	if len(actors) != 1 {
		return nil, errors.New(constUtil.P13UFRecordUnavailable)
	}
	actor := &actors[0]
	member, _, err := p13GuildNeedMember(tx, id)
	if err != nil || member.GuildID != actor.GuildID {
		return nil, errors.New(constUtil.P13UFRecordUnavailable)
	}
	var match db.P13UnionFightMatch
	if err = tx.First(&match, "id=?", actor.MatchID).Error; err != nil {
		return nil, err
	}
	_, camp, err := p13UFFieldFighter(tx, &match, id, actor.GuildID)
	if err != nil || camp != actor.Camp {
		return nil, errors.New(constUtil.P13UFRecordUnavailable)
	}
	if _, err = p13UFFieldReadActor(actor); err != nil {
		return nil, err
	}
	var field db.P13UnionFightField
	if err = tx.First(&field, "match_id=?", match.ID).Error; err != nil {
		return nil, err
	}
	var snapshot p13UFFieldSnapshot
	if p6Digest(field.Snapshot) != field.SnapshotHash || json.Unmarshal(field.Snapshot, &snapshot) != nil || snapshot.PreparationMillis < 0 || snapshot.KickMillis < 0 || match.StartAt < snapshot.PreparationMillis || match.EndAt > math.MaxInt64-snapshot.KickMillis {
		return nil, errors.New(constUtil.P13UFRecordConflict)
	}
	if now.UnixMilli() < match.StartAt-snapshot.PreparationMillis || now.UnixMilli() >= match.EndAt+snapshot.KickMillis {
		return nil, errors.New(constUtil.P13UFRecordUnavailable)
	}
	return &match, nil
}

// p13UFRecordProjection 复用已有完整终末/已应用属性验证，再投影原列表和原录像；不新建比赛、运算或发奖。
func (s *GameService) p13UFRecordProjection(tx *gorm.DB, row *db.P13UnionFightPlayerBattleAttempt) (*pb.UFFightRecord, *pb.SCUFFightRecordReplay, error) {
	if row.Status != constUtil.P13UFPlayerBattleSettled {
		return nil, nil, errors.New(constUtil.P13UFRecordUnavailable)
	}
	if _, err := s.p13UFPlayerBattleReceipt(tx, row, row.PlayerID); err != nil {
		return nil, nil, err
	}
	fightID, err := strconv.ParseInt(row.FightID, 10, 64)
	if err != nil || fightID <= 0 {
		return nil, nil, errors.New(constUtil.P13UFRecordConflict)
	}
	var replay pb.SCUFFightRecordReplay
	var terminal model.CombatFinalHPOutcome
	var before []p13UFPlayerBattleSide
	if proto.Unmarshal(row.StartPB, &replay) != nil || json.Unmarshal(row.Terminal, &terminal) != nil || terminal.Outcome == nil || terminal.Outcome.Winner < 0 || terminal.Outcome.Winner > 1 || json.Unmarshal(row.Before, &before) != nil {
		return nil, nil, errors.New(constUtil.P13UFRecordConflict)
	}
	frames, err := p13UFPlayerBattleFrames(before)
	if err != nil || len(frames) != 2 {
		return nil, nil, errors.New(constUtil.P13UFRecordConflict)
	}
	winner := frames[terminal.Outcome.Winner].GetGuid()
	replay.WinPlayerId = proto.Int64(winner)
	record := &pb.UFFightRecord{FightId: proto.Int64(fightID), WinId: proto.Int64(winner), Time: proto.Int64(row.CreatedAt)}
	for _, frame := range frames {
		record.PlayerInfos = append(record.PlayerInfos, &pb.UFFightRecordSideInfo{Guid: frame.Guid, Camp: frame.Camp, NickName: frame.NickName, MainHeroId: frame.HeroId, FightPower: frame.GetFightInfo().FightPower, Type: proto.Int32(0)})
	}
	return record, &replay, nil
}

// p13UFRecords 返回当前实际场次已结算ordinary14记录；不把这一子集伪装成公会总击杀统计。
func (s *GameService) p13UFRecords(ctx context.Context, id int64, request proto.Message, now time.Time) ([]proto.Message, error) {
	var response proto.Message
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		match, err := s.p13UFRecordIdentity(tx, id, now)
		if err != nil {
			return err
		}
		switch body := request.(type) {
		case *pb.CSUFFightRecord:
			var rows []db.P13UnionFightPlayerBattleAttempt
			if err = tx.Where("match_id=? AND status=?", match.ID, constUtil.P13UFPlayerBattleSettled).Order("created_at DESC, id ASC").Find(&rows).Error; err != nil {
				return err
			}
			list := &pb.SCUFFightRecord{AppVersion: proto.String(constUtil.P13UFRecordOriginalAppVersion)}
			for i := range rows {
				record, _, e := s.p13UFRecordProjection(tx, &rows[i])
				if e != nil {
					return e
				}
				list.Records = append(list.Records, record)
			}
			response = list
		case *pb.CSUFFightRecordReplay:
			if body.GetFightId() <= 0 {
				return errors.New(constUtil.P13UFRecordUnavailable)
			}
			var row db.P13UnionFightPlayerBattleAttempt
			if err = tx.First(&row, "fight_id=? AND match_id=? AND status=?", strconv.FormatInt(body.GetFightId(), 10), match.ID, constUtil.P13UFPlayerBattleSettled).Error; err != nil {
				return errors.New(constUtil.P13UFRecordUnavailable)
			}
			_, replay, e := s.p13UFRecordProjection(tx, &row)
			if e != nil {
				return e
			}
			response = replay
		default:
			return errors.New(constUtil.P13UFRecordUnavailable)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{response}, nil
}
