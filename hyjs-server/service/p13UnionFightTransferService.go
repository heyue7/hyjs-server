package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"math"
	"strconv"
	"time"
)

func (s *GameService) InitializeP13UnionFightTransfer(ctx context.Context) error {
	return db.GetIns().WithContext(ctx).AutoMigrate(&db.P13UnionFightTransferTask{})
}

func p13UFTransferDigest(task *db.P13UnionFightTransferTask) string {
	raw, _ := json.Marshal(struct {
		ID, MatchID       string
		PlayerID, Ordinal int64
		TransferID        string
		Target            []byte
		StartedAt, DueAt  int64
		Source            string
	}{task.ID, task.MatchID, task.PlayerID, task.Ordinal, task.TransferID, task.Target, task.StartedAt, task.DueAt, task.Source})
	return p6Digest(raw)
}
func p13UFTransferTarget(task *db.P13UnionFightTransferTask) (*pb.PointNew, error) {
	if task.SourceHash == "" || task.SourceHash != p13UFTransferDigest(task) {
		return nil, errors.New("公会战原传送来源SHA损坏")
	}
	point := &pb.PointNew{}
	if err := proto.Unmarshal(task.Target, point); err != nil {
		return nil, err
	}
	return point, nil
}
func p13UFTransferSame(a, b *pb.PointNew) bool {
	return a != nil && b != nil && a.GetX() == b.GetX() && a.GetY() == b.GetY() && a.GetZ() == b.GetZ()
}

// Only the trusted frozen endpoint is projected. Client coordinates still
// have to equal this exact result and historical task bytes remain unchanged.
func p13UFTransferProjectedTarget(actor *db.P13UnionFightFieldPlayer, target *pb.PointNew) (*pb.PointNew, error) {
	if actor == nil || target == nil || !p13UFMapProjectionKnown(actor.MapProjection) {
		return nil, errors.New("公会战传送投影无效")
	}
	point := proto.Clone(target).(*pb.PointNew)
	if actor.MapProjection == constUtil.P13UnionFightMapProjectionReFlat {
		point.Y = proto.Float32(0)
	}
	return point, nil
}

