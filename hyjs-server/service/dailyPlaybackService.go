package service

import (
	"context"
	"database/sql"
	"errors"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// dailyPlaybackProjection恢复既有已结算Daily的原播放协议，三包与独立开始回执共同核对归属。
// 旧Daily表没有持久化Hash，当前计算的摘要不能证明防篡改；本函数不改表、不补写授权或历史记录。
func dailyPlaybackProjection(row *db.DailyCopyFight, receipt *db.DailyCopyStartReceipt) (*pb.SCFightPlayback, error) {
	conflict := errors.New(constUtil.P6ReplayConflict)
	if row == nil || receipt == nil || !row.Settled || row.PlayerID < 1 || row.StartStamp <= 0 || row.Type == "" || row.ChapterID == "" || len(row.Start) == 0 || len(row.EndRequest) == 0 || len(row.Response) == 0 || receipt.PlayerID != row.PlayerID || receipt.Stamp != row.StartStamp || receipt.FightID != row.FightID || receipt.ChapterID != row.ChapterID {
		return nil, conflict
	}
	start, err := decodeDailyCopyStart(row)
	if err != nil || start.GetMaxRoundNum() > 40 || start.GetRandSeed() < 0 || start.GetBattlePlayer().GetServerId() != 1 || start.GetMonster().GetTaskId() != row.ChapterID {
		return nil, conflict
	}
	end := &pb.CSDailyCopyFightEnd{}
	settled := &pb.SCDailyCopyFightEnd{}
	if proto.Unmarshal(row.EndRequest, end) != nil || proto.Unmarshal(row.Response, settled) != nil || end.GetFightId() != row.FightID || end.GetChapterId() != row.ChapterID || validateDailyCopyEnd(start, end) != nil || settled.GetIsWin() != end.GetIsWin() || settled.GetLastTime() < 0 || len(settled.Cost) != 1 || settled.Cost[0].GetId() != "stamina" || settled.Cost[0].GetNum() != row.Stamina || settled.Cost[0].GetGuid() != "" {
		return nil, conflict
	}
	if end.GetIsWin() == 0 && (settled.GetStarNum() != 0 || proto.Size(settled.Get) != 0) || end.GetIsWin() == 1 && settled.GetStarNum() != row.WinStars {
		return nil, conflict
	}
	// 原负闪避等属性保留，只拒绝非有限浮点、缺实例及非法HP；不复算当前表或修改原角色。
	for _, side := range [][]*pb.BattleHero{start.GetBattlePlayer().GetFormation(), start.GetMonster().GetFormation()} {
		for _, actor := range side {
			valid := actor != nil && actor.GetId() != "" && actor.GetLevel() > 0 && actor.GetPose() > 0 && actor.GetPose() <= 15 && actor.GetProp() != nil && actor.GetInst() != nil && actor.GetProp().GetHp() > 0 && actor.GetInst().GetCurHp() > 0 && actor.GetInst().GetCurHp() <= actor.GetProp().GetHp()
			if !valid {
				return nil, conflict
			}
			actor.GetProp().ProtoReflect().Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
				if field.Kind() == protoreflect.DoubleKind && !combatFinite(value.Float()) {
					valid = false
					return false
				}
				return true
			})
			if !valid || !combatFinite(actor.GetInst().GetCurHp()) {
				return nil, conflict
			}
		}
	}
	if !combatFinite(start.GetBattlePlayer().GetInst().GetAnger()) || start.GetBattlePlayer().GetInst().GetAnger() < 0 || !combatFinite(start.GetMonster().GetInst().GetAnger()) || start.GetMonster().GetInst().GetAnger() < 0 {
		return nil, conflict
	}
	// Daily isWin=0是真实失败，原SCFightPlayback的0=胜/1=负需要显式反向映射。
	result := int32(1)
	if end.GetIsWin() == 1 {
		result = 0
	}
	return &pb.SCFightPlayback{Fightid: start.FightId, RandSeed: start.RandSeed, BattleType: proto.Int32(4), Attacker: start.BattlePlayer, Defender2: start.Monster, OptRecord: end.OptRecord, ServerResult: proto.Int32(result), ClientResult: proto.Int32(result), AttackerFactor: proto.Float64(1), DefenderFactor: proto.Float64(1), MaxRound: start.MaxRoundNum}, nil
}

// dailyPlayback仅在P6来源查询确无记录时调用；已存在P6的Hash/结果冲突由原路径拒绝，不降级恢复。
func (s *GameService) dailyPlayback(ctx context.Context, id int64, fightID string) ([]proto.Message, error) {
	var reply *pb.SCFightPlayback
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		if !gameSchemaHasTable(tx, &db.DailyCopyFight{}) || !gameSchemaHasTable(tx, &db.DailyCopyStartReceipt{}) {
			return errors.New(constUtil.P6ReplayUnavailable)
		}
		var row db.DailyCopyFight
		err := tx.First(&row, "fight_id=? AND player_id=? AND settled=?", fightID, id, true).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New(constUtil.P6ReplayUnavailable)
		}
		if err != nil {
			return err
		}
		var receipt db.DailyCopyStartReceipt
		if err := tx.First(&receipt, "player_id=? AND stamp=?", id, row.StartStamp).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New(constUtil.P6ReplayConflict)
			}
			return err
		}
		reply, err = dailyPlaybackProjection(&row, &receipt)
		return err
	}, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	return []proto.Message{reply}, nil
}
