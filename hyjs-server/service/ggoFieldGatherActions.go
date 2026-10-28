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
	"hyjs-server/model"
)

func ggoClaimHash(startID, token string) string  { return p6Digest([]byte(startID + "\x00" + token)) }
func ggoCancelHash(startID, token string) string { return p6Digest([]byte(startID + "\x00" + token)) }

func ggoReadStart(tx *gorm.DB, id int64, startID string) (*model.GGOFieldGatherStartResponse, string, error) {
	var row db.GGOFieldReceipt
	err := tx.First(&row, "player_id=? AND request_id=?", id, startID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, "unknown", nil
	}
	if err != nil {
		return nil, "", err
	}
	if row.Operation == constUtil.GGOFieldOpGatherTombstone {
		return nil, "cancelled", nil
	}
	if row.Operation != constUtil.GGOFieldOpGatherStart {
		return nil, "", ggoFieldError(constUtil.GGOFieldRequestConflict, "开始编号已用于其他操作")
	}
	var out model.GGOFieldGatherStartResponse
	if err = json.Unmarshal(row.Response, &out); err != nil {
		return nil, "", err
	}
	return &out, "pending", nil
}

func ggoCancelAttempt(tx *gorm.DB, s *GameService, id int64, g *model.GameState, attempt *db.P8GatherAttempt, requestID, requestHash string, now int64) (*model.GGOFieldGatherCancelResponse, error) {
	frozen, err := ggoFrozenAttempt(attempt)
	if err != nil {
		return nil, err
	}
	var row db.GGOFieldLine
	if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "server_id=? AND line=?", s.OriginalServerID(), frozen.Line).Error; err != nil {
		return nil, err
	}
	var state ggoLineState
	if json.Unmarshal(row.Payload, &state) != nil {
		return nil, errors.New("沙漠场景状态损坏")
	}
	var node *ggoResourceNode
	for i := range state.Resources {
		if state.Resources[i].PointID == attempt.PosID {
			node = &state.Resources[i]
			break
		}
	}
	if node == nil || node.Generation != attempt.Generation || node.ReservedBy != id || node.AttemptToken != attempt.Token {
		return nil, errors.New("沙漠采集预约状态不一致")
	}
	node.ReservedBy = 0
	node.AttemptToken = ""
	node.ReadyAtMS = 0
	attempt.Active = false
	if err = ggoLineSave(tx, &row, &state, now); err != nil {
		return nil, err
	}
	if err = tx.Save(attempt).Error; err != nil {
		return nil, err
	}
	if g.Field != nil && g.Field.Side != nil && (g.Field.Side.GetState() == constUtil.P8FieldFishing || g.Field.Side.GetState() == constUtil.P8FieldMining) {
		g.Field.Side.State = proto.Int32(constUtil.P8FieldIdle)
	}
	out := &model.GGOFieldGatherCancelResponse{RequestID: requestID, StartRequestID: frozen.StartRequestID, Status: "cancelled", AttemptToken: attempt.Token}
	key := attempt.Token
	if err = ggoSaveReceipt(tx, id, requestID, constUtil.GGOFieldOpGatherCancel, requestHash, frozen.EconomyVersion, &key, [6]int64{}, 0, now, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *GameService) GGOFieldGatherClaim(ctx context.Context, req model.GGOFieldGatherClaimRequest, session []byte) (*model.GGOFieldGatherClaimResponse, error) {
	var out model.GGOFieldGatherClaimResponse
	full := false
	err := s.ggoWithProfile(ctx, req.GGOFieldAuth, session, false, false, func(tx *gorm.DB, a *ggoFieldActor) error {
		hash := ggoClaimHash(req.StartRequestID, req.AttemptToken)
		if found, err := ggoReadReceipt(tx, a.Row.Id, req.RequestID, constUtil.GGOFieldOpGatherClaim, hash, &out); err != nil || found {
			return err
		}
		started, status, err := ggoReadStart(tx, a.Row.Id, req.StartRequestID)
		if err != nil {
			return err
		}
		if status != "pending" || started == nil {
			return ggoFieldError(constUtil.GGOFieldStaleGeneration, "采集请求不存在或已取消")
		}
		if started.AttemptToken != req.AttemptToken {
			return ggoFieldError(constUtil.GGOFieldRequestConflict, "采集编号不匹配")
		}
		var existing model.GGOFieldGatherClaimResponse
		if found, err := ggoFindBusinessReceipt(tx, a.Row.Id, constUtil.GGOFieldOpGatherClaim, req.AttemptToken, &existing); err != nil {
			return err
		} else if found {
			out = existing
			return ggoSaveReceipt(tx, a.Row.Id, req.RequestID, constUtil.GGOFieldOpGatherClaim, hash, "", nil, [6]int64{}, 0, a.Now, out)
		}
		var cancelled model.GGOFieldGatherCancelResponse
		if found, err := ggoFindBusinessReceipt(tx, a.Row.Id, constUtil.GGOFieldOpGatherCancel, req.AttemptToken, &cancelled); err != nil {
			return err
		} else if found {
			return ggoFieldError(constUtil.GGOFieldStaleGeneration, "采集已取消")
		}
		var pending ggoPendingGather
		if found, err := ggoFindBusinessReceipt(tx, a.Row.Id, constUtil.GGOFieldOpGatherReady, req.AttemptToken, &pending); err != nil {
			return err
		} else if found {
			if pending.StartRequestID != req.StartRequestID {
				return ggoFieldError(constUtil.GGOFieldRequestConflict, "采集编号不匹配")
			}
			result, isFull, err := ggoDeliverPending(tx, a, &pending, req.RequestID)
			if err != nil {
				return err
			}
			full = isFull
			if result != nil {
				out = *result
			}
			return nil
		}
		var attempt db.P8GatherAttempt
		if err = tx.First(&attempt, "player_id=? AND active=?", a.Row.Id, true).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("采集回执与预约不一致")
			}
			return err
		}
		if attempt.Token != req.AttemptToken {
			return ggoFieldError(constUtil.GGOFieldStaleGeneration, "采集已发生变化")
		}
		result, isFull, err := ggoEndAttempt(tx, a, &attempt, req.RequestID)
		if err != nil {
			return err
		}
		full = isFull
		if result != nil {
			out = *result
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if full {
		return nil, ggoFieldError(constUtil.GGOFieldInventoryFull, "背包容量不足，原奖励已保留")
	}
	return &out, nil
}

func (s *GameService) GGOFieldGatherStatus(ctx context.Context, req model.GGOFieldGatherStatusRequest, session []byte) (*model.GGOFieldGatherStatusResponse, error) {
	out := &model.GGOFieldGatherStatusResponse{StartRequestID: req.StartRequestID}
	err := s.ggoWithProfile(ctx, req.GGOFieldAuth, session, false, false, func(tx *gorm.DB, a *ggoFieldActor) error {
		start, status, err := ggoReadStart(tx, a.Row.Id, req.StartRequestID)
		if err != nil {
			return err
		}
		out.Status = status
		if start == nil {
			return nil
		}
		out.AttemptToken = start.AttemptToken
		out.AcceptedAtMS = start.AcceptedAtMS
		out.ReadyAtMS = start.ReadyAtMS
		out.FatigueCost = start.FatigueCost
		var claim model.GGOFieldGatherClaimResponse
		if found, err := ggoFindBusinessReceipt(tx, a.Row.Id, constUtil.GGOFieldOpGatherClaim, start.AttemptToken, &claim); err != nil {
			return err
		} else if found {
			out.Status = "settled"
			out.ClaimReceipt = &claim
			return nil
		}
		var pending ggoPendingGather
		if found, err := ggoFindBusinessReceipt(tx, a.Row.Id, constUtil.GGOFieldOpGatherReady, start.AttemptToken, &pending); err != nil {
			return err
		} else if found {
			out.Status = "pending_delivery"
			return nil
		}
		var cancelled model.GGOFieldGatherCancelResponse
		if found, err := ggoFindBusinessReceipt(tx, a.Row.Id, constUtil.GGOFieldOpGatherCancel, start.AttemptToken, &cancelled); err != nil {
			return err
		} else if found {
			out.Status = "cancelled"
			return nil
		}
		var attempt db.P8GatherAttempt
		if err = tx.First(&attempt, "player_id=? AND token=? AND active=?", a.Row.Id, start.AttemptToken, true).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("采集回执与预约不一致")
			}
			return err
		}
		return nil
	})
	return out, err
}