// p13UFTransferBegin 保存本场原目标和真实到期时刻；客户端target不能生成新落点。
func (s *GameService) p13UFTransferBegin(ctx context.Context, id int64, request *pb.CSUFBeginTransfer, now time.Time) ([]proto.Message, error) {
	var reply *pb.SCUFBeginTransfer
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
		if actor.Hooked || now.UnixMilli() >= match.EndAt {
			return errors.New("原挂机或比赛结束不能传送")
		}
		var row db.Profile
		if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error; err != nil {
			return err
		}
		_, game, err := decodeProfileForCalendarUpdate(tx, &row)
		if err != nil {
			return err
		}
		if game.PositionLoading || game.Position.GetScene() != constUtil.P13UnionFightFieldTarget {
			return errors.New("原Current场景加载握手尚未完成")
		}
		var pending db.P13UnionFightTransferTask
		err = tx.First(&pending, "match_id=? AND player_id=? AND status=?", match.ID, id, constUtil.P13UnionFightTransferPending).Error
		if err == nil {
			target, err := p13UFTransferTarget(&pending)
			if err != nil {
				return err
			}
			if pending.TransferID != request.GetTransferId() || !p13UFTransferSame(target, request.GetTarget()) {
				return errors.New("角色已有不同传送尚未结束")
			}
			reply = &pb.SCUFBeginTransfer{Guid: proto.Int64(id), Target: target, CostTime: proto.Float64(float64(max(int64(0), pending.DueAt-now.UnixMilli())) / 1000)}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if frame.GetState() != 0 {
			return errors.New("原非空闲角色不能开始新传送")
		}
		original, err := s.p13UFFieldOriginal(match, tx)
		if err != nil {
			return err
		}
		transfer := original.table("UnionFightTransfer", "UFTransferId", request.GetTransferId())
		if transfer == nil || transfer["Camp"] != strconv.FormatInt(actor.Camp, 10) {
			return errors.New("原传送点不属于本阵营")
		}
		if transfer["PassRole"] != "0" {
			return errors.New("原传送点身份限制尚未恢复")
		}
		target, err := fieldPoint(transfer["TransferEnd"])
		if err != nil {
			return err
		}
		target, err = p13UFTransferProjectedTarget(actor, target)
		if err != nil {
			return err
		}
		if !p13UFTransferSame(target, request.GetTarget()) {
			return errors.New("传送目标必须等于本场冻结原终点")
		}
		switch transfer["TransferType"] {
		case "0":
			start, err := fieldPoint(transfer["TransferStart"])
			if err != nil {
				return err
			}
			radius, err := strconv.ParseFloat(transfer["StartR"], 64)
			if err != nil || radius <= 0 {
				return errors.New("原固定传送入口半径缺失")
			}
			if math.Hypot(float64(actor.X-start.GetX()), float64(actor.Z-start.GetZ())) >= radius {
				return errors.New("原固定传送入口水平距离不足")
			}
		case "1":
			if now.UnixMilli() < match.StartAt {
				return errors.New("原回城传送按钮仅Fight阶段")
			}
		default:
			return errors.New("原传送类型尚未恢复")
		}
		seconds, err := strconv.ParseFloat(transfer["TransferTime"], 64)
		if err != nil || seconds <= 0 || seconds > 86400 || seconds*1000 != math.Trunc(seconds*1000) {
			return errors.New("原传送时长无法精确冻结")
		}
		if now.UnixMilli()+int64(seconds*1000) > match.EndAt {
			return errors.New("本场剩余时间不足完整原传送")
		}
		var count int64
		if err = tx.Model(&db.P13UnionFightTransferTask{}).Where("match_id=? AND player_id=?", match.ID, id).Count(&count).Error; err != nil {
			return err
		}
		raw, err := proto.Marshal(target)
		if err != nil {
			return err
		}
		task := db.P13UnionFightTransferTask{ID: p6Digest([]byte(fmt.Sprintf("%s/%d/%d/transfer", match.ID, id, count+1))), MatchID: match.ID, PlayerID: id, Ordinal: count + 1, TransferID: request.GetTransferId(), Target: raw, StartedAt: now.UnixMilli(), DueAt: now.UnixMilli() + int64(seconds*1000), Source: constUtil.P13UnionFightTransferSource + "；本场配置：" + match.ConfigurationHash}
		task.SourceHash = p13UFTransferDigest(&task)
		if err = tx.Create(&task).Error; err != nil {
			return err
		}
		frame.State = proto.Int32(4)
		if seconds > 1 {
			frame.State = proto.Int32(12)
		}
		if err = p13UFFieldSaveActor(tx, actor, frame); err != nil {
			return err
		}
		reply = &pb.SCUFBeginTransfer{Guid: proto.Int64(id), Target: target, CostTime: proto.Float64(seconds)}
		scope = match.ID
		fresh = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	if fresh {
		s.p13UFFieldBroadcast(ctx, scope, id, reply)
	}
	return []proto.Message{reply}, nil
}

func (s *GameService) p13UFTransferCancel(tx *gorm.DB, scope string, id int64, now time.Time) error {
	var task db.P13UnionFightTransferTask
	err := tx.First(&task, "match_id=? AND player_id=? AND status=?", scope, id, constUtil.P13UnionFightTransferPending).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err = p13UFTransferTarget(&task); err != nil {
		return err
	}
	if err = tx.Model(&task).Updates(map[string]any{"status": constUtil.P13UnionFightTransferCanceled, "finished_at": now.UnixMilli()}).Error; err != nil {
		return err
	}
	var actor db.P13UnionFightFieldPlayer
	if err = tx.First(&actor, "match_id=? AND player_id=?", scope, id).Error; err != nil {
		return err
	}
	snapshot, projectionErr := p13UFFieldStoredProjection(tx, actor.MatchID)
	if projectionErr != nil {
		return projectionErr
	}
	if err = p13UFFieldActorProjection(&actor, snapshot); err != nil {
		return err
	}
	frame, err := p13UFFieldReadActor(&actor)
	if err != nil {
		return err
	}
	if frame.GetState() == 4 || frame.GetState() == 12 {
		frame.State = proto.Int32(0)
		return p13UFFieldSaveActor(tx, &actor, frame)
	}
	return nil
}

// p13UFTransferFinishTx 对原到期/角色和实例资格原子推进；失权记录只取消，不应用客户端坐标。
func (s *GameService) p13UFTransferFinishTx(tx *gorm.DB, task *db.P13UnionFightTransferTask, now time.Time) (*pb.SCUFEndTransfer, bool, error) {
	target, err := p13UFTransferTarget(task)
	if err != nil {
		return nil, false, err
	}
	if task.Status == constUtil.P13UnionFightTransferDone {
		if p6Digest(task.Result) != task.ResultHash {
			return nil, false, errors.New("公会战传送结束回执SHA损坏")
		}
		end := &pb.SCUFEndTransfer{}
		err := proto.Unmarshal(task.Result, end)
		return end, false, err
	}
	if task.Status != constUtil.P13UnionFightTransferPending {
		return nil, false, errors.New("本场传送已经撤销")
	}
	var actor db.P13UnionFightFieldPlayer
	if err = tx.First(&actor, "match_id=? AND player_id=?", task.MatchID, task.PlayerID).Error; err != nil {
		return nil, false, err
	}
	snapshot, projectionErr := p13UFFieldStoredProjection(tx, actor.MatchID)
	if projectionErr != nil {
		return nil, false, projectionErr
	}
	if err = p13UFFieldActorProjection(&actor, snapshot); err != nil {
		return nil, false, err
	}
	if actor.MapProjection == constUtil.P13UnionFightMapProjectionReFlat && target.GetY() != 0 {
		return nil, false, errors.New("公会战平面传送冻结终点高度无效")
	}
	var match db.P13UnionFightMatch
	if err = tx.First(&match, "id=?", task.MatchID).Error; err != nil {
		return nil, false, err
	}
	var field db.P13UnionFightFieldPolicy
	if err = tx.First(&field, 1).Error; err != nil {
		return nil, false, err
	}
	var gate db.P13GuildPolicy
	if err = tx.First(&gate, 1).Error; err != nil {
		return nil, false, err
	}
	var enroll db.P13UnionFightPolicy
	if err = tx.First(&enroll, 1).Error; err != nil {
		return nil, false, err
	}
	member, err := p13GuildMember(tx, task.PlayerID)
	if err != nil {
		return nil, false, err
	}
	var row db.Profile
	if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, task.PlayerID).Error; err != nil {
		return nil, false, err
	}
	player, game, err := decodeProfileForCalendarUpdate(tx, &row)
	if err != nil {
		return nil, false, err
	}
	valid := actor.LeftAt == 0 && member.GuildID == actor.GuildID && gate.ManagementEnabled && field.Enabled && enroll.EnrollmentEnabled && enroll.MatchEnabled && game.Position.GetScene() == constUtil.P13UnionFightFieldTarget && task.DueAt <= match.EndAt
	if !valid {
		return nil, false, s.p13UFTransferCancel(tx, task.MatchID, task.PlayerID, now)
	}
	if now.UnixMilli() < task.DueAt {
		return nil, false, nil
	}
	frame, err := p13UFFieldReadActor(&actor)
	if err != nil {
		return nil, false, err
	}
	if frame.GetState() != 4 && frame.GetState() != 12 {
		return nil, false, errors.New("原角色传送状态与任务不一致")
	}
	actor.X, actor.Y, actor.Z = target.GetX(), target.GetY(), target.GetZ()
	actor.UpdatedAt = now.UnixMilli()
	actor.MovementDebt = 0
	frame.State = proto.Int32(0)
	if err = p13UFFieldSaveActor(tx, &actor, frame); err != nil {
		return nil, false, err
	}
	game.Position.Current = proto.Clone(target).(*pb.PointNew)
	if err = saveProfile(tx, &row, player, game); err != nil {
		return nil, false, err
	}
	end := &pb.SCUFEndTransfer{Guid: proto.Int64(task.PlayerID), Target: target, CostTime: proto.Float64(float64(task.DueAt-task.StartedAt) / 1000)}
	task.Result, err = proto.Marshal(end)
	if err != nil {
		return nil, false, err
	}
	task.ResultHash = p6Digest(task.Result)
	task.FinishedAt = now.UnixMilli()
	task.Status = constUtil.P13UnionFightTransferDone
	return end, true, tx.Save(task).Error
}

