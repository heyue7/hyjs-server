package service

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func ggoExtractionStartHash(point string) string  { return p6Digest([]byte(point)) }
func ggoExtractionCancelHash(start string) string { return p6Digest([]byte(start)) }
func ggoSnapshotPoint(p *pb.PointNew) *model.AdminFieldGGOPosition {
	if p == nil {
		return nil
	}
	return &model.AdminFieldGGOPosition{X: ggoFloat64(float64(p.GetX())), Y: ggoFloat64(float64(p.GetY())), Z: ggoFloat64(float64(p.GetZ()))}
}

func (s *GameService) GGOFieldExtractionStart(ctx context.Context, req model.GGOFieldExtractionStartRequest, session []byte) (*model.GGOFieldExtractionStartResponse, error) {
	var out model.GGOFieldExtractionStartResponse
	accepted := false
	// An existing player can still return to safety if the minimum build changes.
	err := s.ggoWithProfile(ctx, req.GGOFieldAuth, session, true, true, func(tx *gorm.DB, a *ggoFieldActor) error {
		hash := ggoExtractionStartHash(req.PointID)
		var prior db.GGOFieldReceipt
		err := tx.First(&prior, "player_id=? AND request_id=?", a.Row.Id, req.RequestID).Error
		if err == nil {
			if prior.Operation == constUtil.GGOFieldOpExtractTombstone {
				return ggoFieldError(constUtil.GGOFieldStaleGeneration, "撤离请求已取消")
			}
			if prior.Operation != constUtil.GGOFieldOpExtractStart || prior.RequestHash != hash {
				return ggoFieldError(constUtil.GGOFieldRequestConflict, "请求编号已用于其他操作")
			}
			return json.Unmarshal(prior.Response, &out)
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		g := a.Game
		cfg := a.Service.GGOFieldConfig
		if g.GGOField == nil || g.PositionLoading || g.Field.Side.GetCurHp() <= 0 || g.Field.Fight != nil && !g.Field.Fight.Settled || g.Run != nil && !g.Run.Complete || g.GGOField.Extraction != nil {
			return ggoFieldError(constUtil.GGOFieldBusy, "当前无法撤离")
		}
		if err = P12FieldPVPRequireIdle(tx, a.Row.Id, time.UnixMilli(a.Now)); err != nil {
			return err
		}
		var attempt db.P8GatherAttempt
		if err = tx.Where("player_id=? AND active=?", a.Row.Id, true).First(&attempt).Error; err == nil {
			return ggoFieldError(constUtil.GGOFieldBusy, "请先结束采集")
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var point *model.AdminFieldGGOPoint
		for i := range cfg.Runtime.Points {
			if cfg.Runtime.Points[i].ID == req.PointID && cfg.Runtime.Points[i].Kind == "exit" {
				point = &cfg.Runtime.Points[i]
				break
			}
		}
		if point == nil {
			return ggoFieldError(constUtil.GGOFieldInvalidLocation, "撤离点不存在")
		}
		position, err := ggoWirePoint(point.Position)
		if err != nil {
			return err
		}
		if !p8Near(g.Position.GetCurrent(), &p8GatherConfig{Point: position, Radius: point.Radius}) {
			return ggoFieldError(constUtil.GGOFieldInvalidLocation, "请靠近撤离点")
		}
		if cfg.Rules.ExtractionSeconds == nil || *cfg.Rules.ExtractionSeconds < 1 || *cfg.Rules.ExtractionSeconds > 300 {
			return errors.New("沙漠撤离配置无效")
		}
		if _, err = ggoWirePoint(cfg.Runtime.SafeReturn); err != nil {
			return err
		}
		out = model.GGOFieldExtractionStartResponse{RequestID: req.RequestID, PointID: req.PointID, AcceptedAtMS: a.Now, ReadyAtMS: a.Now + *cfg.Rules.ExtractionSeconds*1000}
		g.GGOField.Extraction = &model.GGOFieldExtraction{RequestID: req.RequestID, PointID: req.PointID, Line: int64(a.Presence.Line), AcceptedAt: out.AcceptedAtMS, ReadyAt: out.ReadyAtMS, Origin: ggoSnapshotPoint(g.Position.GetCurrent()), SafeReturn: cfg.Runtime.SafeReturn}
		if err = saveProfile(tx, a.Row, a.Player, g); err != nil {
			return err
		}
		if err = ggoSaveReceipt(tx, a.Row.Id, req.RequestID, constUtil.GGOFieldOpExtractStart, hash, cfg.Runtime.EconomyVersion, nil, [6]int64{}, 0, a.Now, out); err != nil {
			return err
		}
		update := tx.Model(&db.GGOFieldReceipt{}).Where("player_id=? AND request_id=?", a.Row.Id, req.RequestID).Updates(map[string]any{"active": true, "ready_at": out.ReadyAtMS})
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected != 1 {
			return errors.New("撤离倒计时未持久化")
		}
		accepted = true
		return nil
	})
	if err == nil && accepted {
		s.ggoScheduleExtraction(ctx, req.PlayerID, req.RequestID, out.ReadyAtMS)
	}
	return &out, err
}

func ggoExtractionRead(tx *gorm.DB, id int64, startID string) (*model.GGOFieldExtractionStartResponse, string, error) {
	var row db.GGOFieldReceipt
	if err := tx.First(&row, "player_id=? AND request_id=?", id, startID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, "unknown", nil
		}
		return nil, "", err
	}
	if row.Operation == constUtil.GGOFieldOpExtractTombstone {
		return nil, "cancelled", nil
	}
	if row.Operation != constUtil.GGOFieldOpExtractStart {
		return nil, "", ggoFieldError(constUtil.GGOFieldRequestConflict, "撤离编号已用于其他操作")
	}
	var out model.GGOFieldExtractionStartResponse
	if err := json.Unmarshal(row.Response, &out); err != nil {
		return nil, "", err
	}
	return &out, "pending", nil
}

func ggoExtractionCancelLocked(tx *gorm.DB, id int64, g *model.GameState, reason string, now int64) (*model.GGOFieldExtractionStatusResponse, error) {
	if g == nil || g.GGOField == nil || g.GGOField.Extraction == nil {
		return nil, errors.New("撤离倒计时缺失")
	}
	x := g.GGOField.Extraction
	result := &model.GGOFieldExtractionStatusResponse{RequestID: x.RequestID, StartRequestID: x.RequestID, Status: "cancelled", AcceptedAtMS: x.AcceptedAt, ReadyAtMS: x.ReadyAt, Reason: reason}
	changed := tx.Model(&db.GGOFieldReceipt{}).Where("player_id=? AND request_id=? AND operation=? AND active=?", id, x.RequestID, constUtil.GGOFieldOpExtractStart, true).Update("active", false)
	if changed.Error != nil {
		return nil, changed.Error
	}
	if changed.RowsAffected != 1 {
		return nil, errors.New("撤离倒计时状态不一致")
	}
	key := x.RequestID
	if err := ggoSaveReceipt(tx, id, ggoSystemReceiptID("extract-cancel", key), constUtil.GGOFieldOpExtractCancel, ggoExtractionCancelHash(key), g.GGOField.EconomyVersion, &key, [6]int64{}, 0, now, result); err != nil {
		return nil, err
	}
	g.GGOField.Extraction = nil
	return result, nil
}

// ggoInterruptExtractionLocked is called inside the same player transaction as
// the accepted action. At the deadline the scheduled result resolves first.
func ggoInterruptExtractionLocked(tx *gorm.DB, id int64, g *model.GameState, now int64) error {
	if g == nil || g.GGOField == nil || g.GGOField.Extraction == nil {
		return nil
	}
	if now >= g.GGOField.Extraction.ReadyAt {
		return ggoFieldError(constUtil.GGOFieldBusy, "撤离结算中，请稍后重试")
	}
	_, err := ggoExtractionCancelLocked(tx, id, g, "action", now)
	return err
}

func ggoExtractionLive(tx *gorm.DB, a *ggoFieldActor, x *model.GGOFieldExtraction, disconnectedSession []byte) (bool, error) {
	if disconnectedSession == nil {
		if value, ok := p10ClientSinks.Load(p10ClientKey{a.Service.p10ClientOwner(), a.Row.Id}); ok && value.(*p10ClientSink).disconnectedAt.Load() != 0 {
			// The serialized disconnect owns the ReadyAt decision and receipt.
			return false, errors.New("断线撤离清理尚未完成")
		}
	}
	g := a.Game
	if x == nil || g.Position == nil || g.Position.GetScene() != constUtil.GGOFieldScene || g.PositionLoading || g.Field == nil || g.Field.Side == nil || g.Field.Side.GetCurHp() <= 0 ||
		g.Field.Fight != nil && !g.Field.Fight.Settled || g.Run != nil && !g.Run.Complete {
		return false, nil
	}
	origin, err := ggoWirePoint(x.Origin)
	if err != nil {
		return false, err
	}
	current := g.Position.GetCurrent()
	if current == nil || current.GetX() != origin.GetX() || current.GetY() != origin.GetY() || current.GetZ() != origin.GetZ() {
		return false, nil
	}
	if err = P12FieldPVPRequireIdle(tx, a.Row.Id, time.UnixMilli(a.Now)); err != nil {
		if err.Error() == constUtil.P12FieldPVPBusy || err.Error() == constUtil.P12FieldPVPJailed {
			return false, nil
		}
		return false, err
	}
	var presence db.P10ScenePresence
	if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&presence, "player_id=?", a.Row.Id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	activeSession := disconnectedSession
	if activeSession == nil {
		v, ok := p10ClientSinks.Load(p10ClientKey{a.Service.p10ClientOwner(), a.Row.Id})
		if !ok {
			return false, nil
		}
		sink := v.(*p10ClientSink)
		if sink.disconnectedAt.Load() != 0 {
			return false, errors.New("断线撤离清理尚未完成")
		}
		activeSession = sink.session
	}
	if len(activeSession) != 16 || presence.Scene != constUtil.GGOFieldScene || int64(presence.Line) != x.Line || presence.SessionHash != p6Digest(activeSession) || subtle.ConstantTimeCompare(a.Row.Session, activeSession) != 1 {
		return false, nil
	}
	if a.Row.AccountID > 0 {
		account, e := AccountForProfile(tx, a.Row)
		if e != nil {
			return false, e
		}
		if !account.Enabled || account.AuthRevision != a.Row.AccountAuthRevision {
			return false, nil
		}
	}
	return true, nil
}

func ggoExtractionFinishLocked(tx *gorm.DB, a *ggoFieldActor) (*model.GGOFieldExtractionStatusResponse, error) {
	return ggoExtractionFinishWithSessionLocked(tx, a, nil)
}

func ggoExtractionFinishWithSessionLocked(tx *gorm.DB, a *ggoFieldActor, disconnectedSession []byte) (*model.GGOFieldExtractionStatusResponse, error) {
	x := a.Game.GGOField.Extraction
	if x == nil || a.Now < x.ReadyAt {
		return nil, ggoFieldError(constUtil.GGOFieldNotReady, "撤离尚未完成")
	}
	live, err := ggoExtractionLive(tx, a, x, disconnectedSession)
	if err != nil {
		return nil, err
	}
	if !live {
		return ggoExtractionCancelLocked(tx, a.Row.Id, a.Game, "interrupted", a.Now)
	}
	point, err := ggoWirePoint(x.SafeReturn)
	if err != nil {
		return nil, err
	}
	if err = ggoPositionBump(a.Game); err != nil {
		return nil, err
	}
	result := &model.GGOFieldExtractionStatusResponse{RequestID: x.RequestID, StartRequestID: x.RequestID, Status: "completed", AcceptedAtMS: x.AcceptedAt, ReadyAtMS: x.ReadyAt, CompletedAtMS: a.Now, Position: x.SafeReturn, PositionRevision: a.Game.GGOField.PositionRevision}
	update := tx.Model(&db.GGOFieldReceipt{}).Where("player_id=? AND request_id=? AND operation=? AND active=?", a.Row.Id, x.RequestID, constUtil.GGOFieldOpExtractStart, true).Update("active", false)
	if update.Error != nil {
		return nil, update.Error
	}
	if update.RowsAffected != 1 {
		return nil, errors.New("撤离倒计时状态不一致")
	}
	key := x.RequestID
	if err = ggoSaveReceipt(tx, a.Row.Id, ggoSystemReceiptID("extract-complete", key), constUtil.GGOFieldOpExtractComplete, ggoExtractionCancelHash(key), a.Game.GGOField.EconomyVersion, &key, [6]int64{}, 0, a.Now, result); err != nil {
		return nil, err
	}
	a.Game.Position.Current = point
	a.Game.PositionLoading = true
	a.Game.Field.Side.State = proto.Int32(constUtil.P8FieldIdle)
	a.Game.GGOField.Extraction = nil
	return result, nil
}

func ggoExtractionStatusLocked(tx *gorm.DB, a *ggoFieldActor, startID string) (*model.GGOFieldExtractionStatusResponse, bool, error) {
	start, status, err := ggoExtractionRead(tx, a.Row.Id, startID)
	if err != nil {
		return nil, false, err
	}
	out := &model.GGOFieldExtractionStatusResponse{RequestID: startID, StartRequestID: startID, Status: status}
	if start == nil {
		return out, false, nil
	}
	out.AcceptedAtMS, out.ReadyAtMS = start.AcceptedAtMS, start.ReadyAtMS
	for _, operation := range []string{constUtil.GGOFieldOpExtractComplete, constUtil.GGOFieldOpExtractCancel} {
		var previous model.GGOFieldExtractionStatusResponse
		found, e := ggoFindBusinessReceipt(tx, a.Row.Id, operation, startID, &previous)
		if e != nil {
			return nil, false, e
		}
		if found {
			return &previous, false, nil
		}
	}
	if a.Game.GGOField == nil || a.Game.GGOField.Extraction == nil || a.Game.GGOField.Extraction.RequestID != startID {
		return nil, false, errors.New("撤离回执与倒计时不一致")
	}
	if a.Now < start.ReadyAtMS {
		return out, false, nil
	}
	result, e := ggoExtractionFinishLocked(tx, a)
	return result, e == nil, e
}

func (s *GameService) GGOFieldExtractionStatus(ctx context.Context, req model.GGOFieldExtractionStatusRequest, session []byte) (*model.GGOFieldExtractionStatusResponse, error) {
	var out *model.GGOFieldExtractionStatusResponse
	err := s.ggoWithProfile(ctx, req.GGOFieldAuth, session, false, false, func(tx *gorm.DB, a *ggoFieldActor) error {
		result, changed, err := ggoExtractionStatusLocked(tx, a, req.StartRequestID)
		if err != nil {
			return err
		}
		out = result
		if changed {
			return saveProfile(tx, a.Row, a.Player, a.Game)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (s *GameService) GGOFieldExtractionCancel(ctx context.Context, req model.GGOFieldExtractionCancelRequest, session []byte) (*model.GGOFieldExtractionStatusResponse, error) {
	var out model.GGOFieldExtractionStatusResponse
	err := s.ggoWithProfile(ctx, req.GGOFieldAuth, session, false, false, func(tx *gorm.DB, a *ggoFieldActor) error {
		hash := ggoExtractionCancelHash(req.StartRequestID)
		if found, err := ggoReadReceipt(tx, a.Row.Id, req.RequestID, constUtil.GGOFieldOpExtractCancel, hash, &out); err != nil || found {
			return err
		}
		start, status, err := ggoExtractionRead(tx, a.Row.Id, req.StartRequestID)
		if err != nil {
			return err
		}
		if status == "unknown" {
			tomb := model.GGOFieldExtractionStatusResponse{RequestID: req.StartRequestID, StartRequestID: req.StartRequestID, Status: "cancelled"}
			if err = ggoSaveReceipt(tx, a.Row.Id, req.StartRequestID, constUtil.GGOFieldOpExtractTombstone, "", "", nil, [6]int64{}, 0, a.Now, tomb); err != nil {
				return err
			}
			out = tomb
		} else if start == nil {
			out = model.GGOFieldExtractionStatusResponse{StartRequestID: req.StartRequestID, Status: "cancelled"}
		} else {
			result, changed, e := ggoExtractionStatusLocked(tx, a, req.StartRequestID)
			if e != nil {
				return e
			}
			if result.Status == "pending" {
				result, e = ggoExtractionCancelLocked(tx, a.Row.Id, a.Game, "cancelled", a.Now)
				if e != nil {
					return e
				}
				changed = true
			}
			if changed {
				if e = saveProfile(tx, a.Row, a.Player, a.Game); e != nil {
					return e
				}
			}
			out = *result
		}
		out.RequestID = req.RequestID
		return ggoSaveReceipt(tx, a.Row.Id, req.RequestID, constUtil.GGOFieldOpExtractCancel, hash, "", nil, [6]int64{}, 0, a.Now, out)
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}
