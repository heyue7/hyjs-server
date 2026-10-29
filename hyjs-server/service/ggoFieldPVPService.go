package service

import (
	"context"
	"errors"
	"math"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Caller holds both profile rows in ascending ID order, as well as the native
// certified PvP result. This function never locks another player's profile.
func (s *GameService) ggoSettlePVP(tx *gorm.DB, winnerID, loserID int64, winner, loser *model.GameState, fightID string, now int64) error {
	cfg := s.GGOFieldConfig
	if cfg == nil || cfg.Runtime == nil || winner == nil || loser == nil || winner.GGOField == nil || loser.GGOField == nil || loser.Field == nil || loser.Field.Side == nil || fightID == "" {
		return errors.New("沙漠PK终局无效")
	}
	if loser.GGOField.PVPReturn != nil {
		return errors.New("前次沙漠死亡尚未返回")
	}
	if _, err := ggoWirePoint(cfg.Runtime.SafeReturn); err != nil {
		return err
	}
	heal := tableNumber(s.table("GameConfigure", "StaticName", "Resurrection2Safe"), "NameValue")
	if math.IsNaN(heal) || math.IsInf(heal, 0) || heal <= 0 || heal > 1 {
		return errors.New("沙漠返回恢复值无效")
	}
	_, winnerTotal, err := ggoProtectedChips(winner.GGOField.Chips, cfg.Runtime.ProtectedChips)
	if err != nil {
		return err
	}
	_, loserTotal, err := ggoProtectedChips(loser.GGOField.Chips, cfg.Runtime.ProtectedChips)
	if err != nil {
		return err
	}
	lost, err := ggoDeathLoss(loser.GGOField.Chips, cfg.Runtime.ProtectedChips, cfg.Rules.DeathLossPercent, nil)
	if err != nil {
		return err
	}
	var count int64
	for _, n := range lost {
		count += n
	}
	if count > 0 {
		if winnerTotal > math.MaxInt64-count {
			return errors.New("胜者芯片总数溢出")
		}
		if loser.GGOField.EconomyVersion == "" {
			return errors.New("沙漠芯片经济版本缺失")
		}
		for i, n := range lost {
			if n > 0 && loser.GGOField.ExchangeValues[i] < 1 {
				return errors.New("沙漠芯片冻结价格缺失")
			}
		}
		if winnerTotal > 0 && (winner.GGOField.EconomyVersion != loser.GGOField.EconomyVersion || winner.GGOField.ExchangeValues != loser.GGOField.ExchangeValues) {
			return ggoFieldError(constUtil.GGOFieldEconomyConflict, "双方芯片经济版本不同，请先结算")
		}
		if loser.GGOField.Revision == math.MaxInt64 || winner.GGOField.Revision == math.MaxInt64 {
			return errors.New("沙漠终端版本溢出")
		}
		if err = ggoApplyChipTransfer(&loser.GGOField.Chips, &winner.GGOField.Chips, lost); err != nil {
			return err
		}
		if winnerTotal == 0 {
			winner.GGOField.EconomyVersion = loser.GGOField.EconomyVersion
			winner.GGOField.ExchangeValues = loser.GGOField.ExchangeValues
		}
		loser.GGOField.Revision++
		winner.GGOField.Revision++
	}
	// A certified winner makes the other side the defeated side even when the
	// original round limit leaves nonzero HP. Safe return waits for presentation.
	loser.Field.Side.CurHp = proto.Float64(0)
	loser.Field.Side.State = proto.Int32(constUtil.P8FieldDead)
	loser.GGOField.PVPReturn = &model.GGOFieldPVPReturn{FightID: fightID, Position: cfg.Runtime.SafeReturn, HealRatio: heal}
	receipt := model.GGOFieldPVPReceipt{FightID: fightID, WinnerID: winnerID, LoserID: loserID, Lost: lost}
	version := loser.GGOField.EconomyVersion
	var debit [6]int64
	for i, n := range lost {
		debit[i] = -n
	}
	key := fightID
	if err = ggoSaveReceipt(tx, loserID, ggoSystemReceiptID("pvp-loss", fightID), constUtil.GGOFieldOpPVPLoss, p6Digest([]byte(fightID)), version, &key, debit, winnerID, now, receipt); err != nil {
		return err
	}
	if err = ggoSaveReceipt(tx, winnerID, ggoSystemReceiptID("pvp-win", fightID), constUtil.GGOFieldOpPVPWin, p6Digest([]byte(fightID)), version, &key, lost, loserID, now, receipt); err != nil {
		return err
	}
	_ = loserTotal
	return nil
}

func (s *GameService) GGOFieldPVPReceipt(ctx context.Context, req model.GGOFieldPVPReceiptRequest, session []byte) (*model.GGOFieldPVPReceipt, error) {
	var out model.GGOFieldPVPReceipt
	found := false
	err := s.ggoWithProfile(ctx, req.GGOFieldAuth, session, false, false, func(tx *gorm.DB, a *ggoFieldActor) error {
		for _, operation := range []string{constUtil.GGOFieldOpPVPWin, constUtil.GGOFieldOpPVPLoss} {
			yes, e := ggoFindBusinessReceipt(tx, a.Row.Id, operation, req.FightID, &out)
			if e != nil {
				return e
			}
			if yes {
				found = true
				break
			}
		}
		if !found {
			return ggoFieldError(constUtil.GGOFieldStaleGeneration, "沙漠PK回执不存在")
		}
		if a.Game.GGOField != nil {
			out.TerminalRevision = a.Game.GGOField.Revision
		}
		out.CreditBalance = ggoCreditBalance(a.Game)
		if out.CreditBalance < 0 {
			return errors.New("信用余额无效")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

type ggoPVPRejectedReceipt struct {
	Stamp          int64  `json:"stamp"`
	OpponentID     int64  `json:"opponentId"`
	NativeResponse []byte `json:"nativeResponse"`
}

func ggoPVPRejectID(stamp int64) string {
	return ggoSystemReceiptID("pvp-reject", strconv.FormatInt(stamp, 10))
}

func ggoReadPVPRejection(tx *gorm.DB, id, stamp, opponent int64, hash string) (*pb.SCSidePvpFightResult, bool, error) {
	var saved ggoPVPRejectedReceipt
	found, err := ggoReadReceipt(tx, id, ggoPVPRejectID(stamp), constUtil.GGOFieldOpPVPReject, hash, &saved)
	if err != nil || !found {
		return nil, found, err
	}
	if saved.Stamp != stamp || saved.OpponentID != opponent {
		return nil, true, ggoFieldError(constUtil.GGOFieldRequestConflict, "PK请求编号与原请求不一致")
	}
	var response pb.SCSidePvpFightResult
	if proto.Unmarshal(saved.NativeResponse, &response) != nil || (response.GetCode() != 2001 && response.GetCode() != 2002 && response.GetCode() != 2007) || response.GetFightId() != "" {
		return nil, true, errors.New("沙漠PK拒绝回执损坏")
	}
	return &response, true, nil
}

// GGOFieldPVPStatus recovers the initiator's committed native result even
// after a reconnect or scene change. Unknown means retry the same stamp/body.
func (s *GameService) GGOFieldPVPStatus(ctx context.Context, req model.GGOFieldPVPStatusRequest, session []byte) (*model.GGOFieldPVPStatusResponse, error) {
	var request pb.CSSidePvpFightResult
	if req.Stamp < 1 || len(req.NativeRequest) < 1 || len(req.NativeRequest) > 1024 || proto.Unmarshal(req.NativeRequest, &request) != nil || request.GetPlayerId() < 1 || request.GetPlayerId() == req.PlayerID || request.GetServerId() < 0 || request.GetFightType() < 0 || request.GetFightType() > 1 {
		return nil, model.NewErrParamsError()
	}
	canonical, err := proto.MarshalOptions{Deterministic: true}.Marshal(&request)
	if err != nil {
		return nil, err
	}
	hash, opponent := p6Digest(canonical), request.GetPlayerId()
	out := &model.GGOFieldPVPStatusResponse{Status: "unknown", Stamp: req.Stamp, OpponentID: opponent, RequestHash: hash}
	err = s.ggoWithProfile(ctx, req.GGOFieldAuth, session, false, false, func(tx *gorm.DB, a *ggoFieldActor) error {
		out.ServerTimeMS = time.Now().UnixMilli()
		if a.Game.GGOField != nil {
			out.PositionRevision = a.Game.GGOField.PositionRevision
		}
		var row db.P12FieldPVPReceipt
		if err := tx.Where("player_id=? AND stamp=?", a.Row.Id, req.Stamp).First(&row).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			rejected, found, e := ggoReadPVPRejection(tx, a.Row.Id, req.Stamp, opponent, hash)
			if e != nil || !found {
				return e
			}
			wire, e := proto.MarshalOptions{Deterministic: true}.Marshal(rejected)
			if e != nil {
				return e
			}
			out.Status, out.NativeResponse = "rejected", wire
			return nil
		}
		if row.OpponentID != opponent || row.RequestHash != hash {
			return ggoFieldError(constUtil.GGOFieldRequestConflict, "PK请求编号与原请求不一致")
		}
		var proof db.GGOFieldReceipt
		if err := tx.Where("player_id=? AND business_key=? AND operation IN ?", a.Row.Id, row.FightID, []string{constUtil.GGOFieldOpPVPWin, constUtil.GGOFieldOpPVPLoss}).First(&proof).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ggoFieldError(constUtil.GGOFieldRequestConflict, "请求编号不属于沙漠PK")
			}
			return err
		}
		if _, _, err := a.Service.p12FieldPVPReadReceipt(tx, &row); err != nil {
			return err
		}
		out.Status, out.FightID, out.WinnerID, out.NativeResponse = "settled", row.FightID, row.WinnerID, append([]byte(nil), row.Response...)
		// Native presentation leases use the realm game clock; return a real
		// server-clock deadline so a device does not inherit its day offset.
		remaining := row.BusyUntil - LocalPlayNow(tx.Statement.Context).UnixMilli()
		if remaining > 0 && remaining <= constUtil.P12FieldPVPBusyMS {
			out.PresentationUntilMS = out.ServerTimeMS + remaining
		}
		var ownState, peerState db.P12FieldPVPState
		if err := tx.First(&ownState, "player_id=?", a.Row.Id).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err := tx.First(&peerState, "player_id=?", opponent).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if a.Game.Position == nil || a.Game.Position.GetScene() != constUtil.GGOFieldScene || a.Game.Field == nil || a.Game.Field.Side == nil || ownState.LastFightID != row.FightID || ownState.BusyUntil != row.BusyUntil || peerState.LastFightID != row.FightID || peerState.BusyUntil != row.BusyUntil || remaining <= 0 || remaining > constUtil.P12FieldPVPBusyMS {
			return nil
		}
		var peerRow db.Profile
		if err := tx.First(&peerRow, "id=?", opponent).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		_, peerGame, err := decodeProfile(&peerRow)
		if err != nil {
			return err
		}
		if peerGame.Position == nil || peerGame.Position.GetScene() != constUtil.GGOFieldScene || peerGame.Field == nil || peerGame.Field.Side == nil {
			return nil
		}
		at := LocalPlayNow(tx.Statement.Context)
		ownSide, err := a.Service.ProjectP12FieldPVPSide(tx, a.Row.Id, a.Game.Field.Side, at)
		if err != nil {
			return err
		}
		peerSide, err := a.Service.ProjectP12FieldPVPSide(tx, opponent, peerGame.Field.Side, at)
		if err != nil {
			return err
		}
		out.ServerTimeMS = time.Now().UnixMilli()
		remaining = row.BusyUntil - LocalPlayNow(tx.Statement.Context).UnixMilli()
		if remaining <= 0 || remaining > constUtil.P12FieldPVPBusyMS {
			return nil
		}
		out.PresentationUntilMS = out.ServerTimeMS + remaining
		out.NativeState, err = proto.MarshalOptions{Deterministic: true}.Marshal(&pb.SCFieldPlayerState{Player: []*pb.PlayerSide{ownSide, peerSide}})
		if err != nil {
			return err
		}
		out.PresentationCurrent = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ggoPVPFinishReturn runs under the profile lock after the native animation lease.
func (s *GameService) ggoPVPFinishReturn(tx *gorm.DB, p *pb.Player, g *model.GameState, fightID string, now int64) (bool, error) {
	if g.GGOField == nil || g.GGOField.PVPReturn == nil {
		return false, nil
	}
	pending := g.GGOField.PVPReturn
	if pending.FightID != fightID || strings.TrimSpace(fightID) == "" {
		return false, errors.New("沙漠PK返回记录不匹配")
	}
	point, err := ggoWirePoint(pending.Position)
	if err != nil {
		return false, err
	}
	if g.Field == nil || g.Field.Side == nil || pending.HealRatio <= 0 || pending.HealRatio > 1 || math.IsNaN(pending.HealRatio) || math.IsInf(pending.HealRatio, 0) {
		return false, errors.New("沙漠PK返回状态无效")
	}
	if err = s.p8Heal(p, g, pending.HealRatio, true); err != nil {
		return false, err
	}
	if err = ggoPositionBump(g); err != nil {
		return false, err
	}
	g.Position.Current = point
	g.PositionLoading = true
	g.Field.Side.State = proto.Int32(constUtil.P8FieldIdle)
	g.Field.Side.Anger = proto.Float64(0)
	state, err := p8LifeState(tx, p.GetPlayerId())
	if err != nil {
		return false, err
	}
	state.DeathAt = 0
	state.SafeRecoveryAt = now
	state.InSafe = true
	state.LastFightID = fightID
	if err = tx.Save(state).Error; err != nil {
		return false, err
	}
	g.GGOField.PVPReturn = nil
	return true, nil
}