func (s *GameService) TickP13UnionFightTransfer(ctx context.Context, now time.Time) error {
	var tasks []db.P13UnionFightTransferTask
	if err := db.GetIns().WithContext(ctx).Where("status=?", constUtil.P13UnionFightTransferPending).Order("due_at,id").Find(&tasks).Error; err != nil {
		return err
	}
	for _, candidate := range tasks {
		var ended *pb.SCUFEndTransfer
		fresh := false
		err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := ServerGameClockGuardTransaction(tx); err != nil {
				return err
			}

			var gate db.P13GuildPolicy
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&gate, 1).Error; err != nil {
				return err
			}
			var task db.P13UnionFightTransferTask
			if err := tx.First(&task, "id=?", candidate.ID).Error; err != nil {
				return err
			}
			if task.Status != constUtil.P13UnionFightTransferPending {
				return nil
			}
			var err error
			ended, fresh, err = s.p13UFTransferFinishTx(tx, &task, now)
			return err
		})
		if err != nil {
			return err
		}
		if fresh {
			s.p13UFFieldBroadcast(ctx, candidate.MatchID, 0, ended)
		}
	}
	return nil
}

// p13UFTransferEnd 保留原定义的结束请求；无原发送者可用时也只允许已保存任务目标和到期回执。
func (s *GameService) p13UFTransferEnd(ctx context.Context, id int64, request *pb.CSUFEndTransfer, now time.Time) ([]proto.Message, error) {
	var ended *pb.SCUFEndTransfer
	var scope string
	fresh := false
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		actor, _, _, _, err := s.p13UFFieldCurrent(tx, id, now)
		if err != nil {
			return err
		}
		var task db.P13UnionFightTransferTask
		if err = tx.Where("match_id=? AND player_id=?", actor.MatchID, id).Order("ordinal DESC").First(&task).Error; err != nil {
			return errors.New("角色没有当前传送来源")
		}
		target, err := p13UFTransferTarget(&task)
		if err != nil {
			return err
		}
		if !p13UFTransferSame(target, request.GetTarget()) {
			return errors.New("结束传送目标与原开始来源不一致")
		}
		ended, fresh, err = s.p13UFTransferFinishTx(tx, &task, now)
		if err != nil {
			return err
		}
		if ended == nil {
			return errors.New("原传送计时尚未结束")
		}
		scope = actor.MatchID
		return nil
	})
	if err != nil {
		return nil, err
	}
	if fresh {
		s.p13UFFieldBroadcast(ctx, scope, id, ended)
	}
	return []proto.Message{ended}, nil
}

func (s *GameService) p13UFTransferRecovery(tx *gorm.DB, actor *db.P13UnionFightFieldPlayer, now time.Time) ([]proto.Message, error) {
	snapshot, projectionErr := p13UFFieldStoredProjection(tx, actor.MatchID)
	if projectionErr != nil {
		return nil, projectionErr
	}
	if err := p13UFFieldActorProjection(actor, snapshot); err != nil {
		return nil, err
	}
	var task db.P13UnionFightTransferTask
	err := tx.First(&task, "match_id=? AND player_id=? AND status=?", actor.MatchID, actor.PlayerID, constUtil.P13UnionFightTransferPending).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	target, err := p13UFTransferTarget(&task)
	if err != nil {
		return nil, err
	}
	return []proto.Message{&pb.SCUFBeginTransfer{Guid: proto.Int64(actor.PlayerID), Target: target, CostTime: proto.Float64(float64(max(int64(0), task.DueAt-now.UnixMilli())) / 1000)}}, nil
}