func (s *GameService) GGOFieldGatherCancel(ctx context.Context, req model.GGOFieldGatherCancelRequest, session []byte) (*model.GGOFieldGatherCancelResponse, error) {
	var out model.GGOFieldGatherCancelResponse
	err := s.ggoWithProfile(ctx, req.GGOFieldAuth, session, false, false, func(tx *gorm.DB, a *ggoFieldActor) error {
		hash := ggoCancelHash(req.StartRequestID, req.AttemptToken)
		if found, err := ggoReadReceipt(tx, a.Row.Id, req.RequestID, constUtil.GGOFieldOpGatherCancel, hash, &out); err != nil || found {
			return err
		}
		start, status, err := ggoReadStart(tx, a.Row.Id, req.StartRequestID)
		if err != nil {
			return err
		}
		if status == "unknown" {
			tomb := model.GGOFieldGatherCancelResponse{RequestID: req.StartRequestID, StartRequestID: req.StartRequestID, Status: "cancelled"}
			if err = ggoSaveReceipt(tx, a.Row.Id, req.StartRequestID, constUtil.GGOFieldOpGatherTombstone, "", "", nil, [6]int64{}, 0, a.Now, tomb); err != nil {
				return err
			}
			out = model.GGOFieldGatherCancelResponse{RequestID: req.RequestID, StartRequestID: req.StartRequestID, Status: "cancelled"}
			return ggoSaveReceipt(tx, a.Row.Id, req.RequestID, constUtil.GGOFieldOpGatherCancel, hash, "", nil, [6]int64{}, 0, a.Now, out)
		}
		if status == "cancelled" {
			out = model.GGOFieldGatherCancelResponse{RequestID: req.RequestID, StartRequestID: req.StartRequestID, Status: "already_cancelled"}
			return ggoSaveReceipt(tx, a.Row.Id, req.RequestID, constUtil.GGOFieldOpGatherCancel, hash, "", nil, [6]int64{}, 0, a.Now, out)
		}
		if start == nil || req.AttemptToken != "" && req.AttemptToken != start.AttemptToken {
			return ggoFieldError(constUtil.GGOFieldRequestConflict, "采集编号不匹配")
		}
		out = model.GGOFieldGatherCancelResponse{RequestID: req.RequestID, StartRequestID: req.StartRequestID, AttemptToken: start.AttemptToken}
		var claim model.GGOFieldGatherClaimResponse
		if found, err := ggoFindBusinessReceipt(tx, a.Row.Id, constUtil.GGOFieldOpGatherClaim, start.AttemptToken, &claim); err != nil {
			return err
		} else if found {
			out.Status = "settled"
			out.ClaimReceipt = &claim
			return ggoSaveReceipt(tx, a.Row.Id, req.RequestID, constUtil.GGOFieldOpGatherCancel, hash, "", nil, [6]int64{}, 0, a.Now, out)
		}
		var pending ggoPendingGather
		if found, err := ggoFindBusinessReceipt(tx, a.Row.Id, constUtil.GGOFieldOpGatherReady, start.AttemptToken, &pending); err != nil {
			return err
		} else if found {
			out.Status = "pending_delivery"
			return ggoSaveReceipt(tx, a.Row.Id, req.RequestID, constUtil.GGOFieldOpGatherCancel, hash, "", nil, [6]int64{}, 0, a.Now, out)
		}
		var previous model.GGOFieldGatherCancelResponse
		if found, err := ggoFindBusinessReceipt(tx, a.Row.Id, constUtil.GGOFieldOpGatherCancel, start.AttemptToken, &previous); err != nil {
			return err
		} else if found {
			out.Status = "already_cancelled"
			return ggoSaveReceipt(tx, a.Row.Id, req.RequestID, constUtil.GGOFieldOpGatherCancel, hash, "", nil, [6]int64{}, 0, a.Now, out)
		}
		var attempt db.P8GatherAttempt
		if err = tx.First(&attempt, "player_id=? AND token=? AND active=?", a.Row.Id, start.AttemptToken, true).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("采集回执与预约不一致")
			}
			return err
		}
		frozen, err := ggoFrozenAttempt(&attempt)
		if err != nil {
			return err
		}
		canFinish := false
		if a.Now >= attempt.ReadyAt {
			canFinish, err = ggoGatherCanFinish(tx, a, &attempt, frozen)
			if err != nil {
				return err
			}
		}
		if canFinish {
			result, full, err := ggoEndAttempt(tx, a, &attempt, ggoSystemReceiptID("claim", attempt.Token))
			if err != nil {
				return err
			}
			if full {
				out.Status = "pending_delivery"
			} else {
				out.Status = "settled"
				out.ClaimReceipt = result
			}
			return ggoSaveReceipt(tx, a.Row.Id, req.RequestID, constUtil.GGOFieldOpGatherCancel, hash, frozen.EconomyVersion, nil, [6]int64{}, 0, a.Now, out)
		}
		cancelled, err := ggoCancelAttempt(tx, a.Service, a.Row.Id, a.Game, &attempt, req.RequestID, hash, a.Now)
		if err != nil {
			return err
		}
		if err = saveProfile(tx, a.Row, a.Player, a.Game); err != nil {
			return err
		}
		out = *cancelled
		return nil
	})
	if err != nil {
		return nil, err
	}
	if out.Status == "" {
		return nil, fmt.Errorf("沙漠取消结果缺失")
	}
	return &out, nil
}
