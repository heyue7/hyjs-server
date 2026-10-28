package service

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"time"

	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

func ggoLeaveHash(mode, scene string, line int32) string {
	data, _ := json.Marshal(struct {
		Mode  string `json:"mode"`
		Scene string `json:"scene"`
		Line  int32  `json:"line"`
	}{mode, scene, line})
	return p6Digest(data)
}

func (s *GameService) GGOFieldLeavePrepare(ctx context.Context, req model.GGOFieldLeavePrepareRequest, session []byte) (*model.GGOFieldLeavePrepareResponse, error) {
	var out model.GGOFieldLeavePrepareResponse
	var tradePeer int64
	err := s.ggoWithProfile(ctx, req.GGOFieldAuth, session, false, false, func(tx *gorm.DB, a *ggoFieldActor) error {
		hash := ggoLeaveHash(req.Mode, req.TargetScene, req.TargetLine)
		if found, err := ggoReadReceipt(tx, a.Row.Id, req.RequestID, constUtil.GGOFieldOpLeavePrepare, hash, &out); err != nil || found {
			return err
		}
		cfg := a.Service.GGOFieldEconomy
		if cfg == nil || cfg.Runtime == nil {
			return ggoFieldError(constUtil.GGOFieldUnavailable, "沙漠结算配置不可用")
		}
		if err := ggoCurrentPresence(tx, a, session); err != nil {
			return err
		}
		if a.Game.GGOField == nil || !ggoPointInsideSafe(a.Game.Position.GetCurrent(), cfg.Runtime.SafeArea) {
			return ggoFieldError(constUtil.GGOFieldInvalidLocation, "请回到安全区确认离开")
		}
		if a.Game.Field.Side.GetCurHp() <= 0 || a.Game.Field.Fight != nil && !a.Game.Field.Fight.Settled || a.Game.Run != nil && !a.Game.Run.Complete {
			return ggoFieldError(constUtil.GGOFieldBusy, "当前无法离开")
		}
		if err := P12FieldPVPRequireIdle(tx, a.Row.Id, time.UnixMilli(a.Now)); err != nil {
			return err
		}
		var active db.P8GatherAttempt
		if err := tx.Where("player_id=? AND active=?", a.Row.Id, true).First(&active).Error; err == nil {
			return ggoFieldError(constUtil.GGOFieldBusy, "当前无法离开")
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		switch req.Mode {
		case "scene":
			if req.TargetLine != 0 {
				return ggoFieldError(constUtil.GGOFieldInvalidLocation, "离开目标无效")
			}
			switch req.TargetScene {
			case "town", "town_main":
			case "field_001", "field_002":
				row := a.Service.fieldSceneRow(req.TargetScene)
				if row == nil || !a.Service.P8FieldSceneAvailable(req.TargetScene) || int64(a.Player.GetLevel()) < int64(tableNumber(row, "levelLimit")) {
					return ggoFieldError(constUtil.GGOFieldInvalidLocation, "目标场景不可用")
				}
			default:
				return ggoFieldError(constUtil.GGOFieldInvalidLocation, "目标场景不可用")
			}
		case "line":
			if req.TargetScene != "" || req.TargetLine < 1 || req.TargetLine == a.Presence.Line {
				return ggoFieldError(constUtil.GGOFieldInvalidLocation, "目标线路无效")
			}
			base, err := p10SceneConfig(tx)
			if err != nil {
				return err
			}
			if base == nil {
				return ggoFieldError(constUtil.GGOFieldUnavailable, "分线暂未开放")
			}
			lineCfg, err := a.Service.adminFieldSceneConfig(tx, constUtil.GGOFieldScene, base)
			if err != nil {
				return err
			}
			if lineCfg == nil || req.TargetLine > lineCfg.LineCount {
				return ggoFieldError(constUtil.GGOFieldInvalidLocation, "目标线路无效")
			}
		default:
			return ggoFieldError(constUtil.GGOFieldInvalidLocation, "离开方式无效")
		}
		tradePeer = ggoCancelTradeForActor(a.Row.Id, a.Game)
		out = model.GGOFieldLeavePrepareResponse{RequestID: req.RequestID, Status: "prepared", Mode: req.Mode, TargetScene: req.TargetScene, TargetLine: req.TargetLine, ExpiresAtMS: a.Now + 120000}
		a.Game.GGOField.LeaveIntent = &model.GGOFieldLeaveIntent{RequestID: req.RequestID, Mode: req.Mode, TargetScene: req.TargetScene, TargetLine: req.TargetLine, ExpiresAt: out.ExpiresAtMS}
		if err := saveProfile(tx, a.Row, a.Player, a.Game); err != nil {
			return err
		}
		return ggoSaveReceipt(tx, a.Row.Id, req.RequestID, constUtil.GGOFieldOpLeavePrepare, hash, a.Game.GGOField.EconomyVersion, nil, [6]int64{}, 0, a.Now, out)
	})
	if err != nil {
		return nil, err
	}
	if tradePeer > 0 {
		ggoTradeNotify(s, ctx, tradePeer)
	}
	return &out, nil
}

// Called from the native transfer transaction after its target checks. Failure
// rolls back both settlement and transition; a successful receipt records the
// converted chips alongside the same committed profile mutation.
func ggoCommitLeave(tx *gorm.DB, s *GameService, id int64, g *model.GameState, mode, scene string, line int32, now int64) error {
	if g == nil || g.GGOField == nil || g.Position == nil || s.GGOFieldEconomy == nil || s.GGOFieldEconomy.Runtime == nil {
		return ggoFieldError(constUtil.GGOFieldUnavailable, "沙漠结算配置不可用")
	}
	intent := g.GGOField.LeaveIntent
	if intent == nil || intent.RequestID == "" || intent.Mode != mode || intent.TargetScene != scene || intent.TargetLine != line || intent.ExpiresAt < now ||
		g.PositionLoading || !ggoPointInsideSafe(g.Position.GetCurrent(), s.GGOFieldEconomy.Runtime.SafeArea) {
		return ggoFieldError(constUtil.GGOFieldInvalidLocation, "请先在安全区确认当前离开目标")
	}
	late, err := s.ggoSettleLateBossAwardsLocked(tx, id, g)
	if err != nil {
		return err
	}
	result, err := ggoSettleTerminal(g, intent.RequestID)
	if err != nil {
		return err
	}
	if late < 0 || result.CreditAdded > math.MaxInt64-late {
		return ggoFieldError(constUtil.GGOFieldCreditOverflow, "信用余额超出上限，芯片仍保留")
	}
	result.CreditAdded += late
	var delta [6]int64
	for i, count := range result.Chips {
		delta[i] = -count
	}
	key := intent.RequestID
	if err = ggoSaveReceipt(tx, id, ggoSystemReceiptID("leave", key), constUtil.GGOFieldOpLeaveComplete, ggoLeaveHash(mode, scene, line), result.EconomyVersion, &key, delta, 0, now, result); err != nil {
		return err
	}
	g.GGOField.LeaveIntent = nil
	return nil
}

func (s *GameService) GGOFieldLeaveStatus(ctx context.Context, req model.GGOFieldLeaveStatusRequest, session []byte) (*model.GGOFieldLeaveStatusResponse, error) {
	out := &model.GGOFieldLeaveStatusResponse{RequestID: req.LeaveRequestID, Status: "unknown"}
	err := s.ggoWithProfile(ctx, req.GGOFieldAuth, session, false, false, func(tx *gorm.DB, a *ggoFieldActor) error {
		var row db.GGOFieldReceipt
		if err := tx.First(&row, "player_id=? AND request_id=?", a.Row.Id, req.LeaveRequestID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		if row.Operation != constUtil.GGOFieldOpLeavePrepare {
			return ggoFieldError(constUtil.GGOFieldRequestConflict, "离开编号已用于其他操作")
		}
		var prepared model.GGOFieldLeavePrepareResponse
		if err := json.Unmarshal(row.Response, &prepared); err != nil {
			return err
		}
		out.Mode, out.TargetScene, out.TargetLine, out.ExpiresAtMS = prepared.Mode, prepared.TargetScene, prepared.TargetLine, prepared.ExpiresAtMS
		var completed model.GGOFieldSettlementResponse
		found, err := ggoFindBusinessReceipt(tx, a.Row.Id, constUtil.GGOFieldOpLeaveComplete, req.LeaveRequestID, &completed)
		if err != nil {
			return err
		}
		if found {
			out.Status = "completed"
			completed.CreditBalance = ggoCreditBalance(a.Game)
			if completed.CreditBalance < 0 {
				return errors.New("信用余额无效")
			}
			if a.Game.GGOField != nil {
				completed.TerminalRevision = a.Game.GGOField.Revision
			}
			out.Settlement = &completed
			return nil
		}
		if a.Game.GGOField != nil && a.Game.GGOField.LeaveIntent != nil && a.Game.GGOField.LeaveIntent.RequestID == req.LeaveRequestID {
			if prepared.ExpiresAtMS < a.Now {
				out.Status = "expired"
			} else {
				out.Status = "prepared"
			}
		} else {
			out.Status = "superseded"
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
